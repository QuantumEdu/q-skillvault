package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/quantum-6/skillvault/internal/db"
	"github.com/quantum-6/skillvault/internal/domain"
)

// VaultMemSaveInput represents parameters for saving an episodic observation memory.
type VaultMemSaveInput struct {
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Type     string   `json:"type"`      // decision, architecture, workflow, bugfix, discovery, pattern, config, feedback, etc.
	Project  string   `json:"project"`   // project name or slug
	TopicKey string   `json:"topic_key"` // topic key (e.g. odd/tasks/my-feature)
	Topic    string   `json:"topic,omitempty"` // alias for TopicKey
	Scope    string   `json:"scope"`     // project, global, session
	Tags     []string `json:"tags"`      // additional custom tags
}

// VaultMemSaveResult summarizes the save or deduplication outcome.
type VaultMemSaveResult struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Type         string    `json:"type"`
	Project      string    `json:"project"`
	TopicKey     string    `json:"topic_key,omitempty"`
	Topic        string    `json:"topic,omitempty"`
	Deduplicated bool      `json:"deduplicated"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// VaultMemSearchInput represents parameters for searching observation memories.
type VaultMemSearchInput struct {
	Query    string `json:"query"`
	Project  string `json:"project"`
	TopicKey string `json:"topic_key"`
	Topic    string `json:"topic,omitempty"` // alias for TopicKey
	Type     string `json:"type"`
	Limit    int    `json:"limit"`
}

// VaultMemSearchResult represents a search or timeline match with a truncated preview.
type VaultMemSearchResult struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Project   string    `json:"project"`
	TopicKey  string    `json:"topic_key,omitempty"`
	Topic     string    `json:"topic,omitempty"`
	Preview   string    `json:"preview"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// VaultMemGetResult contains the complete content of an observation memory.
type VaultMemGetResult struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Project   string    `json:"project"`
	TopicKey  string    `json:"topic_key,omitempty"`
	Topic     string    `json:"topic,omitempty"`
	Content   string    `json:"content"`
	Summary   string    `json:"summary,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// VaultMemTimelineResult contains a target observation with surrounding chronological context.
type VaultMemTimelineResult struct {
	Target VaultMemSearchResult   `json:"target"`
	Before []VaultMemSearchResult `json:"before"`
	After  []VaultMemSearchResult `json:"after"`
}

// CalculateNormalizedHash computes a deterministic SHA256 hex digest of trimmed, lowercased content.
func CalculateNormalizedHash(content string) string {
	normalized := strings.TrimSpace(strings.ToLower(content))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// NormalizedMemoryHash is an alias for CalculateNormalizedHash.
func NormalizedMemoryHash(content string) string {
	return CalculateNormalizedHash(content)
}

// MapObservationType translates Engram observation categories into SkillVault domain types and tag names.
func MapObservationType(t string) (domain.EntryType, string) {
	clean := strings.ToLower(strings.TrimSpace(t))
	switch clean {
	case "decision":
		return domain.EntryTypeDecision, "decision"
	case "architecture":
		return domain.EntryTypeSkill, "architecture"
	case "skill":
		return domain.EntryTypeSkill, "skill"
	case "workflow":
		return domain.EntryTypeWorkflowNote, "workflow"
	case "workflow_note":
		return domain.EntryTypeWorkflowNote, "workflow"
	case "preference", "feedback":
		return domain.EntryTypeFeedback, "feedback"
	case "discovery":
		return domain.EntryTypeReference, "discovery"
	case "bugfix":
		return domain.EntryTypeReference, "bugfix"
	case "pattern":
		return domain.EntryTypeReference, "pattern"
	case "config":
		return domain.EntryTypeReference, "config"
	case "prompt":
		return domain.EntryTypePrompt, "prompt"
	default:
		if domain.EntryType(clean).IsValid() {
			return domain.EntryType(clean), clean
		}
		return domain.EntryTypeReference, clean
	}
}

// MapEngramObservationType is an alias for MapObservationType.
func MapEngramObservationType(t string) (domain.EntryType, string) {
	return MapObservationType(t)
}

func entryToSearchResult(e domain.Entry, tags []domain.Tag) VaultMemSearchResult {
	preview := e.Summary
	if preview == "" {
		preview = e.BodyOptional
	}
	if len(preview) > 300 {
		preview = preview[:300]
	}

	proj := ""
	if e.ProjectID != nil {
		proj = *e.ProjectID
	}

	topicKey := ""
	for _, t := range tags {
		if strings.HasPrefix(t.Name, "topic_key:") {
			topicKey = strings.TrimPrefix(t.Name, "topic_key:")
			break
		} else if strings.HasPrefix(t.Name, "topic:") && topicKey == "" {
			topicKey = strings.TrimPrefix(t.Name, "topic:")
		}
	}

	return VaultMemSearchResult{
		ID:        e.ID,
		Title:     e.Title,
		Type:      string(e.Type),
		Project:   proj,
		TopicKey:  topicKey,
		Topic:     topicKey,
		Preview:   preview,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

// SaveVaultMem checks for duplicates by normalized hash within the project and persists observations.
// Returns (*domain.EntryResult, deduplicated, error).
func (s *EntryService) SaveVaultMem(ctx context.Context, input VaultMemSaveInput) (*domain.EntryResult, bool, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" && strings.TrimSpace(input.Title) == "" {
		return nil, false, fmt.Errorf("content or title is required for memory observation")
	}

	hash := CalculateNormalizedHash(content)
	externalRef := "hash:" + hash

	projName := strings.TrimSpace(input.Project)
	var projectID *string
	if projName != "" {
		// Ensure project exists to allow frictionless memory capture
		p, err := s.projectStore.Get(ctx, projName)
		if err != nil {
			_ = s.projectStore.Save(ctx, domain.Project{
				ID:     projName,
				Name:   projName,
				Slug:   projName,
				Status: domain.StatusActive,
			})
			projectID = &projName
		} else {
			projectID = &p.ID
		}
	}

	// Check if active entry with external_ref already exists in the given project
	existingList, err := s.store.List(ctx, domain.EntryFilter{
		ProjectID:       projectID,
		IncludeArchived: false,
	})
	if err == nil {
		for _, item := range existingList {
			if projName == "" && item.Entry.ProjectID != nil {
				continue
			}
			if item.Entry.ExternalRef == externalRef {
				fullRes, getErr := s.store.Get(ctx, item.Entry.ID, false)
				if getErr == nil {
					return &fullRes, true, nil
				}
				return &domain.EntryResult{
					Entry: item.Entry,
					Tags:  item.Tags,
				}, true, nil
			}
		}
	}

	// Not found: map types, build tags, generate summary
	mappedType, typeTag := MapObservationType(input.Type)

	title := strings.TrimSpace(input.Title)
	if title == "" {
		lines := strings.Split(content, "\n")
		title = strings.TrimSpace(lines[0])
		if len(title) > 60 {
			title = title[:57] + "..."
		}
	}

	summary := content
	if len(summary) > 300 {
		summary = summary[:300]
	}

	topicKey := strings.TrimSpace(input.TopicKey)
	if topicKey == "" {
		topicKey = strings.TrimSpace(input.Topic)
	}

	tags := []string{"mem"}
	if typeTag != "" {
		tags = append(tags, typeTag)
	}
	if topicKey != "" {
		tags = append(tags, "topic_key:"+topicKey)
		cleanTopic := strings.ReplaceAll(topicKey, "/", "-")
		tags = append(tags, "topic:"+cleanTopic)
	}
	if input.Scope != "" {
		tags = append(tags, "scope:"+strings.ToLower(strings.TrimSpace(input.Scope)))
	}
	for _, t := range input.Tags {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}

	saveInput := SaveEntryInput{
		Title:       title,
		Type:        string(mappedType),
		Summary:     summary,
		Body:        content,
		Project:     projName,
		Tags:        tags,
		Status:      string(domain.StatusActive),
		Purpose:     string(domain.PurposeKnowledge),
		ExternalRef: externalRef,
	}

	entryRes, err := s.SaveEntry(ctx, saveInput)
	if err != nil {
		return nil, false, fmt.Errorf("save vault mem: %w", err)
	}

	return &entryRes.Entry, false, nil
}

// SearchVaultMem searches memory observations with 300-char preview truncation and filters for project, topic_key, and type.
func (s *EntryService) SearchVaultMem(ctx context.Context, input VaultMemSearchInput) ([]VaultMemSearchResult, error) {
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var projectPtr *string
	if strings.TrimSpace(input.Project) != "" {
		p := strings.TrimSpace(input.Project)
		projectPtr = &p
	}

	var typePtr *string
	if strings.TrimSpace(input.Type) != "" {
		mapped, _ := MapObservationType(input.Type)
		mappedStr := string(mapped)
		typePtr = &mappedStr
	}

	query := strings.TrimSpace(input.Query)
	var rawResults []domain.EntrySearchResult
	var err error

	if query != "" {
		searchQ := domain.SearchQuery{
			Query:           query,
			ProjectID:       projectPtr,
			Type:            typePtr,
			IncludeArchived: false,
			Limit:           limit * 2,
		}
		rawResults, err = s.store.Search(ctx, searchQ)
		if err != nil {
			return nil, fmt.Errorf("search vault mem: %w", err)
		}
	} else {
		listRes, listErr := s.store.List(ctx, domain.EntryFilter{
			ProjectID:       projectPtr,
			Type:            typePtr,
			IncludeArchived: false,
		})
		if listErr != nil {
			return nil, fmt.Errorf("list vault mem: %w", listErr)
		}
		for _, item := range listRes {
			rawResults = append(rawResults, domain.EntrySearchResult{
				Entry: item.Entry,
				Tags:  item.Tags,
			})
		}
	}

	topicKey := strings.TrimSpace(input.TopicKey)
	if topicKey == "" {
		topicKey = strings.TrimSpace(input.Topic)
	}
	cleanTopic := ""
	if topicKey != "" {
		cleanTopic = strings.ReplaceAll(topicKey, "/", "-")
	}

	var results []VaultMemSearchResult
	for _, item := range rawResults {
		if topicKey != "" {
			matched := false
			for _, tag := range item.Tags {
				tagName := strings.ToLower(tag.Name)
				if tagName == strings.ToLower("topic_key:"+topicKey) ||
					tagName == strings.ToLower("topic:"+topicKey) ||
					tagName == strings.ToLower("topic_key:"+cleanTopic) ||
					tagName == strings.ToLower("topic:"+cleanTopic) ||
					tagName == strings.ToLower(topicKey) ||
					tagName == strings.ToLower(cleanTopic) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		results = append(results, entryToSearchResult(item.Entry, item.Tags))
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// GetVaultMemTimeline retrieves entries chronologically before and after the specified entry.
func (s *EntryService) GetVaultMemTimeline(ctx context.Context, id string, before, after int, project string) ([]VaultMemSearchResult, error) {
	timeline, err := s.GetVaultMemTimelineResult(ctx, id, before, after, project)
	if err != nil {
		return nil, err
	}

	total := len(timeline.Before) + len(timeline.After)
	results := make([]VaultMemSearchResult, 0, total)
	results = append(results, timeline.Before...)
	results = append(results, timeline.After...)
	return results, nil
}

// GetVaultMemTimelineResult retrieves structured timeline context (target, before, after) around an observation.
func (s *EntryService) GetVaultMemTimelineResult(ctx context.Context, id string, before, after int, project string) (*VaultMemTimelineResult, error) {
	if before < 0 {
		before = 0
	}
	if after < 0 {
		after = 0
	}
	if before == 0 && after == 0 {
		before = 3
		after = 3
	}

	targetRes, err := s.store.Get(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("target entry %q not found: %w", id, err)
	}

	proj := strings.TrimSpace(project)
	if proj == "" && targetRes.Entry.ProjectID != nil {
		proj = *targetRes.Entry.ProjectID
	}

	var projectPtr *string
	if proj != "" {
		projectPtr = &proj
	}

	listRes, err := s.store.List(ctx, domain.EntryFilter{
		ProjectID:       projectPtr,
		IncludeArchived: false,
	})
	if err != nil {
		return nil, fmt.Errorf("fetch timeline siblings: %w", err)
	}

	type timelineItem struct {
		result    VaultMemSearchResult
		createdAt time.Time
	}

	var beforeItems []timelineItem
	var afterItems []timelineItem

	for _, item := range listRes {
		if item.Entry.ID == targetRes.Entry.ID || item.Entry.Slug == targetRes.Entry.Slug {
			continue
		}

		res := entryToSearchResult(item.Entry, item.Tags)

		if item.Entry.CreatedAt.Before(targetRes.Entry.CreatedAt) {
			beforeItems = append(beforeItems, timelineItem{result: res, createdAt: item.Entry.CreatedAt})
		} else if item.Entry.CreatedAt.After(targetRes.Entry.CreatedAt) {
			afterItems = append(afterItems, timelineItem{result: res, createdAt: item.Entry.CreatedAt})
		} else {
			if item.Entry.ID < targetRes.Entry.ID {
				beforeItems = append(beforeItems, timelineItem{result: res, createdAt: item.Entry.CreatedAt})
			} else {
				afterItems = append(afterItems, timelineItem{result: res, createdAt: item.Entry.CreatedAt})
			}
		}
	}

	// For before: sort descending (closest to target first), take before items, then reverse to chronological (ascending)
	sort.Slice(beforeItems, func(i, j int) bool {
		return beforeItems[i].createdAt.After(beforeItems[j].createdAt)
	})
	if len(beforeItems) > before {
		beforeItems = beforeItems[:before]
	}
	sort.Slice(beforeItems, func(i, j int) bool {
		return beforeItems[i].createdAt.Before(beforeItems[j].createdAt)
	})

	// For after: sort ascending (closest to target first), take after items
	sort.Slice(afterItems, func(i, j int) bool {
		return afterItems[i].createdAt.Before(afterItems[j].createdAt)
	})
	if len(afterItems) > after {
		afterItems = afterItems[:after]
	}

	beforeResults := make([]VaultMemSearchResult, len(beforeItems))
	for i, it := range beforeItems {
		beforeResults[i] = it.result
	}

	afterResults := make([]VaultMemSearchResult, len(afterItems))
	for i, it := range afterItems {
		afterResults[i] = it.result
	}

	return &VaultMemTimelineResult{
		Target: entryToSearchResult(targetRes.Entry, targetRes.Tags),
		Before: beforeResults,
		After:  afterResults,
	}, nil
}

// VaultMemService provides an optional high-level facade for memory operations.
type VaultMemService struct {
	entryStore   db.EntryStore
	projectStore db.ProjectStore
	entrySvc     *EntryService
	contextSvc   *ContextService
	db           *sql.DB
}

// NewVaultMemService creates an initialized VaultMemService.
func NewVaultMemService(
	entryStore db.EntryStore,
	projectStore db.ProjectStore,
	entrySvc *EntryService,
	contextSvc *ContextService,
) *VaultMemService {
	return &VaultMemService{
		entryStore:   entryStore,
		projectStore: projectStore,
		entrySvc:     entrySvc,
		contextSvc:   contextSvc,
	}
}

// WithDB attaches an underlying sql.DB connection.
func (s *VaultMemService) WithDB(database *sql.DB) *VaultMemService {
	s.db = database
	return s
}

// Save delegates to entrySvc.SaveVaultMem and maps to VaultMemSaveResult.
func (s *VaultMemService) Save(ctx context.Context, input VaultMemSaveInput) (*VaultMemSaveResult, error) {
	entryRes, dedup, err := s.entrySvc.SaveVaultMem(ctx, input)
	if err != nil {
		return nil, err
	}

	proj := ""
	if entryRes.Entry.ProjectID != nil {
		proj = *entryRes.Entry.ProjectID
	}

	topicKey := input.TopicKey
	if topicKey == "" {
		topicKey = input.Topic
	}

	return &VaultMemSaveResult{
		ID:           entryRes.Entry.ID,
		Title:        entryRes.Entry.Title,
		Type:         string(entryRes.Entry.Type),
		Project:      proj,
		TopicKey:     topicKey,
		Topic:        topicKey,
		Deduplicated: dedup,
		UpdatedAt:    entryRes.Entry.UpdatedAt,
	}, nil
}

// Search delegates to entrySvc.SearchVaultMem.
func (s *VaultMemService) Search(ctx context.Context, query string, project string, topic string, typ string, limit int) ([]VaultMemSearchResult, error) {
	return s.entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Query:    query,
		Project:  project,
		TopicKey: topic,
		Topic:    topic,
		Type:     typ,
		Limit:    limit,
	})
}

// Get retrieves full memory content by ID or slug.
func (s *VaultMemService) Get(ctx context.Context, id string) (*VaultMemGetResult, error) {
	res, err := s.entryStore.Get(ctx, id, false)
	if err != nil {
		return nil, fmt.Errorf("get memory %q: %w", id, err)
	}

	projName := ""
	if res.Entry.ProjectID != nil {
		projName = *res.Entry.ProjectID
	}

	var tags []string
	extractedTopic := ""
	for _, t := range res.Tags {
		tags = append(tags, t.Name)
		if strings.HasPrefix(t.Name, "topic_key:") {
			extractedTopic = strings.TrimPrefix(t.Name, "topic_key:")
		} else if strings.HasPrefix(t.Name, "topic:") && extractedTopic == "" {
			extractedTopic = strings.TrimPrefix(t.Name, "topic:")
		}
	}

	return &VaultMemGetResult{
		ID:        res.Entry.ID,
		Title:     res.Entry.Title,
		Type:      string(res.Entry.Type),
		Project:   projName,
		TopicKey:  extractedTopic,
		Topic:     extractedTopic,
		Content:   res.Entry.BodyOptional,
		Summary:   res.Entry.Summary,
		Tags:      tags,
		CreatedAt: res.Entry.CreatedAt,
		UpdatedAt: res.Entry.UpdatedAt,
	}, nil
}

// Timeline delegates to entrySvc.GetVaultMemTimelineResult.
func (s *VaultMemService) Timeline(ctx context.Context, id string, beforeCount, afterCount int) (*VaultMemTimelineResult, error) {
	return s.entrySvc.GetVaultMemTimelineResult(ctx, id, beforeCount, afterCount, "")
}

// TimelineWithProject delegates to entrySvc.GetVaultMemTimelineResult with a project filter.
func (s *VaultMemService) TimelineWithProject(ctx context.Context, id string, beforeCount, afterCount int, project string) (*VaultMemTimelineResult, error) {
	return s.entrySvc.GetVaultMemTimelineResult(ctx, id, beforeCount, afterCount, project)
}

// Context builds a compact context pack string tailored for agent priming.
func (s *VaultMemService) Context(ctx context.Context, project string) (string, error) {
	if s.contextSvc == nil {
		return "", fmt.Errorf("context service not available")
	}

	pack, err := s.contextSvc.GetContext(ctx, ContextInput{
		Mode:            "project",
		Project:         project,
		ExcludeArchived: true,
		MaxChars:        8000,
	})
	if err != nil {
		return "", fmt.Errorf("build context: %w", err)
	}

	var b strings.Builder
	b.WriteString(pack.Header)
	for _, sec := range pack.Sections {
		b.WriteString(fmt.Sprintf("\n### %s\n%s\n", sec.Title, sec.Content))
	}

	return b.String(), nil
}

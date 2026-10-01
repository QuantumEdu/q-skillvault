package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/quantum-6/skillvault/internal/domain"
)

// SaveLinkInput holds all data needed to persist a new link/bookmark entry.
type SaveLinkInput struct {
	URL     string   // canonical URL stored in ExternalRef (required)
	Title   string   // human-readable title; if empty, URL is used
	Summary string   // optional description / notes
	Alias   string   // short keyword for fast retrieval (stored as tag "alias:<value>")
	Tags    []string // additional free-form tags
	Project string   // optional project scope
}

// LinkResult is a resolved link with its metadata.
type LinkResult struct {
	Entry domain.EntryResult
	Tags  []string
}

// LinkService provides link-specific operations on top of EntryService.
// Links are stored as regular entries with type=link; ExternalRef holds the URL
// and aliases are stored as tags prefixed "alias:" for FTS5 searchability.
type LinkService struct {
	entrySvc *EntryService
}

// NewLinkService creates a LinkService backed by the given EntryService.
func NewLinkService(entrySvc *EntryService) *LinkService {
	return &LinkService{entrySvc: entrySvc}
}

// SaveLink persists a URL bookmark as an entry of type=link.
func (s *LinkService) SaveLink(ctx context.Context, input SaveLinkInput) (*GetEntryResult, error) {
	if input.URL == "" {
		return nil, fmt.Errorf("link URL is required")
	}

	title := input.Title
	if title == "" {
		title = input.URL
	}

	tags := make([]string, 0, len(input.Tags)+1)
	tags = append(tags, input.Tags...)
	if input.Alias != "" {
		tags = append(tags, aliasTag(input.Alias))
	}

	return s.entrySvc.SaveEntry(ctx, SaveEntryInput{
		Title:       title,
		Type:        string(domain.EntryTypeLink),
		Summary:     input.Summary,
		Project:     input.Project,
		Tags:        tags,
		Status:      string(domain.StatusActive),
		Purpose:     string(domain.PurposeKnowledge),
		ExternalRef: input.URL,
	})
}

// GetLinkByAlias finds the first link whose alias tag matches the given keyword.
// Returns nil, nil when not found.
func (s *LinkService) GetLinkByAlias(ctx context.Context, alias string) (*domain.EntrySearchResult, error) {
	tag := aliasTag(alias)
	linkType := string(domain.EntryTypeLink)
	results, err := s.entrySvc.SearchByTags(ctx, []string{tag}, true, &linkType, nil, 1)
	if err != nil {
		return nil, fmt.Errorf("get link by alias: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

// GetLinkByID retrieves a link entry by its ID or slug.
func (s *LinkService) GetLinkByID(ctx context.Context, idOrSlug string) (*GetEntryResult, error) {
	return s.entrySvc.GetEntry(ctx, idOrSlug)
}

// SearchLinks performs FTS5 search scoped to type=link entries.
func (s *LinkService) SearchLinks(ctx context.Context, query string, project string, limit int) ([]domain.EntrySearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	linkType := string(domain.EntryTypeLink)
	sq := domain.SearchQuery{
		Query: query,
		Type:  &linkType,
		Limit: limit,
	}
	if project != "" {
		sq.ProjectID = &project
	}
	return s.entrySvc.Search(ctx, sq)
}

// ListLinks returns all active link entries, optionally filtered by project.
func (s *LinkService) ListLinks(ctx context.Context, project string) ([]domain.EntryListResult, error) {
	linkType := string(domain.EntryTypeLink)
	filter := domain.EntryFilter{
		Type: &linkType,
	}
	if project != "" {
		filter.ProjectID = &project
	}
	return s.entrySvc.List(ctx, filter)
}

// aliasTag encodes an alias value as the canonical tag format.
func aliasTag(alias string) string {
	return "alias:" + strings.ToLower(strings.TrimSpace(alias))
}

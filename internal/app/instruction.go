package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/quantum-6/skillvault/internal/domain"
	"github.com/quantum-6/skillvault/internal/vars"
)

// SaveInstructionInput holds all data needed to persist an instruction / snippet recipe.
type SaveInstructionInput struct {
	Command string   // the actionable command / snippet text (required, stored in BodyOptional)
	Title   string   // human-readable title; if empty, generated from alias or command
	Summary string   // optional explanation / notes on when to use
	Alias   string   // short keyword for fast retrieval (stored as tag "alias:<value>")
	Tags    []string // additional tags
	Project string   // optional project scope
}

// ResolvedInstruction contains the interpolated command along with any missing variables.
type ResolvedInstruction struct {
	Entry       domain.Entry
	RawCommand  string
	Command     string   // rendered with variables substituted
	MissingVars []string // placeholders that were not provided in vars map
	Variables   []string // all variables detected in the raw template
	Tags        []domain.Tag
	Alias       string
}

// InstructionService provides instruction and command snippet operations on top of EntryService.
// Instructions are stored as regular entries with type=instruction; BodyOptional holds the command
// template and aliases are stored as tags prefixed "alias:" for fast FTS5 and tag lookup.
type InstructionService struct {
	entrySvc *EntryService
}

// NewInstructionService creates an InstructionService backed by the given EntryService.
func NewInstructionService(entrySvc *EntryService) *InstructionService {
	return &InstructionService{entrySvc: entrySvc}
}

// SaveInstruction persists a command snippet as an entry of type=instruction.
func (s *InstructionService) SaveInstruction(ctx context.Context, input SaveInstructionInput) (*GetEntryResult, error) {
	if strings.TrimSpace(input.Command) == "" {
		return nil, fmt.Errorf("instruction command is required")
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		if input.Alias != "" {
			title = input.Alias
		} else {
			lines := strings.Split(strings.TrimSpace(input.Command), "\n")
			title = lines[0]
			if len(title) > 60 {
				title = title[:57] + "..."
			}
		}
	}

	tags := make([]string, 0, len(input.Tags)+4)
	tags = append(tags, input.Tags...)
	if input.Alias != "" {
		tags = append(tags, aliasTag(input.Alias))
	}

	// Auto-tag with detected placeholder variable names for easy discovery (e.g. "var:input")
	detectedVars := vars.Detect(input.Command)
	for _, v := range detectedVars {
		tags = append(tags, "var:"+strings.ToLower(v))
	}

	return s.entrySvc.SaveEntry(ctx, SaveEntryInput{
		Title:       title,
		Type:        string(domain.EntryTypeInstruction),
		Summary:     input.Summary,
		Body:        input.Command,
		Project:     input.Project,
		Tags:        tags,
		Status:      string(domain.StatusActive),
		Purpose:     string(domain.PurposeWork),
		ExternalRef: input.Alias,
	})
}

// GetInstructionByAlias finds the first instruction whose alias tag matches the given keyword.
// Returns nil, nil when not found.
func (s *InstructionService) GetInstructionByAlias(ctx context.Context, alias string) (*domain.EntrySearchResult, error) {
	tag := aliasTag(alias)
	instType := string(domain.EntryTypeInstruction)
	results, err := s.entrySvc.SearchByTags(ctx, []string{tag}, true, &instType, nil, 1)
	if err != nil {
		return nil, fmt.Errorf("get instruction by alias: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

// ResolveInstruction fetches an instruction by alias and interpolates the provided variables.
func (s *InstructionService) ResolveInstruction(ctx context.Context, alias string, providedVars map[string]string) (*ResolvedInstruction, error) {
	res, err := s.GetInstructionByAlias(ctx, alias)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}

	rawCmd := res.Entry.BodyOptional
	if rawCmd == "" {
		rawCmd = res.Entry.Summary
	}

	allVars := vars.Detect(rawCmd)
	globals := vars.PrepareGlobals(res.Entry.ProjectID)
	resolvedCmd, missing := vars.Resolve(rawCmd, providedVars, globals)

	return &ResolvedInstruction{
		Entry:       res.Entry,
		RawCommand:  rawCmd,
		Command:     resolvedCmd,
		MissingVars: missing,
		Variables:   allVars,
		Tags:        res.Tags,
		Alias:       alias,
	}, nil
}

// SearchInstructions performs FTS5 search scoped to type=instruction entries.
func (s *InstructionService) SearchInstructions(ctx context.Context, query string, project string, limit int) ([]domain.EntrySearchResult, error) {
	if limit <= 0 {
		limit = 20
	}
	instType := string(domain.EntryTypeInstruction)
	sq := domain.SearchQuery{
		Query: query,
		Type:  &instType,
		Limit: limit,
	}
	if project != "" {
		sq.ProjectID = &project
	}
	return s.entrySvc.Search(ctx, sq)
}

// ListInstructions returns all active instruction entries, optionally filtered by project.
func (s *InstructionService) ListInstructions(ctx context.Context, project string) ([]domain.EntryListResult, error) {
	instType := string(domain.EntryTypeInstruction)
	filter := domain.EntryFilter{
		Type: &instType,
	}
	if project != "" {
		filter.ProjectID = &project
	}
	return s.entrySvc.List(ctx, filter)
}

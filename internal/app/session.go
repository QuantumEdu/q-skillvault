package app

import (
	"context"
	"fmt"

	"github.com/quantum-6/skillvault/internal/db"
	"github.com/quantum-6/skillvault/internal/domain"
)

type SessionWrapInput struct {
	Project   string
	Summary   string
	Decisions []string
	Pending   []string
	Learnings []string
	Artifacts []string
}

type SessionWrapOutput struct {
	Entry    *GetEntryResult
	Artifact *domain.Artifact
}

type SessionService struct {
	store           *db.Store
	artifactService *ArtifactService
}

func NewSessionService(store *db.Store, artifactService *ArtifactService) *SessionService {
	return &SessionService{store: store, artifactService: artifactService}
}

func (s *SessionService) SessionWrap(ctx context.Context, input SessionWrapInput) (*SessionWrapOutput, error) {
	if input.Summary == "" {
		return nil, fmt.Errorf("summary is required")
	}

	var projectID *string
	if input.Project != "" {
		proj, err := s.store.Projects.Get(ctx, input.Project)
		if err != nil {
			return nil, fmt.Errorf("project %q not found: %w", input.Project, err)
		}
		projectID = &proj.ID
	}

	sessionBody := input.Summary
	if len(input.Decisions) > 0 {
		sessionBody += "\n\n## Decisions\n"
		for _, d := range input.Decisions {
			sessionBody += fmt.Sprintf("- %s\n", d)
		}
	}
	if len(input.Pending) > 0 {
		sessionBody += "\n## Pending\n"
		for _, p := range input.Pending {
			sessionBody += fmt.Sprintf("- %s\n", p)
		}
	}
	if len(input.Learnings) > 0 {
		sessionBody += "\n## Learnings\n"
		for _, l := range input.Learnings {
			sessionBody += fmt.Sprintf("- %s\n", l)
		}
	}

	id := generateSessionID()
	entry := domain.Entry{
		ID:           id,
		Title:        "Session: " + truncate(input.Summary, 60),
		Slug:         id,
		Type:         domain.EntryTypeSession,
		Summary:      input.Summary,
		BodyOptional: sessionBody,
		Status:       domain.StatusActive,
		ProjectID:    projectID,
	}

	var artifact *domain.Artifact
	if len(input.Artifacts) == 0 {
		if err := s.store.Entries.Save(ctx, entry, nil); err != nil {
			return nil, fmt.Errorf("save session entry: %w", err)
		}
	} else {
		body := sessionBody + "\n\n## Linked Artifacts\n"
		for _, artifactID := range input.Artifacts {
			linked, err := s.store.Artifacts.Get(ctx, artifactID)
			if err != nil {
				return nil, fmt.Errorf("resolve linked artifact: %w", err)
			}
			body += fmt.Sprintf("- %s (%s)\n", linked.Title, linked.ID)
		}
		metadata := domain.Artifact{
			ID: generateArtifactID(), Title: "Session Output: " + truncate(input.Summary, 60),
			Slug: "sess-out-" + id, Type: domain.ArtifactTypeSessionOutput,
			MimeType: "text/markdown", Summary: input.Summary, ProjectID: projectID, SourceEntryID: &entry.ID,
		}
		var err error
		artifact, err = s.artifactService.persistContent(metadata, []byte(body), func(a domain.Artifact) error {
			return s.store.SaveSessionWithArtifact(ctx, entry, a)
		})
		if err != nil {
			return nil, fmt.Errorf("save session artifact: %w", err)
		}
	}

	result, err := s.store.Entries.Get(ctx, entry.ID, true)
	if err != nil {
		return nil, fmt.Errorf("get saved session: %w", err)
	}

	return &SessionWrapOutput{
		Entry:    &GetEntryResult{Entry: result, Artifact: artifact},
		Artifact: artifact,
	}, nil
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

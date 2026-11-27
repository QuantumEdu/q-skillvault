package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/quantum-6/skillvault/internal/db"
	"github.com/quantum-6/skillvault/internal/domain"
	"github.com/quantum-6/skillvault/internal/files"
	"github.com/quantum-6/skillvault/internal/security"
)

type SaveArtifactInput struct {
	Title    string
	Type     string
	Content  string
	FilePath string
	Summary  string
	Project  string
	Tags     []string
}

type ArtifactService struct {
	artifactStore db.ArtifactStore
	entryStore    db.EntryStore
	projectStore  db.ProjectStore
	files         *files.ArtifactFileService
}

func NewArtifactService(artifactStore db.ArtifactStore, entryStore db.EntryStore, projectStore db.ProjectStore, fileStore *files.ArtifactFileService) *ArtifactService {
	return &ArtifactService{
		artifactStore: artifactStore,
		entryStore:    entryStore,
		projectStore:  projectStore,
		files:         fileStore,
	}
}

func (s *ArtifactService) SaveArtifact(ctx context.Context, input SaveArtifactInput) (*domain.Artifact, error) {
	if input.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if err := domain.ValidateArtifactType(input.Type); err != nil {
		return nil, fmt.Errorf("validate type: %w", err)
	}
	if input.Content == "" && input.FilePath == "" {
		return nil, fmt.Errorf("either content or file_path must be provided")
	}
	if input.Content != "" && input.FilePath != "" {
		return nil, fmt.Errorf("choose either content or file_path")
	}

	var projectID *string
	if input.Project != "" {
		proj, err := s.projectStore.Get(ctx, input.Project)
		if err != nil {
			return nil, fmt.Errorf("project %q not found: %w", input.Project, err)
		}
		projectID = &proj.ID
	}

	content := []byte(input.Content)
	if input.FilePath != "" {
		var err error
		content, err = s.files.ReadArtifact(input.FilePath)
		if err != nil {
			return nil, fmt.Errorf("read source artifact: %w", err)
		}
	}

	artifact := domain.Artifact{
		ID:        generateArtifactID(),
		Title:     input.Title,
		Slug:      slugify(input.Title),
		Type:      domain.ArtifactType(input.Type),
		MimeType:  detectMimeType(input.Type),
		Summary:   input.Summary,
		ProjectID: projectID,
		Tags:      input.Tags,
	}
	return s.persistContent(artifact, content, func(a domain.Artifact) error {
		return s.artifactStore.Save(ctx, a)
	})
}

func (s *ArtifactService) persistContent(artifact domain.Artifact, content []byte, save func(domain.Artifact) error) (*domain.Artifact, error) {
	if err := security.RejectSecrets(artifact.Title, artifact.Summary, strings.Join(artifact.Tags, "\n"), string(content)); err != nil {
		return nil, err
	}
	path, hash, size, err := s.files.WriteArtifact(artifact.ID, content, artifact.MimeType)
	if err != nil {
		return nil, err
	}
	artifact.FilePath, artifact.ContentHash, artifact.SizeBytes = path, hash, size
	if err := save(artifact); err != nil {
		return nil, errors.Join(err, s.files.DeleteArtifact(path))
	}
	return &artifact, nil
}

func (s *ArtifactService) GetArtifact(ctx context.Context, id string) (*domain.Artifact, error) {
	a, err := s.artifactStore.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *ArtifactService) ListArtifacts(ctx context.Context, projectID *string) ([]domain.Artifact, error) {
	return s.artifactStore.List(ctx, projectID)
}

func (s *ArtifactService) LinkArtifactToEntry(ctx context.Context, artifactID, entryID string) error {
	_, err := s.artifactStore.Get(ctx, artifactID)
	if err != nil {
		return fmt.Errorf("artifact %q not found: %w", artifactID, err)
	}

	entry, err := s.entryStore.Get(ctx, entryID, true)
	if err != nil {
		return fmt.Errorf("entry %q not found: %w", entryID, err)
	}

	entry.Entry.ArtifactID = &artifactID
	if err := s.entryStore.Save(ctx, entry.Entry, nil); err != nil {
		return fmt.Errorf("save entry with artifact link: %w", err)
	}

	return nil
}

func detectMimeType(artifactType string) string {
	switch domain.ArtifactType(artifactType) {
	case domain.ArtifactTypeMarkdown:
		return "text/markdown"
	case domain.ArtifactTypeJSON:
		return "application/json"
	case domain.ArtifactTypeHTML:
		return "text/html"
	case domain.ArtifactTypeTXT:
		return "text/plain"
	case domain.ArtifactTypePDFReference:
		return "application/pdf"
	case domain.ArtifactTypeAIOutput, domain.ArtifactTypePDFAnalysis, domain.ArtifactTypeSpec, domain.ArtifactTypeReport, domain.ArtifactTypeSessionOutput:
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

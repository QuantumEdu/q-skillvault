package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/quantum-6/skillvault/internal/db"
	"github.com/quantum-6/skillvault/internal/domain"
)

type SaveProjectInput struct {
	ID          string
	Name        string
	Description string
	Status      string
}

type ProjectService struct {
	projectStore db.ProjectStore
}

func NewProjectService(projectStore db.ProjectStore) *ProjectService {
	return &ProjectService{projectStore: projectStore}
}

func (s *ProjectService) SaveProject(ctx context.Context, input SaveProjectInput) (*domain.Project, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	status := domain.StatusActive
	if input.Status != "" {
		if err := domain.ValidateStatus(input.Status); err != nil {
			return nil, fmt.Errorf("validate status: %w", err)
		}
		status = domain.Status(input.Status)
	}

	projID := input.ID
	if projID == "" {
		projID = generateProjectID()
	}

	// The slug is the lookup key (`--project <id or slug>`) and the column
	// is UNIQUE, so store the slugified name, never the raw name: before
	// this fix `add-project --name "Verify Paths"` stored slug "Verify
	// Paths" and `--project verify-paths` failed with "not found". Names
	// that differ only in slug-insignificant characters ("Verify Paths"
	// vs "Verify-Paths") collide on insert; resolve like entries do
	// (slug, slug-2, ...), skipping our own row when an explicit ID
	// re-saves an existing project.
	slug := slugify(input.Name)
	if slug == "" {
		slug = "untitled"
	}
	candidate := slug
	counter := 2
	for {
		existing, err := s.projectStore.Get(ctx, candidate)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				break
			}
			return nil, fmt.Errorf("check slug collision: %w", err)
		}
		if existing.ID == projID {
			break
		}
		candidate = fmt.Sprintf("%s-%d", slug, counter)
		counter++
	}
	slug = candidate

	proj := domain.Project{
		ID:          projID,
		Name:        input.Name,
		Slug:        slug,
		Description: input.Description,
		Status:      status,
	}

	if err := s.projectStore.Save(ctx, proj); err != nil {
		return nil, fmt.Errorf("save project: %w", err)
	}

	return &proj, nil
}

func (s *ProjectService) GetProject(ctx context.Context, id string) (*domain.Project, error) {
	p, err := s.projectStore.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *ProjectService) ListProjects(ctx context.Context) ([]domain.Project, error) {
	return s.projectStore.List(ctx, false)
}

func (s *ProjectService) ListProjectsIncludeArchived(ctx context.Context) ([]domain.Project, error) {
	return s.projectStore.List(ctx, true)
}

func (s *ProjectService) ArchiveProject(ctx context.Context, id string) error {
	return s.projectStore.Archive(ctx, id)
}

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/quantum-6/skillvault/internal/db"
	"github.com/quantum-6/skillvault/internal/domain"
)

type SaveWorkflowStep struct {
	OrderIndex     int
	Title          string
	Instruction    string
	Required       bool
	ExpectedOutput string
	EntrySlug      string
}

type SaveWorkflowInput struct {
	ID          string
	Name        string
	Description string
	Status      string
	Steps       []SaveWorkflowStep
}

func (step *SaveWorkflowStep) UnmarshalJSON(data []byte) error {
	type canonical SaveWorkflowStep
	var fields struct {
		canonical
		Order       int    `json:"order"`
		Description string `json:"description"`
		EntrySlug   string `json:"entry_slug"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*step = SaveWorkflowStep(fields.canonical)
	if step.OrderIndex == 0 {
		step.OrderIndex = fields.Order
	}
	if step.Title == "" {
		step.Title = fields.Description
	}
	if fields.EntrySlug != "" {
		step.EntrySlug = fields.EntrySlug
	}
	return nil
}

type WorkflowService struct {
	workflowStore db.WorkflowStore
}

func NewWorkflowService(workflowStore db.WorkflowStore) *WorkflowService {
	return &WorkflowService{workflowStore: workflowStore}
}

func (s *WorkflowService) SaveWorkflow(ctx context.Context, input SaveWorkflowInput) (*domain.Workflow, error) {
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

	wf := domain.Workflow{
		ID:          generateWorkflowID(),
		Name:        input.Name,
		Slug:        slugify(input.Name),
		Description: input.Description,
		Status:      status,
	}
	if input.ID != "" {
		wf.Slug = slugify(input.ID)
	}

	steps := make([]domain.WorkflowStep, 0, len(input.Steps))
	for _, s := range input.Steps {
		if strings.TrimSpace(s.Title) == "" {
			return nil, fmt.Errorf("step %d title is required", len(steps)+1)
		}
		orderIdx := s.OrderIndex
		if orderIdx == 0 {
			orderIdx = len(steps) + 1
		}
		steps = append(steps, domain.WorkflowStep{
			WorkflowID:     wf.ID,
			OrderIndex:     orderIdx,
			Title:          s.Title,
			Instruction:    s.Instruction,
			Required:       s.Required,
			ExpectedOutput: s.ExpectedOutput,
			EntrySlug:      s.EntrySlug,
		})
	}

	if err := s.workflowStore.Save(ctx, wf, steps); err != nil {
		return nil, fmt.Errorf("save workflow: %w", err)
	}

	return &wf, nil
}

func (s *WorkflowService) RenderWorkflow(ctx context.Context, id string) ([]domain.WorkflowStep, error) {
	return s.workflowStore.Render(ctx, id)
}

func (s *WorkflowService) ListWorkflows(ctx context.Context, includeArchived bool) ([]domain.Workflow, error) {
	return s.workflowStore.List(ctx, includeArchived)
}

package app

import (
	"context"
	"strings"
	"testing"

	"github.com/quantum-6/skillvault/internal/domain"
)

// Regression tests for the get-context slug gap found by the e2e pilot
// (REPORT.md §4): GetContext resolved the project for the header
// (projectStore.Get matches slug or ID) but passed the raw reference to
// every collector, so `get-context --project <slug>` named the project yet
// returned zero decisions while the same call with the ID returned them
// all. The Fix 1 pattern (PR #108) covered the entry reads but not this
// path.
func TestGetContextResolvesProjectSlug(t *testing.T) {
	_, entrySvc, _, _, _, projectSvc, contextSvc, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	proj, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "memory-eval"})
	if err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}
	for _, title := range []string{"Timeout maximo 20 segundos", "Logs en JSON estructurado"} {
		if _, err := entrySvc.SaveEntry(ctx, SaveEntryInput{
			Title:   title,
			Type:    string(domain.EntryTypeDecision),
			Summary: title,
			Project: "memory-eval", // slug on the write path
		}); err != nil {
			t.Fatalf("SaveEntry %q failed: %v", title, err)
		}
	}

	bySlug, err := contextSvc.GetContext(ctx, ContextInput{Project: "memory-eval", MaxChars: 5000})
	if err != nil {
		t.Fatalf("GetContext by slug failed: %v", err)
	}
	for _, want := range []string{"Timeout maximo 20 segundos", "Logs en JSON estructurado", "## Active Decisions"} {
		if !strings.Contains(bySlug.Raw, want) {
			t.Errorf("pack by slug missing %q:\n%s", want, bySlug.Raw)
		}
	}

	byID, err := contextSvc.GetContext(ctx, ContextInput{Project: proj.ID, MaxChars: 5000})
	if err != nil {
		t.Fatalf("GetContext by ID failed: %v", err)
	}
	if bySlug.Raw != byID.Raw {
		t.Errorf("pack by slug differs from pack by ID:\n--- slug ---\n%s\n--- ID ---\n%s", bySlug.Raw, byID.Raw)
	}

	// Unknown project must fail loudly (naming the known projects) instead
	// of returning a pack that looks valid but carries nothing.
	if _, err := contextSvc.GetContext(ctx, ContextInput{Project: "no-existe", MaxChars: 5000}); err == nil {
		t.Fatal("expected error for unknown project, got nil")
	} else {
		if !strings.Contains(err.Error(), `project "no-existe" not found`) {
			t.Errorf("expected not-found error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "memory-eval") {
			t.Errorf("expected error to name known projects, got: %v", err)
		}
	}
}

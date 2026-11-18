package app

import (
	"context"
	"strings"
	"testing"

	"github.com/quantum-6/skillvault/internal/domain"
)

// Regression tests for the --project slug trap measured in the memory
// experiment (round 2, arm A4a): `search --project <slug>` silently
// returned "No results found" while the same query with the project ID
// returned results. The flag promises "slug or ID" and the write path
// already resolves slugs; the read paths passed the raw string to the
// store, where it was compared against project_id only.
func TestSearchEntriesResolvesProjectSlug(t *testing.T) {
	_, entrySvc, _, _, _, projectSvc, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	proj, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "memory-eval"})
	if err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}
	if proj.Slug != "" && proj.Slug != "memory-eval" {
		t.Fatalf("expected slug memory-eval, got %q", proj.Slug)
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

	bySlug, err := entrySvc.SearchEntries(ctx, "timeout", domain.SearchQuery{
		ProjectID: strPtr("memory-eval"),
	})
	if err != nil {
		t.Fatalf("SearchEntries by slug failed: %v", err)
	}
	if len(bySlug) != 1 {
		t.Fatalf("expected 1 result by slug, got %d", len(bySlug))
	}

	byID, err := entrySvc.SearchEntries(ctx, "timeout", domain.SearchQuery{
		ProjectID: &proj.ID,
	})
	if err != nil {
		t.Fatalf("SearchEntries by ID failed: %v", err)
	}
	if len(byID) != 1 {
		t.Fatalf("expected 1 result by ID, got %d", len(byID))
	}

	// Unknown project must fail loudly (naming the known projects) instead
	// of reporting an empty result the caller mistakes for "no matches".
	_, err = entrySvc.SearchEntries(ctx, "timeout", domain.SearchQuery{
		ProjectID: strPtr("no-existe"),
	})
	if err == nil {
		t.Fatal("expected error for unknown project, got nil")
	}
	if !strings.Contains(err.Error(), `project "no-existe" not found`) {
		t.Errorf("expected not-found error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "memory-eval") {
		t.Errorf("expected error to name known projects, got: %v", err)
	}

	// No project filter still searches globally.
	global, err := entrySvc.SearchEntries(ctx, "timeout", domain.SearchQuery{})
	if err != nil {
		t.Fatalf("global SearchEntries failed: %v", err)
	}
	if len(global) != 1 {
		t.Fatalf("expected 1 global result, got %d", len(global))
	}
}

func TestSearchByTagsResolvesProjectSlug(t *testing.T) {
	_, entrySvc, _, _, _, projectSvc, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "memory-eval"}); err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}
	if _, err := entrySvc.SaveEntry(ctx, SaveEntryInput{
		Title:   "Timeout maximo 20 segundos",
		Type:    string(domain.EntryTypeDecision),
		Summary: "El timeout maximo es 20 segundos.",
		Project: "memory-eval",
		Tags:    []string{"timeout"},
	}); err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	typ := string(domain.EntryTypeDecision)
	slug := "memory-eval"
	results, err := entrySvc.SearchByTags(ctx, []string{"timeout"}, false, &typ, &slug, 10)
	if err != nil {
		t.Fatalf("SearchByTags by slug failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result by slug, got %d", len(results))
	}

	unknown := "no-existe"
	if _, err := entrySvc.SearchByTags(ctx, []string{"timeout"}, false, &typ, &unknown, 10); err == nil {
		t.Fatal("expected error for unknown project, got nil")
	}
}

func TestSearchVaultMemResolvesProjectSlug(t *testing.T) {
	_, entrySvc, _, _, _, projectSvc, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "memory-eval"}); err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}
	if _, err := entrySvc.SaveEntry(ctx, SaveEntryInput{
		Title:   "Timeout maximo 20 segundos",
		Type:    string(domain.EntryTypeDecision),
		Summary: "El timeout maximo es 20 segundos.",
		Body:    "El timeout maximo de funciones y endpoints es 20 segundos.",
		Project: "memory-eval",
	}); err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	results, err := entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Query:   "timeout",
		Project: "memory-eval",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("SearchVaultMem by slug failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result by slug, got %d", len(results))
	}

	if _, err := entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Query:   "timeout",
		Project: "no-existe",
		Limit:   10,
	}); err == nil {
		t.Fatal("expected error for unknown project, got nil")
	}
}

func TestListResolvesProjectSlug(t *testing.T) {
	_, entrySvc, _, _, _, projectSvc, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "memory-eval"}); err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}
	if _, err := entrySvc.SaveEntry(ctx, SaveEntryInput{
		Title:   "Timeout maximo 20 segundos",
		Type:    string(domain.EntryTypeDecision),
		Summary: "El timeout maximo es 20 segundos.",
		Project: "memory-eval",
	}); err != nil {
		t.Fatalf("SaveEntry failed: %v", err)
	}

	slug := "memory-eval"
	items, err := entrySvc.List(ctx, domain.EntryFilter{ProjectID: &slug})
	if err != nil {
		t.Fatalf("List by slug failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item by slug, got %d", len(items))
	}
}

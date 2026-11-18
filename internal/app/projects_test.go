package app

import (
	"context"
	"strings"
	"testing"
)

// `add-project` stored the slug as the raw name: `add-project --name
// "Verify Paths"` saved slug "Verify Paths", so `--project verify-paths`
// failed with "not found" while `--project "Verify Paths"` resolved.
// The flag promises "slug or ID", so creation must slugify the name
// (same helper the entries use) and resolution stays id-or-slug.
func TestSaveProjectSlugifiesName(t *testing.T) {
	_, _, _, _, _, projectSvc, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	proj, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "Verify Paths"})
	if err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}
	if proj.Slug != "verify-paths" {
		t.Fatalf("expected slug verify-paths, got %q", proj.Slug)
	}
	if proj.Name != "Verify Paths" {
		t.Fatalf("display name must be preserved, got %q", proj.Name)
	}

	// The slugified form resolves, and so does the ID.
	if _, err := projectSvc.GetProject(ctx, "verify-paths"); err != nil {
		t.Fatalf("GetProject by slug failed: %v", err)
	}
	if _, err := projectSvc.GetProject(ctx, proj.ID); err != nil {
		t.Fatalf("GetProject by ID failed: %v", err)
	}
}

// The slug column is UNIQUE: names that differ only in slug-insignificant
// characters collide. Like entries, the second one gets slug-2, the third
// slug-3, and each still resolves by its own slug.
func TestSaveProjectSlugCollision(t *testing.T) {
	_, _, _, _, _, projectSvc, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	first, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "Verify Paths"})
	if err != nil {
		t.Fatalf("SaveProject 1 failed: %v", err)
	}
	second, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "Verify-Paths"})
	if err != nil {
		t.Fatalf("SaveProject 2 failed: %v", err)
	}
	third, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "Verify  Paths"})
	if err != nil {
		t.Fatalf("SaveProject 3 failed: %v", err)
	}
	if first.Slug != "verify-paths" || second.Slug != "verify-paths-2" || third.Slug != "verify-paths-3" {
		t.Fatalf("unexpected slugs: %q, %q, %q", first.Slug, second.Slug, third.Slug)
	}
	if first.ID == second.ID || first.ID == third.ID || second.ID == third.ID {
		t.Fatal("colliding names must get distinct IDs")
	}
	for _, slug := range []string{"verify-paths", "verify-paths-2", "verify-paths-3"} {
		if _, err := projectSvc.GetProject(ctx, slug); err != nil {
			t.Fatalf("GetProject(%q) failed: %v", slug, err)
		}
	}
}

// Decided behavior: resolution is id-or-slug only (the documented
// contract, same as entries). The raw display name does NOT resolve for
// projects created after the fix.
func TestSaveProjectRawNameDoesNotResolve(t *testing.T) {
	_, _, _, _, _, projectSvc, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "Verify Paths"}); err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}
	if _, err := projectSvc.GetProject(ctx, "Verify Paths"); err == nil {
		t.Fatal("expected error resolving by raw name, got nil")
	} else if !strings.Contains(err.Error(), `project "Verify Paths" not found`) {
		t.Errorf("unexpected error: %v", err)
	}
}

// A name with no slugifiable characters falls back to "untitled" (same as
// entries), with the usual collision suffix for the second one.
func TestSaveProjectEmptySlugFallsBackToUntitled(t *testing.T) {
	_, _, _, _, _, projectSvc, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	first, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "!!!"})
	if err != nil {
		t.Fatalf("SaveProject 1 failed: %v", err)
	}
	second, err := projectSvc.SaveProject(ctx, SaveProjectInput{Name: "???"})
	if err != nil {
		t.Fatalf("SaveProject 2 failed: %v", err)
	}
	if first.Slug != "untitled" || second.Slug != "untitled-2" {
		t.Fatalf("unexpected slugs: %q, %q", first.Slug, second.Slug)
	}
}

// Re-saving an existing project (explicit ID) must not treat its own slug
// as a collision and rename it to slug-2.
func TestSaveProjectUpdateKeepsOwnSlug(t *testing.T) {
	_, _, _, _, _, projectSvc, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()

	proj, err := projectSvc.SaveProject(ctx, SaveProjectInput{ID: "proj-keep", Name: "Verify Paths"})
	if err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}
	if proj.Slug != "verify-paths" {
		t.Fatalf("expected slug verify-paths, got %q", proj.Slug)
	}
	updated, err := projectSvc.SaveProject(ctx, SaveProjectInput{ID: "proj-keep", Name: "Verify Paths!"})
	if err != nil {
		t.Fatalf("re-save failed: %v", err)
	}
	if updated.Slug != "verify-paths" {
		t.Fatalf("re-save renamed own slug to %q", updated.Slug)
	}
}

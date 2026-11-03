package db

import (
	"context"
	"testing"

	"github.com/quantum-6/skillvault/internal/domain"
)

func setupIndexStore(t *testing.T) (*Store, func()) {
	t.Helper()
	sqlDB, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	if err := RunMigrations(sqlDB); err != nil {
		sqlDB.Close()
		t.Fatalf("RunMigrations failed: %v", err)
	}
	store := NewStore(sqlDB)
	return store, func() { _ = store.Close() }
}

func seedEntries(t *testing.T, store *Store, ctx context.Context) {
	t.Helper()
	entries := []domain.Entry{
		{
			ID: "e-kiro", Title: "Restablecí autenticación de Kiro", Slug: "kiro-auth",
			Type: domain.EntryTypeSkill, BodyOptional: "Kiro authentication reset",
			Status: domain.StatusActive,
		},
		{
			ID: "e-sdd", Title: "Metodología SDD Dialéctica", Slug: "sdd-dialectica",
			Type: domain.EntryTypeSkill, BodyOptional: "Clean architecture walkthrough",
			Status: domain.StatusActive,
		},
	}
	for _, entry := range entries {
		if err := store.Entries.Save(ctx, entry, nil); err != nil {
			t.Fatalf("seed %s: %v", entry.ID, err)
		}
	}
}

// TestFTSRebuildCommandIsANoOpOnThisTable pins the behaviour that caused real
// vaults to become 94% unsearchable: the FTS5 'rebuild' command succeeds and
// changes nothing here, so a drift can never be detected by calling it.
func TestFTSRebuildCommandIsANoOpOnThisTable(t *testing.T) {
	store, cleanup := setupIndexStore(t)
	defer cleanup()
	ctx := context.Background()
	seedEntries(t, store, ctx)

	if _, err := store.DB().ExecContext(ctx, "DELETE FROM entries_fts"); err != nil {
		t.Fatalf("clear index: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx,
		"INSERT INTO entries_fts(entries_fts) VALUES('rebuild')"); err != nil {
		t.Fatalf("fts5 rebuild command: %v", err)
	}

	drift, err := store.SearchIndexStatus(ctx)
	if err != nil {
		t.Fatalf("SearchIndexStatus: %v", err)
	}
	if !drift.Stale() {
		t.Fatalf("expected drift to survive the fts5 rebuild command, got %+v", drift)
	}
}

func TestRebuildFTSRestoresEveryEntry(t *testing.T) {
	store, cleanup := setupIndexStore(t)
	defer cleanup()
	ctx := context.Background()
	seedEntries(t, store, ctx)

	if _, err := store.DB().ExecContext(ctx, "DELETE FROM entries_fts"); err != nil {
		t.Fatalf("clear index: %v", err)
	}
	if err := store.Search.RebuildFTS(ctx); err != nil {
		t.Fatalf("RebuildFTS: %v", err)
	}

	drift, err := store.SearchIndexStatus(ctx)
	if err != nil {
		t.Fatalf("SearchIndexStatus: %v", err)
	}
	if drift.Stale() {
		t.Fatalf("index still stale after rebuild: %+v", drift)
	}

	results, err := store.Entries.Search(ctx, domain.SearchQuery{Query: "Kiro"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected the rebuilt index to find an entry by title")
	}
}

func TestEnsureSearchIndexRepairsOnceAndReports(t *testing.T) {
	store, cleanup := setupIndexStore(t)
	defer cleanup()
	ctx := context.Background()
	seedEntries(t, store, ctx)

	if _, err := store.DB().ExecContext(ctx, "DELETE FROM entries_fts"); err != nil {
		t.Fatalf("clear index: %v", err)
	}

	repaired, before, after, err := store.EnsureSearchIndex(ctx)
	if err != nil {
		t.Fatalf("EnsureSearchIndex: %v", err)
	}
	if !repaired {
		t.Fatal("expected the drifted index to be repaired")
	}
	if before != 0 || after != 2 {
		t.Fatalf("unexpected counts: before=%d after=%d", before, after)
	}

	// A second call must be a no-op: never rebuild on every open.
	repaired, _, _, err = store.EnsureSearchIndex(ctx)
	if err != nil {
		t.Fatalf("second EnsureSearchIndex: %v", err)
	}
	if repaired {
		t.Fatal("expected no repair when the index is already consistent")
	}
}

func TestEnsureSearchIndexLeavesAHealthyIndexAlone(t *testing.T) {
	store, cleanup := setupIndexStore(t)
	defer cleanup()
	ctx := context.Background()
	seedEntries(t, store, ctx)

	repaired, before, after, err := store.EnsureSearchIndex(ctx)
	if err != nil {
		t.Fatalf("EnsureSearchIndex: %v", err)
	}
	if repaired || before != after {
		t.Fatalf("healthy index was touched: repaired=%v before=%d after=%d", repaired, before, after)
	}
}

package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/quantum-6/skillvault/internal/domain"
)

func snapshotDB(t *testing.T) (*sql.DB, *sqliteImportExportStore) {
	t.Helper()
	db, err := OpenDB(t.TempDir() + "/vault.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	return db, &sqliteImportExportStore{db: db}
}

func TestMemorySnapshotRoundTripAndReplay(t *testing.T) {
	ctx := context.Background()
	source, exporter := snapshotDB(t)
	entries := &sqliteEntryStore{db: source}
	for _, id := range []string{"old", "current"} {
		e := domain.Entry{ID: id, Slug: id, Title: id, Type: domain.EntryTypeDecision, Status: domain.StatusActive}
		if err := entries.Save(ctx, e, []string{"mem", "topic_key:architecture/auth"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := source.Exec(`UPDATE entries SET normalized_hash=id,topic_key='architecture/auth',revision_count=3,
 duplicate_count=7,pinned=1,review_after='2025-01-01',expires_at='2025-06-01',
 created_at='2024-01-02 03:04:05.123',updated_at='2024-02-03 04:05:06.456'`); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`UPDATE entries SET superseded_by='current' WHERE id='old'`); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`INSERT INTO entry_versions VALUES('ver-old','current',1,'prior','prior summary','prior body','2024-01-01 01:02:03')`); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Exec(`INSERT INTO entry_links(from_entry_id,to_entry_id,relation_type,label,active,created_at)
 VALUES('old','current','supersedes','history',0,'2024-01-02 03:04:05')`); err != nil {
		t.Fatal(err)
	}
	before, err := exporter.ExportAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise JSON, not merely Go objects.
	payload, err := json.Marshal(before)
	if err != nil {
		t.Fatal(err)
	}
	var decoded domain.VaultExport
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	target, importer := snapshotDB(t)
	for i := 0; i < 2; i++ {
		if err := importer.ImportAll(ctx, decoded); err != nil {
			t.Fatal(err)
		}
		after, err := importer.ExportAll(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before.Data, after.Data) {
			t.Fatalf("round trip %d lost state\nbefore=%+v\nafter=%+v", i, before.Data, after.Data)
		}
	}
	results, err := (&sqliteEntryStore{db: target}).Search(ctx, domain.SearchQuery{Query: "old"})
	if err != nil || len(results) != 0 {
		t.Fatalf("superseded entry resurfaced: %+v %v", results, err)
	}
	for _, query := range []string{"current", "architecture"} {
		hits, err := (&sqliteEntryStore{db: target}).Search(ctx, domain.SearchQuery{Query: query})
		if err != nil || len(hits) != 1 || hits[0].Entry.ID != "current" {
			t.Fatalf("restored memory not searchable (%s): %+v %v", query, hits, err)
		}
	}
	// A history collision must roll back content changes too.
	decoded.Data.Entries[0].Title = "must rollback"
	decoded.Data.EntryVersions[0].BodyOptional = "tampered"
	if err := importer.ImportAll(ctx, decoded); err == nil {
		t.Fatal("history collision accepted")
	}
	unchanged, err := importer.ExportAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Data, unchanged.Data) {
		t.Fatal("failed import left partial state")
	}
}

func TestLegacySnapshotPreservesExistingMemoryState(t *testing.T) {
	ctx := context.Background()
	db, store := snapshotDB(t)
	entries := &sqliteEntryStore{db: db}
	if err := entries.Save(ctx, domain.Entry{ID: "a", Slug: "a", Title: "a", Type: domain.EntryTypeReference}, nil); err != nil {
		t.Fatal(err)
	}
	if err := entries.SetMemoryHints(ctx, "a", MemoryHints{Pinned: true, NormalizedHash: "hash", ReviewAfter: "2026-01-01"}); err != nil {
		t.Fatal(err)
	}
	full, err := store.ExportAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, schema := range []int{1, 2, 3} {
		legacy := full
		legacy.Data.Entries = append([]domain.Entry(nil), full.Data.Entries...)
		legacy.SchemaVersion = schema
		legacy.Data.MemoryQuality = nil
		legacy.Data.EntryVersions = nil
		legacy.Data.Entries[0].CreatedAt = time.Time{}
		legacy.Data.Entries[0].UpdatedAt = time.Time{}
		if err := store.ImportAll(ctx, legacy); err != nil {
			t.Fatal(err)
		}
		after, err := store.ExportAll(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(full.Data.MemoryQuality, after.Data.MemoryQuality) {
			t.Fatal("legacy import reset quality")
		}
		if !full.Data.Entries[0].CreatedAt.Equal(after.Data.Entries[0].CreatedAt) {
			t.Fatal("legacy import changed creation time")
		}
	}
}

func TestInvalidMemorySnapshotRejected(t *testing.T) {
	for _, tc := range []struct {
		name     string
		quality  map[string]domain.MemoryQualityRecord
		versions []domain.EntryVersion
	}{
		{name: "unowned", quality: map[string]domain.MemoryQualityRecord{"other": {RevisionCount: 1}}},
		{name: "missing target", quality: map[string]domain.MemoryQualityRecord{"a": {RevisionCount: 1, SupersededBy: "missing"}}},
		{name: "cycle", quality: map[string]domain.MemoryQualityRecord{"a": {RevisionCount: 1, SupersededBy: "a"}}},
		{name: "counter", quality: map[string]domain.MemoryQualityRecord{"a": {RevisionCount: 1, DuplicateCount: -1}}},
		{name: "orphan version", versions: []domain.EntryVersion{{VersionID: "v", EntryID: "other", VersionNumber: 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, store := snapshotDB(t)
			input := domain.VaultExport{SchemaVersion: 4, Data: domain.VaultData{Entries: []domain.Entry{{ID: "a", Title: "a", Type: domain.EntryTypeReference}}, MemoryQuality: tc.quality, EntryVersions: tc.versions}}
			if err := store.ImportAll(context.Background(), input); err == nil {
				t.Fatal("invalid state accepted")
			}
			after, err := store.ExportAll(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(after.Data.Entries) != 0 {
				t.Fatal("invalid import wrote content")
			}
		})
	}
}

func TestExportUsesOneReadSnapshot(t *testing.T) {
	ctx := context.Background()
	db, store := snapshotDB(t)
	entries := &sqliteEntryStore{db: db}
	if err := entries.Save(ctx, domain.Entry{ID: "a", Slug: "a", Title: "before", Type: domain.EntryTypeReference}, nil); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var title string
	if err := tx.QueryRow(`SELECT title FROM entries WHERE id='a'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	// Separate writer commits while the read snapshot remains open (WAL).
	if _, err := db.Exec(`UPDATE entries SET title='after',pinned=1 WHERE id='a'`); err != nil {
		t.Fatal(err)
	}
	view := &sqliteImportExportStore{db: db, snapshot: tx}
	snapshot, err := view.exportAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Data.Entries[0].Title != "before" || snapshot.Data.MemoryQuality["a"].Pinned {
		t.Fatal("mixed generations in export")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	latest, err := store.ExportAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Data.Entries[0].Title != "after" || !latest.Data.MemoryQuality["a"].Pinned {
		t.Fatal("writer change not visible after snapshot")
	}
}

func TestSnapshotTagReplacementKeepsLegacyMergeBehavior(t *testing.T) {
	for _, schema := range []int{3, 4} {
		t.Run(fmt.Sprintf("v%d", schema), func(t *testing.T) {
			ctx := context.Background()
			db, store := snapshotDB(t)
			entries := &sqliteEntryStore{db: db}
			entry := domain.Entry{ID: "a", Slug: "a", Title: "before", Type: domain.EntryTypeReference}
			if err := entries.Save(ctx, entry, []string{"staletag"}); err != nil {
				t.Fatal(err)
			}
			incoming := domain.VaultExport{SchemaVersion: schema, Data: domain.VaultData{
				Entries: []domain.Entry{{ID: "a", Slug: "a", Title: "after", Type: domain.EntryTypeReference}},
				Tags:    []domain.Tag{{ID: "fresh", Name: "fresh", Slug: "fresh"}}, EntryTags: []domain.EntryTag{{EntryID: "a", TagID: "fresh"}},
			}}
			if err := store.ImportAll(ctx, incoming); err != nil {
				t.Fatal(err)
			}
			current, err := entries.Get(ctx, "a", true)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if schema < 4 {
				want = 2
			}
			if len(current.Tags) != want {
				t.Fatalf("got %d tags, want %d", len(current.Tags), want)
			}
			for query, expected := range map[string]int{"after": 1, "before": 0, "fresh": 1, "staletag": want - 1} {
				results, err := entries.Search(ctx, domain.SearchQuery{Query: query})
				if err != nil || len(results) != expected {
					t.Fatalf("search %s after import: %v %v", query, results, err)
				}
			}
		})
	}
}

package db

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/quantum-6/skillvault/internal/domain"
)

func memoryInput(operation, body string) MemoryWrite {
	return MemoryWrite{
		Entry: domain.Entry{Title: "Auth model", Summary: body, BodyOptional: body, Type: domain.EntryTypeDecision, Purpose: domain.PurposeKnowledge},
		Tags:  []string{"mem", "decision"}, ProjectRef: "p", ProjectSlug: "p", BaseID: "auth-model",
		Hints: MemoryHints{NormalizedHash: body, TopicKey: "architecture/auth", Pinned: true, ReviewAfter: "2027-01-01"}, OperationID: operation,
	}
}

func TestAtomicMemoryReplayAndConflict(t *testing.T) {
	ctx := context.Background()
	db, _ := snapshotDB(t)
	store := &sqliteEntryStore{db: db}
	input := memoryInput("event-1", "jwt")
	first, err := store.SaveMemory(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Deduplicated || first.Replayed {
		t.Fatal("first call is neither duplicate nor replay")
	}
	// Tag order/case is normalized for operation identity.
	input.Tags = []string{"DECISION", "mem", "mem"}
	replay, err := store.SaveMemory(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.Deduplicated || !reflect.DeepEqual(first.Entry, replay.Entry) {
		t.Fatal("replay changed original outcome")
	}
	input.OperationID = "event-2"
	duplicate, err := store.SaveMemory(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.Deduplicated || duplicate.Entry.Entry.ID != first.Entry.Entry.ID {
		t.Fatal("distinct relearning must dedupe")
	}
	replay, err = store.SaveMemory(ctx, input)
	if err != nil || !replay.Replayed || !replay.Deduplicated {
		t.Fatalf("dedupe replay: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT duplicate_count FROM entries WHERE id=?`, first.Entry.Entry.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("retry inflated duplicate_count: %d %v", count, err)
	}
	input = memoryInput("event-1", "changed")
	if _, err := store.SaveMemory(ctx, input); !errors.Is(err, ErrMemoryOperationConflict) {
		t.Fatalf("payload conflict: %v", err)
	}
	input = memoryInput("event-1", "jwt")
	input.Hints.Pinned = false
	if _, err := store.SaveMemory(ctx, input); !errors.Is(err, ErrMemoryOperationConflict) {
		t.Fatalf("lifecycle conflict: %v", err)
	}
	// A later supersession must not change the result of an earlier operation.
	if _, err := store.SaveMemory(ctx, memoryInput("event-3", "jwt with refresh")); err != nil {
		t.Fatal(err)
	}
	replay, err = store.SaveMemory(ctx, memoryInput("event-1", "jwt"))
	if err != nil || !reflect.DeepEqual(first.Entry, replay.Entry) {
		t.Fatalf("historical replay changed: %v", err)
	}
}

func TestAtomicMemoryRollbackOnPartialFailure(t *testing.T) {
	ctx := context.Background()
	for _, trigger := range []string{
		`CREATE TRIGGER fail_save BEFORE UPDATE OF normalized_hash ON entries WHEN NEW.normalized_hash='after' BEGIN SELECT RAISE(ABORT,'injected hints failure'); END`,
		`CREATE TRIGGER fail_save BEFORE INSERT ON memory_save_operations WHEN NEW.operation_id='after' BEGIN SELECT RAISE(ABORT,'injected journal failure'); END`,
	} {
		t.Run(trigger[:35], func(t *testing.T) {
			db, exporter := snapshotDB(t)
			store := &sqliteEntryStore{db: db}
			if _, err := store.SaveMemory(ctx, memoryInput("before", "before")); err != nil {
				t.Fatal(err)
			}
			before, err := exporter.ExportAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SaveMemory(ctx, memoryInput("after", "after")); err == nil {
				t.Fatal("fault did not abort save")
			}
			after, err := exporter.ExportAll(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Data, after.Data) {
				t.Fatal("failed save changed entries, hints, tags, or supersession")
			}
			var journal, fts int
			if err := db.QueryRow(`SELECT count(*) FROM memory_save_operations WHERE operation_id='after'`).Scan(&journal); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM entries_fts WHERE entries_fts MATCH 'after'`).Scan(&fts); err != nil {
				t.Fatal(err)
			}
			if journal != 0 || fts != 0 {
				t.Fatal("failed save leaked journal/index state")
			}
		})
	}
}

func TestAtomicMemoryRollsBackNewProjectAndTags(t *testing.T) {
	db, _ := snapshotDB(t)
	store := &sqliteEntryStore{db: db}
	if _, err := db.Exec(`CREATE TRIGGER fail_save BEFORE INSERT ON memory_save_operations BEGIN SELECT RAISE(ABORT,'failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMemory(context.Background(), memoryInput("fail", "content")); err == nil {
		t.Fatal("expected failure")
	}
	for _, table := range []string{"projects", "entries", "tags", "entry_tags", "entries_fts", "memory_save_operations"} {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial %s remained: %d %v", table, count, err)
		}
	}
}

func TestAtomicMemoryArchivedAndDeletedReplay(t *testing.T) {
	ctx := context.Background()
	db, _ := snapshotDB(t)
	store := &sqliteEntryStore{db: db}
	input := memoryInput("private-event", "content that must be removed")
	result, err := store.SaveMemory(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Archive(ctx, result.Entry.Entry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMemory(ctx, input); !errors.Is(err, ErrMemoryOperationUnavailable) {
		t.Fatalf("archive replay: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM entry_tags; DELETE FROM entries`); err != nil {
		t.Fatal(err)
	}
	var encoded string
	if err := db.QueryRow(`SELECT result_json FROM memory_save_operations WHERE operation_id=?`, input.OperationID).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	if encoded != "{}" {
		t.Fatal("hard-deleted content remained in retry journal")
	}
	if _, err := store.SaveMemory(ctx, input); !errors.Is(err, ErrMemoryOperationUnavailable) {
		t.Fatalf("deleted replay: %v", err)
	}
}

func TestAtomicMemoryConcurrentWriters(t *testing.T) {
	const writers = 16
	for _, mode := range []string{"same-operation", "relearning", "revisions"} {
		t.Run(mode, func(t *testing.T) {
			db, _ := snapshotDB(t)
			var path string
			if err := db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
				t.Fatal(err)
			}
			other, err := OpenDB(path)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			stores := []*sqliteEntryStore{{db: db}, {db: other}}
			start := make(chan struct{})
			results := make(chan *MemoryWriteResult, writers)
			failures := make(chan error, writers)
			var wg sync.WaitGroup
			for i := 0; i < writers; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()
					<-start
					input := memoryInput("event", "same")
					if mode == "relearning" {
						input.OperationID = ""
					}
					if mode == "revisions" {
						input = memoryInput(fmt.Sprintf("event-%d", index), fmt.Sprintf("revision-%d", index))
					}
					result, err := stores[index%2].SaveMemory(context.Background(), input)
					if err != nil {
						failures <- err
					} else {
						results <- result
					}
				}(i)
			}
			close(start)
			wg.Wait()
			close(results)
			close(failures)
			for err := range failures {
				t.Error(err)
			}
			if t.Failed() {
				return
			}
			var entries, live, duplicates, revision, journal int
			if err := db.QueryRow(`SELECT count(*),sum(superseded_by IS NULL),sum(duplicate_count),max(revision_count) FROM entries`).Scan(&entries, &live, &duplicates, &revision); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRow(`SELECT count(*) FROM memory_save_operations`).Scan(&journal); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "same-operation":
				var replays int
				for result := range results {
					if result.Replayed {
						replays++
					}
				}
				if entries != 1 || live != 1 || duplicates != 0 || journal != 1 || replays != writers-1 {
					t.Fatalf("bad replay counts: %d %d %d %d %d", entries, live, duplicates, journal, replays)
				}
			case "relearning":
				if entries != 1 || live != 1 || duplicates != writers-1 || journal != 0 {
					t.Fatalf("bad dedupe counts: %d %d %d %d", entries, live, duplicates, journal)
				}
			case "revisions":
				if entries != writers || live != 1 || duplicates != 0 || revision != writers || journal != writers {
					t.Fatalf("broken revision chain: %d %d %d %d %d", entries, live, duplicates, revision, journal)
				}
			}
		})
	}
}

func TestAtomicMemoryInvalidOperationAndCancellation(t *testing.T) {
	db, _ := snapshotDB(t)
	store := &sqliteEntryStore{db: db}
	for _, id := range []string{" padded ", "line\nbreak", strings.Repeat("x", 257)} {
		if _, err := store.SaveMemory(context.Background(), memoryInput(id, "content")); err == nil {
			t.Fatal("invalid operation accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.SaveMemory(ctx, memoryInput("canceled", "content")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM entries`).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid request wrote memory")
	}
}

func TestJournalFailureDoesNotCountRelearning(t *testing.T) {
	db, _ := snapshotDB(t)
	store := &sqliteEntryStore{db: db}
	ctx := context.Background()
	if _, err := store.SaveMemory(ctx, memoryInput("first", "same")); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER fail_relearn BEFORE INSERT ON memory_save_operations WHEN NEW.operation_id='next' BEGIN SELECT RAISE(ABORT,'lost operation'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveMemory(ctx, memoryInput("next", "same")); err == nil {
		t.Fatal("journal fault ignored")
	}
	var count int
	if err := db.QueryRow(`SELECT duplicate_count FROM entries`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed operation counted as relearning: %d %v", count, err)
	}
}

func TestAtomicMemoryExactIDWinsOverAnotherSlug(t *testing.T) {
	ctx := context.Background()
	database, _ := snapshotDB(t)
	store := &sqliteEntryStore{db: database}
	// Legacy/imported entries can have independent IDs and slugs. A dedupe
	// lookup by exact ID must never acknowledge an unrelated slug match.
	for _, entry := range []domain.Entry{
		{ID: "a-other", Slug: "z-target", Title: "Unrelated", Type: domain.EntryTypeDecision, Purpose: domain.PurposeKnowledge, Status: domain.StatusActive},
		{ID: "z-target", Slug: "target-slug", Title: "Target", Type: domain.EntryTypeDecision, Purpose: domain.PurposeKnowledge, Status: domain.StatusActive},
	} {
		if err := store.Save(ctx, entry, []string{entry.Title}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.Exec(`UPDATE entries SET normalized_hash='same' WHERE id='z-target'`); err != nil {
		t.Fatal(err)
	}
	input := memoryInput("collision-check", "same")
	input.ProjectRef = ""
	result, err := store.SaveMemory(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Deduplicated || result.Entry.Entry.ID != "z-target" || len(result.Entry.Tags) != 1 || result.Entry.Tags[0].Name != "Target" {
		t.Fatalf("acknowledged wrong entry: %+v", result)
	}
}

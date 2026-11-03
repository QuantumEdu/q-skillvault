package app

import (
	"context"
	"testing"

	"github.com/quantum-6/skillvault/internal/domain"
)

func saveMem(t *testing.T, svc *EntryService, ctx context.Context, title, content, topic string) *domain.EntryResult {
	t.Helper()
	res, _, err := svc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:    title,
		Content:  content,
		Type:     "architecture",
		Project:  "q-sup",
		TopicKey: topic,
	})
	if err != nil {
		t.Fatalf("save %q: %v", title, err)
	}
	return res
}

// The same fact, restated, is a new revision: the old one leaves the current
// view but stays readable as history.
func TestSameFactRestatedSupersedesItsPreviousRevision(t *testing.T) {
	entrySvc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()
	ctx := context.Background()

	v1 := saveMem(t, entrySvc, ctx, "Auth model", "jwt only", "architecture/auth")
	v2 := saveMem(t, entrySvc, ctx, "Auth model", "jwt with refresh rotation", "architecture/auth")

	if v1.Entry.ID == v2.Entry.ID {
		t.Fatal("a restated fact must create a new entry, not dedupe")
	}

	live, err := entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Project: "q-sup", TopicKey: "architecture/auth",
	})
	if err != nil {
		t.Fatalf("search live: %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("expected exactly 1 live revision, got %d", len(live))
	}

	history, err := entrySvc.store.Search(ctx, domain.SearchQuery{
		Query: "jwt", IncludeSuperseded: true,
	})
	if err != nil {
		t.Fatalf("search history: %v", err)
	}
	if len(history) < 2 {
		t.Fatalf("expected both revisions to remain readable as history, got %d", len(history))
	}
}

// A topic groups related observations. Superseding on the topic alone would
// silently delete unrelated memories, so different facts must coexist.
func TestDifferentFactsUnderTheSameTopicAreNotSuperseded(t *testing.T) {
	entrySvc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()
	ctx := context.Background()

	a := saveMem(t, entrySvc, ctx, "Connection lock", "deadlock on write", "storage/sqlite")
	b := saveMem(t, entrySvc, ctx, "WAL checkpoint", "checkpoint starvation", "storage/sqlite")

	if a.Entry.ID == b.Entry.ID {
		t.Fatal("distinct facts must be distinct entries")
	}

	live, err := entrySvc.SearchVaultMem(ctx, VaultMemSearchInput{
		Project: "q-sup", TopicKey: "storage/sqlite",
	})
	if err != nil {
		t.Fatalf("search live: %v", err)
	}
	if len(live) != 2 {
		t.Fatalf("expected both facts to stay live under the same topic, got %d", len(live))
	}
}

// Re-saving identical content must not create a second row: it is the same fact.
func TestIdenticalContentIsDeduplicatedNotSuperseded(t *testing.T) {
	entrySvc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()
	ctx := context.Background()

	first := saveMem(t, entrySvc, ctx, "Auth model", "jwt only", "architecture/auth")

	res, dedup, err := entrySvc.SaveVaultMem(ctx, VaultMemSaveInput{
		Title:    "Auth model",
		Content:  "  jwt only  ",
		Type:     "architecture",
		Project:  "q-sup",
		TopicKey: "architecture/auth",
	})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if !dedup {
		t.Fatal("expected identical content to be deduplicated")
	}
	if res.Entry.ID != first.Entry.ID {
		t.Fatalf("dedupe must return the existing entry: %s != %s", res.Entry.ID, first.Entry.ID)
	}
}

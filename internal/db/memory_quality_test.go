package db

import (
	"context"
	"testing"

	"github.com/quantum-6/skillvault/internal/domain"
)

func seedMemory(t *testing.T, store *Store, ctx context.Context, id, title, body string) {
	t.Helper()
	if err := store.Entries.Save(ctx, domain.Entry{
		ID: id, Title: title, Slug: id, Type: domain.EntryTypeSkill,
		BodyOptional: body, Status: domain.StatusActive,
	}, nil); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func seedMemoryIn(t *testing.T, store *Store, ctx context.Context, id, title, body, proj string) {
	t.Helper()
	var pid *string
	if proj != "" {
		p := proj
		pid = &p
	}
	if err := store.Entries.Save(ctx, domain.Entry{
		ID: id, Title: title, Slug: id, Type: domain.EntryTypeSkill,
		BodyOptional: body, Status: domain.StatusActive, ProjectID: pid,
	}, nil); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func project(name string) *string { return &name }

func TestSetMemoryHintsStoresHashTopicAndLifecycle(t *testing.T) {
	store, cleanup := setupIndexStore(t)
	defer cleanup()
	ctx := context.Background()
	seedMemory(t, store, ctx, "m1", "Auth model", "jwt only")

	err := store.Entries.SetMemoryHints(ctx, "m1", MemoryHints{
		NormalizedHash: "abc123",
		TopicKey:       "architecture/auth",
		Pinned:         true,
		ReviewAfter:    "2027-01-01",
		ExpiresAt:      "2027-06-01",
	})
	if err != nil {
		t.Fatalf("SetMemoryHints: %v", err)
	}

	var hash, topic, review, expires string
	var pinned int
	err = store.DB().QueryRowContext(ctx,
		`SELECT normalized_hash, topic_key, pinned, review_after, expires_at FROM entries WHERE id = 'm1'`).
		Scan(&hash, &topic, &pinned, &review, &expires)
	if err != nil {
		t.Fatalf("read hints: %v", err)
	}
	if hash != "abc123" || topic != "architecture/auth" || pinned != 1 {
		t.Fatalf("hash/topic/pinned not stored: %q %q %d", hash, topic, pinned)
	}
	if review != "2027-01-01" || expires != "2027-06-01" {
		t.Fatalf("lifecycle not stored: %q %q", review, expires)
	}
}

func TestFindByHashIsScopedToTheProject(t *testing.T) {
	store, cleanup := setupIndexStore(t)
	defer cleanup()
	ctx := context.Background()

	for _, p := range []string{"proj-x", "proj-y"} {
		if err := store.Projects.Save(ctx, domain.Project{
			ID: p, Name: p, Slug: p, Status: domain.StatusActive,
		}); err != nil {
			t.Fatalf("seed project %s: %v", p, err)
		}
	}
	seedMemoryIn(t, store, ctx, "a", "Fact A", "same content", "proj-x")
	seedMemoryIn(t, store, ctx, "b", "Fact B", "same content", "proj-y")
	if err := store.Entries.SetMemoryHints(ctx, "a", MemoryHints{NormalizedHash: "same", TopicKey: "t"}); err != nil {
		t.Fatalf("hints a: %v", err)
	}
	if err := store.Entries.SetMemoryHints(ctx, "b", MemoryHints{NormalizedHash: "same"}); err != nil {
		t.Fatalf("hints b: %v", err)
	}

	id, found, err := store.Entries.FindByHash(ctx, project("proj-x"), "same")
	if err != nil {
		t.Fatalf("FindByHash: %v", err)
	}
	if !found || id != "a" {
		t.Fatalf("expected to find a in proj-x, got found=%v id=%q", found, id)
	}

	// The same content in another project is a different memory.
	other, foundOther, err := store.Entries.FindByHash(ctx, project("proj-y"), "same")
	if err != nil {
		t.Fatalf("FindByHash other project: %v", err)
	}
	if !foundOther || other != "b" {
		t.Fatalf("expected b in proj-y, got found=%v id=%q", foundOther, other)
	}

	// A third project sees nothing.
	_, foundNone, err := store.Entries.FindByHash(ctx, project("proj-z"), "same")
	if err != nil {
		t.Fatalf("FindByHash third project: %v", err)
	}
	if foundNone {
		t.Fatal("hash must not match a project that does not hold it")
	}
}

func TestSupersedeHidesThePreviousRevisionFromSearch(t *testing.T) {
	store, cleanup := setupIndexStore(t)
	defer cleanup()
	ctx := context.Background()

	seedMemory(t, store, ctx, "v1", "Auth model", "jwt only")
	seedMemory(t, store, ctx, "v2", "Auth model", "jwt with refresh")
	if err := store.Entries.SetMemoryHints(ctx, "v1", MemoryHints{
		NormalizedHash: "h1", TopicKey: "architecture/auth",
	}); err != nil {
		t.Fatalf("hints v1: %v", err)
	}

	// The live entry for the topic is the older one, until the newer supersedes it.
	older, found, err := store.Entries.FindByTopic(ctx, nil, "architecture/auth", "Auth model")
	if err != nil || !found || older != "v1" {
		t.Fatalf("expected to find v1 as the live revision, got %q found=%v err=%v", older, found, err)
	}

	revision, err := store.Entries.Supersede(ctx, older, "v2")
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}
	if revision != 2 {
		t.Fatalf("expected the new revision to be 2, got %d", revision)
	}

	if err := store.Entries.SetMemoryHints(ctx, "v2", MemoryHints{
		NormalizedHash: "h2", TopicKey: "architecture/auth",
	}); err != nil {
		t.Fatalf("hints v2: %v", err)
	}

	// The topic now resolves to v2, not v1.
	live, found, err := store.Entries.FindByTopic(ctx, nil, "architecture/auth", "Auth model")
	if err != nil || !found || live != "v2" {
		t.Fatalf("expected v2 to own the topic, got %q found=%v err=%v", live, found, err)
	}

	// Default search hides the superseded revision.
	results, err := store.Entries.Search(ctx, domain.SearchQuery{Query: "Auth"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.Entry.ID == "v1" {
			t.Fatal("superseded revision leaked into default search")
		}
	}

	// History is still reachable on request.
	history, err := store.Entries.Search(ctx, domain.SearchQuery{Query: "Auth", IncludeSuperseded: true})
	if err != nil {
		t.Fatalf("Search include superseded: %v", err)
	}
	seen := false
	for _, r := range history {
		if r.Entry.ID == "v1" {
			seen = true
		}
	}
	if !seen {
		t.Fatal("superseded revision must remain readable with IncludeSuperseded")
	}
}

func TestBumpDuplicateCountsReLearnedFacts(t *testing.T) {
	store, cleanup := setupIndexStore(t)
	defer cleanup()
	ctx := context.Background()
	seedMemory(t, store, ctx, "d1", "Fact", "content")
	if err := store.Entries.SetMemoryHints(ctx, "d1", MemoryHints{NormalizedHash: "h"}); err != nil {
		t.Fatalf("hints: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := store.Entries.BumpDuplicate(ctx, "d1"); err != nil {
			t.Fatalf("BumpDuplicate: %v", err)
		}
	}

	var count int
	if err := store.DB().QueryRowContext(ctx,
		`SELECT duplicate_count FROM entries WHERE id = 'd1'`).Scan(&count); err != nil {
		t.Fatalf("read duplicate_count: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected duplicate_count 3, got %d", count)
	}
}

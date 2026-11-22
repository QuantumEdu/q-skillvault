package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/quantum-6/skillvault/internal/db"
)

func TestMemoryOperationFacadeReplay(t *testing.T) {
	_, mem, cleanup := setupVaultMemTest(t)
	defer cleanup()
	input := VaultMemSaveInput{Title: "A decision", Content: "Durable fact <private>not-for-storage</private>", Project: "p", Topic: " architecture/a ", OperationID: "event-1", Pinned: true}
	first, err := mem.Save(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.TopicKey = "architecture/a"
	input.Topic = ""
	second, err := mem.Save(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.ID != first.ID || second.UpdatedAt != first.UpdatedAt || second.TopicKey != first.TopicKey {
		t.Fatal("facade did not preserve replay result")
	}
	stored, err := mem.Get(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored.Content, "not-for-storage") {
		t.Fatal("private content persisted")
	}
	input.Content = "different payload"
	if _, err := mem.Save(context.Background(), input); !errors.Is(err, db.ErrMemoryOperationConflict) {
		t.Fatalf("expected typed operation conflict: %v", err)
	}
}

func TestTitleOnlyMemoriesHaveIndependentIdentity(t *testing.T) {
	svc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()
	a, dup, err := svc.SaveVaultMem(context.Background(), VaultMemSaveInput{Title: "One fact", Project: "p"})
	if err != nil || dup {
		t.Fatal(err)
	}
	b, dup, err := svc.SaveVaultMem(context.Background(), VaultMemSaveInput{Title: "Another fact", Project: "p"})
	if err != nil || dup {
		t.Fatal(err)
	}
	if a.Entry.ID == b.Entry.ID {
		t.Fatal("unrelated empty-body memories collided")
	}
	again, dup, err := svc.SaveVaultMem(context.Background(), VaultMemSaveInput{Title: " One fact ", Project: "p"})
	if err != nil || !dup || again.Entry.ID != a.Entry.ID {
		t.Fatalf("title-only dedupe: %v", err)
	}
}

func TestMemoryDerivedTextIsUTF8Safe(t *testing.T) {
	svc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()
	result, _, err := svc.SaveVaultMem(context.Background(), VaultMemSaveInput{Content: strings.Repeat("界", 301)})
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(result.Entry.Title) || !utf8.ValidString(result.Entry.Summary) || utf8.RuneCountInString(result.Entry.Title) > 60 || utf8.RuneCountInString(result.Entry.Summary) != 300 {
		t.Fatal("derived memory text is not safely bounded")
	}
}

type nonAtomicEntryStore struct{ db.EntryStore }

func TestMemoryStoreWithoutAtomicCapabilityFailsClosed(t *testing.T) {
	svc, _, cleanup := setupVaultMemTest(t)
	defer cleanup()
	svc.store = nonAtomicEntryStore{svc.store}
	if _, _, err := svc.SaveVaultMem(context.Background(), VaultMemSaveInput{Content: "fact"}); err == nil || !strings.Contains(err.Error(), "atomic") {
		t.Fatal("unsafe fallback accepted")
	}
}

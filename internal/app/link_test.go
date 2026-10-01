package app

import (
	"context"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/quantum-6/skillvault/internal/db"
)

func openLinkTestVault(t *testing.T) *LinkService {
	t.Helper()
	sqlDB, err := db.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := db.RunMigrations(sqlDB); err != nil {
		sqlDB.Close()
		t.Fatalf("RunMigrations: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	store := db.NewStore(sqlDB)
	entrySvc := NewEntryService(store.Entries, store.Projects, store.Artifacts)
	return NewLinkService(entrySvc)
}

func TestLinkService_SaveAndGetByAlias(t *testing.T) {
	ctx := context.Background()
	svc := openLinkTestVault(t)

	_, err := svc.SaveLink(ctx, SaveLinkInput{
		URL:   "https://github.com/charmbracelet/bubbletea",
		Title: "Bubble Tea",
		Alias: "btea",
		Tags:  []string{"go", "tui"},
	})
	if err != nil {
		t.Fatalf("SaveLink: %v", err)
	}

	result, err := svc.GetLinkByAlias(ctx, "btea")
	if err != nil {
		t.Fatalf("GetLinkByAlias: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Entry.ExternalRef != "https://github.com/charmbracelet/bubbletea" {
		t.Errorf("ExternalRef = %q, want URL", result.Entry.ExternalRef)
	}
}

func TestLinkService_SaveLink_RequiresURL(t *testing.T) {
	ctx := context.Background()
	svc := openLinkTestVault(t)

	_, err := svc.SaveLink(ctx, SaveLinkInput{Title: "no url"})
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
}

func TestLinkService_SearchLinks(t *testing.T) {
	ctx := context.Background()
	svc := openLinkTestVault(t)

	inputs := []SaveLinkInput{
		{URL: "https://github.com/charmbracelet/bubbletea", Title: "Bubble Tea TUI", Tags: []string{"go", "tui"}, Alias: "btea"},
		{URL: "https://pkg.go.dev/net/http", Title: "Go net/http package", Tags: []string{"go", "stdlib"}},
		{URL: "https://anthropic.com/api", Title: "Anthropic API docs", Tags: []string{"ai", "claude"}},
	}
	for _, u := range inputs {
		if _, err := svc.SaveLink(ctx, u); err != nil {
			t.Fatalf("SaveLink(%q): %v", u.URL, err)
		}
	}

	results, err := svc.SearchLinks(ctx, "tui", "", 10)
	if err != nil {
		t.Fatalf("SearchLinks: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for 'tui'")
	}
	found := false
	for _, r := range results {
		if r.Entry.ExternalRef == "https://github.com/charmbracelet/bubbletea" {
			found = true
		}
	}
	if !found {
		t.Error("expected bubbletea link in tui search results")
	}
}

func TestLinkService_ListLinks(t *testing.T) {
	ctx := context.Background()
	svc := openLinkTestVault(t)

	for i := 0; i < 3; i++ {
		_, err := svc.SaveLink(ctx, SaveLinkInput{
			URL:   "https://example.com/" + string(rune('a'+i)),
			Title: "Link " + string(rune('A'+i)),
		})
		if err != nil {
			t.Fatalf("SaveLink: %v", err)
		}
	}

	list, err := svc.ListLinks(ctx, "")
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(list) < 3 {
		t.Errorf("expected >=3 links, got %d", len(list))
	}
}

func TestLinkService_AliasNotFoundReturnsNil(t *testing.T) {
	ctx := context.Background()
	svc := openLinkTestVault(t)

	result, err := svc.GetLinkByAlias(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetLinkByAlias: unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for unknown alias, got %+v", result)
	}
}

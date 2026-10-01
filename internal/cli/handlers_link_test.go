package cli

import (
	"testing"
)

func TestParseLinkFlags(t *testing.T) {
	t.Run("parseLinkAddFlags", func(t *testing.T) {
		args := []string{"link", "add", "https://github.com/charmbracelet/bubbletea", "--alias", "btea", "--title", "Bubble Tea", "--summary", "TUI framework", "--tags", "go,tui", "--project", "myproj"}
		flags, err := parseLinkAddFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flags.URL != "https://github.com/charmbracelet/bubbletea" {
			t.Errorf("expected URL https://github.com/charmbracelet/bubbletea, got %s", flags.URL)
		}
		if flags.Alias != "btea" {
			t.Errorf("expected alias btea, got %s", flags.Alias)
		}
		if flags.Title != "Bubble Tea" {
			t.Errorf("expected title Bubble Tea, got %s", flags.Title)
		}
		if flags.Summary != "TUI framework" {
			t.Errorf("expected summary, got %s", flags.Summary)
		}
		if flags.Tags != "go,tui" {
			t.Errorf("expected tags go,tui, got %s", flags.Tags)
		}
		if flags.Project != "myproj" {
			t.Errorf("expected project myproj, got %s", flags.Project)
		}

		// Missing URL
		_, err = parseLinkAddFlags([]string{"link", "add"})
		if err == nil {
			t.Error("expected error for missing URL in link add")
		}
	})

	t.Run("parseLinkGetFlags", func(t *testing.T) {
		args := []string{"link", "get", "btea", "--copy"}
		flags, err := parseLinkGetFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flags.Alias != "btea" {
			t.Errorf("expected alias btea, got %s", flags.Alias)
		}
		if !flags.Copy {
			t.Error("expected copy=true")
		}

		// Missing alias
		_, err = parseLinkGetFlags([]string{"link", "get"})
		if err == nil {
			t.Error("expected error for missing alias in link get")
		}
	})

	t.Run("parseLinkSearchFlags", func(t *testing.T) {
		args := []string{"link", "search", "bubbletea", "--project", "testproj", "--limit", "10"}
		flags, err := parseLinkSearchFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flags.Query != "bubbletea" {
			t.Errorf("expected query bubbletea, got %s", flags.Query)
		}
		if flags.Project != "testproj" {
			t.Errorf("expected project testproj, got %s", flags.Project)
		}
		if flags.Limit != 10 {
			t.Errorf("expected limit 10, got %d", flags.Limit)
		}

		// Missing query
		_, err = parseLinkSearchFlags([]string{"link", "search"})
		if err == nil {
			t.Error("expected error for missing query in link search")
		}
	})

	t.Run("parseLinkListFlags", func(t *testing.T) {
		args := []string{"link", "list", "--project", "testproj"}
		flags, err := parseLinkListFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flags.Project != "testproj" {
			t.Errorf("expected project testproj, got %s", flags.Project)
		}
	})
}

func TestLinkHelpers(t *testing.T) {
	tags := []string{"go", "alias:btea", "tui"}
	alias := extractAlias(tags)
	if alias != "btea" {
		t.Errorf("expected alias 'btea', got %q", alias)
	}

	nonAlias := joinNonAlias(tags)
	if nonAlias != "go, tui" {
		t.Errorf("expected 'go, tui', got %q", nonAlias)
	}

	short := truncate("hello", 10)
	if short != "hello" {
		t.Errorf("expected 'hello', got %q", short)
	}

	long := truncate("this is a very long string that should be truncated", 15)
	if len(long) > 17 { // 14 runes + 3 bytes for "…"
		t.Errorf("expected truncated string, got %q", long)
	}
}

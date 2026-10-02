package cli

import (
	"testing"
)

func TestParseCmdFlags(t *testing.T) {
	t.Run("parseCmdAddFlags", func(t *testing.T) {
		args := []string{"cmd", "add", "ffmpeg -i {{input}} -crf {{crf}} {{output}}", "--alias", "ff-comp", "--title", "Compress Video", "--summary", "H264 compress", "--tags", "video,ffmpeg", "--project", "media"}
		flags, err := parseCmdAddFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flags.Command != "ffmpeg -i {{input}} -crf {{crf}} {{output}}" {
			t.Errorf("expected command, got %s", flags.Command)
		}
		if flags.Alias != "ff-comp" {
			t.Errorf("expected alias ff-comp, got %s", flags.Alias)
		}
		if flags.Title != "Compress Video" {
			t.Errorf("expected title Compress Video, got %s", flags.Title)
		}
		if flags.Summary != "H264 compress" {
			t.Errorf("expected summary, got %s", flags.Summary)
		}
		if flags.Tags != "video,ffmpeg" {
			t.Errorf("expected tags video,ffmpeg, got %s", flags.Tags)
		}
		if flags.Project != "media" {
			t.Errorf("expected project media, got %s", flags.Project)
		}

		// Missing command
		_, err = parseCmdAddFlags([]string{"cmd", "add"})
		if err == nil {
			t.Error("expected error for missing command in cmd add")
		}
	})

	t.Run("parseCmdGetFlags", func(t *testing.T) {
		args := []string{"cmd", "get", "ff-comp", "--var", "input=test.mov", "output=out.mp4", "--copy", "--raw"}
		flags, err := parseCmdGetFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flags.Alias != "ff-comp" {
			t.Errorf("expected alias ff-comp, got %s", flags.Alias)
		}
		if flags.Vars["input"] != "test.mov" {
			t.Errorf("expected var input=test.mov, got %s", flags.Vars["input"])
		}
		if flags.Vars["output"] != "out.mp4" {
			t.Errorf("expected var output=out.mp4, got %s", flags.Vars["output"])
		}
		if !flags.Copy {
			t.Error("expected copy=true")
		}
		if !flags.Raw {
			t.Error("expected raw=true")
		}

		// Missing alias
		_, err = parseCmdGetFlags([]string{"cmd", "get"})
		if err == nil {
			t.Error("expected error for missing alias in cmd get")
		}
	})

	t.Run("parseCmdSearchFlags", func(t *testing.T) {
		args := []string{"cmd", "search", "rebase", "--project", "testproj", "--limit", "15"}
		flags, err := parseCmdSearchFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flags.Query != "rebase" {
			t.Errorf("expected query rebase, got %s", flags.Query)
		}
		if flags.Project != "testproj" {
			t.Errorf("expected project testproj, got %s", flags.Project)
		}
		if flags.Limit != 15 {
			t.Errorf("expected limit 15, got %d", flags.Limit)
		}

		// Missing query
		_, err = parseCmdSearchFlags([]string{"cmd", "search"})
		if err == nil {
			t.Error("expected error for missing query in cmd search")
		}
	})

	t.Run("parseCmdListFlags", func(t *testing.T) {
		args := []string{"cmd", "list", "--project", "testproj"}
		flags, err := parseCmdListFlags(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if flags.Project != "testproj" {
			t.Errorf("expected project testproj, got %s", flags.Project)
		}
	})
}

package app

import (
	"context"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/quantum-6/skillvault/internal/db"
)

func openInstructionTestVault(t *testing.T) *InstructionService {
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
	return NewInstructionService(entrySvc)
}

func TestInstructionService_SaveAndGetByAlias(t *testing.T) {
	ctx := context.Background()
	svc := openInstructionTestVault(t)

	_, err := svc.SaveInstruction(ctx, SaveInstructionInput{
		Command: "git rebase -i HEAD~{{count}}",
		Title:   "Interactive Git Rebase",
		Alias:   "git-rebase",
		Summary: "Rebase the last N commits interactively",
		Tags:    []string{"git", "rebase"},
	})
	if err != nil {
		t.Fatalf("SaveInstruction: %v", err)
	}

	result, err := svc.GetInstructionByAlias(ctx, "git-rebase")
	if err != nil {
		t.Fatalf("GetInstructionByAlias: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Entry.BodyOptional != "git rebase -i HEAD~{{count}}" {
		t.Errorf("BodyOptional = %q, want command template", result.Entry.BodyOptional)
	}
}

func TestInstructionService_ResolveInstruction(t *testing.T) {
	ctx := context.Background()
	svc := openInstructionTestVault(t)

	_, err := svc.SaveInstruction(ctx, SaveInstructionInput{
		Command: "ffmpeg -i {{input}} -vcodec libx264 -crf {{crf}} {{output}}",
		Alias:   "ff-compress",
		Title:   "Compress Video H264",
		Tags:    []string{"ffmpeg", "video"},
	})
	if err != nil {
		t.Fatalf("SaveInstruction: %v", err)
	}

	// 1. Resolve with all variables provided
	resolved, err := svc.ResolveInstruction(ctx, "ff-compress", map[string]string{
		"input":  "raw.mov",
		"crf":    "28",
		"output": "compressed.mp4",
	})
	if err != nil {
		t.Fatalf("ResolveInstruction: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected resolved instruction, got nil")
	}
	expected := "ffmpeg -i raw.mov -vcodec libx264 -crf 28 compressed.mp4"
	if resolved.Command != expected {
		t.Errorf("Command = %q, want %q", resolved.Command, expected)
	}
	if len(resolved.MissingVars) != 0 {
		t.Errorf("expected 0 missing vars, got %v", resolved.MissingVars)
	}

	// 2. Resolve with partial variables provided
	partial, err := svc.ResolveInstruction(ctx, "ff-compress", map[string]string{
		"input": "test.mov",
	})
	if err != nil {
		t.Fatalf("ResolveInstruction partial: %v", err)
	}
	if len(partial.MissingVars) != 2 {
		t.Errorf("expected 2 missing vars (crf, output), got %v", partial.MissingVars)
	}
}

func TestInstructionService_SaveRequiresCommand(t *testing.T) {
	ctx := context.Background()
	svc := openInstructionTestVault(t)

	_, err := svc.SaveInstruction(ctx, SaveInstructionInput{
		Title: "Empty command",
		Alias: "empty",
	})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestInstructionService_SearchAndList(t *testing.T) {
	ctx := context.Background()
	svc := openInstructionTestVault(t)

	recipes := []SaveInstructionInput{
		{Command: "git fetch --prune origin", Alias: "git-prune", Title: "Prune remote branches", Tags: []string{"git"}},
		{Command: "docker compose up -d", Alias: "dc-up", Title: "Docker Compose Up", Tags: []string{"docker"}},
		{Command: "ffmpeg -i in.mp4 out.gif", Alias: "ff-gif", Title: "Convert MP4 to GIF", Tags: []string{"ffmpeg", "gif"}},
	}
	for _, r := range recipes {
		if _, err := svc.SaveInstruction(ctx, r); err != nil {
			t.Fatalf("SaveInstruction(%q): %v", r.Alias, err)
		}
	}

	// Search
	res, err := svc.SearchInstructions(ctx, "docker", "", 10)
	if err != nil {
		t.Fatalf("SearchInstructions: %v", err)
	}
	if len(res) == 0 {
		t.Fatal("expected at least 1 search result for docker")
	}

	// List
	list, err := svc.ListInstructions(ctx, "")
	if err != nil {
		t.Fatalf("ListInstructions: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 instructions, got %d", len(list))
	}
}

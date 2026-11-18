package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quantum-6/skillvault/internal/app"
)

// Tests for `skillvault link-project`, the discoverability fix for the
// memory experiment (round 2, arm A4a): the agent never invoked the CLI
// because nothing in the project folder mentioned SkillVault. The command
// plants a marker section in the project's AGENTS.md naming the vault,
// the binary and the project slug.
func setupLinkProjectServices(t *testing.T) (*Services, string) {
	t.Helper()
	home := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	RunInit()
	svc := OpenVault()

	if _, err := svc.projectSvc.SaveProject(context.Background(), app.SaveProjectInput{Name: "memory-eval"}); err != nil {
		t.Fatalf("SaveProject failed: %v", err)
	}
	return svc, t.TempDir()
}

func TestLinkProjectCreatesAgentsMD(t *testing.T) {
	svc, projDir := setupLinkProjectServices(t)

	var buf bytes.Buffer
	if err := LinkProject(&buf, svc, "memory-eval", projDir); err != nil {
		t.Fatalf("LinkProject failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projDir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"<!-- skillvault:start -->",
		"<!-- skillvault:end -->",
		"memory-eval",
		"skillvault find",
		"--project memory-eval",
		"skillvault context",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("AGENTS.md missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(buf.String(), "Linked project memory-eval") {
		t.Errorf("expected confirmation output, got: %s", buf.String())
	}

	// Idempotency: a second run replaces the section, never duplicates it.
	buf.Reset()
	if err := LinkProject(&buf, svc, "memory-eval", projDir); err != nil {
		t.Fatalf("second LinkProject failed: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(projDir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), "<!-- skillvault:start -->"); n != 1 {
		t.Errorf("expected marker exactly once after re-run, got %d", n)
	}
}

func TestLinkProjectPreservesExistingContent(t *testing.T) {
	svc, projDir := setupLinkProjectServices(t)

	own := "# My project\n\nSome instructions.\n"
	if err := os.WriteFile(filepath.Join(projDir, "AGENTS.md"), []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := LinkProject(&buf, svc, "memory-eval", projDir); err != nil {
		t.Fatalf("LinkProject failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projDir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), own) {
		t.Errorf("existing content was not preserved:\n%s", data)
	}
	if !strings.Contains(string(data), "<!-- skillvault:start -->") {
		t.Errorf("marker section missing:\n%s", data)
	}
}

func TestLinkProjectReplacesStaleSection(t *testing.T) {
	svc, projDir := setupLinkProjectServices(t)

	stale := "<!-- skillvault:start -->\n## SkillVault (project memory)\n\n- Project: old-name (slug `old-name`, id `proj-old`)\n<!-- skillvault:end -->\n"
	if err := os.WriteFile(filepath.Join(projDir, "AGENTS.md"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := LinkProject(&buf, svc, "memory-eval", projDir); err != nil {
		t.Fatalf("LinkProject failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projDir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "old-name") {
		t.Errorf("stale section was not replaced:\n%s", content)
	}
	if n := strings.Count(content, "<!-- skillvault:start -->"); n != 1 {
		t.Errorf("expected marker exactly once, got %d", n)
	}
	if !strings.Contains(content, "memory-eval") {
		t.Errorf("new section missing:\n%s", content)
	}
}

func TestLinkProjectUnknownProject(t *testing.T) {
	svc, projDir := setupLinkProjectServices(t)

	var buf bytes.Buffer
	err := LinkProject(&buf, svc, "no-existe", projDir)
	if err == nil {
		t.Fatal("expected error for unknown project, got nil")
	}
	if !strings.Contains(err.Error(), `project "no-existe" not found`) {
		t.Errorf("expected not-found error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "memory-eval") {
		t.Errorf("expected error to name known projects, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(projDir, "AGENTS.md")); !os.IsNotExist(statErr) {
		t.Error("AGENTS.md must not be created when the project is unknown")
	}
}

func TestParseLinkProjectFlags(t *testing.T) {
	flags, err := ParseLinkProjectFlags([]string{"skillvault", "link-project", "--project", "memory-eval", "--path", "/tmp/proj"})
	if err != nil {
		t.Fatalf("ParseLinkProjectFlags failed: %v", err)
	}
	if flags.Project != "memory-eval" || flags.Path != "/tmp/proj" {
		t.Errorf("unexpected flags: %+v", flags)
	}

	if _, err := ParseLinkProjectFlags([]string{"skillvault", "link-project"}); err == nil {
		t.Error("expected error when --project is missing")
	}
}

func TestParseCommandAcceptsLinkProject(t *testing.T) {
	cmd, err := ParseCommand([]string{"skillvault", "link-project", "--project", "memory-eval"})
	if err != nil {
		t.Fatalf("ParseCommand failed: %v", err)
	}
	if cmd != "link-project" {
		t.Errorf("expected link-project, got %q", cmd)
	}

	// The "project link" alias must resolve to the same canonical command.
	cmd, err = ParseCommand([]string{"skillvault", "project", "link", "--project", "memory-eval"})
	if err != nil {
		t.Fatalf("ParseCommand (alias) failed: %v", err)
	}
	if cmd != "link-project" {
		t.Errorf("expected link-project for alias, got %q", cmd)
	}
}

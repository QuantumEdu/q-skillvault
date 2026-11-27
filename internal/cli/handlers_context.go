package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quantum-6/skillvault/internal/app"
	"github.com/quantum-6/skillvault/internal/domain"
)

func runGetContext(ctx context.Context, svc *Services, args []string) {
	flags, err := ParseGetContextFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	pack, err := svc.contextSvc.GetContext(ctx, app.ContextInput{
		Mode:     flags.Mode,
		Project:  flags.Project,
		Query:    flags.Query,
		Include:  SplitLines(flags.Include),
		MaxChars: flags.MaxChars,
		Format:   flags.Format,
	})
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	fmt.Print(pack.Raw)
}

func runAddProject(ctx context.Context, svc *Services, args []string) {
	flags, err := ParseAddProjectFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	proj, err := svc.projectSvc.SaveProject(ctx, app.SaveProjectInput{
		Name:        flags.Name,
		Description: flags.Description,
	})
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	fmt.Printf("Project saved: %s\n", proj.ID)
	fmt.Printf("  Name:        %s\n", proj.Name)
	fmt.Printf("  Description: %s\n", proj.Description)
}

func runListProjects(ctx context.Context, svc *Services, args []string) {
	projects, err := svc.projectSvc.ListProjects(ctx)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	if len(projects) == 0 {
		fmt.Println("No projects found.")
		return
	}

	fmt.Printf("Projects (%d):\n", len(projects))
	for _, p := range projects {
		fmt.Printf("\n  [%s] %s\n", p.ID, p.Name)
		fmt.Printf("    Status:      %s\n", p.Status)
		if p.Description != "" {
			fmt.Printf("    Description: %s\n", p.Description)
		}
	}
}

const (
	skillvaultMarkerStart = "<!-- skillvault:start -->"
	skillvaultMarkerEnd   = "<!-- skillvault:end -->"
)

// LinkProject writes (or refreshes) the SkillVault marker section in
// <dir>/AGENTS.md for the given project (ID or slug). The marker tells any
// agent that reads the project folder that this project keeps decisions in
// SkillVault, which binary to call and which project to query — the
// discoverability the memory experiment found missing (arm A4a: the agent
// never invoked the CLI because nothing in the project folder mentioned
// it). Idempotent: re-running replaces the marked section instead of
// duplicating it, and content outside the markers is preserved.
func LinkProject(w io.Writer, svc *Services, projectRef, dir string) error {
	ctx := context.Background()
	proj, err := svc.projectSvc.GetProject(ctx, projectRef)
	if err != nil {
		return unknownProjectHint(ctx, svc, projectRef)
	}
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("project directory %q not found: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("project path %q is not a directory", dir)
	}

	section := skillvaultMarkerSection(proj, markerVaultPath(), markerBinaryPath())
	path := filepath.Join(dir, "AGENTS.md")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	updated := upsertMarkerSection(string(existing), section)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Fprintf(w, "Linked project %s (%s) to %s\n", proj.Name, proj.ID, path)
	fmt.Fprintln(w, section)
	return nil
}

// unknownProjectHint builds a "project not found" error that lists the
// known projects (slug + ID) so the caller can retry with a valid value.
func unknownProjectHint(ctx context.Context, svc *Services, ref string) error {
	projects, err := svc.projectSvc.ListProjects(ctx)
	if err != nil || len(projects) == 0 {
		return fmt.Errorf("project %q not found", ref)
	}
	names := make([]string, 0, len(projects))
	for _, p := range projects {
		if p.Slug != "" && p.Slug != p.ID {
			names = append(names, fmt.Sprintf("%s (%s)", p.Slug, p.ID))
		} else {
			names = append(names, p.ID)
		}
	}
	return fmt.Errorf("project %q not found; known projects: %s (see 'skillvault list-projects')", ref, strings.Join(names, ", "))
}

func skillvaultMarkerSection(proj *domain.Project, vaultPath, binaryPath string) string {
	slug := proj.Slug
	if slug == "" {
		slug = proj.Name
	}
	var b bytes.Buffer
	b.WriteString(skillvaultMarkerStart + "\n")
	b.WriteString("## SkillVault (project memory)\n")
	b.WriteString("\n")
	b.WriteString("This project keeps its decisions in SkillVault, a local knowledge vault.\n")
	b.WriteString("\n")
	// Name the real vault and the absolute binary path: the marker used to
	// hardcode ~/.skillvault/vault.db and a bare `skillvault`, so an agent
	// following it could hit a different vault (or an older binary) when
	// SKILLVAULT_HOME points elsewhere or PATH resolves another install.
	fmt.Fprintf(&b, "- Vault: `%s` (binary: `%s`)\n", vaultPath, binaryPath)
	fmt.Fprintf(&b, "- Project: %s (slug `%s`, id `%s`)\n", proj.Name, slug, proj.ID)
	fmt.Fprintf(&b, "- Before answering about project conventions or changing code, check the vault first: `%s find \"<topic>\" --project %s`\n", binaryPath, slug)
	fmt.Fprintf(&b, "- For a full context pack: `%s context --project %s`\n", binaryPath, slug)
	b.WriteString(skillvaultMarkerEnd)
	return b.String()
}

// markerVaultPath returns the absolute path of the vault database this
// process operates on (dbPath honors SKILLVAULT_HOME).
func markerVaultPath() string {
	if abs, err := filepath.Abs(dbPath()); err == nil {
		return abs
	}
	return dbPath()
}

// markerBinaryPath returns the absolute path of the running binary so the
// marker invokes this binary instead of whatever `skillvault` resolves to
// in the agent's PATH (which may be an older version).
func markerBinaryPath() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	return "skillvault"
}

// upsertMarkerSection replaces the marked section if present, otherwise
// appends it after a blank line. Content outside the markers is preserved.
func upsertMarkerSection(existing, section string) string {
	start := strings.Index(existing, skillvaultMarkerStart)
	end := strings.Index(existing, skillvaultMarkerEnd)
	if start >= 0 && end > start {
		end += len(skillvaultMarkerEnd)
		return existing[:start] + section + existing[end:]
	}
	trimmed := strings.TrimRight(existing, "\n")
	if trimmed == "" {
		return section + "\n"
	}
	return trimmed + "\n\n" + section + "\n"
}

func runLinkProject(ctx context.Context, svc *Services, args []string) {
	flags, err := ParseLinkProjectFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if err := LinkProject(os.Stdout, svc, flags.Project, flags.Path); err != nil {
		PrintError(err)
		os.Exit(1)
	}
}

func runSessionWrap(ctx context.Context, svc *Services, args []string) {
	flags, err := ParseSessionWrapFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	input := app.SessionWrapInput{
		Project:   flags.Project,
		Summary:   flags.Summary,
		Decisions: SplitLines(flags.Decisions),
		Pending:   SplitLines(flags.Pending),
		Learnings: SplitLines(flags.Learnings),
	}

	output, err := svc.sessionSvc.SessionWrap(ctx, input)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	fmt.Printf("Session saved: %s\n", output.Entry.Entry.Entry.ID)
	fmt.Printf("  Summary:   %s\n", output.Entry.Entry.Entry.Summary)
	fmt.Printf("  Decisions: %d\n", len(input.Decisions))
	fmt.Printf("  Pending:   %d\n", len(input.Pending))
	fmt.Printf("  Learnings: %d\n", len(input.Learnings))
}

func runExport(ctx context.Context, svc *Services, args []string) {
	packMode := false
	for _, a := range args {
		if a == "--pack" {
			packMode = true
			break
		}
	}

	if packMode {
		packFlags, err := ParseExportPackFlags(args)
		if err != nil {
			PrintError(err)
			os.Exit(1)
		}
		if err := svc.packExportSvc.ExportPack(ctx, app.ExportPackInput{
			Author:      packFlags.Author,
			Version:     packFlags.Version,
			Description: packFlags.Description,
			OutputPath:  packFlags.OutputPath,
		}); err != nil {
			PrintError(err)
			os.Exit(1)
		}
		fmt.Printf("Pack exported to %s\n", packFlags.OutputPath)
		return
	}

	flags, err := ParseExportFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	if err := svc.exportSvc.Export(ctx, flags.OutputPath); err != nil {
		PrintError(err)
		os.Exit(1)
	}
	fmt.Printf("Exported to %s\n", flags.OutputPath)
}

func runBackup(ctx context.Context, svc *Services, args []string) {
	outputPath := filepath.Join(vaultDir(), "exports", fmt.Sprintf("skillvault-backup-%s.json", time.Now().Format("20060102-150405")))
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		PrintError(fmt.Errorf("prepare backup directory: %w", err))
		os.Exit(1)
	}
	if err := svc.exportSvc.Export(ctx, outputPath); err != nil {
		PrintError(err)
		os.Exit(1)
	}
	fmt.Printf("Backup written to %s\n", outputPath)
}

func runImport(ctx context.Context, svc *Services, args []string) {
	flags, err := ParseImportFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	if err := svc.importSvc.ImportFile(ctx, flags.FilePath, flags.Prefix, flags.Pack, flags.StrictAudit); err != nil {
		PrintError(err)
		os.Exit(1)
	}
	fmt.Println("Import completed.")
}

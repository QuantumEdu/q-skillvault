package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/quantum-6/skillvault/internal/app"
	"github.com/quantum-6/skillvault/internal/domain"
)

// ──────────────────────────────────────────────────────────────────────────────
// link add
// ──────────────────────────────────────────────────────────────────────────────

func runLinkAdd(ctx context.Context, svc *Services, args []string) {
	flags, err := parseLinkAddFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	result, err := svc.linkSvc.SaveLink(ctx, app.SaveLinkInput{
		URL:     flags.URL,
		Title:   flags.Title,
		Summary: flags.Summary,
		Alias:   flags.Alias,
		Tags:    TagItems(flags.Tags),
		Project: flags.Project,
	})
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	fmt.Printf("Link saved: %s\n", result.Entry.Entry.ID)
	fmt.Printf("  URL:   %s\n", result.Entry.Entry.ExternalRef)
	fmt.Printf("  Title: %s\n", result.Entry.Entry.Title)
	if flags.Alias != "" {
		fmt.Printf("  Alias: %s\n", flags.Alias)
	}
	if flags.Tags != "" {
		fmt.Printf("  Tags:  %s\n", flags.Tags)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// link get
// ──────────────────────────────────────────────────────────────────────────────

func runLinkGet(ctx context.Context, svc *Services, args []string) {
	flags, err := parseLinkGetFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	result, err := svc.linkSvc.GetLinkByAlias(ctx, flags.Alias)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if result == nil {
		fmt.Fprintf(os.Stderr, "link not found: %q\n", flags.Alias)
		os.Exit(1)
	}

	url := result.Entry.ExternalRef
	fmt.Println(url)

	if flags.Copy {
		if err := copyToClipboard(url); err != nil {
			fmt.Fprintf(os.Stderr, "clipboard: %v (printed above)\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "Copied to clipboard.")
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// link search
// ──────────────────────────────────────────────────────────────────────────────

func runLinkSearch(ctx context.Context, svc *Services, args []string) {
	flags, err := parseLinkSearchFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	results, err := svc.linkSvc.SearchLinks(ctx, flags.Query, flags.Project, flags.Limit)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if len(results) == 0 {
		fmt.Println("No links found.")
		return
	}

	printLinkSearchResults(results)
}

// ──────────────────────────────────────────────────────────────────────────────
// link list
// ──────────────────────────────────────────────────────────────────────────────

func runLinkList(ctx context.Context, svc *Services, args []string) {
	flags, err := parseLinkListFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	items, err := svc.linkSvc.ListLinks(ctx, flags.Project)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if len(items) == 0 {
		fmt.Println("No links stored.")
		return
	}

	fmt.Printf("%-22s  %-12s  %-50s  %s\n", "TITLE", "ALIAS", "URL", "TAGS")
	fmt.Println(strings.Repeat("─", 112))
	for _, item := range items {
		tagNames := tagsToNames(item.Tags)
		alias := extractAlias(tagNames)
		url := truncate(item.Entry.ExternalRef, 50)
		title := truncate(item.Entry.Title, 22)
		tags := joinNonAlias(tagNames)
		fmt.Printf("%-22s  %-12s  %-50s  %s\n", title, alias, url, tags)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// flag parsers
// ──────────────────────────────────────────────────────────────────────────────

type linkAddFlags struct {
	URL     string
	Title   string
	Summary string
	Alias   string
	Tags    string
	Project string
}

func findActionPos(args []string, actions ...string) int {
	for i, a := range args {
		for _, act := range actions {
			if a == act {
				return i
			}
		}
	}
	return -1
}

func parseLinkAddFlags(args []string) (*linkAddFlags, error) {
	pos := findActionPos(args, "add")
	if pos == -1 || len(args) <= pos+1 {
		return nil, fmt.Errorf("usage: skillvault link add <url> [--alias <alias>] [--title <title>] [--tags tag1,tag2] [--project <proj>]")
	}
	f := &linkAddFlags{URL: args[pos+1]}
	rest := args[pos+2:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--alias", "-a":
			i++
			if i >= len(rest) {
				return nil, fmt.Errorf("--alias requires a value")
			}
			f.Alias = rest[i]
		case "--title", "-t":
			i++
			if i >= len(rest) {
				return nil, fmt.Errorf("--title requires a value")
			}
			f.Title = rest[i]
		case "--summary", "-s":
			i++
			if i >= len(rest) {
				return nil, fmt.Errorf("--summary requires a value")
			}
			f.Summary = rest[i]
		case "--tags":
			i++
			if i >= len(rest) {
				return nil, fmt.Errorf("--tags requires a value")
			}
			f.Tags = rest[i]
		case "--project", "-p":
			i++
			if i >= len(rest) {
				return nil, fmt.Errorf("--project requires a value")
			}
			f.Project = rest[i]
		}
	}
	return f, nil
}

type linkGetFlags struct {
	Alias string
	Copy  bool
}

func parseLinkGetFlags(args []string) (*linkGetFlags, error) {
	pos := findActionPos(args, "get")
	if pos == -1 || len(args) <= pos+1 {
		return nil, fmt.Errorf("usage: skillvault link get <alias> [--copy]")
	}
	f := &linkGetFlags{Alias: args[pos+1]}
	for _, a := range args[pos+2:] {
		if a == "--copy" || a == "-c" {
			f.Copy = true
		}
	}
	return f, nil
}

type linkSearchFlags struct {
	Query   string
	Project string
	Limit   int
}

func parseLinkSearchFlags(args []string) (*linkSearchFlags, error) {
	pos := findActionPos(args, "search")
	if pos == -1 || len(args) <= pos+1 {
		return nil, fmt.Errorf("usage: skillvault link search <query> [--project <proj>] [--limit N]")
	}
	f := &linkSearchFlags{Query: args[pos+1], Limit: 20}
	rest := args[pos+2:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--project", "-p":
			i++
			if i >= len(rest) {
				return nil, fmt.Errorf("--project requires a value")
			}
			f.Project = rest[i]
		case "--limit", "-l":
			i++
			if i >= len(rest) {
				return nil, fmt.Errorf("--limit requires a value")
			}
			fmt.Sscanf(rest[i], "%d", &f.Limit) //nolint:errcheck
		}
	}
	return f, nil
}

type linkListFlags struct {
	Project string
}

func parseLinkListFlags(args []string) (*linkListFlags, error) {
	f := &linkListFlags{}
	pos := findActionPos(args, "list", "ls")
	if pos == -1 || len(args) <= pos+1 {
		return f, nil
	}
	rest := args[pos+1:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--project", "-p":
			i++
			if i >= len(rest) {
				return nil, fmt.Errorf("--project requires a value")
			}
			f.Project = rest[i]
		}
	}
	return f, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// clipboard
// ──────────────────────────────────────────────────────────────────────────────

// copyToClipboard writes text to the system clipboard.
// Tries xclip, wl-copy, pbcopy in order of availability.
func copyToClipboard(text string) error {
	candidates := clipboardCandidates()
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err == nil {
			cmd := exec.Command(c[0], c[1:]...) //nolint:gosec
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("no clipboard tool found (install xclip, wl-copy, or pbcopy)")
}

func clipboardCandidates() [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"pbcopy"}}
	case "windows":
		return [][]string{{"clip"}}
	default: // Linux (including WSL)
		return [][]string{
			{"wl-copy"},
			{"xclip", "-selection", "clipboard"},
			{"xsel", "--clipboard", "--input"},
			{"clip.exe"},
			{"/mnt/c/Windows/System32/clip.exe"},
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// display helpers
// ──────────────────────────────────────────────────────────────────────────────

func printLinkSearchResults(results []domain.EntrySearchResult) {
	fmt.Printf("%-22s  %-12s  %-50s  %s\n", "TITLE", "ALIAS", "URL", "TAGS")
	fmt.Println(strings.Repeat("─", 112))
	for _, r := range results {
		alias := extractAlias(tagsToNames(r.Tags))
		url := truncate(r.Entry.ExternalRef, 50)
		title := truncate(r.Entry.Title, 22)
		tags := joinNonAlias(tagsToNames(r.Tags))
		fmt.Printf("%-22s  %-12s  %-50s  %s\n", title, alias, url, tags)
	}
}

// tagsToNames extracts the Name field from []domain.Tag into []string.
func tagsToNames(tags []domain.Tag) []string {
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return names
}

func extractAlias(tags []string) string {
	for _, t := range tags {
		if strings.HasPrefix(t, "alias:") {
			return strings.TrimPrefix(t, "alias:")
		}
	}
	return ""
}

func joinNonAlias(tags []string) string {
	var out []string
	for _, t := range tags {
		if !strings.HasPrefix(t, "alias:") {
			out = append(out, t)
		}
	}
	return strings.Join(out, ", ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

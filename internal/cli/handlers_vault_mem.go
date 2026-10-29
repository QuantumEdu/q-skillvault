package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/quantum-6/skillvault/internal/app"
)

// ──────────────────────────────────────────────────────────────────────────────
// mem save
// ──────────────────────────────────────────────────────────────────────────────

type memSaveFlags struct {
	Content  string
	Title    string
	Type     string
	Project  string
	TopicKey string
	Scope    string
	Tags     string
	JSON     bool
}

func reorderArgs(raw []string) []string {
	var flags []string
	var positionals []string
	for i := 0; i < len(raw); i++ {
		arg := raw[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if !strings.Contains(arg, "=") && arg != "--json" && arg != "-json" && i+1 < len(raw) && !strings.HasPrefix(raw[i+1], "-") {
				flags = append(flags, raw[i+1])
				i++
			}
		} else {
			positionals = append(positionals, arg)
		}
	}
	return append(flags, positionals...)
}

func parseMemSaveFlags(args []string) (memSaveFlags, error) {
	var f memSaveFlags
	fs := flag.NewFlagSet("mem-save", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&f.Content, "content", "", "Observation content")
	fs.StringVar(&f.Title, "title", "", "Short title")
	fs.StringVar(&f.Type, "type", "decision", "Observation type")
	fs.StringVar(&f.Project, "project", "", "Project scope")
	fs.StringVar(&f.TopicKey, "topic-key", "", "Topic key")
	fs.StringVar(&f.TopicKey, "topic", "", "Alias for topic-key")
	fs.StringVar(&f.Scope, "scope", "project", "Scope: project|global|session")
	fs.StringVar(&f.Tags, "tags", "", "Comma-separated tags")
	fs.BoolVar(&f.JSON, "json", false, "Output in JSON format")

	if len(args) > 3 {
		if err := fs.Parse(reorderArgs(args[3:])); err != nil {
			return f, err
		}
	}

	if f.Content == "" && fs.NArg() > 0 {
		f.Content = strings.Join(fs.Args(), " ")
	}

	if f.Content == "" {
		if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err == nil {
				f.Content = strings.TrimSpace(string(data))
			}
		}
	}

	if f.Content == "" {
		return f, fmt.Errorf("mem save requires content (via argument, --content, or stdin)")
	}

	return f, nil
}

func runMemSave(ctx context.Context, svc *Services, args []string) {
	flags, err := parseMemSaveFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	var tags []string
	if flags.Tags != "" {
		tags = TagItems(flags.Tags)
	}

	res, err := svc.vaultMemSvc.Save(ctx, app.VaultMemSaveInput{
		Title:    flags.Title,
		Content:  flags.Content,
		Type:     flags.Type,
		Project:  flags.Project,
		TopicKey: flags.TopicKey,
		Scope:    flags.Scope,
		Tags:     tags,
	})
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	if flags.JSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
		return
	}

	if res.Deduplicated {
		fmt.Printf("Observation deduplicated: %s\n", res.ID)
	} else {
		fmt.Printf("Observation saved: %s\n", res.ID)
	}
	fmt.Printf("  Title:    %s\n", res.Title)
	fmt.Printf("  Type:     %s\n", res.Type)
	if res.Project != "" {
		fmt.Printf("  Project:  %s\n", res.Project)
	}
	if res.TopicKey != "" {
		fmt.Printf("  Topic:    %s\n", res.TopicKey)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// mem search
// ──────────────────────────────────────────────────────────────────────────────

type memSearchFlags struct {
	Query    string
	Project  string
	TopicKey string
	Type     string
	Limit    int
	JSON     bool
}

func parseMemSearchFlags(args []string) (memSearchFlags, error) {
	var f memSearchFlags
	f.Limit = 20
	fs := flag.NewFlagSet("mem-search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&f.Query, "query", "", "Search query")
	fs.StringVar(&f.Project, "project", "", "Filter by project")
	fs.StringVar(&f.TopicKey, "topic-key", "", "Filter by topic key")
	fs.StringVar(&f.TopicKey, "topic", "", "Alias for topic-key")
	fs.StringVar(&f.Type, "type", "", "Filter by type")
	fs.IntVar(&f.Limit, "limit", 20, "Maximum results")
	fs.BoolVar(&f.JSON, "json", false, "Output in JSON format")

	if len(args) > 3 {
		if err := fs.Parse(reorderArgs(args[3:])); err != nil {
			return f, err
		}
	}

	if f.Query == "" && fs.NArg() > 0 {
		f.Query = strings.Join(fs.Args(), " ")
	}

	return f, nil
}

func runMemSearch(ctx context.Context, svc *Services, args []string) {
	flags, err := parseMemSearchFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	results, err := svc.vaultMemSvc.Search(ctx, flags.Query, flags.Project, flags.TopicKey, flags.Type, flags.Limit)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	if flags.JSON {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(b))
		return
	}

	if len(results) == 0 {
		fmt.Println("No matching observations found.")
		return
	}

	fmt.Printf("Found %d observation(s):\n", len(results))
	for _, r := range results {
		topicStr := ""
		if r.TopicKey != "" {
			topicStr = fmt.Sprintf(" [%s]", r.TopicKey)
		}
		projStr := ""
		if r.Project != "" {
			projStr = fmt.Sprintf(" (project: %s)", r.Project)
		}
		fmt.Printf("  • [%s] %s (%s)%s%s\n", r.ID, r.Title, r.Type, topicStr, projStr)
		if r.Preview != "" {
			preview := strings.ReplaceAll(r.Preview, "\n", " ")
			if len(preview) > 120 {
				preview = preview[:117] + "..."
			}
			fmt.Printf("    %s\n", preview)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// mem get
// ──────────────────────────────────────────────────────────────────────────────

type memGetFlags struct {
	ID   string
	JSON bool
}

func parseMemGetFlags(args []string) (memGetFlags, error) {
	var f memGetFlags
	fs := flag.NewFlagSet("mem-get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&f.ID, "id", "", "Observation memory ID")
	fs.BoolVar(&f.JSON, "json", false, "Output in JSON format")

	if len(args) > 3 {
		if err := fs.Parse(reorderArgs(args[3:])); err != nil {
			return f, err
		}
	}

	if f.ID == "" && fs.NArg() > 0 {
		f.ID = fs.Arg(0)
	}

	if f.ID == "" {
		return f, fmt.Errorf("mem get requires an observation ID")
	}

	return f, nil
}

func runMemGet(ctx context.Context, svc *Services, args []string) {
	flags, err := parseMemGetFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	res, err := svc.vaultMemSvc.Get(ctx, flags.ID)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if res == nil {
		fmt.Fprintf(os.Stderr, "observation not found: %s\n", flags.ID)
		os.Exit(1)
	}

	if flags.JSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
		return
	}

	fmt.Printf("Observation: %s\n", res.ID)
	fmt.Printf("  Title:    %s\n", res.Title)
	fmt.Printf("  Type:     %s\n", res.Type)
	if res.Project != "" {
		fmt.Printf("  Project:  %s\n", res.Project)
	}
	if res.TopicKey != "" {
		fmt.Printf("  Topic:    %s\n", res.TopicKey)
	}
	if len(res.Tags) > 0 {
		fmt.Printf("  Tags:     %s\n", strings.Join(res.Tags, ", "))
	}
	fmt.Printf("  Updated:  %s\n", res.UpdatedAt.Format("2006-01-02 15:04:05"))
	fmt.Println("\nContent:")
	fmt.Println(res.Content)
}

// ──────────────────────────────────────────────────────────────────────────────
// mem timeline
// ──────────────────────────────────────────────────────────────────────────────

type memTimelineFlags struct {
	ID      string
	Before  int
	After   int
	Project string
	JSON    bool
}

func parseMemTimelineFlags(args []string) (memTimelineFlags, error) {
	var f memTimelineFlags
	f.Before = 3
	f.After = 3

	fs := flag.NewFlagSet("mem-timeline", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&f.ID, "id", "", "Target observation ID")
	fs.IntVar(&f.Before, "before", 3, "Number of preceding observations")
	fs.IntVar(&f.After, "after", 3, "Number of succeeding observations")
	fs.StringVar(&f.Project, "project", "", "Project scope filter")
	fs.BoolVar(&f.JSON, "json", false, "Output in JSON format")

	if len(args) > 3 {
		if err := fs.Parse(reorderArgs(args[3:])); err != nil {
			return f, err
		}
	}

	if f.ID == "" && fs.NArg() > 0 {
		f.ID = fs.Arg(0)
	}

	if f.ID == "" {
		return f, fmt.Errorf("mem timeline requires an observation ID")
	}

	return f, nil
}

func runMemTimeline(ctx context.Context, svc *Services, args []string) {
	flags, err := parseMemTimelineFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	var res *app.VaultMemTimelineResult
	if flags.Project != "" {
		res, err = svc.vaultMemSvc.TimelineWithProject(ctx, flags.ID, flags.Before, flags.After, flags.Project)
	} else {
		res, err = svc.vaultMemSvc.Timeline(ctx, flags.ID, flags.Before, flags.After)
	}
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if res == nil {
		fmt.Fprintf(os.Stderr, "observation not found: %s\n", flags.ID)
		os.Exit(1)
	}

	if flags.JSON {
		b, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(b))
		return
	}

	fmt.Printf("Timeline for observation [%s]:\n\n", res.Target.ID)
	if len(res.Before) > 0 {
		fmt.Println("--- Before ---")
		for _, b := range res.Before {
			fmt.Printf("  • [%s] %s (%s) — %s\n", b.ID, b.Title, b.Type, b.CreatedAt.Format("2006-01-02 15:04:05"))
		}
		fmt.Println()
	}

	fmt.Printf("▶ TARGET: [%s] %s (%s)\n", res.Target.ID, res.Target.Title, res.Target.Type)
	if res.Target.Preview != "" {
		fmt.Printf("  %s\n", res.Target.Preview)
	}
	fmt.Println()

	if len(res.After) > 0 {
		fmt.Println("--- After ---")
		for _, a := range res.After {
			fmt.Printf("  • [%s] %s (%s) — %s\n", a.ID, a.Title, a.Type, a.CreatedAt.Format("2006-01-02 15:04:05"))
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// mem context
// ──────────────────────────────────────────────────────────────────────────────

type memContextFlags struct {
	Project  string
	MaxChars int
	JSON     bool
}

func parseMemContextFlags(args []string) (memContextFlags, error) {
	var f memContextFlags
	f.MaxChars = 8000

	fs := flag.NewFlagSet("mem-context", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&f.Project, "project", "", "Project scope (required)")
	fs.IntVar(&f.MaxChars, "max-chars", 8000, "Maximum character length")
	fs.BoolVar(&f.JSON, "json", false, "Output in JSON format")

	if len(args) > 3 {
		if err := fs.Parse(reorderArgs(args[3:])); err != nil {
			return f, err
		}
	}

	if f.Project == "" && fs.NArg() > 0 {
		f.Project = fs.Arg(0)
	}

	if f.Project == "" {
		return f, fmt.Errorf("mem context requires a project name (via --project <name> or argument)")
	}

	return f, nil
}

func runMemContext(ctx context.Context, svc *Services, args []string) {
	flags, err := parseMemContextFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	ctxPack, err := svc.vaultMemSvc.Context(ctx, flags.Project)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	if flags.JSON {
		out := map[string]string{
			"project": flags.Project,
			"context": ctxPack,
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return
	}

	fmt.Println(ctxPack)
}

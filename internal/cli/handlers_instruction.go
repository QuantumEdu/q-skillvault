package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/quantum-6/skillvault/internal/app"
	"github.com/quantum-6/skillvault/internal/domain"
	"github.com/quantum-6/skillvault/internal/vars"
)

// ──────────────────────────────────────────────────────────────────────────────
// cmd add
// ──────────────────────────────────────────────────────────────────────────────

func runCmdAdd(ctx context.Context, svc *Services, args []string) {
	flags, err := parseCmdAddFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	result, err := svc.instructionSvc.SaveInstruction(ctx, app.SaveInstructionInput{
		Command: flags.Command,
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

	detectedVars := vars.Detect(flags.Command)

	fmt.Printf("Instruction saved: %s\n", result.Entry.Entry.ID)
	fmt.Printf("  Command: %s\n", result.Entry.Entry.BodyOptional)
	fmt.Printf("  Title:   %s\n", result.Entry.Entry.Title)
	if flags.Alias != "" {
		fmt.Printf("  Alias:   %s\n", flags.Alias)
	}
	if len(detectedVars) > 0 {
		fmt.Printf("  Vars:    {{%s}}\n", strings.Join(detectedVars, "}}, {{"))
	}
	if flags.Tags != "" {
		fmt.Printf("  Tags:    %s\n", flags.Tags)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// cmd get
// ──────────────────────────────────────────────────────────────────────────────

func runCmdGet(ctx context.Context, svc *Services, args []string) {
	flags, err := parseCmdGetFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	resolved, err := svc.instructionSvc.ResolveInstruction(ctx, flags.Alias, flags.Vars)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if resolved == nil {
		fmt.Fprintf(os.Stderr, "instruction not found: %q\n", flags.Alias)
		os.Exit(1)
	}

	cmdStr := resolved.Command

	// If missing variables remain and we are not in raw mode, prompt user if interactive
	if len(resolved.MissingVars) > 0 && !flags.Raw {
		if isTerminalInput() {
			reader := bufio.NewReader(os.Stdin)
			interactiveVars := make(map[string]string)
			for _, k := range resolved.MissingVars {
				fmt.Fprintf(os.Stderr, "Enter value for {{%s}}: ", k)
				val, _ := reader.ReadString('\n')
				val = strings.TrimSpace(val)
				if val != "" {
					interactiveVars[k] = val
				}
			}
			if len(interactiveVars) > 0 {
				for k, v := range interactiveVars {
					flags.Vars[k] = v
				}
				globals := vars.PrepareGlobals(resolved.Entry.ProjectID)
				cmdStr, _ = vars.Resolve(resolved.RawCommand, flags.Vars, globals)
			}
		}
	}

	fmt.Println(cmdStr)

	if flags.Copy {
		if err := copyToClipboard(cmdStr); err != nil {
			fmt.Fprintf(os.Stderr, "clipboard: %v (printed above)\n", err)
		} else {
			fmt.Fprintln(os.Stderr, "Copied to clipboard.")
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// cmd search
// ──────────────────────────────────────────────────────────────────────────────

func runCmdSearch(ctx context.Context, svc *Services, args []string) {
	flags, err := parseCmdSearchFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	results, err := svc.instructionSvc.SearchInstructions(ctx, flags.Query, flags.Project, flags.Limit)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if len(results) == 0 {
		fmt.Println("No instructions found.")
		return
	}

	printCmdSearchResults(results)
}

// ──────────────────────────────────────────────────────────────────────────────
// cmd list
// ──────────────────────────────────────────────────────────────────────────────

func runCmdList(ctx context.Context, svc *Services, args []string) {
	flags, err := parseCmdListFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	items, err := svc.instructionSvc.ListInstructions(ctx, flags.Project)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if len(items) == 0 {
		fmt.Println("No instructions stored.")
		return
	}

	fmt.Printf("%-24s  %-15s  %-45s  %-12s  %s\n", "TITLE", "ALIAS", "COMMAND", "VARS", "TAGS")
	fmt.Println(strings.Repeat("─", 115))
	for _, item := range items {
		tagNames := tagsToNames(item.Tags)
		alias := extractAlias(tagNames)
		title := truncate(item.Entry.Title, 24)
		cmd := truncate(item.Entry.BodyOptional, 45)
		allVars := vars.Detect(item.Entry.BodyOptional)
		varsStr := truncate(strings.Join(allVars, ","), 12)
		tags := joinNonAlias(tagNames)
		fmt.Printf("%-24s  %-15s  %-45s  %-12s  %s\n", title, alias, cmd, varsStr, tags)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// flag parsers
// ──────────────────────────────────────────────────────────────────────────────

type cmdAddFlags struct {
	Command string
	Title   string
	Summary string
	Alias   string
	Tags    string
	Project string
}

func parseCmdAddFlags(args []string) (*cmdAddFlags, error) {
	pos := findActionPos(args, "add")
	if pos == -1 || len(args) <= pos+1 {
		return nil, fmt.Errorf("usage: skillvault cmd add <command> [--alias <alias>] [--title <title>] [--summary <summary>] [--tags tag1,tag2] [--project <proj>]")
	}
	f := &cmdAddFlags{Command: args[pos+1]}
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

type cmdGetFlags struct {
	Alias string
	Vars  map[string]string
	Copy  bool
	Raw   bool
}

func parseCmdGetFlags(args []string) (*cmdGetFlags, error) {
	pos := findActionPos(args, "get")
	if pos == -1 || len(args) <= pos+1 {
		return nil, fmt.Errorf("usage: skillvault cmd get <alias> [--var key=value] [--copy] [--raw]")
	}
	f := &cmdGetFlags{
		Alias: args[pos+1],
		Vars:  make(map[string]string),
	}
	rest := args[pos+2:]
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--copy", "-c":
			f.Copy = true
		case "--raw", "-r":
			f.Raw = true
		case "--var", "-v":
			i++
			if i >= len(rest) {
				return nil, fmt.Errorf("--var requires key=value")
			}
			parts := strings.SplitN(rest[i], "=", 2)
			if len(parts) == 2 {
				f.Vars[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			} else {
				f.Vars[strings.TrimSpace(parts[0])] = ""
			}
		default:
			// Check if arg is key=value without --var prefix
			if strings.Contains(rest[i], "=") && !strings.HasPrefix(rest[i], "-") {
				parts := strings.SplitN(rest[i], "=", 2)
				f.Vars[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}
	return f, nil
}

type cmdSearchFlags struct {
	Query   string
	Project string
	Limit   int
}

func parseCmdSearchFlags(args []string) (*cmdSearchFlags, error) {
	pos := findActionPos(args, "search")
	if pos == -1 || len(args) <= pos+1 {
		return nil, fmt.Errorf("usage: skillvault cmd search <query> [--project <proj>] [--limit N]")
	}
	f := &cmdSearchFlags{Query: args[pos+1], Limit: 20}
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

type cmdListFlags struct {
	Project string
}

func parseCmdListFlags(args []string) (*cmdListFlags, error) {
	f := &cmdListFlags{}
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

func printCmdSearchResults(results []domain.EntrySearchResult) {
	fmt.Printf("%-24s  %-15s  %-45s  %-12s  %s\n", "TITLE", "ALIAS", "COMMAND", "VARS", "TAGS")
	fmt.Println(strings.Repeat("─", 115))
	for _, r := range results {
		tagNames := tagsToNames(r.Tags)
		alias := extractAlias(tagNames)
		title := truncate(r.Entry.Title, 24)
		cmd := truncate(r.Entry.BodyOptional, 45)
		allVars := vars.Detect(r.Entry.BodyOptional)
		varsStr := truncate(strings.Join(allVars, ","), 12)
		tags := joinNonAlias(tagNames)
		fmt.Printf("%-24s  %-15s  %-45s  %-12s  %s\n", title, alias, cmd, varsStr, tags)
	}
}

func isTerminalInput() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

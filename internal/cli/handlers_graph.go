package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/quantum-6/skillvault/internal/app"
	"github.com/quantum-6/skillvault/internal/domain"
)

func runGraph(ctx context.Context, svc *Services, args []string) {
	flags, err := ParseGraphFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	result, err := svc.entryRefSvc.GetEntryGraph(ctx, app.GetGraphInput{
		EntryID:   flags.EntryID,
		Direction: flags.Direction,
		MaxDepth:  flags.Depth,
	})
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	switch flags.Format {
	case "json":
		out := map[string]interface{}{
			"root_entry": flags.EntryID,
			"direction":  flags.Direction,
			"depth":      flags.Depth,
			"node_count": len(result.Nodes),
			"edge_count": len(result.Edges),
			"nodes":      result.Nodes,
			"edges":      result.Edges,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))

	case "dot":
		fmt.Println("digraph G {")
		fmt.Printf("  // Root: %s, %d nodes, %d edges\n", flags.EntryID, len(result.Nodes), len(result.Edges))
		for _, n := range result.Nodes {
			fmt.Printf("  %q;\n", n.EntryID)
		}
		for _, e := range result.Edges {
			label := e.Label
			if label == "" {
				label = string(e.RelationType)
			}
			fmt.Printf("  %q -> %q [label=%q];\n", e.FromEntryID, e.ToEntryID, label)
		}
		fmt.Println("}")

	default: // mermaid
		fmt.Println("graph TD")
		fmt.Printf("  %% Root: %s, depth %d, direction: %s\n", flags.EntryID, flags.Depth, flags.Direction)
		for _, e := range result.Edges {
			label := e.Label
			if label == "" {
				label = string(e.RelationType)
			}
			fmt.Printf("  %s -->|%s| %s\n", e.FromEntryID, label, e.ToEntryID)
		}
		if len(result.Edges) == 0 {
			fmt.Printf("  %s[\"No connections\"]\n", flags.EntryID)
		}
	}
}

func runEntryHistory(ctx context.Context, svc *Services, args []string) {
	flags, err := ParseEntryHistoryFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	versions, err := svc.entryVersionSvc.ListVersions(ctx, flags.ID)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	if len(versions) == 0 {
		fmt.Printf("No version history for entry %s.\n", flags.ID)
		return
	}

	fmt.Printf("Version history for entry %s:\n", flags.ID)
	fmt.Print(FormatTable(versions,
		[]string{"#", "Title", "Saved At"},
		func(v domain.EntryVersion) []string {
			return []string{
				fmt.Sprintf("%d", v.VersionNumber),
				v.Title,
				v.SavedAt,
			}
		}))
}

func runEntryRestore(ctx context.Context, svc *Services, args []string) {
	flags, err := ParseEntryRestoreFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}

	if err := svc.entryVersionSvc.RestoreVersion(ctx, flags.ID, flags.Version); err != nil {
		PrintError(err)
		os.Exit(1)
	}

	fmt.Printf("Restored entry %s to version %d.\n", flags.ID, flags.Version)
}

func runEntryRef(ctx context.Context, svc *Services, args []string) {
	subIdx := -1
	for i, a := range args {
		if a == "add" || a == "list" || a == "remove" {
			subIdx = i
			break
		}
	}
	if subIdx == -1 {
		PrintError(fmt.Errorf("usage: skillvault entry ref <add|list|remove> [args...]"))
		os.Exit(1)
	}

	sub := args[subIdx]
	subArgs := args[subIdx+1:]

	switch sub {
	case "add":
		// Supports:
		// skillvault entry ref add <source_id> <target_id> <ref_type> [--label <txt>]
		// skillvault entry ref add <source_id> <target_id> --rel <ref_type> [--label <txt>]
		// skillvault entry ref add <source_id> <target_id> --type <ref_type> [--label <txt>]
		if len(subArgs) < 2 {
			PrintError(fmt.Errorf("usage: skillvault entry ref add <source_id> <target_id> [<ref_type>] [--rel <type>] [--label <txt>]"))
			os.Exit(1)
		}
		sourceID := subArgs[0]
		targetID := subArgs[1]
		refType := ""
		label := ""

		idx := 2
		for idx < len(subArgs) {
			a := subArgs[idx]
			if (a == "--rel" || a == "--type") && idx+1 < len(subArgs) {
				refType = subArgs[idx+1]
				idx += 2
			} else if a == "--label" && idx+1 < len(subArgs) {
				label = subArgs[idx+1]
				idx += 2
			} else if !strings.HasPrefix(a, "-") && refType == "" {
				refType = a
				idx++
			} else {
				idx++
			}
		}

		if refType == "" {
			PrintError(fmt.Errorf("usage: skillvault entry ref add <source_id> <target_id> <ref_type> (or --rel <type>)"))
			os.Exit(1)
		}

		link, err := svc.entryRefSvc.SaveRef(ctx, app.AddRefInput{
			SourceID: sourceID,
			TargetID: targetID,
			RefType:  refType,
			Label:    label,
		})
		if err != nil {
			PrintError(err)
			os.Exit(1)
		}
		fmt.Printf("Saved ref: %s --[%s]--> %s\n", link.FromEntryID, link.RelationType, link.ToEntryID)
		if link.Label != "" {
			fmt.Printf("  Label: %s\n", link.Label)
		}

	case "list":
		var srcPtr, tgtPtr, typePtr *string
		includeArchived := false
		for i := 0; i < len(subArgs); i++ {
			a := subArgs[i]
			v := ""
			if i+1 < len(subArgs) {
				v = subArgs[i+1]
			}
			switch a {
			case "--source":
				srcPtr = &v
				i++
			case "--target":
				tgtPtr = &v
				i++
			case "--type", "--rel":
				typePtr = &v
				i++
			case "--include-archived":
				includeArchived = true
			}
		}
		links, err := svc.entryRefSvc.ListRefs(ctx, app.ListRefsInput{
			SourceID:        srcPtr,
			TargetID:        tgtPtr,
			RefType:         typePtr,
			IncludeArchived: includeArchived,
		})
		if err != nil {
			PrintError(err)
			os.Exit(1)
		}
		if len(links) == 0 {
			fmt.Println("No refs found.")
			return
		}
		fmt.Printf("Found %d ref(s):\n", len(links))
		for _, l := range links {
			la := ""
			if l.Label != "" {
				la = fmt.Sprintf(" (%s)", l.Label)
			}
			fmt.Printf("  %s --[%s%s]--> %s\n", l.FromEntryID, l.RelationType, la, l.ToEntryID)
		}

	case "remove":
		if len(subArgs) < 3 {
			PrintError(fmt.Errorf("usage: skillvault entry ref remove <source_id> <target_id> <ref_type>"))
			os.Exit(1)
		}
		sourceID := subArgs[0]
		targetID := subArgs[1]
		refType := subArgs[2]
		if err := svc.entryRefSvc.RemoveRef(ctx, sourceID, targetID, refType); err != nil {
			PrintError(err)
			os.Exit(1)
		}
		fmt.Printf("Removed ref: %s --[%s]--> %s\n", sourceID, refType, targetID)

	default:
		PrintError(fmt.Errorf("unknown entry ref subcommand: %s", sub))
		os.Exit(1)
	}
}

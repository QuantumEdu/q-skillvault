package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/quantum-6/skillvault/internal/app"
)

type sessionContinuityFlags struct {
	Checkpoint app.SessionCheckpointInput
	MaxChars   int
	JSON       bool
}

func parseSessionContinuityFlags(args []string) (sessionContinuityFlags, error) {
	var result sessionContinuityFlags
	fs := flag.NewFlagSet("session-continuity", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&result.Checkpoint.Project, "project", "", "Explicit project ID or slug")
	fs.StringVar(&result.Checkpoint.SessionID, "session-id", "", "Stable session identity")
	fs.StringVar(&result.Checkpoint.AgentID, "agent-id", "", "Producer agent identity (also required to resume)")
	fs.BoolVar(&result.JSON, "json", false, "JSON output")
	if len(args) < 3 {
		return result, fmt.Errorf("mem checkpoint or mem resume is required")
	}
	switch args[2] {
	case "checkpoint":
		fs.StringVar(&result.Checkpoint.OperationID, "operation-id", "", "Stable checkpoint retry identity")
		fs.StringVar(&result.Checkpoint.Summary, "summary", "", "Checkpoint summary (or stdin)")
		fs.StringVar(&result.Checkpoint.Event, "event", "manual", "manual|before_compact|agent_end|session_switch")
		fs.Func("decision", "Validated decision (repeatable)", func(value string) error {
			result.Checkpoint.Decisions = append(result.Checkpoint.Decisions, value)
			return nil
		})
		fs.Func("pending", "Pending work (repeatable)", func(value string) error {
			result.Checkpoint.Pending = append(result.Checkpoint.Pending, value)
			return nil
		})
	case "resume":
		fs.IntVar(&result.MaxChars, "max-chars", 12000, "Context budget in Unicode characters (1–64000)")
	default:
		return result, fmt.Errorf("unknown session continuity command")
	}
	if err := fs.Parse(args[3:]); err != nil {
		return result, err
	}
	if fs.NArg() != 0 {
		return result, fmt.Errorf("unexpected positional arguments")
	}
	if result.Checkpoint.Project == "" || result.Checkpoint.SessionID == "" || result.Checkpoint.AgentID == "" {
		return result, fmt.Errorf("--project, --session-id and --agent-id are required")
	}
	if args[2] == "checkpoint" && result.Checkpoint.OperationID == "" {
		return result, fmt.Errorf("--operation-id is required")
	}
	if args[2] == "resume" && (result.MaxChars < 1 || result.MaxChars > 64000) {
		return result, fmt.Errorf("--max-chars must be between 1 and 64000")
	}
	return result, nil
}
func runSessionContinuity(ctx context.Context, svc *Services, args []string) {
	flags, err := parseSessionContinuityFlags(args)
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	var output interface{}
	if args[2] == "checkpoint" {
		if flags.Checkpoint.Summary == "" {
			if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
				data, readErr := io.ReadAll(io.LimitReader(os.Stdin, 65537))
				if readErr != nil {
					PrintError(readErr)
					os.Exit(1)
				}
				if len(data) > 65536 {
					PrintError(fmt.Errorf("checkpoint stdin exceeds 64 KiB"))
					os.Exit(1)
				}
				flags.Checkpoint.Summary = string(data)
			}
		}
		output, err = svc.entrySvc.CheckpointSession(ctx, flags.Checkpoint)
	} else {
		output, err = svc.entrySvc.ResumeSession(ctx, app.SessionResumeInput{SessionIdentity: flags.Checkpoint.SessionIdentity, MaxChars: flags.MaxChars})
	}
	if err != nil {
		PrintError(err)
		os.Exit(1)
	}
	if flags.JSON {
		encoded, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(encoded))
		return
	}
	switch result := output.(type) {
	case *app.SessionCheckpointResult:
		fmt.Printf("Session checkpoint saved: %s (replayed=%t, deduplicated=%t)\n", result.ID, result.Replayed, result.Deduplicated)
	case *app.SessionResumeResult:
		if !result.Found {
			fmt.Println("No checkpoint for this project/session/producer-agent.")
			return
		}
		fmt.Printf("Checkpoint: %s\n%s\n", result.CheckpointID, result.Context)
		if result.Truncated {
			fmt.Println("[Context truncated; increase --max-chars or get the checkpoint by ID.]")
		}
	}
}

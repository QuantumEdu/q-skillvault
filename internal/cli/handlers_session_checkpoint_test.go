package cli

import "testing"

func TestSessionContinuityCLIFlagsAndRouting(t *testing.T) {
	for _, command := range []string{"checkpoint", "resume"} {
		args := []string{"skillvault", "mem", command, "--project", "p", "--session-id", "s", "--agent-id", "codex", "--json"}
		if command == "checkpoint" {
			args = append(args, "--operation-id", "op", "--summary", "Summary", "--decision", "First", "--decision", "Second", "--pending", "Tests")
		}
		route, err := ParseCommand(args)
		if err != nil || route != "mem-"+command {
			t.Fatalf("routing: %s %v", route, err)
		}
		flags, err := parseSessionContinuityFlags(args)
		if err != nil {
			t.Fatal(err)
		}
		if !flags.JSON || flags.Checkpoint.Project != "p" || flags.Checkpoint.SessionID != "s" || flags.Checkpoint.AgentID != "codex" {
			t.Fatalf("lost identities: %+v", flags)
		}
		if command == "checkpoint" && len(flags.Checkpoint.Decisions) != 2 {
			t.Fatal("lost repeated decisions")
		}
	}
	for _, args := range [][]string{
		{"skillvault", "mem", "resume", "--project", "p"},
		{"skillvault", "mem", "checkpoint", "--project", "p", "--session-id", "s", "--agent-id", "a"},
		{"skillvault", "mem", "resume", "--project", "p", "--session-id", "s", "--agent-id", "a", "--max-chars", "0"},
		{"skillvault", "mem", "resume", "--project", "p", "--session-id", "s", "--agent-id", "a", "unexpected"},
	} {
		if _, err := parseSessionContinuityFlags(args); err == nil {
			t.Fatalf("accepted invalid flags: %v", args)
		}
	}
}

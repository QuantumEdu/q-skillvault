package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/quantum-6/skillvault/internal/app"
)

func TestSessionContinuityMCP(t *testing.T) {
	reg, _, cleanup := setupMCPServices(t)
	defer cleanup()
	ctx := context.Background()
	input := map[string]interface{}{"project": "p", "session_id": "s", "agent_id": "codex", "operation_id": "op", "summary": "Validated decision", "decisions": []interface{}{"SQLite"}, "pending": []interface{}{"Tests"}, "event": "before_compact"}
	for index := 0; index < 2; index++ {
		result, err := reg.Call(ctx, "vault_session_checkpoint", input)
		if err != nil || result.IsError {
			t.Fatalf("save: %+v %v", result, err)
		}
		var checkpoint app.SessionCheckpointResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &checkpoint); err != nil {
			t.Fatal(err)
		}
		if checkpoint.Replayed != (index == 1) {
			t.Fatal("MCP retry changed semantics")
		}
	}
	resume := map[string]interface{}{"project": "p", "session_id": "s", "agent_id": "codex", "max_chars": float64(100)}
	result, err := reg.Call(ctx, "vault_session_resume", resume)
	if err != nil || result.IsError {
		t.Fatalf("resume: %+v %v", result, err)
	}
	var contextResult app.SessionResumeResult
	if err := json.Unmarshal([]byte(result.Content[0].Text), &contextResult); err != nil {
		t.Fatal(err)
	}
	if !contextResult.Found || contextResult.Context == "" || len([]rune(contextResult.Context)) > 100 {
		t.Fatal("MCP lost context or budget")
	}
	resume["agent_id"] = "omp"
	result, err = reg.Call(ctx, "vault_session_resume", resume)
	if err != nil || result.IsError {
		t.Fatalf("other producer: %+v %v", result, err)
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &contextResult); err != nil {
		t.Fatal(err)
	}
	if contextResult.Found {
		t.Fatal("cross-agent fallback")
	}
	for _, bad := range []interface{}{true, "30", 3.5, 0, -1, 64001} {
		resume["max_chars"] = bad
		result, err := reg.Call(ctx, "vault_session_resume", resume)
		if err != nil || !result.IsError {
			t.Fatalf("accepted budget %+v", bad)
		}
	}
	for _, key := range []string{"project", "session_id", "agent_id", "operation_id", "summary", "event", "decisions", "pending"} {
		copy := map[string]interface{}{}
		for k, v := range input {
			copy[k] = v
		}
		copy[key] = true
		result, err := reg.Call(ctx, "vault_session_checkpoint", copy)
		if err != nil || !result.IsError {
			t.Fatalf("accepted malformed %s", key)
		}
	}
}

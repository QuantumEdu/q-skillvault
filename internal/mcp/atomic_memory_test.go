package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/quantum-6/skillvault/internal/app"
)

func TestMemoryOperationMCPAliasesAndHints(t *testing.T) {
	reg, _, cleanup := setupMCPServices(t)
	defer cleanup()
	reg.WithCompatEngram(true)
	input := map[string]interface{}{"content": "Atomic durable fact", "title": "Atomic fact", "operation_id": "mcp-event", "project": "p", "pinned": true, "review_after": "2027-01-01", "expires_at": "2027-06-01"}
	var original string
	for i, name := range []string{"vault_mem_save", "mem_save"} {
		result, err := reg.Call(context.Background(), name, input)
		if err != nil || result.IsError {
			t.Fatalf("%s: %+v %v", name, result, err)
		}
		var saved app.VaultMemSaveResult
		if err := json.Unmarshal([]byte(result.Content[0].Text), &saved); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			original = saved.ID
			if saved.Replayed {
				t.Fatal("first call is a replay")
			}
		} else if !saved.Replayed || saved.ID != original {
			t.Fatal("alias did not replay original operation")
		}
	}
	input["content"] = "conflicting change"
	result, err := reg.Call(context.Background(), "mem_save", input)
	if err != nil || !result.IsError {
		t.Fatal("MCP operation conflict was not surfaced")
	}
}

func TestMemoryOperationMCPRejectsWrongTypes(t *testing.T) {
	reg, _, cleanup := setupMCPServices(t)
	defer cleanup()
	for _, key := range []string{"operation_id", "review_after", "expires_at", "pinned"} {
		result, err := reg.Call(context.Background(), "vault_mem_save", map[string]interface{}{"content": "not to be saved", key: float64(17)})
		if err != nil || !result.IsError {
			t.Fatalf("invalid %s accepted", key)
		}
	}
}

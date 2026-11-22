package mcp

import (
	"context"
	"encoding/json"
	"math"

	"github.com/quantum-6/skillvault/internal/app"
)

func (r *ToolRegistry) handleSessionContinuity(ctx context.Context, name string, args map[string]interface{}) (*ToolCallResult, error) {
	if r.entrySvc == nil {
		return errResult("session continuity service unavailable"), nil
	}
	// Strict validation is essential: a malformed identity cannot become a broad query.
	for _, key := range []string{"project", "session_id", "agent_id", "operation_id", "summary", "event"} {
		if value, ok := args[key]; ok {
			if _, ok := value.(string); !ok {
				return errResult(key + " must be a string"), nil
			}
		}
	}
	for _, key := range []string{"decisions", "pending"} {
		if value, ok := args[key]; ok {
			switch list := value.(type) {
			case []string:
			case []interface{}:
				for _, item := range list {
					if _, ok := item.(string); !ok {
						return errResult(key + " must contain strings"), nil
					}
				}
			default:
				return errResult(key + " must be an array of strings"), nil
			}
		}
	}
	identity := app.SessionIdentity{Project: strArg(args, "project"), SessionID: strArg(args, "session_id"), AgentID: strArg(args, "agent_id")}
	if name == "vault_session_checkpoint" {
		result, err := r.entrySvc.CheckpointSession(ctx, app.SessionCheckpointInput{SessionIdentity: identity, OperationID: strArg(args, "operation_id"), Summary: strArg(args, "summary"), Event: strArg(args, "event"), Decisions: parseStrings(args["decisions"]), Pending: parseStrings(args["pending"])})
		if err != nil {
			return errResult(err.Error()), nil
		}
		return jsonResult(result), nil
	}
	maxChars := 0
	if value, ok := args["max_chars"]; ok {
		encoded, err := json.Marshal(value)
		if err != nil {
			return errResult("max_chars must be an integer between 1 and 64000"), nil
		}
		var number float64
		if json.Unmarshal(encoded, &number) != nil || math.Trunc(number) != number || number < 1 || number > 64000 {
			return errResult("max_chars must be an integer between 1 and 64000"), nil
		}
		maxChars = int(number)
	}
	result, err := r.entrySvc.ResumeSession(ctx, app.SessionResumeInput{SessionIdentity: identity, MaxChars: maxChars})
	if err != nil {
		return errResult(err.Error()), nil
	}
	return jsonResult(result), nil
}

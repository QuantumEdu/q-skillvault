package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/quantum-6/skillvault/internal/db"
	"github.com/quantum-6/skillvault/internal/domain"
)

// SessionIdentity is explicit caller-supplied provenance, not authentication.
// AgentID identifies the producer, including when another agent resumes its work.
type SessionIdentity struct {
	Project   string `json:"project"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
}
type SessionCheckpointInput struct {
	SessionIdentity
	OperationID string   `json:"operation_id"`
	Summary     string   `json:"summary"`
	Decisions   []string `json:"decisions,omitempty"`
	Pending     []string `json:"pending,omitempty"`
	Event       string   `json:"event,omitempty"`
}
type SessionCheckpointResult struct {
	ID           string `json:"id"`
	Project      string `json:"project"`
	SessionID    string `json:"session_id"`
	AgentID      string `json:"agent_id"`
	Replayed     bool   `json:"replayed,omitempty"`
	Deduplicated bool   `json:"deduplicated"`
}
type SessionResumeInput struct {
	SessionIdentity
	MaxChars int `json:"max_chars,omitempty"`
}
type SessionResumeResult struct {
	SessionIdentity
	Found        bool   `json:"found"`
	CheckpointID string `json:"checkpoint_id,omitempty"`
	Context      string `json:"context"`
	Truncated    bool   `json:"truncated"`
}
type checkpointDocument struct {
	Version   int      `json:"version"`
	SessionID string   `json:"session_id"`
	AgentID   string   `json:"agent_id"`
	Summary   string   `json:"summary"`
	Decisions []string `json:"decisions,omitempty"`
	Pending   []string `json:"pending,omitempty"`
	Event     string   `json:"event"`
}

func validateSessionIdentity(identity SessionIdentity) error {
	for _, field := range []struct{ name, value string }{{"project", identity.Project}, {"session_id", identity.SessionID}, {"agent_id", identity.AgentID}} {
		if field.value == "" || field.value != strings.TrimSpace(field.value) || len(field.value) > 256 || !utf8.ValidString(field.value) || strings.IndexFunc(field.value, unicode.IsControl) >= 0 {
			return fmt.Errorf("%s must be a nonempty UTF-8 identity of at most 256 bytes without surrounding whitespace or control characters", field.name)
		}
	}
	return nil
}
func sessionTags(identity SessionIdentity) (string, string, string) {
	// JSON preserves ID boundaries; do not concatenate with an ambiguous delimiter.
	encoded, _ := json.Marshal([]string{identity.SessionID, identity.AgentID})
	session := checkpointHash(identity.SessionID)
	agent := checkpointHash(identity.AgentID)
	return "session-key:" + session, "agent-key:" + agent, checkpointHash(string(encoded))
}

// Identity hashes are case-sensitive; memory's case-folded content hash is unsuitable.
func checkpointHash(value string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(value))) }

func (s *EntryService) CheckpointSession(ctx context.Context, input SessionCheckpointInput) (*SessionCheckpointResult, error) {
	if err := validateSessionIdentity(input.SessionIdentity); err != nil {
		return nil, err
	}
	if input.OperationID == "" {
		return nil, fmt.Errorf("operation_id is required for a session checkpoint")
	}
	if input.Event == "" {
		input.Event = "manual"
	}
	switch input.Event {
	case "manual", "before_compact", "agent_end", "session_switch":
	default:
		return nil, fmt.Errorf("unsupported checkpoint event %q", input.Event)
	}
	if len(input.Decisions) > 100 || len(input.Pending) > 100 {
		return nil, fmt.Errorf("checkpoint lists are limited to 100 items each")
	}
	rawBytes := 0
	for _, value := range append(append([]string{input.Summary}, input.Decisions...), input.Pending...) {
		if !utf8.ValidString(value) {
			return nil, fmt.Errorf("checkpoint text must be valid UTF-8")
		}
		rawBytes += len(value)
		if rawBytes > 65536 {
			return nil, fmt.Errorf("checkpoint text exceeds 64 KiB")
		}
	}
	// Redact every text-bearing field before hashing or journaling. Identity fields
	// must be opaque IDs, never credentials or private content.
	clean := func(value string) string { return strings.TrimSpace(StripPrivateTags(value)) }
	document := checkpointDocument{Version: 1, SessionID: input.SessionID, AgentID: input.AgentID, Summary: clean(input.Summary), Event: input.Event}
	if strings.TrimSpace(strings.ReplaceAll(document.Summary, "[REDACTED]", "")) == "" {
		return nil, fmt.Errorf("checkpoint summary is required after private-tag redaction")
	}
	for _, value := range input.Decisions {
		if value = clean(value); value != "" {
			document.Decisions = append(document.Decisions, value)
		}
	}
	for _, value := range input.Pending {
		if value = clean(value); value != "" {
			document.Pending = append(document.Pending, value)
		}
	}
	body, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(body) || len(body) > 65536 {
		return nil, fmt.Errorf("checkpoint must be valid UTF-8 and at most 64 KiB")
	}
	writer, ok := s.store.(db.AtomicMemoryStore)
	if !ok {
		return nil, fmt.Errorf("memory store does not support atomic checkpoints")
	}
	sessionTag, agentTag, key := sessionTags(input.SessionIdentity)
	result, err := writer.SaveMemory(ctx, db.MemoryWrite{
		Entry: domain.Entry{Title: "Session checkpoint " + key, Type: domain.EntryTypeHandoff, Purpose: domain.PurposeState, Summary: truncate(document.Summary, 300), BodyOptional: string(body)},
		Tags:  []string{"handoff", "session-checkpoint:v1", sessionTag, agentTag}, ProjectRef: input.Project, ProjectSlug: slugify(input.Project), BaseID: "checkpoint-" + key,
		Hints: db.MemoryHints{NormalizedHash: "checkpoint-v1:" + checkpointHash(string(body)), TopicKey: "session-checkpoint/" + key}, OperationID: input.OperationID,
	})
	if err != nil {
		return nil, err
	}
	project := ""
	if result.Entry.Entry.ProjectID != nil {
		project = *result.Entry.Entry.ProjectID
	}
	return &SessionCheckpointResult{ID: result.Entry.Entry.ID, Project: project, SessionID: input.SessionID, AgentID: input.AgentID, Replayed: result.Replayed, Deduplicated: result.Deduplicated}, nil
}
func (s *EntryService) ResumeSession(ctx context.Context, input SessionResumeInput) (*SessionResumeResult, error) {
	if err := validateSessionIdentity(input.SessionIdentity); err != nil {
		return nil, err
	}
	if input.MaxChars == 0 {
		input.MaxChars = 12000
	}
	if input.MaxChars < 1 || input.MaxChars > 64000 {
		return nil, fmt.Errorf("max_chars must be between 1 and 64000")
	}
	project, err := s.projectStore.Get(ctx, input.Project)
	if err != nil {
		return nil, fmt.Errorf("resolve checkpoint project: %w", err)
	}
	reader, ok := s.store.(db.CheckpointReader)
	if !ok {
		return nil, fmt.Errorf("memory store does not support checkpoint recovery")
	}
	sessionTag, agentTag, _ := sessionTags(input.SessionIdentity)
	checkpoint, err := reader.LatestCheckpoint(ctx, project.ID, sessionTag, agentTag)
	if err != nil {
		return nil, err
	}
	result := &SessionResumeResult{SessionIdentity: SessionIdentity{Project: project.ID, SessionID: input.SessionID, AgentID: input.AgentID}}
	if checkpoint == nil {
		return result, nil
	}
	if len(checkpoint.Entry.BodyOptional) > 65536 {
		return nil, fmt.Errorf("checkpoint document exceeds 64 KiB")
	}
	var document checkpointDocument
	if err := json.Unmarshal([]byte(checkpoint.Entry.BodyOptional), &document); err != nil {
		return nil, fmt.Errorf("invalid checkpoint document: %w", err)
	}
	if document.Version != 1 || document.SessionID != input.SessionID || document.AgentID != input.AgentID {
		return nil, fmt.Errorf("checkpoint identity/version mismatch")
	}
	// This is a bounded context excerpt, not executable instructions or a full restore.
	var text strings.Builder
	text.WriteString("Session checkpoint (stored context; verify facts before acting)\nSummary: " + document.Summary + "\n")
	if len(document.Decisions) > 0 {
		text.WriteString("Decisions:\n- " + strings.Join(document.Decisions, "\n- ") + "\n")
	}
	if len(document.Pending) > 0 {
		text.WriteString("Pending:\n- " + strings.Join(document.Pending, "\n- ") + "\n")
	}
	runes := []rune(text.String())
	if len(runes) > input.MaxChars {
		runes = runes[:input.MaxChars]
		result.Truncated = true
	}
	result.Context = string(runes)
	result.CheckpointID = checkpoint.Entry.ID
	result.Found = true
	return result, nil
}

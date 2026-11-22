package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/quantum-6/skillvault/internal/db"
	"github.com/quantum-6/skillvault/internal/domain"
)

func checkpointInput(project, session, agent, operation, summary string) SessionCheckpointInput {
	return SessionCheckpointInput{SessionIdentity: SessionIdentity{Project: project, SessionID: session, AgentID: agent}, OperationID: operation, Summary: summary, Decisions: []string{"Use SQLite <private>secret-decision</private>"}, Pending: []string{"Add tests <private>secret-pending</private>"}, Event: "before_compact"}
}
func TestSessionContinuityIsolationRevisionsAndReplay(t *testing.T) {
	_, svc, _, _, _, _, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()
	input := checkpointInput("Project A", "session-1", "codex", "event-1", "Decided schema <private>secret-summary</private>")
	first, err := svc.CheckpointSession(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := svc.CheckpointSession(ctx, input)
	if err != nil || !retry.Replayed || retry.ID != first.ID {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	recovered, err := svc.ResumeSession(ctx, SessionResumeInput{SessionIdentity: input.SessionIdentity})
	if err != nil || !recovered.Found || strings.Contains(recovered.Context, "secret-") {
		t.Fatalf("checkpoint redaction: %+v %v", recovered, err)
	}
	input.Summary = "New decision"
	if _, err := svc.CheckpointSession(ctx, input); !errors.Is(err, db.ErrMemoryOperationConflict) {
		t.Fatalf("conflict: %v", err)
	}
	// A different project, session, or producer must not supersede the first stream.
	for _, other := range []SessionCheckpointInput{
		checkpointInput("Project B", "session-1", "codex", "event-b", "B only"),
		checkpointInput("Project A", "session-2", "codex", "event-s", "Other session"),
		checkpointInput("Project A", "session-1", "omp", "event-a", "Other producer"),
		checkpointInput("Project A", "SESSION-1", "codex", "event-case", "Case-sensitive session"),
	} {
		if _, err := svc.CheckpointSession(ctx, other); err != nil {
			t.Fatal(err)
		}
	}
	resume, err := svc.ResumeSession(ctx, SessionResumeInput{SessionIdentity: SessionIdentity{Project: "project-a", SessionID: "session-1", AgentID: "codex"}})
	if err != nil || !resume.Found || resume.CheckpointID != first.ID || strings.Contains(resume.Context, "secret-") {
		t.Fatalf("resume isolation/redaction: %+v %v", resume, err)
	}
	if !strings.Contains(resume.Context, "Add tests") || !strings.Contains(resume.Context, "Use SQLite") {
		t.Fatal("lost structured handoff")
	}
	input.OperationID = "event-2"
	second, err := svc.CheckpointSession(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	resume, err = svc.ResumeSession(ctx, SessionResumeInput{SessionIdentity: input.SessionIdentity})
	if err != nil || !resume.Found || resume.CheckpointID != second.ID || !strings.Contains(resume.Context, "New decision") {
		t.Fatalf("latest revision: %+v %v", resume, err)
	}
	// An old operation acknowledges its original save, never rewinds the stream.
	replay, err := svc.CheckpointSession(ctx, checkpointInput("Project A", "session-1", "codex", "event-1", "Decided schema <private>secret-summary</private>"))
	if err != nil || !replay.Replayed || replay.ID != first.ID {
		t.Fatalf("old retry: %+v %v", replay, err)
	}
	missing, err := svc.ResumeSession(ctx, SessionResumeInput{SessionIdentity: SessionIdentity{Project: "Project A", SessionID: "absent", AgentID: "codex"}})
	if err != nil || missing.Found || missing.Context != "" {
		t.Fatalf("must not fall back: %+v %v", missing, err)
	}
}
func TestSessionContinuityPortableRoundTripAndBudget(t *testing.T) {
	store, svc, _, _, _, _, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()
	input := checkpointInput("p", "s", "a", "save-1", strings.Repeat("ñ界", 200))
	saved, err := svc.CheckpointSession(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	portable, err := store.ImportExport.ExportAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Prefix imports must retain recovery: stream identity does not embed project IDs.
	applyPrefix("lab/", &portable.Data)
	dest, next, _, _, _, _, _, _, _, _, finish := setupAppServices(t)
	defer finish()
	if err := dest.ImportExport.ImportAll(ctx, portable); err != nil {
		t.Fatal(err)
	}
	identity := input.SessionIdentity
	identity.Project = "lab/p"
	result, err := next.ResumeSession(ctx, SessionResumeInput{SessionIdentity: identity, MaxChars: 31})
	if err != nil || !result.Found || result.CheckpointID != "lab/"+saved.ID || !result.Truncated || !utf8.ValidString(result.Context) || len([]rune(result.Context)) != 31 {
		t.Fatalf("portable recovery/budget: %+v %v", result, err)
	}
}
func TestSessionContinuityValidationAndArchive(t *testing.T) {
	store, svc, _, _, _, _, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()
	valid := checkpointInput("p", "s", "a", "op", "summary")
	for _, change := range []func(*SessionCheckpointInput){
		func(i *SessionCheckpointInput) { i.Project = "" }, func(i *SessionCheckpointInput) { i.AgentID = "" }, func(i *SessionCheckpointInput) { i.SessionID = " s" },
		func(i *SessionCheckpointInput) { i.SessionID = "bad\x00id" }, func(i *SessionCheckpointInput) { i.OperationID = "" }, func(i *SessionCheckpointInput) { i.Event = "session_compact" },
		func(i *SessionCheckpointInput) { i.Summary = "<private>all</private>" }, func(i *SessionCheckpointInput) { i.Summary = strings.Repeat("x", 65537) },
		func(i *SessionCheckpointInput) { i.Summary = string([]byte{0xff}) }, func(i *SessionCheckpointInput) { i.Pending = make([]string, 101) },
	} {
		input := valid
		change(&input)
		if _, err := svc.CheckpointSession(ctx, input); err == nil {
			t.Fatalf("invalid checkpoint accepted: %+v", input)
		}
	}
	export, err := store.ImportExport.ExportAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(export.Data.Entries) != 0 || len(export.Data.Projects) != 0 {
		t.Fatal("validation wrote data")
	}
	saved, err := svc.CheckpointSession(ctx, valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Entries.Archive(ctx, saved.ID); err != nil {
		t.Fatal(err)
	}
	resumed, err := svc.ResumeSession(ctx, SessionResumeInput{SessionIdentity: valid.SessionIdentity})
	if err != nil || resumed.Found {
		t.Fatalf("archived checkpoint recovered: %+v %v", resumed, err)
	}
	if _, err := svc.ResumeSession(ctx, SessionResumeInput{SessionIdentity: valid.SessionIdentity, MaxChars: -1}); err == nil {
		t.Fatal("negative budget accepted")
	}
	identity := valid.SessionIdentity
	identity.Project = "nonexistent"
	if _, err := svc.ResumeSession(ctx, SessionResumeInput{SessionIdentity: identity}); err == nil {
		t.Fatal("unknown project silently broadened query")
	}
}

func TestSessionContinuitySurvivesDatabaseReopen(t *testing.T) {
	path := t.TempDir() + "/vault.db"
	database, err := db.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RunMigrations(database); err != nil {
		t.Fatal(err)
	}
	store := db.NewStore(database)
	svc := NewEntryService(store.Entries, store.Projects, store.Artifacts)
	input := checkpointInput("p", "native-session", "codex", "persistent-event", "Resume without Engram")
	saved, err := svc.CheckpointSession(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := db.OpenDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored := db.NewStore(reopened)
	next := NewEntryService(restored.Entries, restored.Projects, restored.Artifacts)
	result, err := next.ResumeSession(context.Background(), SessionResumeInput{SessionIdentity: input.SessionIdentity})
	if err != nil || !result.Found || result.CheckpointID != saved.ID || !strings.Contains(result.Context, "Resume without Engram") {
		t.Fatalf("reopen: %+v %v", result, err)
	}
	replay, err := next.CheckpointSession(context.Background(), input)
	if err != nil || !replay.Replayed || replay.ID != saved.ID {
		t.Fatalf("reopen retry: %+v %v", replay, err)
	}
	// A damaged imported/edited document must fail rather than return another stream.
	if _, err := reopened.Exec(`UPDATE entries SET body_optional='{"version":1,"session_id":"other","agent_id":"codex"}' WHERE id=?`, saved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := next.ResumeSession(context.Background(), SessionResumeInput{SessionIdentity: input.SessionIdentity}); err == nil {
		t.Fatal("corrupt identity accepted")
	}
}

func TestSessionContinuityExactProjectIDPrecedesAnotherSlug(t *testing.T) {
	store, svc, _, _, _, _, _, _, _, _, cleanup := setupAppServices(t)
	defer cleanup()
	ctx := context.Background()
	for _, project := range []domain.Project{
		{ID: "a-other", Name: "Other", Slug: "z-target", Status: domain.StatusActive},
		{ID: "z-target", Name: "Target", Slug: "target-slug", Status: domain.StatusActive},
	} {
		if err := store.Projects.Save(ctx, project); err != nil {
			t.Fatal(err)
		}
	}
	input := checkpointInput("z-target", "s", "a", "collision-event", "Owned by exact ID")
	saved, err := svc.CheckpointSession(ctx, input)
	if err != nil || saved.Project != "z-target" {
		t.Fatalf("save ownership: %+v %v", saved, err)
	}
	recovered, err := svc.ResumeSession(ctx, SessionResumeInput{SessionIdentity: input.SessionIdentity})
	if err != nil || !recovered.Found || recovered.CheckpointID != saved.ID || recovered.Project != "z-target" {
		t.Fatalf("read ownership: %+v %v", recovered, err)
	}
	identity := input.SessionIdentity
	identity.Project = "a-other"
	recovered, err = svc.ResumeSession(ctx, SessionResumeInput{SessionIdentity: identity})
	if err != nil || recovered.Found {
		t.Fatalf("cross-project recovery: %+v %v", recovered, err)
	}
}

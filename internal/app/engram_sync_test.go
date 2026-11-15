package app

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quantum-6/skillvault/internal/db"
	"github.com/quantum-6/skillvault/internal/domain"
	_ "modernc.org/sqlite"
)

func createMockEngramDB(t *testing.T, dbPath string) {
	t.Helper()
	sqldb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sqldb.Close()

	schema := `
	CREATE TABLE sessions (
		id TEXT PRIMARY KEY,
		project TEXT,
		directory TEXT
	);
	CREATE TABLE observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT,
		type TEXT,
		title TEXT,
		content TEXT,
		project TEXT,
		topic_key TEXT,
		deleted_at TEXT
	);`
	if _, err := sqldb.Exec(schema); err != nil {
		t.Fatal(err)
	}

	_, err = sqldb.Exec(`
		INSERT INTO observations (session_id, type, title, content, project, topic_key) VALUES
		('s1', 'decision', 'Use SQLite for metadata', 'Chose modernc.org/sqlite for CGO-free portability', 'kbs', 'db/driver'),
		('s1', 'architecture', 'Telemetry architecture', 'Standalone daemon with Unix socket', 'kbs', 'arch/telemetry'),
		('s2', 'discovery', 'SIGHUP kills process group', 'Use Setsid to daemonize properly', 'kbs', 'gotchas/signals'),
		('s2', 'decision', 'Other project decision', 'Not for kbs', 'other', 'other/key');
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncEngram_ImportAndIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	engramDBPath := filepath.Join(tmpDir, "mock_engram.db")
	createMockEngramDB(t, engramDBPath)

	vaultDBPath := filepath.Join(tmpDir, "vault.db")
	sqlDB, err := db.OpenDB(vaultDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.RunMigrations(sqlDB); err != nil {
		t.Fatal(err)
	}

	store := db.NewStore(sqlDB)
	entrySvc := NewEntryService(store.Entries, store.Projects, store.Artifacts)

	ctx := context.Background()

	// 1. Initial import with project filter
	res, err := entrySvc.SyncEngram(ctx, EngramSyncOptions{
		DBPath:  engramDBPath,
		Project: "kbs",
	})
	if err != nil {
		t.Fatalf("SyncEngram failed: %v", err)
	}
	if res.Total != 3 {
		t.Errorf("expected 3 total observations for kbs, got %d", res.Total)
	}
	if res.Imported != 3 {
		t.Errorf("expected 3 imported observations, got %d", res.Imported)
	}

	// 2. Second import (idempotency check)
	res2, err := entrySvc.SyncEngram(ctx, EngramSyncOptions{
		DBPath:  engramDBPath,
		Project: "kbs",
	})
	if err != nil {
		t.Fatalf("second SyncEngram failed: %v", err)
	}
	if res2.Skipped != 3 {
		t.Errorf("expected 3 skipped observations on second run, got %d", res2.Skipped)
	}
	if res2.Imported != 0 {
		t.Errorf("expected 0 imported on second run, got %d", res2.Imported)
	}
}

func TestSyncEngram_WithLifecycleOrgAndPrivateRedaction(t *testing.T) {
	tmpDir := t.TempDir()
	engramDBPath := filepath.Join(tmpDir, "engram_v37.db")
	sqldb, err := sql.Open("sqlite", engramDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sqldb.Close()

	schema := `
	CREATE TABLE observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT,
		type TEXT,
		title TEXT,
		content TEXT,
		project TEXT,
		topic_key TEXT,
		pinned INTEGER DEFAULT 0,
		review_after TEXT,
		expires_at TEXT,
		org TEXT,
		normalized_hash TEXT,
		deleted_at TEXT
	);`
	if _, err := sqldb.Exec(schema); err != nil {
		t.Fatal(err)
	}

	_, err = sqldb.Exec(`
		INSERT INTO observations (session_id, type, title, content, project, topic_key, pinned, review_after, expires_at, org, normalized_hash) VALUES
		('s1', 'decision', 'Pin SQLite version <private>key1</private>', 'Content with <private>super-secret</private> notes', 'my-proj', 'db/config', 1, '2026-12-01', '2027-01-01', 'QuantumEdu', '');
	`)
	if err != nil {
		t.Fatal(err)
	}

	vaultDBPath := filepath.Join(tmpDir, "vault.db")
	sqlDB, err := db.OpenDB(vaultDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.RunMigrations(sqlDB); err != nil {
		t.Fatal(err)
	}

	store := db.NewStore(sqlDB)
	entrySvc := NewEntryService(store.Entries, store.Projects, store.Artifacts)

	ctx := context.Background()
	res, err := entrySvc.SyncEngram(ctx, EngramSyncOptions{
		DBPath:  engramDBPath,
		Project: "my-proj",
	})
	if err != nil {
		t.Fatalf("SyncEngram failed: %v", err)
	}
	if res.Imported != 1 {
		t.Fatalf("expected 1 imported, got %d", res.Imported)
	}

	// Verify imported entry in SkillVault
	list, err := store.Entries.List(ctx, domain.EntryFilter{ProjectID: func(s string) *string { return &s }("my-proj")})
	if err != nil {
		t.Fatalf("list entries failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry in store, got %d", len(list))
	}

	imported := list[0]
	// Check private redaction
	if strings.Contains(imported.Entry.Title, "key1") {
		t.Errorf("expected title to redact private key, got %q", imported.Entry.Title)
	}
	if !strings.Contains(imported.Entry.Title, "[REDACTED]") {
		t.Errorf("expected title to contain [REDACTED], got %q", imported.Entry.Title)
	}
	if strings.Contains(imported.Entry.BodyOptional, "super-secret") {
		t.Errorf("expected body to redact private content, got %q", imported.Entry.BodyOptional)
	}

	// Check org tag
	hasOrgTag := false
	for _, tag := range imported.Tags {
		if strings.EqualFold(tag.Name, "org:QuantumEdu") {
			hasOrgTag = true
			break
		}
	}
	if !hasOrgTag {
		t.Errorf("expected tag 'org:QuantumEdu' (case-insensitive) on imported entry, got %+v", imported.Tags)
	}

	// Verify lifecycle hints persisted in entries table
	var pinned int
	var reviewAfter, expiresAt, normHash string
	err = sqlDB.QueryRowContext(ctx, `SELECT pinned, review_after, expires_at, normalized_hash FROM entries WHERE id = ?`, imported.Entry.ID).
		Scan(&pinned, &reviewAfter, &expiresAt, &normHash)
	if err != nil {
		t.Fatalf("query lifecycle hints failed: %v", err)
	}
	if pinned != 1 {
		t.Errorf("expected pinned = 1, got %d", pinned)
	}
	if reviewAfter != "2026-12-01" {
		t.Errorf("expected review_after = '2026-12-01', got %q", reviewAfter)
	}
	if expiresAt != "2027-01-01" {
		t.Errorf("expected expires_at = '2027-01-01', got %q", expiresAt)
	}
	if normHash == "" {
		t.Errorf("expected non-empty normalized_hash")
	}
}

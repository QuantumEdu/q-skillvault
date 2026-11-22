package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/quantum-6/skillvault/internal/domain"
)

var (
	ErrMemoryOperationConflict    = errors.New("memory operation_id was already used with a different request")
	ErrMemoryOperationUnavailable = errors.New("memory operation result is archived or deleted; operation_id cannot be reused")
)

// AtomicMemoryStore is an optional capability; the existing EntryStore contract
// stays source-compatible. Memory writers fail closed without this capability.
type AtomicMemoryStore interface {
	SaveMemory(context.Context, MemoryWrite) (*MemoryWriteResult, error)
}

type MemoryWrite struct {
	Entry       domain.Entry
	Tags        []string
	ProjectRef  string
	ProjectSlug string
	BaseID      string
	Hints       MemoryHints
	OperationID string
}

type MemoryWriteResult struct {
	Entry        domain.EntryResult `json:"entry"`
	Deduplicated bool               `json:"deduplicated"`
	Replayed     bool               `json:"replayed,omitempty"`
	TopicKey     string             `json:"topic_key,omitempty"`
}

// The fingerprint is over effective, redacted request data, never timestamps,
// generated IDs, or mutable database state. Struct encoding and sorted tags keep
// retries stable; request-field aliases are resolved by the application. The
// project reference itself must remain stable across retries.
func memoryFingerprint(input MemoryWrite) (string, error) {
	payload := struct {
		Title, Body, Summary string
		Type                 domain.EntryType
		Purpose              domain.Purpose
		Project              string
		Tags                 []string
		Hints                MemoryHints
	}{input.Entry.Title, input.Entry.BodyOptional, input.Entry.Summary, input.Entry.Type, input.Entry.Purpose, input.ProjectRef, input.Tags, input.Hints}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "v1:" + hex.EncodeToString(digest[:]), nil
}

// SaveMemory obtains the SQLite writer reservation before reading identities.
// This serializes dedupe, ID allocation, project creation and supersession even
// across independent processes. No application mutex or second database is used.
func (s *sqliteEntryStore) SaveMemory(ctx context.Context, input MemoryWrite) (*MemoryWriteResult, error) {
	if input.Entry.Title == "" || input.Hints.NormalizedHash == "" || input.BaseID == "" {
		return nil, fmt.Errorf("memory title, hash and base ID are required")
	}
	if err := domain.ValidateEntryType(string(input.Entry.Type)); err != nil {
		return nil, err
	}
	if input.OperationID != strings.TrimSpace(input.OperationID) || len(input.OperationID) > 256 || strings.IndexFunc(input.OperationID, unicode.IsControl) >= 0 {
		return nil, fmt.Errorf("operation_id must be at most 256 bytes, without surrounding whitespace or control characters")
	}
	input.Tags = domain.NormalizeTags(input.Tags)
	sort.Strings(input.Tags)
	fingerprint, err := memoryFingerprint(input)
	if err != nil {
		return nil, err
	}
	// PRAGMAs are connection-local, so configure the exact connection used below.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout=5000"); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		result, err := s.saveMemoryAttempt(ctx, conn, input, fingerprint)
		if err == nil || !sqliteBusy(err) || time.Now().After(deadline) {
			return result, err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func sqliteBusy(err error) bool {
	var coded interface{ Code() int }
	return errors.As(err, &coded) && (coded.Code()&255 == 5 || coded.Code()&255 == 6)
}

func (s *sqliteEntryStore) saveMemoryAttempt(ctx context.Context, conn *sql.Conn, input MemoryWrite, fingerprint string) (*MemoryWriteResult, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// A no-row UPDATE reserves SQLite's writer lock without changing journal data.
	// All SELECTs follow it, avoiding deferred-read -> SQLITE_BUSY_SNAPSHOT races.
	if _, err := tx.ExecContext(ctx, `UPDATE memory_save_operations SET operation_id=operation_id WHERE 0`); err != nil {
		return nil, err
	}
	if input.OperationID != "" {
		var previous, encoded string
		var id sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT fingerprint,entry_id,result_json FROM memory_save_operations WHERE operation_id=?`, input.OperationID).Scan(&previous, &id, &encoded)
		if err == nil {
			if previous != fingerprint {
				return nil, ErrMemoryOperationConflict
			}
			if !id.Valid {
				return nil, ErrMemoryOperationUnavailable
			}
			var status string
			if err := tx.QueryRowContext(ctx, `SELECT status FROM entries WHERE id=?`, id.String).Scan(&status); err != nil {
				return nil, fmt.Errorf("memory operation target: %w", err)
			}
			if status == "archived" {
				return nil, ErrMemoryOperationUnavailable
			}
			var result MemoryWriteResult
			if err := json.Unmarshal([]byte(encoded), &result); err != nil {
				return nil, fmt.Errorf("invalid memory operation journal: %w", err)
			}
			if result.Entry.Entry.ID != id.String {
				return nil, fmt.Errorf("memory operation journal target mismatch")
			}
			result.Replayed = true
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return &result, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	entry := input.Entry
	entry.ProjectID = nil
	entry.ArtifactID = nil
	entry.CreatedAt = time.Time{}
	entry.UpdatedAt = time.Time{}
	entry.ExternalRef = "hash:" + input.Hints.NormalizedHash
	if input.ProjectRef != "" {
		var projectID string
		err := tx.QueryRowContext(ctx, `SELECT id FROM projects WHERE id=? OR slug=? ORDER BY CASE WHEN id=? THEN 0 ELSE 1 END LIMIT 1`, input.ProjectRef, input.ProjectRef, input.ProjectRef).Scan(&projectID)
		if errors.Is(err, sql.ErrNoRows) {
			projectID = input.ProjectRef
			if _, err := tx.ExecContext(ctx, `INSERT INTO projects(id,name,slug,status) VALUES(?,?,?,'active')`, projectID, input.ProjectRef, input.ProjectSlug); err != nil {
				return nil, fmt.Errorf("create memory project: %w", err)
			}
		} else if err != nil {
			return nil, err
		}
		entry.ProjectID = &projectID
	}
	project := ""
	if entry.ProjectID != nil {
		project = *entry.ProjectID
	}
	var existingID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM entries WHERE COALESCE(project_id,'')=? AND normalized_hash=?
 AND superseded_by IS NULL AND status!='archived' ORDER BY created_at DESC,id DESC LIMIT 1`, project, input.Hints.NormalizedHash).Scan(&existingID)
	deduplicated := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if deduplicated {
		if _, err := tx.ExecContext(ctx, `UPDATE entries SET duplicate_count=duplicate_count+1,updated_at=strftime('%Y-%m-%d %H:%M:%f','now') WHERE id=?`, existingID); err != nil {
			return nil, err
		}
		entry.ID = existingID
	} else {
		entry.ID = input.BaseID
		for counter := 2; ; counter++ {
			var exists bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entries WHERE id=? OR slug=?)`, entry.ID, entry.ID).Scan(&exists); err != nil {
				return nil, err
			}
			if !exists {
				break
			}
			entry.ID = fmt.Sprintf("%s-%d", input.BaseID, counter)
		}
		entry.Slug = entry.ID
		entry.Status = domain.StatusActive
		if err := s.saveEntryTx(ctx, tx, entry, input.Tags); err != nil {
			return nil, err
		}
		revision := 1
		if input.Hints.TopicKey != "" {
			var older string
			var previous int
			err := tx.QueryRowContext(ctx, `SELECT id,revision_count FROM entries WHERE COALESCE(project_id,'')=? AND topic_key=?
    AND title=? AND superseded_by IS NULL AND status!='archived' ORDER BY created_at DESC,id DESC LIMIT 1`, project, input.Hints.TopicKey, entry.Title).Scan(&older, &previous)
			if err == nil {
				if _, err := tx.ExecContext(ctx, `UPDATE entries SET superseded_by=?,updated_at=strftime('%Y-%m-%d %H:%M:%f','now') WHERE id=?`, entry.ID, older); err != nil {
					return nil, err
				}
				revision = previous + 1
			} else if !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE entries SET normalized_hash=?,topic_key=NULLIF(?,''),revision_count=?,pinned=?,review_after=NULLIF(?,''),expires_at=NULLIF(?,'') WHERE id=?`, input.Hints.NormalizedHash, input.Hints.TopicKey, revision, input.Hints.Pinned, input.Hints.ReviewAfter, input.Hints.ExpiresAt, entry.ID); err != nil {
			return nil, err
		}
	}
	stored, err := readEntry(ctx, tx, entry.ID, false)
	if err != nil {
		return nil, err
	}
	result := &MemoryWriteResult{Entry: stored, Deduplicated: deduplicated, TopicKey: input.Hints.TopicKey}
	if input.OperationID != "" {
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO memory_save_operations(operation_id,fingerprint,entry_id,result_json) VALUES(?,?,?,?)`, input.OperationID, fingerprint, entry.ID, string(encoded)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

var _ AtomicMemoryStore = (*sqliteEntryStore)(nil)

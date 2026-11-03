package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Memory write-path mechanics: dedupe, supersession and lifecycle hints.
//
// They live beside the entry store rather than inside Save because they are
// memory-specific: a skill or a reference does not supersede itself, but an
// observation about a topic does. Keeping them here also keeps every lookup on
// an index instead of a full scan of the project.

// SetMemoryHints records the dedupe hash, the topic and the lifecycle fields for
// one entry. Safe to call with empty values: it only writes what it is given.
func (s *sqliteEntryStore) SetMemoryHints(ctx context.Context, id string, hints MemoryHints) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE entries
		   SET normalized_hash = COALESCE(NULLIF(?, ''), normalized_hash),
		       topic_key       = COALESCE(NULLIF(?, ''), topic_key),
		       pinned          = ?,
		       review_after    = COALESCE(NULLIF(?, ''), review_after),
		       expires_at      = COALESCE(NULLIF(?, ''), expires_at),
		       updated_at      = strftime('%Y-%m-%d %H:%M:%f', 'now')
		 WHERE id = ?`,
		hints.NormalizedHash, hints.TopicKey, hints.Pinned, hints.ReviewAfter, hints.ExpiresAt, id)
	if err != nil {
		return fmt.Errorf("set memory hints for %s: %w", id, err)
	}
	return nil
}

// FindByHash returns the live entry that already holds this content hash inside
// the project. One indexed lookup replaces scanning every entry on every save.
func (s *sqliteEntryStore) FindByHash(ctx context.Context, projectID *string, hash string) (string, bool, error) {
	if hash == "" {
		return "", false, nil
	}
	return s.findLiveMemory(ctx, "normalized_hash", hash, projectID)
}

// FindByTopic returns the live entry that owns this exact fact inside the
// project, so a newer save can supersede it.
//
// Identity is topic_key PLUS title, not topic_key alone: a topic groups related
// observations ("storage/sqlite" holds several distinct facts), so superseding
// on the topic would silently delete unrelated memories. Same topic and same
// title means the same fact, restated.
func (s *sqliteEntryStore) FindByTopic(ctx context.Context, projectID *string, topicKey, title string) (string, bool, error) {
	if topicKey == "" || title == "" {
		return "", false, nil
	}
	project := ""
	if projectID != nil {
		project = *projectID
	}
	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM entries
		 WHERE COALESCE(project_id, '') = ?
		   AND topic_key = ?
		   AND title = ?
		   AND superseded_by IS NULL
		   AND status != 'archived'
		 ORDER BY created_at DESC
		 LIMIT 1`, project, topicKey, title).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find live memory by topic: %w", err)
	}
	return id, true, nil
}

func (s *sqliteEntryStore) findLiveMemory(ctx context.Context, column, value string, projectID *string) (string, bool, error) {
	project := ""
	if projectID != nil {
		project = *projectID
	}
	// column is a package-internal constant, never user input.
	query := fmt.Sprintf(`
		SELECT id FROM entries
		 WHERE COALESCE(project_id, '') = ?
		   AND %s = ?
		   AND superseded_by IS NULL
		   AND status != 'archived'
		 ORDER BY created_at DESC
		 LIMIT 1`, column)

	var id string
	err := s.db.QueryRowContext(ctx, query, project, value).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("find live memory by %s: %w", column, err)
	}
	return id, true, nil
}

// Supersede marks olderID as replaced by newerID and returns the revision the
// newer entry now holds. A superseded entry leaves search results but is never
// deleted: the history is the point.
func (s *sqliteEntryStore) Supersede(ctx context.Context, olderID, newerID string) (int, error) {
	if olderID == "" || newerID == "" || olderID == newerID {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("supersede: begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var previous int
	err = tx.QueryRowContext(ctx, `SELECT revision_count FROM entries WHERE id = ?`, olderID).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("supersede: read revision: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE entries
		   SET superseded_by = ?,
		       updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now')
		 WHERE id = ?`, newerID, olderID); err != nil {
		return 0, fmt.Errorf("supersede: mark older: %w", err)
	}

	next := previous + 1
	if _, err := tx.ExecContext(ctx,
		`UPDATE entries SET revision_count = ? WHERE id = ?`, next, newerID); err != nil {
		return 0, fmt.Errorf("supersede: bump revision: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("supersede: commit: %w", err)
	}
	return next, nil
}

// BumpDuplicate records that the same content was offered again. Dedupe that
// silently returns the existing row hides how often a fact is re-learned, which
// is itself signal.
func (s *sqliteEntryStore) BumpDuplicate(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE entries
		   SET duplicate_count = duplicate_count + 1,
		       updated_at = strftime('%Y-%m-%d %H:%M:%f', 'now')
		 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("bump duplicate for %s: %w", id, err)
	}
	return nil
}

// MemoryHints carries the memory-only fields written alongside an entry.
type MemoryHints struct {
	NormalizedHash string
	TopicKey       string
	Pinned         bool
	ReviewAfter    string
	ExpiresAt      string
}

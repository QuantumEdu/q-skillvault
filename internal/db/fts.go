package db

import (
	"context"
	"fmt"
)

// RebuildFTS recreates the full-text index from the entries table.
//
// entries_fts is a regular FTS5 table: it owns its own content. That makes the
// FTS5 'rebuild' command a silent no-op here — it returns success and changes
// nothing, because 'rebuild' only applies to external-content or contentless
// tables. The previous implementation used exactly that command, so a drifted
// index could never be repaired and callers had no way to notice.
//
// The only correct rebuild for this schema is to drop every row and re-insert it
// from entries, inside one transaction.
func (s *sqliteSearchStore) RebuildFTS(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("rebuild fts: begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, "DELETE FROM entries_fts"); err != nil {
		return fmt.Errorf("rebuild fts: clear index: %w", err)
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, COALESCE(title, ''), COALESCE(summary, ''),
		       COALESCE(body_optional, ''), COALESCE(tags_denorm, ''),
		       COALESCE(external_ref, '')
		FROM entries`)
	if err != nil {
		return fmt.Errorf("rebuild fts: read entries: %w", err)
	}

	type ftsRow struct {
		id      string
		title   string
		summary string
		body    string
		tags    string
		ref     string
	}
	var batch []ftsRow
	for rows.Next() {
		var r ftsRow
		if err := rows.Scan(&r.id, &r.title, &r.summary, &r.body, &r.tags, &r.ref); err != nil {
			_ = rows.Close()
			return fmt.Errorf("rebuild fts: scan entry: %w", err)
		}
		batch = append(batch, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("rebuild fts: iterate entries: %w", err)
	}
	_ = rows.Close()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO entries_fts (id, title, summary, body_optional, tags_denorm, external_ref)
		 VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("rebuild fts: prepare insert: %w", err)
	}
	defer func() {
		_ = stmt.Close()
	}()

	for _, r := range batch {
		if _, err := stmt.ExecContext(ctx, r.id, r.title, r.summary, r.body, r.tags, r.ref); err != nil {
			return fmt.Errorf("rebuild fts: index %s: %w", r.id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("rebuild fts: commit: %w", err)
	}
	return nil
}

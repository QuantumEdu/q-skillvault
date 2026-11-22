package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/quantum-6/skillvault/internal/domain"
)

// CheckpointReader is optional so existing EntryStore implementations remain compatible.
type CheckpointReader interface {
	LatestCheckpoint(context.Context, string, string, string) (*domain.EntryResult, error)
}

// LatestCheckpoint never falls back to a global or another project's handoff.
// Identity tags use digests, so separators in caller IDs cannot broaden a query.
func (s *sqliteEntryStore) LatestCheckpoint(ctx context.Context, project, sessionTag, agentTag string) (*domain.EntryResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT e.id FROM entries e
 WHERE e.project_id=? AND e.type='handoff' AND e.status='active' AND e.superseded_by IS NULL
 AND EXISTS(SELECT 1 FROM entry_tags t WHERE t.entry_id=e.id AND t.tag='session-checkpoint:v1')
 AND EXISTS(SELECT 1 FROM entry_tags t WHERE t.entry_id=e.id AND t.tag=?)
 AND EXISTS(SELECT 1 FROM entry_tags t WHERE t.entry_id=e.id AND t.tag=?)
 ORDER BY e.revision_count DESC,e.created_at DESC,e.id DESC LIMIT 1`, project, sessionTag, agentTag).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find session checkpoint: %w", err)
	}
	result, err := readEntry(ctx, tx, id, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &result, nil
}

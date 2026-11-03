package db

import (
	"context"
	"fmt"
)

// SearchIndexDrift reports how far the full-text index is from the entries table.
type SearchIndexDrift struct {
	Entries int
	Indexed int
}

// Stale reports whether the index no longer covers every entry.
func (d SearchIndexDrift) Stale() bool { return d.Entries != d.Indexed }

// SearchIndexStatus counts entries and indexed rows. Two integer counts, so the
// check costs nothing next to opening the vault.
func (s *Store) SearchIndexStatus(ctx context.Context) (SearchIndexDrift, error) {
	var drift SearchIndexDrift
	err := s.db.QueryRowContext(ctx, `
		SELECT (SELECT COUNT(*) FROM entries), (SELECT COUNT(*) FROM entries_fts)
	`).Scan(&drift.Entries, &drift.Indexed)
	if err != nil {
		return SearchIndexDrift{}, fmt.Errorf("search index status: %w", err)
	}
	return drift, nil
}

// EnsureSearchIndex repairs the full-text index when it has drifted from entries.
//
// Drift is silent by nature: a missing row means a memory that exists but can
// never be found, with no error anywhere. Because the index is derived data, the
// repair is always safe — entries are never modified.
//
// Returns repaired, the counts before and after, and any error.
func (s *Store) EnsureSearchIndex(ctx context.Context) (bool, int, int, error) {
	before, err := s.SearchIndexStatus(ctx)
	if err != nil {
		return false, 0, 0, err
	}
	if !before.Stale() {
		return false, before.Indexed, before.Indexed, nil
	}
	if err := s.Search.RebuildFTS(ctx); err != nil {
		return false, before.Indexed, before.Indexed, err
	}
	after, err := s.SearchIndexStatus(ctx)
	if err != nil {
		return true, before.Indexed, 0, err
	}
	return true, before.Indexed, after.Indexed, nil
}

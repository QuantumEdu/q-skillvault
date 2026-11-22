package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/quantum-6/skillvault/internal/domain"
)

func portableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func (s *sqliteImportExportStore) exportMemoryQuality(ctx context.Context) (map[string]domain.MemoryQualityRecord, error) {
	rows, err := s.query(ctx, `SELECT id, COALESCE(normalized_hash,''), COALESCE(topic_key,''), revision_count,
 duplicate_count, COALESCE(superseded_by,''), pinned, COALESCE(review_after,''), COALESCE(expires_at,'') FROM entries ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("export memory quality: %w", err)
	}
	defer rows.Close()
	records := make(map[string]domain.MemoryQualityRecord)
	for rows.Next() {
		var id string
		var record domain.MemoryQualityRecord
		if err := rows.Scan(&id, &record.NormalizedHash, &record.TopicKey, &record.RevisionCount,
			&record.DuplicateCount, &record.SupersededBy, &record.Pinned, &record.ReviewAfter, &record.ExpiresAt); err != nil {
			return nil, err
		}
		records[id] = record
	}
	return records, rows.Err()
}

func (s *sqliteImportExportStore) exportEntryVersions(ctx context.Context) ([]domain.EntryVersion, error) {
	rows, err := s.query(ctx, `SELECT version_id, entry_id, version_number, title, summary, body_optional,
 COALESCE(saved_at,'') FROM entry_versions ORDER BY entry_id, version_number`)
	if err != nil {
		return nil, fmt.Errorf("export entry versions: %w", err)
	}
	defer rows.Close()
	records := make([]domain.EntryVersion, 0)
	for rows.Next() {
		var v domain.EntryVersion
		if err := rows.Scan(&v.VersionID, &v.EntryID, &v.VersionNumber, &v.Title, &v.Summary, &v.BodyOptional, &v.SavedAt); err != nil {
			return nil, err
		}
		records = append(records, v)
	}
	return records, rows.Err()
}

func validateMemorySnapshot(data domain.VaultExport) error {
	if data.SchemaVersion < 4 && (len(data.Data.MemoryQuality) > 0 || len(data.Data.EntryVersions) > 0) {
		return fmt.Errorf("memory quality and versions require export schema v4")
	}
	entries := make(map[string]bool, len(data.Data.Entries))
	for _, e := range data.Data.Entries {
		if entries[e.ID] {
			return fmt.Errorf("duplicate imported entry %q", e.ID)
		}
		entries[e.ID] = true
	}
	for id, record := range data.Data.MemoryQuality {
		if !entries[id] {
			return fmt.Errorf("memory quality references entry %q outside snapshot", id)
		}
		if record.RevisionCount < 1 || record.DuplicateCount < 0 {
			return fmt.Errorf("invalid memory counters for %q", id)
		}
		if record.SupersededBy != "" && !entries[record.SupersededBy] {
			return fmt.Errorf("superseded target %q outside snapshot", record.SupersededBy)
		}
	}
	// Three-color traversal validates long chains in O(entries), not O(chain²).
	states := make(map[string]uint8)
	for id := range data.Data.MemoryQuality {
		var path []string
		for next := id; next != "" && states[next] != 2; next = data.Data.MemoryQuality[next].SupersededBy {
			if states[next] == 1 {
				return fmt.Errorf("supersession cycle at %q", next)
			}
			states[next] = 1
			path = append(path, next)
		}
		for _, node := range path {
			states[node] = 2
		}
	}

	versions := make(map[string]bool)
	numbers := make(map[string]map[int]bool)
	for _, v := range data.Data.EntryVersions {
		if !entries[v.EntryID] || v.VersionID == "" || v.VersionNumber < 1 {
			return fmt.Errorf("invalid imported version %q", v.VersionID)
		}
		if numbers[v.EntryID] == nil {
			numbers[v.EntryID] = make(map[int]bool)
		}
		if versions[v.VersionID] || numbers[v.EntryID][v.VersionNumber] {
			return fmt.Errorf("duplicate imported version %q", v.VersionID)
		}
		versions[v.VersionID] = true
		numbers[v.EntryID][v.VersionNumber] = true
	}
	return nil
}

// Missing legacy metadata does not reset destination quality. History is
// immutable: a collision with different content fails the entire transaction.
func importMemoryState(ctx context.Context, tx *sql.Tx, data domain.VaultExport) error {
	for id, record := range data.Data.MemoryQuality {
		_, err := tx.ExecContext(ctx, `UPDATE entries SET normalized_hash=NULLIF(?,''), topic_key=NULLIF(?,''),
   revision_count=?, duplicate_count=?, superseded_by=NULLIF(?,''), pinned=?, review_after=NULLIF(?,''), expires_at=NULLIF(?,'') WHERE id=?`,
			record.NormalizedHash, record.TopicKey, record.RevisionCount, record.DuplicateCount,
			record.SupersededBy, record.Pinned, record.ReviewAfter, record.ExpiresAt, id)
		if err != nil {
			return fmt.Errorf("import memory quality %s: %w", id, err)
		}
	}
	for _, v := range data.Data.EntryVersions {
		_, err := tx.ExecContext(ctx, `INSERT INTO entry_versions (version_id,entry_id,version_number,title,summary,body_optional,saved_at)
   VALUES(?,?,?,?,?,?,?) ON CONFLICT(version_id) DO NOTHING`,
			v.VersionID, v.EntryID, v.VersionNumber, v.Title, v.Summary, v.BodyOptional, v.SavedAt)
		if err != nil {
			return fmt.Errorf("import version %s: %w", v.VersionID, err)
		}
		var stored domain.EntryVersion
		err = tx.QueryRowContext(ctx, `SELECT version_id,entry_id,version_number,title,summary,body_optional,COALESCE(saved_at,'')
   FROM entry_versions WHERE version_id=?`, v.VersionID).Scan(&stored.VersionID, &stored.EntryID, &stored.VersionNumber,
			&stored.Title, &stored.Summary, &stored.BodyOptional, &stored.SavedAt)
		if err != nil {
			return err
		}
		if stored != v {
			return fmt.Errorf("version %s conflicts with existing history", v.VersionID)
		}
	}
	return nil
}

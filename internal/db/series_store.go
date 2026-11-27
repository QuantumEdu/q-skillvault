package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/quantum-6/skillvault/internal/domain"
	"github.com/quantum-6/skillvault/internal/security"
)

func (s *sqliteSeriesStore) Save(ctx context.Context, series domain.Series) error {
	return saveSeries(ctx, s.db, series)
}

func (s *sqliteSeriesStore) SaveWithEntries(ctx context.Context, series domain.Series, entries []domain.SeriesEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveSeries(ctx, tx, series); err != nil {
		return err
	}
	if err := replaceSeriesEntries(ctx, tx, series.ID, entries); err != nil {
		return err
	}
	return tx.Commit()
}

func saveSeries(ctx context.Context, db interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, series domain.Series) error {
	if err := security.RejectSecrets(series.ID, series.Name, series.Slug, series.Description); err != nil {
		return err
	}
	if series.Status == "" {
		series.Status = domain.StatusActive
	}
	if series.Slug == "" {
		series.Slug = series.Name
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO series (id, name, slug, description, status, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			slug=excluded.slug,
			description=excluded.description,
			status=excluded.status,
			updated_at=CURRENT_TIMESTAMP
	`, series.ID, series.Name, series.Slug, series.Description, string(series.Status))
	if err != nil {
		return fmt.Errorf("save series: %w", err)
	}
	return nil
}

// Prefer exact IDs and resolve a slug only when it identifies a single eligible series.
const seriesIDByReference = `COALESCE(
	(SELECT id FROM series WHERE id = ?),
	(SELECT MIN(id) FROM series WHERE slug = ? AND (? OR status != 'archived') HAVING COUNT(*) = 1)
)`

func (s *sqliteSeriesStore) Get(ctx context.Context, id string, includeArchived bool) (domain.SeriesResult, error) {
	var result domain.SeriesResult
	var description sql.NullString
	var status string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, slug, description, status
		FROM series WHERE id = `+seriesIDByReference, id, id, true).Scan(&result.Series.ID, &result.Series.Name, &result.Series.Slug,
		&description, &status)
	if err == sql.ErrNoRows {
		return result, fmt.Errorf("series %q not found", id)
	}
	if err != nil {
		return result, fmt.Errorf("get series: %w", err)
	}

	result.Series.Status = domain.Status(status)
	if description.Valid {
		result.Series.Description = description.String
	}

	query := `SELECT se.series_id, se.entry_id, se.order_index
		FROM series_entries se WHERE se.series_id = ?`

	if !includeArchived {
		query += " AND se.order_index > 0"
	}
	query += " ORDER BY se.order_index"

	rows, err := s.db.QueryContext(ctx, query, result.Series.ID)
	if err != nil {
		return result, fmt.Errorf("get series entries: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var entry domain.SeriesEntry
		if err := rows.Scan(&entry.SeriesID, &entry.EntryID, &entry.OrderIndex); err != nil {
			return result, fmt.Errorf("scan series entry: %w", err)
		}
		result.Entries = append(result.Entries, entry)
	}

	result.TotalSteps = len(result.Entries)
	if result.Entries == nil {
		result.Entries = []domain.SeriesEntry{}
	}

	return result, nil
}

func (s *sqliteSeriesStore) Compose(ctx context.Context, seriesID string) ([]domain.Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id, e.title, e.slug, e.type, e.summary, e.body_optional, e.status, e.project_id, e.artifact_id
		FROM series_entries se
		JOIN entries e ON e.id = se.entry_id
		JOIN series s ON s.id = se.series_id
		WHERE s.id = `+seriesIDByReference+` AND s.status != 'archived' AND e.status != 'archived'
		ORDER BY se.order_index
	`, seriesID, seriesID, false)
	if err != nil {
		return nil, fmt.Errorf("compose series: %w", err)
	}
	defer rows.Close()

	var results []domain.Entry
	for rows.Next() {
		var e domain.Entry
		var projID, artID, summary, body, status sql.NullString
		if err := rows.Scan(&e.ID, &e.Title, &e.Slug, &e.Type, &summary, &body, &status, &projID, &artID); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		e.Status = domain.Status(status.String)
		if projID.Valid {
			e.ProjectID = &projID.String
		}
		if artID.Valid {
			e.ArtifactID = &artID.String
		}
		if summary.Valid {
			e.Summary = summary.String
		}
		if body.Valid {
			e.BodyOptional = body.String
		}
		results = append(results, e)
	}

	if results == nil {
		results = []domain.Entry{}
	}
	return results, nil
}

func (s *sqliteSeriesStore) List(ctx context.Context, filter domain.SeriesFilter) ([]domain.SeriesListResult, error) {
	query := "SELECT s.id, s.name, s.slug, COALESCE(s.description,''), s.status FROM series s WHERE 1=1"
	var args []interface{}

	if !filter.IncludeArchived {
		query += " AND s.status != 'archived'"
	}
	query += " ORDER BY s.name"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list series: %w", err)
	}
	defer rows.Close()

	var results []domain.SeriesListResult
	for rows.Next() {
		var r domain.SeriesListResult
		var description sql.NullString
		var status string
		if err := rows.Scan(&r.Series.ID, &r.Series.Name, &r.Series.Slug,
			&description, &status); err != nil {
			return nil, fmt.Errorf("scan series: %w", err)
		}
		r.Series.Status = domain.Status(status)
		if description.Valid {
			r.Series.Description = description.String
		}

		err = s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM series_entries WHERE series_id = ?", r.Series.ID).Scan(&r.TotalSteps)
		if err != nil {
			r.TotalSteps = 0
		}

		results = append(results, r)
	}

	if results == nil {
		results = []domain.SeriesListResult{}
	}
	return results, nil
}

func (s *sqliteSeriesStore) ReplaceSeriesEntries(ctx context.Context, seriesID string, entries []domain.SeriesEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := replaceSeriesEntries(ctx, tx, seriesID, entries); err != nil {
		return err
	}
	return tx.Commit()
}

func replaceSeriesEntries(ctx context.Context, tx *sql.Tx, seriesID string, entries []domain.SeriesEntry) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM series_entries WHERE series_id = ?", seriesID); err != nil {
		return fmt.Errorf("delete old entries: %w", err)
	}

	for i, e := range entries {
		orderIndex := i + 1
		if e.OrderIndex > 0 {
			orderIndex = e.OrderIndex
		}
		_, err := tx.ExecContext(ctx,
			"INSERT INTO series_entries (series_id, entry_id, step_num, order_index) VALUES (?, ?, ?, ?)",
			seriesID, e.EntryID, orderIndex, orderIndex,
		)
		if err != nil {
			return fmt.Errorf("insert entry %d (%s): %w", orderIndex, e.EntryID, err)
		}
	}

	return nil
}

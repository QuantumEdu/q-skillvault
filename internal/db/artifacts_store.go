package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/quantum-6/skillvault/internal/domain"
)

func (s *sqliteArtifactStore) Save(ctx context.Context, a domain.Artifact) error {
	return saveArtifact(ctx, s.db, a)
}

func saveArtifact(ctx context.Context, exec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, a domain.Artifact) error {
	tags, err := json.Marshal(a.Tags)
	if err != nil {
		return err
	}
	projectID := interface{}(nil)
	if a.ProjectID != nil {
		projectID = *a.ProjectID
	}
	sourceEntryID := interface{}(nil)
	if a.SourceEntryID != nil {
		sourceEntryID = *a.SourceEntryID
	}

	_, err = exec.ExecContext(ctx, `
		INSERT INTO artifacts (id, title, slug, type, file_path, mime_type, summary, content_hash, size_bytes, project_id, source_entry_id, tags_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title,
			slug=excluded.slug,
			type=excluded.type,
			file_path=excluded.file_path,
			mime_type=excluded.mime_type,
			summary=excluded.summary,
			content_hash=excluded.content_hash,
			size_bytes=excluded.size_bytes,
			project_id=excluded.project_id,
			source_entry_id=excluded.source_entry_id,
			tags_json=excluded.tags_json,
			updated_at=CURRENT_TIMESTAMP
	`, a.ID, a.Title, a.Slug, string(a.Type), a.FilePath, a.MimeType, a.Summary, a.ContentHash, a.SizeBytes, projectID, sourceEntryID, tags)
	if err != nil {
		return fmt.Errorf("save artifact: %w", err)
	}
	return nil
}

func (s *sqliteArtifactStore) Get(ctx context.Context, id string) (domain.Artifact, error) {
	var a domain.Artifact
	var projID, srcID, summary sql.NullString
	var tags string

	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, slug, type, file_path, mime_type, summary, content_hash, size_bytes, project_id, source_entry_id, tags_json
		FROM artifacts WHERE id = ? OR slug = ?
	`, id, id).Scan(&a.ID, &a.Title, &a.Slug, &a.Type, &a.FilePath, &a.MimeType,
		&summary, &a.ContentHash, &a.SizeBytes, &projID, &srcID, &tags)
	if err == sql.ErrNoRows {
		return a, fmt.Errorf("artifact %q not found", id)
	}
	if err != nil {
		return a, fmt.Errorf("get artifact: %w", err)
	}

	if projID.Valid {
		a.ProjectID = &projID.String
	}
	if srcID.Valid {
		a.SourceEntryID = &srcID.String
	}
	if summary.Valid {
		a.Summary = summary.String
	}
	if err := json.Unmarshal([]byte(tags), &a.Tags); err != nil {
		return a, err
	}
	return a, nil
}

func (s *sqliteArtifactStore) List(ctx context.Context, projectID *string) ([]domain.Artifact, error) {
	query := "SELECT id, title, slug, type, file_path, mime_type, COALESCE(summary,''), content_hash, size_bytes, project_id, source_entry_id, tags_json FROM artifacts WHERE 1=1"
	var args []interface{}

	if projectID != nil {
		query += " AND project_id = ?"
		args = append(args, *projectID)
	}
	query += " ORDER BY title"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	defer rows.Close()

	var results []domain.Artifact
	for rows.Next() {
		var a domain.Artifact
		var projID, srcID, summary sql.NullString
		var tags string
		if err := rows.Scan(&a.ID, &a.Title, &a.Slug, &a.Type, &a.FilePath, &a.MimeType,
			&summary, &a.ContentHash, &a.SizeBytes, &projID, &srcID, &tags); err != nil {
			return nil, fmt.Errorf("scan artifact: %w", err)
		}
		if projID.Valid {
			a.ProjectID = &projID.String
		}
		if srcID.Valid {
			a.SourceEntryID = &srcID.String
		}
		if summary.Valid {
			a.Summary = summary.String
		}
		if err := json.Unmarshal([]byte(tags), &a.Tags); err != nil {
			return nil, err
		}
		results = append(results, a)
	}

	if results == nil {
		results = []domain.Artifact{}
	}
	return results, rows.Err()
}

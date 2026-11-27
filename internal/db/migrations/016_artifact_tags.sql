ALTER TABLE artifacts ADD COLUMN tags_json TEXT NOT NULL DEFAULT '[]';
INSERT INTO schema_migrations(version, name) VALUES (16, 'artifact_tags');

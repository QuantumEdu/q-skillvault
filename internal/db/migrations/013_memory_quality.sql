-- 013: memory write-path quality.
--
-- Three mechanics that make memory more than a pile of notes, each backed by an
-- index so the write path stays O(1) instead of scanning every entry in the
-- project on every save:
--
--   normalized_hash  identical content in the same project is ONE memory
--   topic_key        a topic that evolves supersedes its previous revision
--   lifecycle        review_after / expires_at / pinned
--
-- The backfill recovers the values from the conventions the memory path already
-- writes (external_ref = "hash:<sha256>" and the "topic_key:<key>" tag), so
-- existing observations get the new mechanics without a re-save.

ALTER TABLE entries ADD COLUMN normalized_hash TEXT;
ALTER TABLE entries ADD COLUMN topic_key TEXT;
ALTER TABLE entries ADD COLUMN revision_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE entries ADD COLUMN duplicate_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE entries ADD COLUMN superseded_by TEXT;
ALTER TABLE entries ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0;
ALTER TABLE entries ADD COLUMN review_after TEXT;
ALTER TABLE entries ADD COLUMN expires_at TEXT;

UPDATE entries
   SET normalized_hash = substr(external_ref, 6)
 WHERE external_ref LIKE 'hash:%'
   AND (normalized_hash IS NULL OR normalized_hash = '');

UPDATE entries
   SET topic_key = (
       SELECT substr(et.tag, 11)
         FROM entry_tags et
        WHERE et.entry_id = entries.id
          AND et.tag LIKE 'topic_key:%'
        LIMIT 1
   )
 WHERE topic_key IS NULL
   AND EXISTS (
       SELECT 1
         FROM entry_tags et
        WHERE et.entry_id = entries.id
          AND et.tag LIKE 'topic_key:%'
   );

-- COALESCE(project_id, '') because SQLite treats NULLs as distinct in a unique
-- index, which would silently let global memories escape deduplication.
CREATE INDEX IF NOT EXISTS idx_entries_hash
    ON entries (COALESCE(project_id, ''), normalized_hash);

CREATE INDEX IF NOT EXISTS idx_entries_topic
    ON entries (COALESCE(project_id, ''), topic_key);

CREATE INDEX IF NOT EXISTS idx_entries_superseded
    ON entries (superseded_by);

INSERT OR IGNORE INTO schema_migrations (version, name) VALUES (13, 'memory_quality');

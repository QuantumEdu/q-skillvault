-- A local retry journal, not a second memory system or a sync identity.
-- Keep tombstones when an entry is physically removed; never reuse that operation.
CREATE TABLE memory_save_operations (
    operation_id TEXT PRIMARY KEY,
    fingerprint TEXT NOT NULL,
    entry_id TEXT REFERENCES entries(id) ON DELETE SET NULL,
    result_json TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%d %H:%M:%f','now'))
);
CREATE INDEX idx_memory_operations_entry ON memory_save_operations(entry_id);
INSERT INTO schema_migrations(version,name) VALUES(15,'memory_operations');
-- Erase the cached content on physical deletion, even for a caller connection
-- without FK enforcement. Preserve only the retry fingerprint/tombstone.
CREATE TRIGGER memory_operations_forget_deleted BEFORE DELETE ON entries
BEGIN
    UPDATE memory_save_operations SET entry_id=NULL, result_json='{}' WHERE entry_id=OLD.id;
END;

-- Reconcile the persisted CHECK with the public Go relation types.
-- Never edit applied migrations. Preserve inactive edges and timestamps too.
CREATE TABLE entry_links_v14 (
    from_entry_id TEXT NOT NULL REFERENCES entries(id),
    to_entry_id TEXT NOT NULL REFERENCES entries(id),
    relation_type TEXT NOT NULL CHECK(relation_type IN (
        'references','supersedes','related_to','part_of','derived_from','implements',
        'uses','extends','handoff_of','generated_from','depends_on','contradicts','cites'
    )),
    label TEXT DEFAULT '',
    active INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (from_entry_id, to_entry_id, relation_type)
);
INSERT INTO entry_links_v14 SELECT from_entry_id, to_entry_id, relation_type, label, active, created_at FROM entry_links;
DROP TABLE entry_links;
ALTER TABLE entry_links_v14 RENAME TO entry_links;
CREATE INDEX idx_entry_links_from ON entry_links(from_entry_id);
CREATE INDEX idx_entry_links_to ON entry_links(to_entry_id);
CREATE INDEX idx_entry_links_active ON entry_links(active) WHERE active = 1;
INSERT INTO schema_migrations(version,name) VALUES(14,'semantic_relations');

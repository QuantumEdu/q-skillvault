# Search index integrity

The full-text index is derived data. Entries are the truth; `entries_fts` is a
projection of them. A row that exists in `entries` but not in `entries_fts` is a
memory that can never be found, and no command reports an error for it.

## The invariant

```
COUNT(*) FROM entries  ==  COUNT(*) FROM entries_fts
```

Any command that leaves the vault in a state where this equality does not hold has
silently deleted your ability to find something you saved.

## Why `rebuild` is not a repair here

`entries_fts` is a **regular** FTS5 table: it owns its content (note the
`entries_fts_content` and `entries_fts_docsize` shadow tables). The FTS5
`'rebuild'` command only applies to `content=` external-content tables and to
contentless tables. On a regular table it **succeeds and does nothing**:

```sql
-- returns OK, changes no rows
INSERT INTO entries_fts(entries_fts) VALUES('rebuild');
```

This is why the previous implementation of `RebuildFTS` never repaired anything, and
why the failure was invisible: the function returned `nil`.

## How the vault stays consistent

`openVault` (`internal/cli/deps.go`) calls `Store.EnsureSearchIndex` on every command:

1. `SearchIndexStatus` reads the two counts.
2. If they differ, `Search.RebuildFTS` clears the index and re-inserts every row from
   `entries` in one transaction.
3. The repair is announced on stderr:
   `[sk-vault] search index repaired: 24 -> 432 indexed entries`

A healthy index is never touched, so the steady-state cost is two `COUNT(*)` calls.

## Verify it yourself

```bash
sqlite3 ~/.skillvault/vault.db \
  "SELECT (SELECT COUNT(*) FROM entries) AS entries,
          (SELECT COUNT(*) FROM entries_fts) AS indexed"
```

If `entries > indexed`, run any command; the next open repairs it.

## What is still open

The repair is reactive. The structural fix is converting `entries_fts` to an
external-content table with `AFTER INSERT/UPDATE/DELETE` triggers on `entries`, which
makes drift impossible instead of improbable, and makes `'rebuild'` meaningful again.
Until then, `EnsureSearchIndex` is the safety net, and it is not a substitute for it.

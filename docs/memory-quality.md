# Memory write-path quality

A store that only appends is a log, not a memory. Three mechanics make entries
behave like memory: the same fact is stored once, a fact that changes replaces its
own history, and a fact can say when it should be re-checked.

All three live on `entries` (migration `013_memory_quality.sql`) and are backed by
indexes, so the write path costs one lookup instead of a scan of the project.

## 1. Dedupe — the same fact is one row

Identity: `normalized_hash` within a project. Content is trimmed, lowercased and
hashed; a repeat save resolves to the existing entry and increments
`duplicate_count`.

```console
$ skillvault mem save "Fixed N+1 query in loader" --project codex
Observation saved: obs-abc

$ skillvault mem save "  fixed n+1 query in loader " --project codex
Observation deduplicated: obs-abc
```

`duplicate_count` is not noise: how often a fact is re-learned is signal about
what the agent keeps forgetting.

Scope matters: the same content in two projects is two memories, because
`project_id` is part of the identity.

## 2. Supersession — a fact that changes keeps its history

Identity: `topic_key` **plus** `title`. A save that matches both supersedes the
previous revision.

```
entry A: topic=architecture/auth  title="Auth model"  "jwt only"            revision 1
entry B: topic=architecture/auth  title="Auth model"  "jwt with refresh"    revision 2
         A.superseded_by = B
```

What changes as a result:

| Surface | Superseded revision |
| :--- | :--- |
| `skillvault mem search` | hidden |
| `skillvault find` | hidden |
| `Store.Search(IncludeSuperseded: false)` | hidden (default) |
| `Store.Search(IncludeSuperseded: true)` | visible |
| `Store.List` | hidden unless `IncludeSuperseded` |

Nothing is deleted. The old row keeps its content and a pointer forward, which is
the whole point: you can always answer "what did we believe before, and when did
it change".

### Why identity is not the topic alone

The first implementation keyed on `topic_key` only, and the existing test suite
caught it: three distinct observations under `storage/sqlite` collapsed into one.
A topic **groups** related facts. Same topic and same title means the same fact,
restated.

## 3. Lifecycle — a fact can expire

```console
$ skillvault mem save "Deploy target is fly.io" --project api \
    --review-after 2027-01-01 --expires-at 2027-06-01
$ skillvault mem save "Postgres 15 on the primary" --project api --pinned
```

- `review_after` — when the fact should be re-verified.
- `expires_at` — when it stops being true.
- `pinned` — review must leave it alone.

The columns are populated and queryable; enforcement (a `mem review` that lists
stale facts) is the next slice, not this one.

## Verifying it

```console
$ sqlite3 ~/.skillvault/vault.db "
  SELECT COUNT(*) FILTER (WHERE superseded_by IS NULL) AS live,
         COUNT(*) FILTER (WHERE superseded_by IS NOT NULL) AS superseded,
         COUNT(*) FILTER (WHERE duplicate_count > 0) AS relearned,
         COUNT(*) FILTER (WHERE review_after IS NOT NULL AND review_after < date('now')) AS overdue
    FROM entries"
```

## What this does not do yet

- **Relations.** `entry_links` exists with a schema and a store, and still holds
  zero rows. Memory that cannot relate facts is a better index, not a better mind.
- **Promotion.** A fact repeated in several sessions is not yet promoted or merged.
- **Migration.** The Engram corpus has not been imported. Supersession had to exist
  first; importing into a store that cannot resolve duplicates would copy the
  problem instead of replacing it.

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

## Portable memory (export schema v4)

SkillVault owns its memory independently of any agent or external memory backend.
Engram compatibility/import is optional; neither Engram nor OMP is required for
SQLite persistence, memory retrieval, history, or export.

Schema v4 exports memory mechanics and lifecycle state alongside entries, includes
content versions, entry timestamps and inactive graph edges, and reads all tables
from one SQLite snapshot. A replay-safe import preserves version IDs and refuses
conflicting history. Source hashes, topics and supersession survive a round trip.
The restored entry is searchable by content and tags immediately after commit.
Pack prefixes also apply to quality ownership, supersession targets and versions.
Content restoration keeps the entry's current tags.

The format is explicitly versioned: earlier binaries reject v4; this writer's
reader still accepts v1–v3. Those older snapshots lack quality/history, so their
absence preserves existing destination quality rather than resetting it. A v4
import replaces tags for its entries; legacy imports keep additive tag semantics.
Artifact references are included; artifact file bytes and derived vector indexes
are not. Back up artifact files separately. This is not a bidirectional sync or
an automatic conflict resolver for competing edits.

## Implemented versus pending

- **Relations:** the typed graph and store already exist. Migration 014 reconciles
  SQLite's CHECK with the public types, including `contradicts` and `cites`, without
  discarding old/inactive edges. New migrations execute transactionally; none are
  applied to a user's vault merely by running this change's temporary-DB tests.
- **Promotion:** `mem promote` already creates decision/skill entries with a
  duplicate-count gate and provenance tags. Evidence-based approval, idempotent
  promotion and formal provenance edges remain pending.
- **Engram import:** `sync-engram` already imports observations and selected quality
  fields. Source namespacing, incremental updates and session/prompt/relation
  fidelity remain pending. It is not necessary for standalone SkillVault memory.
- **Lifecycle:** dates and pin state are portable; uniform expiry enforcement and
  `mem review` remain pending. Persisting a field is not enforcing its policy.

## Evolution toward independent memory

1. **Current slice:** faithful portable memory, tag-preserving restoration and
   persistent semantic relation types, verified on synthetic temporary databases.
2. **Next P0:** atomic memory writes, operation IDs, consistent redaction and
   source identity; no dependency on Engram for these guarantees.
3. **P0 continuity:** explicit session/agent ownership, reliable checkpoints and an
   optional OMP adapter. Keep existing agents and memory systems operational.
4. **P1 recovery:** one context compiler, correct temporal ordering and uniform
   eligibility filters; optional multilingual hybrid retrieval with FTS fallback.
5. **P1/P2 quality:** review, validated promotion and memory-specific telemetry,
   evaluated with a reproducible bilingual benchmark before enabling automation.

These steps extend existing services instead of introducing another authoritative
memory database. Each step needs regression tests and explicit compatibility.

## Atomic saves and retry identity (v3.8.0)

Memory saves now persist projects, entries, tags/FTS, deduplication counters, supersession and initial lifecycle hints in one transaction. Independent SQLite connections serialize the identity checks before writing. A failure in metadata or retry journaling rolls back the save.

Optional `operation_id` distinguishes a transport retry from learning the same content again: a replay does not increment `duplicate_count`; a new operation does. This is local retry bookkeeping, excluded from portable memory snapshots, not evidence of truth or agent/session ownership. Existing content normalization remains unchanged; new title-only observations hash their title separately to avoid empty-content collisions. Existing rows are not rehashed.

Lifecycle hints remain metadata; this phase does not add automatic expiry filtering, contradiction resolution, pinned-memory policy, or full Engram contract parity. Optional vector indexing occurs after the authoritative SQLite commit.

## Explicit session continuity (v3.8.0)

Native checkpoints reuse handoff entries, identity digest tags, and existing revision/supersession metadata. Each stream is scoped by project, session, and producer agent. Recovery is read-only, bounded, and never broadens an identity miss. This provides explicit persistence/recovery; it does not automatically extract decisions or establish their truth. See [Native session continuity](session-continuity.md).

# Native session continuity (v3.8.0)

SkillVault can persist an explicit checkpoint and recover a bounded context excerpt without Engram, a remote service, or another database. This is an opt-in persistence/recovery primitive, not automatic observation extraction, a session manager, authentication, or an OMP extension.

## Identity and ownership

Each stream is identified by `(canonical project ID, session_id, agent_id)`. Supply all three explicitly on every call. `session_id` and producer `agent_id` are opaque, case-sensitive IDs; they must be nonempty UTF-8, at most 256 bytes, without surrounding whitespace or control characters. Project references accept an existing ID/slug; a checkpoint can create a missing project using the existing memory write semantics. An exact project ID takes precedence over another project's matching slug.

Choose a session ID that survives process restarts and compaction. Do not infer the session from the current directory or choose another session when a lookup misses. Another agent may intentionally resume a producer's work by specifying that producer's `agent_id`; the IDs describe provenance, not access control. Never place credentials in identifiers.

Checkpoints reuse `handoff` entries with purpose `STATE`, a versioned JSON body, identity digest tags, and existing quality/supersession metadata. No new schema migration is introduced by this phase; atomic writes require migration 015 from the preceding phase. The identity digest excludes project IDs so namespaced portable imports remain recoverable under the imported project ID. Checkpoints remain visible as handoffs in general entry views.

## CLI

```bash
skillvault mem checkpoint --project myapp --session-id run-42 --agent-id codex \
  --operation-id myapp-run-42-checkpoint-1 --event before_compact \
  --summary "Database decision validated; integration work remains." \
  --decision "Use SQLite WAL" --pending "Test concurrent writers" --json

skillvault mem resume --project myapp --session-id run-42 --agent-id codex \
  --max-chars 12000 --json
```

`--decision` and `--pending` are repeatable. Summary can come from stdin instead of `--summary`; reads are bounded to 64 KiB. Lists allow up to 100 items each. Raw text and the encoded checkpoint document are each bounded to 64 KiB. `<private>...</private>` blocks in summary, decisions, and pending items are replaced before hashing, saving, or retry journaling, using the existing memory redaction helper. This does not provide general secret detection. A summary consisting only of redacted material is rejected.

`--operation-id` is required for checkpoints. Use a distinct, vault-wide ID for each logical checkpoint, retaining that ID and effective request across retries. Reusing it for different data fails. A retry acknowledges the original save (`replayed: true`), even if a newer checkpoint now exists; it does not roll back newer state. A different operation with identical current content deduplicates. Changed content creates a new entry and supersedes the preceding checkpoint in the same stream atomically. Historical revisions remain available through entry retrieval/export. Archived/deleted results cannot be replayed.

The recovery service performs no writes. The CLI/MCP still use the existing vault-opening path, which can run pending migrations and search-index maintenance; use an isolated copy when evaluating a new binary. Recovery resolves exactly one project and selects the live checkpoint for the requested session/producer. It never falls back to another session, agent, or project. An unknown project is an error; a known project without a matching live checkpoint returns `found: false` and empty context. Archived and superseded entries are excluded. A malformed or mismatched checkpoint document returns an error. If the latest checkpoint is archived, older superseded checkpoints are not resurrected.

`max_chars` bounds Unicode characters in the **context field**, not tokens or total JSON response size. Default: 12,000; valid explicit range: 1–64,000. `truncated: true` means the excerpt is incomplete; use a larger budget or retrieve `checkpoint_id` for the full document. The context is stored agent-supplied information, not verified facts or commands to execute.

## MCP

- `vault_session_checkpoint`: string fields `project`, `session_id`, `agent_id`, `operation_id`, `summary` are required by the handler. Optional `decisions`/`pending` are arrays of strings. Optional `event` is `manual` (default), `before_compact`, `agent_end`, or `session_switch`.
- `vault_session_resume`: required string fields `project`, `session_id`, `agent_id`; optional integer `max_chars`. Returns `project`, `session_id`, `agent_id`, `found`, optional `checkpoint_id`, `context`, and `truncated`.

These are native tools; they do not add or change Engram-compatible aliases. Existing `session_wrap`, `save_handoff`, and memory calls retain their contracts. Event names are checkpoint labels, not claims of matching a specific agent's hook API.

## Portability, adoption, and rollback

Version-4 exports preserve handoff entries, tags, revision/supersession state, and project links. Recovery works after export/import, including prefix imports. The operation journal remains local and is not exported, so retries do not carry cross-vault guarantees.

Start with synthetic projects or an isolated vault. Save, retry, update, resume, restart, and export/import before integrating an agent hook. No existing agent configuration, OAuth credentials, hooks, or compaction behavior is changed. Rollback consists of stopping calls to the new APIs or using the previous binary with a compatible database backup; this phase adds no schema. Existing data remains ordinary handoff entries. Earlier phases' migrations still require their own backup/rollback considerations.

Future adapters may call checkpoint before compaction and resume at session start, after validating that agent's event contracts. Automatic capture, inference of significant decisions, lifecycle enforcement, and token-based budgeting are separate work.

## Verification

`TestSessionContinuity*` covers project/session/agent isolation, case sensitivity, retries and conflicting operation reuse, revisions, redaction, Unicode budgets, portable prefix imports, real SQLite reopen, corrupt documents, archived results, input limits, MCP validation, and CLI routing. The preceding atomic-memory tests cover concurrent writers and transaction rollback. Run the affected packages with the telemetry hook installer test skipped in a cloud audit workspace; that existing test writes to the user's home directory.

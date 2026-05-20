# Task 011: Session Management

## Why

Sessions persist agent conversations across restarts. This task implements JSONL storage, session lifecycle (create, resume, compact), auto-naming, and compaction logic. This is a parallel track after foundation (006). Compaction summarization (LLM call) belongs to SDK (013) — this task owns the pure functions only.

## Comparison Analysis: Session Management vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Storage Format | JSONL with version 3 header + typed entries | JSONL with version 1 header + typed entries |
| Session Structure | Tree with id/parentId, leafId tracking | Linear append-only (MVP), no branching |
| Entry Types | 9 entry types | 7 entry types (subset for MVP) |
| Compaction | `prepareCompaction()` + `compact()` — LLM summarization | Pure functions only — SDK handles LLM call |
| Token Estimation | `chars/4` heuristic | Per-type: text/4, tool/3, thinking/3.5 |
| Session Naming | Auto-generated names | First user message, truncated to 50 chars |
| Directory Structure | `~/.pi/agent/sessions/<encoded-cwd>/` | `~/.tau/sessions/<encoded-cwd>/` |
| CWD Encoding | `/` → `-` replacement | Same approach |

## Main Constraints

- JSONL must be append-only — one JSON object per line
- Corruption recovery: incomplete last line discarded, valid entries preserved
- Compaction pure functions must not call provider — SDK handles LLM summarization
- Auto-naming must not require LLM call — deterministic from first user message
- Compaction constrained to turn boundaries — never split tool call from result

## Dependencies

- `internal/types/` (Task 006)
- `internal/testutil/` (Task 006)

## Subtasks

- [x] **011.1** — `internal/session/storage.go` — JSONL read/write, session header, entry types
- [x] **011.2** — `internal/session/session.go` — Session lifecycle (create, resume, persist, delete)
- [x] **011.2a** — `internal/session/migrate.go` — Version migration scaffolding: header version validation, `migrateV1ToV2()` stub
- [x] **011.3** — `internal/session/naming.go` — Auto-naming strategy
- [x] **011.4** — `internal/session/compaction.go` — Compaction logic (pure functions)
- [x] **011.5** — Unit tests for storage, resume, naming, compaction

## Acceptance Criteria

- [x] JSONL append-only storage with session header (version 1)
- [x] All 7 entry types supported (message, model_change, thinking_level_change, compaction, custom_entry, custom_message, session_info)
- [x] Session resume algorithm rebuilds message list from JSONL (ARCHITECTURE.md §7.2a)
- [x] Corruption recovery: incomplete last line discarded, valid entries preserved
- [x] Auto-naming from first user message (truncated to 50 chars, special characters stripped)
- [x] Compaction logic as pure functions: token estimation, cut point finding, turn boundary constraints
- [x] Per-type token estimation: text/4, tool/3, thinking/3.5
- [x] Compaction constrained to turn boundaries (never split tool call from result)
- [x] Structured summary format in compaction entries
- [x] Compaction does NOT call provider — summarization trigger/integration belongs to SDK (013)
- [x] Session file encoding: `<timestamp>_<8-char-hex>.jsonl`
- [x] Cumulative usage accumulator (data structure): session tracks total tokens and cost across all turns — SDK (013) exposes via `Usage()`
- [x] Session directory path passed via constructor parameter from SDK — session/ does NOT import config/
- [x] Unit tests for storage, resume, naming, compaction (use `testutil/` helpers)
- [x] No internal dependencies except `types` and `testutil`

## Testing & Verification Strategy

**Unit tests** (temp filesystem via `t.TempDir()`):
- Storage: append single entry → read back → verify round-trip; append multiple entries → verify order preserved
- Corruption: write valid entries + truncate mid-line → verify resume discards incomplete line, recovers valid entries
- Resume: create JSONL with mixed entry types → verify message list rebuilt correctly, model_change entries applied
- Naming: "Hello world" → "hello-world"; 60-char message → truncated to 50; special chars stripped
- Token estimation: text "abcd" (4 chars) → 1 token; tool result (3 chars) → 1 token; thinking (3.5 chars) → 1 token
- Compaction: transcript exceeding budget → verify cut point at turn boundary (not mid-tool-call)
- Delete: create session file → delete → verify file removed

**Integration tests**:
- Full lifecycle: create session → append 10 messages → close → reopen → resume → verify all messages present
- Usage tracking: append entries with Usage data → verify cumulative total correct

**Quality gates**:
- Compaction pure functions have zero side effects (no I/O, no provider calls)
- `go test ./internal/session/...` — all pass
- Session package imports only `types`, `testutil`, and stdlib

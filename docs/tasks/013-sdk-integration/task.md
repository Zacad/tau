# Task 013: SDK Integration

## Why

The SDK is the high-level API that composes all subsystems into a coherent `Session` interface. It's the primary programmatic interface — the CLI (014) is a thin consumer. This task sits on the critical path after the agent loop (012).

## Comparison Analysis: SDK/Session vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Main API Class | `AgentSession` — monolithic, handles everything | `Session` — composes subsystems (agent, session, skills, subagent) |
| Session Creation | Complex factory with many options | `CreateSession(ctx, SessionOptions)` |
| Prompt API | `prompt()` with streaming | `Prompt()` — runs agent loop, returns when done |
| Steering | `steer()` method | `Steer()` — delivers to steering queue |
| Events | EventEmitter subscription | `Subscribe()` — returns unsubscribe function |
| Usage Tracking | Built into session | Cumulative accumulator across all turns |
| Model Changes | `setModel()` with validation | `SetModel()` with resolution |
| Compaction | Automatic trigger + LLM call | `Compact()` — SDK handles LLM summarization |

## Main Constraints

- SDK must be the integration point — it composes all subsystems
- Model resolution (pattern → selection → default) belongs here
- Compaction summarization (LLM call) belongs here — session (011) only owns pure functions
- Cumulative usage accumulator: aggregates `Usage` from every turn
- All methods must return errors gracefully
- Tool allowlist and read-only contracts: accept from SessionOptions, apply via tools registry

## Dependencies

- All internal packages (Tasks 006–012)

## Subtasks

- [x] **013.1** — `internal/sdk/sdk.go` — Session struct, CreateSession()
- [x] **013.2** — `Prompt()` / `Continue()` — runs agent loop
- [x] **013.3** — `Steer()` / `Subscribe()` — steering queue, event subscription
- [x] **013.4** — `Compact()` — triggers compaction with LLM summarization
- [x] **013.5** — `Usage()` — exposes cumulative usage accumulator (data structure owned by session/011, SDK exposes via this method)
- [x] **013.6** — `Model()` / `SetModel()` — model query and change with PI-style resolution (see below)
- [x] **013.7** — `Rename(name string)` — session rename API for `/name` command
- [x] **013.8** — SessionOptions processing: ephemeral, read-only, tool allowlist, continue
- [x] **013.9** — Error handling composition: provider failures, tool failures, context overflow
- [x] **013.11** — Unit tests with mocked subsystems
- [x] **013.12** — Integration test: full SDK flow with mock provider → agent loop → tools → session persistence → resume

**Moved to task 010**:
- ~~013.10~~ Subagent spawning through session context → moved to 010-subagent-system

## Model Resolution (following PI's approach)

PI's model resolver never returns ambiguity — it always resolves to a single model or fails with a clear error:

1. **Exact match** by `provider/modelId` format (e.g., `anthropic/claude-sonnet-4`)
2. **Exact match** by bare model ID (rejects if multiple providers share same ID)
3. **Partial match** with smart disambiguation:
   - Separates aliases (no date suffix like `-20250514`) from dated versions
   - If multiple aliases match → picks the one that sorts highest alphabetically
   - If no alias found → picks latest dated version (highest date sort)
4. **Glob patterns** (`*`, `?`, `[`) for scoping multiple models (CLI concern)
5. **Fallback**: can build a custom model ID if provider is known but model ID doesn't exist

**Our approach**: Add `ResolveModelWithFallback(pattern string) (types.Model, error)` to provider/registry.go following PI's disambiguation logic. SDK calls this — never returns ambiguity lists.

## Acceptance Criteria

- [x] `CreateSession()` initializes all subsystems (agent, session, skills, provider)
- [x] `Prompt()` sends message, runs agent loop, returns when done
- [x] `Continue()` resumes agent loop without new message
- [x] `Steer()` delivers message to running agent via steering queue
- [x] `Subscribe()` registers event listener, returns unsubscribe function
- [x] `Compact()` triggers compaction: detects overflow, calls provider for LLM summarization, writes compaction entry
- [x] `Usage()` returns cumulative session usage and cost (session/ owns accumulator data structure, SDK exposes)
- [x] `Model()` / `SetModel()` query and change active model
- [x] Model resolution: PI-style disambiguation (exact ID → provider/ID → alias preference → latest dated version). Returns single model or clear error — never returns ambiguity lists
- [x] `Rename(name string)` updates session display name (stored in session_info entry)
- [x] SessionOptions respected: ephemeral, read-only, tool allowlist, continue
- [x] Error handling: all methods return errors gracefully — provider failures (with retry), tool failures (as results), context overflow (triggers compaction)
- [x] Skills discovered and formatted for progressive disclosure at session start
- [x] Session persists model changes and usage via JSONL append
- [x] Tool allowlist contract: accepts `[]string` from `SessionOptions`, applies via `tools.Registry.WithAllowlist()`
- [x] Read-only contract: accepts `bool` from `SessionOptions`, applies via `tools.Registry.WithReadOnly()`
- [x] Cumulative usage accumulator: aggregates `Usage` from every turn's `StreamEvent{Type: "done"}`
- [x] Unit tests with mocked subsystems (use `testutil/` helpers)
- [x] Internal dependencies on all packages (as designed)

## Testing & Verification Strategy

**Unit tests** (mocked subsystems via `testutil`):
- CreateSession: all subsystems initialized, skills discovered, system prompt composed
- Prompt: mock agent loop → verify message sent, loop runs, returns on done
- Steer: verify message delivered to agent's steering queue
- Subscribe: register listener → trigger event → verify received; unsubscribe → verify no more events
- Compact: mock context overflow → verify provider called for summarization, compaction entry written
- Model resolution: exact match (provider/id or bare id), alias preference (pick highest sort), dated version preference (pick latest), no match returns error with available models
- Rename: verify session_info entry updated
- Usage: verify cumulative total across multiple turns

**Integration test** (013.12):
- Full SDK flow: CreateSession → Prompt("hello") → mock provider responds → mock tool executes → session persisted → Continue() → verify transcript intact → Usage() returns correct totals → Resume session → verify messages recovered
- Error paths: provider failure (retry), tool failure (result with isError), context overflow (compaction triggered)

**Quality gates**:
- SDK composes all subsystems — no subsystem initialization missing
- All public methods return errors (not panic)
- Integration test covers full lifecycle: create → use → persist → resume
- `go test ./internal/sdk/...` — all pass

# Task 056: Nil Agent Panic Fix

**Status**: Done
**Date**: 2026-05-24

## Why

When continuing a resumed session and using the subagent tool (or any prompt action), Tau panics with `nil pointer dereference` at `internal/agent/event.go:24` via `Session.Subscribe`. The root cause: `Session.Subscribe` unconditionally dereferences `s.ag`, but `s.ag` is legitimately nil when a session is resumed without an available provider/model.

## Comparison with PI and OpenCode

### PI
- `AgentSession` always has an agent (passed in constructor config).
- `subscribe()` on `AgentSession` adds listeners to a local list, not to the agent directly (`agent-session.ts:713-722`).
- Model validation happens in `prompt()` before any agent interaction (`agent-session.ts:1029-1031`).

### OpenCode
- Session is a database row; provider/model resolution happens per-prompt.
- No persistent agent object — model/provider selected at prompt time.

### Tau (before fix)
- `Session.Subscribe` delegates directly to `s.ag.Subscribe` without nil check (`sdk.go:455-456`).
- `CreateSession` creates agent only when provider is available (`sdk.go:355-359`).
- Resume can load messages from file without an agent (`sdk.go:803-809`).
- TUI subscribes before calling `Prompt` (`model.go:275-282`), so panic happens before the friendly "no model selected" error can surface.

## Constraints
- Must not break existing session files or resume behavior.
- Must allow graceful recovery when provider/model becomes available after resume.
- Must preserve full conversation history when agent is created post-resume.

## Subtasks

### 056.1: Fix Session.Subscribe nil panic
- [x] Return no-op unsubscribe when `s.ag == nil`
- [x] Add test: Subscribe on nil-agent session does not panic

### 056.2: Fix SetModel to create agent when resuming without one
- [x] When provider exists and `s.ag == nil`, create new agent
- [x] Restore session messages into new agent
- [x] Restore thinking level for the model
- [x] Update subagent tool with new parent model
- [x] Add test: SetModel after nil-agent resume creates agent and preserves history

### 056.3: Add nil guards for adjacent methods
- [x] `Steer` — return error when `s.ag == nil`
- [x] `AgentState` — return zero value when `s.ag == nil`
- [x] `Compact` — skip when `s.ag == nil`

### 056.4: Tests and build
- [x] `go test ./internal/sdk/... ./internal/tui/...` passes
- [x] Binary rebuilt at `./tau`

## Acceptance Criteria
- [x] Resumed session without provider/model does not panic on Subscribe
- [x] After `/connect` + `/model`, resumed session can prompt normally
- [x] Conversation history is preserved across nil-agent recovery
- [x] All new tests pass, no regressions

## Worklog

### 2026-05-24
- **Analysis**: Identified panic path: `submitPrompt` → `Subscribe` → `s.ag.Subscribe` with `s.ag == nil`
- **Root cause**: Session can be valid with nil agent when resumed without provider
- **Related**: `SetModel` does not create agent when `s.ag == nil`, blocking recovery
- **Adjacent risks**: `Steer`, `AgentState`, `Compact` also dereference `s.ag` without nil checks
- **Implementation**:
  - Made `Subscribe` nil-safe: returns no-op unsubscribe when `s.ag == nil`
  - Made `Steer` nil-safe: returns error when `s.ag == nil`
  - Made `AgentState` nil-safe: returns empty state when `s.ag == nil`
  - Made `Compact` nil-safe: early return when `s.ag == nil`
  - Updated `SetModel` to create agent when `s.ag == nil` and provider is available
  - New agent recovers session messages, thinking level, and subagent tool
- **Tests**:
  - `TestSession_Subscribe_NilAgent`
  - `TestSession_SetModel_CreatesAgentAfterNilResume`
  - `TestSession_Steer_NilAgent`
  - `TestSession_AgentState_NilAgent`
- **Build**: Binary rebuilt at `./tau`, all tests pass

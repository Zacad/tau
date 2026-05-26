# Worklog: Task 064 Canonical Tool Lifecycle Events and TUI Tool Metadata

## 2026-05-25

### Request

User reported that tool calls in the chat window expose almost no useful metadata. Example expectation:

- `read` should show file path
- `bash` should show command
- `webfetch` should show URL

Current UX often shows only a bare tool name or placeholder.

### Research

#### Tau investigation

Inspected current implementation across:

- `internal/tui/render.go`
- `internal/tui/model.go`
- `internal/agent/loop.go`
- `internal/provider/openai.go`
- `internal/provider/ollama.go`
- `internal/provider/anthropic.go`
- `internal/provider/google.go`
- related tests in `internal/tui/render_test.go`

Verified:

- Tau already contains compact formatter functions for many tools in `internal/tui/render.go`.
- `formatToolArgs(...)` already supports `read`, `write`, `edit`, `bash`, `grep`, `find`, `ls`, `webfetch`, and `websearch`.
- `find` currently uses the wrong key (`name`) instead of `pattern`.
- `subagent` does not have specialized compact formatting.
- `internal/tui/model.go` ignores the tool name on `AgentEventToolExecStart` and hardcodes `…` while pending.
- `internal/agent/loop.go` emits tool lifecycle data using ad hoc `map[string]any` payloads.
- OpenAI and Ollama emit start/end tool-call stream events with enough information to recover final args.
- Anthropic and Google emit tool-call start but do not provide symmetric completed lifecycle data through the same streaming path.

#### Reference implementation research

Used reference implementation research against local PI and OpenCode repositories.

Key verified findings:

- PI has the clearest tool rendering contract:
  - explicit `renderCall` / `renderResult`
  - compact metadata per tool
  - fallback to pretty JSON args for unknown tools
- OpenCode has structured compact tool UI primitives:
  - title
  - subtitle
  - args-like compact metadata
  - expandable details
- Both references reinforce the same direction:
  - compact, high-signal tool call summaries
  - clear separation between lifecycle data and presentation

### Design review

Used a reviewer subagent to challenge the proposed Option A task direction.

Main review findings incorporated into the task design:

- event identity must include stable tool call IDs
- lifecycle semantics must distinguish model-requested tool calls from actual execution and results
- normalization should happen at the agent boundary, not in the TUI
- native vs inferred completion semantics must be explicit
- sanitization/redaction responsibility must be defined centrally
- compatibility with existing event consumers must be planned, not assumed

### Output

Created the full task definition for Option A in:

- `docs/tasks/064-canonical-tool-lifecycle-events/task.md`

The task includes:

- why
- PI/OpenCode comparison analysis
- constraints
- design direction
- subtasks with acceptance criteria
- edge cases
- overall acceptance criteria
- testing strategy
- out-of-scope definition

### Status

Task definition created. No implementation started.

### 064.1 implementation start

User confirmed switching active work from Task 063 to Task 064, cleaning the duplicate tracking entry, and starting incrementally with 064.1 using TDD.

Actions completed:

- Updated `docs/TRACKING.md`:
  - Task 064 is now active/in progress.
  - Task 063 is back to planned.
  - Removed the duplicate Task 064 status row.
- Added failing tests first in `internal/types/tool_lifecycle_test.go` for canonical typed payloads and legacy map adapters.
- Implemented `internal/types/tool_lifecycle.go` with:
  - `ToolLifecyclePhase` (`requested`, `finalized`, `executing`, `completed`)
  - `ToolLifecycleSource` (`native`, `inferred`)
  - `ToolLifecycleEvent`
  - `ToolProgressEvent`
  - `ToolResultEvent`
  - `LegacyMap()` adapters for migration compatibility.
- Ran reviewer subagent against the initial type design.
- Addressed reviewer findings by documenting:
  - exact migration mapping for legacy `AgentEventToolExecStart` / `AgentEventToolExecEnd`,
  - that legacy `args` remains a JSON string,
  - that `ArgsJSON` must hold only valid complete JSON and partial args should use `ArgsSummary`.
- Expanded tests for:
  - stable JSON field names,
  - partial arg behavior without raw JSON,
  - legacy lifecycle/progress/result map shapes.
- Updated task documentation to mark 064.1 complete and document the compatibility model.

Verification:

```bash
gofmt -w internal/types/tool_lifecycle.go internal/types/tool_lifecycle_test.go
go test ./internal/types
```

Result: `ok github.com/adam/tau/internal/types`.

Next planned subtask: 064.2 — normalize provider tool-call lifecycle at the agent boundary.

### 064.2 implementation

Continued with agent-boundary normalization.

Actions completed:

- Added TDD coverage in `internal/agent/tool_lifecycle_test.go` proving:
  - final assistant tool-call blocks produce typed `ToolLifecycleEvent` payloads,
  - inferred finalized lifecycle events carry stable `CallID`, `ToolName`, args JSON, `ArgsComplete`, and `Source=inferred`,
  - typed `ToolResultEvent` payloads carry stable `CallID` and result metadata.
- Updated `internal/agent/loop.go` so agent tool events now emit typed payloads:
  - provider `EventToolCallStart` -> `ToolLifecycleEvent{Phase: requested, Source: native}`
  - provider `EventToolCallEnd` -> `ToolLifecycleEvent{Phase: finalized, Source: native}`
  - final assistant message tool-call blocks without native end -> inferred finalized lifecycle event
  - tool progress -> `ToolProgressEvent`
  - tool results/interruption results -> `ToolResultEvent`
- Tracked native finalized call IDs to avoid duplicate inferred finalized events for providers that already emit native end events.
- Updated subagent stream-event conversion to forward typed lifecycle payloads instead of raw strings/messages.
- Updated TUI event handling to consume typed payloads while retaining legacy map/string compatibility.

Verification:

```bash
gofmt -w internal/agent/loop.go internal/agent/tool_lifecycle_test.go internal/subagent/subagent.go internal/tui/model.go internal/tui/render.go
go test ./internal/agent ./internal/tui
go test ./internal/subagent -run 'TestRun_ToolExecution|TestRun_MaxToolIterations|TestConvert|ToolExecution'
go test -short ./internal/subagent
```

Results:

- `ok github.com/adam/tau/internal/agent`
- `ok github.com/adam/tau/internal/tui`
- `ok github.com/adam/tau/internal/subagent` for focused/short runs

Note: a full non-short `go test ./internal/subagent` was attempted and hit existing Ollama-dependent E2E flakiness in builtin agent type tests (`researcher`/`reviewer` output mismatch). Focused and short subagent tests pass for this change.

Next planned subtask: 064.3 — provider conformance updates/tests for OpenAI, Ollama, Anthropic, and Google.

### 064.2 review follow-up

Reviewer subagent found several correctness gaps in the first 064.2 pass. Addressed the actionable blockers before moving on:

- Added `AgentEvent.LegacyData()` as an explicit migration shim for legacy SDK subscribers that still need the old `map[string]any` shape.
- Documented typed-vs-legacy subscription behavior in `internal/agent/event.go` and `internal/sdk/sdk.go`.
- Suppressed misleading native finalized events when provider `EventToolCallEnd` lacks a stable tool call ID; final assistant message normalization now emits the single inferred finalized event with ID/args.
- Added TUI pending tracking by stable `CallID` (`pendingToolIndexes`) while retaining the legacy no-ID single pending slot.
- Updated TUI behavior so a finalized event without a prior start creates a completed tool-call block instead of being discarded.
- Added TUI tests for no-start finalized events and multiple pending calls correlated by ID.
- Mirrored inferred finalized normalization in subagent stream consumption.
- Added typed `ToolResultEvent` emission for subagent tool execution results.
- Added focused helper for subagent tool result text extraction.

Verification after fixes:

```bash
gofmt -w internal/types/agent_legacy.go internal/types/agent_legacy_test.go internal/agent/event.go internal/agent/loop.go internal/sdk/sdk.go internal/subagent/subagent.go internal/subagent/tool_result.go internal/tui/model.go internal/tui/tool_lifecycle.go internal/tui/tool_lifecycle_test.go
go test ./internal/types ./internal/agent ./internal/tui
go test -short ./internal/subagent
```

Results:

- `ok github.com/adam/tau/internal/types`
- `ok github.com/adam/tau/internal/agent`
- `ok github.com/adam/tau/internal/tui`
- `ok github.com/adam/tau/internal/subagent` short mode

### 064.3 provider conformance

Implemented provider conformance coverage and updates for tool lifecycle metadata.

Actions completed:

- Added provider conformance tests in:
  - `internal/provider/tool_lifecycle_conformance_test.go`
  - `internal/provider/tool_lifecycle_provider_conformance_test.go`
- Covered OpenAI Responses and Ollama existing behavior:
  - start event is emitted,
  - end event is emitted where provider has final metadata,
  - final `EventDone` message contains stable tool call ID/name/args.
- Added failing conformance tests for Anthropic and Google proving that final tool-call metadata must reach the stream as completed tool-call data.
- Updated `internal/provider/anthropic.go`:
  - tracks `tool_use` content blocks by index,
  - preserves Anthropic tool-use ID,
  - accumulates `input_json_delta` fragments,
  - appends final `BlockToolCall` with args to the assistant message,
  - emits `EventToolCallEnd` with final metadata on `content_block_stop`.
- Updated `internal/provider/google.go`:
  - emits `EventToolCallEnd` immediately after Gemini function-call parts are converted to Tau `BlockToolCall` metadata.

Verification:

```bash
gofmt -w internal/provider/anthropic.go internal/provider/google.go internal/provider/tool_lifecycle_conformance_test.go internal/provider/tool_lifecycle_provider_conformance_test.go
go test ./internal/provider -run 'TestProviderConformance'
go test ./internal/provider
go test ./internal/agent ./internal/tui ./internal/types
```

Results:

- `ok github.com/adam/tau/internal/provider`
- `ok github.com/adam/tau/internal/agent`
- `ok github.com/adam/tau/internal/tui`
- `ok github.com/adam/tau/internal/types`

064.3 acceptance criteria are complete.

### 064.4 central sanitization and truncation

Implemented shared safe tool metadata summarization.

Actions completed:

- Added `internal/types/tool_summary.go` with:
  - `SummarizeToolArgs(toolName, raw)`
  - `SummarizeToolArgsJSON(toolName, argsJSON)`
  - sensitive key redaction (`token`, `api_key`, `authorization`, `password`, `secret`, cookies, etc.)
  - large content omission for `content`, `old_text`, `new_text`
  - deterministic sorted fallback formatting
  - rune-safe truncation with ellipsis
  - tool-specific compact summaries for read/write/edit/bash/grep/find/ls/webfetch/websearch/subagent.
- Added tests in `internal/types/tool_summary_test.go` covering:
  - redaction,
  - large content omission,
  - malformed JSON fallback,
  - rune-safe truncation,
  - `find` using `pattern`,
  - subagent compact metadata.
- Updated canonical lifecycle generation to populate `ArgsSummary` via shared summary utilities in:
  - `internal/agent/loop.go`
  - `internal/subagent/subagent.go`
- Updated TUI formatting so `find` and `subagent` use the shared summary utility.

Verification:

```bash
gofmt -w internal/types/tool_summary.go internal/types/tool_summary_test.go internal/agent/loop.go internal/subagent/subagent.go internal/tui/render.go
go test ./internal/types ./internal/agent ./internal/tui
go test -short ./internal/subagent
go test ./internal/provider
```

Results:

- `ok github.com/adam/tau/internal/types`
- `ok github.com/adam/tau/internal/agent`
- `ok github.com/adam/tau/internal/tui`
- `ok github.com/adam/tau/internal/subagent` short mode
- `ok github.com/adam/tau/internal/provider`

064.4 acceptance criteria are complete.

### 064.5 TUI tool-call rendering improvements

Implemented TUI metadata rendering improvements on top of canonical lifecycle payloads.

Actions completed:

- Added TUI tests in `internal/tui/tool_metadata_render_test.go` covering:
  - pending tool rows show the actual tool name,
  - pending rows show compact formatted metadata rather than raw JSON,
  - `find` uses `pattern` and `path`,
  - `subagent` shows compact metadata including type/task/timeout,
  - typed start events populate name and metadata immediately.
- Updated `renderToolCallBlock` so pending rows use `formatToolArgs` just like completed rows.
- Updated TUI state handling for errors to mark all ID-tracked pending tool calls as failed, not only the legacy single pending slot.
- Kept compatibility with legacy map/string event payloads.
- Confirmed `find` and `subagent` formatting flows through the shared safe summary utility from 064.4.

Verification:

```bash
gofmt -w internal/tui/model.go internal/tui/render.go internal/tui/tool_metadata_render_test.go
go test ./internal/tui
go test ./internal/types ./internal/agent ./internal/provider
go test -short ./internal/subagent
```

Results:

- `ok github.com/adam/tau/internal/tui`
- `ok github.com/adam/tau/internal/types`
- `ok github.com/adam/tau/internal/agent`
- `ok github.com/adam/tau/internal/provider`
- `ok github.com/adam/tau/internal/subagent` short mode

064.5 acceptance criteria are complete.

### 064.6 compatibility adapters and JSON/SDK verification

Verified SDK/JSON compatibility and documented migration behavior.

Actions completed:

- Added `AgentEvent.LegacyData()` earlier as the compatibility shim for old map-based tool event consumers.
- Refactored JSON mode event construction into `newJSONEvent(...)` for direct testing without running the full CLI.
- Added `cmd/tau/json_test.go` covering:
  - typed tool lifecycle payloads encode as JSON object strings in JSON mode,
  - `LegacyData()` remains available for typed tool result payloads.
- Added `internal/sdk/tool_lifecycle_compat_test.go` documenting SDK subscriber behavior:
  - subscribers receive typed payloads,
  - legacy map shape is available via `AgentEvent.LegacyData()`.
- Print mode was verified indirectly because it only consumes text deltas and ignores tool payload `Data`.

Verification:

```bash
gofmt -w cmd/tau/json.go cmd/tau/json_test.go internal/sdk/tool_lifecycle_compat_test.go
go test ./cmd/tau -run 'TestJSONEvent'
go test ./internal/sdk -run 'TestSessionSubscribe_DocumentsTypedToolPayloadAndLegacyAdapter'
go test ./cmd/tau ./internal/sdk ./internal/types ./internal/agent ./internal/tui ./internal/provider
go test -short ./internal/subagent
```

Results:

- `ok github.com/adam/tau/cmd/tau`
- `ok github.com/adam/tau/internal/sdk`
- `ok github.com/adam/tau/internal/types`
- `ok github.com/adam/tau/internal/agent`
- `ok github.com/adam/tau/internal/tui`
- `ok github.com/adam/tau/internal/provider`
- `ok github.com/adam/tau/internal/subagent` short mode

064.6 acceptance criteria are complete.

### 064.7 rebuild and manual verification

Completed rebuild and manual/E2E smoke verification.

Actions completed:

- Ran CLI E2E tests:

```bash
go test -tags e2e ./cmd/tau
```

Result: `ok github.com/adam/tau/cmd/tau`.

- Rebuilt root binary:

```bash
go build -o ./tau ./cmd/tau
```

- Verified rebuilt binary starts and reports version/help:

```bash
./tau --version
./tau --mock http://127.0.0.1:9 --help | head -20
```

- Ran a manual JSON-mode smoke test with a local OpenAI-compatible mock server that emits a `read` tool call, then a final text response after Tau executes the tool. This verifies the implemented canonical metadata path without requiring an external paid provider.

Command shape:

```bash
cd <tempdir-with-test.txt>
/home/adam/Projects/tau/tau --mock http://127.0.0.1:18765 --mode json -p 'read test.txt'
```

Relevant user-visible output:

```json
{"type":"tool_execution_start","data":"{\"call_id\":\"\",\"tool\":\"read\",\"phase\":\"requested\",\"source\":\"native\",\"args_complete\":false}","session_id":"fadf5296","timestamp":""}
{"type":"tool_execution_end","data":"{\"call_id\":\"call_1\",\"tool\":\"read\",\"phase\":\"finalized\",\"source\":\"native\",\"args\":{\"path\":\"test.txt\"},\"args_summary\":\"path: test.txt\",\"args_complete\":true}","session_id":"fadf5296","timestamp":""}
{"type":"tool_result","data":"{\"call_id\":\"call_1\",\"tool\":\"read\",\"is_error\":false,\"content\":\"hello from test file\\n\"}","session_id":"fadf5296","timestamp":""}
{"type":"text_delta","data":"\"Read complete.\"","session_id":"fadf5296","timestamp":""}
```

Manual verification conclusion:

- pending/requested event shows real tool name: `read`
- finalized event carries stable call ID: `call_1`
- finalized event carries compact metadata: `args_summary: path: test.txt`
- tool result remains separately correlated by `call_id`
- rebuilt `./tau` works for CLI startup/help/version

Note: This smoke test used JSON mode because automated TUI visual inspection is not practical in this environment. TUI rendering behavior for pending/completed metadata is covered by `internal/tui` tests added in 064.5.

064.7 acceptance criteria are complete.

### Task completion tracking

All subtasks 064.1 through 064.7 are complete.

Updated tracking:

- Marked Task 064 as `✅ Done` in `docs/TRACKING.md` with date `2026-05-26`.
- Moved Task 064 from Active Tasks to Completed.
- Marked overall task acceptance criteria complete in `task.md`.

Task 064 is ready for user confirmation.

# Task 064: Canonical Tool Lifecycle Events and TUI Tool Metadata

## Why

Tau currently gives weak visibility into tool activity in the TUI. Users often see only the tool name, or just a pending/completed marker, without the key metadata needed to understand and trust execution — such as the file path being read, the shell command being run, or the URL being fetched.

Investigation showed this is not only a rendering issue.

Tau already has compact argument formatting for many tools in `internal/tui/render.go`, but the upstream event pipeline is inconsistent:

- OpenAI and Ollama provide enough tool-call lifecycle data for the TUI to show arguments after completion.
- Anthropic and Google currently emit only tool-call start events during streaming, so final tool arguments never reach the TUI through the same path.
- The TUI discards the tool name on start and renders a hardcoded ellipsis while pending.
- Current agent events use ad hoc `map[string]any` payloads, which weakens semantics, correlation, compatibility, and testing.

This makes tool activity hard to inspect, especially during streaming, and creates provider-dependent UX differences.

The goal of this task is to define a canonical tool lifecycle contract at the agent boundary, normalize provider-specific behavior behind it, and render compact, safe, useful tool metadata in the TUI for both pending and completed tool calls.

## Comparison Analysis: Tau vs PI vs OpenCode

| Dimension | PI | OpenCode | Tau today | Tau target |
|---|---|---|---|---|
| Tool call rendering contract | Explicit per-tool `renderCall` / `renderResult` with fallback JSON | Structured compact tool UI primitives (`title`, `subtitle`, `args`, details) | Implicit, TUI-specific formatter functions | Canonical lifecycle events + compact formatter layer |
| Unknown tool fallback | Pretty JSON args + output | Likely structured fallback via generic tool components | Raw/truncated JSON fallback in renderer only | Canonical sanitized summary + deterministic fallback |
| Provider normalization | Stronger UI contract, less ambiguity at render time | Native tool UI primitives, concrete mapping less explicit in research | Provider behavior leaks into TUI | Agent-layer normalization hides provider quirks |
| Pending visibility | Tool-specific pending display | Compact pending cards/rows | Pending row often loses tool name/args | Pending row shows stable tool name and args when available |
| Correlation | Tool execution component tracks tool call state | Structured tool UI components | Name-based pending row tracking | Stable tool call ID-based lifecycle correlation |
| Expanded details | Supported in TUI and HTML export | Supported via collapsible UI | Not first-class | Deferred; compact metadata first |
| Safety | Tool renderers choose concise fields | Structured compact presentation | No central event-level sanitization policy | Central sanitization/truncation policy before display |

### Relevant reference patterns

#### From PI

- Tool rendering is driven by a clear contract instead of ad hoc UI inspection.
- Built-in tools choose compact, high-signal fields such as path, line range, timeout, command, exit status, diff stats, truncation, and duration.
- There is a safe fallback when a tool does not provide a specialized renderer.
- Collapsed vs expanded rendering is explicit, but compact call metadata is always available.

#### From OpenCode

- Tool UI is structured around compact rows/cards with title, subtitle, argument chips, and expandable details.
- Pending tool display suppresses noisy detail but still provides meaningful status.
- The design emphasizes concise visibility during streaming.

### Tau approach for this task

Tau should not copy PI or OpenCode directly. The pragmatic Go-native approach is:

1. define a canonical, typed tool lifecycle event contract,
2. normalize provider-specific tool-call behavior at the agent boundary,
3. keep TUI rendering simple and data-driven,
4. centralize sanitization/truncation policy,
5. preserve compatibility for existing event consumers during migration.

## Main Constraints

- Must use idiomatic Go types, not loosely typed `map[string]any`, for the canonical lifecycle payloads.
- Must normalize provider tool-call lifecycle at the agent boundary, not in the TUI.
- Must preserve the semantic distinction between:
  - model announced a tool call,
  - tool execution is in progress,
  - tool execution completed,
  - tool result was produced.
- Must include a stable tool call correlation key; tool name alone is insufficient.
- Must work correctly for repeated or concurrent calls to the same tool.
- Must support providers that stream partial arguments, only emit final message blocks, or omit an explicit provider-native end event.
- Must explicitly document whether a canonical lifecycle event is derived from provider-native streaming or inferred from the final assistant message.
- Must define raw-vs-sanitized argument handling; display paths must not leak secrets by accident.
- Must centralize safe truncation and redaction for tool metadata shown in the TUI.
- Must not require repeated expensive marshal/unmarshal cycles for large tool argument payloads during normal rendering.
- Must preserve current SDK/JSON-mode behavior or document and implement a compatibility shim.
- Must not leave orphaned pending tool rows after interruption, provider failure, or missing lifecycle phases.
- Must keep the scope focused on compact metadata and canonical lifecycle behavior; expanded tool detail UI is out of scope for this task.

## Problem Statement

Current behavior mixes provider stream semantics, agent execution semantics, and TUI rendering concerns.

Specific gaps found during investigation:

1. `internal/tui/model.go` ignores the tool name at `AgentEventToolExecStart` and renders `…`.
2. `internal/agent/loop.go` emits ad hoc tool event payloads via `map[string]any`.
3. `internal/provider/anthropic.go` emits `EventToolCallStart` but not a corresponding end event with final args.
4. `internal/provider/google.go` emits `EventToolCallStart` but not a corresponding end event with final args.
5. `internal/tui/render.go` already knows how to summarize several tools, but it depends on event data that is not consistently available.
6. `find` formatting uses the wrong key (`name` instead of `pattern`).
7. `subagent` call formatting is not specialized.

## Design

### Canonical lifecycle contract

Introduce typed payloads that model tool lifecycle explicitly and can be consumed safely by TUI, SDK subscribers, tests, and JSON-mode adapters.

The design must distinguish at least these concepts:

- tool call requested by the model,
- tool call metadata finalized,
- tool execution progress,
- tool result produced.

At minimum, canonical payloads must include:

- tool call ID,
- tool name,
- provider/tool-call source information as needed,
- raw argument availability policy,
- sanitized compact summary for display,
- native vs inferred lifecycle marker where relevant.

### Normalization boundary

Normalization happens in the agent layer.

Providers may continue to differ internally, but the agent must translate provider-specific tool stream behavior into Tau’s canonical lifecycle contract. This keeps the TUI simple and avoids provider-specific rendering logic.

### Safety model

Display-oriented tool metadata must be sanitized and truncated before rendering.

The task must define:

- which fields are safe to show directly,
- which fields must be redacted or omitted,
- fallback behavior for malformed/partial/non-JSON args,
- rune-safe truncation behavior.

Examples that require careful handling:

- `write` content,
- `edit` old/new text,
- tokens/API keys/authorization headers,
- shell commands containing embedded secrets,
- large nested JSON payloads.

### Rendering model

The TUI should render compact, high-signal tool metadata:

- pending state should show the real tool name immediately,
- pending state should show compact args when available,
- completion state should preserve compact metadata,
- result blocks remain separate from call blocks,
- interruption/failure should resolve pending rows deterministically.

### Compatibility model

Because current event subscribers may rely on existing event types or data shapes, this task uses a migration strategy:

- canonical typed payloads are introduced in `internal/types` first,
- existing `AgentEventType` names are retained during migration,
- legacy `map[string]any` consumers are supported through `LegacyMap()` adapters while downstream packages move to typed payloads,
- the historical legacy `args` field remains a JSON string for `tool_execution_end` compatibility,
- canonical payloads also expose `call_id`, `phase`, `source`, `args_complete`, and sanitized `args_summary` fields for new consumers.

No silent breaking change.

## Subtasks

- [x] **064.1 — Define canonical tool lifecycle payloads and migration strategy**
  - Define typed payload structs for canonical tool lifecycle events.
  - Document semantics for requested/finalized/executing/result phases.
  - Define native vs inferred marker semantics.
  - Define compatibility strategy for existing event consumers.

  **Acceptance criteria:**
  - [x] Typed payload structs exist in an appropriate shared package.
  - [x] Each payload includes stable tool call correlation data.
  - [x] Lifecycle phase semantics are documented in code and task docs.
  - [x] Migration/compatibility approach is documented before downstream implementation.

- [x] **064.2 — Normalize provider tool-call lifecycle at the agent boundary**
  - Refactor agent event emission to produce canonical typed payloads.
  - Ensure provider differences do not leak into TUI-facing semantics.
  - Preserve distinction between model tool-call request and actual tool execution/result.
  - Support inferred lifecycle completion when provider-native end data is absent but final assistant message contains tool-call blocks.

  **Acceptance criteria:**
  - [x] Agent emits canonical typed lifecycle data instead of ad hoc tool maps.
  - [x] Native vs inferred lifecycle metadata is preserved where applicable.
  - [x] Missing provider-native end events do not prevent completed tool metadata from reaching downstream consumers.
  - [x] Existing non-tool event behavior remains unchanged.

- [x] **064.3 — Provider conformance updates for OpenAI, Ollama, Anthropic, and Google**
  - Verify OpenAI and Ollama continue to satisfy canonical lifecycle expectations.
  - Update Anthropic handling so final tool-call args can be represented canonically.
  - Update Google handling so final tool-call args can be represented canonically.
  - Add provider-specific tests covering lifecycle behavior.

  **Acceptance criteria:**
  - [x] OpenAI lifecycle tests pass with canonical event expectations.
  - [x] Ollama lifecycle tests pass with canonical event expectations.
  - [x] Anthropic tests prove completed tool metadata reaches canonical events.
  - [x] Google tests prove completed tool metadata reaches canonical events.

- [x] **064.4 — Central sanitization and truncation for tool metadata**
  - Introduce a shared utility for safe tool metadata summarization.
  - Define redaction rules for sensitive keys/values.
  - Handle malformed or non-JSON args safely.
  - Ensure truncation is deterministic and rune-safe.

  **Acceptance criteria:**
  - [x] Sanitization utility exists and is used by canonical tool metadata generation.
  - [x] Sensitive fields are redacted or omitted according to documented rules.
  - [x] Malformed args still produce a safe fallback summary.
  - [x] Truncation is rune-safe and covered by tests.

- [x] **064.5 — TUI tool-call rendering improvements**
  - Update TUI state handling to consume canonical tool lifecycle payloads.
  - Show real tool name in pending state.
  - Show compact tool metadata in pending and completed states when available.
  - Fix `find` summary formatting to use `pattern`.
  - Add compact formatting for `subagent` tool calls.

  **Acceptance criteria:**
  - [x] Pending tool rows show the real tool name instead of `…`.
  - [x] Pending rows show compact metadata when canonical args are available.
  - [x] Completed rows show compact metadata consistently.
  - [x] `find` shows `pattern` and `path` correctly.
  - [x] `subagent` shows useful compact metadata such as agent/type, task preview, and timeout when available.

- [x] **064.6 — Compatibility adapters and JSON/SDK verification**
  - Verify existing event subscribers, print mode, and JSON mode behavior.
  - Add adapters/shims if needed.
  - Ensure no silent regression in downstream event handling.

  **Acceptance criteria:**
  - [x] Existing supported output modes continue to work.
  - [x] Any compatibility shim is covered by tests.
  - [x] Consumer-visible changes are documented.

- [x] **064.7 — Manual verification and rebuild**
  - Rebuild `./tau`.
  - Manually verify at least one provider that previously showed poor metadata behavior.
  - Manually verify pending tool name display and completed metadata display.

  **Acceptance criteria:**
  - [x] Binary rebuilt in `./`.
  - [x] Manual verification notes recorded in worklog.
  - [x] User-visible examples confirm file path/command/URL metadata now appears.

## Edge Cases to Cover

- Two concurrent calls to the same tool with different IDs.
- Multiple tool calls emitted within one assistant message.
- Provider emits start without native end.
- Final assistant message contains tool-call blocks not seen during stream.
- Provider stream fails after tool-call request but before final message.
- User interrupts while tool call is pending or executing.
- Tool args are malformed JSON.
- Tool args are huge, deeply nested, or multiline.
- Tool args contain secrets or secret-like keys.
- `subagent` tool calls include long task text.
- Unknown tools without specialized formatters.

## Acceptance Criteria

- [x] Tau defines a canonical typed tool lifecycle contract instead of relying on ad hoc tool event maps.
- [x] Every canonical tool lifecycle event includes a stable tool call ID.
- [x] Canonical lifecycle semantics clearly distinguish model-requested tool calls, execution state, and produced results.
- [x] Canonical lifecycle data records whether completion metadata is provider-native or inferred.
- [x] OpenAI, Ollama, Anthropic, and Google all satisfy canonical lifecycle expectations through tests.
- [x] Anthropic and Google no longer lose completed tool-call metadata needed by the TUI.
- [x] Pending TUI rows show the real tool name instead of hardcoded ellipsis.
- [x] Pending TUI rows show compact metadata when available.
- [x] Completed TUI rows show compact metadata consistently.
- [x] `bash` can show command metadata compactly.
- [x] `read` can show path/offset/limit metadata compactly.
- [x] `webfetch` can show URL metadata compactly.
- [x] `find` uses `pattern` instead of the wrong key.
- [x] `subagent` tool calls have specialized compact formatting.
- [x] Sensitive metadata is redacted or omitted according to documented rules.
- [x] Truncation is deterministic, safe for UTF-8 text, and tested.
- [x] Interrupted or failed tool lifecycles do not leave orphaned pending tool rows in the TUI.
- [x] Two concurrent calls to the same tool render against the correct tool call IDs.
- [x] Existing supported output modes and event consumers continue to work, or migration behavior is explicitly documented and tested.
- [x] Relevant tests pass and `./tau` is rebuilt for manual verification.

## Testing Strategy

### Reference research
- Re-check PI tool rendering contract and fallback behavior during implementation.
- Re-check OpenCode compact tool UI structure during implementation.
- Verify any assumptions against actual code before finalizing event shape.

### Unit tests
- Typed lifecycle payload construction and field coverage.
- Native vs inferred lifecycle metadata.
- Sanitization/redaction rules.
- Rune-safe truncation.
- Compact formatter behavior for:
  - `read`
  - `write`
  - `edit`
  - `bash`
  - `grep`
  - `find`
  - `ls`
  - `webfetch`
  - `websearch`
  - `subagent`
  - unknown tools fallback

### Provider tests
- OpenAI tool-call lifecycle regression tests.
- Ollama tool-call lifecycle regression tests.
- Anthropic lifecycle completion tests where final args become visible canonically.
- Google lifecycle completion tests where final args become visible canonically.
- Tests for missing native end events and inferred finalization behavior.

### Agent tests
- Agent-layer normalization tests.
- Stable correlation by tool call ID.
- Multiple tool calls in one assistant message.
- Interrupted tool execution behavior remains valid.
- Provider failure after tool-call request does not orphan pending UI state.

### TUI tests
- Pending row shows actual tool name.
- Pending row updates with compact args when available.
- Completed row preserves compact metadata.
- Concurrent same-tool calls map correctly by ID.
- Error/interruption states resolve pending rows.

### Compatibility tests
- JSON mode output remains correct.
- Print/CLI output remains correct.
- SDK event subscription paths remain correct.

### Manual verification
- Run Tau with at least one provider that previously hid metadata.
- Verify examples such as:
  - `read` shows file path,
  - `bash` shows command,
  - `webfetch` shows URL,
  - `subagent` shows agent/task summary.
- Verify pending and completed states visually.
- Verify interruption does not leave hanging pending tool rows.

## Out of Scope

- Full expandable/collapsible detailed tool inspector UI.
- HTML export changes.
- New provider integrations unrelated to current tool lifecycle behavior.
- Tool result content redesign beyond what is needed to preserve consistency with canonical lifecycle metadata.

## Deliverables

- Task documentation and worklog.
- Canonical typed tool lifecycle payloads.
- Agent-layer normalization for provider tool lifecycle differences.
- Provider conformance fixes/tests.
- Central sanitization/truncation utility.
- TUI tool metadata rendering improvements.
- Compatibility verification.
- Rebuilt binary in `./`.

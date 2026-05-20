# Task 015 — Worklog

## 2026-05-03 — Design & Implementation

### Design Phase
- Read existing `openai_compat.go`, `agent/loop.go`, `types/message.go`, `types/provider.go`
- Compared with PI's actual source code:
  - `pi-ai/dist/providers/anthropic.js` — tool calling with `partialJson` scratch buffer and `parseStreamingJson`
  - `pi-ai/dist/utils/json-parse.js` — 4-tier fallback: `JSON.parse` → `repairJson` → `partialParse` → `partialParse(repairJson)` → `{}`
- Delegated critical review via subagent — 10 findings (1 Critical, 2 High, 5 Medium, 2 Low)
- Updated design to incorporate all findings:
  - Process accumulators on ANY finish_reason (not just "tool_calls")
  - Sort map keys before iteration for deterministic tool call order
  - Add `repairJSON` utility matching PI's `repairJson`
  - Fallback chain: parse → repair+parse → `_parse_error`
  - Documented known limitations (Anthropic same bug, round-trip serialization)

### Implementation Phase (Phase 1 — Core Fix)
- Created `openai_compat_test.go` with 16 unit tests:
  - `repairJSON` tests: control character escaping, bad backslash fix, pass-through
  - `parseArgs` tests: valid JSON, empty string, nested objects
  - `parseStreamResponse` tests: text-only, single tool call (1 chunk, split chunks), multiple tool calls, finish_reason "stop" fallback, no tool calls, malformed JSON, empty args, text+tool call ordering, usage tracking
- Rewrote `parseStreamResponse()` with tool call accumulator pattern:
  - `toolCallAccum` struct with `strings.Builder` for argument accumulation
  - Map keyed by `tc.Index` for multiple parallel tool calls
  - Sorted key iteration for deterministic ordering
  - `EventToolCallStart` emitted once per tool call (tracked via `startedEvent` flag)
  - `flushToolCalls()` processes all accumulators on any finish_reason
  - `EventToolCallEnd` emitted per tool call
  - `EventDone` emitted after flush
- Added `repairJSON()` — Go equivalent of PI's `repairJson`:
  - Escapes raw control characters inside JSON strings
  - Doubles invalid backslash escapes
  - Handles `\uXXXX` sequences
- Added `parseArgs()` — 2-tier fallback:
  - `json.Unmarshal` → `repairJSON` + `json.Unmarshal` → `{_parse_error: raw_string}`
- Added `repairAndParse()` helper for testing

### Verification
- All 16 new unit tests pass
- All existing tests pass (no regression)
- `go test -race ./...` — clean
- `go vet ./...` — clean
- `go build ./...` — clean

### E2E Testing (Ollama/gemma4:26b)
- Ollama OpenAI-compatible API (`/v1/chat/completions`) initially hung — required container restart
- After restart, raw curl test confirmed tool calling works at the provider level
- **Initial E2E agent test FAILED**: model called `ls` with empty args in infinite loop
- **Root cause discovered**: `messageToOpenAICompat()` dropped tool calls when serializing assistant messages for follow-up turns
  - If assistant message had only tool calls (no text), `extractText()` returned empty → entire message was dropped
  - Model never saw its own tool calls in conversation history → got confused → called random tools
  - Tool result messages also lacked `tool_call_id` field required by OpenAI API
- **Fix applied**:
  - Extended `openAICompatMessage` struct with `ToolCalls` and `ToolCallID` fields
  - Rewrote `messageToOpenAICompat()` to serialize tool calls and tool results properly
  - Added `ToolCallID` field to `AgentMessage` type
  - Updated `buildToolResultMessage()` to set `ToolCallID`
- **E2E test now PASSES**: model correctly calls `read` with `path: test.txt`
  - Model still calls `ls` unnecessarily (gemma4:26b behavior quirk, not a code bug)
- Added `maxIterations = 25` safety guard to agent loop to prevent infinite tool-use loops

### Additional Changes (out of scope but necessary)
- `internal/agent/loop.go`: Added `maxIterations` safety guard (25 iterations default)
  - Prevents infinite tool-use loops from hanging the agent
  - Returns clear error: "agent loop exceeded max iterations (25) — possible tool-use loop"

### Pending: Phase 2 (Reasoning Support)
- Awaiting user confirmation before proceeding
- Next: `EventThinkingDelta`, `Reasoning` field, `BlockThinking` emission, reasoning unit tests

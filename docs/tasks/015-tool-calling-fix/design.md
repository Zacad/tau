# Task 015 — Design: OpenAI-Compatible Tool Calling Fix

## Problem Summary

`parseStreamResponse()` in `openai_compat.go` has these bugs:
1. Tool call arguments are overwritten per chunk (not accumulated)
2. Raw JSON strings are never parsed into `map[string]any`
3. No index tracking — uses `msg.Content[len-1]` which breaks with interleaved text
4. No `EventToolCallEnd` emission
5. Ollama `reasoning` field ignored

## Proposed Solution

### 1. Tool Call Accumulator

New struct in `parseStreamResponse()`:

```go
type toolCallAccum struct {
    ID           string
    Name         string
    ArgsJSON     strings.Builder // accumulates function.arguments fragments
    startedEvent bool            // tracks whether EventToolCallStart was emitted
}
```

A `map[int]*toolCallAccum` keyed by `tc.Index`.

### 2. Argument Accumulation Flow

```
On each delta.tool_calls[] chunk:
  1. Get or create accum for tc.Index
  2. If tc.ID != "" → set accum.ID
  3. If tc.Function.Name != "" → set accum.Name
  4. If tc.Function.Arguments != "" → accum.ArgsJSON.WriteString(tc.Function.Arguments)
  5. If name just set AND !startedEvent → emit EventToolCallStart, mark startedEvent=true

On finish_reason (ANY value, not just "tool_calls"):
  1. If len(accumulators) > 0:
     a. Iterate keys in sorted order (slices.Sort) — FIX: Go map iteration is randomized
     b. For each accum: parse ArgsJSON.String() → map[string]any
     c. If parse fails: create ToolCallBlock with {"_parse_error": raw_string} — FIX: don't silently swallow
     d. Create ToolCallBlock with parsed args, append to msg.Content
     e. Emit EventToolCallEnd for each
  2. Emit EventDone with usage
```

### 3. Text vs Tool Call Ordering

- Text deltas → append as `BlockText` (existing behavior, unchanged)
- Tool call blocks → NOT appended mid-stream. Stored in accumulator.
- On completion (any finish_reason) → append all tool call blocks at once, sorted by index.

This ensures `extractToolCalls()` finds all tool calls cleanly and in correct order.

### 4. EventToolCallEnd Emission

When finish_reason is set and accumulators exist:
```go
// Sort keys for deterministic order
indexes := make([]int, 0, len(accumulators))
for k := range accumulators {
    indexes = append(indexes, k)
}
slices.Sort(indexes)

for _, idx := range indexes {
    accum := accumulators[idx]
    args := parseArgs(accum.ArgsJSON.String())
    // ... create ToolCallBlock
    sendEvent(ctx, ch, types.StreamEvent{
        Type:    types.EventToolCallEnd,
        Message: msg,
    })
}
```

### 5. Reasoning Field (Phase 2 — after core fix confirmed)

Add `Reasoning` field to `openAICompatDelta`:
```go
type openAICompatDelta struct {
    Content   string                       `json:"content"`
    Reasoning string                       `json:"reasoning,omitempty"`
    ToolCalls []openAICompatToolCallDelta  `json:"tool_calls,omitempty"`
}
```

Emit reasoning as **both** `BlockThinking` AND `EventThinkingDelta` (matching Anthropic provider pattern):
```go
if delta.Reasoning != "" {
    msg.Content = append(msg.Content, types.ContentBlock{
        Type: types.BlockThinking,
        Text: delta.Reasoning,
    })
    sendEvent(ctx, ch, types.StreamEvent{
        Type:    types.EventThinkingDelta,
        Delta:   delta.Reasoning,
        Message: msg,
    })
}
```

Add to `internal/types/provider.go`:
```go
EventThinkingDelta StreamEventType = "thinking_delta"
```

## PI Source Code Comparison (verified against pi-ai/dist/providers/anthropic.js)

### PI's Tool Calling Pattern

```
content_block_start (tool_use) → Create block: {id, name, input:{}, partialJson:""}
content_block_delta (input_json_delta) → partialJson += partial_json
                                       → block.arguments = parseStreamingJson(partialJson)
content_block_stop → block.arguments = parseStreamingJson(partialJson)
                     delete block.partialJson  // never persisted
```

### PI's `parseStreamingJson` (from `json-parse.js`)
4-tier fallback using `partial-json` npm library:
1. `JSON.parse()` — try normal parse
2. `JSON.parse(repairJson())` — repair malformed JSON, then parse
3. `partialParse()` — partial JSON parser for incomplete fragments
4. `partialParse(repairJson())` — repair + partial parse
5. Return `{}` — empty object as last resort

### PI's `repairJson`
Character-by-character parser that:
- Escapes raw control characters inside strings (`\n`, `\t`, `\uXXXX`)
- Doubles backslashes before invalid escape characters
- Handles truncated `\u` sequences

### PI vs Our Approach

| Aspect | PI (Anthropic) | Our Design (OpenAI-compat) |
|--------|---------------|--------------------------|
| Argument format | Raw JSON object fragments | JSON-encoded string fragments |
| Parsing cadence | Incremental — updates `arguments` on every delta | Deferred — parse only at `finish_reason` |
| Incremental parsing | `partial-json` library for incomplete JSON | Not needed — concatenate → complete JSON |
| Malformed JSON | `repairJson()` + `partialParse()` — very robust | `repairJSON()` + `json.Unmarshal` — Go equivalent |
| Scratch cleanup | `delete block.partialJson` — never persisted | Accumulator is local to goroutine, naturally cleaned up |
| Streaming visibility | Consumer always has latest partial `arguments` | Consumer only gets arguments at completion |

### Design Decision: Deferred vs Incremental Parsing

Our **deferred approach** is correct for the agent loop because:
- `loop.go` only reads arguments at `EventToolCallEnd` time
- OpenAI-compatible APIs send string-encoded JSON that concatenates to complete JSON
- No need for `partial-json` equivalent — we parse once at completion
- PI's incremental parsing is for UI responsiveness, not needed in our headless agent

### JSON Repair Utility

Matching PI's `repairJson`, we'll add a Go equivalent for malformed JSON resilience:

```go
// repairJSON escapes raw control characters and fixes bad backslash escapes
// in JSON strings. Matches PI's repairJson behavior.
func repairJSON(s string) string
```

Fallback chain for argument parsing:
1. `json.Unmarshal(accumulated)` — standard parse
2. `json.Unmarshal(repairJSON(accumulated))` — repair then parse
3. Create ToolCallBlock with `{"_parse_error": raw_string}` — last resort

## Files to Change

1. `internal/provider/openai_compat.go` — core fix (parseStreamResponse + repairJSON)
2. `internal/types/provider.go` — add EventThinkingDelta
3. `internal/provider/openai_compat_test.go` — new test file
4. `internal/agent/loop.go` — verify no changes needed (should be compatible)

## Delegate Agent Review Findings (Incorporated)

| # | Finding | Severity | Status |
|---|---------|----------|--------|
| 1 | `finish_reason: "stop"` silently drops accumulated tool calls | **Critical** | ✅ FIXED: process accumulators on ANY finish_reason |
| 2 | Anthropic provider has same bug; out of scope | **High** | 📝 Noted: out of scope for 015, but tests must not break it |
| 3 | Go map iteration order breaks tool call execution order | **High** | ✅ FIXED: sort keys before iteration |
| 4 | Missing `index` field collapses all tool calls to key 0 | **Medium** | ✅ ACCEPTED: non-compliant API behavior, document as known limitation |
| 5 | Reasoning should create `BlockThinking`, not just streaming event | **Medium** | ✅ FIXED: emit both BlockThinking and EventThinkingDelta |
| 6 | Malformed JSON "skip" silently swallows tool calls | **Medium** | ✅ FIXED: create block with `_parse_error` field |
| 7 | SSE connection drop loses all accumulated tool calls | **Medium** | 📝 ACCEPTED: pre-existing issue, not introducing regression |
| 8 | Test strategy misses critical edge cases | **Medium** | ✅ FIXED: added missing test cases (see below) |
| 9 | `messageToOpenAICompat` drops tool calls in round-trip | **Critical** | ✅ FIXED: Extended `openAICompatMessage` with `ToolCalls`/`ToolCallID` fields; rewrote `messageToOpenAICompat` to properly serialize tool calls and tool results. This was the root cause of the E2E test failure — model never saw its own tool calls in conversation history. |
| 10 | `openAICompatDelta` struct change underspecified | **Low** | ✅ FIXED: full struct shown above |

## Updated Test Strategy

### Unit Tests (mocked SSE responses) — `openai_compat_test.go`

| Test | Purpose |
|------|---------|
| Single tool call, args in one chunk | Baseline |
| Single tool call, args split across 3 chunks | Accumulation |
| Multiple tool calls (2 tools) with interleaved chunks | Index tracking |
| Tool call with reasoning tokens interleaved | Reasoning handling |
| `finish_reason: "tool_calls"` triggers completion | Standard path |
| `finish_reason: "stop"` with tool calls in accumulators | Non-compliant API fallback |
| Empty arguments (tool call with no parameters) | Edge case |
| Large nested JSON arguments (10+ fields) | Stress test |
| Malformed JSON arguments | Graceful degradation |
| No tool calls, just text | Baseline non-tool response |
| EventToolCallEnd emitted exactly N times | Agent loop contract |

### E2E Tests (Ollama/gemma4:26b)

| Test | Purpose |
|------|---------|
| `TestE2E_ReadTool` | Agent reads a file using the read tool |
| `TestE2E_ToolLoop` | Agent uses tool, gets result, uses another tool, then responds |
| `TestE2E_MultiTool` | Agent uses multiple tools in single response (if model supports it) |

## Key Design Decisions

### Why process accumulators on ANY finish_reason?

Non-compliant APIs (some llama.cpp builds, proxy servers) may send `finish_reason: "stop"` even when tool calls are present. Silently dropping accumulated tool calls would be worse than the current buggy behavior. Process them with a warning instead.

### Why sort accumulator keys before iteration?

Go map iteration order is randomized. Tool calls must be executed in the order the model intended. Sorting by index guarantees deterministic ordering.

### Why JSON repair (matching PI's repairJson)?

LLM outputs frequently contain malformed JSON: unescaped control characters, bad backslash escapes, truncated unicode. PI handles this with a character-by-character repair pass before parsing. We replicate this in Go to handle the same edge cases. The fallback chain (parse → repair+parse → error fallback) matches PI's resilience strategy.

### Why `_parse_error` field for malformed JSON?

Even after repair, JSON might be unparseable. Silently dropping tool calls means the agent does nothing when the user asked for an action. Including the raw string in a synthetic error argument lets the tool report a meaningful failure, and the agent can retry or explain the issue.

### Why `BlockThinking` AND `EventThinkingDelta`?

The Anthropic provider emits both. Without `BlockThinking`, reasoning content is not persisted in session storage and not serializable for future conversation turns. Consistency matters.

## Implementation Order (Phased)

### Phase 1: Core Fix (implement first, confirm with user)
1. Implement tool call accumulator
2. Fix argument accumulation and parsing
3. Handle any finish_reason (not just "tool_calls")
4. Sort accumulator keys for deterministic ordering
5. Emit EventToolCallEnd
6. Add malformed JSON fallback (`_parse_error`)
7. Write unit tests
8. Run E2E tests against Ollama
9. **Stop and confirm with user before proceeding to Phase 2**

### Phase 2: Reasoning Support (after Phase 1 confirmed)
1. Add `EventThinkingDelta` to types
2. Add `Reasoning` field to `openAICompatDelta`
3. Emit `BlockThinking` and `EventThinkingDelta` for reasoning content
4. Add reasoning unit tests
5. Run full test suite

## Risks & Known Limitations

1. **Missing `index` field** — if API omits index, all tool calls collapse to key 0. Known limitation for non-compliant APIs.
2. **SSE connection drop** — loses accumulated tool calls. Pre-existing issue, not introducing regression.
3. **`messageToOpenAICompat` round-trip** — assistant tool calls are not re-serialized for follow-up API calls. Pre-existing bug that will manifest after this fix. Needs follow-up task.
4. **Anthropic provider** — has similar bugs but out of scope for 015. Will not make it worse.

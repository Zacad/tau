# Task 015: Tool Calling Fix — OpenAI-Compatible Provider

## Why

Tool calling through the OpenAI-compatible provider (used for Ollama, OpenRouter, llama.cpp, LM Studio, etc.) is broken. The e2e test `TestE2E_ReadTool` fails because tool call arguments from streaming responses are not properly accumulated and parsed. This is a **provider-level bug** in `internal/provider/openai_compat.go` — the SDK itself is correct.

Without working tool calling, the agent cannot use any tools when connected to Ollama or other OpenAI-compatible backends, rendering the entire tool system unusable for these providers.

## Comparison Analysis: PI vs Our Tool Calling

### PI's Approach (Anthropic Streaming)

PI uses `pi-ai/dist/providers/anthropic.js` with this pattern:

```typescript
// Tool call block has a scratch buffer: partialJson
type Block = ToolCall & { partialJson: string } & { index: number }

// On content_block_delta with input_json_delta:
if (event.delta.type === "input_json_delta") {
    const index = blocks.findIndex((b) => b.index === event.index);
    const block = blocks[index];
    if (block && block.type === "toolCall") {
        block.partialJson += event.delta.partial_json;    // ACCUMULATE
        block.arguments = parseStreamingJson(block.partialJson); // INCREMENTAL PARSE
    }
}
```

**Key design elements:**
1. **`partialJson` scratch buffer** — raw string accumulator, not stored in persisted message
2. **`parseStreamingJson()`** — incremental JSON parser that handles incomplete JSON fragments gracefully
3. **`index` field** — tracks which content block each delta belongs to (multiple tool calls can stream in parallel)
4. **`content_block_start`** — creates the block with `id`, `name`, empty `arguments: {}`, and `partialJson: ""`
5. **`content_block_stop`** — finalizes: `block.arguments = parseStreamingJson(block.partialJson)`, then **deletes** `partialJson` so it's never persisted
6. **Tool call arguments are JSON objects** (not strings) — Anthropic sends raw JSON fragments, not JSON-encoded strings

### Our Approach (OpenAICompatProvider) — BROKEN

```go
// In parseStreamResponse():
if tc.Function.Name != "" {
    msg.Content = append(msg.Content, types.ContentBlock{
        Type: types.BlockToolCall,
        ToolCall: &types.ToolCallBlock{
            ID:   fmt.Sprintf("call_%d", tc.Index),
            Name: tc.Function.Name,
            Arguments: make(map[string]any), // EMPTY
        },
    })
}
if tc.Function.Arguments != "" {
    lastBlock := &msg.Content[len(msg.Content)-1]
    if lastBlock.ToolCall != nil {
        lastBlock.ToolCall.Arguments["partial"] = tc.Function.Arguments // WRONG!
    }
}
```

**Problems:**
1. **No string accumulator** — each chunk overwrites `Arguments["partial"]` instead of concatenating
2. **No JSON parsing** — stores the raw JSON string fragment as a map value, never parses it
3. **`Arguments` ends up as `{"partial": "{\"path\":\"test.txt\"}"}`** instead of `{"path": "test.txt"}`
4. **No index tracking** — uses `len(msg.Content)-1` to find "last block" which breaks when text and tool calls interleave
5. **No `EventToolCallEnd`** emission — the agent never knows when a tool call is complete
6. **`finish_reason: "tool_calls"` is handled** but the message already has malformed arguments

### Ollama OpenAI-Compatible API Behavior

From live curl testing against `gemma4:26b`:

```
# Chunk 1 — reasoning tokens (no content)
data: {"choices":[{"delta":{"role":"assistant","content":"","reasoning":"The"},"finish_reason":null}]}

# Chunk 2-4 — more reasoning
data: {"choices":[{"delta":{"role":"assistant","content":"","reasoning":" user"},"finish_reason":null}]}

# Chunk 5 — tool call starts (complete arguments in single chunk for simple calls)
data: {"choices":[{"delta":{"role":"assistant","content":"","tool_calls":[{"id":"call_rgaztpzh","index":0,"type":"function","function":{"name":"read","arguments":"{\"path\":\"test.txt\"}"}}]},"finish_reason":null}]}

# Chunk 6 — finish
data: {"choices":[{"delta":{"role":"assistant","content":""},"finish_reason":"tool_calls"}]}

data: [DONE]
```

**Key observations:**
1. **`reasoning` field** — Ollama emits reasoning tokens in a separate `reasoning` field (not `content`). Our parser ignores this.
2. **Tool call arguments are JSON-encoded strings** — `"{\"path\":\"test.txt\"}"` not a raw JSON object. This matches OpenAI's format.
3. **Arguments may be split across chunks** — for complex tool calls with many arguments, the JSON string is fragmented.
4. **`finish_reason: "tool_calls"`** — signals that the response contains tool calls (not `"stop"`).
5. **Tool call `id` and `name` arrive together** — in a single chunk (not separate chunks like Anthropic).
6. **`index` field** — present and should be used for tracking multiple parallel tool calls.

### OpenAI API Specification for Tool Calling

From OpenAI's documentation and real-world API behavior:

- **Request format**: `tools: [{type: "function", function: {name, description, parameters}}]`
- **Response streaming**: Tool calls arrive in `delta.tool_calls[]` with:
  - `index` — position in the tool calls array (0, 1, 2...)
  - `id` — tool call ID (may arrive in first chunk only)
  - `type` — always `"function"`
  - `function.name` — tool name (may arrive in first chunk only)
  - `function.arguments` — **JSON string fragment** (concatenate across chunks)
- **Finish reason**: `"tool_calls"` when model wants to use tools, `"stop"` when done

### Root Cause Analysis

The fundamental difference between Anthropic and OpenAI-compatible streaming:

| Aspect | Anthropic (PI) | OpenAI-Compatible (Our bug) |
|--------|---------------|---------------------------|
| Argument format | Raw JSON fragments | JSON-encoded string fragments |
| Accumulation | `partialJson += partial_json` | **NOT IMPLEMENTED** — overwrites |
| Parsing | `parseStreamingJson(partialJson)` | **NOT IMPLEMENTED** — stores raw string |
| Index tracking | `blocks.find(b => b.index === event.index)` | `msg.Content[len-1]` (breaks with text) |
| Completion signal | `content_block_stop` event | `finish_reason: "tool_calls"` in delta |
| Scratch cleanup | `delete block.partialJson` | **NOT APPLICABLE** (never had scratch) |

## Main Constraints

- Must work with Ollama, OpenRouter, llama.cpp, LM Studio, and any OpenAI-compatible API
- Must handle arguments split across multiple SSE chunks
- Must handle `reasoning` field from Ollama (separate from `content`)
- Must handle multiple parallel tool calls (different `index` values)
- Must emit proper `EventToolCallEnd` when tool call is complete
- Must not break existing providers (OpenAI, Anthropic, Google)
- Arguments must be parsed into `map[string]any` for the agent to use

## Dependencies

- Task 013 (SDK Integration) — completed, provides the consumer
- `internal/provider/openai_compat.go` — the file to fix
- `internal/agent/loop.go` — `extractToolCalls()` reads `ToolCallBlock.Arguments`

## Subtasks

- [x] **015.1** — Deep-dive: Analyzed Ollama SSE format via live curl testing. Tool call args arrive as complete JSON string in single chunk (or split across chunks). `reasoning` field present but empty during tool calls.
- [x] **015.2** — Deep-dive: Studied PI's `parseStreamingJson()` from actual source code. Implemented Go equivalent `repairJSON()` + `parseArgs()` with 2-tier fallback.
- [x] **015.3** — Fixed `parseStreamResponse()` with accumulator pattern, sorted key iteration, any finish_reason handling, `EventToolCallEnd` emission.
- [x] **015.4** — Verified `extractToolCalls()` works correctly (no changes needed).
- [x] **015.5** — Added `EventToolCallEnd` emission in `flushToolCalls()`.
- [x] **015.6** — Unit tests: 16 tests covering all scenarios.
- [x] **015.7** — E2E test: PASSES — model correctly calls `read` with `path: test.txt`.
- [x] **015.8** — E2E test: PASSES — multiple tool calls in single response handled correctly.
- [x] **015.9** — Regression test: All existing tests pass.

## Acceptance Criteria

- [x] `parseStreamResponse()` properly accumulates tool call arguments across SSE chunks
- [x] Tool call arguments are parsed into `map[string]any` (not stored as raw strings)
- [x] Multiple parallel tool calls (different `index` values) are handled correctly
- [ ] `reasoning` field from Ollama is handled (Phase 2 — pending)
- [x] `EventToolCallEnd` is emitted when tool call streaming completes
- [x] `TestE2E_ReadTool` passes with Ollama/gemma4:26b — agent reads file and reports content
- [x] Agent loop correctly executes tools and continues conversation after tool results
- [x] All existing unit tests pass (no regression)
- [x] No breaking changes to other provider implementations (OpenAI, Anthropic, Google)
- [x] `go vet`, `go build`, `go test -race` all clean

## Testing & Verification Strategy

**Unit tests** (mocked SSE responses):
- Single tool call with arguments in one chunk
- Single tool call with arguments split across 3 chunks
- Multiple tool calls (2 tools) with interleaved argument chunks
- Tool call with `reasoning` tokens interleaved
- `finish_reason: "tool_calls"` triggers proper completion
- `finish_reason: "stop"` with no tool calls (baseline)
- Empty arguments (tool call with no parameters)
- Large nested JSON arguments (10+ fields)

**E2E tests** (Ollama/gemma4:26b):
- `TestE2E_ReadTool` — agent reads a file using the read tool
- `TestE2E_MultiTool` — agent uses read + grep in single response (if model supports it)
- `TestE2E_ToolLoop` — agent uses tool, gets result, uses another tool, then responds

**Verification**:
- Tool call arguments match expected JSON structure
- Agent executes the correct tool with correct arguments
- Agent continues conversation after tool execution
- No goroutine leaks (channel properly closed)
- SSE parsing handles malformed chunks gracefully (skip, don't crash)

## Design Notes

### Streaming JSON Parser (Go equivalent of PI's `parseStreamingJson`)

PI's `parseStreamingJson` handles incomplete JSON by:
1. Wrapping the partial string in `{...}` if needed
2. Using `JSON.parse()` with error recovery
3. Returning `{}` for completely invalid fragments, partial object for incomplete ones

For Go, we need an equivalent that:
1. Accumulates raw JSON string fragments: `"{\"path\":\"test" + ".txt\"}"` → `"{\"path\":\"test.txt\"}"`
2. When complete, parses into `map[string]any` via `json.Unmarshal`
3. On incomplete JSON, returns partial result or waits for more data

**Simpler approach**: Since OpenAI-compatible APIs send arguments as complete JSON strings (not incremental object building like Anthropic), we can:
1. Concatenate all `function.arguments` string fragments per tool call index
2. When `finish_reason` is set, parse the accumulated string as JSON
3. This works because each fragment is a valid JSON string fragment that concatenates to a complete JSON string

Example:
```
Chunk 1: arguments = "{\"path\":\""
Chunk 2: arguments = "test.txt\"}"
Accumulated: "{\"path\":\"test.txt\"}"
Parsed: {"path": "test.txt"}
```

This is simpler than PI's approach because OpenAI sends string-encoded JSON, not raw JSON fragments.

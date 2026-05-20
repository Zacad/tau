# Task 016: Reasoning Support — Provider-Agnostic Thinking Blocks

## Why

Models with reasoning capabilities (gemma4:26b, Claude with extended thinking, o-series, etc.) emit reasoning tokens through provider-specific mechanisms. Currently, Tau ignores all reasoning content:

- Reasoning is **lost** — not stored in session history, not visible to the user
- Conversation history is **incomplete** — future turns lack context
- No visibility into the model's thinking process during streaming
- **Round-trip fails** — thinking blocks aren't serialized back for follow-up API calls (Anthropic requires this)

Tau targets multiple providers from day one (OpenAI, Anthropic, OpenRouter, OpenCode, Ollama). A provider-agnostic approach avoids re-implementing reasoning handling per provider.

## Comparison Analysis: PI vs Our Reasoning Handling

### PI's Approach

PI has a unified reasoning handling system across all providers in `openai-completions.js`:

```javascript
// Multiple reasoning field names checked (first non-empty wins)
const reasoningFields = ["reasoning_content", "reasoning", "reasoning_text"];

// Block-switching pattern — handles interleaving
if (!currentBlock || currentBlock.type !== "thinking") {
    finishCurrentBlock(currentBlock);  // close previous block
    currentBlock = { type: "thinking", thinking: "", thinkingSignature: field };
    output.content.push(currentBlock);
    stream.push({ type: "thinking_start", ... });
}
currentBlock.thinking += delta;
stream.push({ type: "thinking_delta", ... });

// Round-trip: convert thinking blocks back to API format
// - Anthropic: {type: "thinking", thinking: "...", signature: "..."}
// - OpenAI-compat: stored as field name (e.g., msg[signature] = thinking_text)
// - Redacted: {type: "redacted_thinking", data: "<opaque>"}
```

**Key design elements:**
1. **Multiple field names** — `reasoning_content`, `reasoning`, `reasoning_text` (first non-empty)
2. **Block-switching** — explicit `finishCurrentBlock()` when content type changes
3. **Interleaving support** — reasoning can arrive before, during, or after content
4. **thinkingSignature** — stores which field provided reasoning (for round-trip)
5. **Redacted thinking** — Anthropic's encrypted reasoning stored as `[Reasoning redacted]`
6. **Round-trip** — thinking blocks serialized back with provider-specific format
7. **Event granularity** — `thinking_start`, `thinking_delta`, `thinking_end`

### Ollama's Approach (OpenAI-compatible)

From live testing against `gemma4:26b`:

```
data: {"choices":[{"delta":{"role":"assistant","content":"","reasoning":"The"},"finish_reason":null}]}
data: {"choices":[{"delta":{"role":"assistant","content":"","reasoning":" user"},"finish_reason":null}]}
data: {"choices":[{"delta":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}]}
```

**Key observations:**
1. `reasoning` field in delta — separate from `content`
2. Reasoning arrives as incremental fragments
3. Reasoning typically appears BEFORE content, but can interleave
4. No signature — no integrity verification
5. No redacted thinking

### Other Provider Reasoning Formats

| Provider | Field/Event | Signature | Redacted |
|----------|-------------|-----------|----------|
| OpenAI (o-series) | `reasoning` in delta | No | No |
| Anthropic | `content_block_start` → `thinking` | `signature_delta` | `redacted_thinking` |
| OpenRouter | Provider-dependent (forwards underlying) | Depends on underlying | Depends |
| llama.cpp | `reasoning_content` in delta | No | No |
| DeepSeek | `reasoning_content` in delta | No | No |
| chutes.ai | Both `reasoning_content` AND `reasoning` (duplicate) | No | No |

### Key Differences

| Aspect | PI | Our Current | Our Target |
|--------|----|-------------|------------|
| Field coverage | 3 field names | 0 (ignored) | 3 field names |
| Block lifecycle | `finishCurrentBlock()` pattern | Simple append | `finishCurrentBlock()` pattern |
| Interleaving | Full support | No support | Full support |
| Signature | Field name + Anthropic signature | None | Field name + Anthropic signature |
| Redacted thinking | Yes (Anthropic) | No | Yes (Anthropic) |
| Round-trip | Provider-specific compat system | No | Provider-specific compat system |
| Event granularity | start/delta/end for thinking + text | delta only | start/delta/end for thinking + text |

## Main Constraints

- Must support **all target providers**: OpenAI, Anthropic, Google, OpenRouter, OpenCode, Ollama
- Must handle reasoning fragments arriving before, during, and after content (interleaving)
- Must store reasoning in message history for round-trip consistency
- Must emit `EventThinkingStart`, `EventThinkingDelta`, `EventThinkingEnd` for streaming consumers
- Must handle models that don't emit reasoning (no-op)
- Must not break existing providers or existing tests
- Anthropic round-trip requires thinking signature integrity

## Dependencies

- Task 015 (Tool Calling Fix) — completed, provides the base provider code
- `internal/types/provider.go` — needs `EventThinkingStart` and `EventThinkingEnd` added
- `internal/types/message.go` — `BlockThinking` already defined
- `internal/provider/openai_compat.go` — main file to modify
- `internal/provider/anthropic.go` — needs thinking block round-trip support

## Subtasks

- [x] **016.1** — Add `EventThinkingStart` and `EventThinkingEnd` to `types/provider.go`
- [x] **016.2** — Add `Reasoning`, `ReasoningContent`, `ReasoningText` fields to `openAICompatDelta` struct
- [x] **016.3** — Implement block-switching pattern in `parseStreamResponse()`:
  - Add `currentBlock` tracking (thinking/text/toolCall)
  - Add `finishCurrentBlock()` method
  - On reasoning delta: finish current block if not thinking, create/append to thinking block
  - On content delta: finish current block if not text, create/append to text block
  - On tool call delta: finish current block if not toolCall, create/append to toolCall block
  - Emit `EventThinkingStart`, `EventThinkingDelta`, `EventThinkingEnd` appropriately
- [x] **016.4** — Handle multiple reasoning field names (first non-empty wins):
  - Check `reasoning_content`, `reasoning`, `reasoning_text` in order
  - Store the field name as thinking signature for round-trip
- [x] **016.5** — Update `messageToOpenAICompat()` for round-trip:
  - `extractText()` already skips `BlockThinking` — no changes needed
  - OpenAI-compat: do NOT send thinking back (API doesn't support it) — verified
- [x] **016.6** — Anthropic provider: verify thinking block round-trip with signature
- [x] **016.7** — Unit tests (8 new tests):
  - Reasoning-only response (no content text)
  - Reasoning followed by content text
  - Reasoning interleaved with content text (reasoning → content → reasoning → content)
  - Reasoning before tool call
  - Multiple reasoning fragments accumulated into single block
  - Multiple thinking blocks (interleaved reasoning → content → reasoning)
  - No reasoning field present (baseline)
  - Empty reasoning field (no-op)
  - Multiple reasoning fields present (first non-empty wins)
  - `reasoning_content` field (llama.cpp format)
- [x] **016.8** — E2E test with Ollama/gemma4:26b: verify reasoning in session history
- [x] **016.9** — Regression test: verify all existing tests still pass

## Acceptance Criteria

- [x] `parseStreamResponse()` checks `reasoning_content`, `reasoning`, `reasoning_text` fields (first non-empty)
- [x] Block-switching pattern handles reasoning/content/toolCall interleaving
- [x] `EventThinkingStart`, `EventThinkingDelta`, `EventThinkingEnd` emitted correctly
- [x] Reasoning content stored as `BlockThinking` blocks in assistant message
- [x] Multiple thinking blocks supported (interleaved reasoning → content → reasoning)
- [x] thinkingSignature stores the field name for OpenAI-compat round-trip
- [x] `messageToOpenAICompat()` handles thinking blocks correctly per provider
- [x] Anthropic provider round-trips thinking blocks with signature
- [x] Models without reasoning work unchanged (no-op)
- [x] E2E test with Ollama — reasoning visible in session history
- [x] All existing unit tests pass (no regression)
- [x] No breaking changes to other provider implementations
- [x] `go vet`, `go build`, `go test -race` all clean

## Testing & Verification Strategy

**Unit tests** (mocked SSE responses):
- Reasoning-only response (no content text)
- Reasoning followed by content text
- Reasoning interleaved: `reasoning → content → reasoning → content` (2 thinking blocks)
- Reasoning before tool call
- Multiple reasoning fragments → single thinking block
- Multiple reasoning fields: `reasoning_content` + `reasoning` present (only first used)
- `reasoning_content` field only (llama.cpp format)
- No reasoning field (baseline — existing behavior preserved)
- Empty reasoning field (no-op)
- Tool call interleaved with reasoning and content

**E2E tests** (Ollama/gemma4:26b):
- `TestE2E_ReasoningInHistory` — prompt model, verify reasoning appears in session history
- Verify `BlockThinking` blocks are correctly ordered relative to text/tool blocks

**Verification**:
- Reasoning content matches expected accumulated string
- Event sequence is correct: thinking_start → thinking_delta(s) → thinking_end → text_start → ...
- No goroutine leaks or channel issues
- SSE parsing handles malformed reasoning gracefully
- Round-trip: assistant message with thinking blocks converts correctly for each provider

## Design Notes

### Block-Switching Pattern (PI's `finishCurrentBlock`)

```go
type blockType int

const (
    blockNone blockType = iota
    blockThinking
    blockText
    blockToolCall
)

// currentBlock tracks which block type is currently active
var currentBlockType blockType
var currentThinkingBlock *types.ContentBlock

func finishCurrentBlock(ch chan<- types.StreamEvent, msg *types.AgentMessage) {
    switch currentBlockType {
    case blockThinking:
        // thinking block already appended, just emit end event
        sendEvent(ctx, ch, types.StreamEvent{
            Type:    types.EventThinkingEnd,
            Message: msg,
        })
    case blockText:
        // text block already appended
    case blockToolCall:
        // tool call block already appended
    }
    currentBlockType = blockNone
    currentThinkingBlock = nil
}
```

### Reasoning Field Priority

```go
// Check multiple reasoning field names (first non-empty wins)
reasoningFields := []string{"reasoning_content", "reasoning", "reasoning_text"}
var reasoningField, reasoningText string
for _, field := range reasoningFields {
    if val := delta[field]; val != "" {
        reasoningField = field
        reasoningText = val
        break
    }
}
```

### Round-Trip Strategy

| Provider | Outgoing Thinking | Notes |
|----------|-------------------|-------|
| OpenAI-compat | NOT sent | API doesn't accept thinking blocks |
| Anthropic | `{type: "thinking", thinking: "...", signature: "..."}` | Signature required |
| Anthropic (redacted) | `{type: "redacted_thinking", data: "<opaque>"}` | Opaque payload passthrough |
| OpenRouter | Depends on underlying provider | Forward to underlying |

### Why Multiple Field Names?

Different OpenAI-compatible providers use different field names for reasoning tokens:
- **Ollama**: `reasoning`
- **llama.cpp server**: `reasoning_content`
- **chutes.ai**: Both `reasoning_content` AND `reasoning` (duplicate content)
- **Other providers**: May use `reasoning_text`

Using first non-empty avoids duplication while maximizing compatibility.

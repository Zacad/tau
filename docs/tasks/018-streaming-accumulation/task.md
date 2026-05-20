# Task 018: Streaming Block Accumulation — Fix O(n²) Concatenation

## Why

Both OpenAI-compat and Anthropic providers accumulate streaming text and reasoning content using string concatenation (`+=`) in the SSE parsing hot path:

```go
msg.Content[thinkingIndex].Text += reasoningText
```

Additionally, every text delta creates a **new `ContentBlock`** appended to the message:

```go
msg.Content = append(msg.Content, types.ContentBlock{
    Type: types.BlockText,
    Text: delta.Delta.Text,
})
```

Problems:
- **O(n²) memory allocations**: Go strings are immutable. Each `+=` creates a new string, copying all previous content. For a 10,000-token response with ~100 deltas per token, this is ~1M string allocations.
- **Fragmented message history**: A 1,000-token text response produces 1,000 separate `ContentBlock` structs, each with its own `{"type":"text","text":"..."}` JSON wrapper. This inflates session storage by 10-20x compared to a single accumulated block.
- **GC pressure**: Thousands of short-lived string allocations increase GC overhead.
- **Round-trip bloat**: `messageToOpenAICompat()` and `messageToAnthropic()` iterate over hundreds of micro-blocks when serializing follow-up requests.

PI solves this with an **in-place accumulator pattern**: a `currentBlock` pointer accumulates all deltas into a single content block, emitting it once at block boundaries.

## Comparison Analysis: PI vs Our Approach

### PI's Approach
PI maintains a `currentBlock` pointer that references the actual block in `output.content`:

```typescript
let currentBlock: ContentBlock | null = null;

if (!currentBlock || currentBlock.type !== "text") {
    finishCurrentBlock(currentBlock);
    currentBlock = { type: "text", text: "" };
    output.content.push(currentBlock);
    stream.push({ type: "text_start", ... });
}
currentBlock.text += delta;  // Accumulates in-place (JS strings are mutable via reference)
stream.push({ type: "text_delta", ... });
```

- **Single block per content segment**: All text deltas → one `text` block, all thinking deltas → one `thinking` block
- **Event granularity**: `text_start` / `text_delta` / `text_end` for each block
- **Minimal allocations**: One block object + one growing string per content segment

### Our Current Approach
Each delta event creates a new `ContentBlock`:

```go
// Text: new block per delta
msg.Content = append(msg.Content, types.ContentBlock{
    Type: types.BlockText,
    Text: delta.Delta.Text,
})

// Thinking: accumulates in-place via index (correct pattern)
msg.Content[thinkingIndex].Text += reasoningText
```

- **Text**: 1,000 deltas → 1,000 blocks (wrong)
- **Thinking**: 500 deltas → 1 block with O(n²) concatenation (correct structure, wrong algorithm)

### Key Differences

| Aspect | PI | Our Current | Our Target |
|--------|----|-------------|------------|
| Text blocks | 1 per segment | 1 per delta | 1 per segment |
| Thinking blocks | 1 per segment | 1 per segment | 1 per segment |
| Text accumulation | In-place (JS ref) | New block per delta | In-place (`strings.Builder`) |
| Thinking accumulation | In-place | `+=` (O(n²)) | In-place (`strings.Builder`) |
| Block lifecycle events | start/delta/end per type | Only thinking has start/end | start/delta/end for all |
| Session storage size | Minimal | 10-20x bloated | Minimal |

## Main Constraints

- Must not change the `StreamEvent` public API (adding `EventTextStart`/`EventTextEnd` is additive, not breaking)
- Must maintain compatibility with existing session data (sessions with fragmented blocks must still load)
- `strings.Builder` cannot be used directly in `ContentBlock` (not JSON-serializable) — need a streaming accumulator pattern
- Must not break existing tests or E2E behavior

## Dependencies

- Task 016 (Reasoning Support) — current streaming implementation
- Task 017 (Thinking Signature Field) — can be done in parallel or before
- `internal/provider/openai_compat.go` — main file to refactor
- `internal/provider/anthropic.go` — text accumulation to fix

## Subtasks

- [x] **018.1** — Design: Define accumulator types for text and thinking content that use `strings.Builder` internally
- [x] **018.2** — Implement `textAccum` struct with `strings.Builder`, start/delta/end event emission
- [x] **018.3** — Refactor `parseStreamResponse()` in OpenAI-compat: use `textAccum` instead of per-delta block creation
- [x] **018.4** — Refactor `parseStreamResponse()` in Anthropic: use `textAccum` instead of per-delta block creation
- [x] **018.5** — Add `EventTextStart` and `EventTextEnd` to `types/provider.go`
- [x] **018.6** — Replace `+=` thinking concatenation with `strings.Builder`-based accumulation
- [x] **018.7** — Update `collectFromStream()` to track `EventTextDelta` for Complete() (already done, verify)
- [x] **018.8** — Add backward-compat: session loading with fragmented text blocks (coalesce on load) — handled by existing `extractText()`
- [x] **018.9** — Unit tests: verify single block per segment, event lifecycle, string builder correctness
- [x] **018.10** — Regression: all existing tests pass

## Acceptance Criteria

- [x] Text deltas accumulate into a single `ContentBlock` per text segment
- [x] Thinking content uses `strings.Builder` for O(n) accumulation
- [x] `EventTextStart`, `EventTextDelta`, `EventTextEnd` emitted correctly
- [x] `EventThinkingStart`, `EventThinkingDelta`, `EventThinkingEnd` emitted correctly (no regression)
- [x] Session storage size reduced (single block per segment vs per-delta)
- [x] Existing sessions with fragmented blocks load correctly
- [x] All existing unit tests pass (no regression)
- [x] E2E test with Ollama passes
- [x] `go vet`, `go build`, `go test -race` all clean

## Testing & Verification Strategy

**Unit tests:**
- Single text block per text segment (multiple deltas → one block)
- Single thinking block per thinking segment (multiple deltas → one block)
- Text → thinking → text interleaving produces correct block sequence
- `EventTextStart` / `EventTextDelta` / `EventTextEnd` sequence correctness
- Large content (10K+ tokens) — verify no O(n²) allocation pattern
- `collectFromStream()` returns correct accumulated message

**Performance test:**
- Benchmark: 10,000 text deltas — compare allocations before/after
- Benchmark: 5,000 thinking deltas — compare allocations before/after

**Backward compat test:**
- Load session with fragmented text blocks (pre-fix format)
- Verify message content is correctly reconstructed

# Task 019: Anthropic Redacted Thinking Support

## Why

Anthropic can return `redacted_thinking` blocks when content safety filters apply to the model's reasoning. These blocks contain encrypted reasoning content that the model generated but was flagged for safety reasons.

Without redacted thinking support:
- `content_block_start` for `redacted_thinking` is silently ignored
- The encrypted payload (`data` field) is lost
- Session history is incomplete — missing safety-filtered reasoning context
- Round-trip fails — redacted thinking blocks are not re-sent to Anthropic in follow-up requests

PI handles `redacted_thinking` by storing it as `[Reasoning redacted]` text with the encrypted payload preserved in the `thinkingSignature` field for round-trip.

## Comparison Analysis: PI vs Our Approach

### PI's Approach

```typescript
case "redacted_thinking":
    const block = {
        type: "thinking",
        thinking: "[Reasoning redacted]",
        thinkingSignature: event.content_block.data,  // encrypted payload
        isRedacted: true,
        index: event.index,
    };
    output.content.push(block);
    stream.push({ type: "thinking_start", contentIndex: ..., partial: output });
```

Round-trip:
```json
{"type": "redacted_thinking", "data": "<encrypted payload>"}
```

### Key Design Elements
1. **Stored as `thinking` type** with `"[Reasoning redacted]"` as visible text
2. **Encrypted payload stored in `thinkingSignature`** — preserved for round-trip
3. **`isRedacted` flag** — distinguishes from normal thinking blocks
4. **Round-trip emits `redacted_thinking` block type** — sends the encrypted payload back

### Our Current State
- `content_block_start` for `redacted_thinking` falls through silently
- No handling for `redacted_thinking` in `messageToAnthropic()` round-trip
- No `isRedacted` flag on `ContentBlock`

## Main Constraints

- Must store the encrypted payload for round-trip integrity (Anthropic requires it)
- Must display something meaningful to the user (e.g., "[Reasoning redacted]")
- Must not confuse redacted thinking with normal thinking in session history
- Must handle round-trip correctly: emit `{"type": "redacted_thinking", "data": "..."}` not `{"type": "thinking", ...}`

## Dependencies

- Task 017 (Thinking Signature Field) — adds `Signature` field to `ContentBlock`; this task should follow
- `internal/types/message.go` — may need `isRedacted` flag on `ContentBlock` or reuse `BlockThinking`
- `internal/provider/anthropic.go` — add `redacted_thinking` handling

## Subtasks

- [ ] **019.1** — Design: Decide whether to add `isRedacted` flag to `ContentBlock` or use a separate `BlockRedactedThinking` type
- [ ] **019.2** — Add `redacted_thinking` handling in Anthropic `content_block_start`
- [ ] **019.3** — Store encrypted payload in `thinkingSignature` (or new field) for round-trip
- [ ] **019.4** — Emit `EventThinkingStart` with visible text "[Reasoning redacted]"
- [ ] **019.5** — Update `messageToAnthropic()` to emit `redacted_thinking` block type on round-trip
- [ ] **019.6** — Unit tests: redacted thinking block creation, round-trip serialization
- [ ] **019.7** — Regression: all existing tests pass

## Acceptance Criteria

- [ ] `content_block_start` with `redacted_thinking` type creates a thinking block
- [ ] Encrypted payload stored for round-trip
- [ ] Visible text is "[Reasoning redacted]"
- [ ] `messageToAnthropic()` produces `{"type": "redacted_thinking", "data": "..."}` for round-trip
- [ ] All existing tests pass (no regression)
- [ ] `go vet`, `go build`, `go test -race` all clean

## Testing & Verification Strategy

**Unit tests:**
- `content_block_start` with `redacted_thinking` creates correct block
- Encrypted payload preserved in session storage
- `messageToAnthropic()` produces correct `redacted_thinking` JSON on round-trip
- Mixed normal + redacted thinking blocks in same message

**Edge cases:**
- `redacted_thinking` without `data` field (malformed response)
- Multiple redacted thinking blocks in same response

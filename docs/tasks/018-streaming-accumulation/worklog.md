# Task 018 Worklog

## Summary
Fixed O(n²) string concatenation in streaming accumulation and fragmented text block creation. Implemented in-place `strings.Builder` accumulators for both text and thinking content.

## Changes Made

### 1. Added new event types (`internal/types/provider.go`)
- `EventTextStart` — emitted when first text delta arrives
- `EventTextEnd` — emitted when text block is finalized

### 2. Implemented accumulator types (`internal/provider/openai_compat.go`)
- `textAccum` — accumulates text deltas into single `ContentBlock` using `strings.Builder`
  - `start()` — creates new text block, emits `EventTextStart`
  - `write()` — appends delta to builder, emits `EventTextDelta`
  - `finish()` — finalizes builder to string, emits `EventTextEnd`
- `thinkingAccum` — same pattern for thinking/reasoning content
  - Tracks `signature` (Anthropic) and `fieldUsed` (OpenAI-compat field switching)
  - Auto-starts on first write (backward compat with tests missing `content_block_start`)

### 3. Refactored `OpenAICompatProvider.parseStreamResponse()`
- Replaced per-delta `ContentBlock` creation with `textAccum`
- Replaced `+=` thinking concatenation with `thinkingAccum` (strings.Builder)
- Added `textAcc.finish()` when reasoning starts (interleaving support)
- Added `textAcc.finish()` and `thinkAcc.finish()` on tool call arrival

### 4. Refactored `AnthropicProvider.parseStreamResponse()`
- Replaced per-delta `ContentBlock` creation with `textAccum`
- Replaced `+=` thinking concatenation with `thinkingAccum`
- `content_block_stop` triggers both accumulator finishes
- Signature stored in `thinkingAccum.signature`, appended on finish

### 5. Updated `collectFromStream()` in both providers
- Added `EventTextStart`, `EventTextEnd` to message tracking cases

### 6. Added unit tests (`internal/provider/openai_compat_test.go`)
- `TestParseStreamResponse_SingleTextBlock_MultipleDeltas` — 5 deltas → 1 block
- `TestParseStreamResponse_SingleThinkingBlock_MultipleDeltas` — 3 deltas → 1 block
- `TestParseStreamResponse_InterleavedTextThinking_TextAccumulates` — correct block ordering
- `TestParseStreamResponse_LargeContent_NoQuadraticBehavior` — 1000 deltas → 1 block

## Verification
- All existing tests pass (no regression)
- New accumulation tests pass
- `go vet` clean
- `go build` clean
- `go test -race ./internal/provider/...` clean
- Binary rebuilt successfully

## Backward Compatibility
- Existing sessions with fragmented text blocks load correctly — `extractText()` already concatenates all `BlockText` blocks
- No migration needed
- New sessions have single consolidated blocks per segment

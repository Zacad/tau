# Task 031: TUI Performance Fix - Streaming Lag

## Why

When the model is generating a response, the TUI has significant lag:
- Ctrl+C to abort takes several seconds to execute
- Scrolling rewinds ~1 line per second
- Spinner blinks much faster than normal (indicates event loop overload)

Root cause: every text delta event triggers a full viewport re-render that re-renders ALL blocks from scratch. During streaming this is O(n) per event where n = number of blocks, and events arrive dozens per second.

## Comparison with PI

PI uses a similar bubbletea architecture but typically throttles viewport updates and caches rendered content to avoid this O(n) per-delta cost.

## Constraints

- Must maintain visual fidelity - no flickering or visual glitches
- Must not break existing streaming behavior (message queuing, slash commands, etc.)
- Must work with any conversation length
- Throttle interval should balance responsiveness with performance (~30fps target)

## Implementation Plan

### 1. Viewport Update Throttling
- Add `lastViewportUpdate time.Time` field to Model
- Add `viewportUpdateInterval` constant (~33ms = ~30fps)
- Skip `updateViewport()` if less than interval has elapsed since last update
- Ensure final viewport update always happens (on MessageEnd, TurnEnd, PromptDone)

### 2. Finalized Block Render Caching
- Add `renderedCache string` and `renderedCacheValid bool` to Model
- Cache rendered string of finalized blocks
- During streaming, only re-render pending block
- Invalidate cache on: resize, new blocks, tool status changes, errors

## Testing Strategy

- Unit test: verify throttle prevents updates within interval
- Unit test: verify final update always happens on turn end
- Unit test: verify cache is used when valid
- Unit test: verify cache is invalidated on resize, new blocks, errors
- Unit test: verify viewport content is identical with/without cache
- Integration: existing TUI tests should pass unchanged

## Acceptance Criteria

- [x] Viewport updates throttled to ~30fps during streaming
- [x] Ctrl+C abort responds within ~100ms during streaming
- [x] Scrolling is responsive during streaming
- [x] Spinner runs at normal speed during streaming
- [x] Finalized block render cache is used during streaming
- [x] Cache is correctly invalidated on all state changes
- [x] All existing tests pass
- [x] No visual regressions (content renders identically)
- [x] go vet/build clean

## Worklog

### 2026-05-08
- Analyzed root cause: O(n) per text delta due to full viewport re-render
- Designed throttle + cache approach
- Created task definition
- Added `viewportUpdateInterval` constant (33ms) and `lastViewportUpdate` field to Model
- Added `renderedCache` and `renderedCacheValid` fields to Model
- Implemented `updateViewportWithForce(bool)` with throttle logic
- Implemented `invalidateRenderedCache()` and cache population in `updateViewportWithForce`
- Added cache invalidation to: `resetForTurn`, `flushPending`, `processEvent` (tool exec end, tool result, subagent start/end), `handlePromptDone`, `handleError`, resize handler, queued message handler, slash command handlers (idle + streaming), model selector
- Added `updateViewportWithForce(true)` call in `PromptDoneMsg` handler for final update
- Wrote 9 new unit tests for throttling and caching behavior
- All 100+ existing tests pass, go vet/build clean
- Updated TRACKING.md and DECISIONS.md

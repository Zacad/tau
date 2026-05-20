# Task 029: Streaming Markdown Rendering — Worklog

## Exploration Phase

### Research Conducted
- Analyzed how opencode and claude-code handle streaming markdown (debounced full re-rendering)
- Benchmarked glamour performance at various content sizes:
  - Typical (~200 tokens): ~1.2ms, 596KB allocations
  - Long (~2K tokens): ~3.7ms, 1.8MB allocations
- Measured renderer creation overhead: ~830µs per call
- Tested glamour behavior with partial/incomplete markdown (all edge cases handled gracefully)
- Evaluated 4 approaches: debounced full re-render, line-by-line, hybrid, custom renderer

### Findings Document
- Created `findings.md` with comprehensive analysis
- Recommended: Debounced Full Re-rendering at 200ms interval

## Implementation Phase

### Changes Made

#### 1. `internal/tui/markdown.go`
- Added `NewRenderer(width int)` function to create reusable glamour renderer
- Added `RenderWithRenderer(r, text, originalMarkdown)` function for rendering with reused instance
- Saves ~830µs per call by avoiding renderer recreation

#### 2. `internal/tui/model.go`
- Added debounced render state fields to Model struct:
  - `pendingRendered string` — cached glamour output
  - `pendingRenderedLen int` — content length at last render (change detection)
  - `lastRenderTime time.Time` — timestamp of last render
  - `glamourRenderer *glamour.TermRenderer` — reused renderer instance
- Added `renderDebounceInterval` constant (200ms)
- Updated `NewModel()` to initialize glamourRenderer
- Updated `resetForTurn()` to clear render cache
- Updated `updateViewport()` to use cached rendered output when available
- Updated `renderPendingBlock()` to accept optional renderedMarkdown parameter
- Added `renderPendingMarkdown()` method for debounced re-rendering

#### 3. `internal/tui/update.go`
- Added `time` import
- Updated `TuiTickMsg` handler to trigger `renderPendingMarkdown()` during streaming
- Updated `AgentEventMsg` handler to start debounce timer via `tea.Every()`
- Updated `tea.WindowSizeMsg` handler to invalidate pending render cache on resize

#### 4. Tests
- Added 16 new unit tests for debounced rendering:
  - `TestRenderPendingBlock_UsesCachedRendered`
  - `TestRenderPendingBlock_FallsBackToPlainText`
  - `TestRenderPendingBlock_ThinkingIgnoresCachedRendered`
  - `TestModel_RenderPendingMarkdown_NoOpForThinking`
  - `TestModel_RenderPendingMarkdown_NoOpForEmptyBuilder`
  - `TestModel_RenderPendingMarkdown_RendersAssistantText`
  - `TestModel_RenderPendingMarkdown_SkipsIfUnchanged`
  - `TestModel_RenderPendingMarkdown_UpdatesOnNewContent`
  - `TestModel_ResetForTurn_ClearsRenderCache`
  - `TestModel_UpdateViewport_UsesCachedRendered`
  - `TestModel_UpdateViewport_FallsBackToPlainText`
  - `TestNewRenderer_CreatesReusableRenderer`
  - `TestRenderWithRenderer_EmptyInput`
  - `TestResize_InvalidatesPendingRenderCache`
- Updated `newTestModel()` to initialize glamourRenderer
- Updated existing `renderPendingBlock` test call to include new parameter

### Verification
- All tests pass (go test ./...)
- go vet clean
- go build clean
- Binary rebuilt at `./tau`

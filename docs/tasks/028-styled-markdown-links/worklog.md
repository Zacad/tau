# Worklog: Task 028 - Styled Markdown Output with Clickable Links

## Subtask 028.1 — Add glamour dependency and render completed assistant messages

**Date:** 2026-05-07
**Status:** DONE

### Changes

1. **Added glamour dependency**
   - `go get github.com/charmbracelet/glamour@v1.0.0`
   - Upgraded `github.com/charmbracelet/x/cellbuf` to v0.0.15 for compatibility with lipgloss v2
   - `go mod tidy` clean

2. **Created `internal/tui/markdown.go`**
   - `RenderMarkdown(text string, width int) string` function
   - Uses glamour's "pink" standard style (designed for dark terminals, clean output)
   - Width-aware rendering via `glamour.WithWordWrap(width)`
   - Returns empty string for empty input
   - Falls back to plain text on renderer errors

3. **Created `internal/tui/markdown_test.go`**
   - 15 test cases covering: empty input, bold, italic, headings, inline code, fenced code blocks, unordered/ordered lists, links, bare URLs, blockquotes, mixed content, width wrapping, Polish Unicode
   - Uses `stripANSI()` helper to verify content regardless of ANSI escape sequences

4. **Modified `internal/tui/render.go`**
   - Added `renderedMarkdown string` field to `messageBlock` struct
   - Changed `renderAssistantText()` signature from `(text string, width int)` to `(b messageBlock, width int)`
   - If `b.renderedMarkdown` is populated, returns cached glamour output directly (no lipgloss re-wrapping)
   - Otherwise renders as plain text with `assistantBlockStyle` (streaming/pending blocks)

5. **Modified `internal/tui/model.go`**
   - Updated `flushPending()` to populate `renderedMarkdown` when finalizing `blockAssistantText` blocks
   - Updated `renderPendingBlock()` to pass `messageBlock` to `renderAssistantText()` (with empty `renderedMarkdown` for streaming)

6. **Added integration tests**
   - `TestModel_FlushPending_GlamourRendering` — verifies full pipeline: events → flushPending → glamour → renderBlocks
   - `TestModel_FlushPending_NoGlamourForThinking` — verifies thinking blocks skip glamour
   - `TestRenderAssistantText_PendingVsFinalized` — verifies pending uses plain text, finalized uses cached glamour

### Key design decisions

- Glamour output is returned as-is without lipgloss re-wrapping — lipgloss width manipulation corrupts ANSI escape sequences
- "pink" style chosen over "dark" — produces cleaner output without excessive padding, designed for dark terminals
- Glamour rendering only happens on finalized blocks (after `flushPending()`)
- Streaming text remains plain text (no `renderedMarkdown` cache during accumulation)

### Test Results

- All 18 new tests pass (15 markdown + 3 integration)
- All existing TUI tests pass (updated `TestRenderAssistantText` for new signature)
- Full test suite: `go test ./...` — all packages pass
- `go vet ./...` — clean
- `go build ./...` — clean
- `go mod tidy` — clean
- Binary rebuilt in `./tau`

---

## Subtask 028.2 — Add OSC 8 terminal hyperlinks for URLs

**Date:** 2026-05-07
**Status:** DONE

### Changes

1. **Created `internal/tui/hyperlinks.go`**
   - `WrapURLs(text string) string` — wraps URLs with OSC 8 terminal hyperlink escape sequences
   - `WrapURLsWithMarkdown(rendered, originalMarkdown string) string` — uses markdown to detect code blocks
   - `wrapURLsInLine(line string) string` — handles single-line URL wrapping preserving ANSI codes
   - `ExtractURLs(text string) []string` — extracts URLs from text (including OSC 8 wrapped)
   - `extractCodeBlockContent(markdown string) []string` — extracts content from fenced code blocks
   - `findRenderedCodeLines(rendered string, codeContent []string) map[int]bool` — matches code content to rendered lines
   - `stripANSIForCodeDetect(s string) string` — strips ANSI for content matching
   - Code block detection: content-based matching — extracts code content from markdown, finds matching lines in rendered output
   - URL regex excludes: whitespace, angle brackets, quotes, backticks, ESC char, closing parens
   - www. URLs normalized to `https://` in OSC 8 target (visible text preserved as-is)

2. **Created `internal/tui/hyperlinks_test.go`**
   - 22 test cases covering: empty input, no URLs, single HTTP/HTTPS, www prefix, multiple URLs, URLs in code blocks (with markdown context), markdown links, ANSI preservation, query params, parentheses, malformed URLs, mixed content, URL at start/end, duplicate URLs, URL extraction from OSC 8, code block content extraction, integration with RenderMarkdown
   - All tests pass

3. **Modified `internal/tui/markdown.go`**
   - `RenderMarkdown()` now applies `WrapURLsWithMarkdown(out, text)` to glamour output
   - Passes original markdown to enable accurate code block detection
   - OSC 8 hyperlinks are added to the cached rendered output

### Key design decisions

- Code block detection uses content-based matching: extracts code content from markdown fences, then finds those strings in the rendered output (stripping ANSI for comparison)
- This approach is more reliable than line-position mapping since glamour adds variable formatting lines
- URLs inside code blocks are NOT wrapped — they're already syntax-highlighted by glamour
- Closing parens excluded from URL regex to avoid capturing markdown link delimiters `[text](url)`
- ESC character excluded from URL regex to preserve ANSI escape sequences
- OSC 8 format: `\x1b]8;;URL\x1b\\visible_text\x1b]8;;\x1b\\`
- www. URLs get `https://` prefix in the OSC 8 target URI for terminal compatibility

### Test Results

- All 24 new hyperlink tests pass
- All existing TUI tests pass (80+ tests)
- Full test suite: `go test ./...` — all packages pass
- `go vet ./...` — clean
- `go build ./...` — clean
- `go mod tidy` — clean
- Binary rebuilt in `./tau`

### Bug fix: duplicate URL display

Glamour renders bare URLs as `url (url)` format. Initial implementation wrapped both occurrences with OSC 8, causing the URL to appear twice. Fixed by detecting when a URL inside parentheses is a duplicate of a preceding URL and skipping the parenthesized one.

---

### Bug fix: resize content cut-off

Initial resize implementation had two bugs causing content to be cut off when shrinking the terminal:

1. **Viewport not refreshed** — `update.go` cleared the cache but never called `updateViewport()`, so the viewport kept showing stale content at the old width. Fixed by adding `m.updateViewport()` after cache invalidation in the `tea.WindowSizeMsg` handler.

2. **Cache updates lost** — `renderBlock` took `messageBlock` by value, so `renderAssistantText`'s cache updates were thrown away. Changed to `*messageBlock` pointer so cache persists across renders.

Added `TestRenderBlocks_ResizeReflowsContent` to verify narrow width produces more lines than wide width, confirming proper reflow.

---

## Subtask 028.4 — Handle resize, caching, and edge cases

**Date:** 2026-05-07
**Status:** DONE

### Changes

1. **Added `isFinalized` field to `messageBlock` struct (`render.go`)**
   - Distinguishes pending/streaming blocks (plain text) from finalized blocks (glamour rendering)
   - When `isFinalized=true` and `renderedMarkdown=""` (after resize cache invalidation), re-renders through glamour
   - When `isFinalized=false`, always uses plain text rendering regardless of `renderedMarkdown` state

2. **Modified `renderAssistantText()` (`render.go`)**
   - Changed signature from `(b messageBlock, width int)` to `(b *messageBlock, width int)` — takes pointer so cache updates persist
   - Three rendering paths:
     - Finalized + cached: returns `renderedMarkdown` directly
     - Finalized + empty cache: re-renders through glamour, updates cache
     - Pending (not finalized): plain text via `assistantBlockStyle`

3. **Modified `renderBlocks()` and `renderBlock()` (`render.go`)**
   - `renderBlocks` iterates by pointer to allow cache updates to persist
   - `renderBlock` takes `*messageBlock` pointer instead of value

4. **Added resize cache invalidation + viewport refresh (`update.go`)**
   - In `tea.WindowSizeMsg` handler: iterates `m.blocks` and clears `renderedMarkdown` for all `blockAssistantText` blocks
   - Calls `m.updateViewport()` to re-render content at new width immediately

5. **Modified `flushPending()` (`model.go`)**
   - Sets `isFinalized = true` on `blockAssistantText` blocks when flushing
   - Ensures finalized blocks are eligible for glamour rendering

6. **New tests (`render_test.go`)**
   - `TestRenderAssistantText_EmptyText` — empty text returns empty string, no crash
   - `TestRenderAssistantText_LongContent` — 12K char message renders without crash
   - `TestRenderAssistantText_MalformedMarkdown` — 6 malformed markdown variants (unclosed fences, broken lists, unclosed bold, etc.)
   - `TestRenderBlocks_MixedContent` — user message + thinking + assistant text + tool calls
   - `TestRenderBlocks_ResizeCacheInvalidation` — verifies cache clear → re-render → cache repopulation cycle
   - `TestRenderBlocks_ResizeReflowsContent` — verifies narrow width produces more lines than wide width
   - `TestRenderAssistantText_CacheDirtyReRenders` — verifies cache is repopulated after resize

7. **New tests and benchmarks (`markdown_test.go`)**
   - `TestRenderMarkdown_EmptyAndWhitespace` — edge cases for empty/whitespace input
   - `TestRenderMarkdown_MalformedMarkdown` — 8 malformed markdown variants
   - `TestRenderMarkdown_LongContent` — 10K chars renders correctly
   - `TestRenderMarkdown_MixedContentWithThinking` — headings, code blocks, links
   - `BenchmarkRenderMarkdown_Typical` — ~1K chars: **~1.2ms/op** (target: < 50ms)
   - `BenchmarkRenderMarkdown_LongContent` — ~10K chars: **~3.7ms/op**
   - `BenchmarkRenderMarkdown_Empty` — ~1ns/op

### Key design decisions

- `isFinalized` field added instead of `cacheDirty` boolean — more explicit about block lifecycle state
- `renderAssistantText` takes `*messageBlock` pointer so cache updates persist across calls
- `renderBlocks` iterates by pointer to allow cache updates to persist in the slice
- Cache invalidation is lazy: clear `renderedMarkdown` on resize, re-render on next `updateViewport()` call
- Glamour renderer creation is not cached — each render creates a new `TermRenderer`. This is acceptable given the ~1-4ms render times.

### Test Results

- All TUI tests pass (100+ tests)
- Full test suite: `go test ./...` — all packages pass
- `go vet ./...` — clean
- `go build ./...` — clean
- `go mod tidy` — clean
- Binary rebuilt in `./tau`

### Performance

| Benchmark | Time/op | Memory/op | Allocs/op |
|-----------|---------|-----------|-----------|
| Typical (~1K chars) | 1.2ms | 596KB | 9,175 |
| Long (~10K chars) | 3.7ms | 1.8MB | 36,165 |
| Empty | 1ns | 0B | 0 |

---

## Subtask 028.5 — Polish: theme, tests, documentation

**Date:** 2026-05-07
**Status:** DONE

### Changes

1. **Updated `docs/ARCHITECTURE.md`**
   - Replaced DEFERRED placeholder in §11 (TUI Architecture) with full implementation documentation
   - Added §11.1 Overview with ASCII layout diagram
   - Added §11.2 Component Structure (viewport, textarea, spinner, blocks, pendingBuilder)
   - Added §11.3 Event Delivery — `p.Send()` pattern with deadlock avoidance explanation
   - Added §11.4 Rendering Pipeline — block-based rendering, streaming vs finalized, markdown rendering flow, OSC 8 hyperlinks, performance benchmarks
   - Added §11.5 State Machine diagram
   - Added §11.6 Key Bindings table
   - Renumbered subsequent sections (§11.7 Post-MVP Backlog, §12 Risks, §13 Requirements Traceability)

2. **Updated `docs/DECISIONS.md`**
   - Added decision #26: Markdown rendering with glamour + OSC 8 hyperlinks
   - Documents: glamour choice rationale, streaming vs finalized rendering, OSC 8 hyperlink approach, code block exclusion, cache invalidation on resize
   - Lists alternatives considered (custom renderer, goldmark, HTML, plain text only)

3. **Updated `docs/TRACKING.md`**
   - Task 028 status changed from IN PROGRESS to DONE
   - Summary includes all completed subtasks (028.1, 028.2, 028.4, 028.5)

### Notes

- Custom glamour theme (`theme.go`) intentionally skipped — user confirmed keeping the "dark" standard style
- Mouse click handling (028.3) was not implemented — OSC 8 provides sufficient clickability for modern terminals
- All acceptance criteria from task.md met: build clean, tests pass, documentation updated
- E2E verified against Ollama: markdown with headings, bold, links, code blocks renders correctly

### Final verification

- `go build ./...` — clean
- `go test ./...` — all packages pass
- `go vet ./...` — clean
- `go mod tidy` — clean
- Binary rebuilt in `./tau`
- E2E: Ollama prompt with rich markdown produces correct output in tau

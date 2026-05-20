# Plan: Task 028 Subtask 4 — Resize, Caching, and Edge Cases

## Current State

- **028.1** (glamour rendering): DONE — `markdown.go`, `renderedMarkdown` cache in `messageBlock`, glamour on finalized blocks
- **028.2** (OSC 8 hyperlinks): DONE — `hyperlinks.go`, URL wrapping with code block detection
- **028.3** (mouse clicks): NOT DONE — skipped per user confirmation
- **028.4** (resize + edge cases): IN PROGRESS — this plan

## Problem

The current `renderAssistantText()` in `render.go` has a bug: when the terminal is resized, the cached `renderedMarkdown` is NOT invalidated, so the old width rendering persists. Additionally, there are no tests for edge cases (empty text, long content, malformed markdown, mixed content) and no performance benchmarks.

## Changes

### 1. Fix `renderAssistantText` in `internal/tui/render.go`

**Current code:**
```go
func renderAssistantText(b messageBlock, width int) string {
    if b.text == "" { return "" }
    if b.renderedMarkdown != "" {
        return b.renderedMarkdown
    }
    return assistantBlockStyle.Width(width).Render(b.text)
}
```

**Problem:** When cache is dirty (resize), clearing `renderedMarkdown` causes fallback to plain text instead of re-rendering through glamour.

**New code:**
```go
func renderAssistantText(b messageBlock, width int, cacheDirty bool) string {
    if b.text == "" { return "" }
    if b.renderedMarkdown != "" && !cacheDirty {
        return b.renderedMarkdown
    }
    // Re-render through glamour (first time or cache invalidated)
    rendered := RenderMarkdown(b.text, width)
    if rendered != "" {
        b.renderedMarkdown = rendered
    }
    if rendered != "" {
        return rendered
    }
    return assistantBlockStyle.Width(width).Render(b.text)
}
```

Key change: Added `cacheDirty` parameter. When true, re-renders through glamour and updates the cache. Falls back to plain text only if glamour fails.

### 2. Add resize cache invalidation in `internal/tui/update.go`

**Current code (line ~14):**
```go
case tea.WindowSizeMsg:
    m.width = msg.Width
    m.height = msg.Height
    m.resize(msg.Width, msg.Height)
    m.viewport, _ = m.viewport.Update(msg)
    m.input, _ = m.input.Update(msg)
    return m, nil
```

**New code:**
```go
case tea.WindowSizeMsg:
    m.width = msg.Width
    m.height = msg.Height
    // Invalidate glamour cache on resize so blocks re-render at new width
    for i := range m.blocks {
        if m.blocks[i].kind == blockAssistantText {
            m.blocks[i].renderedMarkdown = ""
        }
    }
    m.resize(msg.Width, msg.Height)
    m.viewport, _ = m.viewport.Update(msg)
    m.input, _ = m.input.Update(msg)
    return m, nil
```

### 3. Update callers of `renderAssistantText`

**`renderBlock` in `render.go`** (line ~81):
```go
case blockAssistantText:
    return renderAssistantText(b, width, false)
```

**`renderPendingBlock` in `model.go`** (line ~223):
```go
case blockAssistantText:
    return renderAssistantText(messageBlock{text: text}, width, false)
```

### 4. New tests in `internal/tui/render_test.go`

Add the following test functions:

- `TestRenderAssistantText_EmptyText` — verifies empty text returns empty string, no crash
- `TestRenderAssistantText_LongContent` — 10K+ char message renders without crash
- `TestRenderAssistantText_MalformedMarkdown` — unclosed code blocks, broken lists render without crash
- `TestRenderAssistantText_MixedContent` — markdown with thinking blocks, tool calls interleaved
- `TestRenderBlocks_ResizeCacheInvalidation` — verifies that after clearing `renderedMarkdown`, re-render produces new output at different width
- `TestRenderMarkdown_MalformedMarkdown` — glamour handles unclosed fences, broken lists, etc.
- `TestRenderMarkdown_LongContent` — 10K+ chars renders without crash
- `TestRenderMarkdown_EmptyAndWhitespace` — edge cases

### 5. Performance benchmark in `internal/tui/markdown_test.go`

```go
func BenchmarkRenderMarkdown_Typical(b *testing.B) {
    // ~1K chars of typical markdown
    input := "# Heading\n\nSome **bold** and *italic* text with a [link](https://example.com).\n\n- item 1\n- item 2\n- item 3\n\n```go\nfunc main() { fmt.Println(\"hello\") }\n```\n\n> A blockquote with some text.\n\n1. first\n2. second\n3. third"
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        RenderMarkdown(input, 80)
    }
    if b.ElapsedPerOp() > 50*time.Millisecond {
        b.Errorf("render took %v, expected < 50ms", b.ElapsedPerOp())
    }
}

func BenchmarkRenderMarkdown_LongContent(b *testing.B) {
    input := strings.Repeat("Some paragraph text with **bold** and *italic*. ", 200) // ~10K chars
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        RenderMarkdown(input, 80)
    }
}
```

### 6. Update `docs/tasks/028-styled-markdown-links/worklog.md`

Append a new section documenting the changes made for 028.4.

## Files Modified

| File | Change |
|------|--------|
| `internal/tui/render.go` | Add `cacheDirty` param to `renderAssistantText`, update `renderBlock` caller |
| `internal/tui/model.go` | Update `renderPendingBlock` caller, add cache invalidation in resize handler |
| `internal/tui/update.go` | Add cache invalidation loop in `tea.WindowSizeMsg` case |
| `internal/tui/render_test.go` | Add edge case + resize tests |
| `internal/tui/markdown_test.go` | Add malformed markdown, long content, and performance benchmark tests |
| `docs/tasks/028-styled-markdown-links/worklog.md` | Document 028.4 changes |

## Acceptance Criteria

- [ ] Terminal resize triggers re-render of all assistant messages with new width
- [ ] No visual artifacts or corrupted output after resize
- [ ] Empty assistant text doesn't crash or produce empty glamour output
- [ ] Very long messages (10K+ chars) render correctly and viewport scrolls
- [ ] Malformed markdown (unclosed code blocks, broken lists) renders without crashing
- [ ] Mixed content (text + thinking + tool calls) renders correctly
- [ ] Performance: render time < 50ms for typical message (< 2K chars)
- [ ] `go test ./internal/tui/...` passes
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` clean
- [ ] `go mod tidy` clean

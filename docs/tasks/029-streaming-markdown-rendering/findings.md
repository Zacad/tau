# Task 029: Streaming Markdown Rendering — Findings

## 1. Approach Evaluation

### 1.1 Debounced Full Re-rendering (Industry Standard)

**How it works:** Accumulate tokens in `pendingBuilder`, call `RenderMarkdown()` every 100-200ms, cache the output, swap in the rendered result.

**Performance data (from benchmarks):**

| Content Size | Render Time | Allocations | Memory |
|---|---|---|---|
| Typical (~200 tokens) | ~1.2ms | 9,175 | ~596 KB |
| Long (~2K tokens) | ~3.7ms | 36,165 | ~1.8 MB |
| 5K tokens (est.) | ~9ms | ~90K | ~4.5 MB |
| 10K tokens (est.) | ~18ms | ~180K | ~9 MB |

**Renderer creation overhead:** ~830µs per call (new vs reused renderer). Reusing the renderer saves ~15% but requires managing lifecycle.

**Pros:**
- Matches industry standard (opencode, claude-code)
- Correct output — glamour handles all markdown properly
- Simple to implement — just add a timer + re-render call
- No new dependencies
- Works with existing OSC 8 hyperlink pipeline

**Cons:**
- High allocation count (~9K-36K per render) — GC pressure at 150ms interval
- Memory churn: ~600KB-1.8MB per render for typical content
- Full re-parse every interval — redundant work on already-rendered content
- At 150ms interval with 3.7ms render time for 2K tokens: ~2.5% CPU during streaming

**Verdict:** Viable, but GC pressure is the main concern. The render time itself is well within budget (<4ms for 2K tokens), but the allocation volume at 150ms intervals will trigger frequent GC cycles.

---

### 1.2 Line-by-Line with Sanitization

**How it works:** Only render "complete" lines/blocks. Buffer incomplete lines until they form complete markdown constructs. Detect block boundaries (code fences, list items, headings).

**Pros:**
- Lower allocation volume — only new content gets rendered
- Less GC pressure
- Could be fast enough for real-time

**Cons:**
- Extremely complex to implement correctly
- Markdown is not line-oriented — block boundaries span multiple lines (code fences, tables, blockquotes, lists)
- Glamour doesn't support partial rendering — would need to feed complete blocks only
- Edge cases: nested structures, interrupted blocks, mixed content
- High maintenance burden — fragile, hard to test comprehensively
- Would essentially require building a markdown block parser

**Verdict:** Not viable. The complexity-to-benefit ratio is poor. Markdown's block-level structure makes line-by-line rendering fundamentally fragile.

---

### 1.3 Hybrid: Fast Path + Final Render

**How it works:** During streaming, apply simple lipgloss styling (bold via `lipgloss.NewStyle().Bold(true)`, inline code, etc.) to the raw text. On completion, swap for full glamour render.

**Pros:**
- Very low overhead during streaming — lipgloss styling is cheap
- No GC pressure from glamour during streaming
- Final output is correct (full glamour on completion)
- Users get *some* visual improvement during streaming

**Cons:**
- Visual jump when swapping from simple style to glamour — jarring UX
- Only handles inline elements (bold, italic, code) — no headings, lists, code blocks, tables during streaming
- Inconsistent experience — user sees different formatting during vs after streaming
- Complex to implement the swap without viewport scroll jump
- Still need glamour on completion (no performance gain overall)

**Verdict:** Marginal improvement over current state. The visual jump on swap is a significant UX problem. Not recommended as a primary approach.

---

### 1.4 Custom Incremental Renderer

**How it works:** Build a custom markdown tokenizer + ANSI emitter. Maintain state about current block context. Emit ANSI codes as tokens arrive.

**Pros:**
- True incremental — minimal work per token
- Lowest possible latency
- Full control over output

**Cons:**
- Massive implementation effort — essentially building a markdown renderer from scratch
- Would need to replicate glamour's styling (colors, padding, code highlighting via chroma)
- Chroma integration for syntax highlighting is non-trivial
- High maintenance burden
- Duplicates functionality that glamour already provides
- Risk of inconsistencies between streaming and final render

**Verdict:** Not viable. The effort is disproportionate to the benefit. Glamour + goldmark + chroma is a sophisticated stack that would take months to replicate.

---

## 2. Specific Questions Answered

### 2.1 Glamour debouncing feasibility

**Yes, viable with caveats.** Render times are well under 150ms for all realistic response sizes:
- 2K tokens: ~3.7ms (2.5% of 150ms interval)
- 5K tokens: ~9ms (6% of interval)
- 10K tokens: ~18ms (12% of interval)

**The real concern is GC pressure**, not CPU. Each render allocates ~600KB-1.8MB. At 150ms intervals, that's 4-12 MB/s of allocation rate during streaming. Go's GC handles this, but it will cause periodic GC pauses.

**Mitigation:** Reuse the glamour renderer instance (saves ~830µs + some allocations per call). Use a longer debounce interval (200ms) for large responses.

### 2.2 Viewport stability

The current `updateViewport()` already handles scroll position:
```go
wasAtBottom := m.viewport.AtBottom()
oldYOffset := m.viewport.YOffset()
m.viewport.SetContent(content)
if wasAtBottom {
    m.viewport.GotoBottom()
} else {
    m.viewport.SetYOffset(oldYOffset)
}
```

**Key finding:** When re-rendering, the content height can change (glamour adds padding, formatting). If the user is scrolled to bottom, `GotoBottom()` corrects any jump. If the user scrolled up, the `YOffset` preservation works but content above the viewport may shift.

**Risk:** During debounced re-render, the rendered content height changes as more markdown is parsed correctly. This can cause the viewport to "breathe" — content lines appearing/disappearing as glamour interprets partial markdown differently. This is inherent to the debounced approach and is acceptable (same behavior as opencode/claude-code).

### 2.3 Partial markdown edge cases

**Tested with glamour directly:**

| Input | Behavior |
|---|---|
| Unclosed code fence | Renders as code block — correct |
| Partial link `[click here](https://example.com` | Renders as literal text with URL — acceptable |
| Mid-bold `**half bold` | Renders literal `**half bold` — acceptable |
| Unclosed italic `*italic without close` | Renders literal `*italic...` — acceptable |
| Table mid-row | Renders partial table with empty cells — acceptable |
| Code fence opening only | Renders as code block — correct |
| Partial list `- item one\n- item` | Renders as list with partial item — correct |

**Key finding:** Glamour is forgiving with partial markdown. It never panics or returns empty for non-empty input. The worst case is literal rendering of incomplete syntax, which is visually acceptable during streaming.

### 2.4 Event pipeline changes

**No changes needed.** The current event pipeline already calls `m.updateViewport()` on every `AgentEventTextDelta`. The debounce timer would be a separate mechanism that periodically triggers a glamour re-render of the pending content. The event pipeline continues to work as-is for accumulation.

**Optional enhancement:** Could add a `TuiTickMsg` at 150ms interval during streaming state to trigger the debounced re-render. The `TuiTickMsg` handler already exists in `update.go` but is not wired up.

### 2.5 Thinking blocks

**No special treatment needed during streaming.** Thinking blocks are rendered with simple lipgloss styling (dimmed, italic) which is already fast and correct. The streaming markdown concern only applies to `blockAssistantText`. Thinking blocks don't use glamour at all.

### 2.6 Code block streaming

**Glamour handles unclosed code fences gracefully** — they render as code blocks. The visual result during streaming is acceptable: code appears in a formatted block even before the closing ```. No special treatment needed.

---

## 3. Recommendation

### Recommended Approach: Debounced Full Re-rendering

**Justification:**
1. **Industry standard** — opencode and claude-code use this approach
2. **Correct output** — glamour handles all markdown properly, including partial
3. **Feasible performance** — render times are well within budget (<4ms for typical responses)
4. **Simple implementation** — ~50-100 lines of changes to existing code
5. **No new dependencies** — uses existing glamour infrastructure
6. **Maintainable** — straightforward to understand and debug

**The GC pressure concern is manageable:**
- Use a 200ms debounce interval (not 100ms) to reduce allocation rate
- Reuse the glamour renderer instance to save ~830µs + allocations per call
- Only re-render when content has changed since last render
- For very large responses (>5K tokens), the render time is still <10ms — well within budget

---

## 4. Implementation Plan

### 4.1 Architecture Changes

**No structural changes needed.** The existing architecture supports this approach:
- `pendingBuilder` already accumulates streaming content
- `updateViewport()` already handles scroll position
- `TuiTickMsg` handler already exists (just needs to be wired up)
- `RenderMarkdown()` already handles edge cases

### 4.2 New Types/Fields

Add to `Model` struct:
```go
// Debounced streaming render state.
pendingRendered      string        // last glamour-rendered output of pending content
pendingRenderedLen   int           // length of pendingBuilder when last rendered (change detection)
lastRenderTime       time.Time     // timestamp of last glamour render (debounce)
glamourRenderer      *glamour.TermRenderer // reused renderer instance
```

### 4.3 Changes by File

**`internal/tui/model.go`:**
- Add fields above to `Model` struct
- In `NewModel()`: initialize `glamourRenderer` once
- In `resetForTurn()`: reset `pendingRendered`, `pendingRenderedLen`, `lastRenderTime`
- In `processEvent()`: after accumulating text, start debounce timer via `tea.Every(200ms, TuiTickMsg{})`
- Add `startRenderDebounce() tea.Cmd` method

**`internal/tui/update.go`:**
- In `TuiTickMsg` handler: check if debounce interval elapsed and content changed, if so call `renderPendingMarkdown()` and `updateViewport()`
- In `AgentEventMsg` handler: start/restart debounce timer on each text delta

**`internal/tui/markdown.go`:**
- Add `NewRenderer(width int) (*glamour.TermRenderer, error)` function to create reusable renderer
- Add `RenderWithRenderer(r *glamour.TermRenderer, text string, width int) string` function

**`internal/tui/render.go`:**
- Modify `renderPendingBlock()` to accept rendered markdown string
- Add `renderPendingMarkdown()` method on Model that calls glamour with reused renderer

### 4.4 Debounce Logic

```
On AgentEventTextDelta:
  1. Append text to pendingBuilder
  2. If time since lastRender > 200ms OR pendingBuilder.Len() != pendingRenderedLen:
     a. Call glamour.Render(pendingBuilder.String())
     b. Store result in pendingRendered
     c. Update pendingRenderedLen
     d. Update lastRenderTime
  3. updateViewport() — uses pendingRendered if available, falls back to plain text

On TuiTickMsg (every 200ms during streaming):
  1. If pendingBuilder.Len() != pendingRenderedLen:
     a. Re-render through glamour
     b. updateViewport()
```

### 4.5 Testing Strategy

**Unit tests:**
- Debounce timing: verify render only happens after interval
- Change detection: verify no re-render if content unchanged
- Renderer reuse: verify same renderer instance is used
- Scroll position: verify viewport stability across re-renders
- Edge cases: all partial markdown variants from section 2.3

**Benchmarks:**
- Measure GC allocation rate at 200ms interval
- Compare reused vs new renderer allocations
- Measure viewport update latency with debounced render

**E2E tests:**
- Start tau, send prompt, verify styled markdown appears during streaming
- Verify no visual flicker or scroll jumps
- Verify final output matches glamour render

### 4.6 Migration Path

**Fully backward compatible:**
- No changes to event types or protocol
- No changes to finalized block rendering
- Debounce only affects pending/streaming content
- If glamour render fails, falls back to plain text (existing behavior)
- Can be disabled by setting debounce interval to 0

---

## 5. Summary Table

| Approach | Complexity | Performance | Correctness | UX | Maintenance | Recommend? |
|---|---|---|---|---|---|---|
| Debounced Full Re-render | Low | Good (GC concern) | Excellent | Good | Low | **YES** |
| Line-by-Line | Very High | Good | Fragile | Good | Very High | No |
| Hybrid Fast+Final | Medium | Excellent | Partial (streaming) | Poor (jump) | Medium | No |
| Custom Renderer | Very High | Excellent | Good | Good | Very High | No |

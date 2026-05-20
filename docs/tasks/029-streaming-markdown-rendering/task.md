# Task 029: Streaming Markdown Rendering Exploration

## Why

Currently, the TUI renders assistant responses as raw plain text during streaming. Styled markdown (via glamour) is only applied after the full response completes (`AgentEventMessageEnd`). Users see unformatted markdown syntax (`**bold**`, `[links](url)`, code fences, etc.) while the model is generating.

Tools like opencode and claude-code render styled markdown incrementally during streaming, providing a much better user experience. This task explores viable approaches for achieving similar behavior in our Go + bubbletea TUI.

## Comparison Analysis: How Others Do It

### OpenCode (TypeScript/React)
- Accumulates raw markdown from streaming tokens
- Periodically re-renders the **complete** accumulated markdown using a markdown renderer
- React handles DOM diffing and terminal updates
- **Not true incremental parsing** — debounced full re-rendering

### Claude Code (TypeScript)
- Same debounced re-rendering pattern
- Accumulates tokens, re-renders full markdown on a timer/throttle
- No incremental parsing

### Aider (Python)
- Simpler: renders markdown only after response completes
- During streaming: raw text with minimal formatting
- No incremental styling

### Key Insight
**No major tool does true incremental markdown parsing.** The industry standard is **debounced full re-rendering** — re-parse and re-style the complete accumulated content on a timer (100-200ms).

## Main Constraints

- Must work with Go + bubbletea architecture
- Glamour (our current renderer) does NOT support true streaming — `Write()` just buffers, parsing happens on `Close()`
- Goldmark (glamour's parser) requires complete, well-formed markdown — partial markdown can produce incorrect AST
- Must not degrade streaming performance or introduce noticeable latency
- Must handle edge cases: unclosed code fences, partial links, mid-token boundaries
- Should feel responsive — styling should appear quickly after content arrives

## Dependencies

- Task 018 (Streaming Block Accumulation) — provides `EventTextStart`/`EventTextEnd` boundaries
- Task 028 (Styled Markdown Output with Clickable Links) — current glamour rendering infrastructure
- Task 024 (Thinking/Reasoning Rendering) — event delivery pipeline

## Exploration Scope

### Approaches to Evaluate

1. **Debounced Full Re-rendering** (industry standard)
   - Accumulate tokens in `pendingBuilder`
   - Re-render through glamour every 100-200ms
   - Cache rendered output between renders
   - Final render on `AgentEventMessageEnd`

2. **Line-by-Line with Sanitization**
   - Only render "complete" lines/blocks
   - Buffer incomplete lines until complete
   - Detect block boundaries (code fences, list items)

3. **Hybrid: Fast Path + Final Render**
   - During streaming: simple lipgloss styling (bold, italic, code spans)
   - On completion: full glamour render
   - Swap simple render for final render when done

4. **Custom Incremental Renderer**
   - Build custom markdown tokenizer + ANSI emitter
   - Maintain state about current block context
   - Emit ANSI codes as tokens arrive

### For Each Approach, Document

- Implementation complexity (estimated effort)
- Performance characteristics (CPU, memory, latency)
- Correctness (handles all markdown edge cases?)
- Visual stability (flicker, layout shifts)
- Maintenance burden
- Compatibility with existing TUI architecture
- Compatibility with existing features: OSC 8 hyperlinks, code block styling, thinking blocks

### Specific Questions to Answer

1. **Glamour debouncing feasibility**: Can we call `glamour.Render()` every 150ms without performance issues? What's the cost for typical response sizes (2K-10K tokens)?

2. **Viewport stability**: When re-rendering, does the viewport scroll position jump? How to maintain scroll position across re-renders?

3. **Partial markdown edge cases**: What happens when glamour renders incomplete markdown? (e.g., unclosed code fence, partial link `[text](url`)

4. **Event pipeline changes**: Do we need `AgentEventTextEnd` to trigger `flushPending()` at text block boundaries (not just message end)?

5. **Thinking blocks**: Should thinking blocks use different rendering rules during streaming?

6. **Code block streaming**: How to handle code blocks that arrive mid-stream? Should code blocks get special treatment?

## Deliverables

- Exploration document with findings for each approach
- Recommendation with justification
- If viable, draft implementation plan with:
  - Architecture changes needed
  - New types/events
  - Testing strategy
  - Migration path (backward compat)

## Acceptance Criteria

- [x] All 4 approaches evaluated with pros/cons
- [x] Performance benchmarks for glamour debouncing (various response sizes)
- [x] Edge case analysis (partial markdown, code fences, links)
- [x] Clear recommendation with justification
- [x] Implementation plan if recommendation is to proceed
- [x] Findings documented in task directory

## Testing & Verification Strategy

**Benchmarks:**
- Measure glamour render time for 1K, 5K, 10K token markdown documents
- Measure debounce interval impact on perceived latency
- Memory profiling for repeated re-rendering

**Edge case tests:**
- Unclosed code fence during streaming
- Partial link syntax `[text](url`
- Mid-bold `**half`
- Tables arriving row by row
- Nested lists

**Visual tests:**
- Scroll position stability across re-renders
- Flicker/jump detection
- Code block highlighting during streaming

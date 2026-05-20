# Task 028: Styled Markdown Output with Clickable Links

## Why

Model responses are currently rendered as plain text with lipgloss styling. LLMs produce markdown-formatted output (bold, italic, headings, code blocks, lists, links) that is lost in display. Additionally, URLs in responses are plain text — users must copy-paste them. This task adds markdown rendering via `glamour` (charm.land ecosystem, same as bubbletea/lipgloss) and makes links clickable via OSC 8 terminal hyperlinks + mouse click handling.

## Comparison: Our Approach vs opencode vs PI

| Dimension | PI (Web) | opencode Web | opencode TUI | Tau (current) | Tau (proposed) |
|-----------|----------|--------------|--------------|---------------|----------------|
| Markdown rendering | `marked` → HTML → DOM | `marked` → HTML → DOM | `@opentui/core` `<markdown>` element | Plain text + lipgloss | `glamour` → ANSI |
| Link clickability | HTML `<a>` elements | HTML `<a>` elements | `onMouseUp` → `open(url)` | None | OSC 8 hyperlinks + mouse click → `open(url)` |
| Streaming rendering | `PacedMarkdown` with chunked reveal | `stream()` + `remend` healing | `<code filetype="markdown">` tree-sitter | Plain text accumulation | Plain text during stream, glamour on completion |
| Syntax highlighting | `marked-shiki` (WASM) | `marked-shiki` (WASM) | tree-sitter scopes | None | `chroma` (via glamour) |
| Caching | LRU cache (200 entries) + `morphdom` diff | Same | N/A (native renderer) | None | Cache rendered output per block |

## Main Constraints

- Must use `glamour` (charm.land ecosystem, same as bubbletea/lipgloss)
- Streaming text must remain plain text during accumulation (glamour can't handle incomplete markdown)
- Glamour rendering only on finalized blocks (after `AgentEventMessageEnd`)
- Must handle terminal resize (re-render with new width)
- Must support OSC 8 hyperlinks for clickable URLs
- Must handle mouse click events to open links
- Must be backward compatible — no breaking changes to event processing pipeline
- New dependency: `github.com/charmbracelet/glamour` (brings `chroma` for syntax highlighting)

## Dependencies

- Task 021 (Message Rendering & Display) — completed (current block-based rendering)
- Task 024 (Thinking/Reasoning Rendering) — completed (event channel fix)
- `github.com/charmbracelet/glamour` — new dependency
- `github.com/skratchdot/open-golang/open` or `os/exec` — to open URLs
- Bubbletea v2 mouse support (`MouseClickMsg`) — already available
- `goldmark` v1.7.1 — already in go.sum (transitive), may be used for URL extraction

## Architecture

### Current pipeline
```
Provider StreamEvent → Agent AgentEvent → TUI processEvent()
  → pendingBuilder (strings.Builder) → on MessageEnd → messageBlock{kind: blockAssistantText, text: "..."}
  → renderBlock() → lipgloss styled string → viewport
```

### Proposed pipeline
```
Provider StreamEvent → Agent AgentEvent → TUI processEvent()
  → pendingBuilder (strings.Builder) → on MessageEnd → messageBlock{kind: blockAssistantText, text: "...", renderedMarkdown: ""}
  → renderBlock() → if renderedMarkdown == "" { glamour.Render(text, width) → cache in block } → viewport
  → on resize → invalidate renderedMarkdown cache → re-render
```

### Key design decisions

1. **Glamour only on finalized blocks** — streaming text stays plain text. This avoids the problem of glamour failing on incomplete markdown (unclosed code blocks, broken lists, etc.).

2. **OSC 8 hyperlinks** — rendered markdown output gets URLs wrapped in `\x1b]8;;URL\x1b\\text\x1b]8;;\x1b\\` escape sequences. Terminals handle click natively.

3. **Mouse click fallback** — for terminals that don't support OSC 8, add mouse click detection: track link positions, on click extract URL and open via `open` package.

4. **Rendered cache** — each `messageBlock` stores its glamour-rendered output. On resize, cache is invalidated and re-rendered.

## Subtasks

### 028.1 — Add glamour dependency and render completed assistant messages

**Goal:** Replace plain-text assistant rendering with glamour-rendered markdown for finalized messages. Streaming text remains plain text.

**Scope:**
- Add `github.com/charmbracelet/glamour` dependency
- Create `internal/tui/markdown.go` — glamour renderer wrapper
- Modify `messageBlock` struct: add `renderedMarkdown string` field
- Modify `renderAssistantText()`: if `renderedMarkdown` is empty, render through glamour, cache result
- Use glamour's auto-dark theme (or `charm` theme with dark background)
- Width-aware rendering: pass viewport width to glamour

**Files to create:**
- `internal/tui/markdown.go` — `RenderMarkdown(text string, width int) string`
- `internal/tui/markdown_test.go` — unit tests for various markdown inputs

**Files to modify:**
- `internal/tui/render.go` — `renderAssistantText()` uses glamour for finalized blocks
- `internal/tui/styles.go` — possibly adjust/remove `assistantBlockStyle` (glamour handles its own styling)
- `go.mod` / `go.sum` — new dependency

**Acceptance criteria:**
- [ ] `go get github.com/charmbracelet/glamour` succeeds
- [ ] `go build ./...` succeeds
- [ ] `go test ./internal/tui/...` passes (update existing tests for new output format)
- [ ] Assistant messages with markdown (bold, italic, headings, code blocks, lists) render with proper styling
- [ ] Streaming text (during accumulation) remains plain text
- [ ] On `AgentEventMessageEnd`, text is rendered through glamour
- [ ] Manual verification against Ollama: send prompt that produces markdown output, verify rendering

**Handoff notes for next session:**
- Glamour renders complete markdown documents — it does NOT support incremental rendering
- The `renderAssistantText()` function now has two paths: plain text for pending blocks (empty `renderedMarkdown`), glamour for finalized blocks (populated `renderedMarkdown`)
- Glamour's `Render()` function returns a string with ANSI escape codes — this is what gets written to the viewport
- The `messageBlock.renderedMarkdown` field is the cache — when empty, rendering is needed; when populated, use cached value
- Glamour uses the `chroma` library for syntax highlighting in code blocks
- Test with various markdown: `**bold**`, `*italic*`, `# heading`, `` `code` ``, ` ```code blocks``` `, `- lists`, `[links](url)`

---

### 028.2 — Add OSC 8 terminal hyperlinks for URLs

**Goal:** Make URLs in rendered markdown clickable via OSC 8 escape sequences. Works in modern terminals without mouse event handling.

**Scope:**
- Create `internal/tui/hyperlinks.go` — URL detection and OSC 8 wrapping
- URL regex pattern to detect bare URLs and markdown links in text
- Post-process glamour output: find URLs, wrap in OSC 8 sequences
- Handle both: URLs in markdown link text `[text](url)` and bare URLs in text
- Ensure OSC 8 wrapping doesn't break existing ANSI styling from glamour

**Files to create:**
- `internal/tui/hyperlinks.go` — `WrapURLs(text string) string`
- `internal/tui/hyperlinks_test.go` — unit tests for URL detection and wrapping

**Files to modify:**
- `internal/tui/markdown.go` — apply `WrapURLs()` to glamour output before caching

**Acceptance criteria:**
- [ ] URLs in assistant messages are wrapped in OSC 8 escape sequences
- [ ] Clicking a URL in a supported terminal (kitty, iTerm2, GNOME Terminal) opens it in browser
- [ ] OSC 8 wrapping doesn't break glamour's ANSI styling (colors, bold, etc.)
- [ ] Markdown links `[text](url)` have the link text clickable
- [ ] Bare URLs in text are also clickable
- [ ] Unit tests cover: no URLs, single URL, multiple URLs, URLs in code blocks, URLs in link text, malformed URLs
- [ ] `go test ./internal/tui/...` passes

**Handoff notes for next session:**
- OSC 8 format: `\x1b]8;;URL\x1b\\clickable_text\x1b]8;;\x1b\\`
- The URL regex should match `https?://` and `www.` prefixed URLs
- Be careful not to wrap URLs inside code blocks (they're already syntax-highlighted by glamour)
- Some terminals don't support OSC 8 — mouse click handling (next subtask) provides fallback
- The `WrapURLs()` function operates on the final ANSI string — it needs to be careful not to break existing escape sequences
- Consider using a state machine or AST-aware approach if regex proves fragile with ANSI codes

---

### 028.3 — Add mouse click handling to open links

**Goal:** Handle mouse click events in the viewport to detect which link was clicked and open it. Provides fallback for terminals without OSC 8 support.

**Scope:**
- Enable mouse mode in Bubbletea program (`tea.WithMouseAllMotion()` or similar)
- Track link positions in rendered output (start column, end column, line number, URL)
- On `tea.MouseClickMsg`, check if click position overlaps a link
- If yes, open URL via `open` package
- Visual feedback: highlight link on hover (optional, depends on mouse motion support)

**Files to create:**
- `internal/tui/links.go` — link position tracking, hit detection
- `internal/tui/links_test.go` — unit tests for hit detection

**Files to modify:**
- `internal/tui/model.go` — add link position tracking, handle `MouseClickMsg`
- `internal/tui/update.go` — delegate mouse events to link handler
- `cmd/tau/interactive.go` — enable mouse mode in Bubbletea program

**Acceptance criteria:**
- [ ] Mouse clicks in viewport are captured
- [ ] Clicking on a link opens the URL in default browser
- [ ] Clicking outside a link does nothing (scrolls viewport as before)
- [ ] Link position tracking survives viewport scrolling
- [ ] Works in terminals without OSC 8 support
- [ ] `go test ./internal/tui/...` passes
- [ ] Manual verification: click links in rendered markdown, verify they open

**Handoff notes for next session:**
- Bubbletea v2 has `MouseClickMsg`, `MouseMotionMsg`, `MouseWheelMsg` types
- Mouse mode must be enabled: `tea.WithMouseAllMotion()` or `tea.WithMouseCellMotion()`
- Link positions need to be tracked per-line, accounting for ANSI escape sequences (which don't contribute to visible width)
- The `lipgloss.Width()` function returns visible width (excluding ANSI codes) — useful for position calculation
- Consider storing link positions as `[]LinkPos{line, startCol, endCol, url}` on the model
- On resize, link positions must be recalculated (same as rendered markdown cache invalidation)

---

### 028.4 — Handle resize, caching, and edge cases

**Goal:** Ensure markdown rendering survives terminal resize, handles edge cases, and performs well.

**Scope:**
- On `tea.WindowSizeMsg`, invalidate `renderedMarkdown` cache for all blocks
- Re-render all blocks with new width on next `View()` call
- Handle empty assistant text (skip glamour)
- Handle very long content (glamour handles it, but verify viewport scroll)
- Handle malformed markdown (glamour should still render something)
- Performance: measure render time, add caching if needed
- Handle mixed content: markdown with thinking blocks, tool calls interleaved

**Files to modify:**
- `internal/tui/model.go` — resize handler invalidates cache
- `internal/tui/markdown.go` — add empty input handling, error recovery
- `internal/tui/render.go` — ensure all block types work with new rendering

**Acceptance criteria:**
- [ ] Terminal resize triggers re-render of all assistant messages with new width
- [ ] No visual artifacts or corrupted output after resize
- [ ] Empty assistant text doesn't crash or produce empty glamour output
- [ ] Very long messages (10K+ chars) render correctly and viewport scrolls
- [ ] Malformed markdown (unclosed code blocks, broken lists) renders without crashing
- [ ] Mixed content (text + thinking + tool calls) renders correctly
- [ ] Performance: render time < 50ms for typical message (< 2K chars)
- [ ] `go test ./internal/tui/...` passes

**Handoff notes for next session:**
- Glamour's `Render()` is CPU-intensive for large documents — caching is essential
- The resize invalidation should be lazy: mark cache as dirty, re-render on next `View()` call
- Consider adding a render time metric for debugging
- Edge case: what happens when a message is being streamed AND the terminal resizes? The pending block stays plain text, finalized blocks re-render.

---

### 028.5 — Polish: theme, tests, documentation

**Goal:** Final polish — custom theme matching tau's existing color scheme, comprehensive tests, documentation updates.

**Scope:**
- Create custom glamour theme matching tau's existing styles (from `styles.go`)
- Update ARCHITECTURE.md with markdown rendering design
- Update DECISIONS.md with glamour + OSC 8 decision
- Comprehensive test coverage: all markdown elements, edge cases, resize behavior
- E2E verification against Ollama with markdown-producing prompts

**Files to create:**
- `internal/tui/theme.go` — custom glamour style definition
- `docs/tasks/028-styled-markdown-links/worklog.md` — work log

**Files to modify:**
- `docs/ARCHITECTURE.md` — add markdown rendering section
- `docs/DECISIONS.md` — document glamour + OSC 8 decision
- `docs/TRACKING.md` — update task status

**Acceptance criteria:**
- [ ] Custom glamour theme matches tau's existing color scheme (dark background, cyan links, etc.)
- [ ] ARCHITECTURE.md updated with markdown rendering pipeline
- [ ] DECISIONS.md documents why glamour was chosen over alternatives
- [ ] Test coverage: > 80% for `markdown.go`, `hyperlinks.go`, `links.go`
- [ ] E2E verification: send prompt to Ollama that produces rich markdown, verify all elements render correctly
- [ ] `go build ./...` succeeds
- [ ] `go test ./...` passes
- [ ] `go vet ./...` passes
- [ ] `go mod tidy` clean

## Testing Strategy

**Unit tests:**
- Markdown rendering: bold, italic, headings, code blocks (inline + fenced), lists (ordered + unordered), links, tables, blockquotes
- URL detection: no URLs, single URL, multiple URLs, URLs in code blocks, URLs in link text, malformed URLs, IPv4/IPv6 URLs
- Link hit detection: click on link, click outside link, click on overlapping links, click after scroll
- Resize: cache invalidation, re-render with new width
- Edge cases: empty input, very long input, malformed markdown, Unicode content

**Integration tests:**
- Event processing: `AgentEventTextDelta` accumulation → `AgentEventMessageEnd` → glamour render
- Resize during streaming: pending block stays plain, finalized blocks re-render
- Mouse click during streaming: no links available yet, click does nothing

**Manual verification (against Ollama):**
- Send prompt: "Write a markdown document with headings, bold, italic, code blocks, lists, and links"
- Verify: all elements render with proper styling
- Click each link: verify it opens in browser
- Resize terminal: verify re-render is correct
- Send prompt with thinking model (gemma4:26b): verify thinking blocks + markdown response both render correctly

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Glamour transitive deps bloat | Medium | Low | `go mod why` to audit, acceptable if < 10 new deps |
| OSC 8 not supported in user's terminal | Medium | Low | Mouse click handling provides fallback |
| Glamour performance on large messages | Low | Medium | Caching, lazy rendering, measure and optimize |
| ANSI escape sequence corruption in OSC 8 wrapping | Medium | High | Careful regex, test with various glamour output |
| Mouse click position calculation with ANSI codes | High | Medium | Use `lipgloss.Width()` for visible width, test extensively |

## Proposed File Structure

```
internal/tui/
├── markdown.go          # Glamour renderer wrapper
├── markdown_test.go     # Unit tests for markdown rendering
├── hyperlinks.go        # OSC 8 URL wrapping
├── hyperlinks_test.go   # Unit tests for URL detection/wrapping
├── links.go             # Link position tracking, mouse hit detection
├── links_test.go        # Unit tests for hit detection
├── theme.go             # Custom glamour theme (optional, subtask 028.5)
├── render.go            # Modified: glamour path in renderAssistantText()
├── model.go             # Modified: resize cache invalidation, mouse handling
├── update.go            # Modified: mouse event delegation
└── styles.go            # Modified: possibly adjust assistantBlockStyle
```

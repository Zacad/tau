# Task 021: Implementation Plan (Revised)

## Current Architecture (Problem)

The TUI currently stores all conversation content as raw strings in `messageContent` and `streamingText` (strings.Builder). The `View()` method just concatenates these strings and sets viewport content. This makes it impossible to apply lipgloss styling retroactively — View() needs access to structured content to render with styles each frame.

## Review-Informed Architecture

Replace the `messageContent` / `streamingText` string builders with a structured `[]messageBlock` slice. Each block knows its type and renders itself with lipgloss. View() iterates all blocks, calls `.Render(width)` on each, joins them, and sets viewport content.

### Data Model (Revised — reviewer feedback addressed)

```go
// internal/tui/render.go

// blockType categorizes what kind of content a block holds.
type blockType int

const (
    blockUserMessage blockType = iota
    blockAssistantText
    blockThinking
    blockToolCall
    blockTurnSeparator
    blockError
    blockSubAgent
)

// toolStatus tracks a tool call's execution phase.
type toolStatus int

const (
    toolPending toolStatus = iota
    toolSuccess
    toolError
)

// messageBlock is one unit of renderable content in the viewport.
// Width is NOT stored here — it changes on resize and is passed to render functions.
type messageBlock struct {
    kind     blockType
    text     string           // for text, thinking, error
    toolName string           // for tool calls
    toolArgs string           // for tool calls (truncated)
    toolOut  string           // for tool errors: truncated output
    toolSt   toolStatus       // for tool calls
    subID    string           // for subagent blocks
}
```

### Rendering Functions (render.go)

All render functions are pure: `(data, width int) → string`. No side effects, fully testable.

```go
func renderBlock(b messageBlock, width int) string
func renderUserMessage(text string, width int) string
func renderAssistantText(text string, width int) string
func renderThinkingBlock(text string, width int) string
func renderToolCall(name, args string, st toolStatus, errOut string, width int) string
func renderTurnSeparator(width int) string
func renderError(text string, width int) string
func renderSubAgent(id string, done bool, width int) string
```

### Streaming (Revised — reviewer feedback addressed)

**Problem**: `text += delta` on every event is O(n²) for streaming responses.

**Solution**: Use `strings.Builder` for the pending text/thinking block during streaming. On turn completion, finalize into a `messageBlock`.

```go
type Model struct {
    blocks []messageBlock         // finalized blocks
    pendingBuilder *strings.Builder  // active streaming buffer
    pendingKind    blockType         // what kind of block is pending
    turnCount      int
    spinner        spinner.Model
    spinnerActive  bool
}
```

### Tool Call Status Tracking (Revised — reviewer feedback addressed)

**Problem**: `AgentEventToolExecEnd` has no data field (nil). TUI cannot determine success/failure from events alone.

**Solution**: Track pending tool calls in the TUI. On `ToolExecStart`, create a pending block. On `ToolExecEnd`, mark as success. On `AgentEventError` while a tool is pending, mark that tool as errored.

### Footer

```
[model] [cwd] [turns: N] [tokens: X] [$0.00 (local)] [spinner]
```

- Turn count: incremented on each `AgentEventTurnEnd`
- Tokens/cost: fetched from `session.Usage()` on turn end
- Cost: check `usage.Cost.Total == 0` → show "$0.00 (local)"
- Spinner: `bubbles/spinner`, started on `AgentEventMessageStart`, stopped on `AgentEventTurnEnd`/`AgentEventAgentEnd`

### Spinner Tick Integration (Revised — reviewer feedback addressed)

The spinner needs a `tea.Tick` command to animate:

```go
// In Update(), when spinnerActive is true:
return m, tea.Tick(spinnerDuration, func(t time.Time) tea.Msg {
    return spinner.TickMsg{Time: t}
})
```

### Files

| File | Change |
|------|--------|
| `internal/tui/styles.go` | Update: add new style definitions |
| `internal/tui/render.go` | **NEW**: `messageBlock`, `toolStatus`, all render functions |
| `internal/tui/model.go` | Modify: replace string builders with block slice + pending builder + spinner |
| `internal/tui/view.go` | Modify: use `renderBlock()` for viewport, update footer |
| `internal/tui/update.go` | Modify: event processing builds blocks, spinner tick handling |
| `internal/tui/render_test.go` | **NEW**: unit tests for all render functions |

### Truncation Strategy (Revised — reviewer feedback addressed)

- Tool args: truncate to 120 chars, append `…` if longer
- Tool error output: truncate to 200 chars, append `\n... (truncated)` if longer
- Threshold constants defined at top of `render.go` for easy tuning

### Flicker Mitigation (Revised — reviewer feedback addressed)

- bubbletea v2 batches frames automatically
- lipgloss rendering is string formatting (fast)
- First pass: implement naively, verify empirically against Ollama streaming
- If flicker occurs: cache rendered strings for completed blocks, only re-render pending block

### Subtask Breakdown (Revised — split 021.1/021.3 per reviewer feedback)

1. **021.1** — Styled user/assistant messages
2. **021.3** — Thinking/reasoning block display
3. **021.2** — Tool call rendering
4. **021.4** — Working indicator (spinner)
5. **021.5** — Turn separators
6. **021.6** — Enhanced footer
7. **021.7** — Auto-scroll verification
8. **021.9** — Unit tests

### Subtask Acceptance Criteria

#### 021.1 — Styled Messages
- [ ] `messageBlock` type, typed sub-structs (no `map[string]string`)
- [ ] `renderUserMessage()` with distinct prefix style (color/bold)
- [ ] `renderAssistantText()` with proper styling
- [ ] `processEvent()` builds blocks instead of raw strings
- [ ] `View()` renders blocks with lipgloss
- [ ] `strings.Builder` for pending streaming block (not string concatenation)
- [ ] `go test ./internal/tui/...` passes
- [ ] Manual: messages visually distinct in TUI

#### 021.3 — Thinking Blocks
- [ ] `renderThinkingBlock()` in dimmed/italic style
- [ ] Thinking blocks visually distinct from response text
- [ ] Pending thinking block uses `strings.Builder`
- [ ] `go test ./internal/tui/...` passes
- [ ] Manual: thinking blocks visible with gemma4:26b

#### 021.2 — Tool Call Rendering
- [ ] `renderToolCall()` with name + args (truncated 120 chars) + status
- [ ] Pending: shows name + `…` indicator
- [ ] Success: shows name + `✓`
- [ ] Error: shows name + `✗` + truncated output (200 chars) in red
- [ ] Tool status tracked in TUI (pending → success/error)
- [ ] `go test ./internal/tui/...` passes
- [ ] Manual: tool calls visible during Ollama interaction

#### 021.4 — Spinner
- [ ] `bubbles/spinner` added to Model
- [ ] Spinner TickMsg wired in Update()
- [ ] Started on `AgentEventMessageStart`, stopped on `AgentEventTurnEnd`
- [ ] `go test ./internal/tui/...` passes
- [ ] Manual: spinner animates during agent work

#### 021.5 — Turn Separators
- [ ] `blockTurnSeparator` inserted on `AgentEventTurnEnd`
- [ ] Rendered as horizontal rule with subtle styling
- [ ] `go test ./internal/tui/...` passes
- [ ] Manual: visible between multi-turn conversations

#### 021.6 — Enhanced Footer
- [ ] Shows: model | cwd | turns: N | tokens: X | cost
- [ ] `$0.00 (local)` when `usage.Cost.Total == 0`
- [ ] Actual `$` amount for paid providers
- [ ] `go test ./internal/tui/...` passes
- [ ] Manual: footer shows accurate info after turns

#### 021.7 — Auto-scroll
- [ ] Viewport follows streaming output to bottom
- [ ] No full re-render flicker during streaming (verify empirically)
- [ ] Manual: smooth scrolling during streaming

#### 021.9 — Unit Tests
- [ ] All render functions tested (happy path + edge cases)
- [ ] Empty message, very long text, multi-line thinking
- [ ] Tool call: pending, success, error with truncated output
- [ ] `go test ./internal/tui/...` — all pass
- [ ] Coverage: 80%+ for render.go

### Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Flicker during streaming | HIGH | Empirical verification; cache rendered blocks if needed |
| ToolExecEnd has no status data | HIGH | Track pending tool calls in TUI state |
| O(n²) string concat on deltas | HIGH | Use `strings.Builder` for pending block |
| Spinner TickMsg not wired | MEDIUM | Explicit `tea.Tick` in Update() |
| Memory growth in long sessions | LOW | Accept for now; address in future task |
| Resize re-renders all blocks | LOW | Accept (lipgloss is fast); cache if needed |

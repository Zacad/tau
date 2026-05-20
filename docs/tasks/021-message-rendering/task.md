# Task 021: Message Rendering & Display

## Why

Task 020 delivers a functional chat interface, but messages appear as raw text. This task transforms the display into a polished, readable conversation with styled messages, collapsible tool output, thinking blocks, and a working indicator — matching PI's visual quality.

## Comparison: Our Approach vs PI

| Dimension | PI Approach | Our Approach (Task 021) |
|-----------|-------------|------------------------|
| Message styling | Theme-based colors | lipgloss styles |
| Tool calls | Collapsible, summary in collapsed | Styled blocks, name + args visible |
| Thinking blocks | Collapsible, dimmed text | Dimmed/italic rendering |
| Working indicator | Animated spinner in footer | `bubbles/spinner` in footer |
| Turn separators | Visual divider | lipgloss horizontal rule |
| Scroll behavior | Auto-scroll to bottom | `viewport.GotoBottom()` |

## Main Constraints

- Must use `lipgloss` for all styling (consistent with bubbletea ecosystem)
- Tool output rendering must handle: pending, success, error states
- Thinking blocks must be visually distinct from regular text
- Streaming text must update in real-time without full re-render flicker
- Viewport must auto-scroll to bottom during streaming
- Footer must show: working indicator + model + cwd + token usage + cost

## Dependencies

- Task 020 (CLI Foundation) — completed first
- `internal/sdk/` — Usage tracking, event subscription
- `internal/types/` — AgentEvent types, ContentBlock types
- `internal/provider/` — Model metadata for display
- `bubbles/spinner` — working indicator

## Subtasks

- [x] **021.1** — Styled message rendering with lipgloss: user vs assistant differentiation
- [x] **021.2** — Tool call rendering: name, arguments, result, status indicators
- [x] **021.3** — Thinking/reasoning block display: dimmed text, prefix marker
- [x] **021.4** — Working indicator: `bubbles/spinner` in footer during streaming
- [x] **021.5** — Turn separators between conversation turns
- [x] **021.6** — Enhanced footer: token usage, cost, turn count
- [x] **021.7** — Auto-scroll: viewport follows streaming output
- [x] **021.8** — Message timestamp display (optional, subtle) — **DEFERRED**
- [x] **021.9** — Unit tests for message rendering functions

## Acceptance Criteria

- [ ] User messages visually distinct from assistant messages (different styling/color)
- [ ] Tool calls display: tool name, arguments (truncated), execution status (pending → done/error)
- [ ] Thinking blocks rendered in dimmed/italic style, visually distinct from response text
- [ ] Working indicator (spinner) visible in footer during agent streaming
- [ ] Turn separator between conversation turns (derived from `AgentEventTurnEnd` events, UI-only marker)
- [ ] Footer shows: spinner (when working) + model name + cwd + token usage + cost
- [ ] Cost display: shows "$0.00 (local)" for zero-cost models (e.g., Ollama), actual cost for paid providers
- [ ] Viewport auto-scrolls to bottom during streaming output
- [ ] Long tool output is truncated with indication of more content
- [ ] Error messages displayed in red/highlighted style. Error data type: `AgentEventError.Data` is a `string` (error message)
- [ ] Streaming text updates smoothly without full re-render flicker
- [ ] `go test ./cmd/tau/...` — all pass
- [ ] Manual verification against Ollama: tool calls and thinking display correctly

## AgentEvent Data Type Mapping

| EventType | Data Type | Serialization |
|-----------|-----------|---------------|
| `AgentEventStart` | nil | No data field |
| `AgentEventMessageStart` | nil | No data field |
| `AgentEventTextDelta` | `string` | Raw text |
| `AgentEventThinkingDelta` | `string` | Raw text |
| `AgentEventToolExecStart` | `struct{ Tool, Args string }` | JSON |
| `AgentEventToolExecEnd` | nil | No data field |
| `AgentEventMessageEnd` | nil | No data field |
| `AgentEventTurnEnd` | nil | No data field |
| `AgentEventAgentEnd` | nil | No data field |
| `AgentEventError` | `string` | Error message |
| `AgentEventSubAgentStart` | `string` | Subagent ID |
| `AgentEventSubAgentEnd` | `string` | Subagent ID |

## Proposed New Files

```
cmd/tau/
├── render.go            # Message rendering with lipgloss styles
├── footer.go            # Footer component (usage, model, spinner)
├── styles.go            # Centralized lipgloss style definitions
└── render_test.go       # Tests for rendering functions
```

## Testing Strategy

**Unit tests:**
- Message rendering: user message, assistant message, empty message
- Tool call rendering: name, args, result, error, truncated output
- Thinking block rendering: single line, multi-line, mixed with text
- Footer rendering: with/without usage, with/without spinner
- Style definitions: color consistency, width handling

**Manual verification (against Ollama):**
- Send a prompt that triggers tool use
- Verify: tool call shows name + args, then result appears
- Send a prompt that triggers reasoning (gemma4:26b has reasoning)
- Verify: thinking blocks appear dimmed, response text appears normal
- Watch streaming output: verify smooth updates, no flicker
- Check footer: spinner during work, usage after completion

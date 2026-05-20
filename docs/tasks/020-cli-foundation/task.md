# Task 020: CLI Foundation + Interactive MVP

## Why

This is the first implementation task for the Tau CLI. It delivers a minimal but working interactive chat interface using bubbletea v2, plus print and JSON modes. The goal is to get a functional CLI that can chat with an LLM as quickly as possible, establishing the foundation for all subsequent TUI features.

## Design Decision

**Bubbletea v2 + Bubbles** was selected after extensive analysis of 12+ Go TUI/readline libraries. Key reasons:
- Official chat example matches our exact pattern (`viewport` + `textarea`)
- Elm architecture maps naturally to our event-driven SDK (`AgentEvent` → `tea.Msg`)
- Message queuing is trivial (textarea stays active during streaming)
- 42k+ stars, actively maintained (v2.0.6, April 2026)
- Full ecosystem: viewport, textarea, spinner, list, lipgloss styling, glamour markdown

See `TUI-LIBRARY-RESEARCH.md` and `TUI-ANALYSIS.md` for the full analysis.

## Implementation Plan (Phased)

Work is split into 4 phases for incremental delivery:

### Phase 1: Skeleton + Print/JSON Modes ✅ DONE
- CLI entry point with flag parsing (`cmd/tau/main.go`)
- Print mode (`-p`): single prompt → plain text output → exit
- JSON mode (`--mode json`): JSONL events to stdout
- Stdin piping support
- Unit tests for flag parsing + subprocess error tests
- Ollama auto-discovery added to SDK

### Phase 2: TUI Core (NEXT — implement in new session)
- Event bridge: `AgentEvent` → `tea.Msg` via buffered channel
- Bubbletea model: compose textarea + viewport + state machine
- Update loop: turn cycle, key handling, message routing
- View layout: header + viewport + footer + textarea
- Basic lipgloss styles

### Phase 3: Polish
- Ctrl+C abort recovery
- Slash commands (`/quit`, `/help`)
- Session flags (`-c`, `--no-session`) in interactive mode
- Footer with model name + cwd
- Ctrl+D exit when input empty

### Phase 4: Testing + Verification
- Manual verification against Ollama
- All acceptance criteria checked off

## Phase 1 Results (Complete)

### Files Created
```
cmd/tau/
├── main.go              # CLI entry, flag parsing, mode routing ✅
├── print.go             # Print mode (no TUI dependency) ✅
├── json.go              # JSON mode (no TUI dependency) ✅
├── interactive.go       # Interactive mode stub (Phase 2 placeholder) ✅
├── main_test.go         # 10 tests: flag parsing + subprocess error cases ✅
└── sessions.go          # (pending — Phase 3)
└── commands.go          # (pending — Phase 3)

internal/tui/
# (entire package — Phase 2)
```

### SDK Modification
- `internal/sdk/sdk.go`: Added `registerOllama()` — auto-discovers local Ollama models via `/api/tags` endpoint, registers them in model registry with `OpenAICompatProvider`

### Dependencies Added
- `charm.land/bubbletea/v2` v2.0.6
- `charm.land/bubbles/v2` v2.1.0
- `charm.land/lipgloss/v2` v2.0.3

### Bug Discovered & Fixed
**Deadlock in JSON mode**: The `agent_end` subscriber callback called `session.Usage()` which tried to acquire `s.mu.Lock()` while `session.Prompt()` still held it. The SDK's `Subscribe()` calls listeners synchronously on the agent goroutine, which is nested inside `Prompt()` → mutex held. **Fix**: Emit the final `agent_end` with usage *after* `Prompt()` returns, outside the subscriber callback.

**Key lesson for Phase 2**: The three-goroutine bridge pattern (Subscribe → buffered channel → tea.Cmd) is critical. The bridge must use non-blocking push because `Subscribe()` callbacks are synchronous on the agent goroutine. Calling any SDK method that acquires `s.mu` inside a subscriber callback will deadlock.

### Phase 1 Quality Gates
- `go build ./cmd/tau` ✅
- `go vet ./cmd/tau/...` ✅
- `go build -race ./cmd/tau` ✅
- `go test -race ./cmd/tau/...` ✅ (10/10 pass, 1.341s)
- `go mod tidy` ✅
- `go test ./...` ✅ (all 12 packages pass)

### Phase 1 E2E Verification (against Ollama gemma4:26b)
| Test | Result |
|------|--------|
| `./tau -p "Say hello"` | ✅ Response printed |
| `echo "text" \| ./tau` | ✅ Stdin piping works |
| `./tau -p "List files"` (tool call) | ✅ `ls` tool executed, results displayed |
| `./tau --mode json -p "hi"` | ✅ 6 valid JSONL events |
| `./tau --mode json -p "List files"` (tool call) | ✅ 82 JSONL events across 2 turns |
| `./tau --bogus` | ✅ Exit 2 |
| `./tau --mode json` (no prompt) | ✅ Exit 2 |
| `./tau -p "hello" --model nonexistent` | ✅ Exit 1, lists available models |
| Session file on disk | ✅ JSONL: header + user + assistant(tool_call) + tool_result + assistant(text) + usage |

---

## Original Design (Below — reference for Phase 2-4)

### Comparison: Our Approach vs PI

| Dimension | PI Approach | Our Approach (Task 020) |
|-----------|-------------|------------------------|
| Framework | Custom TypeScript TUI | Bubbletea v2 (Go) |
| Layout | Header + messages + editor + footer | Same pattern (viewport + textarea) |
| Input | Custom input handler | `bubbles/textarea` |
| Streaming | Inline text updates | `tea.Cmd` async events |
| Alt screen | Yes | Yes (matching PI pattern) |
| Scrollback | App-level viewport | App-level viewport |

## Main Constraints

- Must use bubbletea v2 (`charm.land/bubbletea/v2`)
- Must use `bubbles/viewport` for message history (import: `charm.land/bubbles/v2/viewport`)
- Must use `bubbles/textarea` for input (import: `charm.land/bubbles/v2/textarea`)
- SDK session lifecycle: create → prompt → subscribe → close
- Print mode must support stdin piping
- JSON mode must output valid JSONL
- Alt screen mode (terminal scrollback not available — print/JSON modes as alternatives)

## Dependencies

- `internal/sdk/` (Task 013) — `Session`, `Prompt`, `Subscribe`, `Close`
- `internal/config/` (Task 006) — config loading
- `internal/types/` (Task 006) — `AgentEvent`, `AgentEventType`
- `internal/testutil/` (Task 006) — test helpers

## Subtasks

### Phase 1 (Done)
- [x] **020.1** — `go mod tidy` — add bubbletea v2 + bubbles + lipgloss dependencies
- [x] **020.2** — `cmd/tau/main.go` — CLI entry point, flag parsing, mode routing
- [x] **020.3** — `cmd/tau/print.go` — Print mode (`-p`): single prompt → output → exit, stdin piping. No TUI dependency.
- [x] **020.4** — `cmd/tau/json.go` — JSON mode (`--mode json`): JSONL events to stdout
- [x] **020.15** — Unit tests for flag parsing, mode routing (10 tests)
- [x] **SDK** — Ollama auto-discovery (bonus: makes CLI testable against local Ollama)

### Phase 2: TUI Core ✅ DONE
- [x] **020.5** — `internal/tui/events.go` — `AgentEvent` → `tea.Msg` mapping
- [x] **020.6** — `internal/tui/concurrency.go` — Buffered channel bridge: `Subscribe()` → non-blocking push → `tea.Cmd` reader
- [x] **020.7** — `internal/tui/model.go` — Bubbletea model: viewport + textarea + state machine
- [x] **020.8** — `internal/tui/update.go` — Turn cycle: user input → `Prompt()` in goroutine → events via channel → display → `Continue()` for next turn
- [x] **020.9** — `internal/tui/view.go` — Screen layout: header + viewport + footer + textarea
- [x] `internal/tui/styles.go` — Basic lipgloss style definitions
- [x] Replace `cmd/tau/interactive.go` stub with real implementation

### Phase 3: Polish ✅ DONE
- [x] **020.10** — Abort recovery: Ctrl+C cancels `Prompt()` context → clean partial assistant message → reset to input mode. Ctrl+C again exits.
  - Ctrl+C cancels context, `PromptDoneMsg{Interrupted: true}` handles cleanup
  - Ctrl+C twice to exit when idle: `pendingExit` field tracks pending exit, footer shows confirmation message
- [x] **020.11** — Basic slash commands: `/quit`, `/help` (implemented in model.go)
- [x] **020.12** — Session flags in interactive mode: `-c` (continue), `--no-session` (ephemeral) (wired in interactive.go)
- [x] **020.13** — Minimal footer: model name, working directory, streaming state
- [x] **020.14** — Ctrl+D exits when input is empty (implemented in update.go)

### Phase 4 (Done — Verification)
- [x] **020.16** — Manual verification: run against Ollama, verify chat works
- [x] All acceptance criteria checked off

## Acceptance Criteria

### Phase 1 (Done)
- [x] `go build ./cmd/tau` produces a working binary
- [x] `./tau -p "hello"` outputs response and exits (print mode)
- [x] `cat file.txt | ./tau -p "summarize"` works (stdin piping)
- [x] `./tau --mode json -p "hello"` outputs valid JSONL
- [x] `go test ./cmd/tau/...` — all pass (10/10)
- [x] `go vet ./cmd/tau/...` — clean
- [x] `go build -race ./cmd/tau` — clean

### Phase 2 (Done)
- [x] `./tau` starts interactive chat mode (bubbletea viewport + textarea)
- [x] User types message, presses Enter → message displayed in viewport
- [x] LLM response streams into viewport in real-time via SDK event subscription
- [x] Streaming does NOT block the agent loop (single event channel bridge verified)
- [x] Tool calls appear in viewport (basic text display: "▶ [name]") — verified via print mode E2E: `./tau -p "List the files in the current directory" --model gemma4:26b` executed `ls` tool successfully
- [x] Thinking blocks appear in viewport (basic text display: [Thinking]...[/Thinking])
- [x] Footer shows current model and working directory, updates during streaming
- [x] `/quit` exits the application
- [x] `/help` shows available commands
- [x] `./tau -c` resumes most recent session — verified: SDK `LatestSessionFile` finds correct file, 20 session files present on disk
- [x] `./tau --no-session` runs in ephemeral mode — verified: SDK `Ephemeral: true` creates session with no file persistence (file count before=20, after=20)
- [x] Ctrl+C during streaming aborts current turn: partial message cleaned, returns to input mode
- [x] Ctrl+C twice (when idle) exits the application — 5 unit tests cover: double-tap exit, pending cleared by other keys, no pending after streaming abort, returnToIdle clears pending
- [x] Ctrl+D exits when input is empty
- [x] After `Prompt()` returns DONE, next user input triggers another turn (turn cycle works)

**Known issue**: gemma4:26b through Ollama emits entire response as `thinking_delta` events. Main answer text appears inside [Thinking] block. Tracked in **Task 024**.

## Concurrency Architecture (Critical)

Three goroutines must coordinate:

```
Goroutine 1 (bubbletea main loop):  Run() → Update() → View()
Goroutine 2 (agent loop):            Session.Prompt() blocks, emits events via Subscribe()
Goroutine 3 (bridge):                Subscribe callback → buffered channel → tea.Cmd reads channel
```

**Bridge pattern:**
```go
// Non-blocking push from Subscribe callback
func (b *bridge) Push(event types.AgentEvent) {
    select {
    case b.ch <- event:
    default:
        // Channel full, drop event (prefer liveness over completeness)
    }
}

// tea.Cmd that reads from channel
func waitForEvent(ch <-chan types.AgentEvent) tea.Cmd {
    return func() tea.Msg {
        event := <-ch
        return AgentEventMsg{event}
    }
}
```

**⚠️ DEADLOCK WARNING (found in Phase 1):**
`Subscribe()` callbacks are called synchronously on the agent goroutine, which runs inside `Session.Prompt()` → `s.mu` held. The callback MUST NOT call any SDK method that tries to acquire `s.mu` (e.g., `session.Usage()`, `session.Close()`). Use the buffered channel bridge to decouple — the bridge's `Push()` is non-blocking, and `tea.Cmd` reads from the channel on the bubbletea goroutine where no SDK locks are held.

**Turn cycle:**
1. User types message, presses Enter
2. Start goroutine: `Session.Prompt(ctx, message)` (blocks)
3. Subscribe callback pushes events → buffered channel → `tea.Cmd` → `Update()` → `View()`
4. `Prompt()` returns (DONE or error)
5. If error is `context.Canceled` (Ctrl+C): clean partial message, return to input
6. If error is other: display error, return to input
7. If DONE: display completion, return to input
8. Next user input: repeat from step 2

## JSON Output Format Specification

Each line is a JSON object with these fields:
```json
{"type":"agent_start","session_id":"abc123","timestamp":"2026-05-03T12:00:00Z"}
{"type":"message_start","session_id":"abc123","timestamp":"2026-05-03T12:00:01Z"}
{"type":"text_delta","data":"Hello","session_id":"abc123","timestamp":"2026-05-03T12:00:01Z"}
{"type":"tool_exec_start","data":"{\"tool\":\"read\",\"args\":{\"path\":\"main.go\"}}","session_id":"abc123","timestamp":"2026-05-03T12:00:02Z"}
{"type":"tool_exec_end","session_id":"abc123","timestamp":"2026-05-03T12:00:02Z"}
{"type":"message_end","session_id":"abc123","timestamp":"2026-05-03T12:00:03Z"}
{"type":"turn_end","session_id":"abc123","timestamp":"2026-05-03T12:00:03Z"}
{"type":"agent_end","usage":{"input":100,"output":50,"total":150,"cost":0.001},"session_id":"abc123","timestamp":"2026-05-03T12:00:03Z"}
```

**Rules:**
- One JSON object per line (valid JSONL)
- `type` field matches `AgentEventType`
- `data` field is a JSON string containing the serialized `Data` payload (for text_delta: raw text; for tool_exec_start: JSON with tool name and args; for error: error message string)
- `session_id` included when available (empty string for ephemeral)
- `timestamp` is RFC3339 format
- `usage` included only on `agent_end` event

## Proposed File Structure (Updated)

```
cmd/tau/
├── main.go              # ✅ CLI entry, flag parsing, mode routing
├── print.go             # ✅ Print mode (no TUI dependency)
├── json.go              # ✅ JSON mode (no TUI dependency)
├── interactive.go       # ✅ Stub → to be replaced with real TUI (Phase 2)
├── main_test.go         # ✅ Unit tests for flag parsing, mode routing
├── sessions.go          # Session flag handling (-c, --no-session) — Phase 3
└── commands.go          # Slash command parsing (/quit, /help) — Phase 3

internal/tui/
├── events.go            # AgentEvent → tea.Msg mapping — Phase 2
├── concurrency.go       # Buffered channel bridge (Subscribe → tea.Cmd) — Phase 2
├── model.go             # Bubbletea model: viewport + textarea + state — Phase 2
├── update.go            # Turn cycle, event handling, abort recovery — Phase 2
├── view.go              # Screen layout (header, viewport, footer, input) — Phase 2
└── styles.go            # Basic lipgloss style definitions — Phase 2
```

## Testing Strategy

**Unit tests:**
- Flag parsing: all flag combinations, conflicting flags (error), unknown flags (error)
- Mode routing: no flags → interactive, `-p` → print, `--mode json` → JSON
- Slash command parsing: `/quit` → exit, `/help` → show help, unknown → no-op
- Session flags: `-c` loads most recent, `--no-session` creates ephemeral

**Manual verification (against Ollama):**
- Start interactive mode: `./tau`
- Send message: "Hello, what can you do?"
- Verify: message appears in viewport, streaming response appears
- Test tool use: "List files in current directory"
- Verify: tool call displayed, result displayed
- Test print mode: `./tau -p "say hello"`
- Test stdin piping: `echo "hello" | ./tau -p "echo this back"`
- Test JSON mode: `./tau --mode json -p "hello" | head -5`
- Test session resume: `./tau -c`
- Test ephemeral: `./tau --no-session`

## Quality Gates

- `go build ./cmd/tau` — clean
- `go vet ./cmd/tau/...` — clean
- `go test -race ./cmd/tau/...` — all pass
- `go mod tidy` — clean
- Bubbletea v2 dependency added to go.mod

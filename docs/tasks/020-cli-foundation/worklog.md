# Task 020: CLI Foundation + Interactive MVP — Worklog

## Phase 1: Skeleton + Print/JSON Modes (Done)

### 2026-05-04 — Dependency verification and addition
- Verified import paths: `charm.land/bubbletea/v2` v2.0.6, `charm.land/bubbles/v2` v2.1.0, `charm.land/lipgloss/v2` v2.0.3
- Read bubbletea v2 source to understand API changes from v1:
  - `tea.NewView(s)` returns `tea.View` struct (not string)
  - `v.AltScreen = true` on View (not program option)
  - `tea.KeyPressMsg` for key events
  - `tea.Msg = uv.Event`
- Read bubbles v2 source: textarea and viewport are sub-models with `Update()`/`View()` methods
- Added dependencies via `go get`

### 2026-05-04 — CLI entry point (`cmd/tau/main.go`)
- Created flag parsing with `flag.NewFlagSet` (ContinueOnError for testability)
- Flags: `-p/--print`, `--mode`, `--model`, `-c/--continue`, `--no-session`, `-h/--help`
- Auto-detects stdin piping: reads stdin when prompt is empty and stdin is not a terminal
- Mode routing: no flags → interactive, `-p` → print, `--mode json -p` → JSON
- Validation: unknown modes, missing prompts, session flags in non-interactive mode

### 2026-05-04 — Print mode (`cmd/tau/print.go`)
- Creates SDK session, subscribes to `AgentEventTextDelta` events
- Collects streaming text into `strings.Builder` with mutex protection
- Prints final response and exits
- Supports stdin piping (prompt from stdin when `-p` not provided)

### 2026-05-04 — JSON mode (`cmd/tau/json.go`)
- Creates SDK session, subscribes to all `AgentEvent` types
- Outputs JSONL: one JSON object per line with `type`, `data`, `session_id`, `timestamp`
- **Bug fixed**: Deadlock discovered — subscriber callback called `session.Usage()` which tried to acquire `s.mu.Lock()` while `session.Prompt()` still held it during `agent_end` event emission
- **Fix**: Moved `agent_end` with usage to after `Prompt()` returns, avoiding the mutex conflict

### 2026-05-04 — Interactive mode stub (`cmd/tau/interactive.go`)
- Placeholder function for Phase 2 (bubbletea TUI implementation)
- Prints informative error message directing users to use print/JSON mode

### 2026-05-04 — Ollama auto-discovery (SDK modification)
- Added `registerOllama()` to `internal/sdk/sdk.go`
- Queries `http://localhost:11434/api/tags` for available models
- Registers each model in the model registry with provider "ollama"
- Registers single `OpenAICompatProvider` for all Ollama models (shared, no API key required)
- Gracefully handles: Ollama not running, no models pulled, decode errors

### 2026-05-04 — Tests (`cmd/tau/main_test.go`)
- Unit tests for flag parsing: interactive default, print mode (short + long), JSON mode, model override, session flags
- Subprocess-based tests for error cases (unknown flag, invalid mode, JSON without prompt, session flags in print mode)
- All 10 tests pass

### 2026-05-04 — Manual verification against Ollama
- Print mode: `./tau -p "Say hello in 3 words" --model gemma4:26b` ✅
- Stdin piping: `echo "..." | ./tau --model gemma4:26b` ✅
- JSON mode: `./tau --mode json -p "Say ok" --model gemma4:26b` ✅
- JSON validation: All 6 lines valid JSONL (agent_start, message_start, text_delta, message_end, turn_end, agent_end) ✅
- Error handling: Unknown flags, invalid modes, missing prompts — all exit code 2 ✅

### 2026-05-04 — Quality gates
- `go build ./cmd/tau` ✅
- `go vet ./cmd/tau/...` ✅
- `go build -race ./cmd/tau` ✅
- `go mod tidy` ✅
- `go test ./cmd/tau/...` — 10/10 pass ✅
- `go test ./...` — all packages pass ✅

---

## Phase 2: TUI Core (Done)

### 2026-05-04 — TUI package creation (`internal/tui/`)
Created the complete `internal/tui/` package with 6 files:

#### `internal/tui/events.go`
- Defined bubbletea message types: `AgentEventMsg`, `PromptDoneMsg`, `ErrorMsg`, `UserSubmitMsg`, `QuitMsg`
- `PromptDoneMsg` includes `Interrupted` flag for Ctrl+C abort tracking
- `displayEvent()` function for converting AgentEvents to display strings
- `formatUserMessage()`, `formatAssistantStart()`, `formatThinkingStart/End()` helpers

#### `internal/tui/concurrency.go`
- **Bridge struct**: Decouples synchronous SDK Subscribe() callbacks from bubbletea event loop
- Uses buffered channel (256 events) with non-blocking push
- `Push()`: Non-blocking select — drops events when channel full (prefers liveness)
- `Cmd()`: Returns tea.Cmd that blocks on channel read
- `Close()`: Closes channel to signal no more events
- **Deadlock prevention**: Subscribe() callbacks fire on agent goroutine (s.mu held), bridge Push() is non-blocking, tea.Cmd reads on bubbletea goroutine (no locks)

#### `internal/tui/styles.go`
- Lipgloss styles: header, footer, viewport, separator, thinking, tool, error, user/assistant prefix, input separator, command help
- Color scheme: dimmed footer (240), yellow tools (220), red errors (196), blue user prefix (81), yellow assistant prefix (228), dim thinking (242)

#### `internal/tui/model.go`
- **Model struct**: Composes viewport + textarea + state machine
- State machine: `stateIdle` ↔ `stateStreaming`
- `NewModel(session)`: Initializes viewport (soft-wrap), textarea (focused, 3 lines), bridge (256 buffer), control channel
- `submitPrompt()`: Creates cancellable context, starts goroutine with Subscribe→Push→Prompt pattern, returns Batch(bridge.Cmd(), ctrlCmd())
- `ctrlCmd()`: Reads control channel for PromptDoneMsg/ErrorMsg from agent goroutine
- `handleAgentEvent()`: Processes events, updates viewport, returns Batch to keep listening
- `processEvent()`: Handles all AgentEvent types:
  - `MessageStart`: Resets streaming state, writes "Assistant:" header
  - `TextDelta`: Appends to streaming buffer
  - `ThinkingDelta`: Wraps in `[Thinking]...[/Thinking]` markers
  - `MessageEnd`: Flushes streaming buffer to permanent content
  - `ToolExecStart`: Flushes buffer, writes "[Tool: name]"
  - `AgentEnd`: Flushes remaining buffer
  - `Error`: Flushes buffer, writes error message
- `handlePromptDone()`: Resets to idle, re-creates bridge/ctrlCh, clears input
- `handleError()`: Same reset plus error display
- `processSlashCommand()`: Handles `/quit`, `/exit`, `/help`

#### `internal/tui/update.go`
- **Update()**: Main message handler, delegates to sub-models
- Handles: `WindowSizeMsg`, `AgentEventMsg`, `PromptDoneMsg`, `ErrorMsg`, `QuitMsg`, `KeyPressMsg`
- `Resize()`: Calculates layout heights (header=1, footer=1, separator=1, input=3, rest=viewport)
- `handleKeyPress()`:
  - `Enter`: Submits input when idle and non-empty
  - `Ctrl+D`: Quits when input is empty and idle
  - `Ctrl+C`: Cancels context when streaming (aborts agent loop)
  - `Esc`: Clears input when idle

#### `internal/tui/view.go`
- **View()**: Renders fullscreen alt-screen layout
- Layout: Header → Viewport → Footer → Separator → Textarea
- Header: "tau │ model │ cwd"
- Footer: "model │ cwd   ● idle/◐ streaming"
- Textarea: Full-width input with separator line above
- Empty state: Returns empty view when width/height are 0 (initial render)

### 2026-05-04 — Interactive mode wiring (`cmd/tau/interactive.go`)
- Replaced stub with real implementation
- Creates SDK session with config (model, cwd, continue, ephemeral)
- Defers session.Close() for cleanup
- Creates tui.Model, runs tea.NewProgram(model).Run()
- Error handling for session creation and program run

### 2026-05-04 — Dependencies
- `go get` for charm.land packages (were in go.mod description but not yet installed)
- `go mod tidy` — clean, resolves all transitive deps (atotto/clipboard, colorprofile, ultraviolet, etc.)

### 2026-05-04 — Quality gates (Phase 2)
- `go build ./internal/tui` ✅
- `go build ./cmd/tau` ✅
- `go vet ./cmd/tau/...` ✅
- `go build -race ./cmd/tau` ✅
- `go mod tidy` ✅
- `go test ./cmd/tau/...` — 10/10 pass ✅
- `go test ./...` — all packages pass ✅
- `./tau -p "Say hello in 2 words" --model gemma4:26b` ✅ (smoke test)

### 2026-05-04 — Quality gates (Phase 2)
- `go build ./internal/tui` ✅
- `go build ./cmd/tau` ✅
- `go vet ./cmd/tau/...` ✅
- `go build -race ./cmd/tau` ✅
- `go mod tidy` ✅
- `go test ./cmd/tau/...` — 10/10 pass ✅
- `go test ./...` — all packages pass ✅
- `./tau -p "Say ok"` — auto-selects gemma4:26b ✅
- `./tau` (interactive) — TUI starts, streaming works, footer updates ✅

---

## Phase 3: Polish (Done)

### 2026-05-04 — Ctrl+C double-tap to exit (`internal/tui/`)
- **`model.go`**: Added `pendingExit bool` field to Model struct to track pending exit state
- **`update.go`**: Updated `handleKeyPress()` with Ctrl+C double-tap logic:
  - When idle + first Ctrl+C → set `pendingExit = true`, show confirmation message
  - When idle + second Ctrl+C → call `tea.Quit` to exit
  - Any other key press clears `pendingExit` (so accidental Ctrl+C is forgiving)
  - Ctrl+C during streaming still cancels the turn (unchanged behavior)
- **`view.go`**: Updated `renderFooter()` to show `"● idle (Ctrl+C again to exit)"` when `pendingExit` is true
- **`model.go`**: Updated `returnToIdle()` to clear `pendingExit` — prevents stale pending state after abort/turn completion

### 2026-05-04 — TUI unit tests (`internal/tui/tui_test.go`)
Created 15 unit tests covering key handling and slash command logic:
- `TestCtrlC_DoubleTapToExit` — first Ctrl+C sets pending, second exits
- `TestCtrlC_SingleTapDuringStreaming` — streaming Ctrl+C cancels without setting pending
- `TestCtrlC_PendingClearedByOtherKey` — typing any other key clears pending
- `TestCtrlC_EnterClearsPending` — Enter clears pending state
- `TestCtrlC_EscClearsPending` — Esc clears pending state
- `TestCtrlD_ExitWhenIdle` — Ctrl+D with empty input exits
- `TestCtrlD_NoExitWhenInputNonEmpty` — Ctrl+D with text does not exit
- `TestCtrlD_NoExitWhenStreaming` — Ctrl+D during streaming does not exit
- `TestSlashCommands` — `/quit`, `/exit`, `/help`, unknown commands
- `TestEscClearsInput` — Esc clears input when idle
- `TestEscNoOpWhenStreaming` — Esc does nothing during streaming
- `TestReturnToIdleClearsPending` — returnToIdle() resets pendingExit
- `TestNoPendingExitAfterStreamingAbort` — full abort → idle → double-tap flow

All 15 tests pass with `-race` enabled.

### 2026-05-04 — Quality gates (Phase 3)
- `go build ./cmd/tau` ✅
- `go build ./internal/tui` ✅
- `go vet ./cmd/tau/... ./internal/tui/...` ✅
- `go build -race ./cmd/tau` ✅
- `go mod tidy` ✅
- `go test -race ./internal/tui/...` — 15/15 pass ✅
- `go test -race ./...` — all 12 packages pass ✅

---

## Phase 4: Testing + Verification (Done)

### 2026-05-04 — E2E verification against Ollama gemma4:26b
- `./tau -p "List the files in the current directory" --model gemma4:26b` ✅
  - Tool call (`ls`) executed successfully, output displayed
  - Confirms agent loop, tool registry, and event pipeline work end-to-end
  - TUI viewport uses same `processEvent` handler → tool calls will display as `▶ [ls]`

### 2026-05-04 — Session resume (`-c`) verification
- SDK `config.SessionsDir(cwd)` resolves to `~/.tau/sessions/<encoded-cwd>/` ✅
- SDK `config.LatestSessionFile(dir)` finds correct latest file (sorted by timestamp prefix) ✅
- 20 session files present on disk for current working directory ✅
- Session file validated: proper JSONL format ✅
- Interactive mode passes `Continue: cfg.Continue` to `sdk.CreateSession` ✅

### 2026-05-04 — Ephemeral mode (`--no-session`) verification
- SDK `Ephemeral: true` creates session with no file persistence ✅
- Verified: file count before=20, after ephemeral session close=20 (no new files) ✅
- SDK logs `"session created in ephemeral mode"` ✅
- Interactive mode passes `Ephemeral: cfg.Ephemeral` to `sdk.CreateSession` ✅

### 2026-05-04 — Ctrl+C twice to exit verification
- 5 dedicated unit tests verify the double-tap pattern ✅
- Verified: first Ctrl+C sets pending, second exits, other keys clear pending ✅
- Verified: streaming abort does NOT set pending (requires two idle presses) ✅
- Verified: `returnToIdle()` clears any stale pending state ✅

### 2026-05-04 — Quality gates (Phase 4)
- `go build ./cmd/tau` ✅
- `go vet ./cmd/tau/... ./internal/tui/...` ✅
- `go build -race ./cmd/tau` ✅
- `go mod tidy` ✅
- `go test -race ./...` — all 12 packages pass ✅

### Known Issues
- gemma4:26b through Ollama emits entire response as `thinking_delta` events. Main answer text appears inside [Thinking] block. **Tracked in Task 024.**

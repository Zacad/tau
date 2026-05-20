# Task 023: Tab Completion, Testing & Polish

## Why

Tasks 020-022 deliver a fully functional CLI with chat, styled messages, and session management. This task adds the finishing touches: tab completion for commands and skills, comprehensive test coverage, E2E testing with mock provider, and production-ready error handling.

## Main Constraints

- Tab completion must work within bubbletea's textarea component
- E2E tests must run without external dependencies (mock provider only)
- Error messages must be user-friendly, not stack traces
- Exit codes must follow conventions (0 = success, 1 = error)
- All code must pass go vet, go build -race, go mod tidy

## Dependencies

- Task 020 (CLI Foundation) — completed first
- Task 021 (Message Rendering) — completed first
- Task 022 (Session Management UI) — completed first
- `internal/testutil/` — mock provider, temp filesystem helpers
- `internal/sdk/` — SDK interface for E2E testing

## Subtasks

- [x] **023.1** — Tab completion for slash commands (`/` → show completions). Inline completion (append best match), not overlay.
- [x] **023.2** — Tab completion for skill names (`/skill:` → show completions). Inline completion.
- [x] **023.3** — Mock provider injection: add `--mock` flag or `PRAXIS_MOCK_URL` env var that forces OpenAI-compatible provider to a custom base URL for E2E testing.
- [x] **023.4** — Graceful error handling: user-friendly messages, non-zero exit codes
- [x] **023.5** — Startup error handling: no config, no auth, no sessions
- [x] **023.6** — Signal handling: SIGINT (Ctrl+C), SIGTERM graceful shutdown
- [x] **023.7** — Session close/cleanup on exit
- [x] **023.8** — Comprehensive unit tests: flag parsing, command processing, rendering, JSON serialization, concurrency bridge
- [x] **023.9** — E2E test: `exec.Command` runs CLI binary with mock HTTP server (streaming response, tool calls)
- [x] **023.10** — E2E test: print mode with mock provider, verify stdout
- [x] **023.11** — E2E test: JSON mode with mock provider, verify JSONL output (parseable, one object per line)
- [x] **023.12** — `--help` flag: comprehensive CLI help text
- [x] **023.13** — `--version` flag: show version information
- [x] **023.14** — Documentation: README for cmd/tau

## Acceptance Criteria

- [x] Typing `/` + Tab shows command completions (inline: appends best match)
- [x] Typing `/skill:` + Tab shows skill name completions (inline: appends best match)
- [x] Invalid API key → user-friendly error message, exit code 1
- [x] Missing config file → graceful fallback to defaults
- [x] Ctrl+C during streaming → abort current turn, return to input
- [x] Ctrl+C twice (when idle) → exit application
- [x] SIGTERM → save session, exit gracefully
- [x] Session file flushed and closed on exit
- [x] `./tau --help` shows comprehensive help text
- [x] `./tau --version` shows version information
- [x] `go test -race ./cmd/tau/... ./internal/tui/...` — all pass
- [x] E2E test: mock provider returns canned streaming response, verify in viewport
- [x] E2E test: print mode outputs expected text, exit code 0
- [x] E2E test: JSON mode outputs valid JSONL, each line parseable independently
- [x] `go vet ./cmd/tau/... ./internal/tui/...` — clean
- [x] `go build -race ./cmd/tau` — clean
- [x] `go mod tidy` — clean

## Implementation

All subtasks completed. See `worklog.md` for details.

### New Files
```
cmd/tau/
├── errors.go            # Error handling and user-friendly messages
├── e2e_test.go          # End-to-end tests with mock provider
└── README.md            # CLI documentation

internal/tui/
├── completion.go        # Tab completion logic
└── completion_test.go   # Tests for tab completion
```

### Modified Files
- `cmd/tau/main.go` — --mock flag, --version flag, enhanced --help
- `cmd/tau/interactive.go` — Signal handling, exitError usage
- `cmd/tau/print.go` — exitError usage
- `cmd/tau/json.go` — exitError usage
- `cmd/tau/main_test.go` — Mock URL, friendlyError, exit constant tests
- `internal/tui/update.go` — Tab key handling in handleKeyPress()
- `internal/sdk/sdk.go` — TAU_MOCK_URL support, mock model resolution

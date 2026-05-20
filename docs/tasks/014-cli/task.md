# Task 014: CLI

## Why

The CLI is the user-facing interface for tau. It provides interactive chat mode, print mode for scripting, and JSON mode for debugging. This is the final implementation task, consuming the SDK (013).

## Comparison Analysis: CLI vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Framework | Node.js CLI with TUI | Go CLI with readline |
| Modes | Interactive only (TUI) | Interactive (default), Print (`-p`), JSON (`--mode json`) |
| Session Flags | Complex session management | `-c`, `-r`, `--session`, `--no-session` |
| Model Selection | Interactive picker | Pattern matching + interactive selection on ambiguity |
| Skill Commands | `/skill:name` in TUI chat | `/skill:name` in interactive mode |
| Auth Override | Config-based only | `--api-key` flag (highest priority in auth chain) |
| Tool Control | Extension-based | `--tools` flag, `--read-only` flag |
| Print Mode | N/A | `tau -p "prompt"` — single prompt, output to stdout, exit |
| JSON Mode | N/A | `--mode json` — JSONL events to stdout for debugging/piping |

## Main Constraints

- Interactive mode needs a readline library (e.g., `github.com/chzyer/readline`)
- Print mode must support stdin piping for scripting
- JSON mode must output valid JSONL — one JSON object per line
- All flags must be parsed before SDK initialization
- Model resolution delegates to SDK for interactive selection
- Skill invocation (`/skill:name`) must be handled in interactive mode input processing

## Dependencies

- `internal/sdk/` (Task 013)
- `internal/testutil/` (Task 006)

## Subtasks

- [ ] **014.1** — `cmd/tau/main.go` — CLI entry point, flag parsing
- [ ] **014.2** — `cmd/tau/interactive.go` — Interactive chat mode with readline
- [ ] **014.3** — `cmd/tau/print.go` — Print mode (`-p`)
- [ ] **014.4** — `cmd/tau/json.go` — JSON output mode (`--mode json`)
- [ ] **014.5** — `cmd/tau/sessions.go` — Session management flags (`-c`, `-r`, `--session`, `--no-session`)
- [ ] **014.6** — Interactive command processing: `/skill:name` (loads skill via SDK), `/model` (triggers SDK model resolution + interactive selection), `/name <new-name>` (calls SDK `Session.Rename()`)
- [ ] **014.7** — Graceful error handling and exit codes
- [ ] **014.8** — Unit tests for flag parsing, mode routing
- [ ] **014.9** — Automated end-to-end test: `exec.Command` runs CLI binary with mock provider, verifies output

## Acceptance Criteria

- [ ] Interactive mode: readline input, streaming output, Ctrl+C abort
- [ ] Print mode (`-p`): single prompt → output → exit, supports stdin piping
- [ ] JSON mode (`--mode json`): structured JSONL events to stdout
- [ ] Session management: new, continue (`-c`), resume (`-r`), specific (`--session`), ephemeral (`--no-session`)
- [ ] Model selection: SDK returns `[]types.Model` + ambiguity flag; CLI handles interactive user prompt
- [ ] Auth override via `--api-key` flag (highest priority in auth chain)
- [ ] Tool allowlisting via `--tools` flag — passed to SDK `SessionOptions.ToolAllowlist`
- [ ] Read-only mode via `--read-only` flag — passed to SDK `SessionOptions.ReadOnly`
- [ ] Context files override via `--no-context-files` flag
- [ ] Skill invocation: `/skill:name` command loads full skill content into active session
- [ ] Graceful error handling: user-friendly messages, non-zero exit on error
- [ ] Unit tests for flag parsing, mode routing (use `testutil/` helpers)
- [ ] Automated end-to-end test: `exec.Command` runs CLI binary with mock provider, verifies JSON output

## Testing & Verification Strategy

**Unit tests**:
- Flag parsing: all flag combinations, conflicting flags (error), missing required values (error), unknown flags (error)
- Mode routing: no flags → interactive, `-p` → print, `--mode json` → JSON
- Session flags: `-c` loads most recent, `-r` lists sessions, `--session <id>` loads specific, `--no-session` creates ephemeral
- Command parsing: `/skill:name`, `/model`, `/name <new-name>` — verify correct SDK method called

**Integration tests**:
- Print mode: `tau -p "hello"` with mock provider → verify text output to stdout, exit code 0
- JSON mode: `tau --mode json -p "hello"` with mock provider → verify JSONL events on stdout, parseable JSON
- Stdin piping: `echo "hello" | tau -p` → verify stdin read as prompt

**Automated E2E test** (014.9):
- `exec.Command` builds and runs CLI binary against mock HTTP server
- Verify: startup → prompt → streaming output → tool call → result → completion → exit code 0
- Error path: invalid API key → user-friendly error message, exit code 1

**Quality gates**:
- CLI binary builds as single static binary (`go build -o tau ./cmd/tau`)
- `go test ./cmd/tau/...` — all pass
- E2E test runs without external dependencies (mock provider only)

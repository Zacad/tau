# tau CLI

The tau command-line interface — a thin consumer of the Tau SDK.

## Installation

```bash
go build -o tau ./cmd/tau
```

For production builds with version info:

```bash
go build -ldflags "-X main.version=$(git describe --tags)" -o tau ./cmd/tau
```

## Usage

### Interactive Mode (default)

Full-screen TUI with chat history, session management, and slash commands.

```bash
tau                          # Start interactive chat
tau -c                       # Resume last session
tau -r                       # Open session picker
tau --model gpt-4o           # Use specific model
tau --no-session             # Ephemeral mode (no persistence)
```

### Print Mode

Single prompt, plain text output, then exit. Supports stdin piping.

```bash
tau -p "Explain quantum computing"
echo "What is Go?" | tau
cat prompt.txt | tau
```

### JSON Mode

Single prompt, JSONL event output to stdout. One JSON object per line, parseable by downstream tools.

```bash
tau --mode json -p "Hello"
echo "Hi" | tau --mode json
```

## Flags

| Flag | Description |
|------|-------------|
| `-p, --print TEXT` | Print mode: send TEXT as prompt, output response, exit |
| `--mode MODE` | Output mode: `interactive` (default), `print`, `json` |
| `--model PATTERN` | Model pattern or ID (e.g., `gpt-4o`, `ollama/llama3`) |
| `-c, --continue` | Resume most recent session |
| `-r` | Open session picker to resume a past session |
| `--no-session` | Run in ephemeral mode (no session persistence) |
| `--mock URL` | Use mock provider at URL (for E2E testing) |
| `--version` | Show version information |
| `-h, --help` | Show help |

## Interactive Mode Commands

| Command | Description |
|---------|-------------|
| `/quit`, `/exit` | Exit the application |
| `/help` | Show help message |
| `/name <name>` | Rename the current session |
| `/session` | Show session information |
| `/model` | Change the active model |
| `/compact` | Trigger context compaction |
| `/clear` | Clear the viewport |
| `/skills` | List available skills |
| `/skill:<name>` | Load a skill's content |

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Shift+Enter` | New line |
| `Tab` | Auto-complete commands and skill names |
| `Ctrl+D` | Quit (when input is empty) |
| `Ctrl+C` | Abort current response / Exit (double-tap when idle) |
| `Esc` | Clear input |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `TAU_HOME` | Config directory (default: `~/.tau`) |
| `TAU_DEFAULT_MODEL` | Default model to use |
| `TAU_MOCK_URL` | Mock provider URL for testing |
| `TAU_DEBUG=1` | Enable debug logging |
| `OPENAI_API_KEY` | OpenAI API key |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `GOOGLE_API_KEY` | Google API key |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime error (provider failure, session error, etc.) |
| 2 | CLI usage error (invalid flags, bad mode, etc.) |

## E2E Testing

Run E2E tests with a mock HTTP server:

```bash
go test -tags=e2e ./cmd/tau/... -v
```

The `--mock` flag or `TAU_MOCK_URL` env var directs the SDK to an OpenAI-compatible endpoint, enabling tests without external dependencies.

## Examples

```bash
# Start interactive chat with a specific model
tau --model ollama/llama3

# Resume your last conversation
tau -c

# Quick one-off question
tau -p "What is the capital of France?"

# Pipe a file as a prompt
cat analysis.md | tau -p "Summarize this:"

# Get structured JSON output for scripting
tau --mode json -p "List 3 benefits of Go" | jq '.data'
```

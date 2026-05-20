# Tau

<p align="center">
  <img src="assets/logo.png" alt="Tau Logo" width="180" height="180">
</p>

<p align="center">
  <strong>Minimalist agentic coding tool — inspired by PI, written in Go.</strong>
</p>

<p align="center">
  A lightweight TUI agent that orchestrates skills and subagents to execute workflows across coding, research, and analysis tasks.
</p>

---

## Goal

Tau is a personal lightweight agent.

Built for a single user who wants skills and subagents working out of the box — without installing or configuring extensions.

## Features

- **Orchestrator Model** — Agent coordinates skills and subagents to execute defined workflows
- **Built-in Skills** — `skill-builder` and `subagent-builder` ready to use
- **Built-in Subagents** — Researcher, Reviewer, Implementor, Security Reviewer, QA
- **Multi-Provider Support** — OpenAI, Anthropic, Google Gemini, OpenRouter, OpenCode Zen, Ollama, and more
- **Session Management** — JSONL-based persistence with auto-naming, resume, and compaction
- **TUI Chat Interface** — Full-screen interactive chat with streaming output
- **Tool System** — File read/write/edit, bash execution, grep, find, web search, web fetch
- **Skills System** — Agent Skills standard compliant, progressive disclosure
- **Thinking/Reasoning Support** — Uniform handling across all providers

## Installation

```bash
git clone https://github.com/adam/tau.git
cd tau
go build -o tau ./cmd/tau
```

For production builds with version info:

```bash
go build -ldflags "-X main.version=$(git describe --tags)" -o tau ./cmd/tau
```

## Quick Start

```bash
# Start interactive chat
tau

# Use a specific model
tau --model claude-sonnet-4-20250514

# Resume last session
tau -c

# Quick one-off question
tau -p "Explain the architecture of this project"

# Pipe input
cat README.md | tau -p "Summarize this file"
```

## Usage

### Modes

| Mode | Description |
|------|-------------|
| `interactive` (default) | Full-screen TUI with chat history and session management |
| `print` | Single prompt, plain text output, then exit |
| `json` | Single prompt, JSONL event output to stdout |

```bash
tau -p "Hello"                    # print mode (auto when -p used)
tau --mode json -p "Hello"        # json mode
tau                               # interactive mode
```

### Flags

| Flag | Description |
|------|-------------|
| `-p, --print TEXT` | Print mode: send TEXT as prompt, output response, exit |
| `--mode MODE` | Output mode: `interactive`, `print`, `json` |
| `--model PATTERN` | Model pattern or ID (e.g., `gpt-4o`, `ollama/llama3`) |
| `-c, --continue` | Resume most recent session |
| `-r` | Open session picker |
| `--no-session` | Ephemeral mode (no persistence) |
| `--tools LIST` | Restrict available tools (e.g., `read,grep,find,ls`) |
| `--read-only` | Disable write, edit, and bash tools |
| `--mock URL` | Use mock provider at URL (for testing) |
| `--version`, `-v` | Show version information |
| `-h, --help` | Show help |

### Interactive Commands

| Command | Description |
|---------|-------------|
| `/quit`, `/exit` | Exit the application |
| `/help` | Show help message |
| `/new` | Start a new session |
| `/resume` | Resume a previous session (session picker) |
| `/name <name>` | Rename current session |
| `/session` | Show session information |
| `/model` | Change active model (picker) |
| `/thinking` | Set thinking/reasoning level (picker) |
| `/compact` | Trigger context compaction |
| `/clear` | Clear viewport |
| `/skills` | List available skills |
| `/skill:<name>` | Load a skill's content |
| `/agents` | List subagent types and user-defined agents |
| `/connect` | Connect to a provider (multi-step wizard) |
| `/disconnect` | Disconnect/disable a provider (multi-step wizard) |
| `/reload` | Reload custom commands |

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Shift+Enter` | New line |
| `Tab` | Auto-complete commands and skill names |
| `Ctrl+D` | Quit (when input empty) |
| `Ctrl+C` | Abort response / Exit (double-tap when idle) |
| `Ctrl+P` | Open command palette |
| `Esc` | Clear input |

### Custom Commands

Define custom commands as markdown files with YAML frontmatter in:
- **Project**: `.tau/commands/` (walk up from cwd)
- **Global**: `~/.tau/commands/`

Commands are loaded automatically and reload with `/reload`.

## Configuration

### Config File

Location: `~/.tau/config.json`

```json
{
  "providers": {
    "openai": {
      "model": "gpt-4o"
    },
    "anthropic": {
      "model": "claude-sonnet-4-20250514"
    },
    "google": {
      "model": "gemini-2.5-pro"
    },
    "openrouter": {
      "enabled": true,
      "models": [
        "anthropic/claude-sonnet-4-20250514",
        "openai/gpt-4o-2024-05-13"
      ]
    }
  },
  "default_model": "claude-sonnet-4-20250514",
  "compaction": {
    "reserve_tokens": 16384,
    "keep_recent_tokens": 20000
  },
  "subagent_timeout": "5m",
  "tool_allowlist": null,
  "read_only": false,
  "load_context_files": true,
  "search": {
    "backend": "auto",
    "searxng_url": "http://localhost:8964"
  }
}
```

### Authentication

Location: `~/.tau/auth.json` (file permissions: `0600`)

```json
{
  "openai": "sk-actual-key-value",
  "anthropic": "$ANTHROPIC_API_KEY",
  "google": "!security find-generic-password -ws 'google-ai'"
}
```

Key formats supported:
- **Literal**: `"sk-actual-key-value"`
- **Environment variable**: `"$MY_API_KEY"` — resolved at runtime
- **Shell command**: `"!command"` — output used as key

### Auth Resolution Chain

1. CLI flag: `--api-key <key>` (highest priority)
2. `auth.json`: `~/.tau/auth.json`
3. Environment variables: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, etc.
4. Config file: `~/.tau/config.json` (fallback)

### Environment Variables

| Variable | Description |
|----------|-------------|
| `TAU_HOME` | Config directory (default: `~/.tau`) |
| `TAU_DEFAULT_MODEL` | Default model to use |
| `TAU_MOCK_URL` | Mock provider URL for testing |
| `TAU_DEBUG=1` | Enable debug logging |
| `OPENAI_API_KEY` | OpenAI API key |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `GOOGLE_API_KEY` | Google API key |

### Context Files

Tau loads `AGENTS.md` and `CLAUDE.md` files to customize agent behavior:

1. Walks up from cwd through parent directories
2. Loads global `~/.tau/AGENTS.md` if present
3. Files are concatenated and prepended to system prompt

Override with `--no-context-files` flag.

### Skills Discovery

| Tier | Path | Priority |
|------|------|----------|
| Built-in | Embedded in binary | Highest |
| Global | `~/.tau/skills/`, `~/.agents/skills/` | Medium |
| Project | `.agents/skills/` (walk up from cwd) | Lowest (overrides) |

Skills follow the [Agent Skills standard](https://agentskills.io) — `SKILL.md` with YAML frontmatter + markdown body.

## Architecture

```
cmd/tau/                    # CLI entry point
internal/
├── agent/                  # Agent loop, state machine, events
├── session/                # JSONL persistence, lifecycle, compaction
├── tools/                  # Tool definitions and execution
├── provider/               # LLM API abstraction, streaming
├── skills/                 # Discovery, parsing, prompt formatting
├── subagent/               # Subagent lifecycle, context management
├── config/                 # Settings loading, path resolution
└── sdk/                    # Public SDK — high-level session API
```

See [ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed documentation.

## Supported Providers

| Provider | API Type | Notes |
|----------|----------|-------|
| OpenAI | `openai-responses` | Direct HTTP |
| Anthropic | `anthropic-messages` | Direct HTTP, thinking support |
| Google Gemini | `google-generative-ai` | Direct HTTP |
| OpenRouter | `openai-completions` | 300+ models |
| OpenCode Zen | Multi-endpoint | Routes by model family |
| OpenCode Go | `openai-completions` | OpenAI-compatible |
| Ollama | `openai-completions` | Local models |
| llama.cpp | `openai-completions` | Local server |
| LM Studio | `openai-completions` | Local server |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Runtime error |
| 2 | CLI usage error |

## Testing

```bash
# Unit tests
go test ./...

# E2E tests with mock provider
go test -tags=e2e ./cmd/tau/... -v
```

Local Ollama for testing:

```bash
cd ollama
docker compose up -d
curl -s http://localhost:11434/api/generate \
  -d '{"model": "ministral-3:14b", "prompt": "Hello", "stream": false}'
```

## License

MIT License — see [LICENSE](LICENSE) for details.

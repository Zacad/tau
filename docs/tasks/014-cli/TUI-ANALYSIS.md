# TUI Analysis: PI vs Tau Task 014

## 1. PI TUI Feature Inventory

### Layout (top to bottom)
| Zone | PI Feature | Description |
|------|-----------|-------------|
| **Header** | Startup banner | Shows shortcuts hint, loaded context files, skills, extensions |
| **Message Area** | Scrollable history | User messages, assistant responses, tool calls/results, notifications, errors, extension UI |
| **Tool Rendering** | Collapsible tool output | `Ctrl+O` cycles: collapsed (summary) ↔ expanded (full output) |
| **Thinking Blocks** | Collapsible reasoning | `Ctrl+T` to collapse/expand; configurable levels (`off`, `minimal`, `low`, `medium`, `high`, `xhigh`) |
| **Editor** | Rich input line | Syntax, `@` file references, tab completion, multi-line (`Shift+Enter`) |
| **Footer** | Status bar | Working directory, session name, token/cache usage, cost, context usage, current model |

### Editor Features
| Feature | Details |
|---------|---------|
| File reference | `@` fuzzy-search project files |
| Path completion | Tab to complete |
| Multi-line input | `Shift+Enter` |
| Image paste | `Ctrl+V` paste images, drag-and-drop onto terminal |
| Bash inline | `!command` runs and sends output, `!!command` runs silently |
| External editor | `Ctrl+G` opens `$VISUAL` or `$EDITOR` |
| Modal editing | Extensible (vim mode example provided) |
| Emacs keybindings | `Ctrl+A/E` line start/end, `Ctrl+B/F` cursor movement, `Ctrl+W` delete word, `Ctrl+U/K` delete line, `Ctrl+Y` yank, `Ctrl+-` undo |
| Kill ring | `Ctrl+Y` paste, `Alt+Y` cycle through deleted text |
| Jump navigation | `Ctrl+]` jump forward, `Ctrl+Alt+]` jump backward |

### Slash Commands
| Command | Description |
|---------|-------------|
| `/login`, `/logout` | OAuth authentication |
| `/model` | Switch models (interactive selector) |
| `/scoped-models` | Enable/disable models for cycling |
| `/settings` | Thinking level, theme, message delivery, transport |
| `/resume` | Pick from previous sessions |
| `/new` | Start new session |
| `/name <name>` | Set session display name |
| `/session` | Show session info (file, ID, messages, tokens, cost) |
| `/tree` | Jump to any point in session history, continue from there |
| `/fork` | Create new session from previous user message |
| `/clone` | Duplicate active branch into new session |
| `/compact [prompt]` | Manual context compaction |
| `/copy` | Copy last assistant message to clipboard |
| `/export [file]` | Export session to HTML |
| `/share` | Upload as private GitHub gist |
| `/reload` | Reload config without restart |
| `/hotkeys` | Show all keyboard shortcuts |
| `/changelog` | Display version history |
| `/quit` | Quit |
| `/skill:name` | Invoke skill (from Agent Skills standard) |
| `/templatename` | Expand prompt template |

### Message Queue (while agent is working)
| Action | Key | Description |
|--------|-----|-------------|
| Steering message | `Enter` | Queued, delivered after current tool batch completes |
| Follow-up message | `Alt+Enter` | Queued, delivered only after agent finishes all work |
| Abort & restore | `Escape` | Cancels current operation, restores queued messages to editor |
| Retrieve messages | `Alt+Up` | Restores queued messages back to editor |

### Session Management
| Feature | Description |
|---------|-------------|
| Auto-save | Sessions saved to `~/.pi/agent/sessions/` by working directory |
| Continue | `pi -c` resumes most recent session |
| Resume | `pi -r` interactive picker for past sessions |
| Specific session | `pi --session <path|id>` |
| Ephemeral | `pi --no-session` no persistence |
| Fork | `pi --fork <path|id>` fork session into new file |
| Tree navigation | `/tree` navigate session tree in-place |
| Branching | `/fork`, `/clone` for branching sessions |
| Auto-naming | Sessions get auto-generated display names |

### Key Bindings (critical shortcuts)
| Key | Action |
|-----|--------|
| `Enter` | Submit input |
| `Ctrl+C` | Clear editor (×2 = quit) |
| `Escape` | Cancel/abort (×2 = open `/tree`) |
| `Ctrl+L` | Open model selector |
| `Ctrl+P` / `Shift+Ctrl+P` | Cycle scoped models forward/backward |
| `Shift+Tab` | Cycle thinking level |
| `Ctrl+O` | Collapse/expand tool output |
| `Ctrl+T` | Collapse/expand thinking blocks |
| `Ctrl+Z` | Suspend to background |
| `Ctrl+D` | Exit (when editor empty) |
| `Alt+Enter` | Queue follow-up message |

### Customization & Extensibility
| Feature | Details |
|---------|---------|
| Extensions | TypeScript modules with full API access |
| Skills | Agent Skills standard, progressive disclosure |
| Themes | Custom themes, hot-reload |
| Prompt templates | Markdown files with `{{variables}}` |
| Keybindings | Fully customizable via `keybindings.json` |
| Pi Packages | npm/git bundle sharing |
| Custom editor | Replace editor entirely (vim mode, etc.) |
| Widgets | Above/below editor placement |
| Status line | Persistent footer status |
| Custom footer | Replace default footer |
| Overlays | Dialogs, selectors on top of content |

### CLI Flags
| Flag | Description |
|------|-------------|
| `-p`, `--print` | Print response and exit |
| `--mode json` | JSONL events to stdout |
| `--mode rpc` | RPC mode (stdin/stdout JSONL) |
| `--model <pattern>` | Model pattern/ID |
| `--provider <name>` | Provider name |
| `--api-key <key>` | API key override |
| `--thinking <level>` | Thinking level |
| `-c`, `--continue` | Continue most recent |
| `-r`, `--resume` | Resume picker |
| `--session <path\|id>` | Specific session |
| `--fork <path\|id>` | Fork session |
| `--no-session` | Ephemeral |
| `--tools <list>` | Tool allowlist |
| `--no-builtin-tools` | Disable built-in tools |
| `--no-tools` | Disable all tools |
| `--no-extensions` | Disable extensions |
| `--no-skills` | Disable skills |
| `--no-context-files`, `-nc` | Disable context files |
| `@files` | Include file contents in message |

---

## 2. Opencode Feature Comparison

| Feature | Opencode | PI | Tau (current) |
|---------|----------|----|-----------------|
| TUI | Full TUI (custom) | Full TUI (custom components) | CLI only (planned) |
| Print mode | `--prompt` flag | `-p` | Planned |
| JSON mode | Not explicit | `--mode json` | Planned |
| Server mode | `serve` (headless) | N/A | Not planned |
| Web UI | `web` command | N/A | Not planned |
| MCP support | Built-in | Extensions | Not planned |
| Plugins | Plugin system | Extensions + packages | Not planned |
| Session management | Yes | Yes | Planned |
| Multi-agent | Agent system | Extensions | Deferred |
| ACP protocol | Built-in | N/A | Not planned |

---

## 3. Current Task 014 Specification

### What We Have
- Interactive mode with readline
- Print mode (`-p`)
- JSON mode (`--mode json`)
- Session flags: `-c`, `-r`, `--session`, `--no-session`
- Model selection (SDK delegates, CLI handles ambiguity)
- Skill invocation (`/skill:name`)
- Auth override (`--api-key`)
- Tool control (`--tools`, `--read-only`)
- Context files override (`--no-context-files`)
- Graceful error handling and exit codes

### What's Missing (Gap Analysis)

#### Critical Gaps
| Gap | Priority | Details |
|-----|----------|---------|
| **Message history display** | HIGH | No specification for how to display conversation history in interactive mode. PI shows scrolling message area with user/assistant/tool messages. |
| **Streaming output rendering** | HIGH | No specification for how streaming text/thinking/tool events render in real-time. PI uses inline updates with working indicator. |
| **Working indicator / spinner** | HIGH | No visual feedback during LLM streaming. Need spinner or progress indicator. |
| **Footer / status bar** | HIGH | No specification for displaying model, session name, token usage, working directory. |
| **Ctrl+C double-press** | MEDIUM | Single Ctrl+C should clear editor, double should quit. Currently not specified. |
| **Session name display** | MEDIUM | No specification for showing current session name/ID during interactive use. |
| **Usage/cost display** | MEDIUM | No specification for showing token usage and cost after each turn. |
| **Tool output rendering** | MEDIUM | No specification for how tool call/results display in interactive mode. |
| **Thinking block rendering** | MEDIUM | No specification for displaying thinking/reasoning content. |
| **Slash commands system** | MEDIUM | Only `/skill:name` specified. Missing: `/model`, `/name`, `/help`, `/quit`, `/compact`, `/session`, etc. |
| **File reference (`@`)** | MEDIUM | PI uses `@` for file references. Not in our spec. |
| **Startup header** | LOW | No specification for initial display (shortcuts, context files, skills loaded). |
| **Command history** | MEDIUM | Readline provides this, but no specification for cross-session persistence. |
| **Abort behavior** | MEDIUM | Ctrl+C/Escape abort specification missing. Need graceful agent abort. |

#### Nice-to-Have (Future)
| Feature | Priority | Details |
|---------|----------|---------|
| **Theme system** | LOW | Color customization for terminal output |
| **Image support** | LOW | Paste/drag images (PI feature) |
| **Bash inline (`!cmd`)** | LOW | Run commands inline, send output to LLM |
| **External editor (`Ctrl+G`)** | LOW | Open input in `$EDITOR` |
| **Compaction trigger** | MEDIUM | Manual `/compact` command |
| **Session tree** | LOW | `/tree` navigation (complex) |
| **Export/share** | LOW | `/export`, `/share` commands |
| **Message queue (steer/follow-up)** | MEDIUM | PI allows queuing messages while agent works |
| **Context files display** | LOW | Show which AGENTS.md files are loaded at startup |

---

## 4. Readline Library Decision

### Options Analyzed

| Library | Stars | Last Updated | Status | Notes |
|---------|-------|-------------|--------|-------|
| `chzyer/readline` | 2,285 | 2025-06 | Active (maintained fork) | Most popular, well-tested, clean API |
| `reeflective/readline` | 138 | 2026-01 | Active | Modern, shell-like, `.inputrc` support |
| `charmbracelet/bubbletea` | 42,060 | 2026-05 | Very active | Full TUI framework, not just readline |
| `charmbracelet/huh` | N/A | Active | Active | Interactive prompts, built on bubbletea |
| `peterh/liner` | N/A | Archived | Dead | Old, archived |

### Decision: `github.com/chzyer/readline`

**Rationale:**
- Most popular and battle-tested readline library for Go
- Recently maintained (2025-06 update)
- Clean, simple API — perfect for our needs (we don't need a full TUI framework)
- Supports: history, tab completion, password mode, multiline, vim/emacs modes
- Used by many production tools (CockroachDB, Kubernetes kubectl plugins, etc.)
- We're building a CLI with an input prompt, not a full-screen TUI application
- Bubbletea would be overkill for our use case — it's designed for full-screen applications, not chat-style interfaces

**What chzyer/readline provides:**
- Line editing with Emacs/vi keybindings
- Command history (in-memory, can persist to file)
- Tab completion API
- Auto-completion suggestions
- Password/silent input mode
- Multiline support (with configurable prompt)
- Ctrl+C handling
- POSIX-compatible terminal handling

**What we need to build on top:**
- Message area rendering (scroll-back)
- Streaming text display with cursor management
- Tool call visualization
- Thinking block display
- Status bar/footer rendering
- Slash command processing
- Working indicator/spinner

---

## 5. Revised TUI Architecture

### Screen Layout (Interactive Mode)
```
┌─────────────────────────────────────────────────────────┐
│ [tau v0.1.0] /home/user/project                      │ ← Header (shown once at startup)
│ Model: gemma4:26b | Session: morning-garden (abc123)    │
│ Tools: 7 enabled | Read-only: no                        │
├─────────────────────────────────────────────────────────┤
│                                                         │
│ User: Hello, can you help me with...                    │ ← Message Area
│                                                         │   (scrollable)
│ Assistant: Sure! Let me start by reading the file.      │
│                                                         │
│ ┌─ read: main.go ────────────────────────────────────┐  │
│ │ package main                                        │  │
│ │ ...                                                  │  │
│ └──────────────────────────────────────────────────────┘  │
│                                                         │
├─────────────────────────────────────────────────────────┤
│ ⏳ Working...  │ /home/user/project │ gemma4:26b │ 12.4k tokens │ $0.03 │
├─────────────────────────────────────────────────────────┤
│ > Can you explain the main function?                    │ ← Input Line
│   ↑↓ history  | / for commands  | @ for files  | Ctrl+C quit │
└─────────────────────────────────────────────────────────┘
```

### Event-to-Display Mapping

| Agent Event | Display Action |
|-------------|---------------|
| `AgentEventStart` | Show "Starting..." indicator |
| `AgentEventMessageStart` | Show "Assistant:" label, start streaming |
| `AgentEventTextDelta` | Append text to current assistant message |
| `AgentEventThinkingDelta` | Append thinking text (dimmed/italic style) |
| `AgentEventToolExecStart` | Show tool call header, collapse output |
| `AgentEventToolExecEnd` | Mark tool call as complete |
| `AgentEventMessageEnd` | Finalize assistant message, show separator |
| `AgentEventTurnEnd` | Show usage summary for this turn |
| `AgentEventAgentEnd` | Show completion indicator, return to input prompt |
| `AgentEventError` | Show error message in red |

### Slash Commands (v1)
| Command | Description |
|---------|-------------|
| `/help` | Show available commands and shortcuts |
| `/model` | Show current model, trigger interactive selection if ambiguous |
| `/name <name>` | Rename current session |
| `/compact` | Trigger context compaction |
| `/session` | Show current session info |
| `/quit` / `/exit` | Exit tau |
| `/skill:name` | Load skill into conversation |
| `/skills` | List available skills |
| `/clear` | Clear screen (message history) |
| `/usage` | Show cumulative token usage |

### Keyboard Shortcuts (v1)
| Key | Action |
|-----|--------|
| `Enter` | Submit input |
| `Ctrl+C` | Abort current operation (×2 to quit) |
| `Ctrl+D` | Exit (when input empty) |
| `Ctrl+L` | Clear screen |
| `↑` / `↓` | Command history navigation |
| `Tab` | Auto-complete (paths after `@`, commands after `/`) |
| `Shift+Enter` | New line in input |
| `/` at start | Enter command mode (show completions) |
| `@` | Trigger file reference search |

---

## 6. Subtasks Revised

Based on this analysis, the subtasks should be:

- **014.1** — `cmd/tau/main.go` — CLI entry point, flag parsing, mode routing
- **014.2** — `cmd/tau/interactive.go` — Interactive chat: readline setup, input loop, signal handling
- **014.3** — `cmd/tau/display.go` — Message area rendering: streaming text, tool calls, thinking blocks, footer
- **014.4** — `cmd/tau/commands.go` — Slash command processing: `/help`, `/model`, `/name`, `/quit`, `/compact`, `/session`, `/skills`, `/skill:name`
- **014.5** — `cmd/tau/print.go` — Print mode (`-p`): single prompt → output → exit, stdin piping
- **014.6** — `cmd/tau/json.go` — JSON output mode (`--mode json`): JSONL events to stdout
- **014.7** — `cmd/tau/sessions.go` — Session management: `-c`, `-r`, `--session`, `--no-session`, resume picker
- **014.8** — `cmd/tau/completion.go` — Tab completion: commands, file references, skills, model names
- **014.9** — Unit tests for flag parsing, mode routing, command processing
- **014.10** — End-to-end test: `exec.Command` runs CLI binary with mock provider, verifies output

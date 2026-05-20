# Task 022: Session Management UI & Commands

## Why

Tasks 020-021 deliver a functional chat with styled messages. This task adds the interactive management layer: session resume picker, model selector, slash commands, and skill invocation — making the CLI a complete productivity tool.

## Comparison: Our Approach vs PI

| Command | PI Implementation | Our Approach (Task 022) |
|---------|------------------|------------------------|
| `/model` | Interactive model selector | `bubbles/list` for model selection |
| `/resume` | Session picker with search | `bubbles/list` for session selection |
| `/name <name>` | Rename session | SDK `Session.Rename()` |
| `/session` | Show session info | Display session metadata |
| `/skill:name` | Load skill content | SDK skill discovery + content injection |
| `/skills` | List available skills | Display discovered skills |
| `/compact` | Trigger compaction | SDK `Session.Compact()` |
| `/clear` | Clear screen | Reset viewport content |
| `/help` | Show commands | Display help text |
| `/quit` | Exit | `tea.Quit` |

## Main Constraints

- Model selector must use SDK's `ListModels()` for available models
- Session resume picker must use config's session discovery
- Skill invocation must use SDK's discovered skills list
- All commands must be parseable from textarea input
- List components must support filtering/search
- Overlay/selector mode must block normal input while active

## Dependencies

- Task 020 (CLI Foundation) — completed first
- Task 021 (Message Rendering) — completed first
- `internal/sdk/` — `ListModels()`, `SetModel()`, `Rename()`, `Compact()`, `Skills()`
- `internal/config/` — session file discovery
- `internal/skills/` — skill discovery, content loading
- `bubbles/list` — interactive list component for selectors

## Subtasks

- [ ] **022.1** — Session resume picker (`-r` flag): interactive list of past sessions. Reads `.jsonl` files from session directories directly (SDK has no `ListSessions()` — parse file headers for name, date, message count).
- [ ] **022.2** — `/name <name>` — rename current session via SDK `Session.Rename()`.
- [ ] **022.3** — `/session` — display session info (ID, name, message count, token usage, cost, cwd). File path omitted — SDK doesn't expose it.
- [ ] **022.4** — Model selector (`/model`): interactive list via `bubbles/list` using SDK `Session.ListModels()`.
- [ ] **022.5** — `/compact` — trigger context compaction via SDK `Session.Compact()`.
- [ ] **022.6** — `/clear` — reset viewport content.
- [ ] **022.7** — Skill commands: `/skill:name` prepends skill's full SKILL.md content as a user message. Shows warning if content exceeds 10k chars.
- [ ] **022.8** — `/skills` — list available discovered skills using SDK `Session.Skills()`.
- [ ] **022.9** — Command help (`/help`) — updated with all commands.
- [ ] **022.10** — Selector overlay mode: blocking normal textarea input while selector is active.
- [ ] **022.11** — Unit tests for command parsing and execution.

## Acceptance Criteria

- [ ] `./tau -r` opens interactive session picker, user selects session to resume
- [ ] Session list shows: session name, date, message count, working directory (scanned from session files directly)
- [ ] `-r` scans sessions from the current working directory only
- [ ] `/name "new-name"` renames current session, confirmation displayed
- [ ] `/session` displays: ID, name, message count, token usage, cost, working directory (no file path — SDK doesn't expose it)
- [ ] `/model` opens interactive model list with filtering/search
- [ ] Model list shows: model name, provider, context window
- [ ] Selecting a model switches the active model via SDK `Session.SetModel()`, confirmation displayed
- [ ] `/compact` triggers compaction via SDK, result displayed (compacted or not needed)
- [ ] `/clear` clears the viewport, conversation history preserved in SDK
- [ ] `/skill:name` prepends the skill's full SKILL.md content as a user message (formatted as "[Skill: name]\n<content>")
- [ ] `/skills` lists all discovered skills with name, description, source
- [ ] Invalid skill name shows error message
- [ ] Unknown commands show "unknown command, /help for available commands"
- [ ] Selector mode blocks normal textarea input until selection or cancel (Escape)
- [ ] `go test ./cmd/tau/...` — all pass
- [ ] Manual verification: all commands work correctly against Ollama

## Proposed New Files

```
cmd/tau/
├── commands.go          # Expanded slash command processing
├── selector_model.go    # Model selector component (bubbles/list)
├── selector_session.go  # Session resume picker component
├── skills.go            # Skill command handling
└── commands_test.go     # Tests for command processing
```

## Testing Strategy

**Unit tests:**
- Command parsing: all command formats, with/without arguments
- Model selector: list creation, filtering, selection, cancellation
- Session picker: list creation, filtering, selection, cancellation
- Skill loading: valid skill, invalid skill, skill not found
- Session rename: success, ephemeral session (no-op)

**Manual verification (against Ollama):**
- Run `./tau -r`, verify session list appears, select a session
- In interactive mode: `/model`, verify model list, select a model
- Send a message, verify response with new model
- `/session`, verify session info displayed correctly
- `/name "test-session"`, verify rename confirmation
- `/skills`, verify skill list displayed
- `/skill:pi-subagents`, verify skill content loaded
- `/compact`, verify compaction result
- `/clear`, verify viewport cleared

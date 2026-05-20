# Task 034: Custom User Commands

## Why

Users want to define their own slash commands for repetitive workflows. OpenCode supports this via `.opencode/commands/*.md` files with frontmatter. PI supports it via extensions. Tau should support custom commands so users can create shortcuts like `/test`, `/review`, `/deploy` that expand to pre-written prompts with optional model/agent overrides.

## Comparison with Existing Tools

### OpenCode
- **Location**: `.opencode/commands/*.md` (project) or `~/.config/opencode/commands/` (global)
- **Format**: Markdown with YAML frontmatter
  ```markdown
  ---
  description: Run tests with coverage
  agent: build
  model: anthropic/claude-sonnet-4-20250514
  ---
  Run the full test suite with coverage report.
  Focus on the failing tests and suggest fixes.
  ```
- **Arguments**: `$ARGUMENTS`, `$1`, `$2`, `$3` positional parameters
- **Shell injection**: `!command` runs shell and injects output
- **File references**: `@file` includes file content
- **Override**: Custom commands can override built-in commands
- **Config**: Also supports `opencode.json` with `"command": { "name": { "template": "..." } }`

### PI
- **Location**: Extensions register commands via `pi.registerCommand("name", { ... })`
- **Format**: TypeScript, programmatic registration
- **More flexible**: Can add custom UI, not just prompt templates

### Claude Code
- No built-in custom command system (relies on CLAUDE.md for instructions)

## Proposed Design

### Command File Format

Follow OpenCode's approach — markdown files with YAML frontmatter:

```
~/.config/tau/commands/test.md
.tau/commands/test.md
```

```markdown
---
description: Run tests and analyze failures
model: openai/gpt-4o
---
Run `go test ./... -v` and analyze any failures.
Suggest fixes for each failing test.
```

### Discovery Locations (priority order)
1. `.tau/commands/` — project-level
2. `~/.config/tau/commands/` — global user-level

### Command Struct Extension

Extend the Command struct from Task 032:

```go
type CustomCommand struct {
    Name        string
    Description string
    Template    string      // prompt template
    Model       string      // optional model override
    Agent       string      // optional agent override
    Source      string      // file path for reload/debug
}
```

### Template Processing

Template placeholders:
- `$ARGUMENTS` — all arguments after command name
- `$1`, `$2`, `$3` — positional arguments
- No shell injection or file references in v1 (future tasks)

### Integration with Dropdown

- Custom commands appear in the dropdown alongside built-in commands
- Visual indicator to distinguish custom from built-in (e.g., `[custom]` tag)
- Sorted: built-in commands first, then custom commands (alphabetically within each group)

### Reload

- Commands discovered on startup
- `/reload` command (from Task 022) re-discovers custom commands

## Constraints

- Custom commands cannot override built-in commands (safety)
- Templates are plain text — no shell execution, no file reading in v1
- Must work with existing dropdown from Task 032
- Must work with fuzzy matching from Task 033

## Subtasks

### 034.1: Custom Command Discovery
- Implement file discovery in `.tau/commands/` and `~/.config/tau/commands/`
- Parse markdown frontmatter (YAML) for name, description, template, model, agent
- Handle malformed files gracefully (log warning, skip file)
- Unit tests for discovery and parsing

### 034.2: Template Processing
- Implement `$ARGUMENTS`, `$1`, `$2`, `$3` substitution
- Handle missing arguments (leave placeholder or empty string)
- Unit tests for template substitution

### 034.3: Dropdown Integration
- Merge custom commands into dropdown alongside built-in
- Add `[custom]` indicator in description
- Custom commands execute by submitting template as prompt (with argument substitution)
- E2E verification: create custom command, see in dropdown, execute with arguments

### 034.4: Documentation
- Update ARCHITECTURE.md with custom commands section
- Update help text to mention custom commands
- Example custom commands in docs

## Acceptance Criteria

1. `.tau/commands/test.md` is discovered and appears in dropdown
2. `~/.config/tau/commands/` commands are discovered globally
3. Custom commands show `[custom]` tag in dropdown
4. `/test arg1 arg2` substitutes `$ARGUMENTS` with "arg1 arg2"
5. `$1`, `$2`, `$3` positional substitution works
6. Malformed command files are skipped with warning
7. Custom commands cannot override built-in commands
8. `/reload` re-discovers custom commands
9. All existing tests pass
10. go vet / go build / go test -race clean

## Out of Scope

- Shell injection (`!command`) — future task
- File references (`@file`) — future task
- Custom commands overriding built-in — not supported
- JSON config file support — markdown files only for now
- Extension system (PI-style TypeScript commands) — future task

## Files to Modify

- New: `internal/tui/customcmd/discovery.go`
- New: `internal/tui/customcmd/discovery_test.go`
- New: `internal/tui/customcmd/template.go`
- New: `internal/tui/customcmd/template_test.go`
- Modify: `internal/tui/command.go` — add custom command merging
- Modify: `internal/tui/dropdown.go` — show custom indicator

## Reference

- OpenCode docs: `https://opencode.ai/docs/commands`
- OpenCode source: `packages/console/app/src/` command handling

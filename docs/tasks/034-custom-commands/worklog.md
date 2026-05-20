# Worklog - Task 034: Custom User Commands

## Research
- OpenCode: `.opencode/commands/*.md` with YAML frontmatter, `$ARGUMENTS`, `$1`/`$2`/`$3`, `!command` shell injection, `@file` references
- PI: Extension-based command registration via TypeScript API
- Claude Code: No custom command system (relies on CLAUDE.md)

## Design Decisions
- Follow OpenCode's markdown + frontmatter approach (simpler than PI's extension system)
- Discovery in `.tau/commands/` (project) and `~/.tau/commands/` (global) — consistent with project's `.tau` convention
- v1: plain text templates only — no shell injection, no file references
- Custom commands cannot override built-in (safety)
- `[custom]` tag in dropdown for visual distinction
- Priority order: project > global > embedded (higher priority overrides same-name commands)
- Command name from frontmatter `name` field, fallback to filename (without `.md`)
- Embedded commands passed as parameter to `DiscoverCommands(cwd, embedded)` for future built-in custom commands

## Implementation
- Created `internal/tui/customcmd/` package:
  - `discovery.go`: `CustomCommand` struct, `DiscoverCommands()`, frontmatter parsing
  - `discovery_test.go`: 14 tests covering parsing, priority merging, malformed files
  - `template.go`: `ProcessTemplate()` for `$ARGUMENTS`, `$1`/`$2`/`$3` substitution
  - `template_test.go`: 9 tests covering all placeholder scenarios
- Modified `internal/tui/command.go`:
  - Added `customCommands`, `embeddedCommands`, `cwd`, `builtinCount` fields to `CommandRegistry`
  - Added `LoadCustomCommands(cwd, embedded)` method
  - Added `/reload` command
  - Updated help text to mention custom commands
- Modified `internal/tui/model.go`:
  - `NewModel` calls `LoadCustomCommands(cwd, nil)` after creating registry
- Added 7 integration tests in `command_test.go`:
  - Empty load, embedded commands, builtin override prevention, template execution, reload, availability

## Test Results
- 22 customcmd package tests: all pass
- 7 integration tests: all pass
- All existing TUI tests pass (updated counts for new `/reload` command)
- go vet/build/mod tidy clean
- Race detector clean (customcmd package verified)

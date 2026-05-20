# Worklog - Task 032: Command Dropdown Discovery

## Implementation

### 032.1: Command Registry
- Created `Command` struct with Name, Description, Handler, Available fields
- Created `CommandRegistry` with Lookup, Filter, AvailableCommands methods
- Created `ParseCommandInput` to handle both space-separated and colon-prefixed commands (e.g., `/skill:name`)
- Migrated all 10 commands from switch statements to registry handlers
- Extracted handler functions: cmdQuit, cmdHelp, cmdName, cmdSession, cmdModel, cmdCompact, cmdClear, cmdSkills, cmdSkill
- Moved `completeSkill`, `longestCommonPrefix`, `defaultCommands` to command.go
- Deleted `completion.go`

### 032.2: Dropdown Component
- Created `CommandDropdown` sub-model with Open, Close, Up, Down, Selected, SelectedText, Height, View methods
- Max 5 visible items with scroll support
- Prefix filtering on command name (case-insensitive)
- Wrap-around navigation (up/down)
- Lipgloss rendering with cursor prefix, selected highlighting, dimmed descriptions
- Full terminal width, normal borders matching input separator style

### 032.3: Integration
- Added `commandDropdown` and `commandRegistry` fields to Model
- Added `updateCommandDropdown()` called after every input update
- Added dropdown key handling in `handleKeyPress`: up/down navigate, enter executes, esc closes, tab navigates
- Replaced `processSlashCommand`/`processSlashCommandStreaming` with `executeCommand`/`executeCommandStreaming`
- Updated `resize()` to subtract dropdown height from viewport space
- Added `renderCommandDropdown()` in view.go between viewport and input
- Tab completion fallback preserved for `/skill:` after dropdown fills input

### 032.4: Polish & Testing
- Added dropdown styles: commandDropdownStyle, commandDropdownCursorStyle, commandDropdownSelectedNameStyle, commandDropdownNameStyle, commandDropdownDescStyle
- Created `command_test.go` with tests for registry, dropdown, ParseCommandInput, executeCommand
- Updated `completion_test.go` → `command_test.go` (deleted old, created new)
- Updated `tui_test.go` to use `executeCommand` instead of `processSlashCommand`
- Added `commandRegistry` initialization to `newTestModel()`

## Files Changed
- **Created**: `internal/tui/command.go`, `internal/tui/command_test.go`
- **Modified**: `internal/tui/model.go`, `internal/tui/update.go`, `internal/tui/view.go`, `internal/tui/styles.go`, `internal/tui/tui_test.go`
- **Deleted**: `internal/tui/completion.go`, `internal/tui/completion_test.go`

## Test Results
- All 237 tests pass
- go vet clean
- go build clean
- Binary rebuilt at `./tau`

## Fixes Applied Post-Implementation
1. **Dropdown shows on `/` alone** — changed tracking key from `query` to `"/" + query` so empty query after `/` triggers dropdown with all commands
2. **Max visible items changed to 7** — `commandDropdownMaxVisible = 7`

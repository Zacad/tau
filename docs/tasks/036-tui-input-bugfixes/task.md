# Task 036: TUI Input Bug Fixes

## Why

Three bugs were discovered in the TUI input/prompt handling:
1. After executing commands (e.g., `/session`), the prompt input moved to a new line instead of clearing
2. After closing the model selector (`/model`) with Esc, the input became inoperable (blurred, couldn't type)
3. Ctrl+D to exit only worked when the input was empty

## Constraints

- Must not change existing command behavior
- Must work within existing bubbletea v2 architecture
- Must maintain test coverage

## Subtasks

### 036.1: Command Execution Input Clear
- Fix: When a command handler returns `nil`, return a no-op `tea.Cmd` to prevent the textarea from also processing Enter
- Affected: `update.go` — `handleKeyPress()` and `executeDropdownSelection()`

### 036.2: Model Selector Input Re-focus
- Fix: Call `m.input.Focus()` when closing the model selector (Esc/q and Enter selection)
- Affected: `selector.go` — `handleSelectorInput()`

### 036.3: Ctrl+D Exit Behavior
- Fix: Remove `m.input.Value() == ""` check so Ctrl+D quits in idle state regardless of input content
- Affected: `update.go` — `handleKeyPress()`

## Acceptance Criteria

1. Commands clear input on Enter without inserting newline
2. Model selector Esc/Enter returns focus to input
3. Ctrl+D exits from idle state regardless of input content
4. All existing tests pass
5. go vet / go build clean

## Files Modified

- `internal/tui/update.go` — command execution no-op Cmd, Ctrl+D behavior
- `internal/tui/selector.go` — input re-focus on selector close
- `internal/tui/tui_test.go` — updated test for Ctrl+D behavior

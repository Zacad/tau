# Task 032: Command Dropdown Discovery

## Why

Currently tau has tab completion for slash commands (`internal/tui/completion.go`), but when a user types `/` they have no visual feedback about what commands are available. They must either know the command name already or press Tab repeatedly to cycle through longest-common-prefix completions. This is a significant UX gap compared to every other terminal coding agent.

Users should see available commands immediately when they type `/`, with filtering as they type, and be able to navigate and select with arrow keys + Enter.

## Comparison with Existing Tools

### PI (pi-coding-agent)
- **Pattern**: Inline autocomplete dropdown below the input line
- **Trigger**: Appears automatically when typing `/`
- **Implementation**: `CombinedAutocompleteProvider` in `@earendil-works/pi-tui` combines slash commands, prompt templates, extension commands, and skills into one provider
- **Features**:
  - Fuzzy filtering via `fuzzyFilter()` function
  - Shows command name + description
  - Argument completions for `/model` (shows available models)
  - Source tags `[user]`, `[project]`, `[npm:...]` for discovery context
  - `SlashCommand` interface with `name`, `description`, `getArgumentCompletions`
  - Configurable `autocompleteMaxVisible` setting
- **Key insight**: Commands are structured objects, not strings. Each command has metadata (name, description, argument completion function).

### OpenCode
- **Pattern**: Command palette (Ctrl+P) + tab completion
- **Trigger**: Ctrl+P opens full-screen overlay, `/` starts command typing
- **Implementation**: Custom commands defined in `opencode.json` or `.opencode/commands/*.md` with frontmatter
- **Features**:
  - Custom commands with `template`, `description`, `agent`, `model`, `subtask`
  - Arguments via `$ARGUMENTS`, `$1`, `$2` placeholders
  - Shell output injection via `!command` syntax
  - File references via `@file` syntax
  - Built-in commands: `/connect`, `/compact`, `/details`, `/editor`, `/exit`, `/export`, `/help`, `/init`, `/models`, `/new`, `/redo`, `/sessions`, `/share`, `/themes`, `/thinking`, `/undo`, `/unshare`

### Claude Code
- **Pattern**: Inline dropdown below input
- **Trigger**: Appears on `/`
- **Features**: Command name + description, arrow key navigation, Enter to select

## Current Tau State

### How commands work now (`internal/tui/`)
- **Command list**: Hardcoded string slice in `completion.go:5-17`:
  ```go
  var slashCommands = []string{"/quit", "/exit", "/help", "/name", "/session", "/model", "/compact", "/clear", "/skills", "/skill:"}
  ```
- **Execution**: Large switch statement in `model.go:599-773` (`processSlashCommand`) and `model.go:778-847` (`processSlashCommandStreaming`)
- **Completion**: Tab-only in `update.go:198-218`, uses prefix matching + longest common prefix
- **No structured command registry**: No `Command` struct, no descriptions, no metadata
- **No visual feedback**: User sees nothing when typing `/`
- **Duplication**: Help text and command handling duplicated between idle and streaming modes

### Existing UI components
- **Model selector**: `selector.go` - uses bubbles `list` component, overlay pattern
- **Input**: `textarea.Model` from bubbles
- **Framework**: bubbletea v2 + bubbles v2 + lipgloss v2

## Proposed Design

### Command Registry

Replace the hardcoded string slice with a structured command registry:

```go
type Command struct {
    Name        string            // e.g., "quit" (without /)
    Description string            // e.g., "Exit tau"
    Handler     CommandHandler    // function to execute
    Available   func(model *Model) bool // optional: hide command in certain states
}

type CommandHandler func(model *Model, args string) (handled bool, cmd tea.Cmd)
```

Benefits:
- Single source of truth for commands
- Descriptions enable dropdown display
- `Available` func controls context-sensitive visibility (e.g., hide `/quit` during streaming)
- Enables testing individual command handlers
- Extensible for future features (aliases, flags, argument parsing)

### Inline Dropdown Component

A new bubbletea sub-component that renders below the input line:

**Trigger**: When input starts with `/`
**Dismiss**: Esc, Enter (after selection), or when `/` prefix is removed
**Navigation**: Arrow keys (up/down), Enter to select, Tab to accept top match
**Filtering**: Fuzzy match on command name (and description in future)

**Layout**:
```
+------------------------------------------+
| /hel                                     |  <- textarea input
| ┌──────────────────────────────────────┐ |
| │ > /help    Show help text            | |  <- selected (cursor)
| │   /hello   ...                       | |
| └──────────────────────────────────────┘ |
+------------------------------------------+
```

**Implementation approach**:
- Custom bubbletea sub-model (`CommandDropdown`)
- Renders as an overlay above the footer, below the textarea
- Uses lipgloss for styling (matching existing tau styles)
- Max 5-7 visible items (scrollable if more commands exist)
- Filters commands from registry based on text after `/`

### Why not use existing libraries

- **bubbles/list**: Designed for standalone full-screen lists, not inline dropdowns. Too heavy for this use case.
- **huh**: Form library, takes over the full screen. Not suitable for inline dropdown.
- **Custom implementation**: The dropdown is simple enough (filter + render + navigate) that a custom component is cleaner and more maintainable than adapting a general-purpose list.

### Integration Points

1. **`completion.go`**: Replace with `command.go` containing registry + dropdown model
2. **`model.go`**: Add `CommandDropdown` as a sub-model, replace switch statement with registry lookup
3. **`update.go`**: Replace tab completion logic with dropdown open/close + navigation
4. **`view.go`**: Render dropdown when active (adjusts viewport height to avoid overlap)

### State Flow

```
User types "/" 
  → dropdown opens with all commands
User types "h" (input: "/h")
  → dropdown filters to /help
User presses ↓
  → moves selection (if multiple matches)
User presses Enter
  → executes selected command, closes dropdown, clears input
User presses Esc
  → closes dropdown, keeps input text
User backspaces "/" away
  → dropdown closes automatically
```

## Constraints

- Must work within existing bubbletea v2 architecture
- Must not break existing command behavior
- Must work during both idle and streaming states (with appropriate command filtering)
- Must maintain backward compatibility with tab completion (Tab should also work for dropdown selection)
- Terminal width changes must be handled gracefully

## Subtasks

### 032.1: Command Registry
- Define `Command` struct with Name, Description, Handler, Available
- Create registry that holds all commands
- Migrate existing commands from switch statement to registry
- Keep behavior identical (all existing commands must work)
- Unit tests for registry (lookup, filtering, availability)

### 032.2: Dropdown Component
- Create `CommandDropdown` bubbletea sub-model
- Implement filtering (prefix match initially, fuzzy later)
- Implement navigation (up/down arrows, Enter select, Esc cancel)
- Implement rendering with lipgloss (selected highlight, description)
- Unit tests for dropdown (filter, navigate, render)

### 032.3: Integration
- Wire dropdown into main TUI model
- Trigger on `/` prefix detection
- Handle dropdown state in Update loop
- Render dropdown in View (adjust layout)
- Replace tab completion trigger with dropdown
- E2E verification: all commands work via dropdown

### 032.4: Polish & Testing
- Style dropdown to match tau theme
- Handle terminal resize
- Handle edge cases (no matches, single match, empty input after `/`)
- Update help text to mention dropdown
- All existing tests must pass
- New tests for dropdown interaction

## Acceptance Criteria

1. Typing `/` immediately shows a dropdown list of all available commands with descriptions
2. Typing after `/` filters the dropdown in real-time
3. Arrow keys navigate the dropdown list
4. Enter executes the selected command
5. Esc closes the dropdown without executing
6. Tab accepts the top/selected match (fills input, doesn't execute)
7. All existing slash commands work exactly as before
8. Commands unavailable in current state are hidden (e.g., `/quit` during streaming)
9. Dropdown renders above the footer, doesn't overlap with viewport content
10. Works with terminal resize
11. All existing tests pass
12. New unit tests for registry and dropdown component
13. go vet / go build / go test -race all clean

## Out of Scope (Future Tasks)

- Multi-step interactive commands (e.g., `/add-provider` wizard) - separate task
- Fuzzy matching (beyond prefix) - can be added later
- Command arguments with inline completion (like `/model` in pi) - separate task
- Command palette (Ctrl+P) for browsing all commands - separate task
- Custom user-defined commands - separate task
- Command history / frecency sorting - separate task

## Files to Modify

- `internal/tui/completion.go` → replace with `internal/tui/command.go`
- `internal/tui/completion_test.go` → replace with `internal/tui/command_test.go`
- `internal/tui/model.go` → add dropdown sub-model, replace switch with registry
- `internal/tui/update.go` → replace tab completion with dropdown handling
- `internal/tui/view.go` → render dropdown when active
- `internal/tui/styles.go` → add dropdown styles

## Reference Files

- `internal/tui/selector.go` - existing overlay pattern to follow
- `internal/tui/completion.go` - current completion logic to replace
- `internal/tui/model.go:599-847` - current command handlers to migrate

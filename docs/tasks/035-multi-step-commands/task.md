# Task 035: Multi-Step Interactive Commands

## Why

Some commands need additional input beyond a simple prompt. For example, adding a new provider requires: (1) choosing the provider, (2) entering API key, (3) testing the connection. Currently all tau commands are single-shot — type `/command` and it executes. Multi-step commands enable guided workflows similar to `/connect` in OpenCode or `/login` in PI.

## Comparison with Existing Tools

### OpenCode — `/connect`
- **Step 1**: Shows list of providers (OpenAI, Anthropic, Google, etc.)
- **Step 2**: User selects a provider
- **Step 3**: Prompts for API key in a text input field
- **Step 4**: Validates the key by making a test request
- **Step 5**: Saves to config, shows success
- **Implementation**: Uses `huh?`-style interactive prompts within the TUI

### PI — `/login`, `/settings`, `/model`
- **`/login`**: Selector for provider → OAuth flow or API key input
- **`/settings`**: Multi-page settings form with Select, Input, Confirm fields
- **`/model`**: Filterable list selector
- **Implementation**: Custom TUI components that temporarily replace the editor area
- Uses `ExtensionInputComponent`, `ExtensionSelectorComponent` for dynamic UI

### charmbracelet/huh
- Form library with `Select`, `Input`, `Text`, `Confirm`, `MultiSelect` fields
- Supports multi-step forms via groups (pages)
- Can be embedded in bubbletea apps
- **Tradeoff**: Takes over full screen, not ideal for inline command flow

## Current Tau State

- All commands are single-shot: type `/command [args]` → execute
- Model selector (`selector.go`) is the only multi-step UI pattern
- Uses bubbles `list` component as an overlay
- No general-purpose multi-step command framework

## Proposed Design

### Command Step Interface

```go
type CommandStep interface {
    // Render returns the UI for this step
    Render(width, height int) string
    // Update handles input and returns next step or nil if done
    Update(msg tea.Msg) (CommandStep, tea.Cmd)
    // Init returns initial command for this step
    Init() tea.Cmd
    // Result returns the collected data when step is complete
    Result() map[string]string
}
```

### Multi-Step Command Registration

```go
type MultiStepCommand struct {
    Name        string
    Description string
    // Steps returns the first step in the flow
    FirstStep func(model *Model) CommandStep
    Available func(model *Model) bool
}
```

### Execution Flow

```
User types /add-provider and presses Enter
  → dropdown closes
  → multi-step mode activates
  → Step 1: "Select provider" (list overlay)
  → User selects "OpenAI"
  → Step 2: "Enter API key" (text input overlay)
  → User enters key and presses Enter
  → Step 3: "Testing connection..." (spinner)
  → Success: saves config, shows confirmation
  → Multi-step mode deactivates, returns to normal input
```

### UI Approach

Use the existing overlay pattern from `selector.go`:
- Temporarily replace the editor area with the step UI
- Show step indicator (e.g., "Step 2/3: Enter API key")
- Support Esc to cancel and return to normal input
- Each step is a self-contained bubbletea sub-model

### Why not huh?

- `huh` takes over the full screen — breaks the tau TUI layout
- tau already has overlay infrastructure (`selector.go`)
- Custom steps give full control over rendering and integration with tau's model state
- Can be embedded inline (above footer, below viewport) without disrupting layout

## Constraints

- Must not break existing single-shot commands
- Must work within existing bubbletea v2 architecture
- Must support Esc to cancel at any step
- Must handle terminal resize during multi-step flow
- Steps must be testable in isolation

## Subtasks

### 035.1: Multi-Step Framework
- Define `CommandStep` interface
- Create `MultiStepRunner` bubbletea sub-model
- Implement step navigation (next, back, cancel)
- Implement step indicator rendering
- Unit tests for framework

### 035.2: Reusable Step Components
- `ListStep` — selector from a list of options (reuses selector.go pattern)
- `InputStep` — single line text input
- `ConfirmStep` — yes/no confirmation
- `MessageStep` — show info/spinner/result
- Unit tests for each component

### 035.3: Example Multi-Step Command
- Implement `/connect` as a proof-of-concept multi-step command
- Step 1: Select provider (ListStep)
- Step 2: Enter API key (InputStep)
- Step 3: Test connection (MessageStep with spinner)
- Step 4: Save and confirm (ConfirmStep)
- E2E verification

### 035.4: Integration
- Wire multi-step commands into command registry
- Dropdown distinguishes single-shot vs multi-step commands
- Handle state transitions (normal → multi-step → normal)
- All existing tests pass

## Acceptance Criteria

1. Multi-step commands appear in dropdown with indicator (e.g., `→`)
2. Selecting a multi-step command activates step-by-step flow
3. Each step renders correctly in the TUI
4. Arrow keys / typing work within each step
5. Enter advances to next step
6. Esc cancels and returns to normal input
7. Step indicator shows current step number
8. `/connect` example works end-to-end
9. Terminal resize handled gracefully during multi-step flow
10. All existing tests pass
11. go vet / go build / go test -race clean

## Out of Scope

- Dynamic steps (steps that change based on previous answers) — future task
- Form validation with error messages — future task
- Multi-step commands with file picker / complex UI — future task
- Persisting multi-step state across sessions — future task

## Files to Modify

- New: `internal/tui/multistep/runner.go`
- New: `internal/tui/multistep/runner_test.go`
- New: `internal/tui/multistep/steps.go` (ListStep, InputStep, ConfirmStep, MessageStep)
- New: `internal/tui/multistep/steps_test.go`
- Modify: `internal/tui/command.go` — add MultiStepCommand type
- Modify: `internal/tui/model.go` — add multi-step state handling
- Modify: `internal/tui/view.go` — render multi-step overlay
- Modify: `internal/tui/dropdown.go` — show multi-step indicator

## Reference

- `internal/tui/selector.go` — existing overlay pattern
- OpenCode `/connect` flow — reference UX
- PI `/login`, `/settings` — reference UX
- charmbracelet/huh — inspiration for step types (not implementation)

# Task 038: Command Palette Modal

## Why

The current command dropdown is an inline component above the input area, triggered by `/` prefix. It's limited to 7 visible items and shares layout space with the viewport. OpenCode uses a centered modal command palette (Ctrl+P) that provides a cleaner, more discoverable interface with full command list, fuzzy search, and consistent handling patterns.

We also need to separate command types at the code level to ensure correct handling: **application commands** (TUI actions like quit, model, connect) always use the palette overlay, while **chat commands** (custom commands, /skill:) execute directly from input.

## Comparison with OpenCode

| Feature | OpenCode | Tau (current) | Tau (target) |
|---------|----------|---------------|--------------|
| Trigger | Ctrl+P / Ctrl+X leader | `/` prefix in input | Ctrl+P + `/` prefix |
| UI | Centered modal overlay | Inline dropdown above input | Centered modal overlay |
| Command types | App actions + slash commands | Single Command struct | AppCommand + ChatCommand types |
| Multi-step | Inline in palette | Separate multi-step area below input | Multi-step rendered inside palette |
| Search | Fuzzy, full command list | Fuzzy, filtered by `/` prefix | Fuzzy, full command list |

## Constraints

- Must preserve backward compatibility: `/` in input still works
- Must reuse existing command registry, fuzzy matching, custom command discovery
- Must reuse existing MultiStepRunner for multi-step command sequencing
- Palette components must be reusable bubbletea sub-models (Init/Update/View)
- Application commands use palette; chat commands execute directly from input
- Each subtask must be independently testable and verifiable in the running application
- Use `/test` command for manual verification during development

## Design

### Command Type Separation

Unexported fields + constructor functions for compile-time safety:

```go
type Command struct {
    name        string
    description string
    typ         commandType
    handler     func(m *Model, args string) tea.Cmd
    available   func(m *Model) bool
    multiStep   func(m *Model) []multistep.CommandStep
}

func NewAppCommand(name, desc string, handler func(m *Model, args string) tea.Cmd) Command
func NewAppMultiStep(name, desc string, steps func(m *Model) []multistep.CommandStep) Command
func NewChatCommand(name, desc string, template string) Command
```

### Palette Components

Reusable bubbletea sub-models that multi-step commands compose:

| Component | Purpose | Used By |
|-----------|---------|---------|
| `PaletteList` | Searchable list with selection | Provider picker, model picker |
| `PaletteInput` | Labeled text input field | API key entry |
| `PaletteConfirm` | Yes/no confirmation | Save confirm, disconnect confirm |
| `PaletteTask` | Async function with spinner | Test connection, discover models |
| `PaletteMessage` | Static info display | Help text, status |

### MultiStepRunner Integration

MultiStepRunner stays as process manager (sequencing, results, transitions). Palette renders its steps via type assertion:

```go
switch step := runner.CurrentStep().(type) {
case *multistep.ListStep:    → render with PaletteList
case *multistep.InputStep:   → render with PaletteInput
case *multistep.ConfirmStep: → render with PaletteConfirm
case *multistep.TaskStep:    → render with PaletteTask
}
```

## Subtasks

### 038.1: Command Type Separation + Test Command
- Add unexported fields to `Command` struct with `commandType` discriminator
- Create constructor functions: `NewAppCommand`, `NewAppMultiStep`, `NewChatCommand`
- Classify existing commands: app commands (quit, help, name, session, model, compact, clear, skills, skill:, reload, connect, disconnect) vs chat commands (custom commands)
- Add `/test` app command — simple handler that prints "Test command executed" to viewport, for manual verification
- Update registry to use constructors
- Update `executeCommand` and `executeDropdownSelection` to handle types
- Update `customCommandToCommand` to create `ChatCommand`

**Tests:**
- Constructor validation (correct type, correct fields set, nil fields for wrong type)
- Registry returns correct types for all commands
- `/test` command executes and displays message

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- Run `tau`, type `/test`, verify "Test command executed" appears in viewport

### 038.2: Palette Modal Shell + TUI Integration
- Create minimal `CommandPalette` model: open/close, basic list rendering (hardcoded, no component yet)
- Centered overlay with dimmed background in `view.go`
- Add `paletteActive` + `palette` fields on Model
- Ctrl+P keybinding → opens palette, blurs input
- Esc → closes palette, re-focuses input
- Resize handles overlay
- `/test` command changed → opens palette instead of printing to viewport
- Palette shows all app commands as a simple list (name + description)
- Enter on selected command → closes palette, executes command

**Tests:**
- Open/close lifecycle
- Ctrl+P trigger
- Esc close
- Overlay rendering
- Resize with overlay

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- Press `Ctrl+P` → overlay appears with command list → `Esc` → closes, back to input
- Select `/test` → palette closes, command executes

### 038.3: Palette Search + Fuzzy Filtering
- Search input at top of palette (bubbles/textinput)
- Fuzzy filtering on command names (reuse existing `fuzzyMatch`)
- Keyboard nav: up/down arrows, Enter select, Esc close
- ~12 visible items with scrolling
- Disabled commands shown but not selectable
- `/test` still opens palette, now with search

**Tests:**
- Search filtering (empty query = all, partial = fuzzy match)
- Navigation with filtered results
- Scrolling (selected item stays visible)
- Disabled command handling (shown but not selectable)

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- `Ctrl+P` → type "mod" → only `/model` shown → clear → all commands return
- Type "con" → see `/connect`, `/disconnect`

### 038.4: PaletteList Component
- Create `internal/tui/palette/` package
- `PaletteList` component: Init/Update/View/Done/Result
- Wraps bubbles/list with palette styling
- Searchable, selectable, returns selected value on Done
- Palette model uses PaletteList for command display
- Shared styles in `palette/styles.go`
- `/test` evolves: opens palette with PaletteList showing test options ("Print message", "Show info", "Cancel")

**Tests:**
- PaletteList independently (render, select, navigate, Done, Result)
- PaletteList search/filtering
- PaletteList selection returns correct value

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- `Ctrl+P` → works as before
- `/test` → palette shows test options → select one → action executes

### 038.5: PaletteInput Component
- `PaletteInput` component: labeled text input, Enter completes, Esc cancels
- Init/Update/View/Done/Result
- Palette supports step transitions (list → input)
- `/test` evolves: PaletteList → select "Enter text" → PaletteInput → type → Enter → viewport shows "You entered: <text>"

**Tests:**
- PaletteInput independently (render, type, Enter, Esc, Done, Result)
- Step transition: list → input → back

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- `/test` → select "Enter text" → type something → Enter → see result in viewport

### 038.6: PaletteConfirm Component
- `PaletteConfirm` component: y/n/Enter handling, Esc cancels
- Init/Update/View/Done/Result
- Palette supports 3-step flows
- `/test` evolves: PaletteList → PaletteInput → PaletteConfirm("Save this text?") → y → viewport shows "Text saved: <text>" / n → "Cancelled"

**Tests:**
- PaletteConfirm independently (y, n, Enter, Esc, Done, Result)
- 3-step flow: list → input → confirm

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- `/test` → full 3-step flow → confirm → see result. Try n → see cancel message

### 038.7: PaletteTask Component
- `PaletteTask` component: runs async function, shows spinner, displays result
- Init starts task, Update handles completion message, View shows spinner/result
- `/test` evolves: PaletteList → PaletteInput → PaletteConfirm → PaletteTask (simulates "processing" with 1s delay) → result shown in viewport

**Tests:**
- PaletteTask independently (spinner, success, error, Done, Result)
- 4-step flow with async task

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- `/test` → full 4-step flow → see spinner → see success message in viewport

### 038.8: PaletteMessage Component + All Components Complete ✅
- `PaletteMessage` component: static display with optional title, Enter/Esc advances
- `/test` can optionally show a final message step
- All 5 components exist: PaletteList, PaletteInput, PaletteConfirm, PaletteTask, PaletteMessage
- Palette model supports arbitrary step sequences with any component combination

**Tests:**
- PaletteMessage independently (render, Enter/Esc advance, Done)
- Full multi-component flow with all 5 types

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- `/test` → exercise all component types

### 038.9: Multi-Step App Commands in Palette ✅
- Wire existing MultiStepRunner into palette
- Type-assert steps → matching palette components
- Route messages from palette to runner
- Step transitions within palette
- Esc during multi-step → cancel, return to command list
- `/connect` works end-to-end in palette
- `/disconnect` works end-to-end in palette

**Tests:**
- Multi-step in palette (transitions, results, Esc cancel)
- `/connect` flow in palette
- `/disconnect` flow in palette

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- `Ctrl+P` → select `/connect` → full connect flow runs inside palette
- Try Esc mid-flow → cancelled, back to command list

### 038.10: Migrate Model Selector to Palette ✅
- Replace `selectorList` with palette-based model picker
- `/model` opens palette with PaletteList of all models
- Display: model-id, provider name, context window
- Current model pre-selected/highlighted
- Search/filter across all models
- Selection → switch model, close palette, confirmation in viewport

**Tests:**
- Model list rendering with provider info
- Current model pre-selection
- Model switching via palette
- Search/filter models

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All existing tests pass
- `Ctrl+P` → `/model` → search for model → select → verify model changed in header

### 038.11: `/` Input Opens Palette + Cleanup
- `/` detection in input → opens palette with `/` pre-filled in search
- Remove `CommandDropdown` struct and all methods
- Remove `commandDropdownMaxVisible`, `lastCommandInput`, `updateCommandDropdown`, `renderCommandDropdown`, `executeDropdownSelection`
- Update help text to reference Ctrl+P
- Full test suite pass, rebuild binary

**Tests:**
- `/` opens palette with prefix
- No dead code references
- All existing tests updated

**Verification:**
- `go vet ./...` clean
- `go build ./...` clean
- All tests pass (full suite)
- Type `/` → palette opens. Type `/con` → filtered. Esc → back to input with `/` cleared
- Binary rebuilt at `./tau`

## Acceptance Criteria

### Main AC
1. Ctrl+P opens command palette modal with all commands listed
2. `/` in input opens palette with `/` pre-filled (backward compatible)
3. Palette has search input, fuzzy filtering, keyboard navigation
4. Application commands execute via palette (quit, model, connect, etc.)
5. Chat commands (custom commands, /skill:) execute directly from input, not via palette
6. Multi-step commands (connect, disconnect) run inside palette with step-by-step UI
7. Palette components are reusable (PaletteList, PaletteInput, PaletteConfirm, PaletteTask, PaletteMessage)
8. Command types enforced at compile time via constructors
9. All existing tests pass, new tests added for palette and components
10. `go vet/build` clean, binary rebuilt

# Worklog - Task 035: Multi-Step Interactive Commands

## Research
- OpenCode `/connect`: provider selection → API key input → test → save
- PI `/login`, `/settings`: multi-page forms with Select, Input, Confirm fields
- huh? library: full-screen forms, not suitable for inline tau overlay
- tau existing: `selector.go` overlay pattern for model selector

## Design Decisions
- Custom `CommandStep` interface (not huh) — integrates with tau's existing overlay pattern
- Reusable step components: ListStep, InputStep, ConfirmStep, MessageStep
- Multi-step mode temporarily replaces editor area, preserves viewport
- Esc cancels at any step and returns to normal input
- `/connect` as proof-of-concept implementation (skeleton — no actual save/test)

## Implementation

### 035.1: Multi-Step Framework
- Created `internal/tui/multistep/runner.go` with `CommandStep` interface and `MultiStepRunner`
- Runner manages step navigation, result collection (`map[string]any`), cancel/finish states
- Supports step self-replacement (a step can return a new step from `Update()`)
- 13 unit tests for runner: init, navigation, cancel, results, resize, self-replacement

### 035.2: Reusable Step Components
- Created `internal/tui/multistep/steps.go` with:
  - `ListStep` — wraps `bubbles/v2/list` for option selection
  - `InputStep` — wraps `bubbles/v2/textinput` for single-line text input
  - `ConfirmStep` — yes/no confirmation (y/n/Esc)
  - `MessageStep` — static message display with optional spinner, auto-advance option
- 24 unit tests for all step types: render, update, result extraction, edge cases

### 035.3: Example Multi-Step Command
- Created `internal/tui/connect.go` with `/connect` skeleton
- Steps: ListStep (select provider) → InputStep (enter API key) → MessageStep (would test) → ConfirmStep (would save)
- Results displayed as assistant message after completion

### 035.4: Integration
- Modified `internal/tui/command.go`:
  - Added `MultiStep` field to `Command` struct
  - Registered `/connect` command with multi-step flow
  - Dropdown renders `→` indicator for multi-step commands
- Modified `internal/tui/model.go`:
  - Added `multiStepRunner` and `multiStepActive` fields
- Modified `internal/tui/view.go`:
  - Added `renderMultiStep()` overlay rendering
  - Input blurred when multi-step active
- Modified `internal/tui/update.go`:
  - `handleKeyPress` routes to `handleMultiStepKey` when active
  - `executeDropdownSelection` detects multi-step commands and starts runner
  - Esc cancels, Enter advances, finish displays results
  - WindowSizeMsg delegates to runner
- Added `multiStepIndicatorStyle` to `internal/tui/styles.go`

## Test Results
- 37 multistep tests (13 runner + 24 steps)
- All existing 100+ TUI tests pass (updated counts from 11→12 commands)
- go vet: clean
- go build: clean
- go test -race: clean

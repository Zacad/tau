# Task 061: Palette Paste Routing

## Why

Custom slash commands such as `/web-research` prompt for arguments in a palette input modal. Terminal paste sends `tea.PasteMsg`, but the modal only received keypress routing, so pasted search topics did not appear in the modal.

## Constraints

- Preserve existing command palette key handling.
- Do not change custom command template behavior.
- Keep paste scoped to the active modal so pasted text does not leak into the main prompt.
- Maintain test coverage.

## Comparison with PI/OpenCode

PI treats paste as a dedicated input path through bracketed paste buffering before insertion. OpenCode also models paste separately from ordinary key input. Tau should route Bubble Tea paste messages to the focused input component instead of relying on keypress handling.

## Subtasks

### 061.1: Add regression coverage

- Verify an active palette input receives `tea.PasteMsg` content.
- Verify pasted modal content does not enter the main prompt input.

### 061.2: Route paste to active palette

- When the palette is active, send `tea.PasteMsg` to `m.palette.Update` before normal prompt delegation.

## Acceptance Criteria

1. Pasting text into `/web-research` argument modal inserts the pasted topic.
2. Pasted text does not leak into the main prompt while a palette modal is active.
3. TUI tests pass.
4. Project tests pass.
5. Binary rebuilt at `./tau`.

## Files Modified

- `internal/tui/update.go` - route paste messages to the active palette.
- `internal/tui/tui_test.go` - regression test for palette paste routing.

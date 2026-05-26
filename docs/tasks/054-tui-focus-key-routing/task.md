# Task 054: TUI Focus Key Routing

## Why

When focus is on the prompt input, key presses (arrow keys, page up/down, etc.) leak through to the chat viewport, causing unwanted scroll position changes. Keyboard events should only affect the focused component:
- Prompt focused: keys affect textarea only
- Chat/viewport focused: keys affect viewport scroll
- Mouse wheel over chat area: always scrolls viewport regardless of keyboard focus

## Constraints

- Must not change existing command/palette behavior
- Must work within existing bubbletea v2 architecture
- Must maintain test coverage
- Must follow TDD approach

## Comparison with PI/OpenCode

PI routes keyboard input only to the focused component via `focusedComponent.handleInput(data)`. OpenCode's ScrollView explicitly ignores scroll keys when an INPUT/TEXTAREA/SELECT element has focus.

## Subtasks

### 054.1: Write failing tests for focus-gated key routing
- Test: prompt focused + PageUp/PageDown does not scroll viewport
- Test: prompt focused + up/down edge cases do not scroll viewport
- Test: mouse wheel still scrolls viewport while prompt focused
- Test: shift+enter inserts only one newline (no double processing)

### 054.2: Implement focus-gated key routing
- When `m.input.Focused()` is true, route unhandled keyboard events only to textarea
- Route viewport keyboard events only when prompt input is not focused
- Keep mouse wheel routed to viewport regardless of focus
- Remove duplicate `m.viewport.Update(msg)` call in default path

### 054.3: Fix handled key paths returning nil
- Ensure handled keys in `handleKeyPress` return no-op command to prevent fallthrough

## Acceptance Criteria

1. Prompt-focused keyboard does not scroll or affect chat viewport
2. Mouse wheel over chat area still scrolls viewport
3. All existing tests pass
4. go vet / go build clean
5. Binary rebuilt at `./tau`

## Files Modified

- `internal/tui/update.go` — focus-gated key routing, remove duplicate viewport update
- `internal/tui/tui_test.go` — new focus routing tests

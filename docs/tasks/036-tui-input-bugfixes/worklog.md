# Worklog — Task 036: TUI Input Bug Fixes

## 036.1: Command Execution Input Clear

**Problem:** After executing commands like `/session`, the prompt input moved to a new line instead of clearing.

**Root cause:** In `update.go`, when `executeCommand()` returned `nil` as its `tea.Cmd`, `handleKeyPress()` also returned `nil`. This caused the `Update()` function to fall through and delegate the Enter key event to the textarea sub-model, which inserted a newline.

**Fix:**
- `update.go:169-180`: When a command is handled but returns `nil`, return a no-op command `func() tea.Msg { return nil }` to prevent textarea from processing Enter
- `update.go:243-252` (`executeDropdownSelection`): Same fix for dropdown selection path

## 036.2: Model Selector Input Re-focus

**Problem:** After pressing Esc to close the model selector (`/model`), the input became inoperable — the default placeholder was visible but typing didn't work.

**Root cause:** In `selector.go`, `handleSelectorInput()` set `m.selectorActive = selectorNone` on Esc/q/Enter but never called `m.input.Focus()`. The input remained blurred from `renderInput()`'s `m.input.Blur()` call.

**Fix:**
- `selector.go:103-104`: Added `m.input.Focus()` after closing selector on Esc/q
- `selector.go:106`: Added `m.input.Focus()` after closing selector on Enter selection

## 036.3: Ctrl+D Exit Behavior

**Problem:** Ctrl+D shortcut for exit only worked when the prompt input was empty.

**Root cause:** In `update.go`, the Ctrl+D handler had a check `if m.input.Value() == "" && m.state == stateIdle` — it required empty input.

**Fix:**
- `update.go:201-203`: Removed the `m.input.Value() == ""` check, now Ctrl+D quits in idle state regardless of input content
- `tui_test.go:181-195`: Updated test `TestCtrlD_NoExitWhenInputNonEmpty` → `TestCtrlD_ExitWithNonEmptyInput` to reflect new expected behavior

## Verification

- All 100+ tests pass
- `go vet` clean
- `go build` clean
- Binary rebuilt at `./tau`

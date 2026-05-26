# Worklog — Task 054: TUI Focus Key Routing

## 054.1: Write failing tests for focus-gated key routing

**Problem:** No tests exist to verify that keyboard events are properly gated by focus state.

**Fix:** Added tests for:
- Prompt focused + PageUp/PageDown does not scroll viewport
- Prompt focused + up/down with empty history does not scroll viewport
- Mouse wheel still scrolls viewport while prompt focused
- Shift+enter inserts exactly one newline

## 054.2: Implement focus-gated key routing

**Root cause:** In `update.go`, unhandled messages are routed to `m.viewport.Update(msg)` even when `m.input.Focused()` is true. Additionally, viewport is updated twice in the default delegation path.

**Fix:**
- Added focus check: when `m.input.Focused()` is true, route keyboard events only to textarea
- Kept mouse wheel routed to viewport regardless of focus
- Removed duplicate `m.viewport.Update(msg)` call at end of Update function

## 054.3: Fix handled key paths

**Fix:** Ensured handled key paths in `handleKeyPress` return no-op commands to prevent fallthrough to sub-models.

## Verification

- All TUI tests pass (including 6 new focus routing tests)
- `go vet ./...` clean
- `go build` clean
- Binary rebuilt at `./tau`

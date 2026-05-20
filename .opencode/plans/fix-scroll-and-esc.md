# Fix Scroll Jumping and ESC Key Abort

## Issues
1. Scroll jumps in chat area when typing in prompt input
2. ESC key should abort current LLM response like Ctrl+C

## Root Causes

### Issue 1: Scroll Jumping
In `internal/tui/update.go:154`, `m.resize()` is called on every keystroke. Since the textarea has `DynamicHeight = true` (model.go:137), even when the height hasn't changed, calling `SetHeight()` on the viewport can cause scroll position jumps.

### Issue 2: ESC Key
In `internal/tui/update.go:386-390`, ESC only handles the `stateIdle` case (clears input). During `stateStreaming`, ESC does nothing.

## Changes

### File: `internal/tui/model.go`
Add field to track previous input height:
```go
// Tracks previous input height to avoid unnecessary resizes.
lastInputHeight int
```

### File: `internal/tui/update.go`

**Fix 1**: Only resize when input height changes (around line 154):
```go
// Re-resize if textarea height changed (DynamicHeight)
if h := m.input.Height(); h != m.lastInputHeight {
    m.lastInputHeight = h
    m.resize(m.width, m.height)
}
```

**Fix 2**: Add ESC abort during streaming (around line 386-390):
```go
case "esc":
    // During streaming: abort the current LLM response
    if m.state == stateStreaming && m.cancelFunc != nil {
        m.cancelFunc()
        return nil
    }
    if m.state == stateIdle {
        m.input.SetValue("")
        return nil
    }
```

## Testing
- Build binary and manually verify:
  1. Type multi-line messages (Shift+Enter) - scroll should not jump
  2. During streaming response, press ESC - should abort like Ctrl+C

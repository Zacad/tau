# UI Improvements Plan

## Changes

### 1. Switch footer and prompt input order
**File:** `internal/tui/view.go`
- Change the order in `View()` from `[header, viewport, footer, input]` to `[header, viewport, input, footer]`

### 2. Add margin and darker background to prompt input
**File:** `internal/tui/view.go` - `renderInput()`
- Add top padding (1 line) and bottom padding (1 line) to input area
- Add darker background color to input area using lipgloss `Background()`
- Keep the separator line but style it to blend with the new background

**File:** `internal/tui/styles.go`
- Add `inputAreaStyle` with:
  - `Background(lipgloss.Color("235"))` - darker background
  - `Padding(1, 1)` - top/bottom margin

### 3. Add darker background to model/assistant answers
**File:** `internal/tui/render.go` - `renderAssistantText()`
- Wrap assistant text in a styled box with darker background
- Use `Background(lipgloss.Color("234"))` for distinction from user messages

**File:** `internal/tui/styles.go`
- Add `assistantBlockStyle` with:
  - `Background(lipgloss.Color("234"))`
  - `Padding(0, 1)` for horizontal padding

### 4. Improve spinner - bouncing dots
**File:** `internal/tui/model.go`
- Replace `spinner.New(spinner.WithSpinner(spinner.Line))` with custom bouncing dots spinner
- Create custom spinner frames: `["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"]` (braille spinner) or bouncing dots pattern

**File:** `internal/tui/styles.go`
- Add `spinnerStyle` with color styling

## Implementation Details

### Spinner Options
The bubbles library supports custom spinners via `spinner.WithSpinner()`. We can define a custom spinner with frames that create a bouncing dots effect:

```go
var bouncingDots = spinner.Spinner{
    Frames: []string{"∙∙∙", "●∙∙", "∙●∙", "∙∙●", "∙∙∙"},
    FPS:    time.Second / 10,
}
```

Or use a more elaborate braille-based spinner for smoother animation.

### Color Palette
- Input background: `235` (dark gray)
- Assistant background: `234` (slightly darker gray)
- Current terminal background: default (typically `235` or similar)

### Layout Changes
- Input area will have visible separation from viewport content
- Footer stays at the very bottom
- Model answers visually distinct from user input

## Testing
- Run existing tests: `go test ./internal/tui/...`
- Manual verification with `go run cmd/tau/main.go`

## 5. Fix scroll jumping during streaming
**Problem:** Viewport unconditionally calls `GotoBottom()` on every streaming event, causing the view to "jump" even when the user has scrolled up to read earlier content.

**File:** `internal/tui/model.go` - `updateViewport()`

**Fix:** Check if the viewport is at the bottom before updating. Only auto-scroll to bottom if the user was already there. Otherwise, preserve their scroll position.

```go
func (m *Model) updateViewport() {
    content := renderBlocks(m.blocks, m.width)
    if m.pendingBuilder.Len() > 0 {
        content += renderPendingBlock(m.pendingBuilder.String(), m.pendingKind, m.width)
    }

    // Track scroll position before updating content.
    wasAtBottom := m.viewport.AtBottom()
    oldYOffset := m.viewport.YOffset()

    m.viewport.SetContent(content)

    // Auto-scroll to bottom only if user was already at (or near) the bottom.
    if wasAtBottom {
        m.viewport.GotoBottom()
    } else {
        // Preserve scroll position after content replacement.
        maxOffset := max(0, m.viewport.TotalLineCount()-m.viewport.VisibleLineCount())
        if oldYOffset > maxOffset {
            oldYOffset = maxOffset
        }
        m.viewport.SetYOffset(oldYOffset)
    }
}
```

**Behavior:**
- When user is at the bottom, new content auto-scrolls into view
- When user scrolls up, their position is preserved during streaming
- No more "jumping" back to bottom while reading earlier messages

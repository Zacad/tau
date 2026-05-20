# Task 038 Subtask 4: PaletteList Component

## Overview
Create a reusable `PaletteList` component in a new `internal/tui/palette/` package, refactor `CommandPalette` to delegate to it, and evolve the `/test` command to demonstrate the component with custom options.

## Files to Create

### 1. `internal/tui/palette/styles.go`
Shared palette styles extracted from `tui/styles.go`:
- `CursorStyle`, `NameStyle`, `SelectedNameStyle`, `DescStyle`
- `SearchStyle`, `DisabledNameStyle`, `DisabledDescStyle`, `DisabledTagStyle`
- `BoxStyle`, `DimStyle`, `OverlayStyle`

### 2. `internal/tui/palette/list.go`
PaletteList component:

```go
package palette

import (
    "sort"
    "strings"

    tea "charm.land/bubbletea/v2"
    "charm.land/bubbles/v2/textinput"
    "charm.land/lipgloss/v2"
)

const MaxVisible = 11

type PaletteItem interface {
    Title() string
    Description() string
    FilterValue() string
}

type PaletteList struct {
    items       []PaletteItem
    avail       []bool
    selected    int
    search      textinput.Model
    filtered    []PaletteItem
    filteredAvail []bool
    positions   [][]int
    done        bool
    cancelled   bool
    result      PaletteItem
    resultIndex int
    width       int
    height      int
}

func (l *PaletteList) Init(items []PaletteItem, avail []bool)
func (l *PaletteList) Update(msg tea.Msg) tea.Cmd
func (l *PaletteList) View() string
func (l *PaletteList) Done() bool
func (l *PaletteList) Cancelled() bool
func (l *PaletteList) Result() (PaletteItem, int)
func (l *PaletteList) SetSize(width, height int)
func (l *PaletteList) Up()
func (l *PaletteList) Down()
func (l *PaletteList) Select()
func (l *PaletteList) Cancel()
func (l *PaletteList) SearchValue() string
```

Key behaviors:
- `Init()` initializes search input, focuses it, filters all items
- `Update()` routes keypresses: up/down navigate, enter selects, esc cancels; other keys route to search input and re-filter
- `View()` renders search bar + filtered list with scroll
- `Done()` returns true after Enter selection
- `Cancelled()` returns true after Esc
- `Result()` returns (selectedItem, index) or (nil, -1) if cancelled
- Unavailable items shown dimmed, skipped during navigation
- Fuzzy matching using `fuzzyMatch` function (will need to be accessible)

### 3. `internal/tui/palette/list_test.go`
Comprehensive tests:
- `TestPaletteList_Init` — items loaded, search focused, all shown
- `TestPaletteList_Render` — non-empty view, contains item titles
- `TestPaletteList_Navigation` — up/down wrap, selection changes
- `TestPaletteList_Select` — Enter sets Done, Result returns item
- `TestPaletteList_Cancel` — Esc sets Cancelled
- `TestPaletteList_SearchFiltering` — empty=all, partial=fuzzy, clear
- `TestPaletteList_NavigationWithFilter` — navigates filtered list
- `TestPaletteList_Scrolling` — selection stays visible with many items
- `TestPaletteList_DisabledItems` — shown dimmed, not selectable
- `TestPaletteList_EmptyItems` — handles empty input gracefully

## Files to Modify

### 4. `internal/tui/palette.go`
Refactor to delegate to PaletteList:

**New types:**
```go
type commandItem struct {
    cmd   Command
    avail bool
}
func (c commandItem) Title() string
func (c commandItem) Description() string
func (c commandItem) FilterValue() string
```

**CommandPalette changes:**
- Add `list PaletteList` field
- Remove: `filtered`, `filteredAvail`, `positions`, `search` fields (moved to PaletteList)
- `Open()` → builds `[]commandItem` + `[]bool` avail, calls `list.Init()`
- `Update()` → delegates to `list.Update()`, checks `list.Done()`/`list.Cancelled()`
- `Up()/Down()/Selected()` → delegate to `list`
- `renderBox()` → uses `list.View()`
- Remove: `filterCommands()`, `ensureSelectableSelection()`, `isSelectable()`, `selectNextAvailable()`, `getVisibleCommands()`, `renderSearchInput()`, `renderPaletteItem()` (all moved to PaletteList)

### 5. `internal/tui/command.go`
Evolve `/test` command:

**New test option item type:**
```go
type testOptionItem struct {
    title string
    desc  string
}
func (t testOptionItem) Title() string
func (t testOptionItem) Description() string
func (t testOptionItem) FilterValue() string
```

**cmdTest changes:**
- Opens palette with PaletteList containing: "Print message", "Show info", "Cancel"
- Selection handler:
  - "Print message" → append "Test command executed" to viewport
  - "Show info" → append "Test info: palette component working" to viewport
  - "Cancel" → no-op
- Need mechanism to populate palette with custom items and handle selection result

**Approach for /test:**
Add a callback mechanism to CommandPalette:
```go
type PaletteHandler func(result palette.PaletteItem, index int) tea.Cmd

func (p *CommandPalette) OpenWithHandler(commands []Command, handler PaletteHandler)
func (p *CommandPalette) OpenWithItems(items []palette.PaletteItem, avail []bool, handler PaletteHandler)
```

Or simpler: add a `selectionHandler` field that gets called when palette selection is made.

### 6. `internal/tui/palette_test.go`
Update existing tests:
- Tests that directly access `p.filtered`, `p.search`, etc. need to use `p.list` instead
- `TestPalette_SearchFiltering` → test via PaletteList
- `TestPalette_NavigationWithFilteredResults` → test via PaletteList
- `TestPalette_ScrollingWithFilteredResults` → test via PaletteList
- `TestPalette_DisabledCommandsShownButNotSelectable` → test via PaletteList
- `TestPalette_SearchInputFocusOnOpen` → test via PaletteList
- `TestPalette_SearchValueClearedOnClose` → test via PaletteList
- Keep high-level CommandPalette tests: open/close, Ctrl+P, Esc, navigation, enter execution, resize, overlay rendering

### 7. `internal/tui/command_test.go`
Update `/test` command test:
- `TestTestCommand_Execution` → verify palette opens with test options
- Add test for selection execution

## fuzzyMatch Accessibility
The `fuzzyMatch` function is currently in `tui` package. Options:
1. Move `fuzzyMatch` to a shared utility package (e.g., `internal/fuzzy/`)
2. Export `fuzzyMatch` from `tui` and import in `palette` (creates circular dependency)
3. Pass `fuzzyMatch` as a function parameter to PaletteList
4. Duplicate fuzzy matching logic in palette package

**Decision**: Option 1 — move `fuzzyMatch` to `internal/fuzzy/fuzzy.go`. This is the cleanest approach and avoids circular dependencies. The function is pure and has no TUI dependencies.

## Implementation Order
1. Create `internal/fuzzy/fuzzy.go` — move `fuzzyMatch` from `tui`
2. Create `internal/tui/palette/styles.go`
3. Create `internal/tui/palette/list.go`
4. Create `internal/tui/palette/list_test.go`
5. Refactor `palette.go` to use PaletteList
6. Update `palette_test.go`
7. Evolve `/test` command in `command.go`
8. Update `command_test.go`
9. Run `go vet ./...`, `go build ./...`, `go test ./...`
10. Rebuild binary

## Acceptance Criteria (from task.md)
- `go vet ./...` clean
- `go build ./...` clean
- All tests pass (full suite)
- `Ctrl+P` → works as before
- `/test` → palette shows test options → select one → action executes

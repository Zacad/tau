# Task 033: Fuzzy Command Matching

## Why

Task 032 introduces an inline dropdown with prefix matching. Users expect fuzzy matching — typing `/ses` should find `/session`, but typing `/mod sel` should also find `/model` (matching "model" + "selector"). Fuzzy matching is standard in every modern CLI tool (fzf, telescope, VS Code command palette) and significantly improves discoverability when users don't remember exact command names.

## Current State (After Task 032)

- Dropdown uses prefix matching: `/h` matches `/help`, `/he` matches `/help`
- No fuzzy matching: `/hp` finds nothing, `help` typed after `/` only matches from start
- Command filtering in `CommandDropdown` model uses `strings.HasPrefix`

## Comparison with Existing Tools

### PI
- Uses `fuzzyFilter()` from `@earendil-works/pi-tui` — filters items by a key function
- Allows matching characters anywhere in the string, not just prefix
- Used for commands, models, skills, sessions, files

### OpenCode
- Command palette (Ctrl+P) uses fuzzy matching
- Slash command completion uses prefix matching

### fzf (industry standard)
- Score-based fuzzy matching with character position weighting
- Consecutive character matches score higher
- First character match scores higher

## Proposed Design

### Algorithm

Implement a simple fuzzy matching algorithm (no external dependencies):

```go
func fuzzyMatch(pattern, target string) (score int, matched bool)
```

Scoring rules:
1. All pattern characters must appear in target in order (subsequence match)
2. Consecutive character matches get bonus points
3. Match at word boundary (after `/`, `-`, `_`, space) gets bonus points
4. Match at start of string gets highest bonus
5. Shorter match distance scores higher

Example scoring:
- `/ses` vs `/session` → high score (prefix match + consecutive)
- `/sion` vs `/session` → medium score (consecutive but not prefix)
- `/sn` vs `/session` → low score (non-consecutive)

### Integration

- Replace `strings.HasPrefix` in `CommandDropdown.filterCommands()` with fuzzy matching
- Sort results by score (descending)
- Optionally highlight matched characters in the dropdown (future polish)

### Implementation Location

- New file: `internal/tui/fuzzy.go`
- Tests: `internal/tui/fuzzy_test.go`
- Integration: `internal/tui/dropdown.go` filter method

## Constraints

- Must be fast (O(n*m) where n=pattern length, m=target length — commands list is small)
- No external dependencies
- Must preserve prefix-match-as-highest-score behavior (backward compatible with 032)

## Subtasks

### 033.1: Fuzzy Match Algorithm ✅
- Implement `fuzzyMatch(pattern, target string) (score int, matched bool)`
- Implement scoring with bonuses for consecutive, word-boundary, start-of-string
- Unit tests covering all scoring rules and edge cases

### 033.2: Dropdown Integration ✅
- Replace prefix filtering with fuzzy matching in CommandDropdown
- Sort filtered results by score descending
- Update tests

### 033.3: Match Highlighting (Optional Polish) ✅
- Highlight matched characters in dropdown items
- Use lipgloss styling (bold or different color for matched chars)
- Handle ANSI escape sequences correctly

## Acceptance Criteria

- [x] 1. `/ses` matches `/session` (prefix — highest score)
- [x] 2. `/sion` matches `/session` (non-prefix fuzzy — lower score)
- [x] 3. `/sn` matches `/session` (sparse fuzzy — lowest score)
- [x] 4. `/bogus` matches nothing
- [x] 5. Results sorted by score (best match first)
- [x] 6. Empty pattern after `/` shows all commands
- [x] 7. All existing tests pass
- [x] 8. go vet / go build / go test -race clean

## Out of Scope

- Match highlighting in UI (subtask 033.3, optional)
- Frecency sorting (frequency + recency) — future task
- Fuzzy matching for skills, models, files — separate tasks

## Files to Modify

- New: `internal/tui/fuzzy.go`
- New: `internal/tui/fuzzy_test.go`
- Modify: `internal/tui/dropdown.go` (filter method)

## Reference

- PI's `fuzzyFilter()` in `@earendil-works/pi-tui`
- fzf scoring algorithm (for inspiration, not direct implementation)

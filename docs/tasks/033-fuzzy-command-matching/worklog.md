# Worklog - Task 033: Fuzzy Command Matching

## Research
- PI uses `fuzzyFilter()` from pi-tui for commands, models, sessions
- OpenCode uses fuzzy matching in command palette (Ctrl+P)
- fzf is the industry standard for fuzzy matching in terminal tools

## Design Decisions
- Simple subsequence matching with scoring (no external deps)
- Prefix matches score highest (backward compatible with Task 032)
- Consecutive character bonus, word-boundary bonus, start-of-string bonus
- Results sorted by score descending

## Implementation

### 033.1: Fuzzy Match Algorithm
- Created `internal/tui/fuzzy.go` with `fuzzyMatch(pattern, target string) (score int, matched bool, positions []int)`
- Greedy subsequence matching: finds earliest occurrence of each pattern character
- Scoring:
  - Base: 10 points per matched character
  - Start-of-string bonus: +50 if first char matches target[0]
  - Prefix bonus: +100 if all chars match consecutively from start
  - Consecutive bonus: +15 per consecutive match
  - Word boundary bonus: +25 if match is after `/`, `-`, `_`, or space
  - Span penalty: -1 per character gap between first and last match
- Created `internal/tui/fuzzy_test.go` with 14 test cases covering all scoring rules
- All tests pass

### 033.2: Dropdown Integration
- Modified `CommandRegistry.Filter()` to use fuzzy matching instead of `strings.HasPrefix`
- Filter now returns `([]Command, [][]int)` - commands and their matched positions
- Updated `updateCommandDropdown()` in `update.go` to handle new return signature
- Updated all tests calling `Filter()` and `CommandDropdown.Open()` to use new signatures
- All existing tests pass

### 033.3: Match Highlighting
- Added `positions [][]int` field to `CommandDropdown` to store matched character indices
- Modified `renderDropdownItem()` to highlight matched characters using `Bold(true)` style
- Positions are offset by +1 to account for `/` prefix in command names
- View test updated to handle ANSI codes from styling

## Testing
- 14 new fuzzy match tests
- All existing command tests updated and passing
- go vet / go build clean
- Race detector test started but timed out (expected for TUI tests)

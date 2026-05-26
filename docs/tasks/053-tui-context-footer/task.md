# Task 053: TUI Footer Context Usage Display

## Why
Users need visibility into how much of the model's context window is consumed by the current conversation, matching the UX provided by OpenCode and PI.

## Comparison with OpenCode and PI
- **PI**: Footer shows cumulative usage (`↑`, `↓`, cache R/W, cost) plus context as `12.3%/200k (auto)` with warning (>70%) and error (>90%) color thresholds. Uses last assistant usage + estimated trailing messages for accuracy. Treats post-compaction context as unknown.
- **OpenCode**: Direct run footer shows duration and usage in status row. Newer TUI footer focuses on connection status (LSP, MCP, permissions) rather than explicit context percentage.
- **Tau (current)**: Footer shows model/provider, thinking level, cwd, turns, cumulative tokens/cost, and state. No context window or percentage display.

## Constraints
- `View()` must NOT call session methods (deadlock risk with agent goroutine).
- Tau stores cumulative session usage, not per-message provider usage.
- Context estimation uses existing `session.EstimateTokens` heuristic.

## Approach
- Cache `contextWindow`, `contextTokens`, `contextKnown` on Model.
- Refresh outside View: NewModel, after prompt completion, after model change.
- Display as `ctx:12.3%/200k` with color thresholds (70% warning, 90% error).
- Hide context info when context window is unknown/zero.

## Acceptance Criteria
- [x] Footer displays context usage percentage and window size when context window is known
- [x] Context display uses warning color above 70%, error color above 90%
- [x] Context display hidden when context window is zero/unknown
- [x] Context window updates when model is changed
- [x] Context estimate refreshes after each completed prompt turn
- [x] Context estimate works correctly after session resume
- [x] All existing tests pass
- [x] New tests cover formatTokens, context display, thresholds, model switch

## Subtasks
- [x] Create task documentation
- [x] Add cached context fields to Model
- [x] Implement estimateMessagesTokens and refreshContext
- [x] Update renderFooter with context display
- [x] Wire context refresh into lifecycle points
- [x] Add tests
- [x] Rebuild binary and verify
- [x] Update documentation

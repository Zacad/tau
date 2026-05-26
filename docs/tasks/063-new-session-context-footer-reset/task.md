# Task 063: Reset Footer Context Usage On New Session

## Why

Users expect `/new` to create a clean session state immediately. The current TUI resets messages, turns, and cumulative usage, but the footer can still display the previous session's context usage percentage. That is misleading because it suggests the fresh session has already consumed context.

## Comparison With PI and OpenCode

- **PI**: Footer context usage is derived from the active session during render. When the session changes, the footer reads the new session state instead of preserving stale cached values.
- **OpenCode**: Footer/session UI is driven by reactive route/session state and updated explicitly when the active session changes.
- **Tau (current bug)**: Footer context usage is cached on the TUI model. `/new` resets other session-derived fields but does not refresh the cached context state, so stale values survive into the new session.

## Main Constraints

- `View()` must not call session methods because of the existing deadlock constraint.
- Fix should remain consistent with the current cached-context design introduced in task 053.
- New-session behavior should stay aligned with resume behavior, which already refreshes cached context state.
- Keep the change minimal and localized.

## Design

After `sdk.Session.NewSession()` succeeds, the TUI `/new` command should refresh cached footer context state from the now-empty active session. This keeps the cached approach while ensuring `/new` behaves like other session lifecycle transitions.

## Subtasks

- [ ] Document the bug and reference comparison
- [ ] Update `/new` command to refresh cached context state
- [ ] Add regression test covering stale footer context after `/new`
- [ ] Run targeted and relevant tests
- [ ] Rebuild binary for manual verification
- [ ] Update tracking and worklog

## Acceptance Criteria

- [ ] After `/new`, footer context usage no longer shows the previous session's value
- [ ] After `/new`, footer context usage reflects an empty new session when context window is known
- [ ] Existing `/new` behavior for turns and cumulative usage reset remains unchanged
- [ ] Regression test covers the stale-context case
- [ ] Relevant tests pass
- [ ] Binary rebuild succeeds

## Testing Strategy

- Add a TUI regression test that seeds stale cached context values, executes `/new`, and verifies footer output is reset.
- Run targeted TUI tests for `/new` and footer rendering behavior.
- Run broader relevant package tests if targeted tests pass.

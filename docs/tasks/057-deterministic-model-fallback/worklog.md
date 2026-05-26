# Task 057: Deterministic Model Fallback - Worklog

## 2026-05-24

### Analysis
- Traced model selection flow through `CreateSession`, `ResumeSession`, and `resolveModel`
- Identified non-deterministic Go map iteration in `ModelRegistry.ListAll()` as root cause
- Found that `ResumeSession` ignored config default when resumed model was unavailable

### Implementation
- Added `resolveModelResult` struct and `resolveModel()` helper function in `internal/sdk/sdk.go`
- Helper implements priority: explicit pattern > config default > deterministic connected fallback
- Fallback sorts candidates by provider then model ID for determinism
- Only considers models from registered/connected providers
- Explicit CLI model failures do NOT silently fall back

### Files Changed
- `internal/sdk/sdk.go`: Added `resolveModel()` helper, refactored `CreateSession` and `ResumeSession`
- `internal/sdk/sdk_test.go`: Added 4 new tests

### Tests Added
- `TestCreateSession_ConfigDefaultUsedWhenSessionModelUnavailable`
- `TestCreateSession_AutoFallbackDeterministic`
- `TestCreateSession_ConfigDefaultProviderUnavailable_FallsBackToConnectedOnly`
- `TestResumeSession_UsesConfigDefaultWhenResumedProviderUnavailable`

### Build
- `go build ./...` succeeds
- `go test ./internal/sdk/...` passes (all 6.6s)
- Binary rebuilt at `./tau`

### Additional Fix: New Session Model Selection
- Identified that when creating a new session (not resume/continue), tau wasn't checking the most recent session file for its model
- Added logic to read the most recent session file's model before creating a new session
- New session now picks up the most recent session's model as a fallback before config default
- Added test `TestCreateSession_NewSessionPicksUpMostRecentSessionModel`

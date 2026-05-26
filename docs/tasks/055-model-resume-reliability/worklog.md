# Worklog: Task 055 - Model Resume Reliability

## 2026-05-24

### Investigation
- Analyzed session selection flow: `CreateSession` → `resumeMostRecent` → `config.LatestSessionFile`
- Found `LatestSessionFile` sorts by filename string (creation timestamp prefix), not file modification time
- Confirmed PI uses `stat(filePath).mtime` for session ordering
- Found `ResumeSession` loads messages/usage/thinking-level but ignores saved model/provider
- Found `Compact` passes full `SessionEntry` to `Append`, causing double-wrapped compaction data

### Implementation
1. **`internal/config/paths.go`**: Changed `LatestSessionFile` to:
   - Stat each .jsonl file to get modification time
   - Sort by mtime with filename as deterministic tie-breaker
   - Added `time` import

2. **`internal/sdk/sdk.go`**: Updated `ResumeSession` to:
   - Read `CurrentModel()` and `CurrentProvider()` from resumed session
   - Resolve model via `provReg.ResolveModelWithFallback()`
   - Switch agent to resolved model/provider
   - Restore thinking level for resolved model
   - Update subagent tool with new parent model
   - Gracefully fall back if provider unavailable

3. **`internal/sdk/sdk.go`**: Fixed `Compact` to:
   - Pass `tausession.CompactionData` directly instead of `types.SessionEntry`

### Tests
- Added 4 tests for `LatestSessionFile` in `internal/config/paths_test.go`
- Added 2 tests for `ResumeSession` model restore in `internal/sdk/sdk_test.go`
- Added 1 test for compaction data shape in `internal/sdk/sdk_test.go`
- All 7 new tests pass
- Existing tests in `./internal/config/...`, `./internal/session/...`, `./internal/sdk/...` pass

### Build
- Binary rebuilt at `./tau`

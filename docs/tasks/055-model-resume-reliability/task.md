# Task 055: Model Resume Reliability

**Status**: Done
**Date**: 2026-05-24

## Why

The remembered model was unreliable — sometimes Tau would resume with the correct model, sometimes not. The root cause was time-bound session selection: `LatestSessionFile` chose sessions by filename (creation timestamp), not by last modification time. Additionally, `/resume` did not restore the saved model/provider from the resumed session file.

## Comparison with PI and OpenCode

### PI
- `SessionManager.list()` sorts sessions by `modified` time (file mtime), not creation time.
- `buildSessionInfo()` extracts `modified` from `stat(filePath).mtime`.
- Session resume restores model from `model_change` entries.

### OpenCode
- Uses SQLite with `time_created` column for ordering.
- Compaction entries use `time_created` for boundary detection.

### Tau (before fix)
- `LatestSessionFile` sorted by filename string (creation timestamp prefix).
- If two sessions created in same second, random ID suffix decided winner.
- Did not account for sessions modified after creation.
- `ResumeSession` loaded messages/usage but ignored saved model/provider.

## Constraints
- Must not break existing session files or filenames.
- Must handle same-second creation times deterministically.
- Must gracefully handle resumed session model whose provider is unavailable.

## Subtasks

### 055.1: Fix LatestSessionFile to use file modification time
- [x] Stat each .jsonl file and sort by mtime
- [x] Use filename as deterministic tie-breaker when mtime is equal
- [x] Add tests: most recent mtime wins, same-mtime filename tie-break, older file modified later wins

### 055.2: Fix ResumeSession to restore saved model/provider
- [x] Read `CurrentModel()` and `CurrentProvider()` from resumed session
- [x] Resolve model via registry (same pattern as `CreateSession`)
- [x] Switch agent to resolved model/provider
- [x] Restore thinking level for resolved model
- [x] Update subagent tool with new parent model
- [x] Gracefully fall back if resumed model's provider is unavailable
- [x] Add tests for model restore with provider

### 055.3: Fix compaction data shape
- [x] `Compact` was passing full `SessionEntry` to `Append`, causing double-wrapping
- [x] Changed to pass `CompactionData` directly
- [x] Add test verifying compaction data round-trips correctly

### 055.4: Tests and build
- [x] `go test ./internal/config/... ./internal/session/... ./internal/sdk/...` passes
- [x] Binary rebuilt at `./tau`

## Acceptance Criteria
- [x] `Continue` resumes the most recently modified session, not the most recently created
- [x] `/resume` restores the saved model and provider from the resumed session
- [x] Compaction entries are written with correct data shape
- [x] All new tests pass, no regressions

## Worklog

### 2026-05-24
- **Analysis**: Identified 3 bugs causing unreliable model remembering:
  1. `LatestSessionFile` sorts by filename (creation time), not mtime
  2. `ResumeSession` ignores saved model/provider
  3. `Compact` double-wraps compaction data
- **Implementation**:
  - Changed `LatestSessionFile` to stat files and sort by mtime with filename tie-breaker
  - Updated `ResumeSession` to resolve and apply saved model/provider
  - Fixed `Compact` to pass `CompactionData` instead of `SessionEntry`
- **Tests**:
  - `TestLatestSessionFile_ReturnsMostRecent`
  - `TestLatestSessionFile_SameMtime_UsesFilename`
  - `TestLatestSessionFile_OlderFileModifiedLater`
  - `TestLatestSessionFile_NoFiles`
  - `TestSession_ResumeSession_RestoresModel`
  - `TestSession_ResumeSession_RestoresModelWithProvider`
  - `TestSession_Compact_WritesCorrectDataShape`
- **Build**: Binary rebuilt at `./tau`, all tests pass

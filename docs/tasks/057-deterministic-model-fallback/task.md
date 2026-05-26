# Task 057: Deterministic Model Fallback

**Status**: Done
**Date**: 2026-05-24

## Why

Model selection on startup was unreliable — Tau would sometimes open with random `ollama/ministral` or `ollama/gemma` models instead of the user's saved `/model` choice. Root cause: when the preferred model (from session or config) was unavailable, the auto-fallback iterated `ModelRegistry.ListAll()` which iterates a Go map (non-deterministic order), picking "first Ollama model" unpredictably.

Additionally, `ResumeSession` kept the current model when the resumed session's model was unavailable, instead of falling back to the user's configured `default_model`.

## Comparison with PI and OpenCode

### PI
- `findInitialModel()` validates restored model against available providers and auth.
- Falls back to settings default, then provider defaults, then first available.
- All fallback paths are deterministic.

### OpenCode
- `defaultModel()` reads `model.json` recent models, validates each against connected providers.
- Falls back deterministically to first connected provider's first model.
- Never picks from catalog models whose providers aren't registered.

### Tau (before fix)
- `CreateSession` fallback used `provReg.Models().ListAll()` (Go map iteration = non-deterministic).
- Picked "first Ollama model" from unsorted list.
- `ResumeSession` kept current model when resumed model unavailable, ignoring config default.

## Constraints
- Must not break existing session files or config.json.
- Explicit CLI model requests should NOT silently fall back (user should know their request failed).
- Session/config model requests SHOULD fall back to config default, then deterministic connected fallback.
- Fallback must only consider registered/connected providers.

## Subtasks

### 057.1: Add shared resolveModel helper
- [x] Extract model resolution into `resolveModel(pattern, cfgDefault, provReg, explicitCLI)`
- [x] Priority: explicit pattern > config default > deterministic connected fallback
- [x] Only considers models whose providers are registered
- [x] Sorts fallback candidates by provider, then model ID (deterministic)
- [x] Explicit CLI failures do NOT fall back (user should know)

### 057.2: Refactor CreateSession to use helper
- [x] Replace duplicated fallback logic with `resolveModel()` call
- [x] Pass `explicitModel` flag for CLI vs session/config distinction

### 057.3: Refactor ResumeSession to use helper
- [x] Replace manual resolution with `resolveModel()` call
- [x] Falls back to config default when resumed model provider unavailable

### 057.4: Tests
- [x] `TestCreateSession_ConfigDefaultUsedWhenSessionModelUnavailable`
- [x] `TestCreateSession_AutoFallbackDeterministic`
- [x] `TestCreateSession_ConfigDefaultProviderUnavailable_FallsBackToConnectedOnly`
- [x] `TestResumeSession_UsesConfigDefaultWhenResumedProviderUnavailable`

### 057.6: Fix new session model selection
- [x] When creating a new session (not resume/continue), check the most recent session file for its model
- [x] Use the most recent session's model as a fallback before config default
- [x] Add test: `TestCreateSession_NewSessionPicksUpMostRecentSessionModel`

### 057.7: Build and verify
- [x] `go build ./...` succeeds
- [x] `go test ./internal/sdk/...` passes
- [x] Binary rebuilt at `./tau`

## Acceptance Criteria
- [x] Fallback is deterministic across multiple runs (same model picked every time)
- [x] Fallback only uses connected/registered providers
- [x] Config default is used when session model provider is unavailable
- [x] Explicit CLI model requests do not silently fall back
- [x] All new tests pass, no regressions in existing tests

## Worklog

### 2026-05-24
- **Root cause analysis**: Identified non-deterministic Go map iteration in `ListAll()` causing random fallback model selection
- **Reference research**: Studied PI's `findInitialModel()` and OpenCode's `defaultModel()` for fallback patterns
- **Implementation**:
  - Added `resolveModel()` helper with deterministic priority chain
  - Refactored `CreateSession` and `ResumeSession` to use shared helper
  - Added `explicitCLI` flag to prevent silent fallback for explicit CLI requests
- **Tests**: Added 4 new tests covering config default fallback, deterministic selection, connected-only fallback, and resume fallback
- **Build**: Binary rebuilt at `./tau`, all SDK tests pass

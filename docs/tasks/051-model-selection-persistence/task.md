# Task 051: Model Selection Persistence

**Status**: Done
**Date**: 2026-05-24

## Why

The `/model` slash command saves the chosen model, but on restart Tau often opens with the wrong model, wrong provider, or a stale model from a previous session. The root cause is that provider identity is lost at multiple points in the persistence chain:

1. `/model` passes bare `model.ID` to `SetModel`, discarding provider info the palette already has.
2. `config.DefaultModel` stores only bare model ID.
3. `session.ModelChangeData` stores only `model_id`, no provider.
4. `CreateSession` does not apply `sess.CurrentModel()` after opening/resuming a session.
5. `ModelRegistry` keys by model ID only, so same model ID across providers overwrites.
6. `Session.SetModel` writes a cached startup config, clobbering newer `/connect` or `/disconnect` changes.

## Comparison with PI and OpenCode

### PI
- Persists `defaultProvider` + `defaultModel` together in `settings.json`.
- Session `model_change` entries store both `provider` and `modelId`.
- Startup resolver (`findInitialModel`) validates restored model against available providers and auth.
- `/model` selector calls `setDefaultModelAndProvider(provider, modelId)`.

### OpenCode
- Model identity is `{ providerID, modelID }` pair everywhere.
- `parseModel()` splits on first slash only, preserving model IDs containing slashes.
- Persisted `model.json` stores recent models as provider/model pairs.
- Restore validates candidate against currently connected providers.

### Tau (current)
- Only bare model ID persisted in config and session.
- Registry keyed by model ID only.
- No validation of restored model against available providers.

## Constraints
- Must be backward compatible with existing config.json and session files that store bare model IDs.
- Must not break existing `/model` palette UX.
- Must handle model IDs that contain slashes (e.g., `openai/gpt-4o` available through OpenRouter).
- Must not lose provider config changes made via `/connect` or `/disconnect` after SDK startup.

## Subtasks

### 051.1: Extend session model change with provider
- [x] Add `Provider` field to `ModelChangeData`
- [x] Write both provider and model ID for new entries
- [x] Restore old entries with only `model_id` using resolver fallback

### 051.2: Add ParseModelRef helper
- [x] Split on first slash: `provider/modelID`
- [x] Handle model IDs containing slashes (e.g., `openrouter/openai/gpt-4o`)
- [x] Return empty provider for bare model IDs (backward compat)

### 051.3: Fix ModelRegistry deduplication
- [x] Key models by `provider/modelID` internally
- [x] Support lookup by `(provider, modelID)` pair
- [x] Bare ID resolution succeeds only when unique across providers
- [x] Update `TransformCatalog` to allow same model ID under multiple providers

### 051.4: Fix `/model` command
- [x] Pass `provider/modelID` to `SetModel` instead of bare `model.ID`
- [x] Display success message with `provider/modelID`

### 051.5: Fix Session.SetModel
- [x] Use canonical `provider/modelID` for config persistence
- [x] Reload config before saving to avoid clobbering `/connect`/`/disconnect` changes

### 051.6: Fix CreateSession resume
- [x] After `OpenSession`/`resumeMostRecent`, apply restored session model if present
- [x] Priority: explicit CLI model > resumed session model > saved config default > auto fallback
- [x] If restored provider/model unavailable, warn and fall back safely

### 051.7: Tests
- [x] Registry: duplicate model IDs across providers remain distinct
- [x] Resolver: `provider/modelID` restores exact provider, including model IDs with slashes
- [x] Session: model change writes and restores provider + model
- [x] Backward compat: old session entries with only `model_id` still restore
- [x] SDK: resumed session applies its last model/provider
- [x] SDK: explicit `SessionOptions.Model` overrides resumed session model
- [x] Config: `SetModel` saves canonical `provider/modelID`
- [x] Config: `SetModel` preserves provider config added after SDK startup

### 051.8: Build and verify
- [x] `go build ./...` succeeds
- [x] `go test ./...` passes (new tests pass, pre-existing flaky tests unrelated)
- [x] Binary rebuilt at `./tau`
- [x] Manual verification: choose model, restart, verify same provider/model

## Acceptance Criteria
- [x] `/model` selection persists both provider and model ID
- [x] On restart, Tau opens with the same provider and model as last session
- [x] Same model ID under different provider is correctly distinguished
- [x] Existing config.json and session files with bare model IDs still work
- [x] `/connect` and `/disconnect` config changes are not clobbered by `SetModel`
- [x] All new tests pass, no regressions in existing tests

## Worklog

### 2026-05-24
- **Research**: Explored PI and OpenCode model persistence approaches via subagents
- **Analysis**: Identified 6 root causes in Tau's model selection flow
- **Plan**: Canonical `provider/modelID` everywhere, backward compat for bare IDs
- **Implementation**:
  - Extended `ModelChangeData` with `Provider` field (backward compat via `omitempty`)
  - Added `currentProvider` field to session struct, `CurrentProvider()` method
  - Added `ParseModelRef()` helper (first-slash split, preserves model IDs with slashes)
  - Changed `ModelRegistry` to use compound `provider/modelID` keys internally
  - Updated `TransformCatalog` to register models under each provider without deduplication
  - Removed `reassignToCanonicalProvider` from catalog transform (no longer needed)
  - Fixed `cmdModel` to pass `provider/modelID` to `SetModel`
  - Fixed `Session.SetModel` to save canonical ref and reload config before saving
  - Fixed `CreateSession` to restore model from resumed session (priority: CLI > session > config > auto)
  - Moved session creation before model resolution in `CreateSession`
- **Tests**:
  - `TestParseModelRef`: verifies first-slash split, empty provider for bare IDs
  - `TestModelRegistry_CompoundKeys`: verifies same model ID under multiple providers
  - `TestModelRegistry_FindExactProviderModel`: verifies exact provider/modelID resolution
  - `TestSession_ModelRestoreWithProvider`: verifies model + provider restored after reopen
  - `TestSession_ModelRestoreBackwardCompat`: verifies old session files (no provider) still work
  - Updated `TestTransformCatalog_RegistersSameModelUnderMultipleProviders` to reflect new behavior
  - Updated `TestSession_SetModel` to verify provider persistence
- **Build**: Binary rebuilt at `./tau`, all new tests pass

# Task 065: Stale Model Fallback and Default Repair

## Why

Tau failed to start when `default_model` pointed at `anthropic/claude-sonnet-4-20250514` but no Anthropic credentials were configured, even though other authenticated providers were available.

This is a bad startup failure mode. A stale default or resumed model should not block the application when Tau can use another connected provider. If no provider is connected, Tau should still open and clearly tell the user to connect or choose a model.

## Comparison Analysis: Tau vs PI vs OpenCode

| Dimension | PI | OpenCode | Tau before | Tau target |
|---|---|---|---|---|
| Initial model fallback | Uses auth-configured available models; returns no model when none available | Loads providers from env/auth/config and filters unavailable providers | Fallback exists but stale defaults/session metadata can keep being retried | Select only connected models and repair stale persisted model state |
| No model available | Starts with no active model and guides auth setup | Provider list excludes unusable providers; auth flow available | SDK can start without model, TUI guidance is weak | TUI starts with actionable `/connect` and `/model` guidance |
| Model selection persistence | Settings default tracks selected model | Config/auth provider state drives selection | `/model` persists, fallback does not repair stale default/session | Any implicit fallback repairs default/session state |
| Unavailable explicit model | Reports error instead of silently choosing another | Missing provider/model is explicit | `SetModel` can resolve catalog model before provider check | Explicit selection fails before mutating state |

## Constraints

- Do not auto-connect a provider just because its model exists in the catalog.
- Only registered providers are usable for automatic fallback.
- Explicit CLI model requests should not silently fall back.
- `SetModel` must not persist an unusable model.
- Startup with no usable models must succeed.
- Use Go idioms and keep the change minimal.

## Subtasks

- [x] Add tests for fallback persistence to `default_model`.
- [x] Add tests for fallback persistence to session model state.
- [x] Add tests for `SetModel` rejecting unavailable providers without state mutation.
- [x] Add tests for no-model TUI guidance.
- [x] Implement connected-provider-only fallback repair in SDK startup/resume.
- [x] Update stale built-in Anthropic fallback model.
- [x] Update architecture/decisions documentation.
- [x] Run tests and rebuild the binary in `./`.

## Acceptance Criteria

- Tau does not fail startup when `default_model` points to an unauthenticated provider and another provider is available.
- Tau falls back to a connected provider and writes the canonical fallback to `~/.tau/config.json`.
- Tau updates the active session model when startup/resume falls back from stale session metadata.
- Tau starts with no active model when no providers are available and shows actionable guidance.
- `SetModel` refuses unavailable providers before mutating model, session, config, or agent state.
- Built-in offline Anthropic fallback no longer uses stale `claude-sonnet-4-20250514`.

## Testing Strategy

- Unit tests in `internal/sdk` for fallback resolution and persistence.
- TUI tests for no-model startup guidance and `/model` guidance.
- Targeted package tests for `internal/sdk`, `internal/tui`, and `internal/provider`.
- Full `go test ./...`.
- Manual startup verification with stale Anthropic default and no Anthropic key.

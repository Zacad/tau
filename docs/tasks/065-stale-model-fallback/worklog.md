# Worklog: Task 065 Stale Model Fallback and Default Repair

## 2026-05-28

- Reproduced configuration facts read-only: `default_model` pointed to `anthropic/claude-sonnet-4-20250514`, `auth.json` had no `anthropic`, and `ANTHROPIC_API_KEY` was unset.
- Compared behavior with PI and OpenCode patterns: both treat only auth-configured/loaded providers as usable for automatic selection.
- Got subagent design review. Key findings: fallback did not repair persisted defaults/session state, `SetModel` could mutate state before verifying provider registration, and TUI no-model guidance was weak.
- Started implementation with TDD-oriented task definition and acceptance criteria.
- Added SDK tests for default repair, session model repair, and `SetModel` no-mutation behavior.
- Added TUI tests for startup and `/model` no-model guidance.
- Implemented connected-provider-aware model resolution so bare IDs prefer registered providers before catalog-only providers.
- Implemented fallback persistence to session and `default_model`.
- Updated `SetModel` to reject unconnected providers before mutating state.
- Updated built-in offline Anthropic fallback from `claude-sonnet-4-20250514` to `claude-sonnet-4-6`.
- Ran targeted tests: `go test ./internal/sdk ./internal/tui ./internal/provider` passed.
- Ran full tests: initial run failed because local Ollama/SearXNG were unavailable; after starting Docker services, full run reached model-dependent E2E tests but failed due nondeterministic local model outputs in existing subagent/tool E2E tests.
- Rebuilt binary in workspace root: `go build -o ./tau ./cmd/tau`.

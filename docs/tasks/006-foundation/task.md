# Task 006: Foundation (types + config + infrastructure)

## Why

Every implementation task depends on foundation types, configuration, and test infrastructure. This is the first implementation task — it establishes the core data structures, config loading, and shared test utilities that all subsequent tasks build on.

## Comparison Analysis: Foundation Setup vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Types Package | Multiple type files across packages (pi-ai, pi-agent-core) | Single `internal/types/` — all shared types, no import cycles |
| Configuration | Complex config with themes, extensions | Minimal JSON config + auth.json with 4-step resolution |
| Test Infrastructure | Unknown | Shared `internal/testutil/` with mock Provider, Tool, temp filesystem |
| External Dependencies | yaml.v3 + jsonschema only | 3 deps: yaml.v3, jsonschema, readline |

## Main Constraints

- `types/` package must have zero internal dependencies
- `config/` must work without any other internal packages
- All types must be usable by provider, tools, skills, subagent, session, and agent packages
- `testutil/` must provide reusable mocks for all subsequent tasks
- Only three external dependencies allowed: `gopkg.in/yaml.v3`, `github.com/invopop/jsonschema`, `github.com/chzyer/readline`

## Subtasks

- [x] **006.1** — `go.mod` setup, add external dependencies (`gopkg.in/yaml.v3`, `github.com/invopop/jsonschema`)
- [x] **006.2** — `internal/types/` — core data structures (AgentMessage, ContentBlock, ToolCallBlock, ToolResult, SessionEntry, StreamEvent, Usage, CostInfo, ExecutionMode, Model)
- [x] **006.3** — `internal/types/` — `Model` struct defined here (not in provider/). Provider imports types.Model to avoid circular dependency.
- [x] **006.4** — `internal/config/` — config loading, path resolution, defaults
- [x] **006.5** — `internal/config/` — context files discovery (AGENTS.md/CLAUDE.md search list computation)
- [x] **006.6** — `internal/testutil/` — mock Provider, mock Tool, temp filesystem helpers
- [x] **006.7** — `internal/types/errors.go` — Sentinel error types (ProviderError, ToolError, SessionError) with Go error wrapping
- [x] **006.8** — Unit tests for types, config, testutil

**Logging convention**: Use `log/slog` (stdlib Go 1.21+). Each package creates a package-level logger. Test utilities capture log output for assertions.

## Acceptance Criteria

- [x] `go.mod` initialized, `go mod tidy` succeeds (3 dependencies: yaml.v3, jsonschema, readline)
- [x] All types from ARCHITECTURE.md §8.1 defined and compile
- [x] `types.Model` struct defined in `types/` — provider imports types.Model, no circular dependency
- [x] Config loads from `~/.tau/config.json` with sensible defaults
- [x] Path resolution returns correct built-in, global, and project paths
- [x] `internal/config/` computes AGENTS.md/CLAUDE.md search list: returns `[]string` of file paths to read
- [x] Agent (012) calls `os.ReadFile` on each path returned by config — config does NOT read file contents
- [x] CWD encoding produces human-readable directory names
- [x] `internal/testutil/` provides MockProvider, MockTool, temp filesystem helpers
- [x] All tests pass (use `go test ./...`)
- [x] Zero internal dependencies on other `internal/` packages (except testutil→types)

## Testing & Verification Strategy

**Unit tests**:
- Types: zero-value initialization, JSON serialization round-trip for `SessionEntry`, `AgentMessage`, `ToolResult`
- Config: load valid JSON, missing file (defaults applied), malformed JSON (error returned), auth.json with all 3 key formats
- CWD encoding: root path `/`, nested paths `/a/b/c`, paths with special characters
- testutil: MockProvider streams events on channel, MockTool executes with params and returns result

**Build verification**:
- `go vet ./internal/types/ ./internal/config/ ./internal/testutil/` — zero warnings
- `go build ./...` — compiles cleanly
- `go mod tidy` — no unused dependencies

**Dependency contract verification**:
- No `internal/` package imports another `internal/` package (except testutil→types)
- External dependency count: exactly 3 (yaml.v3, jsonschema, readline)

**Quality gates**:
- All types have package-level godoc comments
- `go test ./internal/types/ ./internal/config/ ./internal/testutil/` — all pass

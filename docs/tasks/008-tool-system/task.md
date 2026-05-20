# Task 008: Tool System

## Why

Tools are how the agent interacts with the filesystem and executes commands. Without tools, the agent is just a chatbot. This task implements the tool interface, execution modes (parallel/sequential/exclusive), file mutation queue, and all 7 built-in tools.

## Comparison Analysis: Tool System vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Tool Interface | `AgentTool` with TypeBox parameters | Go struct parameters + `jsonschema` generation |
| Execution Model | `Promise.all` for parallel, sequential preflight | `errgroup` for parallel, per-file mutex for sequential |
| File Mutation Queue | Per-file promise chain | Per-file `sync.Mutex` chain |
| Built-in Tools | 7 tools (read, bash, edit, write, grep, find, ls) | Same 7 tools, Go implementation |
| Tool Permissions | Extension-based tool allowlisting | Registry-level `WithAllowlist()` and `WithReadOnly()` |
| Truncation | Configurable limits | Configurable limits, full output saved to temp file |

## Main Constraints

- Write/edit tools must be serialized per-file — no concurrent writes to same file
- Bash tool must run exclusively — no other tool executes concurrently with bash
- Tool parameters must be Go structs with JSON Schema generation
- Tool results must include `isError` and `terminate` flags
- Bash tool must support read-only command filtering for subagent contexts

## Dependencies

- `internal/types/` (Task 006)

## Subtasks

- [ ] **008.1** — `internal/tools/tool.go` — Tool interface, Registry, ExecutionMode. Registry exposes `ExecuteBatch()` method for agent loop to call with grouped tool calls.
- [ ] **008.2** — `internal/tools/read.go` — File read tool
- [ ] **008.3** — `internal/tools/grep.go` — Content search tool
- [ ] **008.4** — `internal/tools/find.go` — File search tool
- [ ] **008.5** — `internal/tools/ls.go` — Directory listing tool
- [ ] **008.6** — `internal/tools/write.go` — File write tool
- [ ] **008.7** — `internal/tools/edit.go` — File edit tool (search/replace)
- [ ] **008.8** — `internal/tools/bash.go` — Shell execution tool
- [ ] **008.9** — `internal/tools/truncate.go` — Output truncation utilities
- [ ] **008.10** — `internal/tools/queue.go` — File mutation queue (per-file mutex chain)
- [ ] **008.11** — Unit tests for all tools with temp filesystem

## Acceptance Criteria

- [ ] `Tool` interface matches ARCHITECTURE.md §8.1
- [ ] `ExecutionMode` enforced: parallel (read/grep/find/ls), sequential (write/edit), exclusive (bash)
- [ ] File mutation queue serializes write/edit per file via mutex
- [ ] All 7 built-in tools implemented
- [ ] Tool results include `isError` and `terminate` flags
- [ ] Truncation applies to large outputs; full output saved to temp file when truncated
- [ ] Bash tool supports read-only command filtering for subagent contexts
- [ ] Registry provides `WithAllowlist([]string)` API
- [ ] Registry provides `WithReadOnly(bool)` API
- [ ] Unit tests with temp filesystem for all tools (use `testutil/` helpers)
- [ ] No internal dependencies except `types`

## Testing & Verification Strategy

**Unit tests** (temp filesystem via `t.TempDir()`):
- Each tool: success path, file not found, permission denied, empty file, large file
- read: content returned correctly, truncation applied for large files
- write: file created, parent dirs created, overwrite behavior
- edit: exact match (success), no match (error with diagnostics), multiple matches (error)
- bash: stdout/stderr capture, exit code, timeout, cancellation, read-only mode blocks mutating commands
- grep/find/ls: pattern matching, recursive search, hidden files, symlinks

**Concurrency tests**:
- Parallel: 10 concurrent read tools, all complete without blocking
- Sequential: 5 concurrent write tools to same file, serialized execution verified (no interleaved writes)
- Exclusive: bash + read concurrently, bash runs alone (read waits)
- File mutation queue: 20 concurrent edits to same file, no corruption, correct final state

**Integration tests**:
- `ExecuteBatch()` with mixed tool calls: verify grouping by ExecutionMode, results returned in source order
- Allowlist: registry with `--tools read,grep` — write/edit/bash return error
- Read-only: registry with `WithReadOnly(true)` — write/edit/bash return error

**Race detection**:
- `go test -race ./internal/tools/...` — no data races in per-file mutex queue

**Quality gates**:
- Each tool has ≥80% line coverage
- No tool imports anything except `types` and stdlib

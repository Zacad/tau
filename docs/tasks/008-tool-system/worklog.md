# Task 008: Tool System — Worklog

## Summary

Implemented the complete tool system: Tool interface, Registry with execution mode enforcement, 7 built-in tools, file mutation queue, and output truncation. All tools verified end-to-end with Ollama/llama3.2.

## Files Created

| File | Purpose |
|------|---------|
| `internal/tools/tool.go` | Tool interface, Registry, ExecuteBatch, allowlist/readonly, hooks |
| `internal/tools/truncate.go` | Output truncation with temp file fallback |
| `internal/tools/queue.go` | Per-file mutex chain for serializing writes |
| `internal/tools/read.go` | File read tool (parallel, line offset/limit) |
| `internal/tools/write.go` | File write tool (sequential, creates parent dirs) |
| `internal/tools/edit.go` | File edit tool (sequential, exact match, diagnostics) |
| `internal/tools/bash.go` | Shell execution tool (exclusive, read-only mode) |
| `internal/tools/grep.go` | Content search tool (parallel, regex, glob filter) |
| `internal/tools/find.go` | File search tool (parallel, glob patterns) |
| `internal/tools/ls.go` | Directory listing tool (parallel, long format, hidden files) |
| `internal/tools/*_test.go` | 75 unit tests across all components |

## Implementation Details

### Execution Modes
- **Parallel**: read, grep, find, ls — run concurrently via `sync.WaitGroup`
- **Sequential**: write, edit — serialized per-file via `MutationQueue`
- **Exclusive**: bash — runs alone, no other tool executes concurrently

### Registry Features
- `WithAllowlist([]string)` — restrict available tools
- `WithReadOnly(bool)` — block write/edit/bash
- `WithBeforeToolCall` / `WithAfterToolCall` — pre/post execution hooks
- `ExecuteBatch()` — groups calls by execution mode, returns results in source order
- `ToolDefinitions()` — generates JSON Schema for provider tool calling

### File Mutation Queue
- Per-file `sync.Mutex` with reference counting
- Deadlock avoidance via sorted lock acquisition
- Context cancellation support

### Truncation
- Configurable character limit (default 10,000)
- Full output saved to temp file when truncated
- Temp file path included in truncated output

### Bash Read-Only Mode
- Regex-based detection of mutating commands (rm, mv, cp, mkdir, chmod, git commit/push/pull, redirects, package managers)
- Read-only commands (ls, cat, grep, find) pass through

## Test Results

- **67 tests** — all passing
- **Coverage**: 80.7% (target: ≥80%)
- **Race detection**: clean (`go test -race`)
- **go vet**: clean
- **go build**: clean
- **go mod tidy**: clean
- **Manual testing**: all 7 tools verified end-to-end with Ollama/llama3.2 via `cmd/test-tools/`

## Test Breakdown

| Component | Tests |
|-----------|-------|
| Truncate | 6 |
| MutationQueue | 5 |
| Registry/Tool | 13 |
| ReadTool | 7 |
| WriteTool | 5 |
| EditTool | 5 |
| BashTool | 7 |
| GrepTool | 5 |
| FindTool | 5 |
| LsTool | 7 |
| Integration (mixed modes, sequential) | 2 |

## Subtasks Status

- [x] **008.1** — Tool interface, Registry, ExecuteBatch
- [x] **008.2** — read tool
- [x] **008.3** — grep tool
- [x] **008.4** — find tool
- [x] **008.5** — ls tool
- [x] **008.6** — write tool
- [x] **008.7** — edit tool
- [x] **008.8** — bash tool
- [x] **008.9** — truncation utilities
- [x] **008.10** — file mutation queue
- [x] **008.11** — unit tests for all tools

## Bugs Fixed During Manual Testing

1. **AfterToolCall hook nil dereference** — `hookResult` could be `nil` when hook returns `(nil, nil)` → added nil check in `executeTool()`
2. **Execution ordering** — parallel tools (read) ran before sequential (write), so read couldn't see write's output in the same batch → changed order to: exclusive → sequential → parallel
3. **LLM sends integers as strings** — Ollama sends `limit: "1000"` instead of `limit: 1000` → added `IntOrString` type in read.go for tolerant unmarshaling

## Design Decisions

1. **Tools store workingDir** — passed via constructor for path resolution
2. **Bash non-zero exit → IsError** — agent needs to know when commands fail
3. **FileTool interface** — optional interface for extracting file paths from params, used by Registry for per-file locking
4. **Grep/find skip hidden dirs** — consistent with PI behavior (skip `.git`, `node_modules`, `vendor`)

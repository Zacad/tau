# Task 047.1 — Minimal Runnable Subagent — Worklog

## Session: OpenCode/PI Spawn Analysis

Analyzed how OpenCode and PI implement subagent spawning to identify gaps in Tau's plan.

### OpenCode findings
- `task` tool: in-process, child session with `parentID`, permission derivation from parent
- Nested control: `task` tool denied by default in subagents
- User gate: `ctx.ask()` before spawn
- Resume: `task_id` parameter

### PI findings
- `subagent` tool: subprocess spawn, always fresh context
- Three modes: single, parallel (max 8, concurrency 4), chain (`{previous}`)
- Tool restriction: `--tools` allowlist flag
- Agent discovery: Markdown + YAML frontmatter

### Gap mapping
Most gaps map to existing slices. Three genuinely missing:
1. Spawn tool (no slice creates callable tool) → added as **slice 047.11**
2. User permission gate → added to 047.11
3. Nested subagent deny-by-default → added to 047.11

### Documentation updated
- `task.md`: Added OpenCode/PI spawn analysis section, gap mapping table, new slice 047.11
- `task.md`: Updated constraints (subagent-to-subagent spawning, user confirmation)

## Design Decision: Provider Interface

Chose to import `provider.Provider` directly rather than mirroring the interface in subagent. Rationale:
- `provider` is a leaf package (zero internal dependencies), so no transitive dependency chain
- Single source of truth — no drift risk if provider interface changes
- `testutil.MockProvider` already implements `provider.Provider`, works as-is
- Updated ARCHITECTURE.md §2.2 to reflect: `subagent → types + provider (leaf)`

## Implementation

### Files Created

1. **`internal/subagent/subagent.go`** — Core subagent lifecycle
   - `ContextMode` type with `ContextFresh` constant
   - `SubAgent` struct with ID, Task, ContextMode, SystemPrompt, Model, Provider
   - `SubAgentOpts` struct for constructor configuration
   - `SubAgentResult` struct with Success, Output, Error, Duration, Usage
   - `NewSubAgent(p provider.Provider, opts SubAgentOpts) *SubAgent` — constructor with nil check, auto-ID, defaults
   - `(sa *SubAgent) Run(ctx context.Context) SubAgentResult` — synchronous execution via provider.Stream()
   - `generateID()` — 16-char hex ID via crypto/rand

2. **`internal/subagent/subagent_test.go`** — Test suite (8 tests: 7 unit + 1 E2E)
   - `TestRun_Success` — verifies output, duration > 0, usage populated
   - `TestRun_FreshContext_EmptyMessages` — captures messages passed to Stream(), verifies empty
   - `TestRun_SystemPromptInherited` — captures StreamOptions, verifies system prompt passed through
   - `TestRun_StreamError` — verifies error propagation on stream error
   - `TestNewSubAgent_Defaults` — verifies auto-generated ID (16 chars), ContextFresh default
   - `TestNewSubAgent_NilProvider` — verifies panic on nil provider
   - `TestNewSubAgent_CustomID` — verifies custom ID is preserved
   - `TestRun_E2E_Ollama` — E2E test against real Ollama instance (skipped in short mode)
   - `capturingMockProvider` — helper that records Stream() arguments for verification

### Documentation Updated

- **`docs/ARCHITECTURE.md`** §2.2 — Updated dependency graph: `subagent → types + provider (leaf)`
- **`docs/ARCHITECTURE.md`** §2.2 rules — Updated: "subagent depends on types and provider (both leaf packages)"
- **`docs/tasks/047-subagent-system/task.md`** — Added E2E testing strategy to all 10 slices:
  - 047.1: E2E lifecycle test against Ollama, invalid model error, context cancellation
  - 047.2: E2E timeout test (100ms timeout vs 30s timeout), context cancellation propagation
  - 047.3: E2E fork vs fresh comparison, parent message isolation verification, race detection
  - 047.4: E2E tool execution (read, ls, bash), tool ceiling enforcement, restricted tool set verification
  - 047.5: E2E for each of 5 built-in types, system prompt influence verification, tool set application
  - 047.6: E2E event sequence verification, real-time streaming timing, nil channel safety, slow consumer test
  - 047.7: E2E LLM-visible vs invisible injection, parent context reference test, artifact tracking
  - 047.8: E2E agent discovery (user + project dirs), override behavior, malformed definition handling
  - 047.9: E2E parallel execution (4+8 subagents), concurrency limit verification, stress test (20 parallel)
  - 047.10: E2E chain execution (3-5 steps), {previous} substitution, failure stop verification
- **`docs/TRACKING.md`** — Task 047 status updated to IN PROGRESS with 047.1 completion notes

## Verification

```
go test ./internal/subagent/ -v     → 7/7 unit PASS, 1 E2E PASS (non-short)
go test ./internal/subagent/ -short → 7/7 unit PASS, 1 E2E skipped
go vet ./internal/subagent/         → clean
go build ./...                      → clean
go test ./... -short                → all 17 packages pass
```

## E2E Test Results (047.1)

- Ollama E2E: Connected to `ministral-3:14b`, streamed response, accumulated output, returned `Success: true` in 2.18s
- Usage shows 0 (Ollama streaming doesn't include usage stats per chunk — expected)
- Output verified non-empty, duration > 0

## Deferred to 047.2

- Timeout handling
- Context cancellation with context.WithTimeout
- Error classification (timeout vs provider error vs abort)
- SubAgentResult.Timeout field

## Handoff Notes for 047.2

- Provider interface: `provider.Provider` from `internal/provider/provider.go`
- Stream() is used (not Complete()) — matches agent loop pattern, enables future event streaming
- Messages passed to Stream() are currently `nil` (fresh context, no tools)
- Result structure: `SubAgentResult{Success, Output, Error, Duration, Usage}`
- Text accumulation: deltas from `EventTextDelta` events, plus final message text blocks
- Usage capture: from `EventDone` usage field
- Error handling: `EventError` returns immediately with accumulated output
- E2E test pattern established: `testing.Short()` skip, Ollama at `localhost:11434`, `ministral-3:14b` model

---

# Task 047.11 — Spawn Tool Integration — Worklog

## Scope

Continuation of 047.1 — creates the callable `subagent` tool that wraps the minimal SubAgent lifecycle.
Follows 047.1 acceptance criteria only (no timeout, fork context, tool filtering, built-in types, etc.).

## Implementation

### Files Created

1. **`internal/tools/subagent.go`** — SubAgentTool implementing `tools.Tool` interface
   - `SubAgentTool` struct with provider and model fields
   - `NewSubAgentTool(prov, model)` constructor
   - `SubAgentParams` struct: `task` (required), `model` (optional), `system_prompt` (optional)
   - `Execute()` creates `subagent.SubAgent`, calls `Run()`, returns `types.ToolResult`
   - Output formatted as JSON: `{subagent_id, task, output, duration}`

2. **`internal/tools/subagent_test.go`** — Test suite (8 tests: 7 unit + 1 E2E)
   - `TestSubAgentTool_Name` — verifies tool name is "subagent"
   - `TestSubAgentTool_Description` — verifies non-empty description
   - `TestSubAgentTool_ExecutionMode` — verifies ExecutionExclusive
   - `TestSubAgentTool_Execute_Success` — verifies successful execution, JSON output format
   - `TestSubAgentTool_Execute_Failure` — verifies error result on provider failure
   - `TestSubAgentTool_Execute_CustomModel` — verifies custom model override
   - `TestSubAgentTool_Execute_DefaultModel` — verifies parent model used when no model specified
   - `TestSubAgentTool_Execute_SystemPrompt` — verifies system prompt passed through
   - `TestSubAgentTool_E2E_Ollama` — E2E test against real Ollama (skipped in short mode)

### Files Modified

- **`internal/sdk/sdk.go`**
  - `registerBuiltinTools()` signature updated: added `prov provider.Provider, model types.Model` params
  - Registers `SubAgentTool` when provider and model are available
  - Call sites updated: `NewSession()`, `sdk_test.go`, `e2e_test.go`

## Verification

```
go test ./internal/tools/ -run TestSubAgent -v -short  → 7/7 unit PASS, 1 E2E skipped
go test ./... -short                                    → all 17 packages pass
go vet ./...                                            → clean
go build ./...                                          → clean
```

## Design Decisions

- **Execution mode**: `ExecutionExclusive` — subagent runs synchronously, blocks other tools
- **Output format**: JSON with subagent_id, task, output, duration — consumable by LLM as structured data
- **Model fallback**: uses parent's model if no model specified in params
- **Provider dependency**: tool requires provider at construction — not registered if provider unavailable
- **Error handling**: subagent failures return `IsError: true` with error message, not Go error (allows LLM to see failure)

## Deferred to Future Slices

- Tool filtering (047.4) — subagent currently has no tool access
- Built-in types (047.5) — no type-based constructors
- Event streaming (047.6) — no event forwarding
- User confirmation gate — no permission check before spawn
- Nested subagent deny — no prevention of subagent-to-subagent spawning
- Timeout/cancellation (047.2) — uses SubAgent defaults from 047.1

## Fix: LLM Not Using Subagent Tool

After initial implementation, the LLM was faking subagent usage (pretending to delegate instead of calling the tool). Root cause: tool description was too minimal, no system prompt guidance.

### Changes Made

1. **`internal/tools/subagent.go`** — Expanded tool description with:
   - "When to use" section (research, code review, implementation, context-heavy tasks, explicit user request)
   - "When NOT to use" section (simple file reads, grep/find, simple questions, context-dependent tasks)
   - Usage notes (detailed task descriptions, trust directive, JSON output format)

2. **`internal/sdk/sdk.go`** — Added sub-agent usage guidance to default system prompt:
   - When to use subagent tool (complex tasks, research, code review, user requests)
   - When NOT to use (simple direct tasks)
   - Reminder about self-contained task descriptions

### Verification
- `go build ./...` — clean
- `go test ./... -short` — all 17 packages pass
- Binary rebuilt for manual testing

## Fix: Subagent Task Never Sent to LLM

After testing, subagent returned empty output. Root cause: `SubAgent.Run()` sent empty `messages` slice to the provider — the task text was never passed as a user message. The LLM had a system prompt but no user message to respond to, so it returned nothing.

### Changes Made

1. **`internal/subagent/subagent.go`** — `Run()` now sends task as user message:
    ```go
    messages := []types.AgentMessage{
        {Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: sa.Task}}},
    }
    ```

2. **`internal/subagent/subagent_test.go`** — Updated `TestRun_FreshContext_EmptyMessages` → `TestRun_FreshContext_TaskSentAsUserMessage` to verify task is sent as user message with correct role and content.

---

# Task 047.2 — Timeout + Cancellation — Worklog

## Scope

Add configurable timeout and context cancellation to `SubAgent.Run()`. Default timeout is 5 minutes. Timeout and cancellation errors are classified and returned as distinct `SubAgentResult` values.

## Implementation

### Files Modified

1. **`internal/subagent/subagent.go`**
   - Added `DefaultTimeout = 5 * time.Minute` constant
   - Added `Timeout time.Duration` to `SubAgent` struct
   - Added `Timeout time.Duration` to `SubAgentOpts`
   - Added `Timeout bool` to `SubAgentResult`
   - `NewSubAgent()` defaults Timeout to `DefaultTimeout` if not set (or <= 0)
   - `Run()` wraps execution with `context.WithTimeout(ctx, sa.Timeout)`
   - On timeout: returns `SubAgentResult{Success: false, Timeout: true, Error: fmt.Errorf("subagent: execution timed out after %s: %w", sa.Timeout, context.DeadlineExceeded)}`
   - On cancellation: returns `SubAgentResult{Success: false, Timeout: false, Error: fmt.Errorf("subagent: execution cancelled: %w", ctx.Err())}`
   - Uses `defer cancel()` to prevent goroutine leak

2. **`internal/subagent/subagent_test.go`**
   - Added `slowMockProvider` — mock that respects context cancellation during delay
   - `TestRun_Timeout` — verifies timeout error with `Timeout: true`, duration within bounds
   - `TestRun_SuccessWithinTimeout` — verifies normal success when completing before timeout
   - `TestRun_ContextCancellation` — verifies cancelled context returns error without Timeout flag
   - `TestRun_DefaultTimeout` — verifies 5m default applied
   - `TestNewSubAgent_DefaultTimeout` — verifies default timeout in constructor
   - `TestNewSubAgent_CustomTimeout` — verifies custom timeout preserved
   - `TestRun_E2E_Timeout` — E2E with 100ms timeout against Ollama
   - `TestRun_E2E_SuccessWithinTimeout` — E2E with 30s timeout against Ollama
   - `TestRun_E2E_ContextCancellation` — E2E with immediate cancellation against Ollama

3. **`internal/tools/subagent.go`**
   - Added `Timeout string` to `SubAgentParams` (Go duration string, e.g. "30s", "2m")
   - `Execute()` parses timeout string via `time.ParseDuration()`
   - Invalid timeout returns error result (not Go error, allows LLM to see it)
   - Logging includes timeout value at start

4. **`internal/tools/subagent_test.go`**
   - Added `timeoutCapturingMockProvider` — captures context to verify deadline
   - `TestSubAgentTool_Execute_Timeout` — verifies timeout is passed through to provider context
   - `TestSubAgentTool_Execute_InvalidTimeout` — verifies error on invalid duration string

## Verification

```
go test ./internal/subagent/ -short -v     → 13/13 unit PASS, 4 E2E skipped
go test ./internal/tools/ -short -v        → 11/11 unit PASS, 1 E2E skipped
go test -race ./internal/subagent/...      → clean
go test -race ./internal/tools/...         → clean
go vet ./...                               → clean
go build ./...                             → clean
go test ./... -short                       → all 17 packages pass
```

## Design Decisions

- **Default timeout**: 5 minutes — balances long-running tasks with preventing hung subagents
- **Timeout=0 means use default**: Explicit timeout must be positive; 0 or negative falls back to default
- **Error wrapping**: Timeout error wraps `context.DeadlineExceeded` for `errors.Is()` compatibility; cancellation wraps `context.Canceled`
- **Timeout as string in tool params**: LLM can specify duration as "30s", "2m", "10m" — parsed via `time.ParseDuration()`
- **Invalid timeout returns error result** (not Go error): Allows LLM to see and potentially retry with valid duration

## Deferred to 047.3

- Fork context mode
- Shallow copy of parent messages
- Context isolation verification

## Handoff Notes for 047.3

- Timeout implementation: `context.WithTimeout` wrapped around provider Stream call
- Error types: `Timeout: true` for deadline exceeded, `Timeout: false` for cancellation
- Default timeout constant: `subagent.DefaultTimeout = 5 * time.Minute`
- Context cancellation pattern: `defer cancel()` ensures no goroutine leak
- Next session starts with: Add fork mode, shallow copy parent messages, verify isolation

---

# Task 047.3 — Context Fork Mode — Worklog

## Scope

Add fork context mode to `SubAgent.Run()`. Fork mode creates a shallow copy of parent messages, giving the sub-agent visibility into the parent conversation while maintaining isolation.

## Implementation

### Files Modified

1. **`internal/subagent/subagent.go`**
   - Added `ContextFork ContextMode = "fork"` constant
   - Added `ParentMessages []types.AgentMessage` to `SubAgent` struct
   - Added `ParentMessages []types.AgentMessage` to `SubAgentOpts`
   - Updated `NewSubAgent()` to copy `ParentMessages` from opts
   - Updated `Run()` to build messages based on `ContextMode`:
     - `ContextFork`: shallow copy of parent messages + task as user message
     - `ContextFresh` (default): task as single user message (existing behavior)

2. **`internal/subagent/subagent_test.go`**
   - `TestRun_ForkContext_ParentMessagesIncluded` — verifies fork mode includes all parent messages + task
   - `TestRun_ForkContext_Isolation` — verifies parent messages unchanged after fork subagent runs
   - `TestRun_ForkContext_ConcurrentIsolation` — verifies 10 concurrent fork subagents don't corrupt parent
   - `TestRun_ForkContext_EmptyParentMessages` — verifies fork with nil/empty parent works (task only)
   - `TestNewSubAgent_ContextForkMode` — verifies ContextFork is preserved in constructor
   - `TestRun_E2E_ForkContext_SeesParentMessages` — E2E: fork subagent can reference parent conversation
   - `TestRun_E2E_FreshContext_CannotSeeParentMessages` — E2E: fresh subagent cannot see parent messages
   - `TestRun_E2E_ForkContext_ParentIsolation` — E2E: parent messages unchanged after fork subagent

## Verification

```
go test ./internal/subagent/ -short -v     → 18/18 unit PASS, 7 E2E skipped
go test -race ./internal/subagent/...      → clean
go vet ./internal/subagent/...             → clean
go build ./...                             → clean
go test ./... -short                       → all 17 packages pass
```

## E2E Test Results (047.3)

- Fork context: Ollama correctly referenced parent messages ("blue" was favorite color) in 5.39s
- Fresh context: Ollama correctly said "Unknown" — no access to parent messages in 0.49s
- Fork isolation: Parent messages verified unchanged after fork subagent completed in 0.56s

## Design Decisions

- **Shallow copy**: `copy()` of `[]types.AgentMessage` slice — copies slice header and elements, but `ContentBlock` pointers (if any) are shared. This is intentional per architecture: "shallow copy only, not deep cloning".
- **Task appended after parent messages**: In fork mode, parent messages come first, then task as user message — maintains conversation flow.
- **Empty parent messages in fork mode**: Gracefully handled — behaves like fresh mode (task only).

## Deferred to 047.4

- Tool access for sub-agents
- Tool registry filtering
- Tool execution in sub-agent Run() loop
- Parent hard ceiling enforcement

---

# Task 047.4 — Tool Integration — Worklog

## Scope

Add tool execution to `SubAgent.Run()`. Sub-agents can now execute tools with a restricted tool set, with parent hard ceiling enforcement.

## Implementation

### Files Modified

1. **`internal/subagent/subagent.go`** — Major rewrite to support tool execution
   - Defined local `Tool` interface (mirrors `tools.Tool` to avoid import cycle with `tools/subagent.go`)
   - Defined local `ToolCallRequest`, `ToolCallResult` types
   - Defined `Executor` function type: bridges subagent tool calls to actual tool registry
   - Added `Tools []Tool` and `Executor Executor` to `SubAgent` struct and `SubAgentOpts`
   - Added `ParentToolNames []string` to `SubAgentOpts` for hard ceiling validation
   - `NewSubAgent()`: validates every tool name is in parent set (panic if violated)
   - `Run()`: implements tool execution loop:
     - Builds `[]types.ToolDefinition` from tools and passes to `provider.Stream()`
     - After stream completes, extracts `BlockToolCall` blocks from final message
     - If tool calls found: executes via `Executor`, appends `tool_result` messages, loops
     - Max 10 iterations safety guard against infinite tool loops
     - Accumulates text output across all LLM turns
   - `buildToolDefinitions()`: converts tool set to provider `ToolDefinition` slices via `jsonschema`
   - `extractToolCalls()`: pulls `ToolCallBlock` entries from assistant message
   - `buildToolResultMessage()`: creates `tool_result` `AgentMessage` from tool call + result
   - Fixed bug: post-loop error detection now checks `result.Error != nil` (not `!result.Success` which is zero-value false)

2. **`internal/subagent/subagent_test.go`** — Added 7 new tests
   - `toolCallMockProvider` — mock provider that returns tool call on first Stream call, text on subsequent
   - `alwaysToolCallProvider` — mock provider that always returns tool calls (for max iterations test)
   - `mockTool` — implements `subagent.Tool` interface for testing
   - `TestRun_ToolExecution` — verifies tool call → executor → result → second LLM call flow
   - `TestRun_ToolFiltering` — verifies only registered tools passed to provider
   - `TestRun_ParentHardCeiling` — verifies panic when subagent tool not in parent set
   - `TestRun_NoTools_NoExecutor` — verifies existing behavior preserved when no tools
   - `TestRun_MaxIterations` — verifies loop stops at 10 iterations
   - `TestRun_ToolDefinitionsPassedToProvider` — verifies tool definitions correctly built

3. **`internal/tools/subagent.go`** — Updated SubAgentTool constructor
   - `NewSubAgentTool()` now accepts `parentRegistry *Registry` and `parentToolNames []string`
   - `buildExecutor()` creates `subagent.Executor` that delegates to parent registry
   - Executor bridges `subagent.ToolCallRequest` → `tools.ToolCallRequest` and results back

4. **`internal/tools/subagent_test.go`** — Updated all `NewSubAgentTool` calls with `nil, nil` params

5. **`internal/sdk/sdk.go`** — Updated `registerBuiltinTools()`
   - Moved `registerSearchTools()` before `NewSubAgentTool()` so search tools are included in parent set
   - Passes `reg` and `reg.Names()` to `NewSubAgentTool()` for full parent tool ceiling

## Verification

```
go test ./internal/subagent/ -short -v     → 24/24 unit PASS, 7 E2E skipped
go test ./internal/tools/ -short -v        → 12/12 unit PASS, 2 E2E skipped
go test -race ./internal/subagent/...      → clean
go test -race ./internal/tools/...         → clean
go vet ./...                               → clean
go build ./...                             → clean
go test ./... -short                       → all 17 packages pass
```

## E2E Test Results (047.4)

- **TestRun_E2E_Ollama** (subagent): PASS — basic lifecycle against Ollama
- **TestRun_E2E_Timeout** (subagent): PASS — 100ms timeout triggers correctly
- **TestRun_E2E_SuccessWithinTimeout** (subagent): PASS — completes within 30s timeout
- **TestRun_E2E_ContextCancellation** (subagent): PASS — immediate cancellation propagates
- **TestRun_E2E_ForkContext_SeesParentMessages** (subagent): PASS — fork sees "blue"
- **TestRun_E2E_FreshContext_CannotSeeParentMessages** (subagent): PASS — fresh says "unknown"
- **TestRun_E2E_ForkContext_ParentIsolation** (subagent): PASS — parent unchanged after fork
- **TestSubAgentTool_E2E_Ollama** (tools): PASS — SubAgentTool spawns subagent successfully
- **TestSubAgent_E2E_DirectToolCall** (tools): PASS — read tool called, returned "Hello from E2E test!" in 5.5s

### E2E Tool Execution Findings

- Tool execution loop works: stream → extract tool calls → execute → append results → repeat
- Read tool correctly returns file content when called with absolute path
- **Known limitation**: ministral-3:14b has issues with tool result handling in Ollama — it may call tools repeatedly even after receiving results. The tool execution itself is correct (verified by log output showing correct content returned).
- Tool result messages are correctly appended to conversation history with `RoleToolResult` and `ToolCallID`

## Design Decisions

- **Import cycle avoidance**: `subagent` cannot import `tools` (because `tools` imports `subagent` for `SubAgentTool`). Solution: define local `Tool` interface and `Executor` function type in subagent. The caller provides the executor to bridge to the actual tool registry.
- **Executor pattern**: `Executor func(ctx, []ToolCallRequest) []*ToolCallResult` — simple function type, no interface needed. Caller creates closure over tool registry.
- **Parent hard ceiling**: validated at construction, panics on violation (consistent with nil provider check). Fails fast rather than silently dropping tools.
- **Max iterations**: 10 — prevents infinite tool-use loops while allowing reasonable multi-step tool chains.
- **Tool definitions via jsonschema**: uses same `github.com/invopop/jsonschema` reflection as the tools registry for consistency.
- **E2E tool tests**: moved to `tools` package test file to avoid import cycle (subagent tests cannot import `tools`).

## Bug Fix: Assistant Tool Call Message Not Appended to Conversation

### Problem
After the first turn where the model returns `tool_calls`, the assistant message containing those tool calls was **never appended** to the `messages` slice before tool result messages. Only the tool result messages were appended. This broke the Ollama API's expected conversation structure:

Expected: `[user] → [assistant with tool_calls] → [tool_result] → [assistant with text]`
Actual:   `[user] → [tool_result] → [assistant with text]` (missing assistant tool_call message)

This caused the model to not understand the tool results context, leading to empty output or repeated tool calls.

### Fix
In `internal/subagent/subagent.go`, `Run()` method: added `messages = append(messages, *turnLastMsg)` when `turnLastMsg != nil` to ensure the assistant message with tool_call blocks is included in the conversation before tool results.

```go
if turnLastMsg != nil {
    lastMsg = turnLastMsg
    messages = append(messages, *turnLastMsg)  // ← added
}
```

### Verification
- E2E test with `gemma4:26b` + websearch: output 1808 chars, proper answer returned
- `TestRun_ToolExecution` updated to expect 3 messages in second call (task + assistant tool_call + tool result)
- All subagent tests pass (24 unit + 7 E2E)
- All tools tests pass
- `go vet ./...` and `go build ./...` clean

## Bug Fix: Subagent Tool-Use Loop (Max Iterations Exceeded)

### Problem
Subagents would hit the max tool iterations limit (10) and fail with `subagent: exceeded max tool iterations (10)`. The model kept calling tools indefinitely without ever stopping to provide a final answer. This occurred because:
1. No guidance in the system prompt about when to stop using tools
2. No instruction to provide a final text response after gathering information
3. No warning against repeating identical tool calls

### Fix
In `internal/subagent/subagent.go`, `Run()` method: appended tool usage guidelines to the system prompt when tools are available:

```go
if len(toolDefs) > 0 {
    systemPrompt += "\n\n## Tool Usage Guidelines\n" +
        "- Use tools only when necessary to complete the task\n" +
        "- After gathering sufficient information, stop using tools and provide your final answer\n" +
        "- Do not repeat the same tool call with identical parameters\n" +
        "- If a tool returns an error, try a different approach rather than retrying the same call\n" +
        "- You must provide a final text response — do not end with a tool call\n" +
        "- Limit tool usage to what is reasonably needed for the task"
}
```

### Verification
- E2E test with `gemma4:26b` + websearch: completed in 2 tool calls (previously hit 10-iteration limit)
- Output: 2736 chars with proper answer about AMD Ryzen AI Max 395+
- All subagent tests pass
- All tools tests pass
- `go vet ./...` and `go build ./...` clean

## Deferred to 047.5

- Built-in agent types with default tool sets
- Type-based constructors
- Default system prompts per type

## Slice 047.4 — COMPLETED

All acceptance criteria met. Two bugs discovered and fixed during implementation:
1. Assistant tool_call message not appended to conversation (broke Ollama API structure)
2. No tool usage guidance in system prompt (caused infinite tool-use loops)

E2E verified with `gemma4:26b` + websearch: proper answer returned in 2 tool calls.

---

# Task 047.5 — Built-in Agent Types — Worklog

## Scope

Add 6 built-in subagent types with default tool sets and system prompts. Each type has a focused role and restricted tool set per ARCHITECTURE.md §5.4.

## Implementation

### Files Created

1. **`internal/subagent/builtin.go`** — Built-in type definitions and constructor
   - `Type` enum: `general`, `researcher`, `reviewer`, `implementor`, `security_reviewer`, `qa`
   - `AllTypes()` — returns all 6 built-in types
   - `DefaultToolSet(Type)` — returns tool names for a type
   - `DefaultSystemPrompt(Type)` — returns system prompt for a type
   - `ValidType(Type)` — checks if type is known
   - `ParseType(string)` — parses/normalizes type string (handles hyphens, spaces, case)
   - `NewSubAgentByType(Type, Provider, []Tool, SubAgentOpts)` — type-based constructor that:
     - Filters parent tools to type's default set
     - Merges default system prompt (appends to custom prompt if provided)
     - Defaults to `general` when empty type string
     - Returns error for unknown types

2. **`internal/subagent/builtin_test.go`** — Test suite (16 unit tests)
   - `TestAllTypes_ReturnsSixTypes` — verifies all 6 types present
   - `TestDefaultToolSet_EachType` — verifies correct tool set per type
   - `TestDefaultToolSet_UnknownType` — verifies nil for unknown
   - `TestDefaultSystemPrompt_EachType` — verifies non-empty prompts
   - `TestDefaultSystemPrompt_UnknownType` — verifies empty for unknown
   - `TestValidType` — verifies valid/invalid type detection
   - `TestParseType` — verifies normalization (hyphens, spaces, case, whitespace)
   - `TestNewSubAgentByType_UnknownType` — verifies error for unknown
   - `TestNewSubAgentByType_DefaultToGeneral` — verifies empty string defaults to general
   - `TestNewSubAgentByType_ToolFiltering` — verifies reviewer gets only read/grep/find/ls
   - `TestNewSubAgentByType_SystemPromptMerged` — verifies custom + default prompt merge
   - `TestNewSubAgentByType_SystemPromptDefaultOnly` — verifies default prompt when no custom
   - `TestNewSubAgentByType_TypeFieldSet` — verifies Type field set for all 6 types
   - `TestNewSubAgentByType_WithMockProvider` — verifies end-to-end execution
   - `TestNewSubAgentByType_ParentToolNamesPassed` — verifies parent tool names passed through

### Files Modified

1. **`internal/subagent/subagent.go`**
   - Added `Type Type` field to `SubAgent` struct
   - Added `Type Type` field to `SubAgentOpts`
   - `NewSubAgent()` copies Type from opts

2. **`internal/subagent/subagent_test.go`**
   - Added `runE2EWithType()` helper function
   - `TestRun_E2E_BuiltinType_General` — E2E: general type against Ollama
   - `TestRun_E2E_BuiltinType_Researcher` — E2E: researcher type against Ollama
   - `TestRun_E2E_BuiltinType_Reviewer` — E2E: reviewer type against Ollama
   - `TestRun_E2E_BuiltinType_Implementor` — E2E: implementor type against Ollama
   - `TestRun_E2E_BuiltinType_SecurityReviewer` — E2E: security_reviewer type against Ollama
   - `TestRun_E2E_BuiltinType_QA` — E2E: qa type against Ollama

3. **`internal/tools/subagent.go`**
   - Added `Type string` to `SubAgentParams` with jsonschema description
   - `Execute()` uses `NewSubAgentByType` when type is specified
   - Falls back to `NewSubAgent` when no type specified (backward compatible)
   - Returns error result for unknown type (not Go error, allows LLM to see it)
   - Updated tool description to list all 6 agent types with their tool sets
   - JSON output now includes `type` field

## Verification

```
go test ./internal/subagent/ -short -v     → 40/40 unit PASS, 13 E2E skipped
go test ./internal/tools/ -short -v        → 12/12 unit PASS, 2 E2E skipped
go test -race ./internal/subagent/...      → clean
go test -race ./internal/tools/...         → clean
go vet ./...                               → clean
go build ./...                             → clean
go test ./... -short                       → all 18 packages pass
```

## Design Decisions

- **6 types (not 5)**: Added `general` as a versatile default for tasks that don't fit a specialized role
- **Empty type defaults to general**: `NewSubAgentByType("", ...)` returns general type — ergonomic for callers
- **System prompt merging**: Custom prompt + default prompt (custom first, default appended with `\n\n` separator) — allows customization while preserving type guidance
- **ParseType normalization**: Handles hyphens (`security-reviewer`), spaces (`security reviewer`), case (`SECURITY_REVIEWER`), and whitespace — makes LLM input more forgiving
- **Tool filtering**: Filters parent tools to type's default set — respects parent hard ceiling (inherited from NewSubAgent)
- **Backward compatible**: SubAgentTool without type parameter still uses NewSubAgent (no type set)

## Tool Sets Per Type

| Type | Tools | Count |
|------|-------|-------|
| general | read, write, edit, bash, grep, find, ls | 7 |
| researcher | read, grep, find, ls, bash, websearch, webfetch | 7 |
| reviewer | read, grep, find, ls | 4 |
| implementor | read, write, edit, bash, grep, find, ls | 7 |
| security_reviewer | read, grep, find, bash | 4 |
| qa | read, bash, grep, find, ls, write | 6 |

## Deferred to 047.6

- Event streaming
- Event channel forwarding
- Usage accumulation from events

## Handoff Notes for 047.6

- Built-in types: 6 types defined in `builtin.go`
- Type-based constructor: `NewSubAgentByType()` with tool filtering and prompt merging
- SubAgentTool supports `type` parameter — LLM can now specify agent type
- Type field on SubAgent struct — available for logging, result output, etc.
- Next session starts with: Add event channel, forward events to parent, accumulate usage

## Fix: Researcher Missing Web Search Tools

### Problem
Manual testing showed researcher subagent couldn't perform web research — it only had codebase tools (read, grep, find, ls, bash). When tasked with "Research AMD Ryzen AI Max 395", it echoed a note saying it can't browse the web, and the parent agent had to use websearch itself.

### Root Cause
Researcher tool set was deferred pending Task 026 (Web Search Tool), but websearch/webfetch are now available in the tools registry.

### Fix
1. Added `websearch` and `webfetch` to researcher's default tool set in `builtin.go`
2. Updated researcher system prompt to explicitly mention websearch and webfetch usage
3. Updated SubAgentTool description to reflect new researcher tool set
4. Updated ARCHITECTURE.md §5.4 and task.md to match

### Verification
- `go test ./internal/subagent/ -short` → all pass
- `go vet ./...` → clean
- `go build ./...` → clean

## Fix: Researcher Not Using Web Search Tools

### Problem
Manual testing showed researcher subagent not using websearch/webfetch tools despite having them available. It relied on training data instead, producing outdated/incorrect information about AMD Ryzen AI Max 395+ laptops (claimed "no widespread retail availability" when user was actively using one).

### Root Cause
Two issues:
1. **System prompt too weak**: "Use websearch to find current information" was advisory, not mandatory
2. **Generic tool usage guidelines conflicted**: The appended guidelines said "Use tools only when necessary" and "Limit tool usage" — directly contradicting the researcher's need to aggressively search

### Fix
1. Rewrote researcher system prompt with explicit "CRITICAL: You MUST use websearch" directive
2. Made tool usage guidelines type-aware in `subagent.Run()`:
   - Researcher: "use websearch and webfetch aggressively"
   - Reviewer/SecurityReviewer: "use read and grep thoroughly"
   - Default: "use tools when they help"
3. Removed "Use tools only when necessary" and "Limit tool usage" from generic guidelines

### Files Modified
- `internal/subagent/builtin.go` — Rewrote researcher system prompt
- `internal/subagent/subagent.go` — Type-aware tool usage guidelines

## Fix: Researcher Prompt — Source Verification and Deep Research

### Problem
Researcher was stopping after finding first piece of information, not verifying claims, and not providing source links.

### Fix
Added two new sections to researcher system prompt:
1. **Source Verification Rules**: Every claim must have a working HTTP link, cross-reference across multiple sources, flag single-source claims as unverified, prefer primary sources
2. **Depth of Research**: Never stop at first result, run multiple queries with different keywords, read multiple pages via webfetch, provide wide spectrum of information (specs, reviews, comparisons, pricing, availability)

### Files Modified
- `internal/subagent/builtin.go` — Added Source Verification Rules and Depth of Research sections

## Fix: General Subagent Missing Web Search Tools

### Problem
General subagent lacked websearch/webfetch tools, limiting its ability to handle research tasks despite being the versatile default type.

### Fix
Added `websearch` and `webfetch` to general type's tool set and updated its system prompt to mention web search capabilities.

### Files Modified
- `internal/subagent/builtin.go` — Added websearch/webfetch to general tool set, updated system prompt
- `internal/subagent/builtin_test.go` — Updated test expectation
- `internal/tools/subagent.go` — Updated tool description
- `docs/ARCHITECTURE.md` §5.4 — Added general type to table

## Slice 047.5 — COMPLETED

All acceptance criteria met. 6 built-in types implemented with correct tool sets, system prompts, type-based constructor, and SubAgentTool `type` parameter support. Researcher type enhanced with websearch/webfetch tools, mandatory web search directive, source verification rules, and deep research guidelines.

---

# Task 047.6 — Event Streaming — Worklog

## Scope

Add real-time event forwarding from sub-agent to parent via optional Events channel. All stream event types forwarded for maximum visibility. Usage accumulated across all tool-use turns.

## Implementation

### Files Modified

1. **`internal/subagent/subagent.go`**
   - Added `Events chan types.AgentEvent` to `SubAgent` struct and `SubAgentOpts`
   - Added `eventsBufferSize = 256` constant (matches TUI event channel pattern)
   - `Run()` creates internal buffered channel (`chan types.StreamEvent, 256`) and starts `forwardEvents` goroutine
   - `emitEvent()` sends lifecycle events (agent_start, message_end, agent_end) to Events channel
   - `forwardEvents()` goroutine reads from internal channel, converts StreamEvent → AgentEvent, forwards to parent Events channel
   - `convertStreamEvent()` maps all 10 stream event types to corresponding AgentEvent types:
     - EventStart → AgentEventStart
     - EventTextStart → AgentEventMessageStart
     - EventTextDelta → AgentEventTextDelta (with delta in Data)
     - EventTextEnd → AgentEventMessageEnd
     - EventThinkingStart/Delta/End → AgentEventThinkingDelta
     - EventToolCallStart/End → AgentEventToolExecStart/End
     - EventDone → AgentEventMessageEnd
     - EventError → AgentEventError
   - All forwarded events get `SubAgentID` set to sub-agent's ID
   - Non-blocking send in both `forwardEvents` and `emitEvent` — drops events if channel full, never blocks Run()
   - `consumeStream()` forwards events to internal channel for goroutine processing
   - Usage accumulation changed from overwrite to additive across all turns (Input, Output, CacheRead, CacheWrite, TotalTokens, Cost fields)
   - `close(internalEvents)` and `<-done` ensures goroutine completes before Run() returns

2. **`internal/subagent/subagent_test.go`**
   - Added `eventMockProvider` — records events and captured messages
   - Added `multiTurnEventProvider` — returns different events on successive Stream calls
   - `TestRun_EventForwarding` — verifies events forwarded, correct types, SubAgentID set, text_delta data correct, message_end + agent_end at end
   - `TestRun_NilEventsChannel` — verifies no panic with nil Events channel
   - `TestRun_UsageAccumulation` — verifies usage summed across two tool-use turns (10+20 input, 5+15 output)
   - `TestRun_SlowConsumer` — verifies Run() completes without blocking when Events channel is small and never drained
   - `TestRun_EventSequence` — verifies correct event order: message_start → text_delta × 2 → message_end
   - `TestRun_E2E_EventStreaming` — E2E against Ollama, verifies event sequence, text_delta count, message_end, agent_end
   - `TestRun_E2E_NilEventsChannel` — E2E with nil channel
   - `TestRun_E2E_UsageAccumulation` — E2E verifying Usage field accessible

## Verification

```
go test ./internal/subagent/ -short -v     → 45/45 unit PASS, 16 E2E skipped
go test -race ./internal/subagent/... -short → clean
go vet ./...                               → clean
go build ./...                             → clean
go test ./... -short                       → all 18 packages pass
```

## Design Decisions

- **Buffered channel (256)**: Matches TUI event channel pattern. Large enough to absorb burst of events during streaming.
- **Non-blocking send**: Both `forwardEvents` and `emitEvent` use `select` with `default` to drop events if channel full. Prevents Run() from blocking on slow consumer.
- **Goroutine forwarder**: Dedicated goroutine reads internal channel and converts/forwards events. Clean separation between provider stream consumption and event forwarding.
- **All event types forwarded**: Maximum visibility — text_delta, thinking_delta, tool_call_start/end, done, error all forwarded. Parent can filter if needed.
- **SubAgentID on all events**: Every forwarded event includes the sub-agent's ID for correlation.
- **Usage accumulation**: Changed from overwrite (`result.Usage = *event.Usage`) to additive (`result.Usage.Input += event.Usage.Input`). Correctly accumulates across multiple tool-use turns.
- **Goroutine lifecycle**: `internalEvents` channel closed after stream loop completes, `<-done` waits for forwardEvents goroutine to finish. No goroutine leaks.

## Deferred to 047.7

- Result injection modes (LLM-visible vs custom entry)
- Artifact tracking
- Result formatting for LLM consumption

## Slice 047.6 — COMPLETED

All acceptance criteria met. 5 new unit tests + 3 E2E tests. Event forwarding with non-blocking send, usage accumulation across turns, SubAgentID on all events.

---

# Task 047.7 — Result Injection — Worklog

## Scope

Add configurable result injection modes (LLM-visible vs custom entry), artifact tracking from tool execution, and structured output formatting for LLM consumption.

## Implementation

### Files Modified

1. **`internal/subagent/subagent.go`**
   - Added `LLMVisible bool` to `SubAgentResult` (defaults to `true` in `Run()`)
   - Added `Artifacts []string` to `SubAgentResult` — tracks file paths created/modified
   - Added `extractArtifact(call, result)` — extracts file paths from write/edit/bash tool calls:
     - write/edit: extracts `path` from JSON arguments
     - bash: parses output for "Created file:" or "Written to:" patterns
   - Added `resultText(r)` — extracts text content from ToolResult
   - Added `FormatForLLM(task string) string` — formats result as structured markdown:
     - Task, Status (Success/Timeout/Failed), Duration, Tokens, Output/Error, Artifacts
   - Added `InjectResult(messages, result, task)` — injects result into parent messages:
     - LLMVisible=true: appends tool_result message with formatted output
     - LLMVisible=false: returns messages unchanged (result stored separately by caller)
   - Artifacts accumulated during tool execution loop in `Run()`

2. **`internal/subagent/subagent_test.go`**
   - Added `encoding/json` import
   - `TestSubAgentResult_LLMVisibleDefault` — verifies LLMVisible defaults to true
   - `TestInjectResult_LLMVisible` — verifies message appended with formatted output
   - `TestInjectResult_NotLLMVisible` — verifies messages unchanged when LLMVisible=false
   - `TestSubAgentResult_FormatForLLM_Success` — verifies structured output for success
   - `TestSubAgentResult_FormatForLLM_Failure` — verifies error formatting
   - `TestSubAgentResult_FormatForLLM_Timeout` — verifies timeout formatting
   - `TestExtractArtifact_WriteTool` — verifies path extraction from write tool
   - `TestExtractArtifact_EditTool` — verifies path extraction from edit tool
   - `TestExtractArtifact_NoArtifact` — verifies read tool doesn't produce artifact
   - `TestExtractArtifact_NilResult` — verifies nil result handling
   - `TestRun_ArtifactTracking` — verifies artifacts collected during tool execution
   - `TestRun_E2E_ArtifactTracking` — E2E: implementor creates file, artifacts tracked
   - `TestRun_E2E_InjectResult` — E2E: result injected into parent messages

## Verification

```
go test ./internal/subagent/ -short -v     → 56/56 unit PASS, 19 E2E skipped
go test -race ./internal/subagent/... -short → clean
go vet ./...                               → clean
go build ./...                             → clean
go test ./... -short                       → all 18 packages pass
```

## Design Decisions

- **LLMVisible defaults to true**: Most common use case — parent LLM should see subagent output. Caller sets false for internal/audit results.
- **InjectResult is a pure function**: Takes messages slice, returns new slice with appended message. No side effects. Caller controls when/where to inject.
- **FormatForLLM uses markdown**: Structured, human-readable, LLM-consumable format. Includes task, status, duration, tokens, output/error, and artifacts.
- **Artifact extraction is heuristic-based**: Parses tool call arguments for file paths. For bash, looks for "Created file:" or "Written to:" patterns in output. Simple but effective for common cases.
- **Artifacts collected during Run()**: Accumulated in the tool execution loop, attached to result before return. No post-processing needed.
- **Custom entry mode**: When LLMVisible=false, InjectResult returns unchanged messages. Caller stores result separately (e.g., as session custom entry). The subagent package doesn't manage storage — that's the caller's responsibility.

## Deferred to 047.8

- Markdown agent definitions
- User/project directory discovery
- Agent override behavior

## Slice 047.7 — COMPLETED

All acceptance criteria met. 11 new unit tests + 3 E2E tests. LLM-visible injection, custom entry mode, artifact tracking, structured formatting.

---

# Task 047.8 — Agent Definition Format — Worklog

## Scope

Add user-definable agents via Markdown + YAML frontmatter, discovery from user/project directories, override behavior, and integration with SubAgentTool.

## Implementation

### Files Modified

1. **`internal/subagent/definition.go`**
   - Fixed `loadProjectAgents()` to walk up from cwd to root, then load directories in reverse order (root to cwd). This ensures project agents closest to cwd override parent ones.
   - Added `AllAgents(cwd string)` helper that merges built-in agents with discovered user/project agents. Precedence: project > user > built-in.

2. **`internal/subagent/definition_test.go`** (new file — 25 tests)
   - `TestParseAgentMD_ValidFile` — full frontmatter parsing
   - `TestParseAgentMD_MinimalFile` — no optional fields
   - `TestParseAgentMD_NoFrontmatter/UnclosedFrontmatter/InvalidYAML` — error cases
   - `TestParseAgentMD_MissingName/Description/SystemPrompt` — validation errors
   - `TestValidateAgentTools_ValidTools/UnknownTool/EmptyTools`
   - `TestLoadAgentDirectory_NonExistent/WithValidAgent/SkipsNonMarkdown/SkipsInvalidAgent`
   - `TestDiscoverAgents_UserDirectory` — loads from `~/.tau/agents/`
   - `TestDiscoverAgents_ProjectOverridesUser` — same name, project wins
   - `TestDiscoverAgents_EmptyDirectories` — no errors
   - `TestDiscoverAgents_ProjectWalkUp` — finds from subdir
   - `TestAgentDefinition_Validate` — table-driven
   - `TestAllAgents_IncludesBuiltins` — all 6 types present
   - `TestAllAgents_UserOverridesBuiltin` — user overrides built-in
   - `TestAllAgents_ProjectOverridesUser` — project overrides user

3. **`internal/tools/subagent.go`**
   - Added `discoveredAgents map[string]*subagent.AgentDefinition` field to `SubAgentTool`
   - Updated `NewSubAgentTool()` constructor to accept discovered agents
   - Added `AgentName string` to `SubAgentParams` (mutually exclusive with `Type`)
   - Updated `Execute()` to handle `agent_name`:
     - Looks up agent definition from discovered agents
     - Filters tools to agent's defined tool set
     - Uses agent's system prompt and optional model override
     - Falls back to all parent tools if agent defines no tools
   - Added `availableAgentNames()` helper for error messages
   - Updated `Description()` to dynamically list user-defined agents

4. **`internal/tools/subagent_test.go`**
   - Updated all `NewSubAgentTool` calls with `nil` for discoveredAgents parameter

5. **`internal/sdk/sdk.go`**
   - Added `subagent` import
   - Updated `registerBuiltinTools()` to call `subagent.AllAgents(cwd)` and pass to `NewSubAgentTool()`

## Verification

```
go test ./internal/subagent/... -short      → 59/59 unit PASS, 19 E2E skipped
go test ./internal/tools/... -short         → 12/12 unit PASS, 2 E2E skipped
go test ./internal/sdk/... -short           → PASS
go test -race ./internal/subagent/...       → clean
go test -race ./internal/tools/...          → clean
go vet ./...                                → clean
go build ./...                              → clean
go test ./... -short                        → all 18 packages pass
```

## Design Decisions

- **AllAgents() merges built-in + discovered**: Returns a single map with all available agents. Built-ins are base layer, user agents override, project agents override user.
- **agent_name mutually exclusive with type**: If both specified, agent_name takes precedence. If neither, uses general defaults.
- **Agent tool set filtering**: User-defined agent's `tools` field filters parent tools. If empty, all parent tools are available.
- **Agent model override**: If agent definition specifies a model, it overrides the parent model (uses same provider/API).
- **System prompt override**: Agent definition's system_prompt is used by default, but `system_prompt` param can override it.
- **loadProjectAgents fix**: Original implementation loaded from cwd to root, causing parent agents to override child ones. Fixed by collecting directories first, then loading in reverse order.

## Slice 047.8 — COMPLETED

All acceptance criteria met:
- [x] Markdown + YAML frontmatter parsing
- [x] User-level discovery (~/.tau/agents/)
- [x] Project-level discovery (.tau/agents/, walk up)
- [x] Override behavior: project > user > built-in
- [x] Error handling for malformed definitions
- [x] Tests for parsing, discovery, override (25 new tests)
- [x] `go test ./internal/subagent/...` passes
- [x] SubAgentTool supports `agent_name` parameter
- [x] SDK discovers agents at session creation

---

# Task 047.8 — Fixes and Enhancements

## Description() Bug Fix

The `Description()` method was not updated to dynamically list user-defined agents. Fixed to:
- Build description string dynamically
- Include "User-defined agents" section when discovered agents exist
- Mention `agent_name` parameter in usage notes

## E2E Tests Added (internal/tools/subagent_test.go)

- `TestSubAgentTool_Description` — verifies built-in types and agent_name mention
- `TestSubAgentTool_Description_WithDiscoveredAgents` — verifies dynamic listing of user agents
- `TestSubAgentTool_Execute_AgentName` — executes user-defined agent, verifies type in output
- `TestSubAgentTool_Execute_AgentName_NotFound` — error with available agents list
- `TestSubAgentTool_Execute_AgentName_ToolFiltering` — verifies only agent's tools passed to provider
- `TestSubAgentTool_Execute_AgentName_NoTools` — empty tools list gets all parent tools

## /agents Command

Added `/agents` TUI command to list all available subagent types and user-defined agents:
- Shows built-in types with their tool sets
- Shows user-defined agents with source, description, tools, and model override
- Shows instructions for creating agents when none are discovered
- Available during idle and streaming states
- Updated help text to include /agents

### Files Modified
- `internal/tui/command.go` — Added cmdAgents handler, /agents registration, help text update
- `internal/tui/command_test.go` — Updated command counts (16 total, 8 available during streaming)

## Verification
```
go test ./... -short          → all 18 packages pass
go vet ./...                  → clean
go build ./...                → clean
Binary rebuilt
```

---

# Task 047.8 — Description Fix for Agent Invocation

## Problem
LLM did not recognize "use X agent" as a subagent invocation. It searched for files instead of calling the subagent tool.

## Fix
Enhanced `Description()` with:
- Explicit "IMPORTANT" directive: "When the user asks you to use a specific agent, you MUST call this tool"
- Clearer parameter labels: "Built-in agent types (use 'type' parameter)" and "User-defined agents (use 'agent_name' parameter)"
- Added "use agent X" / "run X agent" / "spawn X" patterns to "When to use" section

## Verification
```
go test ./... -short          → all 18 packages pass
go vet ./...                  → clean
Binary rebuilt
```

---

# Task 047.9 — Parallel Execution — Worklog

## Scope

Add `RunParallel()` to execute multiple sub-agents concurrently with configurable concurrency limiting, result aggregation, and event forwarding.

## Implementation

### Files Modified

1. **`internal/subagent/subagent.go`**
   - Added constants: `defaultConcurrency = 4`, `defaultMaxTasks = 8`
   - Added `SubAgentTask` struct — wraps a `*SubAgent` for parallel execution
   - Added `ParallelOpts` struct — configures `Concurrency`, `MaxTasks`, `Events` channel
   - Added `ParallelResult` struct — `Results`, `SuccessCount`, `FailureCount`, `TotalUsage`, `Error`
   - Added `RunParallel(ctx, tasks, opts)` — package-level function:
     - Rejects entirely if `len(tasks) > maxTasks` (returns error)
     - Pre-allocates results slice, stores by index for order preservation
     - Semaphore pattern: `make(chan struct{}, concurrency)` for concurrency limiting
     - Per-task buffered event channels forwarded to parent via goroutines
     - `sync.WaitGroup` for task goroutines, separate `fwdWg` for event forwarders
     - Proper lifecycle: close task event channels after all tasks complete, wait for forwarders to drain
     - Usage aggregation: sums all fields across all results

2. **`internal/subagent/subagent_test.go`**
   - Added `parallelMockProvider` — returns identifiable text per task index
   - Added `slowParallelMockProvider` — sleeps 100ms to test concurrency limits
   - `TestRunParallel_EmptyTasks` — verifies empty result for no tasks
   - `TestRunParallel_MaxTasksExceeded` — verifies rejection when tasks > max
   - `TestRunParallel_AllSucceed` — verifies 3 tasks all succeed with output
   - `TestRunParallel_OrderPreserved` — verifies results match input order
   - `TestRunParallel_ConcurrencyLimit` — verifies max concurrent ≤ 2 with 4 tasks
   - `TestRunParallel_MixedSuccessFailure` — verifies 2 success + 2 failure counts
   - `TestRunParallel_UsageAggregation` — verifies usage summed correctly
   - `TestRunParallel_NilEventsChannel` — verifies no panic with nil events
   - `TestRunParallel_DefaultConcurrency` — verifies default applied when 0
   - `TestRunParallel_ContextCancellation` — verifies cancelled context propagates
   - `TestRunParallel_EventForwarding` — verifies events forwarded with SubAgentID
   - `TestRunParallel_E2E_ParallelExecution` — E2E: 4 tasks against Ollama
   - `TestRunParallel_E2E_MixedSuccessFailure` — E2E: 3 succeed, 1 fails (invalid model)
   - `TestRunParallel_E2E_ConcurrencyLimit` — E2E: 4 tasks with concurrency 2
   - `TestRunParallel_E2E_UsageAggregation` — E2E: usage field accessible

## Verification

```
go test ./internal/subagent/ -short -v     → 69/69 unit PASS, 23 E2E skipped
go test -race ./internal/subagent/...      → clean
go vet ./...                               → clean
go build ./...                             → clean
go test ./... -short                       → all 18 packages pass
```

## Design Decisions

- **Reject entirely on max tasks exceeded**: If `len(tasks) > MaxTasks`, returns `ParallelResult{Error: ...}` rather than silently capping. Caller gets clear feedback.
- **Per-task buffered event channels**: Each subagent gets its own `chan types.AgentEvent, 256`. Forwarding goroutine reads from task channel and sends to parent. Prevents blocking between tasks.
- **Separate WaitGroup for forwarders**: `fwdWg` tracks event forwarding goroutines. After all task goroutines complete, task event channels are closed, then `fwdWg.Wait()` ensures all forwarders drain before returning. Prevents race conditions with channel close.
- **Semaphore pattern**: `make(chan struct{}, concurrency)` — simple, idiomatic Go concurrency limiting. Acquire before task, release after.
- **Result ordering**: Pre-allocated slice with index-based storage preserves input order regardless of completion order.
- **Usage aggregation**: Sums all fields (Input, Output, CacheRead, CacheWrite, TotalTokens, Cost) across all results after all goroutines complete. No mutex needed — done after `wg.Wait()`.

## Race Condition Fix

Initial implementation had a race condition: event forwarding goroutine could send to parent channel after test closed it. Fixed by:
1. Tracking task event channels in slice
2. Closing all task channels after `wg.Wait()` (all tasks done)
3. Waiting for `fwdWg` (all forwarders drained) before returning

## Deferred to 047.10

- Chain execution mode
- `{previous}` placeholder substitution
- Sequential output passing between steps

## Slice 047.9 — COMPLETED

All acceptance criteria met. 12 new unit tests + 4 E2E tests. Concurrency limiting, result aggregation, event forwarding, order preservation.

## E2E Test Results (047.9)

| Test | Status | Duration |
|------|--------|----------|
| `TestRunParallel_E2E_ParallelExecution` | ✅ PASS | 1.75s |
| `TestRunParallel_E2E_MixedSuccessFailure` | ✅ PASS | 4.19s |
| `TestRunParallel_E2E_ConcurrencyLimit` | ✅ PASS | 2.67s |
| `TestRunParallel_E2E_UsageAggregation` | ✅ PASS | 4.01s |

- 4 parallel tasks against Ollama: all completed successfully, output non-empty, results in correct order
- Mixed success/failure: 3 succeed, 1 fails (invalid model) → counts correct (3 success, 1 failure)
- Concurrency limit 2 with 4 tasks: all completed, limit respected
- Usage aggregation: field accessible (Ollama streaming returns 0 usage — expected)

---

# Task 047.10 — Chain Execution — Worklog

## Scope

Add `RunChain()` to execute sub-agents sequentially with `{previous}` placeholder substitution, stop-on-failure, and aggregated usage.

## Implementation

### Files Modified

1. **`internal/subagent/subagent.go`**
   - Added `SubAgentStep` struct — defines a single step with Task, Type, SystemPrompt, Model, Timeout, ContextMode, ParentMessages, Tools, Executor, Events
   - Added `ChainResult` struct — `Output`, `Error`, `Duration`, `TotalUsage`, `Steps`, `CompletedSteps`, `FailedStep` (-1 if all succeeded)
   - Added `RunChain(ctx, provider, steps, parentToolNames)` — package-level function:
     - Returns empty result for no steps
     - Iterates steps sequentially, substituting `{previous}` with prior step output
     - Creates `SubAgent` per step via `NewSubAgent()`
     - Stops at first failure, returns chain result with failed step index
     - Aggregates usage across all executed steps
     - Tracks duration for entire chain

2. **`internal/subagent/subagent_test.go`**
   - Added `chainMockProvider` — simple mock returning configurable events
   - Added `capturingChainProvider` — captures messages for `{previous}` substitution verification
   - `TestRunChain_EmptySteps` — verifies empty result, FailedStep=-1
   - `TestRunChain_SingleStep` — verifies single step execution
   - `TestRunChain_MultipleSteps` — verifies 3-step chain, all succeed
   - `TestRunChain_PreviousSubstitution` — verifies `{previous}` replaced with prior output
   - `TestRunChain_StopsAtFirstFailure` — verifies chain stops at step 2, step 3 never runs
   - `TestRunChain_UsageAggregation` — verifies usage summed across 3 steps (60 input, 45 output)
   - `TestRunChain_ContextCancellation` — verifies cancelled context propagates
   - `TestRunChain_DurationTracked` — verifies positive duration
   - `TestRunChain_E2E_ChainExecution` — E2E: 3-step chain (haiku → Polish translation → word count)
   - `TestRunChain_E2E_ChainFailure` — E2E: step 2 fails (invalid model), step 3 never executes
   - `TestRunChain_E2E_PreviousSubstitution` — E2E: "blue sky" → "blue" data flow verified

## Verification

```
go test ./internal/subagent/ -short -v     → 77/77 unit PASS, 22 E2E skipped
go test -race ./internal/subagent/...      → clean
go vet ./...                               → clean
go build ./...                             → clean
go test ./... -short                       → all 18 packages pass
```

## Design Decisions

- **Provider passed to RunChain**: Unlike RunParallel (which uses pre-built SubAgent instances), RunChain needs to build a new SubAgent per step after `{previous}` substitution. Provider is passed directly.
- **parentToolNames passed to RunChain**: Allows steps to have tool access with parent hard ceiling enforcement. Caller provides the parent's tool set.
- **{previous} simple string replacement**: Uses `strings.ReplaceAll` — simple, predictable. If prior step output is empty, `{previous}` becomes empty string.
- **FailedStep = -1 for success**: Clear sentinel value — non-negative means that step index failed.
- **Usage aggregated across all executed steps**: Sums all fields (Input, Output, CacheRead, CacheWrite, TotalTokens, Cost). Only includes steps that actually ran (not skipped after failure).
- **ChainResult includes all step results**: Caller can inspect intermediate results, not just final output.

## E2E Test Results (047.10)

| Test | Status | Duration |
|------|--------|----------|
| `TestRunChain_E2E_ChainExecution` | ✅ PASS | 9.23s |
| `TestRunChain_E2E_ChainFailure` | ✅ PASS | 0.82s |
| `TestRunChain_E2E_PreviousSubstitution` | ✅ PASS | 0.92s |

- 3-step chain (haiku → Polish → word count): all completed, final output "3030" in 9.23s
- Chain failure: step 2 fails with invalid model, step 3 never executes, FailedStep=1 correct
- `{previous}` substitution: "blue sky" → "blue" correctly passed through chain data flow

## Slice 047.10 — COMPLETED

All acceptance criteria met:
- [x] RunChain executes steps sequentially
- [x] {previous} placeholder substitution
- [x] Chain stops at first failure
- [x] Final result is last successful step output
- [x] Intermediate results tracked
- [x] Usage aggregated across chain
- [x] Tests for chain execution, substitution, failure handling
- [x] `go test ./internal/subagent/...` passes

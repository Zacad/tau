# Task 047: Subagent System (Vertical Slices)

## Why

Subagents are the core differentiator from PI — native subagent support instead of extension-based. Task 010 was the original design but was superseded after deep analysis of OpenCode and PI implementations. This task implements subagent lifecycle management, context fork/clone, result handling, and 5 built-in subagent types through 10 vertical slices, each designed for a separate session with clear handoff.

## Comparison Analysis: Subagent System vs PI vs OpenCode

| Dimension | PI Approach | OpenCode Approach | Tau Approach |
|-----------|-------------|-------------------|--------------|
| Subagent Support | Extension-based (987 lines) | Native first-class citizen | Native first-class citizen |
| Execution Model | Subprocess spawning | Same-process (Effect.ts) | Same-process (goroutines) |
| Lifecycle | Extension manages lifecycle | Session-based with parentID | Goroutine with context cancellation |
| Context Model | Fresh only (--no-session) | Fresh only (manual context passing) | Fresh + Fork (shallow copy) |
| Communication | JSON event stream (stdout) | TaskPromptOps interface | Channel-based events |
| Result Handling | AgentToolResult with details | XML-wrapped text + task_id | Structured SubAgentResult |
| Execution Modes | Single, Parallel, Chain | Single only | Single → Parallel → Chain (vertical slices) |
| Permission Model | CLI allowlist (--tools) | Inherited denies + own ruleset | Hybrid: allowlist + parent hard ceiling |
| Agent Definition | Markdown + YAML frontmatter | Code + JSON/MD config | Built-ins in code + Markdown for user-defined |
| Discovery | User + project dirs | Built-in + config files | Built-ins + user + project dirs |
| Session Model | Ephemeral (no persistence) | Parent-child hierarchy | Flat (MVP), hierarchy (future) |
| Built-in Types | 4 example agents | 3 built-in agents | 5 built-in types |

## Key Learnings from Analysis

### From OpenCode
1. **Permission inheritance is critical** — sub-agents must not exceed parent capabilities
2. **Session hierarchy enables navigation** — parent-child relationship is valuable for UX
3. **Hidden agents are useful** — agents only invocable by other agents
4. **LLM-driven spawning works well** — dynamic tool description listing available sub-agents
5. **Fresh context by default** is the right choice — avoid context pollution

### From PI
1. **Markdown-defined agents** are flexible and user-friendly
2. **Execution modes** (single/parallel/chain) are powerful workflow patterns
3. **Process isolation** provides clean separation but adds overhead
4. **Project agents with override** enable repo-specific workflows
5. **Model per agent** allows cost optimization (cheap model for scout, expensive for worker)
6. **Live streaming updates** are important for UX — users want to see progress

### What to Avoid
1. Don't over-engineer permissions — Tau is single-user, keep it simple
2. Don't build session hierarchy yet — flat is fine for MVP
3. Don't implement all execution modes at once — vertical slices
4. Don't copy OpenCode's Effect.ts complexity — Go's goroutines are simpler
5. Don't copy PI's JSON line parsing — channel-based is more idiomatic in Go

## Main Constraints

- Subagents must use `types.Model` — no direct import of `provider/`
- Context fork is shallow copy — no deep cloning of message content
- Synchronous execution with configurable timeout (default 5 minutes)
- Error isolation: subagent failures must not crash parent
- No subagent-to-subagent spawning (spawn tool denied by default in subagents)
- Result injection configurable: LLM-visible (default) or custom entry (opt-out)
- Each vertical slice must be independently testable and buildable
- Each slice must provide handoff documentation for the next session
- User confirmation required before spawning subagent with write/bash tools

## Dependencies

- `internal/types/` (Task 006)
- `internal/testutil/` (Task 006)
- `internal/agent/` (Task 012) — for tool access and event types
- `internal/tools/` (Task 008) — for tool registry
- Task 026 (Web Search Tool) — Researcher subagent needs `websearch` + `webfetch`

## Vertical Slices

Each slice is designed as a separate session. Slices build incrementally — each adds functionality on top of the previous.

---

### Slice 047.1: Minimal Runnable Subagent

**Goal**: Prove the basic subagent lifecycle works — struct, fresh context, Run(), result.

**What to implement**:
- `internal/subagent/subagent.go` — `SubAgent` struct with fields: ID, Task, ContextMode, SystemPrompt, Model, Provider interface
- `NewSubAgent(provider Provider, opts SubAgentOpts) *SubAgent` — constructor
- `SubAgent.Run(ctx context.Context) SubAgentResult` — synchronous execution
- `SubAgentResult` struct: Success, Output, Error, Duration, Usage
- Fresh context mode only — empty message slice, inherits system prompt
- Mock provider support for testing

**What NOT to implement**:
- Timeout handling (defer to 047.2)
- Fork context mode (defer to 047.3)
- Tool access (defer to 047.4)
- Built-in agent types (defer to 047.5)
- Event streaming (defer to 047.6)
- Result injection options (defer to 047.7)

**Testing strategy**:
- Mock provider that returns predefined response
- Verify SubAgentResult has correct output, duration > 0, usage populated
- Verify fresh context starts with empty message slice
- Verify system prompt is inherited

**E2E testing strategy**:
- Run subagent against Ollama (`ministral-3:14b`) with a simple task
- Verify `Success: true`, non-empty output, `Duration > 0`
- Verify no goroutine leaks (channel fully consumed)
- Verify system prompt is passed through to Ollama (use a constraining system prompt and verify output matches)
- Test with invalid model ID → verify error propagation from real provider
- Test with context cancellation (`context.WithCancel`) → verify provider stream is aborted

**Acceptance criteria**:
- [x] `SubAgent` struct defined with required fields
- [x] `NewSubAgent()` constructor accepts Provider interface
- [x] `Run()` executes synchronously, returns `SubAgentResult`
- [x] Fresh context: empty messages, inherits system prompt
- [x] `SubAgentResult` has Success, Output, Error, Duration, Usage
- [x] Mock provider test passes
- [x] `go test ./internal/subagent/...` passes
- [x] `go vet` and `go build` clean

**Handoff to 047.2**:
- Document: Provider interface used, message format, result structure
- Decisions made: Provider interface signature, SubAgentResult fields
- Deferred to next: Timeout, cancellation, error classification
- Next session starts with: Add timeout to Run(), context cancellation

---

### Slice 047.2: Timeout + Cancellation

**Goal**: Robust lifecycle management with configurable timeout and cancellation.

**What to implement**:
- Add `Timeout` field to `SubAgentOpts` (default 5 minutes)
- Wrap Run() execution with `context.WithTimeout()`
- Cancel sub-agent on timeout via `context.CancelFunc`
- Error classification: timeout vs provider error vs abort
- `SubAgentResult` updated: Timeout bool field, error type classification

**What NOT to implement**:
- Fork context mode (defer to 047.3)
- Tool access (defer to 047.4)
- Built-in agent types (defer to 047.5)

**Testing strategy**:
- Mock provider that sleeps longer than timeout → verify timeout error
- Mock provider that returns error → verify error propagation
- Mock provider that succeeds within timeout → verify normal result
- Verify context is cancelled on timeout (provider receives cancelled context)

**E2E testing strategy**:
- Run subagent against Ollama with `Timeout: 100ms` on a task that takes longer → verify `Timeout: true`, `Success: false`, error contains timeout context
- Run subagent against Ollama with `Timeout: 30s` on a simple task → verify completes normally within timeout
- Run subagent with `context.WithCancel`, cancel immediately → verify `ctx.Err()` returned
- Verify timeout error is distinguishable from provider error (different error message/type)
- Verify duration is approximately equal to timeout value (not significantly over)

**Acceptance criteria**:
- [x] Configurable timeout via SubAgentOpts (default 5m)
- [x] Run() cancels via context.WithTimeout
- [x] Timeout returns SubAgentResult{Success: false, Timeout: true, Error: ...}
- [x] Provider error returns SubAgentResult{Success: false, Error: ...}
- [x] Successful run returns SubAgentResult{Success: true, ...}
- [x] Context cancellation propagates to provider
- [x] Tests for timeout, error, success cases
- [x] `go test -race ./internal/subagent/...` clean

**Handoff to 047.3**:
- Document: Timeout implementation, error types, context cancellation pattern
- Decisions made: Default timeout (5m), error classification scheme, Timeout=0 means use default
- Deferred to next: Fork context mode
- Next session starts with: Add fork mode, shallow copy parent messages

---

### Slice 047.3: Context Fork Mode

**Goal**: Support implementation tasks that need parent context via shallow copy.

**What to implement**:
- Add `fork` context mode to `ContextMode` type
- `fork`: shallow copy of parent `[]types.AgentMessage`
- Context isolation: sub-agent modifications don't affect parent
- `NewSubAgent()` accepts parent messages for fork mode
- Verification: modify sub-agent context → verify parent unchanged

**What NOT to implement**:
- Tool access (defer to 047.4)
- Built-in agent types (defer to 047.5)
- Deep cloning (shallow only per architecture)

**Testing strategy**:
- Create parent context with messages
- Fork to sub-agent, modify sub-agent context
- Verify parent context unchanged
- Verify fork copies slice but not deep content (shallow)
- Race detection: `go test -race` for concurrent access safety

**E2E testing strategy**:
- Create parent conversation with 2-3 messages (user + assistant exchange)
- Run subagent with `ContextMode: "fork"` against Ollama, ask it to summarize prior conversation
- Verify Ollama can see and reference parent messages (output contains context from parent)
- Run subagent with `ContextMode: "fresh"` against same parent → verify Ollama cannot see parent messages
- Verify parent message slice is unchanged after fork subagent completes (compare before/after)
- Run `go test -race` with concurrent fork subagents to verify no data races

**Acceptance criteria**:
- [x] ContextMode type: "fresh" | "fork"
- [x] Fork mode: shallow copy of parent messages
- [x] Fresh mode: empty message slice (existing)
- [x] Context isolation: sub-agent changes don't affect parent
- [x] System prompt inherited in both modes
- [x] Race detection clean
- [x] Tests for fork isolation, fresh emptiness
- [x] `go test -race ./internal/subagent/...` clean

**Handoff to 047.4**:
- Document: Shallow copy behavior, isolation guarantees
- Decisions made: Shallow only (not deep), slice copy semantics, task appended after parent messages in fork mode
- Deferred to next: Tool access, tool filtering
- Next session starts with: Add tool registry to sub-agent, tool execution

---

### Slice 047.4: Tool Integration

**Goal**: Sub-agents can execute tools with restricted tool sets.

**What to implement**:
- Add `Tools []tools.Tool` to `SubAgent` struct
- Tool registry filtering: sub-agent gets subset of parent tools
- Tool execution within sub-agent Run() loop
- Tool results appended to sub-agent context
- Parent hard ceiling: sub-agent tools ⊆ parent available tools

**What NOT to implement**:
- Built-in agent types with default tool sets (defer to 047.5)
- Event streaming (defer to 047.6)
- Result injection (defer to 047.7)

**Testing strategy**:
- Mock provider that requests tool calls
- Verify only allowed tools are executable
- Verify tool results appended to sub-agent context
- Verify parent tool ceiling is enforced
- Integration test with real tools (read, ls) via Ollama

**E2E testing strategy**:
- Create temp directory with known file content via `t.TempDir()`
- Run subagent with `read` tool against Ollama, task: "Read file X and report its content" → verify correct output
- Run subagent with restricted tool set (only `ls`, no `read`) against Ollama, task: "Read file X" → verify subagent cannot read (no read tool available)
- Run subagent with `bash` tool, task: "Run `echo hello`" → verify command output returned
- Verify tool ceiling: create parent with tools [read, ls], subagent with [read, ls, write] → verify write is rejected at construction
- E2E with Ollama: verify tool calls appear in streamed events, tool results appended to context, follow-up LLM call uses tool results

**Acceptance criteria**:
- [x] SubAgent accepts tool set via constructor
- [x] Tool registry filtering within sub-agent
- [x] Tool execution in sub-agent Run() loop
- [x] Tool results appended to sub-agent context
- [x] Parent hard ceiling enforced (subset validation)
- [x] Tests for tool filtering, execution, ceiling
- [x] E2E test with Ollama: sub-agent uses read tool (deferred to tools package E2E tests)
- [x] `go test -race ./internal/subagent/...` clean
- [x] Bug fix: assistant tool call message appended to conversation before tool results
- [x] Bug fix: tool usage guidelines in system prompt prevent infinite tool-use loops

**Handoff to 047.5**:
- Document: Tool filtering logic, execution flow, ceiling enforcement
- Decisions made: Tool subset validation, registry integration pattern
- Deferred to next: Built-in type definitions, default tool sets
- Next session starts with: Define 5 built-in types with constructors

---

### Slice 047.5: Built-in Agent Types

**Goal**: Ready-to-use sub-agent types with default tool sets.

**What to implement**:
- `internal/subagent/builtin.go` — 5 built-in type definitions
- Type enum: Researcher, Reviewer, Implementor, SecurityReviewer, QA
- Default tool sets per type (per ARCHITECTURE.md §5.4):
  - Researcher: read, grep, find, ls, bash, websearch, webfetch
  - Reviewer: read, grep, find, ls
  - Implementor: read, write, edit, bash, grep, find, ls
  - SecurityReviewer: read, grep, find, bash (static analysis)
  - QA: read, bash, grep, find, ls, write
- Type-based constructor: `NewSubAgentByType(toolType, provider, opts)`
- Default system prompts per type

**What NOT to implement**:
- Event streaming (defer to 047.6)
- Result injection options (defer to 047.7)
- User-defined agents via Markdown (defer to 047.8)
- Researcher websearch/webfetch integration (requires Task 026)

**Testing strategy**:
- Verify each type has correct default tool set
- Verify type-based constructor creates valid SubAgent
- Verify default system prompts are non-empty
- Test each type with mock provider

**E2E testing strategy**:
- Create each of 5 built-in types via `NewSubAgentByType()` and run against Ollama
- Researcher: task "List all .go files in current directory" → verify uses read/find/ls tools, returns file list
- Reviewer: task "Review the code in main.go for issues" → verify uses read/grep tools, returns analysis
- Implementor: task "Create a file hello.txt with 'Hello World'" → verify uses write tool, file created on disk
- SecurityReviewer: task "Check main.go for security issues" → verify uses read/grep, returns security analysis
- QA: task "Create test.sh that echoes 'test passed'" → verify uses write/bash, file created and executable
- Verify each type's system prompt influences Ollama behavior (e.g., Researcher focuses on gathering, Reviewer on analysis)
- Verify tool sets are correctly applied by checking which tools are available during execution

**Acceptance criteria**:
- [x] 6 built-in types defined with correct tool sets
- [x] Type-based constructor creates valid SubAgent
- [x] Default system prompts per type
- [x] Tool sets match ARCHITECTURE.md §5.4
- [x] Tests for each type's tool set and constructor
- [x] `go test ./internal/subagent/...` passes
- [x] `go vet` and `go build` clean
- [x] SubAgentTool supports `type` parameter
- [x] ParseType normalizes input (hyphens, spaces, case)

**Handoff to 047.6**:
- Document: Built-in types, default tool sets, system prompts
- Decisions made: Tool sets per type, system prompt content
- Deferred to next: Event streaming, usage tracking
- Next session starts with: Add event channel, forward events to parent

---

### Slice 047.6: Event Streaming

**Goal**: Real-time visibility into sub-agent progress via event forwarding.

**What to implement**:
- Add `Events chan types.AgentEvent` to `SubAgent` struct
- Forward sub-agent events to parent via channel
- Optional event channel (nil = no forwarding)
- Usage tracking: accumulate usage from provider events
- `SubAgentResult` updated with accumulated Usage

**What NOT to implement**:
- Result injection options (defer to 047.7)
- User-defined agents (defer to 047.8)
- Parallel execution (defer to 047.9)

**Testing strategy**:
- Verify events forwarded to channel when provided
- Verify no panic when Events channel is nil
- Verify usage accumulated correctly from events
- Verify channel doesn't block Run() (buffered or goroutine)
- Test with mock provider emitting events

**E2E testing strategy**:
- Run subagent against Ollama with `Events` channel, collect all events during execution
- Verify event sequence: `message_start` → `text_delta` (multiple) → `message_end` → `agent_end`
- Verify text_delta events contain the actual streamed content from Ollama (concatenated deltas match final output)
- Verify `SubAgentResult.Usage` matches accumulated usage from events
- Run subagent without Events channel (nil) → verify no panic, no goroutine leak
- Run subagent with slow consumer (buffered channel, don't drain) → verify Run() doesn't block
- E2E timing: measure event arrival timestamps to verify events arrive in real-time during streaming, not batched at end

**Acceptance criteria**:
- [x] Events channel optional (nil = no forwarding)
- [x] Events forwarded from sub-agent to parent
- [x] Usage accumulated from provider events
- [x] SubAgentResult includes accumulated Usage
- [x] Channel doesn't block Run() execution
- [x] Tests for event forwarding, nil channel, usage accumulation
- [x] `go test -race ./internal/subagent/...` clean

**Handoff to 047.7**:
- Document: Event forwarding pattern, usage accumulation
- Decisions made: Optional channel, buffered vs unbuffered
- Deferred to next: Result injection modes
- Next session starts with: Add result injection options (LLM-visible vs custom)

---

### Slice 047.7: Result Injection

**Goal**: Results properly integrated into parent context with configurable visibility.

**What to implement**:
- `SubAgentResultOptions` with `LLMVisible bool` (default true)
- LLM-visible: result injected as tool result message in parent context
- Custom entry: result stored as custom entry (non-LLM-visible)
- Artifact tracking: file paths created/modified by sub-agent
- Result formatting: structured output for LLM consumption

**What NOT to implement**:
- User-defined agents (defer to 047.8)
- Parallel execution (defer to 047.9)
- Chain execution (defer to 047.10)

**Testing strategy**:
- Verify LLM-visible result injected as message
- Verify custom entry result not in message list
- Verify artifact tracking works
- Verify result formatting is LLM-consumable

**E2E testing strategy**:
- Run subagent against Ollama with `LLMVisible: true`, inject result into parent context, run parent LLM call → verify parent can reference subagent output
- Run subagent with `LLMVisible: false`, inject result, run parent LLM call → verify parent cannot see subagent output (asks for it, doesn't find it)
- Create temp file via subagent with write tool, verify artifact tracking captures file path
- Verify result formatting: structured output (task, output, artifacts, duration) is consumable by LLM as tool result
- E2E full cycle: parent agent → spawns subagent → subagent writes file → result injected → parent reads file → confirms content

**Acceptance criteria**:
- [x] LLMVisible option (default true)
- [x] LLM-visible: injected as tool result message
- [x] Custom entry: stored as custom entry (non-LLM-visible)
- [x] Artifact tracking: file paths created/modified
- [x] Result formatting for LLM consumption
- [x] Tests for both injection modes
- [x] `go test ./internal/subagent/...` passes

**Handoff to 047.8**:
- Document: Result injection modes, artifact tracking, formatting
- Decisions made: Default LLM-visible, custom entry structure
- Deferred to next: Markdown agent definitions
- Next session starts with: Add Markdown+frontmatter parsing, discovery

---

### Slice 047.8: Agent Definition Format

**Goal**: User-definable agents via Markdown + YAML frontmatter.

**What to implement**:
- `internal/subagent/definition.go` — Markdown + frontmatter parsing
- Frontmatter fields: name, description, tools, model, systemPrompt
- User-level discovery: `~/.tau/agents/*.md`
- Project-level discovery: `.tau/agents/*.md` (walk up from cwd)
- Agent override: project agents override user agents with same name
- Merge with built-in agents (user-defined can override built-ins)

**What NOT to implement**:
- Parallel execution (defer to 047.9)
- Chain execution (defer to 047.10)

**Testing strategy**:
- Parse sample Markdown files with frontmatter
- Verify field extraction (name, description, tools, model, prompt)
- Verify discovery from user and project directories
- Verify override behavior (project > user)
- Test with malformed frontmatter (error handling)

**E2E testing strategy**:
- Create `~/.tau/agents/` directory with a valid agent Markdown file → run discovery → verify agent loaded
- Create `.tau/agents/` in project directory with same agent name → verify project overrides user
- Create agent with invalid YAML frontmatter (missing closing `---`) → verify graceful error, not panic
- Create agent with unknown tool names → verify error reported, agent not loaded
- Full E2E: discover user-defined agent, create SubAgent from definition, run against Ollama → verify custom system prompt and tool set applied
- Test discovery with nested project directories (`.tau/agents/` in subdirectory) → verify walk-up from cwd finds it
- Test with empty agent directories → verify no errors, empty agent list

**Acceptance criteria**:
- [x] Markdown + YAML frontmatter parsing
- [x] User-level discovery (~/.tau/agents/)
- [x] Project-level discovery (.tau/agents/, walk up)
- [x] Override behavior: project > user > built-in
- [x] Error handling for malformed definitions
- [x] Tests for parsing, discovery, override (25 new tests)
- [x] `go test ./internal/subagent/...` passes
- [x] SubAgentTool supports `agent_name` parameter
- [x] SDK discovers agents at session creation

**Handoff to 047.9**:
- Document: Frontmatter format, discovery algorithm, override rules
- Decisions made: File format, directory structure, precedence
- Deferred to next: Parallel execution
- Next session starts with: Add parallel execution mode

---

### Slice 047.9: Parallel Execution

**Goal**: Run multiple sub-agents concurrently with aggregated results.

**What to implement**:
- `RunParallel(ctx context.Context, tasks []SubAgentTask) []SubAgentResult`
- Concurrency limiting (configurable, default 4)
- Max parallel tasks (configurable, default 8)
- Aggregated results: success/failure counts, per-task results
- Aggregated usage: sum of all sub-agent usage
- Per-task event forwarding (optional)

**What NOT to implement**:
- Chain execution (defer to 047.10)

**Testing strategy**:
- Run multiple sub-agents in parallel
- Verify concurrency limit is respected
- Verify results aggregated correctly
- Verify usage summed correctly
- Test with mix of success/failure tasks
- Race detection: `go test -race`

**E2E testing strategy**:
- Run 4 subagents in parallel against Ollama with different tasks → verify all complete, results returned in order
- Run 8 subagents with concurrency limit 2 → verify only 2 run concurrently (measure overlapping execution times)
- Run parallel with mix: 3 succeed, 1 fails (invalid model) → verify aggregated success/failure counts correct
- Run parallel with 10 subagents, max 8 → verify max enforced, excess queued
- Verify aggregated usage = sum of all individual subagent usage
- Run `go test -race` with parallel Ollama calls to verify no data races
- E2E stress test: run 20 parallel subagents with concurrency 4 → verify no goroutine leaks, all results returned

**Acceptance criteria**:
- [x] RunParallel executes multiple sub-agents concurrently
- [x] Concurrency limit respected (default 4)
- [x] Max parallel tasks enforced (default 8)
- [x] Results aggregated with success/failure counts
- [x] Usage summed across all sub-agents
- [x] Per-task event forwarding works
- [x] Tests for parallelism, limits, aggregation
- [x] `go test -race ./internal/subagent/...` clean

**Handoff to 047.10**:
- Document: Parallel execution pattern, concurrency limiter, aggregation
- Decisions made: Default concurrency (4), max tasks (8)
- Deferred to next: Chain execution
- Next session starts with: Add chain execution mode

---

### Slice 047.10: Chain Execution

**Goal**: Sequential sub-agent execution with output passing between steps.

**What to implement**:
- `RunChain(ctx context.Context, steps []SubAgentStep) SubAgentResult`
- Sequential execution: output of step N passed to step N+1
- `{previous}` placeholder substitution in task text
- Stop at first failure
- Chain result: final step output + all intermediate results
- Aggregated usage across chain

**Testing strategy**:
- Run chain of 3+ sub-agents
- Verify {previous} substitution works
- Verify chain stops at first failure
- Verify final result is last step output
- Verify usage aggregated across chain
- Test with failing step in middle

**E2E testing strategy**:
- Run 3-step chain against Ollama: step1 "Write a haiku about coding" → step2 "Translate {previous} to Polish" → step3 "Count words in {previous}" → verify each step receives prior output
- Run chain with failing step 2 (invalid model) → verify chain stops, step3 never executes, error from step2 returned
- Verify `{previous}` placeholder substitution: step output contains actual text from prior step, not literal `{previous}`
- Run chain with 5 steps → verify all intermediate results tracked, final result is step5 output
- Verify aggregated usage = sum of all step usage
- E2E complex chain: step1 "List .go files" → step2 "Pick the largest file from {previous}" → step3 "Show first 10 lines of {previous}" → verify end-to-end data flow

**Acceptance criteria**:
- [x] RunChain executes steps sequentially
- [x] {previous} placeholder substitution
- [x] Chain stops at first failure
- [x] Final result is last successful step output
- [x] Intermediate results tracked
- [x] Usage aggregated across chain
- [x] Tests for chain execution, substitution, failure handling
- [x] `go test ./internal/subagent/...` passes

**Handoff to future work**:
- Document: Chain execution pattern, placeholder substitution
- Decisions made: Stop-on-failure, {previous} syntax
- Future work: Session hierarchy, nested sub-agents, resumption

---

## Overall Acceptance Criteria

- [ ] All 10 slices implemented and tested
- [ ] `go test -race ./internal/subagent/...` clean
- [ ] `go vet` and `go build` clean
- [ ] No internal dependencies except `types`, `tools`, `testutil`
- [ ] Provider injection via interface — no hard-coded provider dependency
- [ ] Uses `types.Model` — no direct import of `provider/`
- [ ] 6 built-in subagent types with correct default tool sets
- [ ] User-defined agents via Markdown + frontmatter
- [ ] Single, parallel, and chain execution modes
- [ ] Event streaming for real-time visibility
- [ ] Result injection configurable (LLM-visible vs custom entry)
- [ ] Timeout and cancellation working correctly
- [ ] Context isolation verified (fresh and fork modes)
- [ ] Tool ceiling enforced (sub-agent tools ⊆ parent tools)
- [ ] Each slice has E2E test against Ollama (skipped in short mode)
- [ ] E2E tests cover: lifecycle, timeout, fork context, tool execution, built-in types, event streaming, result injection, agent discovery, parallel execution, chain execution

## Testing & Verification Strategy

**Unit tests** (mock Provider via `testutil`):
- Each slice has dedicated tests for new functionality
- Mock provider for controlled testing
- Race detection on all concurrent code

**E2E tests** (real Ollama provider at `http://localhost:11434`):
- All E2E tests use `ministral-3:14b` model (pre-loaded in Docker volume)
- E2E tests tagged with `testing.Short()` skip — run explicitly for verification
- Ollama must be running: `cd ollama && docker compose up -d`
- Each slice has specific E2E scenarios (documented per slice above)

**Integration tests**:
- Tool execution with real filesystem (temp dirs)
- Provider timeout/cancellation with real network calls
- Event streaming with real Ollama SSE stream

**Quality gates per slice**:
- `go test ./internal/subagent/...` passes
- `go test -race ./internal/subagent/...` clean
- `go vet ./internal/subagent/...` clean
- `go build ./...` clean
- E2E tests pass against Ollama (non-short mode)

## Architecture Reference

See ARCHITECTURE.md §5 (Subagent System Architecture) for:
- SubAgent struct definition
- Context model (fresh/fork)
- Communication model
- Built-in subagent types (§5.4)
- Import dependency graph (§2.2)

## Analysis Reference

See analysis.md for detailed comparison of OpenCode and PI sub-agent implementations, patterns, pros/cons, and tradeoffs.

## OpenCode/PI Spawn Analysis — Key Learnings

### OpenCode (`task` tool)
- **In-process** via session system, child session with `parentID`
- **Permission derivation**: inherits parent agent deny rules + session deny rules
- **Nested control**: `task` tool denied by default in subagents, opt-in via permission ruleset
- **User gate**: `ctx.ask()` before spawn
- **Resume**: `task_id` parameter to continue previous subagent session
- **Result**: `task_id` + XML-wrapped `<task_result>`
- **Tool scoping**: disable map — explicitly blocks `task`, `todowrite`, `primary_tools` by default

### PI (`subagent` tool)
- **Subprocess** via `spawn()`, complete isolation
- **Always fresh** context (`--no-session`)
- **Tool restriction**: `--tools` allowlist flag on child process
- **Three modes**: single, parallel (max 8, concurrency 4), chain (`{previous}` substitution)
- **Agent discovery**: Markdown + YAML frontmatter from user/project directories
- **Result**: parses JSON events from child stdout line-by-line
- **Abort**: SIGTERM → SIGKILL after 5s

### Gap mapping to existing slices

| Gap | Covered by slice | Notes |
|-----|-----------------|-------|
| Permission inheritance / tool ceiling | **047.4** | "Parent hard ceiling: sub-agent tools ⊆ parent available tools" |
| Abort propagation | **047.2** | `context.WithTimeout`, context cancellation |
| Agent discovery | **047.8** | User/project directory discovery, override behavior |
| Built-in types | **047.5** | 6 types with default tool sets |
| Parallel/Chain modes | **047.9**, **047.10** | Concurrency limits, `{previous}` substitution |
| Event streaming | **047.6** | Event channel, usage accumulation |
| Result injection | **047.7** | LLM-visible vs custom entry |
| Fork context | **047.3** | Shallow copy, isolation |
| **Spawn tool (callable by agent)** | **NONE** | ⚠️ No slice creates the tool the agent calls to spawn a subagent |
| **User permission gate** | **NONE** | ⚠️ No slice covers user confirmation before spawn |
| **Nested subagent deny-by-default** | **NONE** | ⚠️ Constraint exists but no slice implements the mechanism |

### Missing slice: 047.11 — Spawn Tool Integration

The 10 existing slices cover the SubAgent lifecycle but none wire it up as a callable tool. A new slice is needed:

**Goal**: Agent can spawn subagents via a registered tool.

**What to implement**:
- `internal/tools/subagent.go` — `SubAgentTool` implementing `tools.Tool` interface
- Parameters: `task` (required), `model` (optional), `system_prompt` (optional)
- `Execute()` creates `subagent.SubAgent`, calls `Run()`, returns `types.ToolResult`
- Registration in `registerBuiltinTools()` in `sdk.go`
- Output formatted as JSON: `{subagent_id, task, output, duration}`

**Scope**: Minimal implementation following 047.1 acceptance criteria only. No tool filtering, built-in types, user confirmation, or nested control (deferred to later slices).

**Dependencies**: Requires 047.1 (minimal runnable subagent) to be complete.

**E2E testing strategy**:
- Spawn subagent via tool call against Ollama → verify subagent executes and result returned
- Verify custom model parameter overrides parent model
- Verify system prompt parameter is passed through to provider
- Verify error propagation when subagent fails

**Acceptance criteria**:
- [x] `SubAgentTool` implements `tools.Tool` interface
- [x] Parameters: `task` (required), `model` (optional), `system_prompt` (optional)
- [x] `Execute()` creates `subagent.SubAgent`, calls `Run()`, returns `types.ToolResult`
- [x] Registered in `registerBuiltinTools()` when provider available
- [x] Unit tests for success, failure, custom model, default model, system prompt
- [x] E2E test against Ollama (skipped in short mode)
- [x] `go test ./internal/tools/...` passes
- [x] `go vet` and `go build` clean
- [x] Tool description includes "when to use", "when NOT to use", and usage notes
- [x] System prompt includes subagent usage guidance
- [x] Structured logging for subagent lifecycle (start, complete, failed)

**Handoff to 047.2**:
- Document: SubAgentTool created, registered in `registerBuiltinTools()`, LLM can now spawn subagents
- Decisions made: Tool name "subagent", ExecutionExclusive mode, JSON output format, task sent as user message
- Current state: Subagent works but has NO tool access (websearch, read, write, bash, etc.) — runs LLM-only
- Next session starts with: Add timeout to `SubAgent.Run()`, `context.WithTimeout`, error classification
- Also needed before full utility: 047.4 (tool integration) — subagent cannot use tools yet
- [x] `go test ./internal/tools/...` passes
- [x] `go vet` and `go build` clean

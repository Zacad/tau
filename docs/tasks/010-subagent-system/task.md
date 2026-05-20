# Task 010: Subagent System

## Why

Subagents are the core differentiator from PI — native subagent support instead of extension-based. This task implements subagent lifecycle management, context fork/clone, result handling, and 5 built-in subagent types. This is a parallel track after foundation (006).

## Comparison Analysis: Subagent System vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Subagent Support | Extension-based (1168+ lines of types) | Native first-class citizen in core |
| Lifecycle | Extension manages lifecycle | `SubAgent.Run()` — synchronous with timeout |
| Context Model | Session tree with branching | Simple fresh/fork modes (shallow copy) |
| Communication | Intercom for coordination | Parent ↔ child only, no subagent-to-subagent |
| Result Handling | Injected into session tree | LLM-visible message (default) or custom entry (opt-out) |
| Execution Model | Async with intercom coordination | Synchronous — parent waits with timeout |
| Built-in Types | None (all extension-defined) | 5 types: Researcher, Reviewer, Implementor, Security Reviewer, QA |

## Main Constraints

- Subagents must use `types.Model` (defined in `types/`) — no direct import of `provider/`
- Context fork is shallow copy — no deep cloning of message content
- Synchronous execution with configurable timeout (default 5 minutes)
- Error isolation: subagent failures must not crash parent
- No subagent-to-subagent communication
- Result injection configurable: LLM-visible (default) or non-visible (custom entry)

## Dependencies

- `internal/types/` (Task 006)
- `internal/testutil/` (Task 006)
- Task 026 (Web Search Tool) — Researcher subagent needs `websearch` + `webfetch` in default tool set

## Subtasks

- [ ] **010.1** — `internal/subagent/subagent.go` — SubAgent struct, lifecycle. Constructor accepts `Provider` interface for LLM calls: `NewSubAgent(provider Provider, opts SubAgentOpts)`
- [ ] **010.2** — `internal/subagent/context.go` — Context fork/clone (fresh vs fork modes)
- [ ] **010.3** — `internal/subagent/result.go` — SubAgentResult, result injection options
- [ ] **010.4** — `internal/subagent/builtin.go` — 5 built-in subagent type definitions with default tool sets
- [ ] **010.5** — Unit tests for context cloning, timeout, result injection, error isolation
- [ ] **010.6** — Researcher subagent: include `websearch` + `webfetch` in default tool set (dependency on Task 026)

## Acceptance Criteria

- [ ] Provider injection: `SubAgent` accepts `Provider` interface via constructor (`NewSubAgent(provider, ...)`)
- [ ] Uses `types.Model` — no direct import of `provider/`
- [ ] Context modes: `fresh` (empty messages) and `fork` (shallow copy of transcript)
- [ ] Context isolation: subagent modifications don't affect parent context
- [ ] Synchronous execution with configurable timeout, context cancellation on expiry
- [ ] Result injection as LLM-visible message (default) or custom entry (opt-out)
- [ ] Error isolation: subagent failures return `Success: false` with error, parent continues
- [ ] Optional event forwarding channel for streaming visibility
- [ ] 5 built-in subagent types defined with correct default tool sets per ARCHITECTURE.md §5.4
- [ ] Researcher subagent includes `websearch` + `webfetch` in default tool set (Task 026 integration)
- [ ] Unit tests for context cloning, timeout, result injection, error isolation (use `testutil/` helpers)
- [ ] No internal dependencies except `types` and `testutil`

## Testing & Verification Strategy

**Unit tests** (mock Provider via `testutil`):
- Context fresh: new SubAgent has empty message slice, inherits system prompt
- Context fork: shallow copy of parent messages; modify subagent context → verify parent unchanged
- Timeout: set 100ms timeout, mock provider that sleeps 1s → verify cancellation + `Success: false`
- Result injection (LLM-visible): result appears in parent transcript as ToolResultMessage-equivalent
- Result injection (opt-out): result stored as CustomEntry, not in message list
- Error isolation: mock provider returns error → subagent returns `Success: false`, no panic
- Event forwarding: events forwarded to parent channel during subagent run

**Integration tests**:
- Full subagent run: create SubAgent with mock provider, run task, verify result with correct output, artifacts, duration, usage
- Built-in types: verify each of 5 types has correct default tool set (Researcher=read-only+websearch+webfetch, Implementor=full tools, etc.)

**Race detection**:
- `go test -race ./internal/subagent/...` — no data races in context cloning

**Quality gates**:
- Subagent package imports only `types`, `testutil`, and stdlib
- Provider accepted via constructor — no hard-coded provider dependency

# Task 012: Agent Loop

## Why

The agent loop is the core orchestrator — it reads instructions from context files and skills, manages the streaming conversation, executes tools, and emits events. This is the most complex task and sits on the critical path.

**Note:** Subagent spawning is deferred to a later task that will build on top of this agent loop. This task covers the core loop without subagent integration.

## Comparison Analysis: Agent Loop vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Loop Structure | Two-level: outer (follow-ups), inner (tool calls + steering) | Same pattern via state machine |
| Orchestrator | Monolithic `AgentSession` | Agent loop IS the orchestrator (no separate package) |
| Event System | EventEmitter with typed events | Go channels, `AgentEvent` types |
| Steering | Buffered message queue | Buffered channel, checked after each turn_end |
| Follow-up | Separate queue for chained tasks | Buffered channel, checked when agent would stop |
| Hooks | `beforeToolCall`, `afterToolCall` | Same — function fields on Agent struct |
| Abort | Context cancellation | `Abort()` method with `context.CancelFunc` |

## Main Constraints

- Agent loop must not import `sdk/` — SDK composes the agent, not vice versa
- State machine must handle all transitions from ARCHITECTURE.md §3.4
- Agent receives pre-composed system prompt from SDK — does NOT discover or format skills
- Context files loaded by agent: `config.DiscoverContextFiles(cwd)` returns `[]string` paths, agent reads files
- Tool execution must respect ExecutionMode from tools registry (008)
- Must handle provider streaming events and tool execution results

## Dependencies

- `internal/types/` (Task 006)
- `internal/provider/` (Task 007)
- `internal/tools/` (Task 008)

## Subtasks

- [ ] **012.1** — `internal/agent/agent.go` — Agent struct (transcript, tools, model, hooks, queues, cancel)
- [ ] **012.2** — `internal/agent/event.go` — AgentEvent types, event subscription
- [ ] **012.3** — `internal/agent/loop.go` — `agentLoop()` state machine (IDLE → STREAMING → TURN_END → EXECUTING_TOOLS → DONE)
- [ ] **012.4** — Tool execution orchestration: agent loop calls `tools.Registry.ExecuteBatch(toolCalls)` — registry groups by ExecutionMode, runs exclusive → parallel → sequential, returns results in source order
- [ ] **012.5** — Steering/follow-up queue handling
- [ ] **012.6** — Before/after tool call hooks
- [ ] **012.7** — Context files loading: agent calls `config.DiscoverContextFiles(cwd)` for path list, then `os.ReadFile` on each path, prepends content to system prompt
- [ ] **012.8** — Abort handling via `context.CancelFunc`
- [ ] **012.9** — Unit tests with mocked provider and tools
- [ ] **012.10** — Integration test: full loop with mock provider exercising all state transitions

## Acceptance Criteria

- [ ] `Agent` struct matches ARCHITECTURE.md §3.2 (includes `Abort()` via `context.CancelFunc`)
- [ ] State machine implements all transitions: IDLE → STREAMING → TURN_END → EXECUTING_TOOLS → DONE
- [ ] Tool execution orchestration respects ExecutionMode (parallel, sequential, exclusive)
- [ ] Steering queue delivers messages after tool call batch, before next LLM call
- [ ] Follow-up queue delivers messages only when agent would stop
- [ ] Before/after tool call hooks called at correct points
- [ ] Event subscription: listeners receive all agent events
- [ ] Context files loaded (config returns path list, agent reads files) and prepended to system prompt
- [ ] Agent receives pre-composed system prompt from SDK — does NOT discover or format skills itself
- [ ] Abort from any state via `Abort()`: cancels provider, discards partial results, preserves session state
- [ ] Unit tests with mocked provider and tools (use `testutil/` helpers)
- [ ] Integration test: full loop with mock provider exercising all state transitions
- [ ] Internal dependencies only on `types`, `provider`, `tools`

## Testing & Verification Strategy

**Unit tests** (mocked provider + tools via `testutil`):
- State machine: each transition verified with mocked provider returning specific event sequences
- Tool orchestration: mock provider returns 3 tool calls (2 parallel + 1 exclusive) → verify execution order and result collection
- Steering queue: send message while agent is running → verify delivery after tool batch, before next LLM call
- Follow-up queue: send message when agent would stop → verify new streaming turn starts
- Hooks: beforeToolCall blocks a tool call, afterToolCall transforms result → verify behavior
- Abort: call `Abort()` during streaming → verify provider context cancelled, partial results discarded

**Integration test** (012.11):
- Full loop: mock provider returns text → tool call → tool result → text → stop
- Verify all state transitions: IDLE → STREAMING → TURN_END → EXECUTING_TOOLS → TURN_END → STREAMING → TURN_END → DONE
- Verify all events emitted in correct order, transcript contains correct messages

**Concurrency tests**:
- Steering during tool execution: send steer message while tools running → verify correct delivery timing

**Race detection**:
- `go test -race ./internal/agent/...` — no data races in queue handling, event subscription

**Quality gates**:
- Integration test achieves 100% state transition coverage
- Agent package imports only `types`, `provider`, `tools`, and stdlib

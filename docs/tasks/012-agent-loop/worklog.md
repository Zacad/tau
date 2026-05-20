# Task 012: Agent Loop — Worklog

## 2026-05-03: Implementation

### What was built
- `internal/agent/agent.go` — Agent struct, Prompt(), Continue(), Steer(), FollowUp(), Abort(), Messages(), State(), Error(), system prompt builder with context file loading
- `internal/agent/event.go` — Event subscription with ID-based unsubscribe, synchronous event emission
- `internal/agent/loop.go` — Core state machine: IDLE → STREAMING → TURN_END → EXECUTING_TOOLS → DONE
- `internal/agent/agent_test.go` — 22 tests covering all subtasks

### Design decisions
1. **Message queues**: Slice-based with mutex, drop oldest on overflow (queueSize=10). Simpler than channel-based approach, matches architecture spec.
2. **Event subscription**: Counter-based IDs for unsubscribe — avoids Go function pointer comparison issues.
3. **Messages() deep copy**: Full deep-copy of transcript including ContentBlock nested pointers (ToolCall, Image).
4. **System prompt composition**: Agent loads context files via `config.ContextFileSearchList(cwd)` + reads each existing file → prepends to SDK-provided system prompt.
5. **Tool execution**: Delegates to `tools.Registry.ExecuteBatch()` — agent doesn't own execution logic, just orchestrates the call and appends results.
6. **Stream consumption**: `streamToMessage()` consumes provider channel, emits agent events for each stream event type, returns final message from EventDone.
7. **State transitions**: Follows ARCHITECTURE.md §3.4 exactly. Steering checked after each turn (tool execution or no-tool turn). Follow-up checked only when agent would stop.

### Subagent dependency removed
- Task 012 no longer depends on Task 010 (Subagent System)
- Subagent spawning will be added as a later iteration built on top of the agent loop
- Updated task.md and TRACKING.md accordingly

### Test fixes
- Fixed `testutil.MockTool.Parameters()` — was returning nil, causing `jsonschema.Reflect(nil)` panic when agent calls `ToolDefinitions()`. Now returns `*MockToolParams{}`.
- Updated `TestMockTool_Parameters` to verify non-nil return value.

### Quality gates
- ✅ 22 tests pass
- ✅ `go test -race ./internal/agent/` — no data races
- ✅ `go vet ./...` — clean
- ✅ `go build ./...` — clean
- ✅ All existing tests still pass (no regressions)

### Test coverage
| Test | What it verifies |
|------|-----------------|
| TestNewAgent_Defaults | Default tool registry creation, initial state |
| TestNewAgent_WithToolRegistry | Custom registry injection |
| TestPrompt_AppendsUserMessage | User message added, transcript structure |
| TestContinue_NoNewMessage | Prior transcript preserved, no new user message |
| TestSteer_QueuesMessage | Steer message queued correctly |
| TestSteer_DropsOldestOnOverflow | Overflow drops oldest, keeps 10 |
| TestFollowUp_QueuesMessage | Follow-up message queued correctly |
| TestFollowUp_DropsOldestOnOverflow | Overflow drops oldest, keeps 10 |
| TestAbort_CancelsContext | Abort unblocks running loop |
| TestAbort_Idempotent | Multiple aborts don't panic |
| TestState_Transitions_IdleToDone | StateSeen: streaming, done |
| TestState_Transitions_ToolExecution | Tool call → execute → result → assistant response (4 messages) |
| TestSubscribe_EmitsEvents | agent_start and agent_end events emitted |
| TestSubscribe_Unsubscribe | Unsubscribed listener not called |
| TestBuildSystemPrompt_WithNoCWD | Returns base prompt when no CWD |
| TestBuildSystemPrompt_WithContextFiles | Context files loaded and prepended |
| TestBuildSystemPrompt_NonExistentFiles | Graceful fallback when no files exist |
| TestFollowUp_TriggersAdditionalTurn | Follow-up causes second provider call |
| TestSteering_TriggersAdditionalTurn | Steer causes second provider call |
| TestToolExecution_ErrorInResult | Tool error → error result in transcript |
| TestProviderError_PropagatesToCaller | Provider error returned to caller |
| TestMessages_ReturnsCopy | Deep copy — mutations don't affect internal state |

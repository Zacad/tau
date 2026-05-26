# Task 062: Interrupted Tool Call Continuation

## Why

During a `web-deep-research` run, the user interrupted execution after the assistant emitted parallel `subagent` tool calls. When the user continued, OpenAI Responses rejected the request with:

`Bad request: No tool output found for function call call_7L8TnEJBezrPauyOo9WiIr54.`

The conversation transcript could contain an assistant message with tool calls but no matching tool-result messages. Provider APIs such as OpenAI Responses require every function call in history to have a matching function call output before the next request.

## Comparison With PI and OpenCode

This is an agent-loop integrity issue rather than a skill design issue. The relevant reference pattern is that tool-call transcripts must remain provider-valid across cancellation, resume, and continuation.

OpenCode and PI both treat subagents/tool calls as part of the conversation contract. Tau must preserve the same invariant: once a tool call is appended to history, a corresponding result must also exist, even if the result is an interruption error.

## Main Constraints

- Fix must be minimal and localized to the agent loop.
- Do not discard the assistant tool-call message, because it may already be visible in the TUI and useful context.
- Do not wait indefinitely for in-flight tools after user interruption.
- Preserve provider validity for subsequent `continue` calls.

## Design

When cancellation happens during the tool-execution phase, append one synthetic error `tool_result` message for every in-flight tool call before returning the cancellation error.

The synthetic result text is `Tool execution interrupted: <error>`.

## Acceptance Criteria

- Interrupting during tool execution leaves no dangling assistant tool calls in memory.
- A subsequent continue has matching tool outputs for all prior tool calls.
- Regression test covers a tool that remains blocked after context cancellation.
- Targeted tests pass.
- Binary rebuild succeeds.

## Testing Strategy

- Add an agent-loop unit test for interruption during tool execution.
- Run `go test ./internal/agent`.
- Run targeted packages covering agent, SDK, provider, and tools.
- Rebuild `./tau`.

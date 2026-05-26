# Task 052: OpenAI Responses API Tool Call Fix

## Why
OpenAI models (via Responses API) stopped the agent loop immediately after emitting a thinking block instead of executing tools and continuing work. The model would say "I'll search for X", show thinking, then stop.

## Root Cause
1. **SSE event parsing mismatch**: Tau expected `response.output_item.added` data to have `id`, `type`, `name` at the top level. OpenAI actually nests these under an `item` key: `{"item": {"id": "fc_xxx", "type": "function_call", "call_id": "call_xxx", ...}}`.
2. **Missing call_id**: Tool results require `call_id` to match back to the original function call. Tau only stored the item ID.
3. **Incorrect tool result format**: Tool results were sent as plain user messages instead of proper `function_call_output` items required by the Responses API.

## What Changed
- `openAIOutputItem` struct now parses nested `item` field
- Tool call IDs use composite format `call_id|item_id`
- `messageToOpenAI()` returns proper Responses API input items:
  - User messages → `{"role": "user", "content": [{"type": "input_text", "text": "..."}]}`
  - Assistant text → `{"role": "assistant", "content": [{"type": "output_text", "text": "..."}]}`
  - Tool calls → `{"type": "function_call", "id": "...", "call_id": "...", "name": "...", "arguments": "..."}`
  - Tool results → `{"type": "function_call_output", "call_id": "...", "output": "..."}`

## Acceptance Criteria
- [x] Tool calls from OpenAI Responses API are correctly parsed from SSE stream
- [x] Tool results are sent back in proper Responses API format
- [x] Agent loop continues after tool execution (no premature stop)
- [x] All existing OpenAI provider tests pass
- [x] New regression tests for tool call streaming added

## Worklog
- 2026-05-24: Identified root cause by comparing Tau's SSE parsing against PI's `openai-responses-shared.ts`
- 2026-05-24: Fixed `openAIOutputItem` struct to handle nested `item` field
- 2026-05-24: Added `call_id` capture and composite tool call ID format
- 2026-05-24: Rewrote `messageToOpenAI()` to produce proper Responses API input items
- 2026-05-24: Updated existing tests in `zen_test.go` for new SSE format
- 2026-05-24: Added new regression tests for tool call streaming and message conversion
- 2026-05-24: All provider tests pass, binary rebuilt

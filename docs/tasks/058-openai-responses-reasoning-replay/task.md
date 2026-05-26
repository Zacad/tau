# Task 058: OpenAI Responses Reasoning Replay Fix

## Why
Subagent calls using OpenAI Responses models failed after a tool timeout with:

`Bad request: Item 'fc_...' of type 'function_call' was provided without its required 'reasoning' item: 'rs_...'`

This blocked the parent agent from continuing after a subagent/tool result was added to the transcript.

## Comparison Analysis

### PI
- PI uses provider SDK abstractions that preserve provider-specific reasoning/tool metadata more completely.
- The relevant lesson is that provider-native replay shape matters for reasoning models, especially after tool calls.

### OpenCode
- OpenCode includes `reasoning.encrypted_content` for OpenAI reasoning models and preserves reasoning parts through provider metadata when replaying messages.
- OpenCode also avoids replaying provider metadata across incompatible model/provider boundaries.

### Tau
- Tau stored OpenAI Responses tool calls as `call_id|fc_item_id` and replayed both pieces.
- Tau stored reasoning summaries as display text only, not as native `rs_...` reasoning items with encrypted content.
- Replaying `fc_...` without its paired `rs_...` item violates OpenAI Responses validation.

## Constraints
- Keep the fix minimal and localized to the OpenAI provider.
- Preserve `call_id`, because `function_call_output` requires it for tool result linkage.
- Do not introduce broad provider metadata schema changes unless required.
- Preserve existing agent loop, subagent, and session behavior.

## Design
When serializing a previous assistant tool call for OpenAI Responses follow-up requests, Tau now sends:

- `type: "function_call"`
- `call_id`
- `name`
- `arguments`

Tau does not replay the provider item `id` (`fc_...`). This prevents OpenAI from requiring a paired native reasoning item that Tau does not currently persist. Tool results continue to use the stripped `call_id`.

## Testing Strategy
- Add a focused unit regression test for `messageToOpenAI` assistant tool call serialization.
- Add a full request-building regression test covering assistant tool call plus tool result replay.
- Run targeted provider tests and full repository tests.
- Rebuild `./tau` for manual testing readiness.

## Acceptance Criteria
- [x] OpenAI Responses assistant tool call replay omits `function_call.id`.
- [x] OpenAI Responses assistant tool call replay preserves `call_id`.
- [x] OpenAI Responses tool result replay preserves matching `call_id`.
- [x] Regression tests cover the failing replay shape.
- [x] Documentation and decision log updated.

## Subtasks
- [x] Research OpenCode and PI handling of reasoning/tool replay.
- [x] Add failing regression tests.
- [x] Implement minimal OpenAI provider serialization fix.
- [x] Update architecture and decision documentation.
- [x] Run tests and rebuild binary.

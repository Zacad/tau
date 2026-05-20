# Task 044: Zen Provider — Anthropic & OpenAI Error Handling

## Why

Task 043 fixed Google Gemini schema compatibility, but manual verification revealed that Anthropic and OpenAI models via OpenCode Zen also fail with tool calling and have error handling issues. These need to be fixed for Zen to be fully functional.

## Root Cause

Learnings from Task 043 manual verification:

### Anthropic via Zen
- **Tool schema format**: `jsonschema.Schema` generates `$ref`/`$defs` format. Anthropic's API requires `type: "object"` at the top level of `input_schema`. Sending `$ref` results in: `tools.0.custom.input_schema.type: Field required`
- **Same root cause as Google**: `*jsonschema.Schema` struct passed directly without conversion to `map[string]any` and `$ref` inlining.

### OpenAI via Zen
- **`include: ["session_usage"]` not supported**: Zen's OpenAI endpoint rejects `session_usage` in the `include` field. Error: `Invalid value: 'session_usage'. Supported values are: 'file_search_call.results', ...`
- **Tool schema format**: OpenAI Responses API also expects `type: "object"` at top level in tool parameters, not `$ref`/`$defs`.

## Reference Implementation

### OpenCode (`~/Projects/opencode`)
- **Schema normalization**: `packages/opencode/src/provider/transform.ts` — `sanitizeGemini()` handles Google. Similar normalization needed for Anthropic and OpenAI.
- **Tool schema**: OpenCode's AI SDK providers handle schema normalization internally via `@ai-sdk/anthropic` and `@ai-sdk/openai`.

### PI (`~/Projects/pi`)
- **Model resolver**: `packages/coding-agent/src/core/model-resolver.ts`
- **Provider setup**: Per-provider factory functions mapping to AI SDK packages.

## Constraints
- Must not break direct Anthropic/OpenAI provider usage (non-Zen)
- Must preserve valid JSON Schema fields
- Must handle both `$ref`/`$defs` and direct schema formats

## Subtasks

- [ ] **044.1**: Fix Anthropic tool schema in `anthropic.go`
  - Marshal `*jsonschema.Schema` to JSON, unmarshal as `map[string]any`
  - Inline `$ref` from `$defs` (same approach as Google fix in Task 043)
  - Ensure `type: "object"` is present at top level

- [ ] **044.2**: Fix OpenAI tool schema in `openai.go`
  - Same marshal/unmarshal + `$ref` inlining approach
  - Ensure `type: "function"` is set on tool objects (OpenAI Responses API requirement)

- [ ] **044.3**: Fix OpenAI `include` field for Zen
  - Remove `session_usage` from `include` array (not supported by Zen)
  - Add provider-level config or BaseURL check to conditionally include it

- [ ] **044.4**: Tests
  - Unit test Anthropic schema sanitization with `*jsonschema.Schema`
  - Unit test OpenAI schema sanitization with `*jsonschema.Schema`
  - E2E test Claude via Zen with tool calling
  - E2E test GPT via Zen with tool calling

- [ ] **044.5**: Documentation
  - Update DECISIONS.md
  - Update TRACKING.md
  - Update worklog.md

## Acceptance Criteria
- [ ] `claude-sonnet-4-6` via `opencode-zen` works with tool calling
- [ ] `claude-opus-4-6` via `opencode-zen` works with tool calling
- [ ] `gpt-5.4` via `opencode-zen` works with tool calling
- [ ] `gpt-5.5` via `opencode-zen` works with tool calling
- [ ] All existing tests pass
- [ ] Binary rebuilds successfully
- [ ] Manually verified via `tau -p` against each model

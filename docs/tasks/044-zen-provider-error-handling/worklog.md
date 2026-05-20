# Worklog — Task 044: Zen Provider — Anthropic & OpenAI Error Handling

## Summary
Fix Anthropic and OpenAI models via OpenCode Zen to work correctly with tool calling and error handling.

## Root Cause Analysis (from Task 043 manual verification)

### Anthropic via Zen
- **Error**: `tools.0.custom.input_schema.type: Field required`
- **Cause**: `jsonschema.Schema` generates `$ref`/`$defs` format. Anthropic requires `type: "object"` at top level.
- **Verified**: `curl` with inlined schema works; with `$ref` fails.

### OpenAI via Zen
- **Error**: `Invalid value: 'session_usage'. Supported values are: 'file_search_call.results', ...`
- **Cause**: Zen's OpenAI endpoint doesn't support `session_usage` in `include` field.
- **Verified**: `curl` without `include` works; with `include: ["session_usage"]` fails.

## Implementation

### 044.1: Shared schema sanitization (`schema.go`)
- Created `sanitizeToolSchema()` function that:
  - Marshals `*jsonschema.Schema` to JSON, unmarshals as `map[string]any`
  - Strips meta fields: `$schema`, `$id`, `$ref`, `$defs`
  - Recursively inlines all `$ref` references from `$defs`
  - Ensures `type: "object"` at top level if properties exist
  - Filters `required` array to only include existing properties
- Created `toolDefToSchema()` helper for use by providers

### 044.2: Anthropic tool schema fix (`anthropic.go`)
- Changed `InputSchema: t.Parameters` to `InputSchema: toolDefToSchema(t)`
- This ensures `$ref` is inlined and `type: "object"` is present

### 044.3: OpenAI tool schema fix (`openai.go`)
- Changed `Parameters: t.Parameters` to `Parameters: toolDefToSchema(t)`
- Added `Type: "function"` to `openAITool` struct (Responses API requirement)
- Conditionally excludes `session_usage` from `include` field when BaseURL is not official OpenAI API

### 044.4: OpenAI-compat tool schema fix (`openai_compat.go`)
- Changed `Parameters: t.Parameters` to `Parameters: toolDefToSchema(t)`
- Ensures consistency across all OpenAI-compatible providers

### 044.5: Tests (`schema_test.go`)
- 13 new tests:
  - `TestSanitizeToolSchema_NilSchema`
  - `TestSanitizeToolSchema_RemovesMetaFields`
  - `TestSanitizeToolSchema_InlinesRefs`
  - `TestSanitizeToolSchema_AddsObjectType`
  - `TestSanitizeToolSchema_FiltersRequired`
  - `TestToolDefToSchema`
  - `TestOpenAIProvider_IncludeField_WithZenBaseURL`
  - `TestOpenAIProvider_IncludeField_WithOpenAIBaseURL`
  - `TestOpenAIProvider_IncludeField_WithEmptyBaseURL`
  - `TestOpenAIProvider_ToolSchemaSanitization`
  - `TestAnthropicProvider_ToolSchemaSanitization`
  - `TestInlineSchemaRefs_NestedRefs`
  - `TestInlineSchemaRefs_PreservesValidFields`

## Verification
- All tests pass: `go test ./...`
- `go vet ./...` clean
- `go build ./...` clean
- Binary rebuilt: `go build -o tau ./cmd/tau`

## Files Changed
- `internal/provider/schema.go` (new) — shared schema sanitization
- `internal/provider/schema_test.go` (new) — tests
- `internal/provider/anthropic.go` — use `toolDefToSchema()`
- `internal/provider/openai.go` — use `toolDefToSchema()`, add `type: "function"`, conditional `include`
- `internal/provider/openai_compat.go` — use `toolDefToSchema()`, DeepSeek `reasoning_content` handling
- `internal/provider/openai_compat_test.go` — DeepSeek tests
- `internal/sdk/sdk.go` — added Info-level logging for Zen model discovery count
- `docs/DECISIONS.md` — decision #34
- `docs/TRACKING.md` — task status update

## DeepSeek Fix (post-task discovery)

DeepSeek models on Zen require ALL assistant messages to have a `reasoning_content` field (even if empty). Without this, Zen returns "insufficient balance" error (misleading error message).

- Added `ReasoningContent *string` field to `openAICompatMessage`
- Detect DeepSeek models by ID (`strings.Contains(model.ID, "deepseek")`)
- Extract thinking content from `BlockThinking` blocks and populate `reasoning_content`
- Use pointer type to ensure empty string is serialized (DeepSeek requires the field to be present)
- Non-DeepSeek models are unaffected

Matches OpenCode's behavior in `transform.ts` lines 286-302.

### Manual Verification
```bash
./tau --print "Say hello in one word." --model deepseek-v4-flash-free
```
Response: `Hello` ✅

**Note**: Use `--print` or `-p="text"` syntax. The `-p text` form without `=` consumes the next flag as its value due to Go flag parsing behavior.

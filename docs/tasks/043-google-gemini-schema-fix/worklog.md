# Worklog — Task 043: Google Gemini JSON Schema Compatibility Fix

## Summary
Fixed Google Gemini API rejecting JSON Schema meta fields (`$schema`, `$id`, `$ref`, `$defs`) in tool function declaration parameters.

## Root Cause
The `github.com/invopop/jsonschema` library generates JSON Schema with meta fields that Google's API doesn't accept. When tools are passed to Gemini, the API returns HTTP 400 with errors like:
```
Invalid JSON payload received. Unknown name "$schema" at 'tools[0].function_declarations[0].parameters'
```

## Bug Fix (Post-Implementation Discovery)
During manual verification, two additional bugs were found:

### Bug 1: Type mismatch in `sanitizeGoogleSchema()`
`ToolDefinition.Parameters` is `*jsonschema.Schema` (struct pointer), but `sanitizeGoogleSchema()` expected `map[string]any`. The type assertion failed silently, returning the original unchanged — meta fields were never stripped.

**Fix**: Marshal `*jsonschema.Schema` to JSON, unmarshal as `map[string]any`, then sanitize.

### Bug 2: `$ref`/`$defs` not inlined
`jsonschema.Schema` generates schemas with `$ref` pointing to `$defs`. Stripping both left an empty object. Google doesn't support `$ref`.

**Fix**: Inline the `$defs` definition when a `$ref` is encountered.

### Bug 3: Content skipped when `FinishReason` present
Gemini sends text AND `finishReason` in the same SSE event. The parser checked `FinishReason` first and skipped content processing, resulting in empty responses.

**Fix**: Process content before checking `FinishReason`.

## Reference Implementation
**OpenCode** (`~/Projects/opencode`): `packages/opencode/src/provider/transform.ts` — `sanitizeGemini()` function
- Recursively processes schema objects
- Converts integer enums to string enums
- Filters `required` array to only include fields that exist in `properties`
- Removes `properties`/`required` from non-object types

## Changes

### 1. `sanitizeGoogleSchema()` function (`google.go`)
- Changed signature from `any` to `map[string]any`
- Inlines `$ref` definitions from `$defs` (Google doesn't support `$ref`)
- Recursively strips `$schema`, `$id`, `$ref`, `$defs` meta fields
- Converts integer/number enums to string enums (Google requirement)
- Filters `required` array to only include fields that exist in `properties`
- Removes `properties`/`required` from non-object types

### 2. Applied in `buildRequest()` (`google.go`)
- Tool parameters now marshaled from `*jsonschema.Schema` to JSON, unmarshaled as `map[string]any`, then passed through `sanitizeGoogleSchema()`

### 3. Fixed `parseStreamResponse()` (`google.go`)
- Content processing moved before `FinishReason` check to handle Gemini's single-event response format

### 4. Tests (`zen_test.go`)
- `TestSanitizeGoogleSchema_StripsMetaFields` — verifies meta fields are removed recursively
- `TestSanitizeGoogleSchema_IntegerEnumsToString` — verifies integer enum conversion
- `TestSanitizeGoogleSchema_FiltersRequired` — verifies required field filtering
- `TestSanitizeGoogleSchema_NilAndNonObject` — edge cases
- `TestSanitizeGoogleSchema_ArrayItems` — verifies nested schema sanitization in arrays
- `TestSanitizeGoogleSchema_InlinesRef` — verifies `$ref`/`$defs` inlining
- `TestGoogleProvider_SanitizesJsonSchema` — end-to-end test with real `*jsonschema.Schema`

### 5. Documentation
- Updated `TRACKING.md`
- Updated `AGENTS.md` with reference implementation locations (`~/Projects/opencode`, `~/Projects/pi`)

## Test Results
All tests pass. Binary rebuilt.

## Manual Verification
- `gemini-3.1-pro` via `opencode-zen`: basic chat ✅
- `gemini-3.1-pro` via `opencode-zen` with tool calling (web_search): ✅

## Notes
- This fix is specific to Google/Gemini provider — other providers (OpenAI, Anthropic, etc.) are unaffected
- The sanitization is applied at the point of building the request, not at schema generation time

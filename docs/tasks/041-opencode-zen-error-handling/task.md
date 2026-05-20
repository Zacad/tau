# Task 041: OpenCode Zen Provider Full Error Handling

## Why
OpenCode Zen uses the shared `OpenAICompatProvider` which received core streaming fixes in task 040. However, Zen-specific error handling gaps remain: error messages show generic "OpenAI-compatible" branding, no Zen-specific error pattern detection exists, connection test errors lack detail, and there's no E2E testing against the real Zen API.

## Comparison with PI
PI shows provider-specific error messages and handles provider-specific error formats. Tau should match this behavior for OpenCode Zen.

## Constraints
- Must not break other providers using `OpenAICompatProvider` (OpenRouter, Ollama, llama.cpp, LM Studio, OpenCode Go)
- Must maintain backward compatibility with existing error handling
- Zen API key required for E2E testing (use auth.json or env var)

## Design

### Problem 1: Generic Error Message Branding
`parseStreamResponse()` emits `"OpenAI-compatible stream error: ..."` — users see internal implementation details instead of the provider name.

**Fix**: Pass provider name to `parseStreamResponse()` and use it in error messages: `"OpenCode Zen stream error: ..."`.

### Problem 2: No Zen-Specific Error Pattern Detection
`ClassifyAPIError()` doesn't recognize Zen-specific error keywords (e.g., "zen", "opencode" specific messages).

**Fix**: Add Zen-specific keyword detection to improve error classification accuracy.

### Problem 3: Connection Test Error Handling
`testOpenAICompatZen()` returns generic errors without detailed classification.

**Fix**: Use `ClassifyAPIError()` for connection test errors, return typed `APIError` with `UserMessage()`.

### Problem 4: No E2E Testing Against Zen API
No integration tests verify Zen error handling with real API responses.

**Fix**: Add E2E test that connects to Zen API and verifies error handling (rate limit, auth failure, model unavailable).

## Subtasks

- [x] 041.1: Pass provider name to `parseStreamResponse()` — update signature and all call sites
- [x] 041.2: Update SSE error messages to use provider name instead of "OpenAI-compatible"
- [x] 041.4: Improve connection test error handling with `APIError` classification
- [x] 041.3: Add Zen-specific error pattern detection to `ClassifyAPIError()`
- [x] 041.5: Add E2E tests for Zen error scenarios (auth failure, model unavailable)
- [x] 041.6: Update all existing tests for new `parseStreamResponse` signature
- [x] 041.7: Documentation — update ARCHITECTURE.md and DECISIONS.md

## Acceptance Criteria
- [ ] Stream error messages show "OpenCode Zen" instead of "OpenAI-compatible"
- [ ] All other OpenAI-compatible providers show their own names in error messages
- [ ] Zen-specific error patterns are correctly classified
- [ ] Connection test returns detailed, user-friendly error messages
- [ ] E2E tests pass against real Zen API (or skip gracefully without key)
- [ ] All existing tests pass
- [ ] Binary rebuilds successfully
- [ ] Documentation updated

# Task 040: OpenCode Go Provider Error Handling

## Why
When the OpenCode Go weekly limit was reached, tau hung completely — no error message, no info, app totally frozen, couldn't abort or quit. The root cause was that the OpenAI-compatible provider buffered the entire HTTP response before parsing SSE events, and didn't handle SSE error events.

## Problem Analysis

### Root Cause 1: HTTP Response Buffering
`OpenAICompatProvider.Stream()` used `DefaultHTTPClient.Do()` which calls `io.ReadAll(resp.Body)`. For streaming SSE responses, this blocks until the server closes the connection. Context cancellation does NOT stop `io.ReadAll`, causing the app to hang indefinitely.

### Root Cause 2: Missing SSE Error Event Handling
`parseStreamResponse()` didn't handle `event: error` SSE events. When opencode-go sent error events (e.g., "Weekly limit reached"), they were silently discarded.

### Root Cause 3: Generic Error Messages
`APIError.UserMessage()` returned hardcoded generic messages for rate limit/quota errors, discarding the provider's detailed message with reset dates.

## Changes Made

### 1. `internal/provider/openai_compat.go` — Incremental SSE Streaming
- Rewrote `Stream()` to make direct HTTP requests using `net/http` with `http.NewRequestWithContext(ctx, ...)`
- Response body passed as `io.Reader` to `parseStreamResponse()` for incremental reading
- Uses `bufio.Scanner` to read SSE events line-by-line (matching Ollama provider pattern)
- Context cancellation now properly stops the scanner loop
- Added SSE `event: error` handling
- Error detection in JSON parse failures and empty-choices cases

### 2. `internal/provider/http.go` — Non-Retryable 429 Detection
- Added `isRetryableRateLimit()` to detect quota/weekly/monthly/daily limit errors from response body
- Returns immediately for non-retryable limits instead of wasting retries

### 3. `internal/types/errors.go` — Provider Message Preservation
- Updated `UserMessage()` to return the provider's original message when available
- Updated 429 classification to distinguish between transient rate limits and quota errors

### 4. Tests
- Updated `collectStreamEvents()` helper for new `parseStreamResponse` signature
- Converted mock-based Stream tests to use `httptest.NewServer`
- Added context cancellation test for streaming
- Added `TestAPIError_UserMessage_ReturnsProviderMessage`
- Updated quota classification tests

## Acceptance Criteria
- [x] Tau shows error message when OpenCode Go weekly limit is reached
- [x] App doesn't hang — context cancellation works during streaming
- [x] Provider's detailed error message (with reset date) is displayed
- [x] All existing tests pass
- [x] Binary rebuilds successfully

## Subtasks
- [x] 040.1: Fix HTTP streaming — replace io.ReadAll with incremental reading
- [x] 040.2: Add SSE error event handling to parseStreamResponse
- [x] 040.3: Handle JSON parse errors in SSE stream
- [x] 040.4: Detect non-retryable 429 errors in http.go
- [x] 040.5: Preserve provider error messages in UserMessage()
- [x] 040.6: Update tests for new streaming architecture
- [x] 040.7: Documentation

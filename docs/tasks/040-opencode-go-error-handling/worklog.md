# Worklog — Task 040: OpenCode Go Provider Error Handling

## Session 1: Root Cause Analysis & Initial Fixes

### Investigation
- Explored `openai_compat.go` parseStreamResponse — found missing SSE error event handling
- Compared with `anthropic.go:263-267` and `openai.go:218-222` which both handle `event: error`
- Found `http.go` retry logic retries all 429s including non-retryable quota limits
- Identified `io.ReadAll(resp.Body)` in `DefaultHTTPClient.Do()` as the hang source

### Initial Fixes Applied
1. Added SSE error event handling to `openai_compat.go`
2. Added error content detection in JSON parse failures
3. Added `isRetryableRateLimit()` in `http.go`
4. Wrote tests for new error handling

### User Feedback
- User reported app still hung after initial fixes
- Error message shown was generic "Rate limit reached" instead of provider's detailed message

## Session 2: Deep Dive — HTTP Streaming Architecture

### Root Cause Identified
- `DefaultHTTPClient.Do()` uses `io.ReadAll(resp.Body)` which blocks until EOF
- For streaming SSE, this waits for server to close connection (up to 5-minute timeout)
- Context cancellation does NOT stop `io.ReadAll`
- Ollama provider avoids this by using `bufio.Scanner` on `resp.Body` directly

### Architecture Change
- Rewrote `OpenAICompatProvider.Stream()` to make direct HTTP requests (like Ollama)
- Changed `parseStreamResponse()` to accept `io.Reader` instead of `[]byte`
- Uses `bufio.Scanner` for incremental line-by-line SSE parsing
- Context cancellation now properly stops the scanner loop

### Test Updates
- Updated `collectStreamEvents()` helper for new signature
- Converted mock-based tests to `httptest.NewServer`
- Added context cancellation test

## Session 3: Error Message Quality

### Issue
- `APIError.UserMessage()` returned hardcoded generic messages
- Provider's detailed message (with reset date) was discarded

### Fix
- Updated `UserMessage()` to return provider's original message when available
- Updated 429 classification to distinguish rate limits from quota errors
- Added tests for provider message preservation

### Final State
- All tests pass (14 packages)
- Binary rebuilt successfully
- Documentation created

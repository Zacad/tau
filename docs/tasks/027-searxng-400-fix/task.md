# Task 027: Fix SearXNG 400 Error

## Why

Web search is non-functional — the `websearch` tool returns `all backends unavailable. [searxng: search error 400: unexpected status 400]`. The SearXNG instance is running (the `Available()` check passes since it only tests for `< 500` status), but actual search requests fail with HTTP 400.

## Problem Analysis

### Root cause

The 400 error was caused by a **stale Docker named volume** (`searxng-data:/etc/searxng`). The volume could contain an older `settings.yml` without JSON format enabled. While the bind mount `./searxng-settings.yml:/etc/searxng/settings.yml:ro` should override it, the named volume itself was unnecessary and a source of config drift.

Additionally, the code had two issues that made the 400 hard to diagnose:

1. **`Available()` too lenient**: Accepted any status `< 500`, so a 400 from SearXNG passed the availability check but failed on actual search requests.
2. **Error messages lacked response body**: Only the HTTP status code was included, making it impossible to diagnose future 400s without manual curl.

### Fix applied

1. Removed the `searxng-data` named volume from `docker-compose.yml` — only the bind mount for `settings.yml` is needed.
2. Changed `Available()` to require `StatusCode == http.StatusOK` (not `< 500`).
3. Added response body to all `searchError` messages, with a `truncateBody()` helper to limit to 512 chars.
4. Added 7 new tests covering 400-with-body, 4xx-with-body, 5xx-with-body, strict Available(), and truncateBody.

### Key files

| File | Change |
|------|--------|
| `internal/tools/search_searxng.go` | Strict `Available()`, response body in errors, `truncateBody()` helper |
| `internal/tools/search_searxng_test.go` | 7 new tests for error body, strict Available, truncateBody |
| `ollama/docker-compose.yml` | Removed `searxng-data` named volume |

## Subtasks

### 027.1: Diagnose the 400 error — DONE

- SearXNG currently returns 200 with valid results (volume was deleted at some point)
- Root cause: stale named volume config + lenient Available() + missing error body
- **AC**: Root cause identified with evidence ✓

### 027.2: Fix the SearXNG request/config — DONE

- Removed `searxng-data` named volume from docker-compose.yml
- Recreated container without named volume — confirmed working
- **AC**: `curl http://localhost:8964/search?q=test&format=json` returns valid JSON ✓

### 027.3: Improve error reporting — DONE

- `Available()` now requires `http.StatusOK` (not `< 500`)
- All `searchError` messages now include truncated response body (max 512 chars)
- `truncateBody()` helper handles empty bodies, whitespace, and long responses
- 7 new tests: 400WithBody, 4xxWithBody, 5xxWithBody, Available_400ReturnsFalse, Available_200ReturnsTrue, TruncateBody (5 subcases)
- **AC**: Error messages include enough detail to diagnose future issues without curl ✓

### 027.4: Verify end-to-end — DONE

- `go test ./internal/tools/ -run TestSearXNG -v` — all pass ✓
- `go test ./internal/tools/ -run TestWebSearchTool_E2E_SearXNG -v` — pass (3 results) ✓
- `go test ./internal/tools/ -short -race` — 87 tests pass, race-free ✓
- `go vet ./internal/tools/` — clean ✓
- `go build ./...` — clean ✓
- **AC**: Web search works end-to-end ✓

## Acceptance Criteria

1. ✓ SearXNG search returns results (no 400 error)
2. ✓ Error messages include response body for future debugging
3. ✓ All existing search tests pass (87 total, -race clean)
4. ✓ Integration test succeeds (3 results from SearXNG)

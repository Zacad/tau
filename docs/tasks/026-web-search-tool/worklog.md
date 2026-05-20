# Task 026 Worklog: Web Search Tool

## 2026-05-05 — Implementation

### 026.1 — Search backend abstraction
- Defined `SearchBackend` interface: `Name()`, `Available()`, `Search()`
- Defined `SearchResult`, `SearchOptions`, `WebSearchParams` structs
- Implemented `WebSearchTool` with:
  - Dynamic date injection in description
  - Tool naming enforcement ("Do not use aliases like google_search")
  - Integrity rules ("Always cite URLs", "Never fabricate")
  - Runtime fallback: try backends in priority order, skip degraded
  - Degradation tracking: 401/403 marks backend as degraded for session
  - `isRetryableHTTPCode()` for error classification
- Tests: 28 unit tests covering fallback, degradation, empty results, cancellation, format

### 026.4a — SearXNG Docker Compose
- Added SearXNG service to `ollama/docker-compose.yml`
- Port: 8964:8080 (avoids 8888 conflicts)
- Settings file: `ollama/searxng-settings.yml` (JSON API enabled, limiter disabled)
- Named volume `tau-searxng-data` for persistence
- Health check via `/healthz`
- Updated AGENTS.md with SearXNG quick commands and config docs
- Verified: SearXNG returns search results for "Go programming"

### 026.5 — WebFetchTool
- HTTP GET with `tau-agent` User-Agent
- HTML→Markdown via `github.com/JohannesKaufmann/html-to-markdown`
- Strips `<style>` and `<script>` before conversion
- SSRF protection: DNS resolution + private IP blocking (RFC 1918, loopback, link-local)
- Redirect validation: blocks redirects to private IPs
- Response size limits: 5MB max, 2MB HTML truncation before conversion
- Binary content detection: returns metadata instead of raw bytes
- Format options: markdown (default), text, html
- Timeout: default 30s, max 120s
- `NewWebFetchToolForTest()` skips SSRF for httptest (127.0.0.1) testing
- Tests: 17 unit tests including SSRF, conversion, error handling, size limits

### 026.2 — Tavily backend
- POST to `https://api.tavily.com/search`
- API key resolution via existing auth chain (`tavily` / `TAVILY_API_KEY`)
- Returns titles, URLs, content (optional), scores
- Error classification: 401/403 non-retryable, 429 retryable, 5xx retryable
- Tests: 10 unit tests with mock HTTP server

### 026.3 — Brave backend
- GET to `https://api.search.brave.com/res/v1/web/search`
- API key via `X-Subscription-Token` header
- Auth resolution: `brave` / `BRAVE_API_KEY`
- Returns titles, URLs, descriptions (no inline content)
- Same error classification as Tavily
- Tests: 9 unit tests with mock HTTP server

### 026.4 — SearXNG backend
- GET to `{baseURL}/search?q=...&format=json`
- `Available()`: checks `/healthz` then `/` with 2s timeout
- Default URL: `http://localhost:8964`
- No API key required
- Returns titles, URLs, snippets
- Tests: 8 unit tests + 1 integration test with real SearXNG

### 026.6 — Tool descriptions
- Already implemented within tool structs
- websearch: dynamic date, naming enforcement, 2 integrity rules
- webfetch: usage guidance, format info, truncation notice

### 026.7 — SDK/config/auth integration
- Added `SearchConfig` to `internal/config/config.go` (`backend`, `searxng_url`)
- Updated `registerBuiltinTools()` to accept `*config.Config`
- New `registerSearchTools()` function:
  - Discovers SearXNG (health check), Tavily key, Brave key
  - Logs availability at startup
  - Creates backends in priority order
  - Supports `search.backend` config override via `reorderBackends()`
  - Registers `websearch` only if backends available
  - Always registers `webfetch`
- Auth keys: `tavily`/`TAVILY_API_KEY`, `brave`/`BRAVE_API_KEY` via existing 4-step chain
- Updated all test callers of `registerBuiltinTools`

### 026.8 — Documentation
- Updated ARCHITECTURE.md: package layout, tool parallelism, config format, dependencies
- Added 4 decisions to DECISIONS.md (#20-#23)
- Updated TRACKING.md

### E2E Verification

#### SearXNG Backend (real instance at localhost:8964)
- `websearch` returns real results (Go Programming Language, Wikipedia, Coursera)
- `webfetch` fetches go.dev and converts HTML→markdown successfully
- Fallback chain: unreachable backend → SearXNG fallback works correctly
- SSRF: all private IPs blocked (127.0.0.1, localhost, 10.0.0.1, 169.254.169.254)
- Registration: `NewWebSearchTool(nil)` returns nil → tool not registered

#### CLI E2E (gemma4:26b via Ollama native chat API)
- Tool definitions correctly include `websearch` and `webfetch` (9 tools total)
- Startup logging shows: SearXNG reachable, available backends [searxng]
- gemma4:26b does not reliably call tools via native Ollama chat API — this is a pre-existing model behavior limitation, not a search tool bug
- Tools are correctly registered and definitions are well-formed JSON Schema

### Follow-up: Partial results support
- Added `PartialResultError` type carrying both `[]SearchResult` and `error`
- WebSearchTool fallback loop: if backend returns `PartialResultError` with results, return them with "Warning: search was incomplete" appended
- Empty partial results fall through to normal error handling / next backend
- Tests: partial results returned with warning, empty partial falls through, partial takes priority over fallback, `Error()`/`Unwrap()` methods

### Test Summary
- 81.1% coverage for `internal/tools/` package
- All core functions at 80%+ coverage
- `go vet`, `go build`, `go test -race ./...` all clean
- Unit tests: 72+ tests across search.go, search_tavily.go, search_brave.go, search_searxng.go, webfetch.go
- Integration test: real SearXNG at localhost:8964
- E2E tests: 5 tests in search_e2e_test.go (SearXNG search, webfetch, SSRF, fallback chain, registration)

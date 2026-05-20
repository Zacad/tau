# Task 026: Web Search Tool

## Why

A coding agent without web search is limited to its training data cutoff. Search capabilities are critical for:
- Finding up-to-date documentation, API references, and library versions
- Researching current best practices and security advisories
- Investigating bugs and error messages with fresh context
- Enabling the Researcher subagent to gather real information from the web

The agent must be able to discover and use this tool reliably — it needs to "just work" and be highly visible to the LLM through clear tool descriptions.

## Comparison Analysis: Web Search Across Code Agents

### PI (pi-ai)
| Dimension | PI Approach |
|-----------|-------------|
| Built-in web search | **No** — no web search tool in core |
| Extension mechanism | MCP (Model Context Protocol) — tools added via MCP servers |
| Search via extensions | Brave Search MCP, Exa MCP, custom MCP servers |
| Tool discovery | MCP `tools/list` → JSON Schema definitions exposed to LLM |
| Auth | Per-MCP-server API keys, configured in extension settings |

### OpenCode (anomalyco/opencode)
| Dimension | OpenCode Approach |
|-----------|-------------------|
| `websearch` tool | Uses **Exa AI** via hosted MCP endpoint (`https://mcp.exa.ai/mcp`) |
| `webfetch` tool | HTTP GET → HTML→Markdown conversion, always available, no API key |
| Parameters (search) | `query`, `numResults` (default 8), `type` (auto/fast/deep), `livecrawl` |
| Parameters (fetch) | `url`, `format` (markdown/text/html), `timeout` (max 120s) |
| Availability gating | websearch only with OpenCode provider OR `OPENCODE_ENABLE_EXA=1` |
| Content extraction | TurndownService for HTML→Markdown, 5MB response limit |
| Permission model | `ctx.ask({ permission: "websearch/webfetch", patterns: [...] })` |
| LLM description | Dynamic template: "The current year is {{year}}" injected into description |

### Claude Code (Anthropic)
| Dimension | Claude Code Approach |
|-----------|---------------------|
| `WebSearch` tool | **Server-side** — delegates to Anthropic API's `web_search` server tool. API executes searches and returns results with `encrypted_content`, titles, URLs, `page_age` |
| `WebFetch` tool | **Client-side** — Claude Code fetches URL from user's machine, converts HTML→markdown. Identifies as `Claude-User` UA |
| Search execution | Anthropic API handles search infrastructure. Results include encrypted content that only the API can decrypt |
| Parameters (search) | `max_uses` (limit per request), `allowed_domains`, `blocked_domains`, `user_location` (localization) |
| Parameters (fetch) | URL + domain-specific permission rules (e.g., `WebFetch(domain:docs.python.org)`) |
| Content extraction | WebSearch: encrypted content + citations with `cited_text`. WebFetch: HTML→markdown, strips `<style>`/`<script>`, 5MB limit |
| Pricing | $10/1K searches + token costs (WebSearch). WebFetch = standard token costs only |
| Safety | WebSearch: server-side domain filtering. WebFetch: hostname preflight check to `api.anthropic.com` (cached 5 min) |
| Availability | Available by default, permission-based (`allow`/`deny`/`ask` per domain) |
| Subagent support | Fixed: "deferred tools (WebSearch, WebFetch, etc.) not being available to skills with `context: fork`" |
| Key insight | **Server-side search** means no API key management for users, but locks to Anthropic provider. The domain permission model (`WebFetch(domain:...)`) is elegant for security |

### Feynman (getcompanion-ai/feynman)
| Dimension | Feynman Approach |
|-----------|-----------------|
| Architecture | Built on **Pi runtime** (`@mariozechner/pi-coding-agent`) + **alphaXiv** (academic papers) |
| Search implementation | **Delegates to Pi's `pi-web-access` package** — not implemented directly |
| Search tools | `web_search`, `fetch_content`, `get_search_content` (from `pi-web-access`) |
| Search backends | Exa, Perplexity, Gemini (auto-fallback: Exa → Perplexity → Gemini) |
| Configuration | `feynman search set <auto|perplexity|exa|gemini> [api-key]`, stored in `~/.feynman/web-search.json` |
| Academic search | `alpha_search` (papers), `alpha_get_paper` (full text/sections), `alpha_ask_paper` (Q&A on PDF), `alpha_read_code` (paper repos) |
| Deep research pattern | 7-step pipeline: Plan → Scale Decision → Gather Evidence → Draft → Cite → Review → Deliver |
| Scale-adaptive | Direct search (3-10 tool calls) for simple questions; 2-6 researcher subagents for complex |
| Integrity enforcement | "6 Integrity Commandments": never fabricate, URL-or-it-didn't-happen, never extrapolate unread content |
| Subagent types | Researcher (web_search + fetch), Verifier (citations + URL checks), Reviewer (adversarial audit), Writer (no search) |
| Forced on-disk verification | After claiming any fix, must verify with `rg`/`grep`/`diff`/`stat` that change actually landed |
| Provenance sidecars | Every deliverable gets `.provenance.md` tracking sources consulted/accepted/rejected |
| Key insight | **Integrity-first search**: explicit tool naming enforcement (prevents LLM hallucination of non-existent tools like `google_search`), scale-adaptive orchestration (don't inflate simple questions), context hygiene for subagents (write to file progressively, return lightweight summary) |

### Deep Research Agents (dzhng/deep-research, gpt-researcher)
| Dimension | Deep Research Approach |
|-----------|----------------------|
| Pattern | Multi-step recursive: query → search → extract learnings → follow-up questions → recurse |
| Search backends (gpt-researcher) | 15 pluggable: Tavily (primary), Google, Bing, DuckDuckGo, Serper, Exa, SearxNG |
| Content extraction | Firecrawl (search+scrape in one call) or Tavily (returns cleaned content directly) |
| Architecture | Plan-and-Solve: planner generates questions → execution agents crawl → publisher synthesizes |
| Breadth/depth | Breadth halves at each depth level, parallel processing with concurrency limits |

### Other Code Agents
| Agent | Web Search? | Implementation |
|-------|-------------|----------------|
| Aider | No (only URL scraping via `/web`) | HTTP fetch, no search API |
| Cline | Yes (browser automation) | Puppeteer-based, not API-driven |
| Cursor/Windsurf | Yes (proprietary) | Closed-source implementations |
| Goose | Yes (MCP extensions) | Brave/Tavily via MCP |
| SWE-agent | No | No web access at all |
| Continue | Extensible via MCP | No default search API |

### Search API Comparison

| API | Pricing | Free Tier | Content Extraction | Go SDK | Result Quality | Notes |
|-----|---------|-----------|-------------------|--------|---------------|-------|
| **Tavily** | $0.005/search | 1K searches/mo | Built-in (returns markdown) | No (REST) | High (AI-optimized) | Purpose-built for AI agents, returns relevance scores |
| **Brave Search** | $0.003/query | 2K queries/mo | No (metadata only) | **Yes** (official) | High | Independent index, official MCP server exists |
| **Serper** | $0.004/query | 2.5K queries/mo | No (metadata only) | No (REST) | High (Google results) | Fast, Google SERP results |
| **Exa AI** | $0.001/search | 1K searches/mo | Built-in (neural search) | No (MCP/REST) | Very High | Neural/semantic search, OpenCode uses this |
| **Google CSE** | $5/1000 queries | 100 queries/day | No | No (REST) | Highest | Google index, complex setup |
| **SearXNG** | Free (self-hosted) | Unlimited | No | No | Variable | Matches local Ollama philosophy, zero cost |

### Key Findings

1. **Two distinct tools needed**: `websearch` (query → results) and `webfetch` (URL → content). This is the universal pattern — OpenCode, Claude Code, and Feynman all implement both.
2. **Server-side vs client-side search**: Claude Code's approach (Anthropic API handles search) is elegant but provider-locked. Our multi-provider architecture requires client-side search with pluggable backends — closer to OpenCode/Feynman's approach.
3. **Tavily is the best primary backend**: Returns cleaned content directly (no separate scraping step), AI-optimized results with relevance scoring, generous free tier, purpose-built for agents.
4. **Brave Search is the best secondary**: Independent index, official Go SDK, official MCP server, good free tier.
5. **SearXNG for self-hosted**: Zero cost, matches the project's local-first philosophy (like Ollama for LLMs), can be run alongside Ollama in Docker.
6. **Content extraction matters**: Search APIs that return just titles/snippets are insufficient. Tavily and Exa solve this; others need a companion `webfetch`. Claude Code uses encrypted content from its API.
7. **Domain-level permission model**: Claude Code's `WebFetch(domain:...)` permission rules are elegant. We should support domain-based allow/deny for webfetch.
8. **Tool naming enforcement**: Feynman explicitly forbids LLM from using non-existent tool aliases (`google_search`, `search_google`). Tool description must name the exact tool and discourage alternatives.
9. **Integrity-first search**: Feynman's "6 Integrity Commandments" are valuable — URL-or-it-didn't-happen, never fabricate, never extrapolate unread content. These should inform our tool descriptions.
10. **Deep research is a higher-order pattern**: Multi-step search with recursive exploration (Feynman's 7-step pipeline, gpt-researcher's Plan-and-Solve) should be built on top of basic search/fetch tools, as a skill or subagent behavior.
11. **LLM discoverability is critical**: Tool description must clearly instruct the LLM when and how to use it. OpenCode injects current year; Feynman enforces exact tool names.
12. **Availability gating**: OpenCode gates websearch behind a provider flag. Claude Code makes tools available by default with permissions. We should make search work out of the box when an API key is configured, and degrade gracefully when not available.
13. **Safety preflight**: Claude Code's hostname safety check (cached 5 min) and SSRF prevention are important patterns. Our webfetch must validate URLs and block private IPs.
14. **Subagent search access**: Both Claude Code and Feynman give researcher/verifier subagents access to search tools. Our Researcher subagent must include websearch + webfetch in its tool set.

## Main Constraints

- Must follow existing `Tool` interface in `internal/tools/tool.go` — `Name()`, `Description()`, `Parameters()`, `ExecutionMode()`, `Execute()`
- Must use Go struct parameters with `jsonschema` tags for JSON Schema generation (existing pattern)
- Must be a leaf package — only depends on `internal/types`, stdlib, and `html-to-markdown` (one justified new dependency)
- Must work with the Registry's execution mode system — `websearch` and `webfetch` are both `ExecutionParallel`
- Must handle missing API keys gracefully — tool should not register if no backend is available
- Must not introduce heavy dependencies (no headless browser, no CGo, no MCP client)
- Must truncate results to fit within context window (existing `Truncate()` utility)
- Must work alongside existing tools without conflicts
- Must be provider-agnostic — search works regardless of which LLM provider is active
- Must support self-hosted option (SearXNG) for local-first operation

See `analysis.md` for the full pattern evaluation and decision rationale.

## Design Drivers

1. **Reliability**: The tool must work consistently. Search API failures should return clear errors, not crash the agent.
2. **Discoverability**: The LLM must understand when and how to use search. Tool descriptions must name the exact tool, discourage aliases (Feynman pattern), and include context like current date (OpenCode pattern).
3. **Integrity**: Results must be source-grounded. Tool descriptions should instruct the LLM to cite URLs, never fabricate results, and verify before claiming (Feynman's "Integrity Commandments").
4. **Configurability**: Multiple search backends with auto-fallback (Feynman: Exa → Perplexity → Gemini). API keys via existing auth resolution chain.
5. **Graceful degradation at every layer**: See "Graceful Fallback Specification" below — every failure mode has a defined, safe outcome. The agent never crashes, never hangs, and never presents fabricated results due to a search failure.
6. **Content quality**: Results should include actual page content, not just titles and snippets. This is what makes search useful for a coding agent.
7. **Security**: SSRF protection via IP validation, response size limits, content sanitization (strip `<script>`/`<style>`).

## Graceful Fallback Specification

Every possible failure scenario for websearch and webfetch must have a defined, tested outcome. This is the contract:

### websearch — Backend Availability

| Scenario | Startup Behavior | Runtime Behavior |
|----------|-----------------|------------------|
| No API keys + SearXNG not running | Do NOT register `websearch` tool | N/A (tool doesn't exist) |
| Only SearXNG running | Register tool, SearXNG as sole backend | If SearXNG fails → return error to LLM |
| Only Tavily key configured | Register tool, Tavily as sole backend | If Tavily fails → return error to LLM |
| Only Brave key configured | Register tool, Brave as sole backend | If Brave fails → return error to LLM |
| Multiple backends available | Register tool, priority: SearXNG → Tavily → Brave | Fallback to next on failure (see below) |

### websearch — Runtime Failure Handling

| Failure Type | Detection | Fallback Behavior | Error to LLM (if all fail) |
|-------------|-----------|-------------------|---------------------------|
| Network error (connection refused, timeout) | `net/http` error, context deadline | Try next backend in priority order | "Web search failed: all backends unavailable. [Tavily: connection refused, Brave: no API key]" |
| HTTP 401/403 (invalid/expired API key) | Status code check | Try next backend. Log warning about bad key. | "Web search failed: authentication error. Check your API keys." |
| HTTP 429 (rate limited) | Status code check | Try next backend. If all rate-limited, return error with Retry-After hint. | "Web search failed: rate limited. Try again in a moment." |
| HTTP 5xx (backend server error) | Status code check | Try next backend | "Web search failed: server error from all backends." |
| Malformed JSON response | `json.Unmarshal` error | Try next backend. Log raw response for debugging. | "Web search failed: unexpected response format." |
| Empty results (success but no matches) | Zero-length results array | Do NOT fallback — empty results is a valid response | Return empty results with "No results found for query." |
| Partial results (got some, then error) | Error mid-stream | Return whatever results were collected before the error. Append warning. | Results + "Warning: search was incomplete. Some backends failed." |
| Context cancellation | `ctx.Done()` | Return immediately with partial results (if any) or error | "Web search cancelled." |
| Backend took too long | Per-backend timeout (10s default) | Cancel request, try next backend | (falls through to next backend) |

### websearch — Backend Priority & Availability Rules

```
At session startup:
  backends = []
  if SearXNG reachable (GET /healthz or GET / with 2s timeout):
    backends = append(backends, searxng)
  if TAVILY_API_KEY resolved (auth chain):
    backends = append(backends, tavily)
  if BRAVE_API_KEY resolved (auth chain):
    backends = append(backends, brave)
  if user configured a preferred backend in config.json:
    reorder backends so preferred is first
  
  if len(backends) == 0:
    do NOT register websearch tool
    log: "websearch: no search backend available. Configure TAVILY_API_KEY, BRAVE_API_KEY, or run SearXNG."
  else:
    register websearch tool with backends list
    log: "websearch: available backends [searxng, tavily]"

At search execution time:
  for each backend in backends:
    result, err = backend.Search(ctx, query, opts)
    if err == nil:
      return result
    if err is retryable (network, timeout, 5xx):
      log: "websearch: backend %s failed: %v, trying next", backend.Name(), err
      continue
    if err is non-retryable (401, bad key):
      log: "websearch: backend %s auth error: %v, skipping permanently", backend.Name(), err
      mark backend as degraded for this session
      continue
  return error to LLM with summary of all failures
```

### webfetch — Failure Handling

webfetch has no backend abstraction — it makes direct HTTP requests. Failure handling:

| Failure Type | Detection | Behavior |
|-------------|-----------|----------|
| SSRF (private IP, localhost, metadata endpoint) | IP validation before request | Return error: "URL blocked: resolves to private/reserved IP address" |
| Invalid URL scheme (not http/https) | URL parse check | Return error: "URL must start with http:// or https://" |
| DNS resolution failure | `net/http` error | Return error: "Failed to resolve hostname: <domain>" |
| Connection refused | `net/http` error | Return error: "Connection refused: <url>" |
| HTTP 403 (blocked by site) | Status code | Return error: "Access denied by <domain>. The site may block automated requests." |
| HTTP 404 | Status code | Return error: "Page not found: <url>" |
| HTTP 429 | Status code | Return error: "Rate limited by <domain>. Try again later." |
| HTTP 5xx | Status code | Return error: "Server error at <domain> (HTTP <code>)" |
| Redirect to private IP | Per-connection IP validation in Transport | Block redirect, return error: "Redirect to private IP blocked" |
| Response too large (>5MB) | Byte counter during read | Truncate at 5MB, append "[Content truncated at 5MB]" |
| HTML too large before conversion | Pre-conversion size check | Truncate HTML at 2MB before markdown conversion |
| HTML→markdown conversion error | Library error | Fall back to raw text (strip tags with regex) |
| Timeout | Context deadline | Return error: "Request timed out after <timeout>s" |
| Context cancellation | `ctx.Done()` | Return immediately with partial content (if any) |
| Binary response (PDF, image, etc.) | Content-Type header | Return metadata: "Binary content (application/pdf, <size> bytes). URL: <url>" |

### Startup Logging

At session start, log the search availability status so the user knows exactly what's available:

```
websearch: SearXNG not reachable at http://localhost:8964
websearch: Tavily API key found
websearch: Brave API key not configured
websearch: available backends [tavily]
webfetch: always available (no API key required)
```

Or when everything works:

```
websearch: SearXNG reachable at http://localhost:8964
websearch: Tavily API key found
websearch: Brave API key found
websearch: available backends [searxng, tavily, brave]
webfetch: always available (no API key required)
```

Or when nothing is available:

```
websearch: SearXNG not reachable at http://localhost:8964
websearch: Tavily API key not configured
websearch: Brave API key not configured
websearch: NO backends available — tool disabled. Configure an API key or start SearXNG.
webfetch: always available (no API key required)
```

## Subtasks

### 026.1 — Search backend abstraction (`internal/tools/search.go`)
- Define `SearchBackend` interface: `Name() string`, `Available() bool`, `Search(ctx, query, opts) ([]SearchResult, error)`
- Define `SearchResult` struct: `Title`, `URL`, `Snippet`, `Content` (optional), `Score` (optional)
- Define `SearchOptions` struct: `MaxResults`, `IncludeContent`
- Define `WebSearchTool` struct implementing `Tool` interface with dynamic description (date injection)
- `NewWebSearchTool(backends []SearchBackend, date string) *WebSearchTool` — returns nil if no backends
- Backend priority: SearXNG (if reachable) → Tavily (if key) → Brave (if key), configurable override in config
- Runtime fallback: on backend failure, try next in priority order (see Graceful Fallback Specification)
- Backend health tracking: mark backends as `degraded` after auth errors (401/403) — skip for rest of session
- Per-backend timeout: 10s default, configurable
- Partial results: if error occurs after collecting some results, return collected + warning
- If `NewWebSearchTool` returns nil (no backends), SDK does NOT register the tool
- Startup availability logging (see Graceful Fallback Specification)

### 026.2 — Tavily search backend (`internal/tools/search_tavily.go`)
- Implement `SearchBackend` for Tavily API (`https://api.tavily.com/search`)
- `Available()`: returns true if API key resolved via auth chain
- Parameters: query, max_results, include_raw_content, search_depth
- Returns: titles, URLs, content (if `IncludeContent`), relevance scores
- API key resolution via existing auth chain (`tavily` in auth.json / `TAVILY_API_KEY` env)
- Error classification: 401/403 = non-retryable (degrade), 429 = retryable (fallback), 5xx = retryable (fallback)

### 026.3 — Brave search backend (`internal/tools/search_brave.go`)
- Implement `SearchBackend` for Brave Search API (`https://api.search.brave.com/res/v1/web/search`)
- `Available()`: returns true if API key resolved via auth chain
- Returns: titles, URLs, descriptions (no inline content — use webfetch separately)
- API key resolution via existing auth chain (`brave` in auth.json / `BRAVE_API_KEY` env)
- Error classification: 401/403 = non-retryable (degrade), 429 = retryable (fallback), 5xx = retryable (fallback)

### 026.4 — SearXNG search backend (`internal/tools/search_searxng.go`)
- Implement `SearchBackend` for SearXNG API (self-hosted, no API key)
- `Available()`: HTTP GET to base URL with 2s timeout — returns true if 2xx
- Configurable base URL (default: `http://localhost:8964`)
- Returns: titles, URLs, snippets
- Error classification: connection refused = retryable (fallback), timeout = retryable (fallback)

### 026.4a — SearXNG Docker Compose (`ollama/docker-compose.yml`)
- Add SearXNG service to existing `ollama/docker-compose.yml` (alongside Ollama)
- Image: `searxng/searxng:latest`
- Port mapping: `8964:8080` (host 8964 → container 8080, avoids common conflicts with 8888)
- Named volume `tau-searxng-data` for persistent config
- Configure SearXNG for JSON API access (settings.yml with `formats: [html, json]`)
- Health check endpoint available at `/healthz`
- Update AGENTS.md quick commands section with SearXNG start/stop/logs commands

### 026.5 — WebFetch tool (`internal/tools/webfetch.go`)
- Implement `WebFetchTool` struct implementing `Tool` interface
- HTTP GET with `tau-agent` User-Agent (for robots.txt allowlisting)
- HTML → Markdown via `github.com/JohannesKaufmann/html-to-markdown`
- Strip `<style>` and `<script>` contents before conversion
- Truncate very large HTML before conversion to prevent hangs
- Response size limit (5MB), pre-conversion limit (2MB)
- SSRF protection: resolve hostname, block private/loopback/link-local IPs
- Custom `http.Transport` with per-connection IP validation (DNS rebinding defense)
- Redirect handling: validate redirect target IP too, block redirects to private IPs
- Format options: markdown (default), text, html
- Timeout with context cancellation (default 30s, max 120s)
- Binary content detection: return metadata instead of raw bytes for PDFs, images, etc.
- No API key required — always registered when websearch is available OR independently

### 026.6 — Tool descriptions and LLM discoverability
- Dynamic date injection in websearch description (OpenCode pattern)
- Tool naming enforcement: "Use websearch. Do not use aliases like google_search, search_web" (Feynman pattern)
- Integrity instructions: "Always cite URLs. Never fabricate results." (Feynman pattern, 2-3 rules only)
- Clear when-to-use guidance: websearch for discovery, webfetch for known URLs
- Ensure tools appear naturally in agent's tool list

### 026.7 — Integration with existing systems
- Register `websearch` and `webfetch` in SDK session creation (only if backend available)
- Add to Researcher subagent's default tool set
- Add API key slots to auth resolution chain (`tavily`, `brave`)
- Add `search` section to config struct (`backend`, `searxng_url`)
- Log search backend availability at startup
- Update ARCHITECTURE.md with new tools
- Update DECISIONS.md with search backend choices

### 026.8 — Comprehensive tests
- Unit tests for each search backend (mock HTTP server via `httptest`)
- Unit tests for webfetch (mock HTTP server, various content types)
- Integration tests with SearXNG (Docker-based)
- Edge cases: network errors, timeouts, malformed responses, rate limits, large responses
- Security tests: SSRF prevention (private IPs, localhost, metadata endpoint)
- Race detection: parallel search + fetch execution

## Acceptance Criteria

### Functional — websearch
- [x] `websearch` tool registered via `ToolDefinitions()` when at least one backend is available
- [x] `websearch` tool NOT registered when zero backends are available (no broken tools visible to LLM)
- [x] Tavily backend works with valid API key; `Available()` returns false without key
- [x] Brave backend works with valid API key; `Available()` returns false without key
- [x] SearXNG backend works with local instance; `Available()` returns false when unreachable
- [x] SearXNG Docker Compose service added alongside Ollama on port 8964
- [x] Backend priority: SearXNG → Tavily → Brave; configurable override in config
- [x] Runtime fallback: if primary backend fails, try next in priority order
- [x] Auth errors (401/403): mark backend as degraded for session, skip on subsequent calls
- [x] Rate limits (429): fallback to next backend; if all rate-limited, return error with hint
- [x] Network errors/5xx: fallback to next backend
- [x] Malformed JSON: fallback to next backend, log raw response
- [x] Empty results: return as-is (not an error, no fallback)
- [x] Partial results: return collected results + warning about incomplete search
- [x] Context cancellation: return immediately with partial results or error
- [x] Per-backend timeout (default 10s): cancel and try next
- [x] All backends fail: return clear error to LLM summarizing which backends failed and why
- [x] Search results include titles, URLs, snippets; content included when `includeContent=true`
- [x] Startup availability logged: which backends available, which missing, what to configure
- [x] Tool description enforces exact tool name and discourages aliases
- [x] Tool description includes 2-3 integrity rules
- [x] Tool description includes current date dynamically

### Functional — webfetch
- [x] `webfetch` tool always registered (no backend dependency)
- [x] Converts HTML to markdown correctly
- [x] Strips `<style>` and `<script>` before conversion
- [x] Truncates HTML >2MB before conversion; truncates response >5MB
- [x] Blocks SSRF: private IPs (10/8, 172.16/12, 192.168/16), loopback (127/8), link-local (169.254/16), metadata (169.254.169.254)
- [x] Blocks redirects to private IPs
- [x] Identifies as `tau-agent` in User-Agent header
- [x] Binary content detection: returns metadata, not raw bytes
- [x] Supports context cancellation
- [x] Timeout: default 30s, max 120s
- [x] Returns clear errors for: DNS failure, connection refused, 403, 404, 5xx, timeout

### Functional — integration
- [ ] Researcher subagent has websearch + webfetch in tool set
- [x] Search API keys resolved via existing 4-step auth chain
- [x] Config supports `search.backend` (auto/searxng/tavily/brave) and `search.searxng_url`

### Non-Functional
- [x] One new external dependency: `html-to-markdown` (justified)
- [x] No CGo, no headless browser, no MCP client
- [x] Both tools are `ExecutionParallel` — can run concurrently
- [x] Response size limited to prevent context overflow
- [x] All HTTP clients have proper timeouts
- [x] Provider-agnostic — search works with any LLM provider

### Testing
- [x] Unit tests for each search backend with mock HTTP server
- [x] Unit tests for webfetch with mock HTTP server
- [x] Fallback tests: primary fails → secondary succeeds, primary fails → secondary fails → error
- [x] Degradation tests: 401 from primary → marked degraded → skipped on retry
- [x] Partial results test: mock server returns 2 results then errors → 2 results + warning returned
- [x] Security tests for SSRF prevention
- [x] Race detection passes: `go test -race ./internal/tools/...`
- [x] Coverage ≥80% for all new files
- [x] Integration test with SearXNG in Docker

### Documentation
- [x] ARCHITECTURE.md updated with search/fetch tools
- [x] DECISIONS.md updated with search backend choices
- [x] Config documentation updated

## Testing & Verification Strategy

### Unit Tests (mock HTTP servers via `httptest`)

**Tavily backend**:
- Success with content (`includeContent=true`)
- Success without content (`includeContent=false`)
- API error (401 — invalid key, non-retryable)
- Rate limit (429 — retryable, fallback)
- Server error (5xx — retryable, fallback)
- Malformed JSON response
- Empty results (valid response, no fallback)
- Network timeout

**Brave backend**:
- Success with titles/URLs/descriptions
- API error (401 — non-retryable)
- Rate limit (429 — retryable)
- Malformed JSON
- Network error

**SearXNG backend**:
- Success with titles/URLs/snippets
- Connection refused (SearXNG not running)
- Malformed JSON
- Timeout

**Fallback chain (websearch)**:
- Primary succeeds → returns results immediately (no fallback)
- Primary fails (network) → secondary succeeds → returns secondary results
- Primary fails (401) → primary marked degraded → secondary succeeds
- Primary fails (401) → secondary fails (429) → tertiary succeeds
- All backends fail → returns error summarizing all failures
- Degraded backend skipped on subsequent calls within same session
- Empty results from primary → returns empty results (no fallback)
- Partial results: mock returns 2 results then closes connection → returns 2 results + warning

**WebFetch**:
- HTML→markdown conversion
- Text response (no conversion needed)
- 404 → clear error
- 403 → "access denied" error
- Timeout → "timed out" error
- Redirect to public IP → follows normally
- Redirect to private IP → blocked with error
- Private IP URL → blocked with error
- Large HTML (>2MB) → truncated before conversion
- Large response (>5MB) → truncated
- Style/script tags → stripped from output
- Binary content (PDF) → metadata returned, not raw bytes
- Context cancellation → returns immediately

**SSRF validation**:
- `http://localhost` → blocked
- `http://127.0.0.1` → blocked
- `http://10.0.0.1` → blocked
- `http://172.16.0.1` → blocked
- `http://192.168.1.1` → blocked
- `http://169.254.169.254` → blocked
- `http://example.com` → allowed
- `https://go.dev` → allowed

**Availability & registration**:
- No API keys + SearXNG unreachable → `NewWebSearchTool` returns nil → tool not registered
- Only SearXNG running → tool registered with 1 backend
- Tavily key only → tool registered with 1 backend
- Multiple backends → tool registered with all, priority order correct
- Config override `search.backend: "tavily"` → Tavily first regardless of default priority

### Integration Tests
- **SearXNG**: Docker-based test with local instance
- **Tavily/Brave**: Marked as external, run with `-tags=integration` + API key env vars

### Security Tests
- SSRF: attempt to fetch `http://localhost`, `http://127.0.0.1`, `http://10.0.0.1`, `http://169.254.169.254`
- DNS rebinding: mock DNS returning public IP then private IP on redirect
- Response size: mock server returning 10MB response, verify truncation
- Content sanitization: HTML with script/style tags, verify markdown output is clean
- Large HTML: verify truncation before conversion prevents hangs

### End-to-End Verification
- Start tau with Tavily API key configured
- Ask agent: "Search for the latest Go 1.24 release notes"
- Verify agent calls `websearch`, receives results, and can summarize them
- Ask agent: "Fetch the content of https://go.dev/doc/go1.24"
- Verify agent calls `webfetch`, receives markdown, and can reference it
- Verify agent does NOT hallucinate tool aliases like `google_search` (Feynman integrity test)
- Verify Researcher subagent has access to websearch + webfetch
- Start tau with NO search API keys and SearXNG not running
- Verify websearch tool is NOT registered (no broken tool visible to LLM)
- Verify startup logging shows which backends are available/unavailable
- Start tau with SearXNG + Tavily, then stop SearXNG mid-session
- Verify websearch falls back to Tavily automatically

---

## Decisions (from analysis.md)

See `analysis.md` for the full pattern-by-pattern evaluation. Summary:

| Decision | Rationale |
|----------|-----------|
| Client-side search with pluggable backends | Provider-agnostic (P8), local-first (P9), leaf package (P6) |
| Two tools: websearch + webfetch | Proven universal pattern, clean separation (P1) |
| Backend priority: SearXNG → Tavily → Brave | SearXNG = zero config local-first; Tavily = best content; Brave = quality alternative |
| Runtime auto-fallback on backend failure | Graceful degradation (P11) — every failure has a defined outcome |
| Auth errors degrade backend for session | Bad key = permanent skip, not retryable; avoids wasting tokens on repeated failures |
| Hide tool when no backend available | Avoid confusing LLM with broken tools |
| webfetch always registered (no backend dependency) | Direct HTTP, always works regardless of search API configuration |
| SSRF protection, not domain permissions | Single-user tool (P3), simpler security model sufficient |
| No hostname safety preflight | Provider-agnostic (P8), local-first (P9) |
| html-to-markdown as one new dependency | Essential for webfetch, focused library, justified |
| Tool naming enforcement in descriptions | Prevents LLM hallucination, especially local models (P9) |
| 2-3 integrity rules in descriptions | Concise protection against fabrication |
| Dynamic date injection | Proven by OpenCode, improves query quality |
| Deep research deferred to skill layer | Orchestration is instructions (P10), not code |
| MCP search deferred to post-MVP | No extension system in MVP (P7) |

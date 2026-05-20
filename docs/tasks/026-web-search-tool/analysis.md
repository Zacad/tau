# Task 026: Web Search Tool — Pattern Analysis & Proposal

## Tau Core Principles (Evaluation Criteria)

Every pattern must be evaluated against these principles derived from REQUIREMENTS.md, ARCHITECTURE.md, and DECISIONS.md:

| # | Principle | Source |
|---|-----------|--------|
| P1 | **Minimalist** — "minimalist agentic coding tool" | REQUIREMENTS.md §1 |
| P2 | **Works out of the box** — "skills and subagents working out of the box — without installing or configuring extensions" | REQUIREMENTS.md §1 |
| P3 | **Single user** — "Not for distribution — personal tool" | REQUIREMENTS.md §2 |
| P4 | **Minimal dependencies** — "Dependencies: Minimal" | REQUIREMENTS.md §9 |
| P5 | **Go idiomatic** — "Language: Go", "use idiomatic approach" | REQUIREMENTS.md §9, AGENTS.md |
| P6 | **Leaf packages** — tools depend only on `internal/types` and stdlib | ARCHITECTURE.md §2.2 |
| P7 | **No extension system (MVP)** — explicitly out of scope | REQUIREMENTS.md §10, ARCHITECTURE.md §1.4 |
| P8 | **Provider-agnostic** — multi-provider from day one (OpenAI, Anthropic, Google, Ollama, etc.) | ARCHITECTURE.md §6 |
| P9 | **Local-first** — Ollama/local models as first-class citizens | REQUIREMENTS.md §6 |
| P10 | **Orchestration is instructions, not code** — agent loop follows AGENTS.md/skills, no framework | ARCHITECTURE.md §1.1 |
| P11 | **Graceful degradation** — failures should not crash, degrade cleanly | ARCHITECTURE.md §9.5 |

---

## Pattern Analysis

### Pattern 1: Server-Side Search (Claude Code)

**How it works**: The Anthropic API itself executes searches. Claude Code sends a `web_search` server tool definition in the API request. The API returns results with `encrypted_content` that only the Anthropic API can decrypt. Zero client-side search infrastructure.

| | |
|---|---|
| **Pros** | Zero API key management for users. No search backend code to maintain. Encrypted content prevents tampering. Built-in citation system with `cited_text`. Domain filtering at API level (`allowed_domains`, `blocked_domains`). Location-aware results. |
| **Cons** | **Hard-locked to Anthropic provider** — violates P8 (provider-agnostic). Encrypted content is opaque — can't inspect, cache, or transform it. $10/1K searches is expensive. No self-hosted option — violates P9 (local-first). Cannot work with Ollama or OpenRouter. |
| **Trade-offs** | Simplicity vs provider independence. This is the simplest possible implementation but at the cost of being Anthropic-only. For a multi-provider tool like tau, this is a non-starter as the primary approach. |

**Verdict**: **REJECT as primary approach.** Incompatible with P8 (provider-agnostic) and P9 (local-first). However, we could optionally support Anthropic's server-side search as a backend when using the Anthropic provider — this would be a future enhancement, not MVP.

---

### Pattern 2: Client-Side Search with Pluggable Backends (OpenCode / Feynman)

**How it works**: Search runs on the user's machine. The tool calls a search API (Exa, Tavily, Brave, etc.) directly via HTTP. Multiple backends are supported, with configuration determining which one to use.

| | |
|---|---|
| **Pros** | **Provider-agnostic** (P8) — works with any LLM provider. Backend is swappable — user picks what works for them. Can work with self-hosted backends (SearXNG) — supports P9 (local-first). Full control over result format, caching, and transformation. Can combine search + fetch into a coherent experience. |
| **Cons** | User must configure API keys. More code to maintain (backend implementations). Need to handle different API response formats. Need fallback logic when backends fail. |
| **Trade-offs** | Flexibility vs complexity. Each backend is ~100-150 lines of Go. With 3 backends (Tavily, Brave, SearXNG), that's ~400 lines of backend code plus the abstraction layer. Acceptable for a Go project. |

**Verdict**: **ACCEPT.** This is the right architecture for tau. Aligns with P2 (works out of box with any key), P8 (provider-agnostic), P9 (SearXNG for local), P6 (leaf package — just HTTP calls).

---

### Pattern 3: MCP-Based Search (Goose / Continue / PI)

**How it works**: Search tools come from external MCP (Model Context Protocol) servers. The agent connects to MCP servers configured by the user, discovers tools via `tools/list`, and invokes them via `tools/call`.

| | |
|---|---|
| **Pros** | Industry standard — growing ecosystem of MCP servers. No need to implement search backends — delegate to existing MCP servers (Brave, Exa, etc.). Extensible — users can add any MCP server. |
| **Cons** | **Requires extension system** — violates P7 (no extension system in MVP). Need MCP client implementation in Go (adds dependency complexity). User must install and configure MCP servers — violates P2 (works out of the box). MCP is still evolving — API stability concerns. |
| **Trade-offs** | Extensibility vs simplicity. MCP is the "right" long-term architecture for extensibility, but it's premature for tau MVP. The project explicitly chose native subagents over PI's extension-based approach (DECISIONS.md: "Subagents as First-Class Citizens"). |

**Verdict**: **DEFER to post-MVP.** The `SearchBackend` interface should be designed so MCP backends can be added later without refactoring. But MVP should use native Go implementations of search backends, not MCP clients.

---

### Pattern 4: Two-Tool Pattern — websearch + webfetch (Universal)

**How it works**: Two separate tools: `websearch` takes a query and returns search results; `webfetch` takes a URL and returns page content. Used by OpenCode, Claude Code, and Feynman.

| | |
|---|---|
| **Pros** | Clean separation of concerns — search discovers, fetch retrieves. LLM can decide when to search vs when to fetch a known URL. Search backends that don't return content (Brave, SearXNG) still work — webfetch fills the gap. Each tool is simple and testable. |
| **Cons** | Two tools to maintain instead of one. LLM must learn when to use each — potential confusion. Some backends (Tavily, Exa) return content directly, making webfetch redundant for those results. |
| **Trade-offs** | Modularity vs convenience. The two-tool pattern is strictly more capable — the LLM can search without fetching (metadata-only results), or fetch without searching (known URLs). A single "search_and_fetch" tool would be less flexible. |

**Verdict**: **ACCEPT.** This is the proven pattern. Aligns with P1 (minimalist — each tool does one thing), P5 (idiomatic Go — small, focused interfaces). The LLM discoverability concern is addressed by clear tool descriptions.

---

### Pattern 5: Search API with Built-in Content Extraction (Tavily / Exa)

**How it works**: The search API returns not just titles and snippets, but full page content as markdown. This eliminates the need for a separate fetch step.

| | |
|---|---|
| **Pros** | Fewer API calls — search results include content. Better results for AI agents — content is cleaned and optimized. Tavily returns relevance scores. Reduces latency — one call instead of search + fetch. |
| **Cons** | **Only works with Tavily/Exa** — not with Brave or SearXNG. Increases response size — full content for 8 results could be huge. Content quality varies — not all pages extract well via API. Creates backend-specific behavior — Tavily returns content, Brave doesn't, LLM gets inconsistent results. |
| **Trade-offs** | Richness vs consistency. If we always include content from search results, the tool behavior differs by backend. If we normalize (always use webfetch for content), we have consistent behavior but extra API calls. |

**Verdict**: **ACCEPT with normalization.** The `websearch` tool should have an `includeContent` parameter (like Feynman's `includeContent: true`). Backends that support it natively (Tavily, Exa) return content inline. Backends that don't (Brave, SearXNG) return just metadata. The tool result format is consistent — each result has `title`, `url`, `snippet`, and optionally `content`. This way, the LLM can decide whether to request content in the search call or fetch specific URLs afterward.

---

### Pattern 6: Auto-Fallback Backend Chain (Feynman)

**How it works**: Multiple search backends are configured. If the primary fails (API error, rate limit, no key), automatically try the next one. Feynman's order: Exa → Perplexity → Gemini.

| | |
|---|---|
| **Pros** | Higher reliability — one backend failing doesn't break the agent. Works toward P2 (works out of the box) — if any key is configured, search works. Simple to implement — try each backend in order until one succeeds. |
| **Cons** | Can hide configuration issues — user doesn't know which backend is actually being used. Latency — if primary fails, there's a delay before fallback responds. Different backends return different result quality. |
| **Trade-offs** | Reliability vs transparency. Auto-fallback is friendlier (P2) but can be confusing. We should log which backend is being used so the user can see. |

**Verdict**: **ACCEPT.** Implementation: try backends in priority order (configured primary → Tavily → Brave → SearXNG). Skip backends without API keys. Log which backend handled the request. Aligns with P11 (graceful degradation) and P2 (works out of the box with any configured key).

---

### Pattern 7: Domain Permission Model (Claude Code)

**How it works**: `WebFetch(domain:docs.python.org)` permission rules control which domains can be fetched. Allow/deny/ask per domain. Integrates with network sandbox.

| | |
|---|---|
| **Pros** | Fine-grained security control. Prevents SSRF to internal domains. Allows safe auto-approval for known good domains. Integrates with the existing `BeforeToolCall` hook system. |
| **Cons** | Adds configuration complexity. Claude Code's implementation is tied to their enterprise/managed settings system. For a single-user tool (P3), the security model is over-engineered. |
| **Trade-offs** | Security vs simplicity. For a personal tool, SSRF protection (blocking private IPs) is sufficient. Domain-level permissions add complexity without much benefit for a single user. |

**Verdict**: **SIMPLIFY.** Implement SSRF protection (block private IPs, localhost) as the primary defense — this is the critical security requirement. Domain allow/deny can be a future enhancement if needed. The existing `BeforeToolCall` hook already provides a mechanism for custom blocking logic. Do NOT build a domain permission system in MVP.

---

### Pattern 8: Hostname Safety Preflight (Claude Code)

**How it works**: Before fetching a URL, Claude Code sends just the hostname to `api.anthropic.com` to check against a safety blocklist. Results cached per hostname for 5 minutes.

| | |
|---|---|
| **Pros** | Prevents access to known-malicious domains. Centralized blocklist maintained by Anthropic. |
| **Cons** | **Requires outbound call to Anthropic** — violates P8 (provider-agnostic) and P9 (local-first). User may not want all hostnames sent to Anthropic. Adds latency. Not applicable to a multi-provider tool. |
| **Trade-offs** | Safety vs privacy/independence. A centralized safety check is valuable for enterprise tools but inappropriate for a personal, provider-agnostic tool. |

**Verdict**: **REJECT.** Incompatible with P8 (provider-agnostic) and P3 (personal tool). Instead, implement local SSRF protection (block RFC 1918 IPs, localhost, link-local) and optional user-maintained blocklist in config. This matches the self-contained, local-first philosophy.

---

### Pattern 9: Tool Naming Enforcement (Feynman)

**How it works**: The system prompt explicitly states: "For web search, call `web_search`; do not call non-existent aliases such as `google:search`, `google_search`, or `search_google`." Prevents LLM from hallucinating tool names.

| | |
|---|---|
| **Pros** | Prevents a real, observed failure mode — LLMs invent tool names. Especially important for smaller models (Ollama/local). Zero implementation cost — just text in the description. |
| **Cons** | None — this is purely a description quality improvement. |
| **Trade-offs** | None. |

**Verdict**: **ACCEPT.** Include in tool descriptions. Example: "Use the websearch tool to search the web. Do not use non-existent aliases like google_search, web_search, or search_web." Aligns with P5 (pragmatic), P9 (important for local models which are more prone to hallucination).

---

### Pattern 10: Integrity Instructions in Tool Descriptions (Feynman)

**How it works**: The researcher agent prompt includes "6 Integrity Commandments": never fabricate results, URL-or-it-didn't-happen, never extrapolate unread content, etc.

| | |
|---|---|
| **Pros** | Prevents the LLM from fabricating search results or claiming to have verified something it didn't. Particularly important for research tasks where accuracy matters. Low cost — just text in descriptions. |
| **Cons** | Adds token overhead to every tool description. Some instructions may be ignored by smaller models. |
| **Trade-offs** | Token cost vs reliability. For a coding agent, fabricated search results are a real risk. A few extra tokens in the description is worth the protection. |

**Verdict**: **ACCEPT with restraint.** Include 2-3 key integrity rules in the websearch description, not 6 full commandments. Keep it concise to minimize token overhead. Example: "Always cite URLs from results. Never fabricate or modify search results. If a search fails, report the failure honestly."

---

### Pattern 11: Dynamic Date Injection in Description (OpenCode)

**How it works**: The websearch tool description includes `{{year}}` which is replaced with the current year at runtime. "The current year is 2026. You MUST use this year when searching for recent information."

| | |
|---|---|
| **Pros** | LLMs have training data cutoffs — explicit date helps them formulate better queries (e.g., "Go 1.24 release 2026"). Simple to implement — string replacement at tool registration time. Proven by OpenCode. |
| **Cons** | Adds implementation complexity — the `Description()` method must be dynamic, not a static string. Slightly different from existing tools which have static descriptions. |
| **Trade-offs** | Utility vs implementation simplicity. The date injection is genuinely useful for a search tool. The implementation cost is minimal — store the date at construction time and interpolate. |

**Verdict**: **ACCEPT.** The `WebSearchTool` struct takes a `currentDate` field at construction time. The `Description()` method interpolates it. This is a small deviation from static descriptions but justified by the significant utility for search quality. Aligns with P2 (works well out of the box).

---

### Pattern 12: Availability Gating (OpenCode)

**How it works**: OpenCode's `websearch` tool is only available when using the OpenCode provider OR when `OPENCODE_ENABLE_EXA=1` is set. The tool is registered but filtered out of `ToolDefinitions()` based on provider.

| | |
|---|---|
| **Pros** | Prevents confusing the LLM with unavailable tools. Avoids tool call failures from missing API keys. |
| **Cons** | Tool disappears silently — user may not understand why search isn't available. OpenCode's approach ties availability to a specific provider — not appropriate for multi-provider tau. |
| **Trade-offs** | Hiding vs showing unavailable tools. If we hide the tool when no API key is configured, the LLM can't even try to search (which is correct — it would fail anyway). If we show it, the LLM might try and get an error. |

**Verdict**: **ACCEPT adapted.** Two behaviors:
1. If **any** search backend is available (API key configured or SearXNG reachable), register the tool normally.
2. If **no** backend is available, do NOT register the tool — it would only produce errors and confuse the LLM.
3. At startup, log which search backends are available (so the user knows).

This is simpler than OpenCode's provider-gating and aligns with P2 (if configured, it works) and P11 (graceful degradation).

---

### Pattern 13: Scale-Adaptive Deep Research (Feynman)

**How it works**: Simple questions get direct search (3-10 tool calls). Complex topics get 2-6 parallel researcher subagents. Explicit rule: "Do not inflate a simple explainer into a multi-agent survey."

| | |
|---|---|
| **Pros** | Cost-efficient — doesn't waste API calls on simple questions. Appropriate resource allocation. Prevents the "sledgehammer for a nut" problem. |
| **Cons** | Requires the LLM to judge question complexity — not always reliable. Adds orchestration complexity. |
| **Trade-offs** | Efficiency vs simplicity. The judgment happens in the skill/prompt, not in code. The search tool itself is simple — the complexity is in the orchestration layer. |

**Verdict**: **DEFER to skill layer.** The search tools should be simple and focused (P1). Scale-adaptive behavior belongs in a `deep-research` skill that instructs the agent when to use subagents vs direct search. This aligns with P10 (orchestration is instructions, not code). The tools just need to work well — the orchestration patterns come later.

---

### Pattern 14: Provenance Sidecars (Feynman)

**How it works**: Every research deliverable gets a `.provenance.md` file tracking sources consulted, accepted, rejected, and verification status.

| | |
|---|---|
| **Pros** | Full audit trail. Helps verify research quality. Valuable for reproducibility. |
| **Cons** | Overhead for a coding agent — most search queries are quick lookups, not deep research. Creates extra files in the project. |
| **Trade-offs** | Thoroughness vs friction. Provenance makes sense for research deliverables, not for "what's the latest version of React?" |

**Verdict**: **DEFER to deep-research skill.** Not appropriate for the tool layer. A future `deep-research` skill could instruct the agent to create provenance files. The tool itself should just return results.

---

### Pattern 15: Forced On-Disk Verification (Feynman)

**How it works**: After claiming any fix or edit, must verify with `rg`/`grep`/`diff`/`stat` that the change actually landed. Cannot say "fixed" unless the verification command succeeds.

| | |
|---|---|
| **Pros** | Prevents the LLM from claiming actions it didn't perform. Catches silent failures. |
| **Cons** | This is a general agent behavior, not specific to search. Enforcing it in tool code would be overly complex. |
| **Trade-offs** | Reliability vs flexibility. This is best enforced at the skill/prompt level, not in code. |

**Verdict**: **OUT OF SCOPE for this task.** This is an agent behavior pattern, not a search tool feature. Could be addressed in a future "agent reliability" task.

---

### Pattern 16: HTML→Markdown Conversion (Universal — OpenCode, Claude Code, Feynman)

**How it works**: webfetch converts HTML to markdown before returning to the LLM. Different implementations: OpenCode uses TurndownService (JS), Claude Code strips `<style>`/`<script>` then converts, Feynman delegates to `pi-web-access`.

| | |
|---|---|
| **Pros** | Markdown is the natural format for LLMs — cleaner, more token-efficient than HTML. Stripping `<style>`/`<script>` prevents CSS-heavy pages from consuming context budget. Truncating large HTML before conversion prevents hangs. |
| **Cons** | Requires an HTML→markdown library — adds a dependency. Conversion quality varies — some pages convert poorly. Adds processing time. |
| **Trade-offs** | Quality vs dependency. `github.com/JohannesKaufmann/html-to-markdown` is the best Go option (4.5K stars, actively maintained, high-quality conversion). It's a single dependency that solves a real problem. |

**Verdict**: **ACCEPT.** Use `github.com/JohannesKaufmann/html-to-markdown` for conversion. Strip `<style>`/`<script>` before conversion (Claude Code pattern). Truncate large HTML before conversion to prevent hangs. This is one new external dependency — justified by the value it provides. Aligns with P4 (minimal dependencies — one focused library).

---

### Pattern 17: SSRF Protection via IP Validation (Claude Code-inspired)

**How it works**: Before fetching a URL, validate that the resolved IP address is not a private/reserved address (RFC 1918, loopback, link-local, etc.).

| | |
|---|---|
| **Pros** | Prevents the agent from accessing internal services (localhost, internal APIs, cloud metadata endpoints). Critical security feature for any tool that makes HTTP requests. |
| **Cons** | DNS rebinding attacks — IP could change between validation and request. Need to resolve DNS before making the request. Some legitimate URLs might resolve to private IPs (unlikely but possible). |
| **Trade-offs** | Security vs completeness. DNS rebinding is a real attack but unlikely in a personal tool (P3). The protection is still valuable — it prevents accidental SSRF (e.g., LLM asks to fetch `http://localhost:8080`). |

**Verdict**: **ACCEPT.** Implement IP validation by resolving the hostname and checking against private ranges. This is a security essential. Use `net.Dialer` with a custom `Resolver` to control DNS resolution. For DNS rebinding: use a custom `Transport` that validates the resolved IP on every connection, not just before the request.

---

## Proposal: What to Implement in Tau

### Summary Decision Table

| Pattern | Decision | Rationale |
|---------|----------|-----------|
| 1. Server-side search (Claude Code) | REJECT | Violates P8 (provider-agnostic), P9 (local-first) |
| 2. Client-side pluggable backends | ACCEPT | Core architecture — aligns with P8, P9, P6 |
| 3. MCP-based search | DEFER | Violates P7 (no extension system) — add post-MVP |
| 4. Two tools: websearch + webfetch | ACCEPT | Universal proven pattern — P1 (minimalist) |
| 5. Search with content extraction | ACCEPT adapted | `includeContent` param, normalized result format |
| 6. Auto-fallback backend chain | ACCEPT | P11 (graceful degradation), P2 (works out of box) |
| 7. Domain permission model | SIMPLIFY | SSRF protection sufficient for single-user (P3) |
| 8. Hostname safety preflight | REJECT | Violates P8 (provider-agnostic), P9 (local-first) |
| 9. Tool naming enforcement | ACCEPT | Zero cost, prevents real failure mode (P9 — local models) |
| 10. Integrity instructions | ACCEPT with restraint | 2-3 rules, not 6 commandments |
| 11. Dynamic date injection | ACCEPT | Proven by OpenCode, helps query quality |
| 12. Availability gating | ACCEPT adapted | Hide tool when no backend available, log availability |
| 13. Scale-adaptive research | DEFER to skill | P10 (orchestration is instructions, not code) |
| 14. Provenance sidecars | DEFER to skill | Not appropriate for tool layer |
| 15. Forced on-disk verification | OUT OF SCOPE | Agent behavior, not search tool feature |
| 16. HTML→Markdown conversion | ACCEPT | Essential for webfetch, one justified dependency |
| 17. SSRF protection | ACCEPT | Security essential |

### Proposed Architecture

```
internal/tools/
├── search.go              # SearchBackend interface, WebSearchTool, SearchResult
├── search_tavily.go       # Tavily backend implementation
├── search_brave.go        # Brave Search backend implementation
├── search_searxng.go      # SearXNG backend implementation
├── webfetch.go            # WebFetchTool, HTML→markdown, SSRF protection
├── search_test.go         # Backend abstraction tests
├── search_tavily_test.go  # Tavily tests (mock HTTP)
├── search_brave_test.go   # Brave tests (mock HTTP)
├── search_searxng_test.go # SearXNG tests (mock HTTP)
└── webfetch_test.go       # WebFetch tests (mock HTTP, SSRF)
```

### Proposed Search Backend Priority

1. **SearXNG** — if reachable (no API key needed, local-first P9)
2. **Tavily** — if API key configured (best content extraction)
3. **Brave** — if API key configured (independent index, good quality)

Rationale: SearXNG first because it requires zero configuration and aligns with the local-first philosophy. If the user has Ollama running (already the default test setup), SearXNG can run alongside it. Tavily second because of its AI-optimized results with built-in content extraction. Brave third as a quality alternative.

### Proposed Tool Parameters

**websearch**:
```go
type WebSearchParams struct {
    Query           string `json:"query" jsonschema:"required,description=Search query"`
    MaxResults      int    `json:"maxResults,omitempty" jsonschema:"description=Maximum results (default 8)"`
    IncludeContent  bool   `json:"includeContent,omitempty" jsonschema:"description=Include page content in results (uses more tokens)"`
}
```

**webfetch**:
```go
type WebFetchParams struct {
    URL     string `json:"url" jsonschema:"required,description=URL to fetch (must be http or https)"`
    Format  string `json:"format,omitempty" jsonschema:"description=Output format: markdown (default), text, or html"`
    Timeout int    `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds (max 120, default 30)"`
}
```

### Proposed Tool Descriptions

**websearch** (dynamic — date interpolated):
> Search the web for information. Use this tool when you need current, up-to-date information that may not be in your training data — such as recent library versions, documentation, error solutions, or current best practices. The current date is {{date}}. Always cite URLs from search results. Never fabricate or modify search results. Use the websearch tool to search the web. Do not use non-existent aliases like google_search, search_web, or web_lookup.

**webfetch**:
> Fetch the content of a web page and convert it to markdown. Use this when you have a specific URL and need to read its content. The URL must be a valid http or https URL. This tool is read-only and does not modify any files. Prefer websearch when you need to discover information; use webfetch when you already know the URL you want to read. Results may be truncated for very large pages.

### New External Dependency

| Dependency | Purpose | Justification |
|-----------|---------|---------------|
| `github.com/JohannesKaufmann/html-to-markdown` | HTML→Markdown conversion for webfetch | Only viable Go option. 4.5K stars, actively maintained, high-quality output. Single focused dependency. Does one thing well. |

No other new dependencies — all search backends use stdlib `net/http` + `encoding/json`.

### Configuration Integration

**`~/.tau/config.json`** additions:
```json
{
  "search": {
    "backend": "auto",
    "searxng_url": "http://localhost:8964"
  }
}
```

**`~/.tau/auth.json`** additions:
```json
{
  "tavily": "tvly-...",
  "brave": "BSA-..."
}
```

**Environment variables**: `TAVILY_API_KEY`, `BRAVE_API_KEY` — resolved via existing 4-step auth chain.

### Subagent Integration

Researcher subagent (from ARCHITECTURE.md §5.4) currently has tools: `read, grep, find, ls, bash (read-only)`. Add: `websearch, webfetch`.

This aligns with Feynman's pattern where the researcher agent has full search access, and with REQUIREMENTS.md §3.3: "Researcher — research and gather information."

---

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| html-to-markdown produces poor output for some sites | Medium | Low | Fallback to raw text format; user can request `format=text` |
| Tavily/Brave API changes | Low | Medium | Backend abstraction isolates changes to one file |
| SearXNG not running | High | Low | Auto-fallback to Tavily/Brave; log warning at startup |
| DNS rebinding SSRF | Low | High | Custom Transport with per-connection IP validation |
| LLM ignores tool naming instructions | Medium (higher for local models) | Medium | Test with Ollama; may need stronger prompt language |
| Large search results consume context budget | Medium | Medium | Truncation via existing `Truncate()`; `includeContent` defaults to false |
| Rate limiting from search APIs | Low | Low | Return clear error; user can switch backends |

# Web Search Tool Research for Tau

## 1. How Popular Coding Agents Implement Web Search

### Aider

**Web search built-in?** Partial - URL scraping only, no search API
- **What it does:** Scrapes web pages by URL and converts to markdown for chat context
- **How:** Uses `/web <url>` command or pasting URLs in chat. Internally uses `aider/scrape.py` with `playwright` or `httpx` + `beautifulsoup4` + `html2text` + `pygments` for fetching and converting pages to markdown
- **No search API integration:** Aider does NOT have web search (query-based search). It only scrapes specific URLs the user provides
- **Result format:** Markdown text from scraped page, added directly to LLM context
- **Special features:** None - purely manual URL-based context addition

### Cline

**Web search built-in?** Yes - via browser automation + `@url` context provider
- **Browser automation:** Uses Puppeteer/Chrome to browse the web interactively. The LLM can navigate, click, type, and take screenshots. This is the primary "web search" mechanism
- **`@url` context:** User can paste a URL and Cline fetches/converts it to markdown for context
- **No dedicated search API:** Cline relies on browser automation for web search rather than a search API. The LLM navigates to Google/Bing in a headless browser
- **Tool definition (browser):** `browser_action` with actions: `launch`, `click`, `type`, `scroll_down`, `scroll_up`, `close`
- **Result format:** Screenshots + console logs from browser interactions
- **Special features:** Human-in-the-loop approval for all browser actions, remote browser support, viewport configuration

### Cursor

**Web search built-in?** Yes - proprietary implementation
- **How:** Cursor has built-in web search accessible via `@Web` context provider and automatic web search in agent mode
- **Implementation is closed-source** but known to use a combination of:
  - Custom search indexing
  - Real-time web fetching for documentation
  - Integration with their proprietary indexing pipeline
- **Result format:** Markdown snippets with source URLs, injected into LLM context
- **Special features:** Automatic docs fetching for libraries, `@Docs` context provider for adding documentation sources, `@Web` for general web search

### Windsurf/Codeium

**Web search built-in?** Yes - via "Cascade" agent
- **How:** Windsurf's Cascade agent can search the web using a built-in search tool
- **Implementation is closed-source**, but known features:
  - Real-time web search for finding documentation and solutions
  - Context-aware search that considers the current codebase
- **Result format:** Summarized search results with source links
- **Special features:** Integrated with Codeium's indexing infrastructure, automatic relevance filtering

### Continue

**Web search built-in?** Yes - via context providers and MCP
- **How:** Continue supports web search through:
  - `@url` context provider (fetches and converts URLs to markdown)
  - MCP (Model Context Protocol) servers for extensible search
  - Custom context providers that can integrate any search API
- **No default search API:** Continue doesn't ship with a built-in search API provider out of the box. Users configure their own via `config.json`
- **Result format:** Depends on the context provider used
- **Special features:** Extensible via MCP, supports custom context providers, `@url` for URL fetching

### Goose (Block)

**Web search built-in?** Yes - via MCP extensions
- **How:** Goose implements web capabilities through MCP (Model Context Protocol) server extensions:
  - Built-in `web-search` extension using various search backends
  - `browser` extension for web browsing/automation
  - Extensions are configured in goose's `session.yaml` or via CLI flags
- **Search API used:** Configurable - supports Brave Search API, Tavily, and others via MCP
- **Tool definition:** Exposed as MCP tools (e.g., `web_search`, `fetch_url`)
- **Result format:** Structured results via MCP protocol
- **Special features:** Rust-native implementation, extensible via MCP, built-in browser automation

### SWE-agent

**Web search built-in?** No
- **How:** SWE-agent operates entirely within a sandboxed environment with bash, file editing, and search tools limited to the repository
- **No web search:** SWE-agent is designed for autonomous bug fixing on GitHub issues. It has no web search capability
- **Available tools:** `bash`, `search_dir`, `search_file`, `open`, `create`, `edit`, `submit` - all repo-local

---

## 2. Search API Comparison

### Tavily API

| Aspect | Details |
|--------|---------|
| **Type** | Purpose-built for AI agents |
| **Pricing** | Free: 1,000 credits/month; Pay-as-you-go: $0.008/credit; Project plans from ~$32/month for 4,000 credits |
| **Rate limits** | Varies by plan; enterprise has custom limits |
| **Result quality** | High - designed for LLM consumption with relevance scoring |
| **Ease of integration** | Excellent - Python/JS SDKs, single POST endpoint |
| **Content extraction** | Built-in! `include_raw_content` returns full page markdown/text; `search_depth` controls relevance vs latency |
| **Special features** | `include_answer` (LLM-generated answer), `search_depth` (basic/advanced/fast/ultra-fast), domain filtering, country boost, time range filtering, auto_parameters, `/extract` endpoint for URL scraping, `/research` endpoint for deep research, `/crawl` for site crawling |
| **Latency** | ~180ms p50 for basic search |
| **Best for** | AI agents needing high-quality, structured search results with content extraction |

### Serper API

| Aspect | Details |
|--------|---------|
| **Type** | Google SERP scraper |
| **Pricing** | Free: 2,500 queries; Then $2/1,000 queries (very cheap) |
| **Rate limits** | Generous for the price |
| **Result quality** | Good - returns Google SERP data directly |
| **Ease of integration** | Simple REST API, single GET endpoint |
| **Content extraction** | No built-in content extraction - returns snippets only. Need separate scraping step |
| **Special features** | Search, Images, News, Maps, Places, Videos, Shopping, Scholar, Patents, Autocomplete endpoints. Fast (1-2 second response) |
| **Latency** | 1-2 seconds |
| **Best for** | Cheap, fast Google search results when you only need snippets |

### Brave Search API

| Aspect | Details |
|--------|---------|
| **Type** | Independent search index |
| **Pricing** | Search: $5/1,000 requests; Answers: $4/1,000 + $5/M tokens; Free $5 monthly credits |
| **Rate limits** | 50 QPS (Search), 2 QPS (Answers) |
| **Result quality** | High - own independent index, anti-SEO spam |
| **Ease of integration** | Good - REST API with Go SDK support, also OpenAI-compatible chat completions endpoint |
| **Content extraction** | No built-in page extraction - returns snippets. Has `/llm/context` endpoint that returns structured data optimized for LLMs with up to 5 snippets per result |
| **Special features** | Independent index (not Google/Bing derivative), Goggles (custom reranking), schema-enriched results, LLM Context endpoint, MCP server available, SOC 2 attested, Zero Data Retention option |
| **Latency** | Low - own infrastructure |
| **Best for** | Privacy-focused, independent index, excellent MCP support, Go SDK available |

### Google Custom Search API

| Aspect | Details |
|--------|---------|
| **Type** | Google search via official API |
| **Pricing** | Free: 100 queries/day; $5/1,000 queries after that (max 10k/day) |
| **Rate limits** | 10,000 queries/day max |
| **Result quality** | Best - direct Google results |
| **Ease of integration** | Moderate - requires API key + Custom Search Engine ID setup |
| **Content extraction** | No content extraction - snippets only |
| **Special features** | Can restrict search to specific sites, image search |
| **⚠️ CRITICAL** | **CLOSED TO NEW CUSTOMERS** as of 2026. Existing customers have until Jan 1, 2027 to migrate. Not a viable option. |
| **Best for** | Not viable for new projects |

### Bing Search API

| Aspect | Details |
|--------|---------|
| **Type** | Microsoft Bing search API |
| **Pricing** | Free: 1,000 queries/month; S1: $7/1,000 queries; S2: $20/1,000 queries (higher volume) |
| **Rate limits** | Varies by tier (1-100 QPS) |
| **Result quality** | Good - Bing results |
| **Ease of integration** | Moderate - Azure subscription required |
| **Content extraction** | No content extraction - snippets only |
| **Special features** | Web, image, video, news search; Azure ecosystem integration |
| **⚠️ Note** | Microsoft has been deprecating/restructuring Bing Search API offerings |
| **Best for** | Organizations already in Azure ecosystem |

### SearXNG (Self-hosted)

| Aspect | Details |
|--------|---------|
| **Type** | Open-source metasearch engine |
| **Pricing** | Free (self-hosted) - only costs are infrastructure |
| **Rate limits** | Self-imposed (configurable) |
| **Result quality** | Variable - aggregates from multiple search engines |
| **Ease of integration** | Moderate - needs Docker/self-hosting, REST API available |
| **Content extraction** | No content extraction - returns aggregated snippets |
| **Special features** | Privacy-focused, no tracking, aggregates 70+ search engines, fully self-hosted, configurable, can add/remove search engines, JSON API |
| **Latency** | Slower - aggregates from multiple engines |
| **Best for** | Privacy-first, zero-cost, self-hosted deployments, offline/air-gapped environments |

---

## 3. Summary Comparison Table

| API | Price/1K | Content Extraction | LLM-Optimized | Free Tier | MCP Server | Go SDK |
|-----|----------|-------------------|---------------|-----------|------------|--------|
| **Tavily** | $8 | Built-in (markdown/text) | Yes | 1K/mo | Yes | Via REST |
| **Serper** | $2 | No (snippets only) | No | 2.5K total | No | Via REST |
| **Brave** | $5 | Partial (LLM context endpoint) | Yes | $5/mo credits | **Yes** | **Yes** |
| **Google CSE** | $5 | No | No | 100/day | No | Via REST |
| **Bing** | $7 | No | No | 1K/mo | No | Via REST |
| **SearXNG** | Free | No | No | Unlimited | Community | Via REST |

---

## 4. Recommendation for Tau

### Primary Recommendation: **Tavily API**

**Why:**
1. **Purpose-built for AI agents** - returns results optimized for LLM consumption with relevance scores
2. **Built-in content extraction** - `include_raw_content` returns full page markdown, eliminating the need for a separate scraping step
3. **Rich search parameters** - domain filtering, time range, country boost, topic (general/news/finance), search depth control
4. **LLM-generated answers** - `include_answer` provides quick answers alongside search results
5. **Multi-endpoint** - `/search`, `/extract`, `/crawl`, `/research` covers all use cases
6. **Simple API** - Single POST endpoint, Python/JS SDKs, easy to integrate in Go via REST
7. **Good free tier** - 1,000 credits/month for development
8. **Industry standard** - Used by LangChain, CrewAI, AutoGen, and many agent frameworks

### Secondary/Alternative: **Brave Search API**

**Why as alternative:**
1. **Independent index** - not dependent on Google/Bing, better anti-SEO spam
2. **Official Go SDK** - native Go client library available
3. **MCP server** - official MCP server for easy agent integration
4. **LLM Context endpoint** - `/llm/context` returns structured data for LLMs
5. **Privacy-focused** - good for users who value privacy
6. **SOC 2 attested** - enterprise-ready

### Self-hosted Option: **SearXNG**

**Why for self-hosted:**
1. Zero cost after deployment
2. Complete privacy - no data leaves your infrastructure
3. Works with local Ollama setup (matches tau philosophy)
4. Aggregates multiple search engines for broader coverage
5. Docker deployment aligns with existing ollama docker-compose setup

### Recommended Architecture for Tau

```
┌─────────────────────────────────────┐
│           Tau Agent Loop          │
├─────────────────────────────────────┤
│  web_search tool (LLM-callable)     │
│  ├─ query: string                   │
│  ├─ max_results: int (default 5)    │
│  ├─ search_depth: basic|advanced    │
│  └─ include_domains: []string       │
├─────────────────────────────────────┤
│         Search Provider Interface    │
├──────────┬──────────┬───────────────┤
│  Tavily  │  Brave   │   SearXNG    │
│ (default)│(alternative)│(self-hosted)│
└──────────┴──────────┴───────────────┘
```

**Tool definition for the LLM:**

```json
{
  "name": "web_search",
  "description": "Search the web for information. Returns relevant results with titles, URLs, and content snippets. Use this to find documentation, API references, solutions to errors, or current information.",
  "parameters": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "The search query"
      },
      "max_results": {
        "type": "integer",
        "description": "Maximum number of results to return (default: 5, max: 10)",
        "default": 5
      },
      "search_depth": {
        "type": "string",
        "enum": ["basic", "advanced"],
        "description": "Use 'advanced' for more thorough results (costs 2x credits)",
        "default": "basic"
      },
      "include_domains": {
        "type": "array",
        "items": {"type": "string"},
        "description": "Only include results from these domains"
      }
    },
    "required": ["query"]
  }
}
```

**Additional tool for page extraction:**

```json
{
  "name": "web_fetch",
  "description": "Fetch and extract the content of a web page as markdown. Use this to read documentation pages, API references, or any URL.",
  "parameters": {
    "type": "object",
    "properties": {
      "url": {
        "type": "string",
        "description": "The URL to fetch"
      }
    },
    "required": ["url"]
  }
}
```

### Result Formatting

Search results should be formatted for the LLM as:

```
<web_search_results>
<result>
<title>Result Title</title>
<url>https://example.com/page</url>
<content>Relevant content snippet from the page...</content>
</result>
<result>
<title>Another Result</title>
<url>https://docs.example.com/api</url>
<content>Another relevant snippet...</content>
</result>
</web_search_results>
```

### Key Design Decisions

1. **Two separate tools** - `web_search` for querying, `web_fetch` for reading specific pages. This mirrors how Aider, Cline, and other agents work
2. **Provider interface** - Abstract search behind an interface so users can swap Tavily/Brave/SearXNG
3. **Content extraction via Tavily** - Use Tavily's `include_raw_content` to get full page markdown, avoiding the need for a separate HTML-to-markdown pipeline
4. **Domain filtering** - Critical for coding agents to target docs sites (e.g., pkg.go.dev, developer.mozilla.org)
5. **Rate limiting** - Implement client-side rate limiting to avoid burning through API credits
6. **Caching** - Cache search results by query to avoid redundant API calls within a session

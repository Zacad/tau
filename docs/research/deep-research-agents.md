# Deep Research Agents - Research Findings

## 1. Feynman Agent

**Not found as a significant project.** The only match was a 0-star repo (`Fahad7177-jeh/deep-research-agent`) using LangGraph/LangChain with "Feynman-based explanation pedagogy" - this refers to the Feynman Technique (explain things simply), not a project called "Feynman Agent". No established project by that name exists.

---

## 2. Open-Source Deep Research Implementations

### 2.1 dzhng/deep-research (18.9k stars, TypeScript, MIT)

The most popular minimal deep research agent. Goal: <500 LoC, easy to understand.

**Search API**: Firecrawl (combined search + scrape, returns markdown)
**LLM**: o3-mini (default), DeepSeek R1 via Fireworks, custom OpenAI-compatible endpoints

**Architecture - Recursive Depth/Breadth Exploration**:

```
User Query + breadth (3-10) + depth (1-5)
  → generateSerpQueries(): LLM generates N targeted SERP queries
  → For each query: firecrawl.search() → processSerpResult()
  → processSerpResult(): LLM extracts learnings + follow-up questions
  → If depth > 0: recurse with new context (prior goals + follow-ups + learnings)
  → Breadth halves at each depth level (newBreadth = ceil(breadth/2))
  → Parallel processing with p-limit concurrency control
  → writeFinalReport(): LLM synthesizes all learnings into markdown
```

**Key implementation details**:
- `generateSerpQueries()`: Uses structured output (zod schema) - returns `{query, researchGoal}` per query
- `processSerpResult()`: Returns `{learnings: string[], followUpQuestions: string[]}`
- Content trimmed to 25k chars per source, 15s search timeout, 5 results per query
- Deduplication via `new Set()` on learnings and URLs
- Error handling: timeouts and failures return empty results, don't crash
- Concurrency limit: 2 (free Firecrawl) or configurable

**System prompt**: "You are an expert researcher" - emphasizes accuracy, detail, organization, proactivity, speculation flagging

### 2.2 assafelovic/gpt-researcher (26.9k stars, Python, Apache 2.0)

The most feature-complete open deep research framework.

**Supported Search APIs** (retriever abstraction):
| API | Notes |
|-----|-------|
| **Tavily** | Primary/default, purpose-built for AI agents |
| Google Search | Google Programmable Search |
| Bing | Microsoft Search API |
| DuckDuckGo | Free, no API key |
| Serper | Google SERP API |
| SerpAPI | Google SERP scraping |
| SearchAPI | Multi-engine search |
| Searx | Self-hosted meta-search |
| Exa | Semantic search for AI |
| ArXiv | Academic papers |
| Semantic Scholar | Academic papers |
| PubMed Central | Biomedical literature |
| BoCha | Chinese search |
| MCP | Model Context Protocol (custom data sources) |
| Custom | Pluggable interface |

**Architecture - Plan-and-Solve + RAG**:
1. Create task-specific agent from research query
2. **Planner** generates research questions (sub-queries)
3. **Execution agents** (crawlers) gather information per question in parallel
4. Summarize + source-track each resource
5. Filter + aggregate summaries
6. **Publisher** writes final research report

**Deep Research mode** (tree-based):
- Configurable depth and breadth
- Concurrent processing across branches
- Smart context management across research branches
- ~5 min per deep research, ~$0.4 cost (o3-mini high)
- Inspired by STORM paper

**Tavily implementation**:
- POST to `https://api.tavily.com/search`
- Options: `search_depth` (basic/advanced), `max_results`, `topic`, `include_domains`, `exclude_domains`, `days`, `include_answer`, `include_raw_content`, `include_images`, `use_cache`
- Returns: `[{url, content}]` - already extracted/cleaned content

---

## 3. Proprietary Deep Research Systems

### 3.1 OpenAI Deep Research

- **Model**: o3 optimized for web browsing + data analysis
- **Training**: End-to-end RL on hard browsing/reasoning tasks
- **Behavior**: Plans multi-step trajectory, backtracks, reacts to real-time info
- **Capabilities**: Browse files, Python tool, embed graphs/images, cite specific passages
- **Duration**: 5-30 minutes per query
- **Latest** (2026): MCP support for custom data, visual browser in agent mode, real-time progress tracking, follow-up prompt refinement
- **Performance**: 26.6% on Humanity's Last Exam, 72.57% on GAIA (cons@64)

### 3.2 Google Deep Research / Deep Research Max

- **Model**: Gemini 3.1 Pro
- **Two variants**:
  - Deep Research: Optimized for speed, lower latency
  - Deep Research Max: Extended test-time compute, iterative reason/search/refine
- **Key features**:
  - MCP support for proprietary data sources
  - Native chart/infographic generation (HTML + Nano Banana)
  - Collaborative planning (review/guide research plan before execution)
  - Combine: Google Search + MCPs + URL Context + Code Execution + File Search
  - Real-time streaming of intermediate reasoning
  - Multimodal research grounding (PDFs, CSVs, images, audio, video)
- **Used in**: Gemini App, NotebookLM, Google Search, Google Finance

### 3.3 Perplexity

- Real-time search + AI synthesis, citation-focused
- More "search engine with AI" than deep multi-step research
- Pro Search mode does multi-step reasoning with follow-up questions

---

## 4. Common Patterns

### 4.1 Multi-Step Search Pipeline

All deep research agents follow a similar pattern:

```
1. QUERY DECOMPOSITION
   LLM generates multiple targeted search queries from the research question
   
2. PARALLEL SEARCH
   Execute multiple searches concurrently (with concurrency limits)
   
3. CONTENT EXTRACTION
   Scrape/parse full page content (not just snippets)
   Convert to markdown for clean LLM input
   
4. LEARNING EXTRACTION
   LLM summarizes each source to extract key facts/learnings
   
5. FOLLOW-UP GENERATION
   LLM generates follow-up questions based on what was found
   
6. RECURSIVE EXPLORATION
   If depth remains: use follow-ups + prior learnings as new context
   Generate new queries → search → extract → repeat
   
7. SYNTHESIS
   Combine all accumulated learnings into final report
   Include all source URLs as citations
```

### 4.2 Search API Comparison

| API | Cost | Quality | AI-Optimized | Scrape Included | Rate Limits |
|-----|------|---------|-------------|-----------------|-------------|
| **Tavily** | Free tier + paid | High | Yes (returns cleaned content) | Yes | Generous |
| **Firecrawl** | Free tier + paid | High | Yes (search + scrape + markdown) | Yes | Free tier limited |
| **Serper** | $50/50k queries | High (Google) | No (SERP only) | No | Good |
| **SerpAPI** | $50/5k queries | High (Google) | No (SERP only) | No | Good |
| **Google Programmable** | Free 100/day | Highest | No | No | 100/day free |
| **Bing** | Free tier | High | No | No | Varies |
| **DuckDuckGo** | Free | Medium | No | No | Aggressive |
| **Exa** | Paid | High (semantic) | Yes (content included) | Yes | Moderate |
| **Brave Search** | Free tier | Good | No | No | Moderate |
| **Searx** | Free (self-hosted) | Good | No | No | Unlimited |

**For AI agents, the key differentiator is whether the API returns cleaned content or just SERP links.** Tavily, Firecrawl, and Exa return content - others require a separate scraping step.

### 4.3 How Results Are Processed

1. **Content extraction**: Full page text scraped and converted to markdown
2. **Token management**: Content trimmed per source (e.g., 25k chars in dzhng/deep-research)
3. **LLM summarization**: Each source summarized to extract key learnings
4. **Deduplication**: URLs and learnings deduplicated across iterations
5. **Source tracking**: All URLs tracked and cited in final report
6. **Error handling**: Timeouts/failures return empty results, don't crash the pipeline

### 4.4 Depth/Breadth Parameters

| System | Breadth Default | Depth Default | Breadth Reduction |
|--------|----------------|---------------|-------------------|
| dzhng/deep-research | 4 | 2 | Halves each level |
| GPT Researcher | Configurable | Configurable | Configurable |
| OpenAI Deep Research | Auto | Auto (5-30 min) | N/A |
| Google Deep Research Max | Auto | Auto (extended compute) | N/A |

### 4.5 LLM Usage Patterns

**Structured output is universal**: All implementations use LLM with structured output schemas:
- Query generation: `{queries: [{query, researchGoal}]}`
- Result processing: `{learnings: string[], followUpQuestions: string[]}`
- Report writing: `{reportMarkdown: string}`

**This is the key insight**: The LLM is used as a transformation engine at each step, with schemas constraining output to exactly what's needed for the next step.

---

## 5. Key Takeaways for Tau

1. **No unique "Feynman Agent"** exists - the concept refers to using the Feynman Technique as a pedagogical constraint within research agents
2. **The recursive depth/breadth pattern** from dzhng/deep-research is the cleanest implementation to study - <500 LoC
3. **Tavily is the de facto standard** search API for AI research agents (cleanest integration, AI-optimized results)
4. **Firecrawl provides search + scrape** in one call, simplifying the pipeline
5. **Structured output schemas** are critical for reliable pipeline operation
6. **Concurrent search execution** with bounded concurrency is essential for performance
7. **The retriever abstraction** in GPT Researcher is the right pattern - pluggable search backends
8. **MCP is emerging** as the standard for connecting proprietary data sources to deep research

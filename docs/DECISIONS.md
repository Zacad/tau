# DECISIONS

<!-- Architectural and design decisions log -->

## 2026-05-14 — OpenAI-Compatible Provider Streaming Architecture

- **Decision**: `OpenAICompatProvider.Stream()` makes direct HTTP requests using `net/http` with `bufio.Scanner` for incremental SSE parsing, bypassing `DefaultHTTPClient`
- **Rationale**: `DefaultHTTPClient.Do()` uses `io.ReadAll(resp.Body)` which blocks until EOF. For streaming SSE responses, this hangs until the server closes the connection (up to 5-minute timeout). Context cancellation does NOT stop `io.ReadAll`. Direct HTTP requests with `bufio.Scanner` enable incremental event delivery and proper context cancellation.
- **Alternatives considered**: Custom HTTPClient with streaming support, context-aware io.ReadAll wrapper, http.RoundTripper
- **Context**: Task 040 — app hung when OpenCode Go weekly limit was reached

## 2026-05-02 — Tool Name: Tau

- **Decision**: Tool is named **tau**
- **Rationale**: Greek for "practical action" — theory put into practice. Reflects the core philosophy: the agent doesn't just chat, it *does the work* through skills and subagents. CLI command: `tau`
- **Alternatives considered**: harness, kit, relay, loom, axiom, basis, ergon, techne, nous
- **Context**: Naming discussion after task 002 (Requirements Definition)

## 2026-05-02 — Session Storage: JSONL Format

- **Decision**: Use JSONL (JSON Lines) for session persistence
- **Rationale**: Simple append-only format, proven by PI's implementation, no CGO dependency (vs SQLite), trivial to inspect/debug/backup, natural fit for streaming recording. Each line is a complete JSON object — corruption only affects last line.
- **Alternatives considered**: SQLite, plain JSON, in-memory with periodic save
- **Context**: Task 004 (Architecture Design), §8.2 Storage Format Justification

## 2026-05-02 — Provider Interface Per API Type

- **Decision**: One Provider implementation per API type (not per individual provider). OpenAI-compatible providers share the same implementation with configuration differences.
- **Rationale**: Covers 7 of 9 target providers with a single OpenAI-compat implementation (OpenRouter, OpenCode Zen, OpenCode Go, Ollama, llama.cpp, LM Studio). Minimizes code duplication.
- **Alternatives considered**: Per-provider implementations, unified interface with adapters
- **Context**: Task 004 (Architecture Design), §6.1 Provider Interface

## 2026-05-02 — Tool Parameters as Go Structs + JSON Schema

- **Decision**: Use Go structs with `github.com/invopop/jsonschema` for JSON Schema generation from struct tags
- **Rationale**: Compile-time type safety with automatic JSON Schema generation. LLM tool calling requires proper JSON Schema. Manual schema or reflection-based generation is too error-prone.
- **Alternatives considered**: Manual JSON Schema per tool, runtime validation, `encoding/json` only (no schema)
- **Context**: Task 004 (Architecture Design), review finding #6 (Critical)

## 2026-05-02 — Streaming via Go Channels

- **Decision**: Use Go channels for streaming events from providers and agent loop
- **Rationale**: Idiomatic Go concurrency, no external event bus dependency, natural fit for the event-driven loop pattern.
- **Alternatives considered**: Callback-based streaming, external event bus library
- **Context**: Task 004 (Architecture Design), §6.5 Streaming Events

## 2026-05-02 — Subagents as First-Class Citizens

- **Decision**: Build subagents natively into the core, not as extensions
- **Rationale**: Core requirement from REQUIREMENTS.md — skills and subagents must work out of the box. PI's extension-based approach is 1168+ lines of types. Native implementation with goroutines + channels is simpler and more performant.
- **Alternatives considered**: Extension system (PI's approach), plugin-based architecture
- **Context**: Task 004 (Architecture Design), §5 Subagent System Architecture

## 2026-05-02 — Per-Type Token Estimation Heuristics

- **Decision**: Use per-content-type heuristics: text=chars/4, tool results=chars/3, thinking=chars/3.5
- **Rationale**: More accurate than single heuristic since tool results and thinking blocks have different token densities. No external tokenizer dependency.
- **Alternatives considered**: tiktoken port, BPE tokenizer, single chars/4 for all content
- **Context**: Task 004 (Architecture Design), §7.5 Compaction Strategy

## 2026-05-02 — 4-Step Auth Resolution Chain

- **Decision**: CLI flag → auth.json → environment variables → config file
- **Rationale**: Matches PI's resolution order. CLI flag for scripting, dedicated auth.json with 0600 permissions for security, env vars for CI, config file as fallback. Supports literal, env var reference, and shell command key formats.
- **Alternatives considered**: Simple 2-step chain (env → config)
- **Context**: Task 004 (Architecture Design), review finding #4 (High)

## 2026-05-02 — Synchronous Subagent Execution

- **Decision**: Subagents run synchronously with configurable timeout (default 5 minutes). Parent waits.
- **Rationale**: Simplest correct approach for single-user tool. Async would add complexity for minimal benefit. Timeout prevents indefinite blocking.
- **Alternatives considered**: Async with result queue, goroutine pool
- **Context**: Task 004 (Architecture Design), review finding #17 (High)

## 2026-05-02 — Split `types` Package for Shared Data

- **Decision**: Introduce `internal/types/` package for all shared data structures (AgentMessage, ToolResult, SessionEntry, etc.)
- **Rationale**: Eliminates import cycles between agent, session, and subagent packages. Follows Go idiomatic patterns (cf. pi-ai package in PI).
- **Alternatives considered**: Direct imports between packages, interface-based decoupling
- **Context**: Task 004 (Architecture Design), review finding #13 (High)

## 2026-05-02 — CWD Encoding via `/` → `-` Replacement

- **Decision**: Encode working directory paths by replacing `/` with `-` for session directory naming
- **Rationale**: Human-readable, no collision risk, matches PI's approach. Simpler than hash or base64 encoding.
- **Alternatives considered**: SHA256 hash, base64 encoding
- **Context**: Task 004 (Architecture Design), review finding #1 (Low)

## 2026-05-02 — Auto-Naming from First User Message

- **Decision**: Session names derived from first user message (truncated to 50 chars), not from LLM summarization
- **Rationale**: No LLM call needed, instant, deterministic. LLM summarization for naming would add cost and latency for minimal benefit.
- **Alternatives considered**: LLM-based summarization, timestamp-only naming
- **Context**: Task 004 (Architecture Design), review finding #11 (Low)

## 2026-05-02 — No Orchestrator Package: Agent Loop IS the Orchestrator

- **Decision**: No separate `orchestrator/` package. The agent loop reads AGENTS.md and skills, then follows instructions autonomously. Orchestration is declarative (in context files), not imperative (in code).
- **Rationale**: The three-component split (ContextManager, WorkflowEngine, AgentCoordinator) was over-engineering. PI's `AgentSession` is monolithic and works. For a single-user tool, AGENTS.md defines the workflows — the agent loop just executes them. The `subagent` package is the only genuinely new component compared to PI.
- **Alternatives considered**: Three-component orchestrator split (review finding #7 recommendation), single AgentCoordinator
- **Context**: Task 004 (Architecture Design), user discussion on orchestration philosophy

## 2026-05-02 — Config Format: JSON

- **Decision**: Use JSON for `~/.tau/config.json` instead of YAML
- **Rationale**: Consistency with `auth.json`, simpler parsing with stdlib `encoding/json`, no external dependency needed. YAML reserved only for skill frontmatter (required by Agent Skills standard).
- **Alternatives considered**: YAML, TOML
- **Context**: Task 004 (Architecture Design), user decision #6

## 2026-05-02 — Tool Parallelism Specification

- **Decision**: Parallel: read, grep, find, ls. Sequential: write, edit (per-file mutex). Exclusive: bash.
- **Rationale**: Read-only tools are safe to run concurrently. Write/edit tools must be serialized to prevent file corruption. Bash is exclusive because it can have side effects that affect other tools' execution environment.
- **Alternatives considered**: All parallel, all sequential
- **Context**: Task 004 (Architecture Design), review finding #10 (Medium), user decision #3

## 2026-05-02 — Token Heuristic Accuracy Note

- **Decision**: Document that `chars/N` heuristics are a rough MVP estimate. Plan to add a real tokenizer (e.g., tiktoken port) in post-MVP for accurate compaction triggers.
- **Rationale**: Current heuristics (text=chars/4, tool results=chars/3, thinking=chars/3.5) are conservative overestimates. They work for MVP but may cause premature or delayed compaction. Real token counting will improve accuracy.
- **Alternatives considered**: Port tiktoken now, use exact token counting from provider APIs
- **Context**: Task 004 (Architecture Design), review finding #30 (Low), user decision #7

## 26. Markdown rendering with glamour + OSC 8 hyperlinks (Task 028)

- **Decision**: Use `github.com/charmbracelet/glamour` for markdown rendering in assistant messages. Streaming text remains plain text; glamour rendering only on finalized blocks (after `AgentEventMessageEnd`). URLs wrapped with OSC 8 terminal hyperlink escape sequences for clickability. Code block URLs excluded from wrapping. Rendered output cached per block with lazy invalidation on resize.

- **Rationale**: Glamour is part of the charm.land ecosystem (same as bubbletea/lipgloss), ensuring compatibility and consistent styling. It uses `chroma` for syntax highlighting in code blocks. Streaming text cannot use glamour because incomplete markdown (unclosed fences, broken lists) causes rendering failures. OSC 8 provides native clickable links in modern terminals (kitty, iTerm2, GNOME Terminal) without requiring mouse event handling. The "dark" standard style is used — clean output designed for dark terminals. Cache invalidation on resize ensures proper reflow at new width.

- **Alternatives considered**: Custom markdown renderer (rejected: reinventing the wheel, charm ecosystem already solves this), `goldmark` + custom ANSI output (rejected: more complex, glamour handles this), HTML rendering (rejected: not suitable for terminal), keeping plain text only (rejected: loses markdown formatting from LLM responses).

- **Context**: Task 028 (Styled Markdown Output with Clickable Links). Subtasks: 028.1 (glamour rendering), 028.2 (OSC 8 hyperlinks), 028.4 (resize/caching/edge cases).

---

## 17. Execution order: sequential before parallel

- **Decision**: In `Registry.ExecuteBatch()`, sequential tools (write/edit) execute BEFORE parallel tools (read/grep/find/ls), after exclusive tools (bash).
- **Rationale**: When an LLM calls write + read in the same batch, the read must see the write's output. Running sequential first ensures file mutations complete before reads occur. Original architecture had parallel before sequential, which caused read failures when LLMs batched write+read calls.
- **Context**: Task 008 (Tool System), discovered during manual testing with Ollama

## 19. Provider-agnostic reasoning support (Task 016)

- **Decision**: Reasoning/thinking tokens are handled uniformly across all providers via a block-switching streaming pattern. Each provider's reasoning field is mapped to a single internal model:
  - **OpenAI-compat providers** (Ollama, OpenRouter, OpenCode, llama.cpp, etc.): scan for `reasoning_content`, `reasoning`, and `reasoning_text` fields in the delta — use the first non-empty field to avoid duplication (e.g., chutes.ai returns both `reasoning_content` and `reasoning` with the same content)
  - **Anthropic providers**: `content_block_start/delta/stop` events for `thinking` and `redacted_thinking` blocks, including `signature_delta` for integrity verification
  - **Round-trip support**: `BlockThinking` blocks are serialized back when converting messages to API format. Provider-specific compat flags control the format (plain text, separate field with signature key, or native thinking block)
  - **Event granularity**: Providers emit `EventThinkingStart`, `EventThinkingDelta`, and `EventThinkingEnd` events — matching PI's `thinking_start/delta/end` pattern for consumers that need block lifecycle awareness
  - **Interleaving support**: Reasoning can arrive before, during, or after content text. The stream parser uses a `currentBlock` pointer that switches between thinking/text/toolCall blocks, finishing the previous block when the type changes

- **Rationale**: Tau targets multiple providers from day one (OpenAI, Anthropic, OpenRouter, OpenCode, Ollama). A provider-agnostic approach avoids re-implementing reasoning handling per provider. PI's block-switching pattern (`finishCurrentBlock`) proven in production handles all interleaving edge cases. The compat flag system allows per-provider round-trip behavior without code duplication.

- **Alternatives considered**: Ollama-only reasoning handling (rejected — incompatible with multi-provider strategy), Anthropic-only thinking pattern (rejected — doesn't cover OpenAI-compat providers)

- **Context**: Task 016 (Reasoning Support), PI comparison analysis against `openai-completions.js` and `anthropic.js` source

## 18. Tolerant integer parsing for tool parameters

- **Decision**: Use `IntOrString` custom type for integer tool parameters (e.g., read's limit/offset) that accepts both JSON numbers and strings.
- **Rationale**: LLMs (especially smaller models like llama3.2) frequently send integer fields as strings (e.g., `"1000"` instead of `1000`). Strict JSON unmarshaling causes tool execution failures. Tolerant parsing improves reliability without changing the tool interface.
- **Alternatives considered**: Strict JSON Schema enforcement (rejected — too brittle), schema post-processing (rejected — complex), `any` type with manual conversion (rejected — loses type safety)
- **Context**: Task 008 (Tool System), discovered during manual testing with Ollama

## 20. Web search: client-side with pluggable backends (Task 026)

- **Decision**: Implement client-side web search with pluggable backends (SearXNG, Tavily, Brave). Two tools: `websearch` (query → results) and `webfetch` (URL → markdown content).
- **Rationale**: Provider-agnostic — search works regardless of which LLM is active. SearXNG provides zero-cost local-first option (like Ollama for search). Tavily offers best content extraction. Brave provides independent index quality. Auto-fallback on backend failure ensures reliability. Hiding websearch when no backend is available avoids confusing the LLM with broken tools.
- **Alternatives considered**: Server-side search (Claude Code pattern — rejected: provider-locked), MCP-based search (rejected: no extension system in MVP), single search-and-fetch tool (rejected: less flexible)
- **Context**: Task 026 (Web Search Tool), comparison analysis against PI, OpenCode, Claude Code, Feynman

## 21. Search backend priority: SearXNG → Tavily → Brave (Task 026)

- **Decision**: Default backend priority is SearXNG first (if reachable), then Tavily (if API key), then Brave (if API key). Configurable override via `search.backend`.
- **Rationale**: SearXNG aligns with local-first philosophy — zero configuration, runs alongside Ollama in Docker. Tavily second for its AI-optimized results with built-in content extraction. Brave third as a quality independent-index alternative.
- **Alternatives considered**: Tavily-first (rejected: doesn't match local-first philosophy), single backend only (rejected: no fallback)
- **Context**: Task 026 (Web Search Tool)

## 22. html-to-markdown as one new dependency (Task 026)

- **Decision**: Add `github.com/JohannesKaufmann/html-to-markdown` as the only new external dependency for the web search feature.
- **Rationale**: Essential for webfetch — HTML must be converted to markdown for LLM consumption. This is the best Go option (4.5K stars, actively maintained, high-quality conversion). All search backends use stdlib `net/http` + `encoding/json` — no additional dependencies.
- **Alternatives considered**: Custom HTML→markdown converter (rejected: too complex), regex-only stripping (rejected: poor quality)
- **Context**: Task 026 (Web Search Tool)

## 23. SSRF protection via IP validation, not domain permissions (Task 026)

- **Decision**: Implement SSRF protection by resolving hostnames and blocking private/reserved IPs (RFC 1918, loopback, link-local). No domain-level permission system.
- **Rationale**: Single-user tool — domain permissions are over-engineered. IP validation covers the critical security requirement (preventing access to internal services, cloud metadata endpoints). DNS rebinding defense via redirect validation.
- **Alternatives considered**: Domain allow/deny model (Claude Code — rejected: too complex for single-user tool), hostname safety preflight (rejected: requires outbound call to third-party)
- **Context**: Task 026 (Web Search Tool)

## 24. TUI event delivery via `p.Send()` instead of blocking channel Cmd (Task 026)

- **Decision**: Use `tea.Program.Send()` to inject agent events directly into the bubbletea event loop, instead of reading from a channel via a blocking `tea.Cmd` returned in `tea.Batch`.
- **Rationale**: Bubbletea v2's `execBatchMsg` uses `sync.WaitGroup` — it waits for ALL commands in a batch to complete before returning. A blocking channel-read command (`func() tea.Msg { return <-ch }`) in a batch prevents the WaitGroup from completing, causing a 3-way deadlock: (1) timer goroutine blocked on `p.Send()` (unbuffered channel), (2) event loop blocked on `cmds <- cmd`, (3) `handleCommands` blocked on `wg.Wait()`. This froze the entire TUI during tool execution. `p.Send()` bypasses the command system entirely, sending directly to the event loop's message channel.
- **Alternatives considered**: Heartbeat goroutine sending to event channel (rejected: same deadlock), `tea.Every`/`tea.Tick` (rejected: still requires blocking Cmd in batch), `tea.Sequence` (rejected: doesn't solve the blocking issue)
- **Context**: TUI freezing during tool execution (websearch taking 5-30s). Root cause discovered via deep research subagent analysis of bubbletea v2 source code.

## 25. Avoid `s.mu` acquisition in TUI event handlers during agent loop

- **Decision**: Do not call `Session.Usage()` or any method that acquires `s.mu` from TUI event handlers while the agent loop is running.
- **Rationale**: `Session.Prompt` holds `s.mu` for the entire agent loop. If a TUI event handler (e.g., `AgentEventTurnEnd`) calls `s.Usage()` which tries to acquire `s.mu`, the event loop blocks. The agent goroutine then tries to send the next event via `p.Send()`, but the event loop is blocked on `s.mu` → deadlock. Usage is now captured in `handlePromptDone` after `s.Prompt` returns and releases `s.mu`.
- **Alternatives considered**: Read-write mutex (rejected: `persistNewMessages` also writes), separate usage goroutine (rejected: race conditions), atomic usage counter (rejected: too complex)
- **Context**: TUI complete freeze during tool execution, discovered after fixing the `tea.Batch` deadlock.

## 26. SDK-Level Message Queue for Next-Turn Queuing (Task 030)

- **Decision**: Implement message queuing at the SDK `Session` level, not in the TUI `Model`. Queue state (`messageQueue []string`, `overflowCount int`) lives in `Session` with thread-safe methods: `EnqueueMessage()`, `DequeueMessage()`, `PendingCount()`, `OverflowCount()`, `ResetOverflow()`.
- **Rationale**: Better separation of concerns — queue is a session-level concern, not a UI concern. TUI remains thin, just calling SDK methods. SDK queue is reusable by any consumer (not just the TUI). Thread-safe via `s.mu` mutex. Max 10 messages, FIFO, drops oldest on overflow.
- **Alternatives considered**: TUI-level queue (rejected: couples queue logic to UI, not reusable), Reuse existing follow-up queue (rejected: follow-up queue is agent-internal, not designed for user-typed messages)
- **Context**: Task 030 — allow user input during streaming, queue for next turn.

## 27. Viewport update throttling + finalized block render cache (Task 031)

- **Decision**: Throttle viewport updates to ~30fps (33ms interval) during streaming. Cache rendered finalized blocks to avoid O(n) re-render per text delta. Cache invalidated on: resize, new finalized blocks (flushPending), tool status changes, errors, subagent events, queued messages, slash commands.
- **Rationale**: Every text delta event was triggering `updateViewport()` which called `renderBlocks()` re-rendering ALL blocks from scratch. During streaming this is O(n) per event where n = number of blocks, and events arrive dozens per second. This caused: (1) Ctrl+C abort taking several seconds (queued behind hundreds of events), (2) scrolling at ~1 line/second, (3) spinner blinking faster than intended (overloaded event loop). Throttling reduces viewport updates from ~50/sec to ~30/sec. Caching reduces render cost from O(n) to O(1) per update during streaming (only pending block changes).
- **Alternatives considered**: Throttle only (rejected: doesn't fix O(n) problem for long conversations), Cache only (rejected: doesn't address event flood causing Ctrl+C lag), Debounce instead of throttle (rejected: throttle provides more consistent responsiveness)
- **Context**: Task 031 — TUI performance fix for streaming lag.

## 28. `/model` command shows only models from connected providers

- **Decision**: `Session.ListModels()` filters the model registry to return only models whose provider is currently registered (connected). The `/model` palette displays this filtered list. Model switching mid-session works unchanged via `session.SetModel()`.
- **Rationale**: Built-in models (OpenAI, Anthropic, Google) were always shown regardless of whether those providers had API keys or were enabled in config. This confused users by presenting models they couldn't actually use. Filtering by connected providers ensures the palette only shows actionable choices.
- **Alternatives considered**: Show all models but gray out unavailable ones (rejected: more complex UX, still shows clutter), Provider-level filtering only (rejected: users need to see specific model names)
- **Context**: `/model` command showed 10 built-in models even with only Ollama connected.

## 29. Provider exception handling with typed errors

- **Decision**: Introduce `APIError` type in `types/errors.go` with error categories: `rate_limit`, `credit_exhausted`, `quota_exceeded`, `auth_failed`, `permission_denied`, `server_error`, `bad_request`, `unknown`. All provider implementations use `types.ClassifyAPIError(statusCode, body)` to parse HTTP error responses and emit user-friendly messages via `UserMessage()`. Classification uses both HTTP status codes and message content extraction from common JSON error formats (`{"error": {"message": "..."}}`, `{"error": "..."}`, `{"message": "..."}`).
- **Rationale**: Provider error responses were previously shown as raw HTTP status + body text (e.g., "API error 429: {...}"). Users need actionable messages like "Rate limit reached. Please wait before sending more requests." Typed errors also enable future programmatic handling (e.g., automatic retry on rate limits, prompting for new API key on auth failure).
- **Alternatives considered**: Provider-specific error types (rejected: too much duplication), HTTP retry only (rejected: doesn't help with non-retriable errors like credit exhaustion), Raw error pass-through (rejected: poor UX)
- **Context**: Handling all provider exceptions including rate limits, credit exhaustion, weekly/monthly quotas, and auth failures.

## 30. `/connect` and `/disconnect` command refactoring

- **Decision**: Refactor `/connect` to fix critical BaseURL bug (testOpenAICompat was using hardcoded `https://example.com/v1`), use `ProviderInfo.TestConnection`/`DiscoverModels` functions directly instead of duplicate switch statements, add "already connected" state detection, and validate API keys before testing. Refactor `/disconnect` to only show providers explicitly connected via `/connect` (auth.json + config entry), not auto-registered providers from environment variables. Add warning when disconnecting the active model's provider.
- **Rationale**: The `/connect` command was fundamentally broken for OpenAI-compatible providers due to the hardcoded test URL. The duplicate switch statements in `testProviderConnection`/`discoverProviderModels` violated DRY and were a maintenance burden. The `/disconnect` command showed auto-registered providers which confused users about what was "connected".
- **Alternatives considered**: Keep switch statements (rejected: violates DRY, hard to maintain), Show all providers in disconnect (rejected: confusing UX), No already-connected detection (rejected: wastes user time)
- **Context**: Full review and refactor of `/connect` and `/disconnect` commands.

## 31. `/model` command displays `modelID/provider` format

- **Decision**: The `/model` palette title now shows `modelID/provider` format (e.g., `gpt-4o/openai`, `ministral-3:14b/ollama`) instead of just the model ID with provider in the description.
- **Rationale**: With multiple providers connected, users need to quickly identify which provider a model belongs to. The `model/provider` format is consistent with the PI-style resolution syntax and makes provider context immediately visible.
- **Alternatives considered**: Keep current format (provider in description) (rejected: less visible), Provider as prefix (rejected: inconsistent with resolution syntax)
- **Context**: `/model` command improvement for multi-provider setups.

## 32. Provider-branded error messages in streaming

- **Decision**: `OpenAICompatProvider.parseStreamResponse()` receives the provider name as a parameter and uses it in all SSE error messages (e.g., "opencode-zen stream error: ..." instead of "OpenAI-compatible stream error: ..."). HTTP status errors in `Stream()` also include the provider name prefix (e.g., "opencode-zen: Invalid API key provided").
- **Rationale**: Users see internal implementation details ("OpenAI-compatible") instead of the provider they configured. Provider-branded errors improve clarity and match PI's behavior. This applies to all providers using `OpenAICompatProvider` (OpenCode Zen, OpenCode Go, OpenRouter, Ollama, llama.cpp, LM Studio).
- **Alternatives considered**: Keep generic "OpenAI-compatible" prefix (rejected: confusing UX), Provider-specific error formatting per provider (rejected: unnecessary complexity, shared implementation)
- **Context**: Task 041 — OpenCode Zen Provider Full Error Handling.

## 34. Tool schema $ref inlining for all providers (Task 044)

- **Decision**: Create shared `sanitizeToolSchema()` function that converts `*jsonschema.Schema` to `map[string]any`, strips meta fields (`$schema`, `$id`), and recursively inlines all `$ref` references from `$defs`. Applied to Anthropic, OpenAI Responses, and OpenAI-compat providers. OpenAI Responses API also conditionally excludes `session_usage` from `include` field when using Zen (not supported). OpenAI tools also include `type: "function"` on tool objects (Responses API requirement).
- **Rationale**: `github.com/invopop/jsonschema` generates `$ref`/`$defs` format for nested types. Anthropic requires `type: "object"` at top level of `input_schema` and rejects `$ref`. OpenAI Responses API also rejects `$ref`. OpenAI-compat providers (Ollama, etc.) may also reject `$ref`. Shared function avoids duplication and ensures consistent behavior across all providers. The `session_usage` field is only supported by official OpenAI API — Zen's OpenAI endpoint rejects it with a validation error.
- **Alternatives considered**: Per-provider schema sanitization (rejected: duplication, maintenance burden), Keep `$ref` and hope providers accept it (rejected: verified failures via curl)
- **Context**: Task 044 — Zen Provider Anthropic & OpenAI Error Handling. Discovered during Task 043 manual verification.

## 33. OpenCode Zen multi-endpoint routing via ZenProvider wrapper (Task 042)

- **Decision**: Create `ZenProvider` wrapper that holds four sub-providers (`OpenAIProvider`, `AnthropicProvider`, `GoogleProvider`, `OpenAICompatProvider`) and dispatches based on `model.API`. Model classification by ID prefix: `gpt-*` → Responses API, `claude-*` → Messages API, `gemini-*` → Google Generative AI, others → Chat Completions. `GoogleProvider` extended with `authMode` field (`"key-param"` | `"bearer"`) for Zen gateway auth. `DiscoverZenModels()` fetches `/v1/models`, classifies each model, and registers with correct API type, reasoning support, context window, max tokens, and cost.
- **Rationale**: OpenCode Zen routes different model families to different API endpoints with different request/response formats. A single `OpenAICompatProvider` with `/chat/completions` only works for OpenAI-compatible models. The wrapper approach keeps the provider interface clean — `Registry.Get("opencode-zen")` returns one provider, model resolution unchanged, and `model.Provider` is always `"opencode-zen"`. GoogleProvider extension for Bearer auth is minimal (one field, one conditional in `Stream()`).
- **Alternatives considered**: Register 4 separate providers (`zen-gpt`, `zen-claude`, etc.) (rejected: complicates model resolution and `/connect` flow), Extract generic `MultiEndpointProvider` (rejected: over-engineering for a single use case, YAGNI), Create separate `ZenGoogleProvider` (rejected: ~80% code overlap with GoogleProvider)
- **Context**: Task 042 — OpenCode Zen All Models Support. Zen docs: `https://opencode.ai/docs/zen`

## 35. OpenRouter provider via OpenAICompatProvider composition (Task 045)

- **Decision**: `OpenRouterProvider` embeds `baseProvider` and composes `OpenAICompatProvider` internally for streaming/parsing. OpenRouter-specific behavior layered on top: (1) attribution headers (`HTTP-Referer`, `X-OpenRouter-Title`, `X-OpenRouter-Categories`) in every request, (2) thinking level mapped to `{ "reasoning": { "effort" } }` format in request body, (3) provider routing preferences from `model.Compat["routing"]` passed as `provider` object. `OpenAICompatConfig` extended with `ThinkingLevel` and `ProviderRouting` fields. Curated model list of 20 popular models with accurate pricing/context windows. User-configurable models via `config.json` `providers.openrouter.models` array.
- **Rationale**: OpenRouter is OpenAI Chat Completions compatible, so reusing `OpenAICompatProvider` avoids duplicating SSE parsing, delta accumulation, and tool call handling. Composition over inheritance — `OpenRouterProvider.Stream()` creates a fresh `OpenAICompatProvider` with OpenRouter-specific config per call, enabling per-request thinking level and routing. Attribution headers are required by OpenRouter's spec. Curated list avoids the 300+ model noise while covering the most popular models. User-defined models provide flexibility for new/niche models.
- **Alternatives considered**: Enhance `OpenAICompatProvider` directly with OpenRouter support (rejected: pollutes generic provider with provider-specific logic), Separate full implementation (rejected: unnecessary duplication of SSE parsing), Fetch models from OpenRouter API at startup (rejected: adds latency, API rate limits, curated list is sufficient)
- **Context**: Task 045 — OpenRouter Provider Support

## 36. Palette SelectionHandler must not close palette when transitioning to task step

- **Decision**: In `executePaletteSelection()`, after calling the SelectionHandler, check if the palette transitioned to another step (task, multi-step, message, or confirm) before closing. If so, keep the palette open to allow the new step to complete.
- **Rationale**: The `/resume` command's SelectionHandler calls `ShowTask()` to start a background task, but `executePaletteSelection()` was closing the palette immediately after the handler returned. This caused `TaskResultMsg` to be ignored (since `m.paletteActive` was false), preventing `handleResumeComplete()` from running and leaving the viewport empty after session selection.
- **Alternatives considered**: Have SelectionHandler return a flag indicating whether to close (rejected: changes interface, breaks existing handlers), Always keep palette open after SelectionHandler (rejected: breaks flows where handler is terminal)
- **Context**: `/resume` command — chat history not rendering after selecting a session to resume.

## 37. Session resume reuses existing SDK infrastructure instead of re-creating session

- **Decision**: Added `Session.ResumeSession(filePath string)` method to the SDK that swaps the internal session file and loads messages into the existing agent, reusing all existing infrastructure (provider registry, model, tools). The TUI's `resumeSessionTask` calls this method instead of `sdk.CreateSession()`.
- **Rationale**: `sdk.CreateSession()` re-registers all providers on every call, making HTTP requests to Ollama (2s timeout), OpenCode Zen (10s timeout), OpenRouter (15s timeout), etc. During resume, this caused the TUI to hang for 10-37 seconds while waiting for provider registration to complete. `ResumeSession()` only opens the session file and loads messages — no network calls, instant completion.
- **Alternatives considered**: Add `SkipProviderRegistration` flag to `SessionOptions` (rejected: still creates unnecessary objects), Cache provider registry globally (rejected: complicates lifecycle, session isolation), Keep `CreateSession` but add progress indicator (rejected: doesn't solve the underlying problem)
- **Context**: `/resume` command hung after selecting a session — spinner showed "Resuming session..." but task never completed due to slow provider re-registration.

## 38. SelectionHandler must return the tea.Cmd from ShowTask

- **Decision**: The SelectionHandler callback in `cmdResume` must return the `tea.Cmd` from `palette.ShowTask()`, not `nil`. This cmd is what starts the background goroutine that executes the task function.
- **Rationale**: `ShowTask()` calls `task.Init()` which returns a `tea.Batch(cmd, func() tea.Msg { ... })`. The anonymous function runs the task in a goroutine and sends `TaskResultMsg` when done. If the handler discards this cmd (returns `nil`), the goroutine is never started and the palette spinner hangs indefinitely.
- **Alternatives considered**: Have `ShowTask` schedule the task internally without returning a cmd (rejected: breaks bubbletea's command model, makes testing harder)
- **Context**: `/resume` command showed "Resuming session..." spinner but nothing happened — task goroutine was never started because handler returned `nil` instead of the cmd from `ShowTask()`.

# DECISIONS

<!-- Architectural and design decisions log -->

## 2026-05-25 — Canonical Tool Lifecycle Contract Normalizes Provider Tool Metadata

- **Decision**: Tau will introduce a canonical typed tool lifecycle contract at the agent boundary, including stable tool call IDs, explicit lifecycle semantics, and native-vs-inferred completion metadata. TUI tool-call rendering will consume this canonical data instead of provider-leaking ad hoc maps.
- **Rationale**: Tau already has compact tool argument formatting for many tools, but provider lifecycle inconsistencies prevent those summaries from reaching the TUI reliably. OpenAI and Ollama provide richer tool-call completion data, while Anthropic and Google currently do not. Canonical agent-layer normalization avoids provider-specific TUI logic, enables correct correlation for repeated/concurrent same-tool calls, and creates a safe place to centralize sanitization and truncation.
- **Alternatives considered**: Keep current `map[string]any` event payloads and patch individual TUI cases (rejected: preserves weak semantics and provider leakage), Fix only provider end events without typed payloads (rejected: improves happy path but keeps lifecycle ambiguity and compatibility risk), Move normalization into TUI (rejected: wrong layer, increases UI complexity and provider coupling).
- **Context**: Task 064 — user requested tool calls in the chat window to show actionable metadata such as file paths, commands, and URLs instead of only bare tool names or placeholders.

## 2026-05-25 — Interrupted Tool Calls Get Synthetic Tool Results

- **Decision**: If the agent is interrupted while executing tool calls, it appends a synthetic error `tool_result` message for each in-flight tool call before returning the cancellation error.
- **Rationale**: OpenAI Responses requires every `function_call` in conversation history to have a matching `function_call_output` before the next request. Interrupting after an assistant tool-call message but before tool results left the in-memory transcript invalid, causing `continue` to fail with `No tool output found for function call`.
- **Alternatives considered**: Drop the assistant tool-call message on cancellation (rejected: loses visible transcript state and is harder to make safe), Wait for all tools to finish after interruption (rejected: violates user interruption intent), Only handle this in OpenAI serialization (rejected: the transcript invariant should be provider-independent).
- **Context**: Task 062 — failure observed during `web-deep-research` after interrupting parallel `subagent` tool calls and then continuing.

## 2026-05-25 — Web Deep Research Uses Ledger and Reconciliation Gates

- **Decision**: The `web-deep-research` skill now requires a canonical `00-candidate-ledger.md`, per-angle candidate/entity deltas, a pre-synthesis `03-reconciliation.md`, and semantic coverage verification. The final report goal is a comprehensive sourced report with concise prose, not a narrow recommendation memo.
- **Rationale**: Evaluation showed that a discovered and shortlisted candidate could disappear from the final report without explicit rejection. A ledger and reconciliation gate make tracked entities the controlling artifact for synthesis and force explicit dispositions for recommended, conditional, excluded, unresolved, and benchmark-only entities.
- **Alternatives considered**: Add runtime orchestration code (rejected for this iteration: prompt/artifact gates are smaller and preserve Agent Skills compatibility), Add only a reviewer subagent (rejected: useful but too freeform without a ledger), Keep report terse and recommendation-focused (rejected: user wants wide research with proven information).
- **Context**: Task 060 — `web-deep-research` reliability improvement after analysis in `docs/research/web-deep-research-skill-evaluation-may-2026.md`.

## 2026-05-25 — Subagent Timeout Floor Matches Default

- **Decision**: Subagent execution now enforces a 5 minute minimum timeout and falls back to `subagent.DefaultTimeout` when no configured timeout is available. The documented `subagent_timeout` string form (for example, `"5m"`) is accepted by config parsing.
- **Rationale**: A 2 minute minimum was too short for delegated tasks that require multiple model/tool iterations, especially local or slower models. Tau already documented a 5 minute default, so the effective minimum should match that expectation instead of letting model-generated `2m` values cancel useful work early.
- **Alternatives considered**: Keep 2 minute minimum and only change the tool prompt (rejected: existing model behavior can still emit short timeouts), Remove subagent timeout entirely (rejected: bounded execution is still needed), Increase default above 5 minutes (rejected: user can configure longer values explicitly).
- **Context**: User reported subagents still failing with `subagent: execution timed out after 2m0s: context deadline exceeded` after the OpenAI replay fix.

## 2026-05-24 — OpenAI Responses Function Call Replay Omits Item IDs

- **Decision**: OpenAI Responses follow-up request serialization preserves `function_call.call_id` but omits the provider item `id` (`fc_...`) when replaying previous assistant tool calls.
- **Rationale**: Reasoning-capable OpenAI Responses models can require a `function_call` item ID to be replayed with its paired native `reasoning` item (`rs_...`). Tau currently stores reasoning summaries as `BlockThinking` display text, not native encrypted reasoning items. Replaying `fc_...` without the matching `rs_...` causes a 400 Bad Request. `call_id` is sufficient for linking `function_call_output` tool results, so omitting `id` is the smallest safe fix.
- **Alternatives considered**: Persist and replay native OpenAI Responses reasoning items and encrypted content (rejected for this fix: broader schema/storage change), Drop composite `call_id|item_id` entirely (rejected: keeping it preserves parse-time information and existing tool result stripping logic).
- **Context**: User reported subagent call failure followed by OpenAI error: `function_call` item was provided without required `reasoning` item.

## 2026-05-24 — Deterministic Model Fallback

- **Decision**: Model selection fallback is now deterministic and only considers connected providers:
  - `resolveModel()` helper implements priority: explicit CLI > valid resumed session > valid config default > sorted connected fallback
  - Fallback sorts candidates by provider name then model ID (alphabetical) before selecting
  - Only models from registered/connected providers are considered for fallback
  - Explicit CLI model requests do NOT silently fall back (user should know their request failed)
  - `ResumeSession` falls back to config default when resumed session model provider is unavailable
  - **New session model selection**: When creating a new session (not resume/continue), tau now checks the most recent session file for its model and uses it as a fallback before config default. This ensures that when a user sets a model via `/model` and then restarts tau, the new session picks up the most recent session's model.
- **Rationale**: Previous fallback iterated `ModelRegistry.ListAll()` which iterates a Go map (non-deterministic order), picking "first Ollama model" unpredictably. This caused Tau to open with random `ollama/ministral` or `ollama/gemma` models instead of the user's saved `/model` choice. `ResumeSession` also kept the current model when the resumed model was unavailable, ignoring the user's configured `default_model`. Additionally, new sessions didn't check the most recent session file for its model, causing tau to fall back to config default or random models instead of the user's last-used model.
- **Alternatives considered**: Use provider-specific priority list (rejected: adds maintenance burden, alphabetical sort is simpler and deterministic), Always fall back even for explicit CLI requests (rejected: silent fallback hides configuration problems from users), Store last-used model in a separate file (rejected: session file already has this info, no need for duplication)
- **Context**: User reported model remembering was unreliable — sometimes correct, sometimes random Ollama models.

## 2026-05-24 — Model Resume Reliability

- **Decision**: Session selection and model restoration now use file modification time and saved session state:
  - `LatestSessionFile` selects sessions by file modification time (mtime), not filename creation timestamp
  - Filename is used as a deterministic tie-breaker when mtimes are equal
  - `ResumeSession` restores the saved model and provider from the resumed session's `model_change` entry
  - Compaction entries are written as `CompactionData` directly (not double-wrapped `SessionEntry`)
- **Rationale**: Previous `LatestSessionFile` sorted by filename string, which encodes creation time to second precision. If two sessions were created in the same second, the random ID suffix decided the winner unpredictably. Sessions modified after creation (e.g., via `/resume`) were not selected as "latest". `ResumeSession` ignored the saved model, keeping the runtime model instead. The compaction bug caused malformed data that could not be read back.
- **Alternatives considered**: Use session header timestamp instead of file mtime (rejected: requires reading every file, mtime is simpler and matches PI's approach), Store creation time in header and sort by that (rejected: doesn't solve the "modified after creation" problem)
- **Context**: User reported model remembering was unreliable — sometimes correct, sometimes not.

## 2026-05-24 — OpenAI Responses API Tool Call Fix

- **Decision**: Fixed OpenAI Responses API provider to correctly parse streaming tool call events and properly format tool results for multi-turn conversations.
  - SSE event parsing now handles nested `item` field in `response.output_item.added` events (OpenAI sends `{"item": {"id": "fc_xxx", "type": "function_call", "call_id": "call_xxx", ...}}`)
  - Tool call IDs use composite format `call_id|item_id` to preserve both identifiers required by the Responses API
  - Tool results are now sent as `{"type": "function_call_output", "call_id": "...", "output": "..."}` items instead of plain user messages
  - Assistant messages with tool calls are serialized as proper Responses API input items (separate `message` and `function_call` items)
- **Rationale**: The previous implementation parsed `response.output_item.added` data as a flat object with `id`, `type`, `name` at the top level. OpenAI actually nests these fields under an `item` key. This caused tool calls to be silently dropped — the model would think, emit a tool call, Tau would miss it, see no tool calls in the response, and stop the agent loop. Additionally, tool results were sent as plain user messages instead of proper `function_call_output` items, which the Responses API requires for correct conversation continuation.
- **Alternatives considered**: Use `response.output_item.done` instead of `added` for tool call parsing (rejected: `added` is the correct event for starting tool call accumulation), Send tool results as user messages with special formatting (rejected: Responses API requires `function_call_output` items)
- **Context**: User reported OpenAI models stopping after thinking block instead of executing tools and continuing the agent loop.

## 2026-05-24 — Model Selection Persistence via Canonical Provider/ModelID

- **Decision**: Model identity is now persisted and restored as a canonical `provider/modelID` pair everywhere:
  - `config.json` `default_model` stores `provider/modelID` (e.g., `anthropic/claude-sonnet-4-20250514`)
  - Session `model_change` entries store both `model_id` and `provider`
  - `/model` command passes `provider/modelID` to `SetModel`
  - `ModelRegistry` uses compound `provider/modelID` keys internally, allowing same model ID under multiple providers
  - `CreateSession` restores model from resumed session with priority: explicit CLI > resumed session > config default > auto fallback
  - `SetModel` reloads config before saving to avoid clobbering `/connect`/`/disconnect` changes
- **Rationale**: Previous implementation stored only bare model ID, causing provider identity loss across restarts. Same model ID under different providers could not be distinguished. Config writes from cached startup state clobbered provider config changes made during the session.
- **Backward compatibility**: Old session files with only `model_id` (no provider) still work — resolver falls back to smart disambiguation. Old config.json with bare model IDs still work — resolver handles both formats.
- **Alternatives considered**: Separate `default_provider` + `default_model` config fields (rejected: more complex, canonical ref is simpler), hash-based model identity (rejected: loses human readability)
- **Context**: Task 051 — user reported inconsistent model/provider restoration across Tau restarts.

## 2026-05-23 — Subagent Model Resolution via Provider Registry

- **Decision**: Subagent model resolution now uses a 4-step priority chain with the provider registry instead of blindly inheriting the parent's provider type:
  1. Agent frontmatter model (from user-defined agent `SKILL.md`)
  2. Prompt model (specified in subagent tool call)
  3. Parent agent's model
  4. Subagent default models list (first available provider)
- **Rationale**: The previous implementation inherited the parent's `Provider` and `API` fields when a custom model was specified, causing cross-provider subagent calls to fail. For example, a parent using `claude-sonnet-4-20250514` (anthropic) that spawned a subagent with model `ministral-3:14b` would send the Ollama model ID to the Anthropic API, resulting in `provider stream error`. The fix uses `provReg.ResolveModelWithFallback()` to correctly resolve the model to its actual provider.
- **Alternatives considered**: Require explicit `provider/model` format for all cross-provider calls (rejected: breaks natural model name resolution), auto-detect provider from model ID pattern (rejected: fragile, requires maintaining pattern list)
- **Context**: Bug discovered during web-deep-research skill execution — all 3 researcher subagents failed with identical `provider stream error: model: claude-sonnet-4-20250514`. Analysis saved to `docs/research/subagent-provider-stream-error/`.

## 2026-05-23 — Canonical provider reassignment requires matching model ID

- **Decision**: `reassignToCanonicalProvider()` only reassigns a model to its canonical provider (openai, anthropic, google) if the canonical provider already has a model with the exact same ID in the catalog. Previously, any model matching the ID prefix pattern (e.g., `gpt-*` → openai) was reassigned regardless of whether the canonical provider had that model.
- **Rationale**: Proxy providers in the models.dev catalog use non-standard model IDs (e.g., `gpt-5-5` with hyphens instead of `gpt-5.5` with dots, `openai-gpt-55`, `databricks-gpt-5-5`). When these were blindly reassigned to the canonical provider, the model ID stayed non-standard but the BaseURL pointed to the real API (e.g., `https://api.openai.com/v1`). This caused `Bad request: The requested model 'gpt-5-5' does not exist` errors when calling the real OpenAI API. The fix ensures only models that the canonical provider actually offers get reassigned.
- **Alternatives considered**: Normalize proxy model IDs to canonical format (rejected: fragile, would need mapping for every proxy variant), Blocklist non-standard ID patterns (rejected: maintenance burden, new patterns emerge constantly)
- **Context**: Bug discovered when user selected `gpt-5.5` but config.json saved `gpt-5-5` from frogbot provider, which was incorrectly reassigned to openai.

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

- **Decision**: Create shared `sanitizeToolSchema()` function that converts `*jsonschema.Schema` to `map[string]any`, strips meta fields (`$schema`, `$id`), and recursively inlines all `$ref` references from `$defs`. Applied to Anthropic, OpenAI Responses, and OpenAI-compat providers. OpenAI tools include `type: "function"` on tool objects (Responses API requirement). The `include` field only uses valid OpenAI Responses API values (`reasoning.encrypted_content` for reasoning models) — `session_usage` is not a supported value and was removed.
- **Rationale**: `github.com/invopop/jsonschema` generates `$ref`/`$defs` format for nested types. Anthropic requires `type: "object"` at top level of `input_schema` and rejects `$ref`. OpenAI Responses API also rejects `$ref`. OpenAI-compat providers (Ollama, etc.) may also reject `$ref`. Shared function avoids duplication and ensures consistent behavior across all providers. The `session_usage` value was incorrectly added — OpenAI Responses API rejects it with: `Invalid value: 'session_usage'. Supported values are: 'file_search_call.results', 'web_search_call.results', ...`
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

## 39. Mouse-based text selection with auto-scroll and clipboard copy

- **Decision**: Implement mouse-based text selection in the TUI viewport with auto-scroll during drag and clipboard copy on release. Uses `MouseModeCellMotion` to capture click/drag/release events. Selection state tracks start/end line/column. Auto-scroll triggers when cursor is within 3 rows of viewport top/bottom edge. Clipboard copy uses OSC 52 escape sequence (works in Kitty, Ghostty, Alacritty, WezTerm, iTerm2, tmux) with fallback to `wl-copy`/`xclip`/`xsel` on Linux, `pbcopy` on macOS, `clip` on Windows. Visual highlighting uses viewport's built-in `SetHighlights` API.
- **Rationale**: Users couldn't copy text from the chat window when content exceeded visible screen height because the alternate screen buffer doesn't support native terminal selection scrolling. OpenCode and PI both support this — OpenCode via `@opentui/core` framework, PI via terminal emulator native selection. Bubble Tea v2's `MouseModeCellMotion` provides the necessary mouse event capture (click, motion with button held, release).
- **Alternatives considered**: Switch to normal screen buffer (rejected: breaks fullscreen TUI experience), Use terminal's native selection (rejected: doesn't work in alternate screen buffer), Implement custom scroll-on-selection in viewport content (rejected: viewport's `SetHighlights` provides built-in support)
- **Context**: User request — "if text fragment is longer than visible on the screen, i cant because chat window doesnt scroll when i reach edge of the screen with cursor"

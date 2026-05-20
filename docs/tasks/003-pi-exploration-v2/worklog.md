# Task 003: PI Exploration v2 — Worklog

## Session: 2026-05-02 — Deep PI Architecture Analysis

### Overview

Systematically explored PI's source code (via `.d.ts` type definitions and `.js` implementation files) across all three packages:
- `@mariozechner/pi-ai` — Provider/model abstraction layer
- `@mariozechner/pi-agent-core` — Agent loop, types, tools
- `@mariozechner/pi-coding-agent` (main package) — Session management, extensions, skills, SDK, TUI

Read and analyzed: `index.d.ts`, `agent-session.d.ts`+`.js`, `session-manager.d.ts`+`.js`, `skills.d.ts`+`.js`, `model-registry.d.ts`+`.js`, `sdk.d.ts`+`.js`, `resource-loader.d.ts`+`.js`, `messages.d.ts`+`.js`, `agent-loop.d.ts`+`.js` (pi-agent-core), `types.d.ts` (pi-agent-core + extensions), `compaction/*.d.ts`+`.js`, `tools/*.d.ts`+`.js`, `register-builtins.d.ts`, `models.d.ts`.

---

### 003.1 — Sub-Agent Patterns

**Finding**: PI does NOT have built-in sub-agent support. Sub-agents are implemented exclusively via extensions.

**How extensions implement sub-agent-like behavior**:
1. **Extension API** provides `pi.exec()` for shell command execution
2. **Event system** (`EventBus`) allows extensions to communicate
3. **Custom tools** can be registered via `pi.registerTool()` — an extension could register a "spawn subagent" tool
4. **Custom messages** (`CustomMessage`) allow extensions to inject arbitrary message types into conversation
5. **`appendEntry()`** lets extensions store custom data in session files

**Context model implications**:
- PI's session system supports tree/branching via `SessionManager.branch()` and `SessionManager.branchWithSummary()`
- `SessionManager.createBranchedSession()` extracts a single conversation path into a new file
- `SessionManager.forkFrom()` copies history from another project directory
- `CustomEntry` type allows extensions to persist state without polluting LLM context
- `CustomMessageEntry` allows extensions to inject messages INTO LLM context

**Verified from source code**: `session-manager.js` confirms tree structure with id/parentId, leaf pointer, branch/branchWithSummary/createBranchedSession methods. The `buildSessionContext()` function walks from leaf to root via parent pointers, handling compaction and branch summaries.

**Go-native sub-agent design implications**:
- No need for extension system — build sub-agents as first-class citizens
- Use a simple `SubAgent` struct with: `Task string`, `Context []AgentMessage`, `Tools []Tool`, `Result chan Result`
- Context isolation: each subagent gets its own message slice (fork pattern)
- Results returned via channel/callback, not appended to main conversation
- Parent-child only: no subagent-to-subagent communication (matches our requirement)
- Fresh context by default (matches our requirement)
- Side-task pattern: parent can spawn one-shot subagent with cloned context

---

### 003.2 — Skill System

**Skill Discovery** (verified from `skills.js` source):
- **Global**: `~/.pi/agent/skills/` (via `agentDir`)
- **Project**: `.agents/skills/` in cwd
- **Explicit**: `--skill` CLI flag paths
- Uses `.gitignore`, `.ignore`, `.fdignore` files for exclusion patterns
- Skips `node_modules` directories

**Skill Format** (Agent Skills standard — verified from source):
```yaml
---
name: skill-name
description: What this skill does
disable-model-invocation: false
---
# Markdown content with instructions
```

**Validation rules** (verified from `skills.js`):
- Name must match parent directory name
- Name max 64 chars, lowercase a-z, 0-9, hyphens only
- No leading/trailing hyphens, no consecutive hyphens
- Description required, max 1024 chars

**Discovery Rules** (verified from source):
- If directory contains `SKILL.md`, treat as skill root — do NOT recurse further
- Otherwise, load direct `.md` children in root
- Recurse into subdirectories to find `SKILL.md` files
- Symlinks are followed (statSync check)

**Skill Prompt Integration** (verified from `skills.js`):
- `formatSkillsForPrompt(skills)` — formats skills as XML for system prompt
- Skills with `disableModelInvocation=true` excluded from prompt
- Progressive disclosure: only name + description shown in system prompt

**Cross-tool compatibility**:
- Agent Skills standard (agentskills.io) — SKILL.md with frontmatter + markdown
- Same format works across PI, OpenCode, Claude Code

**Go implications**:
- Parse SKILL.md frontmatter (YAML) + markdown body
- Discovery paths: `~/.tau/skills/`, `~/.agents/skills/`, `.agents/skills/`
- `/skill:name` command to explicitly load
- Progressive disclosure in system prompt
- Built-in skills: `skill-builder`, `subagent-builder`

---

### 003.3 — Agent Loop

**Architecture layers** (verified from `agent-loop.js` source):
1. **`agentLoop()` / `agentLoopContinue()`** — Low-level loop in `pi-agent-core` (448 lines)
2. **`Agent` class** — Stateful wrapper around the loop, owns transcript, emits events
3. **`AgentSession`** — High-level session management (2543 lines), adds persistence, compaction, steering

**Core Loop Flow** (verified from `runLoop()` in agent-loop.js):
```
Outer while loop:
  Inner while loop (process tool calls + steering messages):
    Process pending steering messages (inject before next assistant response)
    streamAssistantResponse():
      transformContext (optional) → AgentMessage[] filtering
      convertToLlm (AgentMessage[] → Message[])
      streamSimple(model, llmContext) → event stream
      Emit: message_start → message_update (streaming) → message_end
    If tool calls in response:
      Sequential mode: execute one-by-one with emit per tool
      Parallel mode: preflight sequentially, execute concurrently via Promise.all
      For each tool: prepareToolCall → beforeToolCall hook → execute → afterToolCall hook
      Emit: tool_execution_start → tool_execution_update → tool_execution_end
      Create ToolResultMessage, add to context
      Check terminate flag from tool results
    Emit turn_end
    Check steering queue for new messages
  If no more tool calls: check follow-up queue
  If follow-ups exist: continue outer loop
  Otherwise: emit agent_end, return
```

**Key Verified Interfaces**:
```typescript
// agent-loop.js exports:
agentLoop(prompts, context, config, signal?, streamFn?) → EventStream
agentLoopContinue(context, config, signal?, streamFn?) → EventStream

// AgentLoopConfig:
convertToLlm: (messages: AgentMessage[]) => Message[]
transformContext?: (messages, signal) => Promise<AgentMessage[]>
getApiKey?: (provider) => Promise<string | undefined>
getSteeringMessages?: () => Promise<AgentMessage[]>
getFollowUpMessages?: () => Promise<AgentMessage[]>
toolExecution?: "sequential" | "parallel"
beforeToolCall?: (context, signal) => Promise<BeforeToolCallResult>
afterToolCall?: (context, signal) => Promise<AfterToolCallResult>
```

**Events emitted** (verified): `agent_start`, `agent_end`, `turn_start`, `turn_end`, `message_start`, `message_update`, `message_end`, `tool_execution_start`, `tool_execution_update`, `tool_execution_end`

**Orchestrator Pattern** (verified from AgentSession):
- `_handleAgentEvent()` processes events through `_agentEventQueue` (Promise chain for ordering)
- `message_end` events trigger session persistence via `sessionManager.appendMessage()`
- `agent_end` events trigger auto-compaction check and auto-retry check
- `steer()` / `followUp()` queue messages for injection at appropriate points

**Go implications**:
- Event-driven loop using channels for streaming
- `Agent` struct owns: `messages []Message`, `tools []Tool`, `model Model`
- `AgentLoop` function: `func AgentLoop(ctx context.Context, config AgentConfig) error`
- Hooks: `BeforeToolCall`, `AfterToolCall` as function fields
- Steering/follow-up via buffered channels
- Event queue via sequential goroutine processing (like PI's Promise chain)

---

### 003.4 — Provider Abstraction

**Model Type** (verified from `types.d.ts`):
```typescript
interface Model<TApi extends Api> {
    id: string;
    name: string;
    api: TApi;
    provider: Provider;
    baseUrl: string;
    reasoning: boolean;
    input: ("text" | "image")[];
    cost: { input, output, cacheRead, cacheWrite };
    contextWindow: number;
    maxTokens: number;
    headers?: Record<string, string>;
    compat?: OpenAICompletionsCompat | OpenAIResponsesCompat | AnthropicMessagesCompat;
}
```

**API Types** (verified): `openai-completions`, `openai-responses`, `anthropic-messages`, `google-generative-ai`, `google-vertex`, `bedrock-converse-stream`, `mistral-conversations`, `azure-openai-responses`, `openai-codex-responses`

**Built-in Providers** (25 total): amazon-bedrock, anthropic, google, google-vertex, openai, azure-openai-responses, openai-codex, deepseek, github-copilot, xai, groq, cerebras, openrouter, vercel-ai-gateway, zai, mistral, minimax, moonshotai, huggingface, fireworks, opencode, opencode-go, kimi-coding, cloudflare-workers-ai, cloudflare-ai-gateway

**Auth Resolution** (verified from `model-registry.js`):
- Uses `resolveConfigValueOrThrow()` for API key resolution
- Supports environment variables, command-based config, OAuth
- `AuthStorage` persists credentials to `auth.json`
- `getApiKeyAndHeaders(model)` resolves both API key and custom headers per model
- OAuth token refresh handled by provider-specific login flow

**OpenAI Compatibility Layer** (verified from `model-registry.js` schema):
- `OpenAICompletionsCompat`: supportsStore, supportsDeveloperRole, supportsReasoningEffort, reasoningEffortMap, thinkingFormat (openai/openrouter/deepseek/zai/qwen/qwen-chat-template), cacheControlFormat, requiresToolResultName, requiresAssistantAfterToolResult, requiresThinkingAsText, requiresReasoningContentOnAssistantMessages
- `OpenAIResponsesCompat`: sendSessionIdHeader, supportsLongCacheRetention
- `AnthropicMessagesCompat`: supportsEagerToolInputStreaming, supportsLongCacheRetention

**Go implications**:
- `Provider` interface: `Stream(ctx, model, messages, tools) <-chan Event`
- `Model` struct with metadata
- Registry pattern for provider discovery
- Auth resolution chain: env → config file → OAuth
- Compatibility layer for OpenAI-compatible providers (essential for our 8 target providers)
- Our target providers: OpenAI (openai-responses), Anthropic (anthropic-messages), Gemini (google-generative-ai), OpenCode Zen, OpenCode Go, OpenRouter, Ollama (openai-completions compat), llama.cpp/LM Studio (openai-completions compat)

---

### 003.5 — Session Storage

**Format**: JSONL (JSON Lines) — verified from `session-manager.js` source

**Session Header**:
```json
{"type":"session","version":3,"id":"uuid","timestamp":"2026-05-02T...","cwd":"/path","parentSession":"/path"}
```

**Entry Types** (verified):
1. `SessionMessageEntry` — LLM messages
2. `ThinkingLevelChangeEntry` — thinking level changes
3. `ModelChangeEntry` — model switches
4. `CompactionEntry` — compaction summaries (with `firstKeptEntryId`, `tokensBefore`, `details`)
5. `BranchSummaryEntry` — branch abandonment summaries
6. `CustomEntry` — extension data (NOT in LLM context)
7. `CustomMessageEntry` — extension messages (IN LLM context)
8. `LabelEntry` — user bookmarks
9. `SessionInfoEntry` — session display name

**Tree Structure** (verified from source):
- Each entry has `id` (short UUID, 8 hex chars) and `parentId` (null for root)
- `leafId` pointer tracks current position
- `appendMessage()` creates child of current leaf, advances leaf
- `branch(fromId)` moves leaf to earlier entry
- `branchWithSummary(fromId, summary)` — branch + append branch_summary entry
- `createBranchedSession(leafId)` — extract path to new JSONL file
- Migrations: v1→v2 (add id/parentId), v2→v3 (rename hookMessage to custom)

**Context Building** (verified from `buildSessionContext()` in session-manager.js):
- Walks from leaf to root via parent pointers
- Collects thinkingLevel, model from path entries
- Handles compaction: emits summary first, then kept messages (from firstKeptEntryId), then post-compaction messages
- Handles branch summaries: converts to BranchSummaryMessage

**Session Directory**: `~/.pi/agent/sessions/<encoded-cwd>/`
- CWD encoded into safe directory name via `getDefaultSessionDir()`

**Compaction** (verified from `compaction.js`):
- **Settings**: `enabled: true`, `reserveTokens: 16384`, `keepRecentTokens: 20000`
- **Trigger**: `shouldCompact(contextTokens, contextWindow, settings)` — when context exceeds (window - reserveTokens)
- **Process**:
  1. `findCutPoint()` — walks backwards accumulating token estimates (chars/4 heuristic)
  2. `prepareCompaction()` — identifies messages to summarize, turn prefix, file operations
  3. `generateSummary()` — LLM call with summarization system prompt
  4. Replace summarized entries with compaction summary in `<summary>...</summary>` XML tags
- **Iterative**: Previous compaction's details (readFiles, modifiedFiles) preserved and merged
- **Split turns**: Can cut mid-turn, keeps tool results with the turn

**Persistence** (verified):
- Append-only via `appendFileSync` with `\n` separator
- `_persist()` called immediately on each append
- `CURRENT_SESSION_VERSION = 3`
- Migration handled at load time

**Go implications**:
- JSONL format — simple, append-only, portable
- Tree via id/parentId — easy in Go
- Compaction as pure function (separate from I/O) — verified in PI source
- Token estimation: `chars/4` heuristic (conservative, overestimates)
- Session directory: `~/.tau/sessions/<encoded-cwd>/`
- For MVP: no branching, simple append-only log with compaction

---

### 003.6 — Tool System

**Tool Definition** (verified from `tools/*.d.ts`):
```typescript
// pi-ai base:
interface Tool<TParameters extends TSchema> {
    name: string;
    description: string;
    parameters: TParameters;  // TypeBox schema
}

// pi-agent-core extension:
interface AgentTool<TParameters, TDetails> extends Tool<TParameters> {
    label: string;
    prepareArguments?: (args: unknown) => Static<TParameters>;
    execute: (toolCallId, params, signal?, onUpdate?) => Promise<AgentToolResult<TDetails>>;
    executionMode?: "sequential" | "parallel";
}
```

**Tool Result**:
```typescript
interface AgentToolResult<T> {
    content: (TextContent | ImageContent)[];
    details: T;
    terminate?: boolean;  // Hint to stop agent
}
```

**Built-in Tools** (verified from `tools/index.d.ts`):
| Tool | Description | Key Params |
|------|-------------|-----------|
| `read` | Read file contents + images | `path`, `offset?`, `limit?` |
| `bash` | Execute shell command | `command` |
| `edit` | Edit file (search/replace) | `path`, `search`, `replace` |
| `write` | Write file | `path`, `content` |
| `grep` | Search file contents | `pattern`, `path?` |
| `find` | Find files by name | `pattern`, `path?` |
| `ls` | List directory | `path?` |

**Tool Creation Factories** (verified):
- `createTool(toolName, cwd, options?)` → AgentTool
- `createToolDefinition(toolName, cwd, options?)` → ToolDefinition (extension format)
- `createCodingTools(cwd)` → [read, bash, edit, write]
- `createReadOnlyTools(cwd)` → [read, grep, find, ls]
- `createAllTools(cwd)` → { read, bash, edit, write, grep, find, ls }

**Tool Execution** (verified from `agent-loop.js`):
- Sequential: each tool prepared, executed, finalized before next
- Parallel: all tools preflighted sequentially, then allowed tools execute via `Promise.all`
- `prepareToolCall`: find tool → prepare args → validate schema → beforeToolCall hook
- `executePreparedToolCall`: call tool.execute() with onUpdate callback for streaming
- `finalizeExecutedToolCall`: afterToolCall hook → create ToolResultMessage

**File Mutation Queue** (verified from `file-mutation-queue.js`):
- `withFileMutationQueue()` wraps tool operations
- Uses per-file queue (Map of promises) to serialize writes to same file
- Prevents race conditions when multiple tools write same file

**Truncation** (verified from `truncate.js`):
- `DEFAULT_MAX_LINES` and `DEFAULT_MAX_BYTES` limits
- `truncateHead`, `truncateTail`, `truncateLine` utilities
- Truncation details included in tool result for UI display

**Extension Tool Registration** (verified from extensions/types.d.ts):
- `pi.registerTool(definition)` with ToolDefinition including: name, label, description, parameters (TypeBox), execute, optional renderCall/renderResult, promptSnippet, promptGuidelines

**Go implications**:
- Tool interface: `Name()`, `Description()`, `Parameters()`, `Execute(ctx, params) Result`
- Parameters as Go structs (type system replaces TypeBox)
- Tool registry: `map[string]Tool`
- Parallel execution via goroutines with `errgroup`
- File mutation queue via mutex per file path
- Truncation utilities for oversized output

---

### 003.7 — Go Package Structure Mapping

**PI's 3-package architecture**:
```
@mariozechner/pi-ai          → Provider/model abstraction
@mariozechner/pi-agent-core  → Agent loop, tools, types
@mariozechner/pi-coding-agent → Session, extensions, SDK, TUI
```

**Proposed Go package structure for Tau**:

```
tau/
├── cmd/tau/              → CLI entry point (main.go)
├── internal/
│   ├── agent/               → Agent loop, agent state, events
│   │   ├── agent.go         → Agent struct (state, transcript, events)
│   │   ├── loop.go          → agentLoop() function
│   │   └── event.go         → AgentEvent types, event bus
│   ├── session/             → Session management, JSONL storage
│   │   ├── session.go       → Session struct, lifecycle
│   │   ├── storage.go       → JSONL read/write, tree structure
│   │   ├── compaction.go    → Compaction logic (pure functions)
│   │   └── context.go       → BuildSessionContext from tree
│   ├── tools/               → Tool definitions and execution
│   │   ├── tool.go          → Tool interface, registry
│   │   ├── read.go          → Read tool
│   │   ├── bash.go          → Bash tool
│   │   ├── edit.go          → Edit tool
│   │   ├── write.go         → Write tool
│   │   ├── grep.go          → Grep tool
│   │   ├── find.go          → Find tool
│   │   ├── ls.go            → Ls tool
│   │   ├── truncate.go      → Output truncation utilities
│   │   └── queue.go         → File mutation queue
│   ├── provider/            → LLM provider abstraction
│   │   ├── provider.go      → Provider interface
│   │   ├── model.go         → Model struct, registry
│   │   ├── openai.go        → OpenAI provider
│   │   ├── anthropic.go     → Anthropic provider
│   │   ├── google.go        → Google Gemini provider
│   │   ├── opencode.go      → OpenCode Zen provider
│   │   ├── opencode_go.go   → OpenCode Go provider
│   │   ├── openrouter.go    → OpenRouter provider
│   │   ├── local.go         → Ollama/llama.cpp/LM Studio
│   │   ├── auth.go          → Auth resolution chain
│   │   └── stream.go        → Streaming event types
│   ├── skills/              → Skill discovery and loading
│   │   ├── skill.go         → Skill struct, loading
│   │   ├── discovery.go     → Directory scanning, frontmatter parsing
│   │   └── prompt.go        → formatSkillsForPrompt
│   ├── subagent/            → Sub-agent system (NEW — not in PI)
│   │   ├── subagent.go      → SubAgent struct, lifecycle
│   │   ├── context.go       → Context fork/cloning
│   │   └── result.go        → Result handling
│   ├── config/              → Configuration management
│   │   ├── config.go        → Settings, loading
│   │   └── paths.go         → Directory paths
│   └── sdk/                 → SDK for embedding
│       └── sdk.go           → CreateSession, high-level API
└── pkg/                     → Public packages (if any)
```

**Dependency graph**:
```
cmd/tau → internal/sdk
internal/sdk → internal/agent, internal/session, internal/provider, internal/tools, internal/skills, internal/subagent, internal/config
internal/agent → internal/tools, internal/provider
internal/session → internal/agent (messages types)
internal/tools → (stdlib only + os/exec)
internal/provider → (HTTP client, streaming)
internal/skills → (file I/O, YAML frontmatter parsing)
internal/subagent → internal/agent, internal/tools
internal/config → (file I/O, YAML/JSON)
```

**External dependencies** (minimal):
- YAML parsing for skill frontmatter (`gopkg.in/yaml.v3`)
- HTTP client (stdlib `net/http`)
- JSON encoding (stdlib `encoding/json`)
- Token counting: chars/4 heuristic (no external lib needed)

**Key interface boundaries**:
- `Provider` interface: isolate LLM API differences
- `Tool` interface: isolate tool implementations
- `Skill` interface: isolate skill discovery/loading
- `SubAgent` interface: isolate sub-agent orchestration
- `SessionStorage` interface: isolate persistence format

---

### 003.8 — Findings Document

#### Patterns to ADOPT from PI

1. **JSONL session format** — Simple, append-only, portable. Verified in source: uses `appendFileSync` with `\n` separator.
2. **Event-driven agent loop** — Clean two-level loop (outer for follow-ups, inner for tool calls). Go channels perfect fit.
3. **Tool abstraction with typed parameters** — Go structs replace TypeBox naturally.
4. **Provider interface with stream function** — Maps to Go channels: `Stream(ctx) <-chan Event`.
5. **Skill discovery from directories** — SKILL.md format is standard. Verified: respects .gitignore, skips node_modules.
6. **Compaction as pure function** — Verified: `prepareCompaction()` and `compact()` are pure, session manager handles I/O.
7. **Progressive skill disclosure** — Only name+description in system prompt, full content on demand.
8. **Auth resolution chain** — Env vars → config file → OAuth. Clear priority order.
9. **Steering/follow-up message queues** — Clean way to interact with running agent.
10. **BeforeToolCall/AfterToolCall hooks** — Perfect for orchestrator pattern (skill → subagent → skill).
11. **Tree/branching session structure** — id/parentId pattern, leaf pointer. Simple and effective.
12. **Tool result `terminate` flag** — Clean way for tools to signal "stop after this batch".

#### Patterns to ADAPT from PI

1. **Extension system** → **Built-in sub-agents**. PI's extension system is 1168+ lines of types. We'll build sub-agents natively.
2. **TypeBox JSON schemas** → **Go structs**. Compile-time safety vs runtime validation.
3. **EventBus with typed events** → **Go channels + context**. Native concurrency primitives.
4. **Tree/branching sessions** → **Simple append-only for MVP**. Branching deferred post-MVP.
5. **Heavy React/TUI** → **Minimal TUI or CLI-first**. Performance over richness.
6. **npm package ecosystem** → **Single binary, stdlib-first**. No runtime dependencies.
7. **Promise-based event queue** → **Sequential goroutine processing**. Same ordering guarantee, Go-native.

#### Patterns to AVOID from PI

1. **Extension system complexity** — Massive (1168 lines of types alone). Not needed for personal tool.
2. **Heavy dependency tree** — React, Ink, TypeScript runtime, dozens of npm packages. Go single binary is cleaner.
3. **Tree/branching session complexity** — Overkill for single-user MVP.
4. **OAuth complexity** — API keys sufficient initially.
5. **Multi-mode architecture** (interactive/print/RPC) — Start with interactive CLI only.
6. **Package manager / skill marketplace** — Out of scope per requirements.
7. **TypeBox runtime validation** — Go compile-time types eliminate this.
8. **Complex message transformation pipeline** — PI has CustomAgentMessages with declaration merging. Go uses interfaces + type assertions.

#### Go-Specific Implementation Notes

1. **Agent loop**: `context.Context` for cancellation, `chan Event` for streaming
2. **Provider interface**: `Stream(ctx, model, messages, tools) <-chan StreamEvent`
3. **Tool interface**: Struct with `Name()`, `Description()`, `Execute(ctx, params) Result`
4. **Session storage**: Append to JSONL file, read with `bufio.Scanner`
5. **Compaction**: Pure function — takes messages, returns summary + cutoff index
6. **Sub-agents**: Goroutine per subagent, results via channels
7. **Skills**: `os.ReadDir` + YAML frontmatter parsing
8. **Auth**: `os.Getenv` → config file → fallback
9. **TUI**: Start with simple readline, upgrade later

#### Key Architecture Risks

1. **Token estimation** — PI uses chars/4 heuristic. May need better estimation for compaction triggers.
2. **Context window management** — Different models have different limits. Provider interface must expose this.
3. **Tool output size** — Can blow up context quickly. Truncation strategy is critical.
4. **Sub-agent context cloning** — Deep copy of message slice. Memory considerations for large sessions.
5. **Provider API differences** — Each provider has unique quirks (tool format, thinking format, streaming). Compatibility layer needed.

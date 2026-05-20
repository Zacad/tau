# Task 001 Worklog — PI Exploration

## Session Start — 2026-05-02

### 001.1 — PI Package Architecture ✅

**Three core packages + coding-agent wrapper:**

| Package | Purpose | Key Files |
|---------|---------|-----------|
| `pi-ai` (v0.71.0) | Unified LLM API: providers, models, types, streaming, auth | `api-registry`, `providers/`, `models`, `types`, `stream`, `oauth` |
| `pi-agent-core` (v0.71.0) | Agent framework: agent loop, state management, event stream, tools | `agent`, `agent-loop`, `proxy`, `types` |
| `pi-tui` (v0.71.0) | Terminal UI: components, editor, fuzzy search, rendering | `components/`, `editor-component`, `keybindings`, `fuzzy`, `autocomplete` |
| `pi-coding-agent` (v0.71.0) | Full coding agent: session management, extensions, modes, tools, resource loading | `core/`, `modes/`, `cli/`, `utils/` |

**Package dependency chain:**
```
pi-coding-agent → pi-agent-core → pi-ai
pi-coding-agent → pi-tui
pi-coding-agent → jiti (TS execution), photon-node (image processing), diff, glob, etc.
```

**Key coding-agent core modules:**
- `agent-session.ts` (110KB) — Main session lifecycle, prompt handling, event streaming
- `agent-session-runtime.ts` (13KB) — Session replacement, fork, clone, new session
- `agent-session-services.ts` (4KB) — Service factory for runtime creation
- `session-manager.ts` (41KB) — JSONL session persistence, tree structure, branching
- `settings-manager.ts` (29KB) — Settings loading, merging, persistence
- `model-registry.ts` (32KB) — Model discovery, auth resolution, availability
- `resource-loader.ts` (31KB) — Extension, skill, prompt, theme discovery
- `sdk.ts` (12KB) — SDK factory functions
- `compaction/` — Auto-compaction, branch summarization
- `extensions/` — Extension runtime, event system, UI protocol
- `tools/` — Built-in tools: read, write, edit, bash, grep, find, ls
- `export-html/` — Session HTML export

**Run modes (in `modes/`):**
- `interactive/` — Full TUI with editor, commands, chat history
- `print/` — Single-shot output
- `rpc/` — JSONL protocol over stdin/stdout

**Go Architecture Implications:**
- Clean 3-layer separation maps well to Go packages:
  - `llm/` → pi-ai equivalent (providers, models, streaming)
  - `agent/` → pi-agent-core equivalent (agent loop, state, tools)
  - `tui/` → pi-tui equivalent (terminal UI)
  - `tau/` → pi-coding-agent equivalent (session, modes, config)
- The `agent-session` is the orchestrator tying everything together

### 001.2 — Provider/Model System ✅

**Provider architecture (pi-ai):**
- Each provider implements a unified API through `pi-ai/providers/`
- 4 base API protocols: `openai-completions`, `openai-responses`, `anthropic-messages`, `google-generative-ai`
- Provider implementations handle protocol translation (e.g., Azure, Bedrock, Cloudflare all map to one of the 4 base APIs)
- `transform-messages` — normalizes message formats across providers
- `simple-options` — normalizes provider-specific options

**Model system:**
- Built-in models defined in `models.generated` (updated each release)
- Custom models via `models.json`: define providers with `baseUrl`, `api`, `apiKey`, `models[]`
- Model attributes: `id`, `name`, `reasoning`, `input[]`, `contextWindow`, `maxTokens`, `cost{}`, `compat{}`
- `compat` field handles provider-specific quirks (developer role, reasoning effort, thinking format, cache control, etc.)
- `ModelRegistry` resolves models by provider+id, checks API key availability

**Auth resolution order:**
1. CLI `--api-key`
2. `auth.json` (API key or OAuth token)
3. Environment variable
4. Custom provider keys from `models.json`

**Key values in auth.json:**
- Literal value, env var name, or `!shell command`
- OAuth tokens for subscriptions (Claude Pro, ChatGPT Plus, GitHub Copilot)
- `0600` file permissions

**Go Implications:**
- Need a provider interface with 4 base protocol implementations
- Model registry should be config-driven (JSON file + built-in defaults)
- Auth storage: simple JSON file with `0600` permissions
- The `compat` approach is smart — instead of a provider per quirk, one provider with compatibility flags
- Shell command auth resolution is clever for keychain/1Password integration

### 001.3 — Session Management ✅

**Session format: JSONL with tree structure (v3)**
- Each line is a JSON object with `type` field
- Entries linked via `id` (8-char hex) and `parentId`
- First entry: `SessionHeader` with version, id, timestamp, cwd
- Parent session tracking for fork/clone operations

**Entry types:**
- `message` — AgentMessage (user, assistant, toolResult, bashExecution, custom, branchSummary, compactionSummary)
- `model_change` — Model switch mid-session
- `thinking_level_change` — Thinking level change
- `compaction` — Context compaction with summary + firstKeptEntryId
- `branch_summary` — Branch navigation context preservation
- `custom` — Extension state (not in LLM context)
- `custom_message` — Extension-injected messages (in LLM context)
- `label` — User bookmarks
- `session_info` — Display name

**Content block types:**
- `TextContent` — plain text
- `ImageContent` — base64 encoded images
- `ThinkingContent` — model reasoning/thinking
- `ToolCall` — tool invocation with id, name, arguments

**SessionManager API:**
- `create(cwd)` — new session
- `open(path)` — open existing
- `continueRecent(cwd)` — continue most recent
- `inMemory()` — no persistence
- `forkFrom(sourcePath, targetCwd)` — fork from another project
- Tree navigation: `getLeafId()`, `getEntry(id)`, `getBranch()`, `getTree()`, `branch(entryId)`
- Context building: `buildSessionContext()` — walks leaf→root, handles compaction boundaries

**Context building rules:**
1. Walk from leaf to root
2. If compaction on path: emit summary first, then messages from firstKeptEntryId onwards
3. Convert BranchSummaryEntry and CustomMessageEntry to appropriate formats
4. Extract current model and thinking level from entries

**Go Implications:**
- JSONL is ideal for Go — simple append-only writes, easy parsing
- Tree structure is elegant — no need for separate files per branch
- `buildSessionContext()` is the key function for LLM context preparation
- Session files should be stored per-project directory naming convention
- Need atomic writes for session persistence (write to temp, rename)

### 001.4 — Tool System ✅

**Built-in tools (7 total):**
- `read` — read file contents (text + images)
- `bash` — execute shell commands with streaming output
- `edit` — targeted text replacement in files
- `write` — create/overwrite files
- `grep` — search file contents
- `find` — search file paths
- `ls` — list directory contents

**Tool architecture:**
- Tools defined via `defineTool()` with name, label, description, parameters (TypeBox schema), execute function
- `codingTools` preset: [read, bash, edit, write]
- `readOnlyTools` preset: [read, grep, find, ls]
- Tool factory functions for custom cwd: `createReadTool(cwd)`, etc.
- Tool results returned as `content: [{type, text}]` + `details` (tool-specific metadata)
- Streaming tool output via `onUpdate` callback during execution
- Tool definitions use JSON Schema (via TypeBox)

**Tool execution flow:**
1. LLM returns ToolCall in assistant message
2. Tool matched by name, execute called with arguments
3. Results streamed back via tool_execution_update events
4. Tool result appended as ToolResultMessage
5. Agent continues loop with tool results in context
6. `buildSessionContext()` transforms messages to LLM format

**Go Implications:**
- Tool interface: Name, Description, Parameters (JSON Schema), Execute function
- Need tool registry (map name → tool)
- Tool results should be content blocks + optional metadata
- Streaming output is important for long-running bash commands
- File-mutation-queue handles concurrent file edits — important for safety

### 001.5 — Agent Loop ✅

**Core loop (pi-agent-core):**
```
agentLoop(prompts, context, config) → EventStream<AgentEvent, AgentMessage[]>
agentLoopContinue(context, config) → EventStream<AgentEvent, AgentMessage[]>  (for retries)
```

**Loop steps:**
1. Add user prompt to context messages
2. Convert AgentMessage[] to provider-specific Message[] via `convertToLlm`
3. Stream LLM response → emit message_update events
4. If response has tool calls:
   - Execute each tool
   - Append ToolResultMessage for each
   - Continue loop (back to step 2)
5. If no tool calls (stopReason = "stop"): loop ends
6. Return all new messages

**Event types emitted:**
- `agent_start`, `agent_end` — lifecycle boundaries
- `turn_start`, `turn_end` — one LLM response + tool calls
- `message_start`, `message_update`, `message_end` — streaming
- `tool_execution_start`, `tool_execution_update`, `tool_execution_end` — tool lifecycle
- `compaction_start`, `compaction_end` — context management
- `auto_retry_start`, `auto_retry_end` — error recovery
- `queue_update` — steering/follow-up queue changes
- `extension_error` — extension failures

**Turn lifecycle:**
- Turn = one assistant response + resulting tool calls + results
- Multiple turns per prompt (when tools are used)
- Each turn ends with `turn_end` event containing assistant message + tool results

**Error handling:**
- Auto-retry on transient errors (overloaded, rate limit, 5xx)
- Configurable max retries
- Abort capability during retry

**Go Implications:**
- Agent loop is straightforward: prompt → LLM → tool calls → results → repeat
- Event-driven architecture maps well to Go channels
- Need EventStream abstraction (could use channels or callbacks)
- AbortSignal → context.Context in Go
- The convertToLlm step is critical — each provider needs message format transformation

### 001.6 — RPC Protocol ✅

**Transport:** JSONL over stdin/stdout (LF delimiters only)

**Commands (stdin):**
- `prompt` — send user message (with optional streamingBehavior, images)
- `steer` — queue steering message (delivered after current turn)
- `follow_up` — queue follow-up (delivered after agent finishes)
- `abort` — cancel current operation
- `new_session` — start fresh session
- `get_state` — get session state
- `get_messages` — get conversation history
- `set_model` / `cycle_model` / `get_available_models` — model control
- `set_thinking_level` / `cycle_thinking_level` — thinking control
- `set_steering_mode` / `set_follow_up_mode` — queue modes
- `compact` / `set_auto_compaction` — context management
- `set_auto_retry` / `abort_retry` — retry control
- `bash` — execute shell command (result included in next prompt context)
- `abort_bash` — cancel running command
- `get_session_stats` — token/cost statistics
- `export_html` — session export
- `switch_session` / `fork` / `clone` / `get_fork_messages` — session management
- `set_session_name` / `get_last_assistant_text` — session metadata
- `get_commands` — available extension commands/skills/prompts

**Events (stdout):**
- All event types from agent loop (see 001.5)
- `extension_ui_request` — extension user interaction (select, confirm, input, editor, notify, setStatus, setWidget, setTitle, set_editor_text)
- Extension UI responses come back on stdin as `extension_ui_response`

**Framing:**
- Strict LF delimiters
- Accept optional \r\n input
- No generic line readers (Unicode separators issue)

**Go Implications:**
- JSONL protocol is simple and language-agnostic
- Command/Response/Event model maps cleanly
- Extension UI sub-protocol is sophisticated — need request/response correlation via `id`
- For our Go tau, RPC mode could be the primary programmatic interface
- Need careful stdin/stdout handling with proper framing

### 001.7 — TUI Architecture ✅

**Component system (pi-tui):**
- `Component` interface: `render(width)`, `handleInput(data)`, `wantsKeyRelease`, `invalidate()`
- Components render to string arrays (one per line)
- SGR + OSC 8 reset appended to each line
- Focus management via `Focusable` interface (IME support)
- Container components with child focus propagation
- Overlay system with anchor positioning, responsive visibility

**Built-in components:**
- `Text` — word-wrapped text
- `Box` — bordered container
- `Container` — child component layout
- `Spacer` — flexible spacing
- `Markdown` — markdown rendering (via marked)
- `Editor` — text input with completion
- `Input` — single-line input
- `SelectDialog` — option selection

**TUI app structure:**
- Main area: message history + tool output
- Editor: text input at bottom with completion
- Footer: status line (cwd, session, tokens, cost, model)
- Custom rendering for tool calls/results
- Theme system with hot-reload

**Command system:**
- Slash commands: `/model`, `/settings`, `/compact`, `/tree`, `/fork`, `/clone`, etc.
- Skill commands: `/skill:name`
- Prompt templates: `/name`
- Extension commands: registered via `pi.registerCommand()`

**Go Implications:**
- For minimalist approach, could skip full component system initially
- Bubbletea (charm.sh) is a natural Go equivalent for TUI
- Or build minimal custom TUI — just need: message display, input editor, footer
- Editor needs: file completion (@), path completion (Tab), multi-line (Shift+Enter)
- Theme system can be simple: color scheme config
- Command system is straightforward: parse `/` prefix, dispatch to handler

### 001.8 — Learnings: Patterns to Adopt ✅

**Patterns to adopt from PI:**
1. **4 base API protocols** — OpenAI Completions, OpenAI Responses, Anthropic Messages, Google Generative AI. Map all providers to these.
2. **JSONL session format** — Simple, append-only, parseable, supports tree structure
3. **Session tree with id/parentId** — Elegant branching without file proliferation
4. **Event-driven architecture** — Clean separation of concerns, easy to extend
5. **Context building via walk** — buildSessionContext() pattern for preparing LLM context
6. **Compaction with structured summaries** — Goal/Constraints/Progress format
7. **Provider compatibility flags** — Instead of per-provider quirks, use compat config
8. **Auth resolution order** — CLI → file → env → custom, with shell command support
9. **Tool definition via schema** — JSON Schema parameters, execute function
10. **Event streaming** — Real-time updates for TUI, RPC, and programmatic consumers
11. **Skills progressive disclosure** — Only descriptions in system prompt, full SKILL.md loaded on demand
12. **Message serialization for compaction** — [User]:, [Assistant]:, [Tool result]: format

**Design decisions to diverge from PI:**
1. **No TypeScript runtime** — Single Go binary, no npm, no node_modules
2. **Sub-agents built-in** — Not via extensions, native support from start
3. **Skills built-in** — Not discovered, explicitly configured
4. **No extension system initially** — Keep it simple, add later if needed
5. **Simpler TUI** — Use bubbletea or minimal custom, not full component system
6. **No pi packages** — No npm/git package management for extensions
7. **No OAuth subscriptions initially** — API keys only
8. **Simpler settings** — Single config file, no global/project merge complexity
9. **No compaction initially** — Can add later, simpler to start without it
10. **Provider config in YAML/TOML** — More idiomatic for Go than JSON

**Go-specific considerations:**
1. **Go's net/http** — Built-in HTTP client, no undici needed
2. **Go channels** — Natural fit for event streaming
3. **context.Context** — Built-in cancellation, no AbortSignal needed
4. **encoding/json** — Standard library JSON handling
5. **Bubbletea** — Mature TUI framework for Go (charm.sh)
6. **Single binary** — No runtime dependencies, easy deployment
7. **Performance** — Go's concurrency model is better suited for agent loops
8. **Type safety** — Go structs instead of TypeScript interfaces
9. **No jiti** — No need for runtime TS compilation

### 001.9 — Gap Analysis ✅

**Features PI lacks that we need:**
1. **Sub-agent orchestration** — PI explicitly excludes this. Need:
   - Parent-child session management
   - Task delegation and result aggregation
   - Sub-agent lifecycle (spawn, monitor, terminate)
   - Context sharing between parent and child
   - Tool access control for sub-agents

2. **Skills orchestration** — PI loads skills passively. We need:
   - Active skill selection based on task
   - Skill chaining (multiple skills for one task)
   - Skill-specific tool provisioning
   - Skill context management

3. **Provider config simplicity** — PI's models.json is powerful but complex. We need:
   - Simpler default configuration
   - YAML/TOML instead of JSON
   - Sensible defaults for all required providers

4. **Built-in session management** — PI requires configuration. We need:
   - Auto-session per project
   - Simple session listing/resume
   - Session naming

5. **Performance focus** — PI is Node.js. We need:
   - Lower memory footprint
   - Faster startup
   - Better concurrent tool execution

**Sub-agent approaches to consider:**
1. **In-process goroutines** — Each sub-agent runs as goroutine with its own AgentSession
2. **Process-per-sub-agent** — Spawn child processes (like tmux approach PI mentions)
3. **Hybrid** — In-process for simple tasks, separate process for isolation

**Skills orchestration requirements:**
1. Skill registry with metadata (name, description, triggers)
2. Skill loader (SKILL.md parsing, script execution)
3. Skill activation logic (automatic vs explicit)
4. Skill-specific tool provisioning
5. Skill context injection into system prompt

## Session End — All subtasks completed ✅

# ARCHITECTURE

Tau — a minimalist agentic coding tool built in Go.

## 1. System Overview

### 1.1 Core Philosophy

**Orchestration is not code — it's instructions.** The agent loop reads context files (AGENTS.md), skill definitions (SKILL.md), and follows them autonomously. No special orchestration framework needed.

```
AGENTS.md (project rules and workflows)
  └── "When implementing: plan → spawn Implementor → review"

SKILL.md files (capabilities)
  └── "I handle code review. For deep analysis, spawn Researcher"

Agent loop reads both → follows instructions naturally
```

### 1.2 Component Diagram

```
┌─────────────────────────────────────────────────────────┐
│                        CLI (cmd/tau)                  │
│                   User input, model selection             │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                       SDK (Session)                      │
│         High-level API, event subscription               │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                      Agent Loop                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  System Prompt (composed once at session start):   │ │
│  │  1. Context files (AGENTS.md, CLAUDE.md)           │ │
│  │  2. Static capabilities                            │ │
│  │  3. Skill progressive disclosure                   │ │
│  └────────────────────────────────────────────────────┘ │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Loop: stream → tools → results → repeat           │ │
│  │  - Provider.Stream() → LLM API call                │ │
│  │  - Tool.Execute() → tool results                   │ │
│  │  - SubAgent.Run() → subagent results               │ │
│  │  - Steering/follow-up queue checks between turns   │ │
│  └────────────────────────────────────────────────────┘ │
└──────┬──────────┬──────────┬──────────┬────────────────┘
       │          │          │          │
  ┌────▼───┐ ┌────▼────┐ ┌──▼───┐ ┌───▼────┐
  │Session │ │ Provider│ │Tools │ │Subagent│
  │Persist │ │ Stream  │ │      │ │ Spawner│
  └────────┘ └─────────┘ └──────┘ └────────┘

┌─────────────────────────────────────────────────────────┐
│                    Persistence Layer                     │
│         JSONL sessions │ Config │ Auth                   │
└─────────────────────────────────────────────────────────┘
```

### 1.3 Data Flow

```
User Input
    │
    ▼
SDK.CreateSession() → loads config, resolves model, discovers skills
    │
    ▼
Compose system prompt:
  1. Walk cwd → parent dirs, load AGENTS.md/CLAUDE.md
  2. Load global ~/.tau/AGENTS.md
  3. Discover skills (builtin, global, project)
  4. Format skills as progressive disclosure (name + description)
  5. Concatenate: context files + capabilities + skills
    │
    ▼
Agent loop starts with composed system prompt
    │
    ├──► Provider.Stream() → LLM API call (streaming)
    ├──► Events emitted: message_start → message_update → message_end
    ├──► Tool calls detected
    │       ├──► BeforeToolCall hook (allow/block)
    │       ├──► Tool.Execute() (parallel/sequential/exclusive)
    │       ├──► AfterToolCall hook (transform results)
    │       └──► ToolResultMessage appended to context
    ├──► If agent spawns subagent → SubAgent.Run() → result injected
    ├──► Steering/follow-up queues checked between turns
    ├──► Session persisted (JSONL append, immediate per message)
    ├──► Loop continues until stop condition
    │
    ▼
Result returned to user
```

### 1.4 System Boundaries

- **In scope**: Agent loop, skills, subagents, providers, tools, session management, CLI, context files
- **Out of scope (MVP)**: Extension system, web UI, multi-user, RPC mode, branching sessions, OAuth

### 1.5 External Dependencies

| Dependency | Purpose | Justification |
|---|---|---|
| `gopkg.in/yaml.v3` | Skill frontmatter parsing | Agent Skills standard requires YAML |
| `github.com/invopop/jsonschema` | Tool parameter JSON Schema generation | LLM tool calling requires proper JSON Schema from Go structs |
| `github.com/JohannesKaufmann/html-to-markdown` | HTML→Markdown conversion for webfetch | Essential for webfetch; 4.5K stars, actively maintained, focused library |
| `net/http` (stdlib) | Provider HTTP calls | No external HTTP client needed |
| `encoding/json` (stdlib) | Session serialization, config | Built-in |
| `bufio` (stdlib) | JSONL streaming I/O | Built-in |

---

## 2. Package Structure

### 2.1 Layout

```
tau/
├── cmd/tau/
│   └── main.go                 # CLI entry point
├── internal/
│   ├── types/                  # Shared data structures (no internal deps)
│   │   ├── message.go          # AgentMessage, ContentBlock, etc.
│   │   ├── tool.go             # ToolResult, ToolCallBlock, BashExecution
│   │   ├── provider.go         # Usage, CostInfo, StreamEvent, StreamOptions
│   │   └── session.go          # SessionEntry types
│   ├── agent/                  # Agent loop, state, event management
│   │   ├── agent.go            # Agent struct (owns transcript + tools)
│   │   ├── loop.go             # agentLoop() — the core loop
│   │   └── event.go            # AgentEvent types, event subscription
│   ├── session/                # Session management
│   │   ├── session.go          # Session lifecycle (create, resume, persist)
│   │   ├── storage.go          # JSONL read/write
│   │   ├── compaction.go       # Compaction logic (pure functions)
│   │   └── naming.go           # Auto-naming strategy
│   ├── tools/                  # Tool system
│   │   ├── tool.go             # Tool interface, registry
│   │   ├── read.go             # File read
│   │   ├── write.go            # File write
│   │   ├── edit.go             # File edit
│   │   ├── bash.go             # Shell execution
│   │   ├── grep.go             # Content search
│   │   ├── find.go             # File search
│   │   ├── ls.go               # Directory listing
│   │   ├── truncate.go         # Output truncation
│   │   ├── queue.go            # File mutation serialization
│   │   ├── search.go           # SearchBackend interface, WebSearchTool, fallback
│   │   ├── search_tavily.go    # Tavily search backend
│   │   ├── search_brave.go     # Brave Search backend
│   │   ├── search_searxng.go   # SearXNG search backend (self-hosted)
│   │   └── webfetch.go         # WebFetchTool, HTML→markdown, SSRF protection
│   ├── provider/               # LLM provider abstraction
│   │   ├── provider.go         # Provider interface
│   │   ├── model.go            # Model struct, registry
│   │   ├── registry.go         # Provider registration
│   │   ├── openai.go           # OpenAI provider
│   │   ├── anthropic.go        # Anthropic provider
│   │   ├── google.go           # Google Gemini provider
│   │   ├── openrouter.go       # OpenRouter provider
│   │   ├── opencode.go         # OpenCode Zen + OpenCode Go
│   │   ├── local.go            # Ollama / llama.cpp / LM Studio
│   │   └── auth.go             # Auth resolution chain
│   ├── skills/                 # Skill system
│   │   ├── skill.go            # Skill struct
│   │   ├── discovery.go        # Directory scanning
│   │   ├── parser.go           # SKILL.md frontmatter parsing
│   │   └── prompt.go           # Progressive disclosure formatting
│   ├── subagent/               # Sub-agent system (new — not in PI)
│   │   ├── subagent.go         # SubAgent struct, lifecycle
│   │   ├── context.go          # Context fork/clone
│   │   └── result.go           # Result handling
│   ├── config/                 # Configuration
│   │   ├── config.go           # Settings, loading
│   │   └── paths.go            # Directory paths
│   └── sdk/                    # Public SDK
│       └── sdk.go              # Session — high-level API
├── pkg/                        # Public packages (future)
├── docs/                       # Project documentation
└── go.mod
```

### 2.2 Import Dependency Graph

```
cmd/tau
    └── internal/sdk (Session)
            ├── internal/agent
            │       ├── internal/types (leaf data)
            │       ├── internal/provider
            │       └── internal/tools
            ├── internal/session
            │       └── internal/types
            ├── internal/provider (leaf)
            ├── internal/tools (leaf)
            ├── internal/skills (leaf)
            ├── internal/subagent
            │       ├── internal/types
            │       └── internal/provider (leaf)
            └── internal/config (leaf)
```

**Rules**:
- `types`, `provider`, `tools`, `skills`, `config` are leaf packages — no internal dependencies on each other
- `agent` depends only on leaf packages + `types`
- `session` depends only on `types` (not on `agent`)
- `subagent` depends on `types` and `provider` (both leaf packages)
- `sdk` (Session) composes all subsystems — it's the session lifecycle manager
- Graph is strictly acyclic

### 2.3 Package Responsibilities

| Package | Responsibility | Public API Surface |
|---|---|---|
| `types` | Core data structures — messages, tools, events | Pure data types, no behavior |
| `agent` | Agent loop, state machine, event emission | `NewAgent()`, `Agent.Prompt()`, `Agent.Continue()`, event subscription |
| `session` | JSONL persistence, lifecycle, compaction | `OpenSession()`, `Append()`, `BuildContext()`, `Compact()` |
| `tools` | Tool definitions and execution | `Tool` interface, `NewRegistry()`, `Register()` |
| `provider` | LLM API abstraction, streaming | `Provider` interface, `Model` struct, `NewRegistry()` |
| `skills` | Discovery, parsing, prompt formatting | `DiscoverSkills()`, `LoadSkill()`, `FormatForPrompt()` |
| `subagent` | Subagent lifecycle, context management | `NewSubAgent()`, `SubAgent.Run()`, context modes |
| `config` | Settings loading, path resolution | `LoadConfig()`, `DefaultPaths()` |
| `sdk` | Session — high-level API, composes all subsystems | `CreateSession()`, `Session.Prompt()`, `Session.Steer()`, `Session.Subscribe()` |

---

## 3. Agent Loop Design

### 3.1 The Loop IS the Orchestrator

There is no separate orchestrator package. The agent loop reads its instructions from context files and skills, then executes them autonomously. This is the same pattern PI uses (`AgentSession`) but simpler:

```
AGENTS.md tells the agent WHAT to do (workflows, rules)
SKILL.md tells the agent HOW to do it (capabilities, subagent recommendations)
Agent loop executes both
```

### 3.2 Agent Struct

```go
type Agent struct {
    // Transcript
    messages     []types.AgentMessage
    systemPrompt string

    // Tools
    tools         map[string]tools.Tool
    beforeToolCall  func(types.BeforeToolCallContext) (*types.BeforeToolCallResult, error)
    afterToolCall   func(types.AfterToolCallContext) (*types.AfterToolCallResult, error)

    // Streaming
    provider  provider.Provider
    model     provider.Model

    // Steering/follow-up
    steerQueue    chan types.AgentMessage
    followUpQueue chan types.AgentMessage

    // Events
    listeners []func(types.AgentEvent)
}
```

### 3.3 Steering and Follow-Up Message Queues

**Steering queue**: Buffered channel. Messages are delivered after the current tool call batch completes, before the next LLM call. Allows user to type while agent is working.

**Follow-up queue**: Buffered channel. Messages are delivered only when the agent would otherwise stop (no more tool calls, no steering). Used for chained follow-up tasks.

```go
// Delivery semantics:
// - steerQueue: checked after each turn_end, before next LLM call
// - followUpQueue: checked only when agent would be done
// - Both are drained in FIFO order
// - Queue size: 10 (buffered) — overflow drops oldest with warning
```

### 3.4 Agent Loop State Machine

```
                    ┌──────────────┐
                    │    IDLE      │
                    │ (no active   │
                    │  run)        │
                    └──────┬───────┘
                           │
                    prompt() │ or continue()
                           ▼
                    ┌──────────────┐
                    │  STREAMING   │──────────────────┐
                    │  (calling    │                  │
                    │  provider)   │                  │
                    └──────┬───────┘                  │
                           │                          │
                    message_end event                 │
                           │                          │
                           ▼                          │
                    ┌──────────────┐                  │
                    │   TURN_END   │                  │
                    │  (checking   │                  │
                    │   tools)     │                  │
                    └──────┬───────┘                  │
                           │                          │
              ┌────────────┴────────────┐             │
              │                         │             │
         has tools               no tools              │
              │                         │             │
              ▼                         ▼             │
    ┌──────────────────┐    ┌─────────────────┐      │
    │ EXECUTING_TOOLS  │    │ CHECK_QUEUES    │      │
    │ (parallel/seq)   │    │ steerQueue?     │      │
    └────────┬─────────┘    │ followUpQueue?  │      │
             │              └────────┬────────┘      │
             │                       │               │
    all tools done             has queued?           │
             │              ┌──────┴──────┐          │
             │             YES           NO          │
             │              │             │          │
             ▼              ▼             ▼          │
    ┌──────────────┐ ┌──────────┐ ┌───────────┐     │
    │  TURN_END    │ │ STREAMING│ │   DONE    │◄────┘
    │  (loop back) │ │ (again)  │ │           │
    └──────┬───────┘ └──────────┘ └───────────┘
           │
           │ (after each tool result)
           ▼
    ┌──────────────┐
    │ PERSIST      │
    │ (append to   │
    │  JSONL)      │
    └──────┬───────┘
           │
           ▼
    (back to TURN_END)
```

**State transitions**:
- `IDLE → STREAMING`: User calls `prompt()` or `continue()`
- `STREAMING → TURN_END`: Provider finishes streaming (`message_end` event)
- `TURN_END → EXECUTING_TOOLS`: Assistant message contains tool calls
- `EXECUTING_TOOLS → TURN_END`: All tool results collected, persisted
- `TURN_END → STREAMING`: Steering queue has messages (checked after each turn)
- `TURN_END → DONE`: No tools, no steering, no follow-up
- `TURN_END → STREAMING` (via follow-up): Follow-up queue has messages (checked when agent would stop)

**Abort handling**: `Abort()` can be called from any state. Current provider call is cancelled via `context.CancelFunc`. Partial results are discarded. Session state preserved.

### 3.5 Context Files

**Discovery** (matching PI's AGENTS.md/CLAUDE.md loading):
1. Walk up from cwd through parent directories, looking for `AGENTS.md` and `CLAUDE.md`
2. Load global `~/.tau/AGENTS.md` if present
3. Files are concatenated in order (root → cwd)
4. Content prepended to system prompt before skill disclosure
5. Override via `--no-context-files` flag

**AGENTS.md as orchestration rules**: This is where workflows are defined. Example:

```markdown
# Project Rules

## Workflows
- When implementing features: use planning skill → spawn Implementor → spawn Reviewer
- When reviewing PRs: use security-reviewer skill
- When debugging: spawn Researcher to investigate, then Implementor to fix

## Subagent Guidelines
- Always spawn subagents with forked context for code changes
- Use fresh context for research tasks
- Subagent results should be LLM-visible
```

The agent reads this and follows it through its normal loop — no special code needed.

---

## 4. Skills System Architecture

### 4.1 Skill Discovery Mechanism

Three-tier discovery (matching PI and cross-tool compatibility):

| Tier | Path | Priority |
|---|---|---|
| Built-in | Embedded in binary | Highest |
| Global | `~/.tau/skills/`, `~/.agents/skills/` | Medium |
| Project | `.agents/skills/` (walk up from cwd through parent dirs) | Lowest (overrides) |

**Discovery rules** (aligned with Agent Skills standard):
- Directory containing `SKILL.md` is treated as skill root — no recursion
- Direct `.md` children in skill root loaded as additional references
- Symlinks followed
- `.gitignore`, `.ignore` patterns respected
- `node_modules` directories skipped

### 4.2 Skill Loading and Progressive Disclosure

```go
type Skill struct {
    Name               string   // Matches directory name
    Description        string   // Max 1024 chars
    DisableModelInvocation bool // If true, excluded from prompt
    Content            string   // Full markdown body
    Scripts            []string // Available script paths
    References         []string // Reference file paths
    Assets             []string // Asset file paths
    Source             string   // "builtin" | "global" | "project"
}
```

**Progressive disclosure** (system prompt format):
```xml
<skills>
<skill name="skill-name" description="What this skill does"/>
<skill name="another-skill" description="Another capability"/>
</skills>
```

Full skill content loaded only when:
- User explicitly invokes via `/skill:name`
- Agent autonomously decides to use a skill
- Workflow requires a specific skill

### 4.3 Skill-Tool Relationship

- Skills **describe capabilities** and may **recommend tools** but don't own tools
- Tool availability is managed at session level based on active skill context
- Built-in skills:
  - `skill-builder`: Creates new SKILL.md files following Agent Skills standard
  - `subagent-builder`: Creates new subagent definitions

### 4.4 Agent Skills Standard Compliance

| Requirement | Implementation |
|---|---|
| SKILL.md format | YAML frontmatter + markdown body |
| Name validation | lowercase, hyphens, 0-9, max 64 chars |
| Description required | Max 1024 chars |
| Directory name matches skill name | Enforced on load |
| Progressive disclosure | Only name+description in system prompt |
| Cross-tool compatibility | Format compatible with PI, OpenCode, Claude Code |

---

## 5. Subagent System Architecture

### 5.1 Subagent Lifecycle

The only genuinely new component compared to PI (where subagents are extension-based):

```
1. Agent decides to spawn subagent (driven by AGENTS.md rules or skill instructions)
2. Agent creates SubAgent with:
   - Task description
   - Context mode (fresh | fork)
   - Tool set (subset of parent tools)
   - Model (inherits or overridden)
   - System prompt (skill-specific or default)
3. Subagent runs in isolated goroutine
4. Events from subagent optionally forwarded to parent (streaming visibility)
5. Subagent completes → result returned via channel
6. Result injected into parent context (LLM-visible by default)
7. Agent continues with enriched context
```

### 5.2 Context Model

| Mode | Behavior |
|---|---|
| `fresh` | Empty message slice, inherits system prompt and tools |
| `fork` | Cloned message slice from parent (current transcript, shallow copy) |

**Context isolation**:
- Each subagent gets its own `[]AgentMessage` slice
- Modifications don't affect parent context
- Only final result returned to parent
- No subagent-to-subagent communication

### 5.3 Communication Model

```
Parent                          Subagent
  │                               │
  ├──► NewSubAgent(task, ctx)────►│
  │                               │
  │          ◄── Events (optional)│
  │          ◄── Result (chan)    │
  │                               │
  │──► Result received            │
  │──► Inject into parent context │
  │──► Continue agent loop        │
```

- **Parent ↔ child only**: No subagent-to-subagent communication
- **Result injection**:
  - **Default**: LLM-visible as `ToolResultMessage`-equivalent — parent agent can act on results
  - **Opt-out**: `custom_entry` (non-LLM-visible) for logging/metadata only
  - Configurable via `SubAgentResultOptions{LLMVisible: bool}`
- **Execution model**: Subagent runs **synchronously** — parent agent loop waits (with configurable timeout, default 5 minutes)
- **Timeout handling**: If subagent exceeds timeout, it is cancelled via `context.CancelFunc`, error returned to parent
- **Streaming visibility**: Parent can optionally receive subagent events for real-time progress display via forwarded channel
- **Error isolation**: Subagent failures are caught and returned as `SubAgentResult{Success: false, Error: ...}` — parent continues unaffected

### 5.4 Built-in Subagent Types

| Type | Purpose | Default Tools |
|---|---|---|
| General | Versatile default for any task | read, write, edit, bash, grep, find, ls, websearch, webfetch |
| Researcher | Research, information gathering | read, grep, find, ls, bash, websearch, webfetch |
| Reviewer | Code/content review | read, grep, find, ls |
| Implementor | Feature implementation | read, write, edit, bash, grep, find, ls |
| Security Reviewer | Security analysis | read, grep, find, bash (static analysis) |
| QA | Testing, quality assurance | read, bash, grep, find, ls, write |

### 5.5 Subagent Struct

```go
type SubAgent struct {
    ID           string
    Type         string          // "researcher" | "reviewer" | "implementor" | ...
    Task         string
    ContextMode  string          // "fresh" | "fork"
    Tools        []tools.Tool
    Model        provider.Model
    SystemPrompt string
    Events       chan types.AgentEvent  // Optional: for streaming visibility
    Result       chan SubAgentResult
    cancel       context.CancelFunc
}

type SubAgentResult struct {
    Success    bool
    Output     string
    Artifacts  []string          // File paths created/modified
    Error      error
    Duration   time.Duration
    Usage      types.Usage
}
```

---

## 6. Provider Abstraction and Model Selection

### 6.1 Provider Interface

```go
type Provider interface {
    Name() string
    Stream(ctx context.Context, model Model, messages []Message, tools []ToolDefinition, opts StreamOptions) <-chan StreamEvent
    Complete(ctx context.Context, model Model, messages []Message, tools []ToolDefinition, opts StreamOptions) (*AssistantMessage, error)
}

type ToolDefinition struct {
    Name        string
    Description string
    Parameters  *jsonschema.Schema  // Generated from Go struct via github.com/invopop/jsonschema
}
```

**Design decision**: Single `Provider` interface per API type (not per provider). OpenAI-compatible providers share the same implementation with configuration differences.

### 6.1a StreamOptions

```go
type ThinkingLevel string

const (
    ThinkingOff     ThinkingLevel = "off"
    ThinkingMinimal ThinkingLevel = "minimal"
    ThinkingLow     ThinkingLevel = "low"
    ThinkingMedium  ThinkingLevel = "medium"
    ThinkingHigh    ThinkingLevel = "high"
    ThinkingXHigh   ThinkingLevel = "xhigh"
)

type StreamOptions struct {
    ThinkingLevel ThinkingLevel   // "off" | "minimal" | "low" | "medium" | "high" | "xhigh"
    MaxTokens     int             // Max completion tokens
    Temperature   float64         // Sampling temperature (0.0–1.0)
    SystemPrompt  string          // Override default system prompt
    Tools         []ToolDefinition
}
```

### 6.2 Model Registry

```go
type Model struct {
    ID            string
    Name          string
    Provider      string
    API           string          // "openai-responses" | "anthropic-messages" | "google-generative-ai" | ...
    BaseURL       string
    Reasoning     bool            // Whether model supports thinking/reasoning
    InputTypes    []string        // ["text"] | ["text", "image"]
    Cost          CostInfo
    ContextWindow int
    MaxTokens     int
    Headers       map[string]string
    Compat        map[string]any  // Compatibility overrides
}

type CostInfo struct {
    Input      float64  // $/1M input tokens
    Output     float64  // $/1M output tokens
    CacheRead  float64  // $/1M cache read tokens
    CacheWrite float64  // $/1M cache write tokens
}

type Usage struct {
    Input       int
    Output      int
    CacheRead   int
    CacheWrite  int
    TotalTokens int
    Cost        CostDollars
}

type CostDollars struct {
    Input      float64
    Output     float64
    CacheRead  float64
    CacheWrite float64
    Total      float64
}
```

**Model discovery**:
- Built-in models defined in code (OpenAI, Anthropic, Gemini, etc.)
- Custom models via config file
- Local models auto-discovered via Ollama API

### 6.3 Authentication Handling

Resolution chain (4-step, matching PI's `resolveConfigValueOrThrow`):

1. **CLI flag**: `--api-key <key>` (highest priority, for scripting)
2. **`auth.json`**: Dedicated credential store at `~/.tau/auth.json` with `0600` permissions
3. **Environment variables**: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, etc.
4. **Config file**: `~/.tau/config.json` (fallback)

Key formats supported in `auth.json`:
- **Literal**: `"sk-actual-key-value"`
- **Environment variable reference**: `"$MY_ANTHROPIC_KEY"` — resolved at runtime
- **Shell command**: `"!security find-generic-password -ws 'anthropic'"` — output used as key

```json
// ~/.tau/auth.json
{
  "openai": "sk-...",
  "anthropic": "$ANTHROPIC_API_KEY",
  "google": "!security find-generic-password -ws 'google-ai'"
}
```

```json
// ~/.tau/config.json
{
  "providers": {
    "openai": {
      "model": "gpt-4o"
    },
    "anthropic": {
      "model": "claude-sonnet-4-20250514"
    }
  },
  "default_model": "claude-sonnet-4-20250514"
}
```

**OAuth deferred to post-MVP** — but the resolution chain is designed to accept OAuth tokens as a 5th priority step later.

### 6.4 Provider Implementations

| Provider | API Type | Implementation |
|---|---|---|
| OpenAI | `openai-responses` | Direct HTTP to `api.openai.com` |
| Anthropic | `anthropic-messages` | Direct HTTP to `api.anthropic.com` |
| Google Gemini | `google-generative-ai` | Direct HTTP to `generativelanguage.googleapis.com` |
| OpenRouter | `openai-completions` (compat) | OpenAI-compatible with routing headers |
| OpenCode Zen | **multi-endpoint** | `ZenProvider` wrapper routes by `model.API` |
| OpenCode Go | `openai-completions` (compat) | OpenAI-compatible |
| Ollama | `openai-completions` (compat) | OpenAI-compatible on `localhost:11434` |
| llama.cpp | `openai-completions` (compat) | OpenAI-compatible server |
| LM Studio | `openai-completions` (compat) | OpenAI-compatible server |

### 6.4a OpenCode Zen Multi-Endpoint Routing

OpenCode Zen routes different model families to different API endpoints with different request/response formats. `ZenProvider` is a wrapper that holds four sub-providers and dispatches based on `model.API`:

```
ZenProvider
├── OpenAIProvider       → /responses         (gpt-* models)
├── AnthropicProvider    → /messages          (claude-* models)
├── GoogleProvider       → /models/{id}       (gemini-* models, Bearer auth)
└── OpenAICompatProvider → /chat/completions  (qwen*, minimax*, glm*, kimi*)
```

**Model classification** (`ClassifyZenModelAPI`):
| Model ID prefix | API type | Endpoint |
|---|---|---|
| `gpt-*` | `openai-responses` | `/v1/responses` |
| `claude-*` | `anthropic-messages` | `/v1/messages` |
| `gemini-*` | `google-generative-ai` | `/v1/models/{id}:streamGenerateContent` |
| others | `openai-completions` | `/v1/chat/completions` |

**GoogleProvider Bearer auth**: The standard Google provider uses `?key=` URL parameter authentication. For Zen gateway routing, `GoogleProvider` supports a `bearer` auth mode that uses `Authorization: Bearer <key>` header instead. Configured via `GoogleConfig{AuthMode: "bearer"}`.

**Model discovery**: `DiscoverZenModels()` fetches `/v1/models` from Zen, classifies each model by ID prefix, and registers with correct `API` type, reasoning support, context window, max tokens, and cost information.

### 6.5 Streaming Events

```go
type StreamEvent struct {
    Type    string              // "start" | "text_delta" | "thinking_delta" | "thinking_start" | "thinking_end" | "toolcall_start" | "toolcall_end" | "done" | "error"
    Delta   string              // Partial text or thinking content
    Message *AssistantMessage   // Accumulated message
    Usage   *Usage              // Final usage stats (on "done")
    Error   error               // Error details
}
```

### 6.6 HTTP Streaming Architecture

OpenAI-compatible providers (`OpenAICompatProvider`) use **incremental SSE streaming** via `bufio.Scanner` on the HTTP response body. This matches the Ollama provider pattern and ensures:

- **Context cancellation works**: `scanner.Scan()` returns when context is cancelled, unlike `io.ReadAll` which blocks until EOF
- **Incremental event delivery**: SSE events are parsed and emitted as they arrive, not buffered
- **No hangs on server errors**: If the server stops sending without closing the connection, the HTTP client timeout (5 minutes) is the only limit

Direct HTTP requests are made using `net/http` with `http.NewRequestWithContext(ctx, ...)`, bypassing the `DefaultHTTPClient` which uses `io.ReadAll` for retry logic (unsuitable for streaming).

Other providers (OpenAI, Anthropic, Google) use `DefaultHTTPClient` with `io.ReadAll` because their APIs return the complete response before streaming begins (or use different streaming mechanisms).

### 6.7 Reasoning / Thinking Block Architecture

Tau handles reasoning tokens uniformly across all providers via a **block-switching streaming pattern**.

#### Content Block Lifecycle

The stream parser maintains a `currentBlock` pointer. When a new content type arrives (thinking, text, or tool call), the previous block is finalized and a new block is created:

```
finishCurrentBlock(currentBlock):
  - If block is "text": emit text_end event
  - If block is "thinking": emit thinking_end event
  - If block is "toolCall": parse arguments, emit toolcall_end event

On incoming delta:
  - Determine content type (thinking / text / tool_call)
  - If currentBlock type differs: finishCurrentBlock(), create new block
  - Append delta to currentBlock
  - Emit appropriate delta event
```

This pattern handles **interleaving** — reasoning can arrive before, during, or after content text, and multiple thinking blocks can exist in a single assistant message.

#### Provider-Specific Reasoning Fields

| Provider API | Reasoning Field(s) | Block Creation | Signature |
|---|---|---|---|
| OpenAI-compat (Ollama, OpenRouter, OpenCode, llama.cpp) | `reasoning_content`, `reasoning`, `reasoning_text` (first non-empty) | Implicit on first non-empty reasoning delta | Stores field name for round-trip |
| Anthropic | `content_block_start` → `thinking` / `redacted_thinking` | Explicit event-driven | `signature_delta` for integrity |
| OpenAI Responses | `reasoning` in delta | Implicit | N/A |

For OpenAI-compatible providers, multiple field names are checked to avoid duplication (e.g., chutes.ai returns both `reasoning_content` and `reasoning` with identical content — only the first is used).

#### Thinking Block Structure

```go
type ContentBlock struct {
    Type     ContentBlockType  // "thinking" | "text" | "tool_call" | "image"
    Text     string            // For thinking: accumulated reasoning content
    ToolCall *ToolCallBlock    // For tool_call blocks
    Image    *ImageBlock       // For image blocks
}
```

The `thinkingSignature` field (stored separately by Anthropic, or as the field name for OpenAI-compat) is used during round-trip serialization.

#### Round-Trip Serialization

When converting assistant messages back to API format for follow-up requests, `BlockThinking` blocks are handled per provider:

| Provider | Round-Trip Behavior |
|---|---|
| OpenAI-compat | Thinking text is NOT sent back — OpenAI Chat Completions API doesn't accept thinking blocks in follow-up messages |
| Anthropic | Thinking blocks sent as `{type: "thinking", thinking: "...", signature: "..."}` — signature required for integrity |
| Anthropic (redacted) | Redacted thinking sent as `{type: "redacted_thinking", data: "<opaque payload>"}` |
| OpenRouter | Provider-dependent — some forward thinking, some don't |

#### Reasoning Request Parameters

Different providers accept reasoning/thinking configuration in different formats:

| Provider | Parameter | Values |
|---|---|---|
| OpenAI | `reasoning_effort` | `low`, `medium`, `high` |
| Anthropic | `thinking: {type, budget_tokens}` | `enabled`/`disabled`, budget in tokens |
| OpenRouter | `reasoning: {effort}` | Provider-specific effort levels |
| DeepSeek | `thinking: {type}` + `reasoning_effort` | `enabled`/`disabled`, effort mapping |
| Z.ai / Qwen | `enable_thinking` | boolean |

Tau maps `ThinkingLevel` (`off`, `minimal`, `low`, `medium`, `high`, `xhigh`) to provider-specific parameters via a compat mapping system.

---

## 7. Session Management Design

### 7.1 Session Persistence Format

**Format**: JSONL (JSON Lines) — one JSON object per line, append-only.

**Session header** (first line):
```json
{"type":"session","version":1,"id":"uuid","timestamp":"2026-05-02T...","cwd":"/path","name":"auto-generated-name"}
```

**Entry types** (aligned with PI's entry types, MVP subset):
| Type | Purpose | LLM-visible | PI Equivalent |
|---|---|---|---|
| `message` | User/assistant/tool messages | Yes | SessionMessageEntry |
| `model_change` | Model switch event | No | ModelChangeEntry |
| `thinking_level_change` | Thinking level adjustment | No | ThinkingLevelChangeEntry |
| `compaction` | Compaction summary | Yes (as summary) | CompactionEntry |
| `custom_entry` | Subagent results, internal state | No | CustomEntry |
| `custom_message` | Extension messages | Yes | CustomMessageEntry |
| `session_info` | Display name, metadata | No | SessionInfoEntry |

**Naming convention**: Session files named as `<timestamp>_<8-char-hex-id>.jsonl` (e.g., `20260502T143022_a3f7b2c1.jsonl`). Timestamp ensures collision-free filenames even if hex ID collides. For display, only the 8-char hex ID is shown.

### 7.2 Session Lifecycle

```
Create → Append messages → Auto-save → Compact (when needed) → Resume (restart) → Delete
```

| Operation | Behavior |
|---|---|
| **Create** | New JSONL file, header written, auto-generated name |
| **Append** | Immediate append, `\n` separator |
| **Auto-save** | Every message appended immediately |
| **Resume** | Read JSONL, rebuild message list from entries |
| **Compact** | Summarize old messages, replace with compaction entry |
| **Delete** | Remove session file |

### 7.2a Session Resume Algorithm

```
1. Open session file for reading
2. Read first line → parse session header, validate version
3. For each subsequent line:
   a. Parse JSON into SessionEntry
   b. If parse fails → stop, discard incomplete last line (corruption recovery)
   c. Switch on entry type:
      - `message` → append to message list
      - `model_change` → update current model
      - `thinking_level_change` → update thinking level
      - `compaction` → note firstKeptEntryId for context rebuild
      - `custom_entry` → store in metadata (not in message list)
      - `custom_message` → append to message list (LLM-visible)
      - `session_info` → update session metadata
4. If compaction entries exist, rebuild context:
   a. Walk compaction entries in order
   b. For each: replace messages before firstKeptEntryId with summary
5. Return rebuilt message list + current model + thinking level
```

### 7.2b Model Resolution

```
1. User provides model pattern (e.g., "claude", "gpt-4", "gemini-2.5")
2. If pattern is exact model ID → return matching model
3. If pattern is partial → search registry by:
   a. Model name (case-insensitive substring match)
   b. If multiple matches → prompt user to select
4. If no pattern provided → use default model from config
5. If config has no default → prompt user with available models
```

### 7.3 Auto-Naming Strategy

1. **Default**: Use first user message, truncated to 50 chars, stripped of special characters
2. **Fallback**: Timestamp-based name (`2026-05-02-143022`)
3. **Refinement**: User can rename session via `/name <new-name>` command
4. Stored in `session_info` entry

No LLM call required for naming — instant and deterministic.

### 7.4 Session Directory Structure

```
~/.tau/sessions/
├── <encoded-cwd>/
│   ├── 20260502T143022_a3f7b2c1.jsonl
│   └── 20260502T150112_b4e8c3d2.jsonl
└── <encoded-cwd>/
    └── 20260503T092200_c5f9d4e3.jsonl
```

- CWD encoded by replacing `/` with `-` (e.g., `/home/adam/Projects/tau` → `-home-adam-Projects-tau-`), matching PI's approach. Human-readable, no collision risk.
- Session file naming: `<timestamp>_<8-char-hex-id>.jsonl` — timestamp ensures uniqueness.

### 7.5 Compaction Strategy

**Trigger**: Context tokens > (model context window - reserve tokens)

**Process** (turn-aware, matching PI's `prepareCompaction()` and `compact()`):

1. **Token estimation**: Use per-type heuristics:
   - Text: `chars / 4`
   - Tool results: `chars / 3` (more token-dense due to structured formatting)
   - Thinking blocks: `chars / 3.5`
   - Images: estimated from metadata

2. **Find cut point**: Walk backwards from end, accumulating token estimates until budget exceeded.

3. **Constrain to turn boundaries**: Never split a tool call from its result. Cut points must be at:
   - After a complete turn (user → assistant → tool results)
   - After a user message (start of a new turn)
   - Never mid-tool-call-batch, never between tool call and result

4. **Split turn handling**: If a single turn exceeds `keepRecentTokens`:
   - Generate two summaries: (a) historical summary of prior turns, (b) turn prefix summary
   - Merge both summaries into a single compaction entry

5. **Iterative compaction**: On repeated compactions, the summarized span starts at the previous compaction's `firstKeptEntryId`, not the compaction entry itself.

6. **LLM summarization**: Call the active model with a structured summarization system prompt.

7. **Replace**: Summarized messages replaced with `<summary>...</summary>` XML block in structured format:
   ```
   <summary>
   ## Goal
   ## Constraints
   ## Progress
   ## Key Decisions
   ## Next Steps
   ## Critical Context
   ## Files Read
   ## Files Modified
   </summary>
   ```

8. **Write compaction entry**: Append to JSONL with `firstKeptEntryId`, `tokensBefore`, `details`.

**Settings**:
- `reserveTokens`: 16384 tokens (buffer for tool results + new turn)
- `keepRecentTokens`: 20000 tokens (recent context kept without summarization)

**MVP simplification**: No branching, simple linear compaction.

---

## 8. Data Models and Persistence Layer

### 8.1 Core Data Structures

```go
// Message types
type AgentMessage struct {
    ID        string
    Role      string          // "user" | "assistant" | "tool_result"
    Content   []ContentBlock
    Timestamp time.Time
    // Provider tracking — which API/model produced this assistant message
    API       string          // "openai-responses" | "anthropic-messages" | ...
    Model     string          // Model ID that generated this response
}

type ContentBlock struct {
    Type     string           // "text" | "thinking" | "tool_call" | "image"
    Text     string
    ToolCall *ToolCallBlock
    Image    *ImageBlock
}

type ToolCallBlock struct {
    ID        string
    Name      string
    Arguments map[string]any
}

type ImageBlock struct {
    Data     string    // Base64
    MimeType string
}

// Tool
type Tool interface {
    Name() string
    Description() string
    Parameters() any            // Go struct type for JSON schema generation via invopop/jsonschema
    ExecutionMode() ExecutionMode  // Parallel | Sequential | ExclusivePerFile
    Execute(ctx context.Context, params any) (*ToolResult, error)
}

type ExecutionMode string

const (
    ExecutionParallel    ExecutionMode = "parallel"
    ExecutionSequential  ExecutionMode = "sequential"
    ExecutionExclusive   ExecutionMode = "exclusive_per_file"
)

type ToolResult struct {
    Content   []ContentBlock
    Details   any
    IsError   bool
    Terminate bool              // Hint to stop agent after this batch
}

// BashExecution captures semantic bash execution details
type BashExecution struct {
    Command          string
    Output           string
    ExitCode         int
    Cancelled        bool
    Truncated        bool
    FullOutputPath   string          // Path to full output file if truncated
    ExcludeFromContext bool         // User excluded from context (!!command)
}

// Session entry (JSONL)
type SessionEntry struct {
    Type      string            // "session" | "message" | "model_change" | "compaction" | "custom_entry" | "custom_message" | "session_info" | "thinking_level_change"
    ID        string
    ParentID  string            // For future tree/branching support
    Data      json.RawMessage   // Type-specific payload
    Timestamp time.Time
}
```

### 8.1a Tool Execution Parallelism

Each tool declares its `ExecutionMode`:

| Execution Mode | Tools | Behavior |
|---|---|---|
| `Parallel` | `read`, `grep`, `find`, `ls`, `websearch`, `webfetch` | Can execute concurrently with other parallel tools |
| `Sequential` | `write`, `edit` | Must execute one at a time; serialized via per-file mutex chain |
| `Exclusive` | `bash` | Runs alone — no other tool executes concurrently |

**Execution flow** when assistant returns multiple tool calls:
1. **Preflight**: Validate all tool calls sequentially (schema check, `BeforeToolCall` hook)
2. **Group**: Partition tools by execution mode
3. **Execute exclusive first**: Run `bash` calls one at a time
4. **Execute sequential group**: Run `write`/`edit` via per-file mutex chain (serialize writes to same file)
5. **Execute parallel group**: Run all parallel tools concurrently via `errgroup` (read/grep/find/ls — after mutations so they can see write results)
6. **Collect results**: All results gathered, `AfterToolCall` hook applied to each
7. **Create ToolResultMessages**: One per tool call, in original source order

**File mutation queue** (per-file serialization):
- `write` and `edit` tools acquire a per-file mutex before executing
- Prevents race conditions when multiple tools target the same file
- Implemented as `map[string]*sync.Mutex` with lazy initialization

### 8.2 Storage Format Justification

**JSONL chosen over SQLite because**:
1. **Simplicity**: Append-only file, no database engine needed
2. **Proven**: PI uses this successfully — battle-tested format
3. **Portability**: Plain text, easy to inspect, debug, backup
4. **Single binary**: No CGO dependency (SQLite requires CGO or third-party driver)
5. **Streaming**: Natural fit for append-only session recording
6. **MVP scope**: No complex queries needed — only sequential read and append

**Trade-offs acknowledged**:
- No efficient random access (mitigated by index file in future)
- No complex querying (not needed for MVP)
- File size grows (mitigated by compaction)

### 8.3 Migration Strategy

- Session version in header (`version: 1`)
- Migration functions on load: `migrateV1ToV2()` etc.
- Backward compatible: old sessions readable by new versions
- No forward compatibility needed (single-user tool)

### 8.4 Backup/Recovery

- JSONL files are plain text — trivial to backup
- Each entry is a complete JSON object — file corruption only affects last line
- Recovery: read valid lines until parse error, discard incomplete last line

### 8.5 Config File Format

**Format**: JSON (`~/.tau/config.json`)

```json
{
  "providers": {
    "openai": {
      "model": "gpt-4o"
    },
    "anthropic": {
      "model": "claude-sonnet-4-20250514"
    },
    "google": {
      "model": "gemini-2.5-pro"
    }
  },
  "default_model": "claude-sonnet-4-20250514",
  "compaction": {
    "reserve_tokens": 16384,
    "keep_recent_tokens": 20000
  },
  "subagent_timeout": "5m",
  "tool_allowlist": null,
  "read_only": false,
  "load_context_files": true,
  "search": {
    "backend": "auto",
    "searxng_url": "http://localhost:8964"
  }
}
```

#### Adding Custom OpenRouter Models

OpenRouter exposes 300+ models. By default, tau discovers the top 30 most popular models (round-robin across providers, sorted by context window). You can add any additional model ID via the `models` field:

```json
{
  "providers": {
    "openrouter": {
      "enabled": true,
      "models": [
        "anthropic/claude-sonnet-4-20250514",
        "openai/gpt-4o-2024-05-13",
        "minimax/minimax-m2.7",
        "cohere/command-r-plus-08-2024"
      ]
    }
  }
}
```

Custom models appear alongside the auto-discovered models in `/model`. The model ID format must match OpenRouter's naming convention: `author/model-name` (e.g., `openai/gpt-5`, `anthropic/claude-opus-4.7`). Free-tier models use the `:free` suffix (e.g., `deepseek/deepseek-v4-flash:free`).

**Rationale for JSON over YAML**: Consistency with `auth.json`, simpler parsing with stdlib `encoding/json`, no external dependency needed. YAML reserved only for skill frontmatter (required by Agent Skills standard).

---

## 9. Security and Error Handling

### 9.0 Tool Permissions

- **Tool allowlisting**: `--tools read,grep,find,ls` flag restricts available tools (matching PI's `--tools` flag)
- **Read-only mode**: `--read-only` flag disables write, edit, and bash tools entirely
- **Working directory scope**: Tools operate within cwd; user is ultimately responsible (single-user tool)

### 9.1 API Key Security

- Keys stored in `~/.tau/auth.json` with `0600` file permissions
- Keys loaded into memory, never logged
- Environment variables supported (preferred for CI/automation)
- Keys never included in session files or logs
- `git` commands automatically exclude `~/.tau/` directory

### 9.2 File System Access Controls

- Tools operate within user's working directory
- No path traversal protection beyond standard OS permissions (user trusts the agent)
- `edit` tool uses search/replace pattern — requires exact match to prevent accidental modification
- `write` tool overwrites — user responsibility to review before committing
- File mutation queue serializes writes per file — prevents race conditions

### 9.3 Error Propagation Strategy

```
Provider error (HTTP/network)
    │
    ▼
StreamEvent{Type: "error", Error: err}
    │
    ▼
Agent loop catches error
    │
    ├──► Retry with exponential backoff (max 2 retries)
    │       ├──► Rate limit (429) → parse Retry-After header, wait exact duration
    │       └──► Other errors → retry with exponential delay (1s, 2s, 4s)
    │
    └──► If retries exhausted → emit error message to user
            └──► Session state preserved, user can continue
```

**Error categories**:
| Category | Handling |
|---|---|
| Provider HTTP errors | Retry with backoff, surface to user |
| Tool execution errors | Return as tool result with `isError: true` |
| Context overflow | Trigger compaction, or error if compaction fails |
| File I/O errors | Return as tool result, don't crash |
| Invalid input | Return descriptive error, don't crash |

**Provider-branded error messages**: All error messages from `OpenAICompatProvider` include the provider name (e.g., "opencode-zen stream error: ..." instead of "OpenAI-compatible stream error: ..."). This applies to SSE streaming errors and HTTP status errors. Users see the provider they configured, not internal implementation details.

**Tool result persistence**: Tool results are persisted immediately (before LLM confirmation) to ensure recovery if the subsequent LLM call fails.

**Mid-turn crash recovery**: Session is append-only — on restart, `session.Session` replays JSONL, rebuilds message list from valid entries. Incomplete last line is discarded.

**Rate limit handling**: Each provider implementation parses `Retry-After` header (HTTP 429) and waits the exact duration. Generic rate limits without `Retry-After` use exponential backoff capped at 60 seconds.

### 9.4 Cost Tracking

Cumulative token usage and cost tracked per session:
- Every `StreamEvent` of type `done` includes final `Usage` struct
- `Session` accumulates usage across all turns
- SDK exposes `Session.Usage()` for querying current session cost
- CLI displays running cost in footer (future TUI)

### 9.5 Graceful Degradation

- Provider failure: agent can still use tools and read session history
- Skill loading failure: skill silently skipped, user notified
- Subagent failure: error returned to parent, parent continues
- Session corruption: recoverable from valid entries, partial data preserved
- Config missing: defaults applied, user prompted for required values

---

## 10. Decisions

See `DECISIONS.md` for full decision log. Key decisions for this architecture:

| # | Decision | Rationale |
|---|---|---|
| 1 | JSONL session storage | Simplicity, single binary, proven by PI |
| 2 | Provider interface per API type | Minimize duplication, OpenAI compatibility covers many providers |
| 3 | Go structs for tool parameters | Compile-time safety, no runtime schema validation |
| 4 | Channels for streaming | Idiomatic Go, no external event bus needed |
| 5 | No extension system in MVP | Single user, built-in capabilities sufficient |
| 6 | No branching sessions in MVP | Linear history sufficient, reduces complexity |
| 7 | Per-type token heuristics | chars/4 for text, chars/3 for tool results — no external tokenizer |
| 8 | 4-step auth resolution chain | CLI flag → auth.json → env → config file, matching PI |
| 9 | `github.com/invopop/jsonschema` for tool schemas | LLM requires proper JSON Schema; reflection-based generation is too complex |
| 10 | Subagents as first-class citizens | Core requirement, native implementation over extension pattern |
| 11 | Synchronous subagent execution | Parent waits with timeout — simplest correct approach for single user |
| 12 | `types` package for shared data structures | Eliminates import cycles, follows Go idiomatic patterns (cf. pi-ai) |
| 13 | CWD encoding via `/` → `-` replacement | Human-readable, no collision risk, matches PI |
| 14 | Auto-naming from first user message | No LLM call needed, instant, deterministic |
| 15 | No orchestrator package — agent loop IS the orchestrator | Orchestration is declarative (AGENTS.md), not imperative code. Simpler, aligned with how agentic tools actually work |

---

## 10.4 SDK Interface (Session)

The SDK's `Session` struct is the primary programmatic interface. It composes the agent loop, session persistence, skills, and subagents — analogous to PI's `AgentSession`. CLI is a thin consumer.

```go
func CreateSession(ctx context.Context, opts SessionOptions) (*Session, error)
func (s *Session) Prompt(ctx context.Context, message string) error
func (s *Session) Continue(ctx context.Context) error
func (s *Session) Steer(message string) error
func (s *Session) Subscribe(listener func(AgentEvent)) func()
func (s *Session) Compact(ctx context.Context) error
func (s *Session) Usage() types.Usage
func (s *Session) Model() types.Model
func (s *Session) SetModel(model types.Model) error
```

**SessionOptions**:
```go
type SessionOptions struct {
    Model         string          // Model pattern or exact ID
    WorkingDir    string          // cwd for tool execution
    SessionPath   string          // Explicit session file path (optional)
    Continue      bool            // Resume most recent session
    Ephemeral     bool            // Don't save session
    ToolAllowlist []string        // Restrict available tools
    ReadOnly      bool            // Disable write/edit/bash
}
```

## 10.5 CLI Modes and Session Management

### CLI Modes

| Flag | Mode | Description |
|---|---|---|
| (default) | Interactive | Full chat loop with readline input. User types messages, agent responds with streaming output. |
| `-p` / `--print` | Print | Single prompt → output → exit. Reads prompt from argument or stdin. Streams output to stdout. For scripting and piping. Example: `tau -p "review this file"` |
| `--mode json` | JSON | Structured JSON output for debugging and session analysis. Each event emitted as a JSON line. |

**Print mode output format**:
- Streaming text written directly to stdout as received
- Tool calls shown as: `🔧 tool_name(arg="value")`
- Tool results shown as: `✓ tool_name → result summary`
- Exit code: 0 on success, 1 on error

**JSON mode output format**:
- One JSON object per line (JSONL)
- Includes all agent events: `agent_start`, `message_start`, `text_delta`, `tool_execution_start`, `tool_execution_end`, `message_end`, `turn_end`, `agent_end`
- Final line: `{"type":"agent_end","usage":{...},"stop_reason":"stop"}`

### Session Management CLI Flags

| Flag | Behavior |
|---|---|
| (none) | New session, auto-named |
| `-c` / `--continue` | Continue most recent session |
| `-r` / `--resume` | List past sessions, interactive selection |
| `--session <path|id>` | Open specific session by ID or file path |
| `--no-session` | Ephemeral mode — don't save session |
| `--api-key <key>` | Override API key (highest priority in auth chain) |
| `--tools <list>` | Comma-separated tool allowlist (e.g., `read,grep,find,ls`) |
| `--read-only` | Disable write, edit, bash tools |
| `--no-context-files` | Skip AGENTS.md/CLAUDE.md loading |

---

## 11. TUI Architecture

### 11.1 Overview

Tau uses **bubbletea v2** for the TUI with a **viewport + textarea** layout. The terminal runs in alt-screen mode. The viewport displays the conversation history (scrollable), and the textarea at the bottom accepts user input.

```
┌─────────────────────────────────────────────┐
│  Header: tau · model · provider             │
├─────────────────────────────────────────────┤
│                                             │
│  ▸ User message                             │
│                                             │
│  ┌───────────────────────────────────────┐  │
│  │ Assistant response (markdown)         │  │
│  │                                       │  │
│  │ · thinking                            │  │
│  │ reasoning text...                     │  │
│  │                                       │  │
│  │ ✓ read_file  path: main.go            │  │
│  │ ↳ read → file contents                │  │
│  │                                       │  │
│  └───────────────────────────────────────┘  │
│                                             │
│  ─────────────────────────────────────────  │
│                                             │
│  ▸ Follow-up question                       │
│                                             │
├─────────────────────────────────────────────┤
│  Footer: model · turns · tokens · cost      │
├─────────────────────────────────────────────┤
│  > Type a message...                        │
│                                             │
└─────────────────────────────────────────────┘
```

### 11.2 Component Structure

| Component | Purpose |
|---|---|
| `viewport` | Scrollable conversation display (soft-wrap disabled, content pre-formatted) |
| `textarea` | Multi-line user input (dynamic height, 1-8 lines) |
| `spinner` | Custom bouncing dots spinner in footer during streaming |
| `blocks` | Slice of `messageBlock` — finalized rendered content |
| `pendingBuilder` | `strings.Builder` for accumulating streaming content |

### 11.3 Event Delivery

Agent events are delivered from the agent goroutine to the bubbletea event loop via `tea.Program.Send()`:

```
Agent goroutine                          Event loop
     │                                       │
     ├──► session.Subscribe(callback)───────►│
     │    callback: p.Send(AgentEventMsg)    │
     │                                       │
     │──► Session.Prompt() runs agent loop───►│
     │                                       │
     │──► Prompt returns ────────────────────►│
     │    p.Send(PromptDoneMsg)              │
```

**Critical design decision**: `p.Send()` is used instead of a blocking channel-read `tea.Cmd` in `tea.Batch`. Bubbletea v2's `execBatchMsg` uses `sync.WaitGroup` — a blocking channel command in a batch causes a 3-way deadlock during tool execution.

### 11.4 Rendering Pipeline

#### Block-based rendering

Content is rendered as a slice of `messageBlock` structs, each with a `kind` (user message, assistant text, thinking, tool call, tool result, turn separator, error, subagent start/end):

```
Events → processEvent() → pendingBuilder → flushPending() → blocks[] → renderBlocks() → viewport
```

#### Streaming vs finalized

- **Streaming**: Content accumulates in `pendingBuilder` via `AgentEventTextDelta` / `AgentEventThinkingDelta`. Rendered as plain text on each event (no glamour).
- **Finalized**: On `AgentEventMessageEnd`, `flushPending()` creates a `messageBlock` with `isFinalized=true`. Assistant text blocks get glamour-rendered output cached in `renderedMarkdown`.

#### Markdown rendering

Assistant messages are rendered as markdown using `github.com/charmbracelet/glamour`:

1. **During streaming**: Plain text rendering via `assistantBlockStyle` (lipgloss dark background). Glamour cannot handle incomplete markdown (unclosed code blocks, broken lists).
2. **On message end**: `RenderMarkdown(text, width)` renders through glamour with the "dark" standard style, then applies OSC 8 hyperlink wrapping via `WrapURLsWithMarkdown()`.
3. **Caching**: Each `messageBlock` stores its glamour-rendered output in `renderedMarkdown`. Subsequent renders return the cached value.
4. **Resize**: On `tea.WindowSizeMsg`, all `renderedMarkdown` caches are cleared. Next `updateViewport()` call re-renders through glamour with the new width.

```
Assistant text block:
  isFinalized=false → plain text (streaming)
  isFinalized=true, renderedMarkdown="" → RenderMarkdown() → cache → return
  isFinalized=true, renderedMarkdown≠"" → return cached value
```

#### OSC 8 hyperlinks

URLs in rendered markdown are wrapped with OSC 8 terminal hyperlink escape sequences:

```
\x1b]8;;https://example.com\x1b\\visible text\x1b]8;;\x1b\\
```

- URLs inside code blocks are **not** wrapped (already syntax-highlighted by glamour).
- Code block detection uses content-based matching: extracts code content from fenced blocks in the original markdown, then finds those strings in the rendered output (stripping ANSI for comparison).
- Glamour renders bare URLs as `url (url)` format — the parenthesized duplicate is detected and skipped to avoid double-wrapping.
- `www.` URLs are normalized to `https://` in the OSC 8 target URI.

#### Rendering performance

| Content size | Render time | Memory |
|---|---|---|
| ~1K chars (typical) | ~1.2ms | ~596KB |
| ~10K chars (long) | ~3.7ms | ~1.8MB |
| Empty | ~1ns | 0B |

### 11.5 State Machine

```
IDLE ──Enter/submit──► STREAMING ──AgentEnd/PromptDone──► IDLE
         │                      │
         │                      ├──ToolExecStart──► (still streaming)
         │                      ├──ToolExecEnd──►   (still streaming)
         │                      └──Error──►         (still streaming)
         │
         └─Ctrl+C─────────► Abort (cancel context)
```

### 11.6 Key Bindings

| Key | Action |
|---|---|
| `Enter` | Send message |
| `Shift+Enter` | New line in input |
| `Ctrl+D` | Quit (when input is empty) |
| `Ctrl+C` | Abort current response (double-tap to exit) |
| `Esc` | Clear input |
| `PgUp/PgDn` | Scroll viewport |
| `Mouse wheel` | Scroll viewport |
| `Mouse click+drag` | Select text in viewport (auto-scrolls at edges) |
| `Mouse release` | Copy selected text to clipboard |

### 11.6.1 Mouse Selection

The viewport supports mouse-based text selection with auto-scroll and clipboard copy:

**How it works:**
1. **Click** (left button) starts selection at cursor position
2. **Drag** updates selection end position; auto-scrolls when cursor is within 3 rows of viewport top/bottom edge
3. **Release** ends selection and copies text to clipboard

**Clipboard mechanism:**
- Primary: OSC 52 escape sequence (works in Kitty, Ghostty, Alacritty, WezTerm, iTerm2, tmux, screen)
- Fallback (Linux): `wl-copy` (Wayland) → `xclip` (X11) → `xsel`
- Fallback (macOS): `pbcopy`
- Fallback (Windows): `clip`

**Visual feedback:**
- Selected text range is highlighted using viewport's `SetHighlights` API
- Highlight style: blue background (`color 57`) with white foreground

**Implementation:**
- Mouse mode: `tea.MouseModeCellMotion` (captures click, drag, release events)
- Selection state: `selectionState` struct with start/end line/column
- Auto-scroll: `tea.Every(100ms)` timer triggers `AutoScrollMsg` during drag near edges
- Text extraction: Works from raw block content (not rendered output with ANSI codes)

### 11.7 Command System

#### Command Registry

Commands are registered in a `CommandRegistry` with a `Command` struct containing `Name`, `Description`, `Handler`, and optional `Available` function. Built-in commands are registered at startup via `registerAll()`.

| Built-in Command | Description | Availability |
|---|---|---|
| `/quit`, `/exit` | Exit the application | Always |
| `/help` | Show help text | Always |
| `/new` | Start a new session | Idle only |
| `/resume` | Resume a previous session | Idle only |
| `/name <name>` | Rename the current session | Idle only |
| `/session` | Show session information | Always |
| `/model` | Change the active model | Idle only |
| `/compact` | Trigger context compaction | Idle only |
| `/clear` | Clear the viewport | Always |
| `/skills` | List available skills | Always |
| `/skill:<name>` | Load a skill's content | Idle only |
| `/reload` | Reload custom commands | Idle only |

#### Command Dropdown

Typing `/` opens an inline dropdown above the textarea. Commands are filtered using fuzzy subsequence matching with scoring (prefix, consecutive, word-boundary bonuses). Navigation via arrow keys, Enter/Tab to select, Esc to close. The viewport shrinks when the dropdown is active.

#### Custom Commands

Custom commands are defined as markdown files with YAML frontmatter in `.tau/commands/` (project) or `~/.tau/commands/` (global). Priority order: project > global > embedded.

```markdown
---
name: test
description: Run tests with coverage
model: openai/gpt-4o
---
Run `go test ./... -v` and analyze failures.
Focus on: $ARGUMENTS
First file: $1
```

**Template placeholders:**
- `$ARGUMENTS` — all arguments after command name
- `$1`, `$2`, `$3` — positional arguments (split by whitespace)
- Missing positional args → empty string

**Discovery:** Commands are loaded at startup via `LoadCustomCommands(cwd, embedded)`. The `/reload` command re-discovers custom commands without restarting.

**Dropdown integration:** Custom commands appear alongside built-in commands with a `[custom]` tag in the description. Built-in commands always take priority — custom commands cannot override them.

**File format:**
- Filename without `.md` is the fallback command name (overridden by `name` in frontmatter)
- `description` — shown in dropdown (required for usability, defaults to "Custom command")
- `model` — optional model override (reserved for future use)
- `agent` — optional agent override (reserved for future use)
- Content after `---` closing delimiter is the prompt template

#### Session Resume (`/resume`)

The `/resume` command opens a palette-based session picker listing past sessions for the current working directory, sorted by most recent first.

**Flow:**
1. `cmdResume()` scans `~/.tau/sessions/<encoded-cwd>/` for `.jsonl` files
2. Parses each file header to extract name, timestamp, ID
3. Opens palette with `sessionResumeItem` entries showing relative time ("5m ago") and session ID
4. User selects a session → palette task step runs `resumeSessionTask()`
5. Task closes current session, calls `sdk.CreateSession(SessionPath=...)` to resume
6. On success, `handleResumeComplete()` resets TUI state (blocks, usage, turnCount, cached fields)

**Design decisions:**
- Palette-based (not separate full-screen picker) — consistent with `/model`, `/thinking`
- Current cwd only — global session listing deferred to post-MVP
- Session swap is synchronous within palette task — TUI model's `m.session` pointer replaced directly
- All TUI state reset after resume — ensures clean slate with resumed session's model/thinking level

---

## 11.8 Post-MVP Backlog

Items deferred or noted for future implementation:

| Item | Description | Priority |
|---|---|---|
| Tree/branching sessions | Full tree structure with `id`/`parentId` for branching conversations | Low |
| OAuth authentication | 5th step in auth resolution chain for providers supporting OAuth | Low |
| Session index file | For efficient random access to large session files | Low |
| Skill hot-reload | File watching for skill directory changes | Low |
| Custom command shell injection | `!command` runs shell and injects output into template | Medium |
| Custom command file references | `@file` includes file content in template | Medium |
| Actual tokenizer | Replace `chars/N` heuristics with real token counting for accurate compaction | Medium |

---

## 12. Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Token estimation inaccuracy | Compaction triggers too early/late | Start with per-type heuristics (chars/4, chars/3 for tool results), tune based on usage |
| Provider API differences | Broken streaming or tool calls | Extensive compatibility testing, per-provider integration tests |
| Large tool outputs | Context window exhaustion | Aggressive truncation with configurable limits |
| Subagent memory overhead | High memory usage with many subagents | Context cloning is shallow copy, synchronous execution limits concurrent subagents |
| Skill discovery performance | Slow startup with many skills | Cache skill index, lazy loading |
| JSONL file corruption | Session data loss | Per-entry validity, recovery from last valid entry |
| JSON Schema generation bugs | Tool calling failures | Unit test each tool's schema output against LLM requirements |
| Auth resolution complexity | Key not found at runtime | Clear error messages indicating which resolution step failed |
| Steering queue overflow | Lost user input | Buffered size of 10 with warning on overflow; user can retry |

---

## 13. Requirements Traceability

| Requirement | Architecture Section | Status |
|---|---|---|
| Orchestrator model | §3 Agent Loop Design (AGENTS.md-driven) | ✓ |
| Built-in skills | §4 Skills System | ✓ |
| Built-in subagent types | §5 Subagent System | ✓ |
| Core tools | §2 Package Structure (tools/) | ✓ |
| Sub-agent context model | §5.2 Context Model | ✓ |
| Sub-agent communication | §5.3 Communication Model | ✓ |
| Sub-agent results without pollution | §5.3 (configurable LLM visibility) | ✓ |
| Agent Skills standard | §4.4 Standard Compliance | ✓ |
| Skill discovery paths | §4.1 Discovery Mechanism | ✓ |
| Progressive disclosure | §4.2 Loading | ✓ |
| Cross-tool skill compatibility | §4.4 Standard Compliance | ✓ |
| OpenAI provider | §6.4 Provider Implementations | ✓ |
| Anthropic provider | §6.4 Provider Implementations | ✓ |
| Google Gemini provider | §6.4 Provider Implementations | ✓ |
| OpenCode Zen/Go providers | §6.4 Provider Implementations | ✓ |
| OpenRouter provider | §6.4 Provider Implementations | ✓ |
| Local models support | §6.4 Provider Implementations | ✓ |
| Session persistence | §7 Session Management | ✓ |
| Auto-naming | §7.3 Auto-Naming | ✓ |
| Resume across restarts | §7.2a Session Resume Algorithm | ✓ |
| Go language | §2 Package Structure | ✓ |
| Minimal dependencies | §1.5 External Dependencies | ✓ |
| Single binary | §8.2 Storage Format Justification | ✓ |
| Context files (AGENTS.md) | §3.5 Context Files | ✓ |

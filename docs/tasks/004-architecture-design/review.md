# Architecture Review — Tau

**Reviewer role:** Senior Systems Architect
**Date:** 2026-05-02
**Documents reviewed:** ARCHITECTURE.md, REQUIREMENTS.md, DECISIONS.md, task.md
**PI reference version:** pi-mono (verified via source docs)

---

## Finding 1 — Session Directory Encoding Is Wrong vs PI

**Section reference:** §7.4
**Issue type:** Contradiction

**Description:** The architecture states CWD is "encoded into safe directory name (hash or base64)". PI's actual implementation replaces `/` with `-` (e.g., `--home--adam--Projects--tau--`). This is a simple, human-readable encoding. Hash or base64 is neither needed nor compatible with PI's proven approach.

**Recommendation:** Use PI's `-` replacement encoding. It's simpler, human-readable, and avoids hash collisions entirely. If encoding is truly needed, document why PI's approach was rejected.

**Severity:** Low

---

## Finding 2 — Tree Structure (`id`/`parentId`) Contradicts MVP Design

**Section reference:** §8.1 (`ParentID` in `AgentMessage`), §2.1 (import graph), §1.3 (out of scope: branching)

**Description:** `AgentMessage` struct includes `ParentID string // For tree structure (future)`, yet §1.3 explicitly excludes branching from MVP and §7.5 calls it "simple linear compaction." This field is dead weight now and creates confusion. PI's session format is fundamentally tree-based (v3); every entry has `id` + `parentId`. Tau claims linear JSONL append but still carries the tree scaffolding. If tree support is truly deferred, remove `ParentID` from all structs. If it's needed for compaction to work correctly (it is in PI — `firstKeptEntryId` references entry IDs), then it needs to be in `SessionEntry` not `AgentMessage`.

**Recommendation:** Either (a) remove all tree-related fields and explicitly design linear-only, or (b) adopt PI's tree structure from the start. The half-measure creates migration pain later. Given that PI's tree structure is append-only with backward-compatible linear sessions (v1 auto-migrated to v3), adopting it now costs almost nothing and saves a schema rewrite.

**Severity:** Medium

---

## Finding 3 — Missing Session Entry Types Compared to PI

**Section reference:** §7.1

**Description:** Tau defines 6 entry types (`session`, `message`, `model_change`, `compaction`, `custom`, `session_info`). PI has 9+: `session`, `message`, `model_change`, `thinking_level_change`, `compaction`, `branch_summary`, `custom`, `custom_message`, `label`. Missing:
- `thinking_level_change` — required if reasoning/thinking support is planned (§6.2 Model has `Reasoning bool`)
- `custom_message` — PI distinguishes `CustomEntry` (non-LLM-visible) from `CustomMessageEntry` (LLM-visible). The architecture uses a single `custom` entry with an `LLM-visible` flag, which is inconsistent with PI's clean separation
- `label` — bookmarks in the session tree (future, but PI uses them extensively)

**Recommendation:** Align entry types with PI's naming and semantics. Specifically, split `custom` into `custom_entry` (non-LLM) and `custom_message` (LLM-visible) to match PI's `CustomEntry`/`CustomMessageEntry` distinction. Add `thinking_level_change` to the entry type set since the model struct already tracks reasoning capability.

**Severity:** Medium

---

## Finding 4 — Auth Resolution Chain Is Underspecified

**Section reference:** §6.3

**Description:** The architecture describes a 2-step resolution chain (env vars → config file → error). PI uses a 4-step chain: CLI `--api-key` flag → `auth.json` entry → environment variable → `models.json` custom keys. Missing from Tau:
- CLI flag override (critical for scripting)
- `auth.json` as a dedicated credential store with `0600` permissions (PI stores keys in `~/.pi/agent/auth.json`, not in general config)
- Shell command key resolution (e.g., `"!security find-generic-password -ws 'anthropic'"`) — PI supports this for password manager integration
- Environment variable reference resolution (key value can be `MY_ANTHROPIC_KEY` string that references another env var)
- OAuth token support (listed as deferred, but the resolution chain should be designed to accept it later)

**Recommendation:** Design the auth resolution chain to match PI's 4-step order. Add `auth.json` as a separate credential store. Support CLI `--api-key` flag. Document the three key formats (literal, env var reference, shell command). Leave hooks for OAuth tokens even if deferred.

**Severity:** High

---

## Finding 5 — Provider Interface Missing Thinking/Reasoning Level Support

**Section reference:** §6.1, §6.5

**Description:** The `Provider.Stream()` interface accepts `StreamOptions` but the struct is never defined. PI supports thinking levels (`off`, `minimal`, `low`, `medium`, `high`, `xhigh`) and passes them to provider API calls. The `Model` struct has `Reasoning bool` but no thinking level field. Without thinking level in the interface or options, reasoning-capable models (Claude 3.5+, o1, Gemini 2.5) cannot be used effectively. Also, PI uses `api` field on `AssistantMessage` to track which API type generated the response — this is missing from Tau's `AgentMessage`.

**Recommendation:** Add `ThinkingLevel` field to `StreamOptions`. Add `api` and `model` fields to `AssistantMessage` (or equivalent) to track which API type and model produced each response. This is essential for multi-model sessions and for provider-specific response parsing.

**Severity:** High

---

## Finding 6 — Tool Parameter Schema Generation Is Unspecified

**Section reference:** §8.1 (`Tool.Parameters() any`), §3.2 (tool interface)

**Description:** The `Tool` interface has `Parameters() any` returning a Go struct type for "JSON schema generation." But there is no specification of how Go struct tags are converted to JSON Schema. The LLM requires properly formatted JSON Schema for tool definitions. Go's `encoding/json` marshals to JSON, not to JSON Schema. Without a schema generator (reflect-based or manual), tools won't be callable by the LLM. PI uses TypeBox for runtime schema construction; Tau claims to replace it with Go structs but doesn't specify the schema generation mechanism.

**Recommendation:** Either (a) add a JSON Schema generator using Go reflection with struct tags (e.g., `jsonschema` tags), (b) have each tool manually provide a JSON Schema string, or (c) explicitly add a lightweight JSON Schema library as a dependency. Document which approach is chosen. This is a showstopper for tool calling.

**Severity:** Critical

---

## Finding 7 — Orchestrator Is a God Object

**Section reference:** §3.2

**Description:** The `Orchestrator` struct holds direct references to all seven subsystems: `agent`, `skills`, `subagents`, `session`, `config`, `tools`, `provider`. This is a classic God Object anti-pattern. It creates tight coupling, makes testing difficult, and violates the single responsibility principle. PI's SDK (`createAgentSession`) solves this via a factory pattern where the `AgentSession` owns lifecycle and delegates to specialized managers.

**Recommendation:** Split the orchestrator into focused components:
- `ContextManager` — system prompt composition, skill disclosure
- `WorkflowEngine` — skill → subagent → skill flows
- `SessionOrchestrator` — session lifecycle + persistence coordination

The `Orchestrator` should then coordinate these via interfaces, not own them directly. Alternatively, follow PI's `AgentSession` pattern where the session owns everything and the orchestrator is just the agent loop coordinator.

**Severity:** Medium

---

## Finding 8 — Missing Steering and Follow-Up Message Queues

**Section reference:** §3.3.2, §5.3

**Description:** PI has a message queue system for steering messages (delivered after current tool call batch) and follow-up messages (delivered after agent finishes). This is critical for interactive use — users can type while the agent is working. The architecture mentions `BeforeToolCall`/`AfterToolCall` hooks but completely omits the message queue mechanism. Without it, the user must wait for the agent to finish before providing new input.

**Recommendation:** Add a message queue subsystem to the agent loop design. Define `SteerQueue` and `FollowUpQueue` as buffered channels. Document the delivery semantics: steering interrupts after the current tool batch, follow-up waits for the full turn. This is essential for the interactive experience.

**Severity:** High

---

## Finding 9 — Compaction Strategy Lacks Turn-Aware Cut Points

**Section reference:** §7.5

**Description:** The architecture describes compaction as: "walk backwards from end, accumulating token estimates, find cut point, summarize." PI's compaction has critical sophistication this design omits:
1. **Cut points are constrained to turn boundaries** — never cut at tool results (they must stay with their tool call)
2. **Split turn handling** — when a single turn exceeds `keepRecentTokens`, PI generates two summaries (history + turn prefix) and merges them
3. **Iterative compaction** — on repeated compactions, the summarized span starts at the previous compaction's `firstKeptEntryId`, not the compaction entry itself
4. **Structured summary format** — PI uses a specific markdown format (Goal, Constraints, Progress, Key Decisions, Next Steps, Critical Context, read-files, modified-files)

The architecture's description is too vague to implement correctly.

**Recommendation:** Add explicit turn-aware cut point rules. Document that tool results cannot be separated from their tool calls. Define the split turn handling strategy. Specify the structured summary format. Reference PI's `prepareCompaction()` and `compact()` functions as the model.

**Severity:** High

---

## Finding 10 — Tool Execution Parallelism Is Missing

**Section reference:** §2.1 (tools/ package), §3.3.2

**Description:** PI supports parallel tool execution via `Promise.all` with sequential preflight for file mutations. The architecture mentions a "file mutation queue" but doesn't specify:
- Which tools can execute in parallel
- How the agent requests parallel vs sequential execution
- How the `executionMode` is determined (PI has it on the `AgentTool` interface)
- How parallel tool results are collected and ordered

For a Go implementation, this maps to `errgroup` or `sync.WaitGroup`, but the design doesn't specify the coordination model.

**Recommendation:** Add `ExecutionMode` to the `Tool` interface (`Parallel`, `Sequential`, `ExclusivePerFile`). Document the parallel execution model: agent returns multiple tool calls in one response → runtime executes compatible tools concurrently → results collected and returned in order. Use `errgroup` for Go implementation. The file mutation queue (per-file mutex chain) should be documented as the serialization mechanism for write/edit tools.

**Severity:** Medium

---

## Finding 11 — Session Auto-Naming Is Under-Specified

**Section reference:** §7.3

**Description:** The strategy says: "First assistant response analyzed for topic, name generated from first few meaningful words." This is impossibly vague. How is the topic extracted? Is it an LLM call? A regex? Keyword extraction? What happens if the first response is a tool call (no text)? PI uses the first user message as the default session name, with optional LLM-generated refinement via `/name` command.

**Recommendation:** Clarify the naming strategy. Options: (a) Use the first user message truncated to N chars (simplest, no LLM call), (b) Send first assistant response to a cheap model for summarization (costs tokens), (c) Use first meaningful text from either user or assistant. Option (a) is recommended for a personal tool — no LLM call needed, instant, deterministic.

**Severity:** Low

---

## Finding 12 — Missing Context File Support (AGENTS.md/CLAUDE.md)

**Section reference:** Entire document

**Description:** PI loads `AGENTS.md` and `CLAUDE.md` at startup from global, parent directories, and cwd — concatenated in order. This is a core feature for project conventions and context. The architecture doesn't mention context files anywhere. Given that Tau is a coding tool, context files are essential for project-specific instructions.

**Recommendation:** Add a context file subsystem. Define discovery paths matching PI: `~/.tau/AGENTS.md` (global), parent directories walking up from cwd, and cwd. Specify that matching files are concatenated. Allow override via `--no-context-files` flag.

**Severity:** High

---

## Finding 13 — Package Dependency Graph Has a Potential Cycle

**Section reference:** §2.2

**Description:** The import graph shows:
```
session → agent
orchestrator → agent, session, subagent
```
But `agent` depends on `provider` and `tools`. If `session` needs to rebuild context for resumption, it may need access to message types from `agent`. The graph claims acyclicity, but `orchestrator` depends on both `session` and `agent`, and `session` depends on `agent`. If `agent` ever needs session persistence (e.g., auto-save), this creates a cycle: `agent → session → agent`.

**Recommendation:** Introduce a shared `types` or `model` package that defines core data structures (`AgentMessage`, `ToolResult`, etc.) independently of behavior. Both `agent` and `session` depend on `types`, not on each other. This eliminates the cycle and follows Go idiomatic patterns (see `pi-ai` package as PI's shared types layer).

**Severity:** High

---

## Finding 14 — Error Handling Lacks Retry Budget and Circuit Breaker

**Section reference:** §9.3

**Description:** The error propagation strategy specifies "max 2 retries with exponential backoff" but doesn't address:
- Rate limit handling across providers (429 with `Retry-After` vs. generic backoff)
- Circuit breaker pattern — if a provider is consistently failing, should requests be queued or failed fast?
- Partial failure recovery — what happens if a tool call succeeds but the provider call to report results fails?
- Context loss on error — if the agent crashes mid-turn, is the session recoverable?
- Tool call result persistence before LLM confirmation — PI saves tool results immediately; if the subsequent LLM call fails, the tool result is already persisted

**Recommendation:** Add per-provider rate limit handling with `Retry-After` header parsing. Define a circuit breaker (open after N consecutive failures, half-open after timeout). Specify that tool results are persisted immediately (before LLM confirmation) to ensure recovery. Define the recovery procedure for mid-turn crashes.

**Severity:** Medium

---

## Finding 15 — Missing Cost Tracking and Token Usage Display

**Section reference:** §6.2, §6.5

**Description:** PI tracks cost per session (input/output/cache tokens with dollar amounts) and displays it in the footer. The `Model` struct has `Cost CostInfo` but the `StreamEvent` and `Usage` types are under-specified. The `Usage` field in `StreamEvent` is `*Usage` but the struct is never defined. Without cost tracking, the user has no visibility into spending — critical even for a personal tool.

**Recommendation:** Define `Usage` struct matching PI's: `Input`, `Output`, `CacheRead`, `CacheWrite`, `TotalTokens`, `Cost` (with per-type dollar amounts). Ensure every `StreamEvent` of type `done` includes final usage. Track cumulative usage per session and expose via the SDK.

**Severity:** Medium

---

## Finding 16 — External Dependency Claim May Be Unrealistic

**Section reference:** §1.4

**Description:** The architecture claims only `gopkg.in/yaml.v3` as an external dependency. This is likely insufficient for:
- **JSON Schema generation** (Finding 6) — need a reflect-based JSON Schema generator
- **CLI argument parsing** — the `cmd/tau` entry point needs `flag` or `cobra`. If using stdlib `flag`, it's limited for subcommands
- **UUID generation** — session IDs need UUIDs; stdlib doesn't provide them
- **File watching** (for future skill hot-reload) — would need `fsnotify`
- **Terminal/TTY detection** — for interactive vs pipe mode detection

The claim might be achievable with careful stdlib usage, but Finding 6 (JSON Schema) alone may require a third-party package.

**Recommendation:** Be honest about which dependencies are truly unavoidable. Document the rationale for each. For JSON Schema, either write a minimal reflect-based generator (acceptable for a personal tool) or explicitly accept a dependency. For UUIDs, use `crypto/rand` to generate hex strings (8 hex chars as the architecture states — though this is only 2^32, see Finding 18).

**Severity:** Medium

---

## Finding 17 — Subagent Result Injection Ambiguity

**Section reference:** §5.3, §3.3.1 (step 6)

**Description:** The architecture says results are "injected back to parent context (not as LLM-visible message)" in §3.3.1 but §5.3 says "Results returned as `CustomEntry` (not LLM-visible) or as `ToolResultMessage` (LLM-visible) based on caller preference." This is contradictory within the same document. Additionally, the mechanism for injecting results is unclear: does the parent agent loop pause while the subagent runs? Does it continue processing and the result is queued? What if the parent completes before the subagent?

**Recommendation:** Clarify: (a) The default behavior — results are LLM-visible (as `ToolResultMessage`-equivalent) so the parent agent can act on them. (b) The opt-out — results can be non-LLM-visible for logging/metadata. (c) The execution model — subagent runs synchronously (parent waits) or asynchronously (result queued). For a personal tool, synchronous with timeout is the simplest correct approach. Document the exact injection point in the agent loop.

**Severity:** High

---

## Finding 18 — Session ID Collision Risk

**Section reference:** §7.4

**Description:** "Session ID: short UUID (8 hex chars)" — 8 hex chars = 32 bits = 2^32 ≈ 4.3 billion possibilities. By the birthday paradox, collisions become likely after ~65,000 sessions. For a personal tool, this might be acceptable, but PI uses the same 8-char hex IDs. However, PI's file naming includes a timestamp prefix (`<timestamp>_<uuid>.jsonl`), making the full filename unique even if the UUID portion collides.

**Recommendation:** Follow PI's naming convention: `<timestamp>_<8-char-hex-id>.jsonl`. This makes the filename collision-free (timestamp + ID) while keeping IDs short for display. Alternatively, use a full UUID internally and truncate for display.

**Severity:** Low

---

## Finding 19 — Missing Bash Execution as Dedicated Message Type

**Section reference:** §8.1

**Description:** PI has `BashExecutionMessage` as a distinct type with `Command`, `Output`, `ExitCode`, `Cancelled`, `Truncated`, `FullOutputPath`, and `ExcludeFromContext` fields. Tau rolls bash output into generic `ToolResult`. This loses important semantics:
- Bash commands can be excluded from context (`!!command` in PI)
- Exit codes are semantically different from tool errors
- Cancellation state (user pressed Ctrl+C) is distinct from execution failure
- Truncation metadata is lost

**Recommendation:** Add `BashExecution` as a distinct content block type or message role. Include `Command`, `Output`, `ExitCode`, `Cancelled`, and `Truncated` fields. This improves session replay, debugging, and tool result semantics.

**Severity:** Low

---

## Finding 20 — Missing CLI Modes (Print, JSON)

**Section reference:** §1.1, §1.3

**Description:** PI supports four modes: interactive, print (`-p`), JSON (`--mode json`), and RPC (`--mode rpc`). The architecture only describes the CLI and SDK layers without mode support. Print mode is essential for scripting and piping. JSON mode is essential for debugging and session analysis.

**Recommendation:** Add CLI mode support to the architecture. At minimum, support print mode (`-p`) for non-interactive use. JSON mode is valuable for debugging. Define the output format for each mode.

**Severity:** Medium

---

## Finding 21 — StreamOptions Struct Never Defined

**Section reference:** §6.1

**Description:** The `Provider.Stream()` method accepts `StreamOptions` but this struct is never defined. Given the gaps found elsewhere (missing thinking level, model tracking, etc.), this struct needs explicit definition.

**Recommendation:** Define `StreamOptions` explicitly:
```go
type StreamOptions struct {
    ThinkingLevel string          // "off" | "minimal" | "low" | "medium" | "high" | "xhigh"
    MaxTokens     int
    Temperature   float64
    SystemPrompt  string          // Override default system prompt
    Tools         []ToolDefinition
}
```

**Severity:** Medium

---

## Finding 22 — Missing Resume/Continue CLI Interface

**Section reference:** §7.2

**Description:** The session lifecycle lists "Resume" as an operation but the CLI interface for selecting a session to resume is not defined. PI supports `-c` (continue most recent), `-r` (browse and select), `--session <path|id>`, and `--fork <path|id>`. Without these, session resumption is not actionable.

**Recommendation:** Define the CLI flags for session management:
- `-c` / `--continue` — continue most recent session
- `-r` / `--resume` — list and select from past sessions
- `--session <path|id>` — open specific session
- `--no-session` — ephemeral mode (don't save)

**Severity:** Medium

---

## Finding 23 — Skills Discovery Path Inconsistency

**Section reference:** §4.1

**Description:** The architecture lists global paths as `~/.tau/skills/` and `~/.agents/skills/`. PI uses `~/.pi/agent/skills/` and `~/.agents/skills/`. The `.agents/skills/` path is for cross-tool compatibility (shared with Claude Code, OpenCode). But PI also discovers skills from `.pi/skills/` (project-level) and walks up parent directories for project skills. The architecture's project path `.agents/skills/` in cwd only checks cwd, not parent directories.

**Recommendation:** Match PI's discovery: project skills should walk up from cwd through parent directories, checking `.agents/skills/` at each level. This is important for monorepo scenarios where a skill is defined at the repo root but invoked from a subdirectory.

**Severity:** Low

---

## Finding 24 — Orchestrator ↔ Agent Loop Interface Is Unclear

**Section reference:** §3.3, §5.1

**Description:** The orchestrator "manages event flow" (§3.1, point 5) and the agent loop "emits events" (§1.2). But the interface between them is not defined. Does the orchestrator create the agent? Does the agent call back into the orchestrator? Who owns the tool registry? The data flow diagram shows the agent loop receiving prompt from the orchestrator but doesn't show the event return path.

**Recommendation:** Define the orchestrator ↔ agent interface explicitly. Options: (a) Orchestrator creates agent, agent emits events via channel, orchestrator subscribes. (b) Agent calls orchestrator via callback interface for skill loading and subagent spawning. Document ownership: who creates the agent, who owns the session, who manages the tool registry.

**Severity:** High

---

## Finding 25 — Missing Prompt Template System

**Section reference:** Entire document

**Description:** PI supports prompt templates as reusable Markdown files with variable substitution (`{{focus}}`). These are distinct from skills (templates expand inline, skills add capabilities). For a personal coding tool, templates are useful for recurring tasks (code review, commit messages, PR descriptions). The architecture doesn't mention them.

**Recommendation:** Add a prompt template subsystem. Define format (Markdown with Go template syntax or simple `{{variable}}` substitution). Define discovery paths (`~/.tau/prompts/`, `.pi/prompts/`). Define invocation (`/templatename`). This is lower priority than other gaps but should be noted for post-MVP.

**Severity:** Low

---

## Finding 26 — Architecture Lacks Sufficient Detail for Task 005 (Task Breakdown)

**Section reference:** Overall, acceptance criteria for task.md

**Description:** Several components are described at too high a level to drive implementation tasks:
- **Agent loop**: No state machine definition, no event type enumeration (only `StreamEvent` types are defined, not agent loop events)
- **Tool execution**: No parallel/sequential execution model, no error handling for individual tool calls
- **Session resume**: No algorithm for rebuilding context from JSONL
- **Provider model selection**: No algorithm for resolving `--model` pattern to a specific model
- **SDK**: Only `sdk.go` is listed with two functions — no interface definition
- **Config**: No config format defined (YAML? JSON?) despite DECISIONS.md mentioning `config.yaml`

**Recommendation:** Before Task 005, add:
1. Agent loop state machine diagram with states and transitions
2. Complete event type enumeration for the agent loop (not just provider events)
3. Session resume algorithm (read JSONL → rebuild message list → validate)
4. Model resolution algorithm (pattern matching against registry)
5. SDK interface definition (what the SDK exposes, what it hides)
6. Config file format specification

**Severity:** High

---

## Finding 27 — Security: No Tool Permission Gates or Sandboxing

**Section reference:** §9.2

**Description:** "No path traversal protection beyond standard OS permissions (user trusts the agent)." For a personal tool, this is pragmatic. But the architecture doesn't mention:
- Tool allowlisting (PI supports `--tools read,grep,find,ls` for read-only mode)
- Bash command allowlist or blocklist
- Working directory restriction (can the agent modify files outside cwd?)
- Dangerous command detection (e.g., `rm -rf /`, `curl | bash`)

**Recommendation:** Add tool allowlisting as a config option. Document that this is a single-user tool and the user is ultimately responsible. Consider adding a `--read-only` flag that disables write, edit, and bash tools. This is already supported by PI's `--tools` flag.

**Severity:** Low

---

## Finding 28 — TUI Deferral Creates SDK Design Gap

**Section reference:** §11

**Description:** The SDK is described as a "public SDK" but the architecture doesn't separate SDK concerns from CLI concerns. If TUI is deferred, the SDK should be the primary programmatic interface. But `sdk.go` only has `CreateSession()` and `RunPrompt()` — no event subscription, no streaming, no steering. PI's SDK has `prompt()`, `steer()`, `subscribe()`, `compact()`, `branch()`.

**Recommendation:** Design the SDK as the primary interface from the start, with CLI as a thin consumer. Define event subscription and streaming in the SDK. The TUI (when built) will also be an SDK consumer. This ensures the SDK is actually useful and not an afterthought.

**Severity:** Medium

---

## Finding 29 — Missing AGENTS.md Integration

**Section reference:** Entire document

**Description:** The project itself has an `AGENTS.md` at the root (`/var/home/adam/Projects/tau/AGENTS.md`). PI loads AGENTS.md files as context. Tau should be able to eat its own dog food — it should load the project's AGENTS.md when running in the tau directory. The architecture doesn't mention context file loading at all (see Finding 12), and specifically doesn't address how AGENTS.md files should be integrated.

**Recommendation:** Addressed in Finding 12. Additionally, document that Tau loads AGENTS.md from the project root and any parent directories, matching PI's behavior.

**Severity:** Low (duplicate of Finding 12)

---

## Finding 30 — `chars/4` Heuristic Claim About PI Is Inaccurate

**Section reference:** §7.5, DECISIONS.md (Decision 7)

**Description:** The architecture claims "PI uses the same heuristic" for `chars/4` token estimation. PI's actual compaction implementation is more nuanced: it uses character count for estimation but the actual token counting involves walking the message tree and summing estimated tokens per content block. The `chars/4` figure is a simplification. The bigger issue is that the architecture doesn't specify how token estimation handles different content types (tool results, images, thinking blocks all have different token densities).

**Recommendation:** Acknowledge that `chars/4` is a rough heuristic and specify per-content-type adjustments if any. Consider adding a configurable token estimation multiplier in config. Document that this is an MVP simplification and can be improved later with an actual tokenizer.

**Severity:** Low

---

## Summary by Severity

| Severity | Count | Key Issues |
|----------|-------|------------|
| Critical | 1 | #6 — Tool parameter JSON Schema generation is unspecified (showstopper) |
| High | 9 | #4 auth chain, #5 thinking levels, #8 message queues, #9 compaction cut points, #12 context files, #13 package cycle, #17 subagent injection, #24 orchestrator-agent interface, #26 insufficient detail for Task 005 |
| Medium | 9 | #2 tree structure, #3 entry types, #7 God object, #10 tool parallelism, #14 error handling, #15 cost tracking, #16 dependency claims, #21 StreamOptions, #22 resume CLI, #28 SDK design |
| Low | 11 | #1 directory encoding, #11 auto-naming, #18 session ID, #19 bash message type, #20 CLI modes, #23 skill paths, #25 prompt templates, #27 security gates, #29 AGENTS.md, #30 token heuristic |

## Priority Actions Before Task 005

1. **Define JSON Schema generation for tool parameters** (#6) — blocking implementation
2. **Complete the auth resolution chain** (#4) — provider support is a core requirement
3. **Add thinking/reasoning level to provider interface** (#5) — required for modern model support
4. **Add steering/follow-up message queues** (#8) — required for interactive use
5. **Resolve package dependency cycle** (#13) — affects all subsequent implementation
6. **Clarify orchestrator ↔ agent loop interface** (#24) — fundamental architectural question
7. **Add sufficient detail for Task 005** (#26) — acceptance criterion for this task
8. **Define compaction cut point rules** (#9) — critical for long-session reliability

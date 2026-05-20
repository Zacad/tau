# Task 005: Task Breakdown

## Why

Architecture is complete and reviewed (Task 004 DONE, 16 decisions documented, all Critical/High findings resolved). Requirements are defined (Task 002 DONE). PI internals are mapped (Tasks 001 & 003 DONE).

Now we need to decompose the architecture into actionable, independently implementable tasks. This breakdown defines the execution plan — dependency order, parallelization opportunities, and acceptance criteria for every implementation task (006 through 014). This is the bridge between architecture and code.

## Comparison Analysis: Task Organization vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Task Organization | Implicit through development history | Explicit task-driven breakdown with tracking |
| Dependency Management | Developer knowledge and intuition | Documented dependency graph from architecture |
| Task Granularity | Unknown (open source, iterative) | Package-level boundaries from ARCHITECTURE.md |
| Tracking | Git history | TRACKING.md with per-task acceptance criteria |
| Testing | Unknown | TDD enforced per task in AGENTS.md |
| Subagent Use | Extension-based | Native subagent for critical review of design |

**Why this matters**: PI evolved organically. Tau starts with a validated architecture and explicit task decomposition — reducing risk of rework and ensuring every requirement is traced to implementation.

## Main Constraints

- Each task must be independently implementable and testable
- Tasks must follow the architecture's strict dependency graph (acyclic)
- Each task must have clear, verifiable acceptance criteria
- TDD approach for all implementation tasks (AGENTS.md requirement)
- Go language only, idiomatic approach, minimal dependencies
- No task should depend on implementation details of another — only on its public interfaces
- Built-in skills (skill-builder, subagent-builder) are SKILL.md files, not code — included in skills task

## Architecture Inputs

The breakdown is driven by ARCHITECTURE.md components:

| Architecture Component | Section | Maps To Task |
|------------------------|---------|--------------|
| Core data structures (§8.1) | Data Models | 006 |
| Config file format (§8.5) | Data Models | 006 |
| Provider interface (§6.1) | Provider Abstraction | 007 |
| Model registry (§6.2) | Provider Abstraction | 007 |
| Auth resolution (§6.3) | Provider Abstraction | 007 |
| Provider implementations (§6.4) | Provider Abstraction | 007 |
| Streaming events (§6.5) | Provider Abstraction | 007 |
| Tool interface (§8.1) | Data Models | 008 |
| Tool execution parallelism (§8.1a) | Data Models | 008 |
| Built-in tools | ARCHITECTURE.md | 008 |
| Skill discovery (§4.1) | Skills System | 009 |
| Skill loading (§4.2) | Skills System | 009 |
| Skills standard (§4.4) | Skills System | 009 |
| Built-in skills (§4.3) | Skills System | 009 |
| Subagent lifecycle (§5.1) | Subagent System | 010 |
| Context model (§5.2) | Subagent System | 010 |
| Communication model (§5.3) | Subagent System | 010 |
| Built-in subagent types (§5.4) | Subagent System | 010 |
| Session persistence (§7.1) | Session Management | 011 |
| Session lifecycle (§7.2) | Session Management | 011 |
| Auto-naming (§7.3) | Session Management | 011 |
| Compaction (§7.5) | Session Management | 011 |
| Agent loop (§3) | Agent Loop Design | 012 |
| State machine (§3.4) | Agent Loop Design | 012 |
| Steering queues (§3.3) | Agent Loop Design | 012 |
| SDK interface (§10.4) | SDK Interface | 013 |
| CLI modes (§10.5) | CLI Modes | 014 |
| CLI session flags (§10.5) | CLI Modes | 014 |

## Dependency Graph

```
                          ┌──────────────────────────────────┐
                          │        Task 006: Foundation       │
                          │  types + config + testutil (no)   │
                          └─────┬─────┬─────┬─────┬──────────┘
                                │     │     │     │
              ┌─────────────────┤     │     │     ├──────────────┐
              ▼                 ▼     ▼     ▼     ▼              ▼
      ┌──────────────┐ ┌──────────┐ ┌───────┐ ┌────────┐ ┌──────────┐
      │ 007: Provider│ │ 008:Tool │ │ 009:  │ │ 010:   │ │ 011:     │
      │  (006)       │ │(006)     │ │Skills │ │Subagent│ │ Session  │
      │              │ │          │ │(006)  │ │(006)   │ │ (006)    │
      └──────┬───────┘ └────┬─────┘ └───────┘ └────────┘ └──────────┘
             │              │
             └──────┬───────┘
                    ▼
           ┌─────────────────┐
           │  012: Agent     │
           │  (006+007+008+  │
           │   010)          │
           └────────┬────────┘
                    ▼
           ┌─────────────────┐
           │  013: SDK       │
           │  (all internal) │
           └────────┬────────┘
                    ▼
           ┌─────────────────┐
           │  014: CLI       │
           │  (013)          │
           └─────────────────┘
```

**Critical Path**: 006 → 007 + 008 → 012 → 013 → 014

**Parallel Tracks** (after 006 completes):
- 009 (Skills) — fully independent
- 010 (Subagent) — depends on 006 only (uses `types.Model`, not `provider/` directly)
- 011 (Session) — fully independent

These three can run in parallel with each other and with 007/008.

## Proposed Task Breakdown

---

### Task 006: Foundation (types + config + infrastructure)

**Scope**: Project setup, core data structures, configuration system, build/test infrastructure.

**Packages**:
- `go.mod` — Module initialization, dependencies (`gopkg.in/yaml.v3`, `github.com/invopop/jsonschema`)
- `internal/types/` — All shared data structures (AgentMessage, ContentBlock, ToolCallBlock, ToolResult, SessionEntry, Provider types, StreamEvent, Usage, CostInfo, ExecutionMode, BashExecution, Model re-export for cross-package use, etc.)
- `internal/config/` — Configuration loading (`~/.tau/config.json`), path resolution (`~/.tau/`, `.agents/`, `~/.agents/`), defaults, compaction settings, subagent timeout, context files discovery paths (AGENTS.md/CLAUDE.md walk-up search list)
- `internal/testutil/` — Shared test utilities: mock provider, mock tools, temp filesystem helpers

**Key Decisions Applied**:
- Decision #12: `types` package eliminates import cycles
- Decision #14: Config format is JSON
- Decision #13: CWD encoding via `/` → `-`
- Auth resolution chain: `auth.json` at `~/.tau/auth.json` with `0600` permissions

**Dependencies**: None (leaf packages)

**Acceptance Criteria**:
- [ ] `go.mod` initialized with all external dependencies resolved
- [ ] All types from ARCHITECTURE.md §8.1 defined and compile
- [ ] `provider.Model` type re-exported in `types/` so 010, 011 can use it without importing `provider/`
- [ ] Config loads from `~/.tau/config.json` with defaults
- [ ] Path resolution returns correct built-in, global, and project paths
- [ ] Context files discovery: computes AGENTS.md/CLAUDE.md search list (cwd → parent dirs → global)
- [ ] CWD encoding produces human-readable directory names
- [ ] `internal/testutil/` provides mock Provider, mock Tool, temp filesystem helpers
- [ ] Unit tests cover all types and config loading
- [ ] Zero internal dependencies on other `internal/` packages (except `testutil` which depends on `types`)

---

### Task 007: Provider System

**Scope**: LLM provider abstraction, model registry, authentication, streaming.

**Packages**:
- `internal/provider/provider.go` — `Provider` interface (`Stream()`, `Complete()`)
- `internal/provider/model.go` — `Model` struct, model registry
- `internal/provider/registry.go` — Provider registration, model resolution algorithm
- `internal/provider/auth.go` — 4-step auth resolution (CLI flag → auth.json → env → config)
- `internal/provider/openai.go` — OpenAI provider (openai-responses API)
- `internal/provider/anthropic.go` — Anthropic provider (anthropic-messages API)
- `internal/provider/google.go` — Google Gemini provider (google-generative-ai API)
- `internal/provider/openai_compat.go` — OpenAI-compatible provider (covers OpenRouter, OpenCode Zen, OpenCode Go, Ollama, llama.cpp, LM Studio)
- `internal/provider/stream.go` — StreamEvent types, streaming channel handling

**Key Decisions Applied**:
- Decision #2: Provider interface per API type (not per provider)
- Decision #4: Go channels for streaming
- Decision #8: 4-step auth resolution chain
- Decision #3: Go structs for tool parameters + `github.com/invopop/jsonschema`

**Dependencies**: `internal/types/` (Task 006)

**Acceptance Criteria**:
- [ ] `Provider` interface satisfies all ARCHITECTURE.md §6.1 requirements
- [ ] Model registry supports exact ID and pattern matching
- [ ] Auth resolution chain works for all 3 key formats (literal, env ref, shell command)
- [ ] OpenAI, Anthropic, Google providers stream correctly
- [ ] OpenAI-compatible provider covers all 5 compat providers
- [ ] `StreamOptions` supports ThinkingLevel, MaxTokens, Temperature
- [ ] Rate limit handling (429 Retry-After) implemented
- [ ] Error propagation with exponential backoff (max 2 retries)
- [ ] Unit tests with mocked HTTP for all providers
- [ ] No internal dependencies except `types`

---

### Task 008: Tool System

**Scope**: Tool interface, registry, execution modes, built-in tools, file mutation queue.

**Packages**:
- `internal/tools/tool.go` — `Tool` interface, `Registry`, `ExecutionMode`
- `internal/tools/read.go` — File read tool
- `internal/tools/write.go` — File write tool
- `internal/tools/edit.go` — File edit tool (search/replace)
- `internal/tools/bash.go` — Shell execution tool
- `internal/tools/grep.go` — Content search tool
- `internal/tools/find.go` — File search tool
- `internal/tools/ls.go` — Directory listing tool
- `internal/tools/truncate.go` — Output truncation utilities
- `internal/tools/queue.go` — File mutation serialization (per-file mutex chain)

**Key Decisions Applied**:
- Decision #3: Go structs + jsonschema for tool parameters
- Decision #16: Tool parallelism specification (parallel: read/grep/find/ls, sequential: write/edit, exclusive: bash)
- Decision #9: Synchronous subagent execution (relevant for tool termination)

**Dependencies**: `internal/types/` (Task 006)

**Acceptance Criteria**:
- [ ] `Tool` interface matches ARCHITECTURE.md §8.1
- [ ] `ExecutionMode` enforced: parallel tools run concurrently, sequential serialized, exclusive runs alone
- [ ] File mutation queue serializes write/edit per file via mutex
- [ ] All 7 built-in tools implemented (read, write, edit, bash, grep, find, ls)
- [ ] Tool results include `isError` and `terminate` flags
- [ ] Truncation applies to large outputs with configurable limits
- [ ] Bash tool supports read-only command filtering for subagent contexts
- [ ] Registry provides `WithAllowlist(tools []string) Registry` API — accepts allowlist from SDK
- [ ] Registry provides `WithReadOnly(bool) Registry` API — disables write/edit/bash
- [ ] Unit tests with temp filesystem for all tools (use `testutil/` helpers)
- [ ] No internal dependencies except `types`

---

### Task 009: Skills System

**Scope**: Skill discovery, SKILL.md parsing, progressive disclosure, built-in skills.

**Packages**:
- `internal/skills/skill.go` — `Skill` struct
- `internal/skills/discovery.go` — 3-tier directory scanning (built-in, global, project)
- `internal/skills/parser.go` — SKILL.md YAML frontmatter parsing
- `internal/skills/prompt.go` — Progressive disclosure formatting (name + description)
- `skills/builtin/skill-builder/SKILL.md` — Built-in skill for creating skills
- `skills/builtin/subagent-builder/SKILL.md` — Built-in skill for creating subagents

**Key Decisions Applied**:
- Agent Skills standard compliance (§4.4)
- Progressive disclosure: only name + description in system prompt
- 3-tier discovery: built-in > global > project
- `gopkg.in/yaml.v3` for frontmatter parsing (only non-stdlib external dependency)

**Dependencies**: `internal/types/` (Task 006)

**Acceptance Criteria**:
- [ ] 3-tier discovery finds skills in all paths
- [ ] SKILL.md parsing validates name, description, frontmatter
- [ ] Name validation: lowercase, hyphens, 0-9, max 64 chars
- [ ] Directory name must match skill name
- [ ] Progressive disclosure output matches ARCHITECTURE.md §4.2 format
- [ ] Symlinks followed, `node_modules` skipped, `.gitignore` respected
- [ ] Built-in skills (skill-builder, subagent-builder) defined as SKILL.md files
- [ ] Graceful handling: invalid skills silently skipped with warning
- [ ] Unit tests with temp filesystem for discovery and parsing
- [ ] No internal dependencies except `types`

---

### Task 010: Subagent System

**Scope**: Subagent lifecycle, context management, built-in subagent types, result handling.

**Packages**:
- `internal/subagent/subagent.go` — `SubAgent` struct, lifecycle management
- `internal/subagent/context.go` — Context fork/clone (fresh vs fork modes)
- `internal/subagent/result.go` — `SubAgentResult`, result injection options
- `internal/subagent/builtin.go` — Built-in subagent type definitions (Researcher, Reviewer, Implementor, Security Reviewer, QA) with default tool sets

**Key Decisions Applied**:
- Decision #6: Subagents as first-class citizens (native, not extensions)
- Decision #10: Synchronous subagent execution with configurable timeout (default 5m)
- Parent ↔ child only communication
- LLM-visible result injection (default) with opt-out for custom entries
- No subagent-to-subagent communication

**Dependencies**: `internal/types/` (Task 006)

**Acceptance Criteria**:
- [ ] `SubAgent` struct matches ARCHITECTURE.md §5.5
- [ ] Context modes: `fresh` (empty messages) and `fork` (shallow copy of transcript)
- [ ] Context isolation: subagent modifications don't affect parent
- [ ] Synchronous execution with configurable timeout, context cancellation on expiry
- [ ] Result injection as LLM-visible message (default) or custom entry (opt-out)
- [ ] Error isolation: subagent failures return `Success: false` with error, parent continues
- [ ] Optional event forwarding channel for streaming visibility
- [ ] 5 built-in subagent types defined with correct default tool sets
- [ ] `SubAgent` uses `types.Model` (re-exported from `provider.Model`) — no direct import of `provider/`
- [ ] Unit tests for context cloning, timeout, result injection, error isolation (use `testutil/` helpers)
- [ ] No internal dependencies except `types` and `testutil`

---

### Task 011: Session Management

**Scope**: JSONL storage, session lifecycle, auto-naming, compaction.

**Packages**:
- `internal/session/session.go` — Session lifecycle (create, resume, persist)
- `internal/session/storage.go` — JSONL read/write, session header, entry types
- `internal/session/compaction.go` — Compaction logic (pure functions)
- `internal/session/naming.go` — Auto-naming strategy

**Key Decisions Applied**:
- Decision #1: JSONL session storage
- Decision #7: Per-type token estimation heuristics (text=chars/4, tool=chars/3, thinking=chars/3.5)
- Decision #13: CWD encoding via `/` → `-`
- Decision #14: Auto-naming from first user message
- Session directory: `~/.tau/sessions/<encoded-cwd>/`
- File naming: `<timestamp>_<8-char-hex-id>.jsonl`

**Dependencies**: `internal/types/` (Task 006)

**Acceptance Criteria**:
- [ ] JSONL append-only storage with session header (version 1)
- [ ] All 7 entry types supported (message, model_change, thinking_level_change, compaction, custom_entry, custom_message, session_info)
- [ ] Session resume algorithm rebuilds message list from JSONL (ARCHITECTURE.md §7.2a)
- [ ] Corruption recovery: incomplete last line discarded, valid entries preserved
- [ ] Auto-naming from first user message (truncated to 50 chars, special characters stripped)
- [ ] Compaction logic as pure functions: token estimation, cut point finding, turn boundary constraints
- [ ] Per-type token estimation: text/4, tool/3, thinking/3.5
- [ ] Compaction constrained to turn boundaries (never split tool call from result)
- [ ] Structured summary format in compaction entries
- [ ] Compaction does NOT call provider — summarization trigger/integration belongs to SDK (013)
- [ ] Session file encoding: `<timestamp>_<8-char-hex>.jsonl`
- [ ] Cumulative usage accumulator: session tracks total tokens and cost across all turns
- [ ] Unit tests for storage, resume, naming, compaction (use `testutil/` helpers)
- [ ] No internal dependencies except `types` and `testutil`

---

### Task 012: Agent Loop

**Scope**: Agent struct, core loop, state machine, event system, steering queues, tool orchestration.

**Packages**:
- `internal/agent/agent.go` — `Agent` struct (transcript, tools, model, hooks, queues)
- `internal/agent/loop.go` — `agentLoop()` — the core loop
- `internal/agent/event.go` — `AgentEvent` types, event subscription

**Key Decisions Applied**:
- Decision #15: No orchestrator package — agent loop IS the orchestrator
- Decision #4: Go channels for streaming
- Decision #5: No extension system in MVP
- Steering queue: buffered channel, checked after each turn_end
- Follow-up queue: buffered channel, checked when agent would stop
- Before/after tool call hooks
- Context files discovery (AGENTS.md/CLAUDE.md)

**Dependencies**: `internal/types/`, `internal/provider/`, `internal/tools/`, `internal/subagent/` (Tasks 006, 007, 008, 010)

**Acceptance Criteria**:
- [ ] `Agent` struct matches ARCHITECTURE.md §3.2 (includes `Abort()` method via `context.CancelFunc`)
- [ ] State machine implements all transitions: IDLE → STREAMING → TURN_END → EXECUTING_TOOLS → DONE
- [ ] Tool execution orchestration respects ExecutionMode (parallel, sequential, exclusive)
- [ ] Subagent spawn orchestration: when assistant requests subagent, loop calls `SubAgent.Run()`, waits with timeout, injects result into transcript
- [ ] Subagent abort: parent `Abort()` cancels running subagent via context
- [ ] Steering queue delivers messages after tool call batch, before next LLM call
- [ ] Follow-up queue delivers messages only when agent would stop
- [ ] Before/after tool call hooks called at correct points
- [ ] Event subscription: listeners receive all agent events
- [ ] Context files loaded (using search list from config) and prepended to system prompt
- [ ] Abort from any state via `Abort()` method: cancels provider, discards partial results, preserves session state
- [ ] Unit tests with mocked provider and tools (use `testutil/` helpers)
- [ ] Integration test: full loop with mock provider exercising all state transitions
- [ ] Internal dependencies only on `types`, `provider`, `tools`, `subagent`

---

### Task 013: SDK Integration

**Scope**: High-level Session API that composes all subsystems.

**Packages**:
- `internal/sdk/sdk.go` — `Session` struct, `CreateSession()`, `Prompt()`, `Continue()`, `Steer()`, `Subscribe()`, `Compact()`, `Usage()`, `Model()`, `SetModel()`

**Key Decisions Applied**:
- SDK Session is analogous to PI's `AgentSession`
- Composes agent loop, session persistence, skills, subagents
- `SessionOptions` with model, working dir, session path, continue, ephemeral, tool allowlist, read-only
- Session is the primary programmatic interface

**Dependencies**: All internal packages (Tasks 006–012)

**Acceptance Criteria**:
- [ ] `CreateSession()` initializes all subsystems (agent, session, skills, provider)
- [ ] `Prompt()` sends message, runs agent loop, returns when done
- [ ] `Continue()` resumes agent loop without new message
- [ ] `Steer()` delivers message to running agent via steering queue
- [ ] `Subscribe()` registers event listener, returns unsubscribe function
- [ ] `Compact()` triggers compaction: detects overflow, calls provider for LLM summarization, writes compaction entry
- [ ] `Usage()` returns cumulative session usage and cost (accumulated across all turns)
- [ ] `Model()` / `SetModel()` query and change active model
- [ ] Model resolution: pattern matching (exact ID → substring → default → interactive prompt)
- [ ] SessionOptions respected: ephemeral, read-only, tool allowlist, continue
- [ ] Error handling: all methods return errors gracefully — provider failures (with retry), tool failures (as results), context overflow (triggers compaction), subagent failures (error isolation)
- [ ] Skills discovered and formatted for progressive disclosure at session start
- [ ] Subagent spawning available through session context
- [ ] Cumulative usage accumulator: aggregates `Usage` from every turn's `StreamEvent{Type: "done"}`
- [ ] Tool allowlist contract: accepts `[]string` from `SessionOptions`, applies via `tools.Registry.WithAllowlist()`
- [ ] Read-only contract: accepts `bool` from `SessionOptions`, applies via `tools.Registry.WithReadOnly()`
- [ ] Unit tests with mocked subsystems (use `testutil/` helpers)
- [ ] Internal dependencies on all packages (as designed)

---

### Task 014: CLI

**Scope**: CLI entry point, user interaction modes, session management flags, model selection.

**Packages**:
- `cmd/tau/main.go` — CLI entry point, flag parsing
- `cmd/tau/interactive.go` — Interactive chat mode (default)
- `cmd/tau/print.go` — Print mode (`-p` / `--print`)
- `cmd/tau/json.go` — JSON output mode (`--mode json`)
- `cmd/tau/sessions.go` — Session management flags (`-c`, `-r`, `--session`, `--no-session`)

**Key Decisions Applied**:
- CLI modes: interactive (default), print (`-p`), JSON (`--mode json`)
- Session flags: `--continue`, `--resume`, `--session`, `--no-session`
- Auth override: `--api-key`
- Tool restriction: `--tools`, `--read-only`
- Context files: `--no-context-files`
- Model selection: pattern matching, interactive selection on ambiguity
- Print mode: streaming to stdout, tool calls shown as `🔧 tool_name(args)`
- JSON mode: one JSON object per line (JSONL events)

**Dependencies**: `internal/sdk/` (Task 013)

**Acceptance Criteria**:
- [ ] Interactive mode: readline input, streaming output, Ctrl+C abort
- [ ] Print mode (`-p`): single prompt → output → exit, supports stdin piping
- [ ] JSON mode (`--mode json`): structured JSONL events to stdout
- [ ] Session management: new, continue (`-c`), resume (`-r`), specific (`--session`), ephemeral (`--no-session`)
- [ ] Model selection: pattern matching, interactive selection on multiple matches (delegates to SDK model resolution)
- [ ] Auth override via `--api-key` flag (highest priority in auth chain)
- [ ] Tool allowlisting via `--tools` flag — passed to SDK `SessionOptions.ToolAllowlist`
- [ ] Read-only mode via `--read-only` flag — passed to SDK `SessionOptions.ReadOnly`
- [ ] Context files override via `--no-context-files` flag
- [ ] Skill invocation: `/skill:name` command loads full skill content into active session
- [ ] Graceful error handling: user-friendly messages, non-zero exit on error
- [ ] Unit tests for flag parsing, mode routing (use `testutil/` helpers)
- [ ] Manual end-to-end test: interactive session with mock provider (reuse mock from `testutil/`)

---

## Requirements Traceability Matrix

| Requirement | REQUIREMENTS.md § | Covered By Task |
|-------------|-------------------|-----------------|
| Orchestrator model | 3.1 | 012 (agent loop IS orchestrator) |
| Built-in skills: skill-builder | 3.2 | 009 |
| Built-in skills: subagent-builder | 3.2 | 009 |
| Subagent: Researcher | 3.3 | 010 |
| Subagent: Reviewer | 3.3 | 010 |
| Subagent: Implementor | 3.3 | 010 |
| Subagent: Security Reviewer | 3.3 | 010 |
| Subagent: QA | 3.3 | 010 |
| Core tools: read, write, edit, bash, grep, find, ls | 3.4 | 008 |
| Subagent definition format compatibility | 4 | 010 |
| Context model (fresh default) | 4 | 010 |
| Parent ↔ child only communication | 4 | 010 |
| Skills: Agent Skills standard | 5 | 009 |
| Skills: 3-tier discovery | 5 | 009 |
| Skills: `/skill:name` command | 5 | 014 (CLI) + 009 (discovery) |
| Provider: OpenAI | 6 | 007 |
| Provider: Anthropic | 6 | 007 |
| Provider: Google Gemini | 6 | 007 |
| Provider: OpenCode Zen | 6 | 007 (OpenAI-compat) |
| Provider: OpenCode Go | 6 | 007 (OpenAI-compat) |
| Provider: OpenRouter | 6 | 007 (OpenAI-compat) |
| Provider: Local models | 6 | 007 (OpenAI-compat) |
| Session: persistence & resume | 7 | 011 |
| Session: auto-naming | 7 | 011 |
| Session: branching (deferred) | 7 | Post-MVP backlog |
| TUI / TUI chat interface (deferred) | 8 | Post-MVP backlog (ARCHITECTURE.md §11.1) |
| Language: Go | 9 | All tasks |
| Dependencies: minimal | 9 | All tasks (only yaml.v3 + jsonschema) |
| Performance: lightweight | 9 | Architecture decisions |

## Subtasks

- [x] **005.1** — Define Task 006: Foundation (types + config + infrastructure)
- [x] **005.2** — Define Task 007: Provider System
- [x] **005.3** — Define Task 008: Tool System
- [x] **005.4** — Define Task 009: Skills System
- [x] **005.5** — Define Task 010: Subagent System
- [x] **005.6** — Define Task 011: Session Management
- [x] **005.7** — Define Task 012: Agent Loop
- [x] **005.8** — Define Task 013: SDK Integration
- [x] **005.9** — Define Task 014: CLI
- [x] **005.10** — Requirements traceability validation
- [x] **005.11** — Subagent critical review of task breakdown (delegate agent review completed, findings incorporated)

## Acceptance Criteria

- [ ] Each task (006–014) has clear scope and package boundaries
- [ ] Dependencies between tasks are documented and acyclic
- [ ] Each task has verifiable acceptance criteria
- [ ] Critical path identified (006 → 007+008 → 012 → 013 → 014)
- [ ] Parallel execution opportunities identified (009, 010, 011 after 006)
- [ ] All ARCHITECTURE.md components are covered by at least one task
- [ ] All REQUIREMENTS.md items are traced to implementation tasks
- [ ] All DECISIONS.md items are mapped to their implementing task
- [ ] Task breakdown reviewed and challenged by subagent (delegate agent)
- [ ] No gaps between architecture and implementation plan

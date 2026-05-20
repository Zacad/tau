# Subagent System — Comparative Analysis

## Overview

This document analyzes how OpenCode and PI implement sub-agents, extracts patterns, and documents pros/cons/tradeoffs to inform Tau's subagent system design.

---

## 1. OpenCode: Native Session-Based Sub-Agents

### Architecture

OpenCode implements sub-agents as **first-class native citizens** in the core. Sub-agents are defined in code and configurable via `opencode.json` or markdown files in `.opencode/agents/`.

### Key Design Decisions

| Aspect | Implementation |
|--------|---------------|
| **Definition** | `Info` schema with `mode: "subagent" | "primary" | "all"` |
| **Built-in agents** | 3: `general`, `explore`, `scout` |
| **Spawning** | Via `task` tool — LLM decides which sub-agent to invoke |
| **Session model** | Parent-child session hierarchy (`parentID` field) |
| **Context** | Fresh context by default — parent must provide detailed task description |
| **Communication** | `TaskPromptOps` interface: cancel, resolvePromptParts, prompt |
| **Result format** | XML-wrapped text: `<task_result>...</task_result>` with `task_id` for resumption |
| **Permissions** | Derived from parent: inherits deny rules (edit-class + session deny rules) |
| **Tool access** | Sub-agents get own permission ruleset + inherited parent denies (hard ceiling) |
| **Nested spawning** | `task` tool disabled in sub-agents by default unless explicitly permitted |
| **Execution** | Synchronous — parent waits via `Effect.forkChild()` |
| **UI** | TUI: subagent footer with sibling navigation; Run mode: tab-based view |

### Code References

- Agent definitions: `packages/opencode/src/agent/agent.ts:28-231`
- Task tool (spawning): `packages/opencode/src/tool/task.ts:32-174`
- Permission derivation: `packages/opencode/src/agent/subagent-permissions.ts:1-34`
- Session hierarchy: `packages/opencode/src/session/session.ts:213, 581-675`
- Subtask handling: `packages/opencode/src/session/prompt.ts:700-891`
- State management: `packages/opencode/src/cli/cmd/run/subagent-data.ts:12-825`

### Pros

1. **Native integration** — no extension overhead, sub-agents are first-class
2. **Permission inheritance** — security model propagates parent restrictions automatically
3. **Session hierarchy** — parent-child relationship enables navigation, inspection, resumption
4. **Event streaming** — real-time visibility into sub-agent progress
5. **Resumable** — `task_id` allows resuming a sub-agent session later
6. **LLM-driven spawning** — task tool description dynamically lists available sub-agents
7. **Hidden agents** — can define agents only invocable by other agents (not by user)

### Cons

1. **Complexity** — session hierarchy, permission derivation, event reduction add significant code
2. **Fresh context only** — no fork mode; parent must manually provide all context in task text
3. **No parallel/chain modes** — each sub-agent is spawned individually
4. **TypeScript/Effect dependency** — relies on Effect.ts for async orchestration
5. **Result limited to text** — no structured artifact tracking beyond text output

---

## 2. PI: Extension-Based Process-Spawned Sub-Agents

### Architecture

PI deliberately **does not include sub-agents in core**. Instead, sub-agents are implemented as an **example extension** (`examples/extensions/subagent/`) that spawns separate `pi` processes.

### Key Design Decisions

| Aspect | Implementation |
|--------|---------------|
| **Definition** | Markdown files with YAML frontmatter (`name`, `description`, `tools`, `model`) |
| **Built-in agents** | 4 example agents: `scout`, `planner`, `reviewer`, `worker` |
| **Spawning** | Via `subagent` tool (extension-registered) — LLM decides mode |
| **Process model** | Separate `pi` subprocess per sub-agent |
| **Context** | Always fresh — `--no-session` flag, no history sharing |
| **Communication** | JSON event stream from child stdout → parent parses line-by-line |
| **Result format** | `AgentToolResult` with text content + structured details |
| **Permissions** | Tool restriction via `--tools` CLI flag (comma-separated allowlist) |
| **Execution modes** | Single, Parallel (max 8, concurrency 4), Chain (sequential with `{previous}` substitution) |
| **Agent discovery** | User-level (`~/.pi/agent/agents/`) + Project-level (`.pi/agents/`) with override support |
| **UI** | Collapsed/expanded views with TUI components, status icons, usage stats |

### Code References

- Main extension: `packages/coding-agent/examples/extensions/subagent/index.ts:987 lines`
- Agent discovery: `packages/coding-agent/examples/extensions/subagent/agents.ts:126 lines`
- Agent definitions: `examples/extensions/subagent/agents/*.md`
- Workflow prompts: `examples/extensions/subagent/prompts/*.md`
- Documentation: `examples/extensions/subagent/README.md`

### Pros

1. **Complete isolation** — separate process = separate memory, separate context window
2. **Flexible execution modes** — single, parallel, chain out of the box
3. **Markdown-defined agents** — easy to create/edit without code changes
4. **Tool restriction** — simple `--tools` CLI flag for permission control
5. **Project agents** — repo-local agent definitions with override support
6. **Model per agent** — each sub-agent can use a different model
7. **Live streaming** — `onUpdate` callback for real-time progress
8. **No core complexity** — keeps PI core minimal, delegates to extension

### Cons

1. **Process overhead** — spawning subprocesses is slower than goroutines
2. **No context sharing** — always fresh, no fork mode
3. **No session persistence** — `--no-session` means no resumption
4. **No permission inheritance** — tool restriction is allowlist-only, no deny rules
5. **Extension dependency** — requires extension system to work
6. **JSON parsing fragility** — line-by-line JSON parsing can break on malformed output
7. **No nested spawning** — sub-agents cannot spawn their own sub-agents
8. **Manual context passing** — `{previous}` placeholder in chain mode is fragile

---

## 3. Pattern Comparison

| Dimension | OpenCode | PI | Tau (proposed) |
|-----------|----------|----|----------------|
| **Integration** | Native core | Extension | Native core |
| **Execution** | Same-process (Effect) | Subprocess | Same-process (goroutine) |
| **Context** | Fresh only | Fresh only | Fresh + Fork |
| **Permissions** | Inherited denies + own ruleset | CLI allowlist only | Inherited + per-agent |
| **Spawning** | Task tool (LLM-driven) | Subagent tool (LLM-driven) | Tool + SDK API |
| **Modes** | Single only | Single, Parallel, Chain | Single → Parallel → Chain |
| **Definition** | Code + JSON/MD config | Markdown + frontmatter | Markdown + frontmatter |
| **Discovery** | Built-in + config files | User + project dirs | User + project dirs |
| **Result** | XML text + task_id | Structured AgentToolResult | Structured SubAgentResult |
| **Session** | Parent-child hierarchy | Ephemeral | Parent-child (future) |
| **Streaming** | Event forwarding | JSON line parsing | Channel-based events |
| **Resumption** | Yes (task_id) | No | Future (session ID) |

---

## 4. Tradeoffs for Tau (Go Implementation)

### 4.1 Execution Model: Goroutines vs Subprocesses

**Goroutines (recommended for Tau):**
- Pros: Fast startup, shared memory, no IPC overhead, easy cancellation via context
- Cons: Less isolation, shared process limits, harder to debug

**Subprocesses (PI approach):**
- Pros: Complete isolation, independent context windows, crash-safe
- Cons: Slow startup, IPC complexity, binary must be on PATH, harder to test

**Decision**: Use goroutines for MVP. Tau is a single-user tool — process isolation is less critical. Subprocess spawning can be added later if needed.

### 4.2 Context Model: Fresh vs Fork

**Fresh (both OpenCode and PI default):**
- Pros: Clean context window, no token waste, no parent context pollution
- Cons: Must manually pass all relevant context in task description

**Fork (Tau-specific addition):**
- Pros: Sub-agent sees parent's conversation history, better for code changes
- Cons: Token cost, potential context pollution

**Decision**: Support both. Fresh for research tasks, fork for implementation tasks. Shallow copy only (matching existing architecture).

### 4.3 Permission Model

**Inherited denies (OpenCode approach):**
- Pros: Security ceiling — sub-agents can never exceed parent permissions
- Cons: More complex to implement

**Allowlist only (PI approach):**
- Pros: Simple to implement and understand
- Cons: No protection against permission escalation

**Decision**: Hybrid. Sub-agents have their own tool set (allowlist), but cannot exceed parent's available tools (hard ceiling).

### 4.4 Definition Format

**Code-defined (OpenCode built-ins):**
- Pros: Type-safe, validated at compile time
- Cons: Requires code changes to add new agents

**Markdown + frontmatter (PI approach):**
- Pros: Easy to create/edit, no code changes, human-readable
- Cons: Parsing overhead, less type safety

**Decision**: Support both. Built-in agents defined in code (type-safe). User-defined agents via Markdown + frontmatter (flexible).

### 4.5 Execution Modes

**Single only (OpenCode):**
- Pros: Simple to implement
- Cons: No parallelism, no workflows

**Single + Parallel + Chain (PI):**
- Pros: Powerful workflow patterns
- Cons: More complexity

**Decision**: Vertical slices — start with single, add parallel, then chain. Each in separate sessions.

---

## 5. Key Learnings from OpenCode and PI

### From OpenCode
1. **Permission inheritance is critical** — sub-agents must not exceed parent capabilities
2. **Session hierarchy enables navigation** — parent-child relationship is valuable for UX
3. **Hidden agents are useful** — agents only invocable by other agents
4. **LLM-driven spawning works well** — dynamic tool description listing available sub-agents
5. **Fresh context by default** is the right choice — avoid context pollution

### From PI
1. **Markdown-defined agents** are flexible and user-friendly
2. **Execution modes** (single/parallel/chain) are powerful workflow patterns
3. **Process isolation** provides clean separation but adds overhead
4. **Project agents with override** enable repo-specific workflows
5. **Model per agent** allows cost optimization (cheap model for scout, expensive for worker)
6. **Live streaming updates** are important for UX — users want to see progress

### What to Avoid
1. **Don't over-engineer permissions** — Tau is single-user, keep it simple
2. **Don't build session hierarchy yet** — flat is fine for MVP
3. **Don't implement all execution modes at once** — vertical slices
4. **Don't copy OpenCode's Effect.ts complexity** — Go's goroutines are simpler
5. **Don't copy PI's JSON line parsing** — channel-based is more idiomatic in Go

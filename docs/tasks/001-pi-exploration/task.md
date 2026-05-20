# Task 001: PI Exploration

## Why

Before building our own Go-based agentic tau, we must deeply understand PI's architecture, design decisions, strengths, and weaknesses. PI is the reference implementation we're inspired by — we need to know what to borrow, what to improve, and what to do differently in Go.

This exploration will inform all subsequent tasks: requirements, architecture, and task breakdown.

## Comparison Analysis: What PI Does vs What We Need

| Area | PI Approach | Our Constraint |
|------|-------------|----------------|
| Language | TypeScript/Node.js | Go — lightweight, compiled, single binary |
| Architecture | 3 npm packages (pi-ai, pi-agent-core, pi-tui) | Single binary, no runtime deps |
| Extensibility | Extension system, skills, themes, packages | Minimal built-in feature set, no extensibility layer initially |
| Sub-agents | Deliberately absent; via extensions | Must support out of the box |
| Skills | Agent Skills standard, on-demand | Must support out of the box |
| Providers | 20+ via built-in + custom models | Core 8: OpenAI, Anthropic, Gemini, OpenCode Zen, OpenCode Go, OpenRouter, local models |
| Session Management | JSONL with tree structure, branching, fork, clone | Need persistent sessions, but scope TBD |
| Modes | Interactive (TUI), Print, RPC | TBD — likely need at least interactive and programmatic |
| UI | Rich TUI with Ink/React | Minimal TUI — performance over richness |
| Compaction | Built-in, LLM-based summarization | TBD |
| Auth | API keys + OAuth subscriptions | API keys sufficient initially |

## Main Constraints

- Exploration only — no code changes to tau project
- Must be thorough enough to inform architecture decisions
- Must identify PI's internal layering and data flow
- Must surface patterns worth adopting and anti-patterns to avoid

## Subtasks

- [x] **001.1** — Map PI's package architecture (pi-ai, pi-agent-core, pi-tui, main agent)
- [x] **001.2** — Analyze provider/model system: how providers are defined, authenticated, selected
- [x] **001.3** — Analyze session management: JSONL format, tree structure, branching, compaction
- [x] **001.4** — Analyze tool system: built-in tools, tool execution, streaming results
- [x] **001.5** — Analyze agent loop: prompt → LLM → tool calls → results → repeat
- [x] **001.6** — Analyze RPC protocol: command/event framing, extension UI sub-protocol
- [x] **001.7** — Analyze TUI architecture: editor, message display, commands, shortcuts
- [x] **001.8** — Document learnings: what to adopt, what to improve, what to avoid
- [x] **001.9** — Identify gaps: what PI doesn't do that we need (sub-agents, skills orchestration)

## Acceptance Criteria

- [ ] All subtasks completed with documented findings in worklog
- [ ] Clear understanding of PI's architecture layers and their responsibilities
- [ ] Documented list of patterns to adopt from PI
- [ ] Documented list of design decisions to diverge from PI
- [ ] Documented list of features PI lacks that we need
- [ ] Findings comprehensive enough to inform Task 002 (Requirements) and Task 004 (Architecture)

## Subtask Acceptance Criteria

### 001.1 — Package Architecture
- [ ] Each PI package's responsibility documented
- [ ] Dependencies between packages mapped
- [ ] Key types/interfaces identified per package

### 001.2 — Provider/Model System
- [ ] Provider registration and discovery mechanism documented
- [ ] Model selection and cycling logic documented
- [ ] Auth resolution order documented
- [ ] Custom provider extension mechanism documented

### 001.3 — Session Management
- [ ] JSONL session format documented
- [ ] Tree structure and branching mechanics documented
- [ ] Compaction trigger and process documented
- [ ] Session persistence and recovery documented

### 001.4 — Tool System
- [ ] Tool definition interface documented
- [ ] Built-in tools cataloged
- [ ] Tool execution flow (call → execute → result → next turn) documented
- [ ] Streaming tool output mechanism documented

### 001.5 — Agent Loop
- [ ] Core agent loop steps documented
- [ ] Message transformation before LLM call documented
- [ ] Turn lifecycle (start → response → tool calls → results → end) documented
- [ ] Error handling and retry mechanism documented

### 001.6 — RPC Protocol
- [ ] Command types and parameters documented
- [ ] Event types and payloads documented
- [ ] Extension UI sub-protocol documented
- [ ] Framing and transport details documented

### 001.7 — TUI Architecture
- [ ] TUI component structure documented
- [ ] Editor integration documented
- [ ] Command system documented
- [ ] Event-driven UI updates documented

### 001.8 — Learnings Document
- [ ] Patterns to adopt listed with rationale
- [ ] Design decisions to diverge listed with rationale
- [ ] Go-specific considerations identified

### 001.9 — Gap Analysis
- [ ] Features PI lacks but we need listed
- [ ] Sub-agent approaches analyzed (what PI extensions do, what we could do in Go)
- [ ] Skills orchestration requirements identified

## Worklog

See `worklog.md` for detailed work documentation.

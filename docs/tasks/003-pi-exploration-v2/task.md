# Task 003: PI Exploration v2

## Why

Task 001 was a broad exploration of PI's architecture. Task 002 defined our specific requirements. Now we need a targeted, requirements-driven re-exploration of PI — examining the exact subsystems we plan to build, understanding their implementation details at a deeper level, and identifying concrete patterns (or anti-patterns) that will directly inform our Go architecture in Task 004.

This is not a repeat of Task 001. It is a focused investigation of PI's internals for the specific features we committed to building: sub-agents, skills, provider abstraction, session management, tool execution, and the agent loop.

## Comparison Analysis: PI Implementation vs Our Requirements

| Requirement | PI Has It? | Gap | Action |
|-------------|-----------|-----|--------|
| Sub-agents out of the box | No (extension-based) | Must build natively | Study how extensions implement sub-agents, then design Go-native version |
| Built-in skills (skill-builder, subagent-builder) | Partial (skill discovery exists) | Must add creation skills | Study PI's skill loading, extend for creation workflows |
| Agent as orchestrator | Partial (agent loop exists) | Need structured + free-form | Deep-dive into PI's agent loop, identify orchestration hooks |
| 8 provider types | Yes (20+ supported) | Subset of PI's capability | Map PI provider interfaces to our 8 target providers |
| Persistent sessions | Yes (JSONL) | Need resume across restarts | Study PI's JSONL format, compaction, tree structure |
| TUI (deferred) | Yes (rich Ink/React) | Minimal in scope | Identify minimal subset for our needs |
| Minimal dependencies | N/A | PI has heavy deps (Node.js) | Note every external dep PI uses — avoid in Go |
| Go single binary | N/A | Different runtime model | Map PI's modular architecture to Go package structure |

## Main Constraints

- Exploration only — no code changes to tau project
- Must be driven by REQUIREMENTS.md — each investigation area maps to a specific requirement
- Must produce actionable findings for Task 004 (Architecture Design)
- Must identify exactly which PI patterns to adopt, adapt, or avoid
- Must surface concrete Go implementation implications (package boundaries, interfaces, data structures)

## Subtasks

- [x] **003.1** — Deep-dive: PI sub-agent patterns (how extensions implement sub-agents, context management, communication)
- [x] **003.2** — Deep-dive: PI skill system (loading, discovery, progressive disclosure, Agent Skills standard compliance)
- [x] **003.3** — Deep-dive: PI agent loop (message transformation, tool call handling, turn lifecycle, error recovery, compaction triggers)
- [x] **003.4** — Deep-dive: PI provider abstraction (ModelProvider interface, auth resolution, model selection, streaming)
- [x] **003.5** — Deep-dive: PI session storage (JSONL format, tree structure, compaction, persistence model)
- [x] **003.6** — Deep-dive: PI tool system (tool definition, execution model, streaming output, built-in tools)
- [x] **003.7** — Map PI architecture to Go package structure (what goes where, interface boundaries, dependency graph)
- [x] **003.8** — Document findings: patterns to adopt, adapt, avoid — with Go-specific implications

## Acceptance Criteria

- [x] All subtasks completed with documented findings in worklog
- [x] Each requirement from REQUIREMENTS.md has a corresponding PI investigation finding
- [x] Clear Go package structure proposal derived from PI analysis
- [x] Specific interfaces identified for each subsystem (providers, tools, skills, sub-agents, sessions)
- [x] Documented list of PI dependencies to avoid
- [x] Findings specific enough to inform Task 004 (Architecture Design) without requiring another exploration pass
- [x] No significant PI internals relevant to our requirements remain unexamined

## Subtask Acceptance Criteria

### 003.1 — Sub-Agent Patterns
- [x] PI's extension-based sub-agent mechanism documented
- [x] Context model (fresh vs fork) mapped to PI session capabilities
- [x] Parent-child communication pattern analyzed
- [x] Result return mechanism (how results get back without polluting context) documented
- [x] Go-native sub-agent design implications identified

### 003.2 — Skill System
- [x] PI's skill discovery mechanism (global, project, built-in) fully mapped
- [x] Agent Skills standard (agentskills.io) compliance details documented
- [x] Progressive disclosure mechanism (how skills are shown in system prompt) analyzed
- [x] SKILL.md format parsing and validation documented
- [x] skill-builder and subagent-builder skill design implications identified
- [x] Cross-tool compatibility (PI, OpenCode, Claude Code skill formats) analyzed

### 003.3 — Agent Loop
- [x] Complete message transformation pipeline documented (user input → system prompt → tool messages → LLM)
- [x] Tool call execution and result aggregation detailed
- [x] Compaction trigger conditions and process documented
- [x] Error handling and retry strategy mapped
- [x] Orchestrator pattern implications (skill → subagent → skill workflows) identified

### 003.4 — Provider Abstraction
- [x] PI's ModelProvider interface and implementation pattern documented
- [x] Auth resolution order (env vars, config files, OAuth) mapped
- [x] Streaming response handling detailed
- [x] Model selection/cycling mechanism documented
- [x] Our 8 target providers mapped to PI interfaces
- [x] Local model support (Ollama, llama.cpp, LM Studio) integration points identified

### 003.5 — Session Storage
- [x] Complete JSONL message format documented (all message types, roles, metadata)
- [x] Tree/branching structure implementation details documented
- [x] Compaction process (trigger, summarization, replacement) fully mapped
- [x] Session file naming, discovery, and persistence model documented
- [x] Auto-naming mechanism analyzed
- [x] Minimal viable session storage design for MVP identified (branching deferred)

### 003.6 — Tool System
- [x] PI's tool definition interface and registration mechanism documented
- [x] Each built-in tool's implementation approach analyzed (read, write, bash, grep, git, code execution)
- [x] Tool execution lifecycle (call → execute → result → next turn) detailed
- [x] Streaming tool output mechanism documented
- [x] Security considerations (sandboxing, permissions) noted

### 003.7 — Go Package Structure Mapping
- [x] Proposed Go package layout with responsibilities
- [x] Interface boundaries between packages defined
- [x] Dependency graph (which packages depend on which) mapped
- [x] External dependency analysis (what Go libraries we'll need vs can avoid)
- [x] Single binary implications noted (embedding, static compilation)

### 003.8 — Findings Document
- [x] Patterns to adopt from PI — with Go-specific implementation notes
- [x] Patterns to adapt from PI — with rationale for changes
- [x] Patterns to avoid from PI — with alternative Go approaches
- [x] External dependency minimization strategy documented
- [x] Key architecture risks identified

## Worklog

See `worklog.md` for detailed work documentation.

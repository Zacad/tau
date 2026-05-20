# Task 004: Architecture Design

## Why

Requirements are defined (Task 002) and PI internals are understood (Tasks 001 & 003). Now we need a concrete Go architecture that bridges what the user needs with how PI actually works under the hood. This architecture will directly drive Task 005 (Task Breakdown) and all subsequent implementation work.

## Comparison Analysis: Architecture Approach vs PI's Architecture

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Architecture | Extension-based, plugin system | Built-in capabilities, minimal extension surface |
| Modularity | Extension SDK, custom tools, themes | Simple Go packages, no plugin SDK |
| Skill System | Full Agent Skills standard with discovery | Simplified discovery, Agent Skills compatible |
| Subagent System | Built-in with intercom, chain, parallel | Core orchestrator with skill/subagent coordination |
| Provider Support | Provider abstractions, model registry | Multiple providers with minimal abstraction layer |
| Session Management | Complex session model with branching | Simple persistence with resume capability |
| Configuration | Complex config system with themes | Minimal configuration, convention over configuration |

## Main Constraints

- Go language only, idiomatic approach
- Minimal dependencies - prefer stdlib where possible
- Single binary distribution
- Must satisfy all REQUIREMENTS.md specifications
- Architecture must be extensible but not over-engineered
- Design for single user, personal tool
- No extension system in MVP
- No multi-user support
- No web UI in MVP

## Inputs from Task 003 (PI Exploration v2)

Task 003 produced verified findings from PI's source code. All architecture decisions MUST reference these insights. The key findings are organized by subsystem below — each one should directly inform the corresponding architecture subtask.

### 003.1 — Sub-Agent Patterns (→ subtask 004.5)
- PI has NO built-in sub-agent support — it's extension-based
- Session tree supports branching via `branch()`, `branchWithSummary()`, `createBranchedSession()`
- `CustomEntry` for non-LLM data, `CustomMessageEntry` for LLM-visible data
- **Design implication**: Build sub-agents as first-class citizens, use goroutines + channels, context isolation via message slice cloning, parent-child only communication

### 003.2 — Skill System (→ subtask 004.4)
- Discovery paths: `~/.pi/agent/skills/` (global), `.agents/skills/` (project), `--skill` CLI flag
- Agent Skills standard: SKILL.md with YAML frontmatter + markdown body
- Validation rules: name must match dir, max 64 chars, lowercase/hyphens only
- `formatSkillsForPrompt()` — progressive disclosure, only name+description shown
- **Design implication**: Parse YAML frontmatter + markdown, 3-tier discovery, `/skill:name` command, built-in skills: `skill-builder`, `subagent-builder`

### 003.3 — Agent Loop (→ subtask 004.3)
- Two-level loop: outer (follow-ups), inner (tool calls + steering)
- Events: `agent_start/end`, `turn_start/end`, `message_start/update/end`, `tool_execution_start/update/end`
- Hooks: `beforeToolCall`, `afterToolCall` — critical for orchestrator pattern
- Steering/follow-up message queues for interaction with running agent
- **Design implication**: Event-driven loop using Go channels, `Agent` struct owns transcript + tools + model, hooks as function fields, steering via buffered channels

### 003.4 — Provider Abstraction (→ subtask 004.6)
- 25 built-in providers across 9 API types (openai-completions, openai-responses, anthropic-messages, etc.)
- `Model` struct: id, name, api, provider, baseUrl, reasoning, input types, cost, contextWindow, maxTokens
- Auth resolution: env vars → config file → OAuth (`resolveConfigValueOrThrow()`)
- OpenAI compatibility layer essential for our target providers
- **Design implication**: `Provider` interface with `Stream() <-chan Event`, `Model` registry, auth chain (env → config → OAuth), compatibility layer for OpenAI-compatible providers

### 003.5 — Session Storage (→ subtask 004.7, 004.9)
- JSONL format with session header (version 3) + typed entries (9 entry types)
- Tree structure: each entry has `id` + `parentId`, `leafId` tracks current position
- `buildSessionContext()` walks leaf→root, handles compaction + branch summaries
- Compaction: trigger when context > (window - reserveTokens), `chars/4` token heuristic
- Session dir: `~/.pi/agent/sessions/<encoded-cwd>/`
- **Design implication**: JSONL append-only, tree via id/parentId, compaction as pure function, token estimation via chars/4, session dir `~/.tau/sessions/`

### 003.6 — Tool System (→ subtask 004.9)
- `AgentTool` interface: Name, Description, Parameters (TypeBox), Execute, executionMode
- 7 built-in tools: read, bash, edit, write, grep, find, ls
- Parallel execution via `Promise.all`, sequential preflight
- File mutation queue: per-file promise chain to serialize writes
- Tool result `terminate` flag to signal "stop after this batch"
- **Design implication**: Go struct-based tool interface, `errgroup` for parallel execution, mutex-per-file for write serialization, truncation utilities

### 003.7 — Go Package Structure (→ subtask 004.2)
- Proposed structure: `internal/agent/`, `internal/session/`, `internal/tools/`, `internal/provider/`, `internal/skills/`, `internal/subagent/`, `internal/config/`, `internal/sdk/`
- Dependency graph: acyclic, tools+provider are leaf packages, sdk depends on all
- External deps: only `gopkg.in/yaml.v3` for skill frontmatter
- Key interfaces: `Provider`, `Tool`, `Skill`, `SubAgent`, `SessionStorage`
- **Design implication**: Use proposed structure as starting point, validate during architecture review

### 003.8 — Patterns to Adopt/Adapt/Avoid (→ all subtasks)
- **Adopt**: JSONL sessions, event-driven loop, typed tool params, provider stream interface, skill discovery, compaction as pure function, progressive disclosure, auth chain, steering queues, before/after hooks, tree sessions, terminate flag
- **Adapt**: Extension system → built-in sub-agents, TypeBox → Go structs, EventBus → channels, tree/branching → simple append-only (MVP), React/TUI → minimal CLI, Promise queue → sequential goroutine
- **Avoid**: Extension complexity (1168 lines), heavy deps, tree/branching (MVP), OAuth (MVP), multi-mode (RPC/print), TypeBox runtime validation, complex message transformation

## Subtasks

- [x] **004.1** — High-level system architecture and component diagram
- [x] **004.2** — Package structure and Go module organization
- [x] **004.3** — Core orchestrator design
- [x] **004.4** — Skills system architecture
- [x] **004.5** — Subagent system architecture  
- [x] **004.6** — Provider abstraction and model selection
- [x] **004.7** — Session management design
- [x] **004.8** — TUI architecture (deferred to implementation)
- [x] **004.9** — Data models and persistence layer
- [x] **004.10** — Security and error handling approach
- [x] **004.11** — Consolidate into ARCHITECTURE.md

## Acceptance Criteria

- [x] ARCHITECTURE.md contains complete architecture documentation
- [x] Each major component has clear responsibilities and interfaces
- [x] Package structure follows Go idiomatic conventions
- [x] Architecture addresses all REQUIREMENTS.md items
- [x] Dependencies are minimal and justified
- [x] Security considerations are documented
- [x] Architecture is reviewed and challenged by subagent
- [x] Design decisions are documented in DECISIONS.md
- [x] Architecture is specific enough to drive task breakdown (Task 005)

## Subtask Acceptance Criteria

### 004.1 — High-level System Architecture
- [x] Component diagram showing major system parts
- [x] Data flow between components documented
- [x] External dependencies identified
- [x] System boundaries defined

### 004.2 — Package Structure
- [x] Go package layout follows standard conventions
- [x] Package responsibilities are clear and non-overlapping
- [x] Import dependency graph is acyclic
- [x] Public APIs are minimal and well-defined

### 004.3 — Core Orchestrator Design
- [x] Orchestrator responsibilities clearly defined
- [x] Orchestration patterns (skill → subagent → skill) supported
- [x] Free-form orchestration supported
- [x] Context management strategy defined

### 004.4 — Skills System Architecture
- [x] Skill discovery mechanism (global, project, built-in)
- [x] Skill loading and progressive disclosure design
- [x] Skill-tool relationship defined
- [x] Agent Skills standard compatibility maintained

### 004.5 — Subagent System Architecture
- [x] Subagent lifecycle (create, execute, return results)
- [x] Context model (fresh vs fork)
- [x] Communication model (parent ↔ child only)
- [x] Result handling without context pollution

### 004.6 — Provider Abstraction
- [x] Provider interface design
- [x] Model registry and selection strategy
- [x] Authentication handling
- [x] Required providers supported (OpenAI, Anthropic, Gemini)

### 004.7 — Session Management
- [x] Session persistence format and location
- [x] Session lifecycle (create, resume, delete)
- [x] Auto-naming strategy
- [x] Resume across restarts supported

### 004.8 — TUI Architecture
- [x] **DEFERRED** - to be addressed during implementation phase

### 004.9 — Data Models and Persistence
- [x] Core data structures defined
- [x] Storage format (JSON, SQLite, etc.) justified
- [x] Migration strategy (if needed)
- [x] Backup/recovery considerations

### 004.10 — Security and Error Handling
- [x] API key security approach
- [x] File system access controls
- [x] Error propagation strategy
- [x] Graceful degradation patterns

### 004.11 — Consolidation
- [x] Architecture written to docs/ARCHITECTURE.md
- [x] Key decisions documented in DECISIONS.md
- [x] Architecture reviewed against REQUIREMENTS.md
- [x] Gaps and risks identified
- [x] Architecture ready for task breakdown

## Worklog

See `worklog.md` for detailed work documentation.
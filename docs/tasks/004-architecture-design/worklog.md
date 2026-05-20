# Worklog - Task 004: Architecture Design

## 2026-05-02

### Task Definition
- Created task.md with architecture design requirements
- Defined 11 subtasks covering all architectural aspects
- Established comparison with PI's architecture approach
- Set acceptance criteria for each subtask
- TUI architecture deferred to implementation phase
- Aligned with REQUIREMENTS.md specifications

### Architecture Research
- Read PI source code extensively: pi-ai types, pi-agent-core types, stream.d.ts, api-registry.d.ts, models.d.ts
- Verified agent loop, provider interface, session storage, tool system, skill discovery from source
- Read task 003 worklog for verified findings
- Compared PI patterns with known Claude Code / OpenCode approaches

### Architecture Writing (Iteration 1)
- Wrote comprehensive ARCHITECTURE.md covering all 10 active subtasks (004.1–004.11)
- Covered: system overview, package structure, orchestrator, skills, subagents, providers, sessions, data models, security, CLI, TUI (deferred), requirements traceability
- Updated DECISIONS.md with 7 key decisions

### Architecture Review (Delegate Agent)
- Launched delegate agent to critically review architecture
- Review produced 30 findings: 1 Critical, 9 High, 9 Medium, 11 Low
- Key findings: JSON Schema generation unspecified (Critical), auth chain underspecified, package dependency cycle, steering queues missing, compaction cut points vague, orchestrator-agent interface unclear, context files missing

### Architecture Rewrite (Iteration 2)
- Completely rewrote ARCHITECTURE.md addressing all Critical and High findings
- Added `internal/types/` package to eliminate import cycles
- Split Orchestrator into ContextManager, WorkflowEngine, AgentCoordinator
- Added steering/follow-up message queue design
- Added 4-step auth resolution chain (CLI flag → auth.json → env → config)
- Added `StreamOptions` struct with `ThinkingLevel` support
- Added `Usage` and `CostDollars` structs for cost tracking
- Added `BashExecution` struct for semantic bash tracking
- Added context files (AGENTS.md/CLAUDE.md) discovery design
- Added session resume algorithm and model resolution algorithm
- Added CLI modes (interactive, print, JSON) and session management flags
- Added tool allowlisting and read-only mode
- Added `github.com/invopop/jsonschema` as dependency for tool JSON Schema generation
- Added turn-aware compaction with structured summary format
- Added per-type token heuristics (text=chars/4, tool=chars/3, thinking=chars/3.5)
- Added subagent execution model (synchronous with timeout)
- Updated DECISIONS.md with 12 total decisions
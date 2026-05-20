# Worklog - Task 005: Task Breakdown

## 2026-05-02

### Task Definition
- Created task.md with implementation task breakdown
- Defined 9 implementation tasks (006–014) based on ARCHITECTURE.md package structure
- Each task maps to specific architecture sections with package-level boundaries
- Dependency graph derived from ARCHITECTURE.md import dependency graph
- Critical path identified: 006 → 007+008 → 012 → 013 → 014
- Parallel tracks identified: 009 (Skills), 010 (Subagent), 011 (Session) — all runnable after 006

### Requirements Traceability
- Mapped all REQUIREMENTS.md items to implementation tasks
- All core features (orchestrator, skills, subagents, tools, providers, sessions) covered
- Deferred items (TUI, branching sessions) placed in post-MVP backlog with correct section references

### Subagent Critical Review (delegate agent)
Launched delegate agent review of the initial task breakdown. Review produced:
- **2 Critical findings**
- **6 High findings**
- **8 Medium findings**
- **5 Low findings**

### Findings Incorporated

**Critical fixes:**
- C1: Added `internal/subagent/` as explicit dependency for Task 012 (Agent Loop). Subagent spawn orchestration now clearly owned by agent loop.
- C2: Added subagent integration acceptance criteria to Task 012 (spawn, wait with timeout, inject result, abort cancellation).

**High fixes:**
- H1: Task 007 kept as single task (splitting would change numbering). Noted internal sub-phases in task description.
- H2: Model resolution ownership clarified — SDK (013) owns pattern matching + resolution, CLI (014) delegates to SDK for interactive selection.
- H3: Compaction split — Task 011 owns pure functions only. Task 013 (SDK) owns LLM summarization integration.
- H4: Context files discovery split — Task 006 (config) computes search list, Task 012 (agent) loads and prepends to system prompt.
- H5: Tool allowlist contract documented — SDK (013) accepts from CLI, passes to tools registry via `WithAllowlist()` and `WithReadOnly()` APIs.
- H6: Task 006 expanded to include `go.mod`, dependency resolution, and `internal/testutil/` (shared mock infrastructure).

**Medium fixes:**
- M1: Added `/skill:name` command to Task 014 acceptance criteria.
- M2: `provider.Model` re-exported in `types/` — Task 010 no longer needs direct dependency on Task 007.
- M3: Auto-naming AC updated to include "special characters stripped".
- M5: Shared `internal/testutil/` package scoped in Task 006.
- M6: TUI traceability corrected to reference post-MVP backlog with ARCHITECTURE.md §11.1 reference.
- M7: Error handling acceptance criteria added to Task 013 (provider failures, tool failures, context overflow, subagent failures).
- M8: Cumulative usage accumulator explicitly assigned to Task 013 (SDK) and Task 011 (session-level tracking).

**Low notes:**
- L2: `Abort()` method will be added to Agent struct during Task 012 implementation.
- L3: Readline library will be added to ARCHITECTURE.md §1.5 during Task 014 implementation.
- L4: Resolved by re-exporting `provider.Model` through `types/` in Task 006.
- L5: File granularity in Task 014 left as-is — implementation discretion.

### Updated Dependency Graph
Task 012 now correctly depends on 006, 007, 008, and 010 (was missing 010).
Parallel tracks 009, 010, 011 remain valid — Task 010 uses `types.Model` (re-exported), not direct `provider/` import.

### Next Steps
- Awaiting user confirmation
- After confirmation: create individual task directories (006–014) with task.md files

### Second Round — Delegate Agent Review of All 9 Tasks (006–014)
Launched delegate agent review of the individual task files. Review produced **24 findings**:
- **3 Critical**, **8 High**, **8 Medium**, **4 Low**

### Second Round — Findings Incorporated

**Critical fixes:**
- **CR1**: `types.Model` vs `provider.Model` circular dependency — resolved by defining `Model` struct directly in `types/`. Provider imports `types.Model`. No circular dependency.
- **CR2**: Decision #16 reference doesn't exist — replaced with direct ARCHITECTURE.md §8.1a reference in Task 008.
- **CR3**: Readline dependency (3rd external dep) — added to Task 006.1 go.mod. Updated dependency count across all tasks.

**High fixes:**
- **H4**: StreamEvent defined in both 006 and 007 — clarified: type lives in `types/`, 007.5 handles streaming channel logic only.
- **H5**: Cumulative usage accumulator owned by both 011 and 013 — clarified: session/ (011) owns data structure, SDK (013) exposes via `Usage()`.
- **H6**: Tool orchestration boundary ambiguous — Task 008.1 now specifies `ExecuteBatch()` method; Task 012.4 specifies agent loop calls it.
- **H7**: Agent loop has no dependency on Skills — clarified: SDK composes system prompt, agent receives it pre-composed.
- **H8**: Subagent provider injection unspecified — Task 010.1 now specifies `NewSubAgent(provider Provider, opts SubAgentOpts)` constructor.
- **H9**: Session.Delete and migration missing from 011 — added subtask 011.2a for migration scaffolding.
- **H10**: Built-in skills embedding unspecified — added 009.7 for `//go:embed` directive and embedded filesystem.
- **H11**: SDK interactive model prompt breaks library contract — SDK returns `[]types.Model` + ambiguity flag; CLI (014) handles interactive selection.

**Medium fixes:**
- **M12**: Task 012 too large — noted, kept as single task (critical path considerations).
- **M13**: No integration testing beyond 012 — added integration test subtask 013.12 for full SDK flow.
- **M14**: No logging infrastructure — added `log/slog` convention to Task 006.
- **M16**: OpenAI-compatible provider count — corrected from 5 to 6.
- **M17**: Context file loading interface explicit — config returns `[]string` paths, agent calls `os.ReadFile`.
- **M18**: Session path resolution — session/ receives path via constructor from SDK, no config import.

**Low notes (deferred to implementation):**
- **L19**: Comparison tables have unverified PI claims — noted, acceptable as qualitative context.
- **L20**: No Makefile/CI task — will be addressed during Task 006 implementation.
- **L21**: Sentinel error types — added to Task 006.7 (`types/errors.go`).
- **L22**: `/name` command backend — added `Session.Rename()` to Task 013.7.
- **L23**: Manual E2E test → automated — changed to `exec.Command` test in Task 014.9.
- **L24**: `BashExecution` type placement — deferred to Task 008 implementation (can be local or in types).

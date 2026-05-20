# Task 002 Worklog — Requirements Definition

## Session — 2026-05-02

### Subtask 002.1 — Target User & Primary Use Cases (DONE)

#### Requirements gathered from user:

**Primary User**: Single user (the author). Personal tool, not for distribution.

**Problem with PI**: Extensions don't match user's needs. Would need to build all extensions themselves to align with their workflow. User wants skills and subagents in standard workflow without installing/configuring extensions.

**Primary Use Cases**:
1. **Coding** — standard development workflow
2. **Article writing** — creating written content
3. **Research & analysis** — analyzing and synthesizing knowledge
4. **Generic task orchestration** — core agent acts as orchestrator for workflows defined via skills and subagents

**Key insight**: Core agent should serve primarily as an **orchestrator** — coordinating skills and subagents to execute defined workflows, rather than being the primary worker itself.

**No extension layer** — features must be built-in and ready to use.

### Subtask 002.2 — Core Features (DONE)

**Built-in Skills (MVP)**:
1. **Skill Builder** — skill for creating new skills
2. **Subagent Builder** — skill for creating new subagents

**Built-in Subagent Types**:
- Researcher
- Reviewer
- Implementor
- Security Reviewer
- QA

**Orchestration Model**:
- Both structured workflows (skill A → subagent B → skill C) and free-form orchestration supported

**Core Tools (MVP)**:
- File reading/editing
- Bash command execution
- Code execution
- Git operations
- Grep/search
- TUI chat

**Out of scope for MVP**:
- RCP (Remote Command Protocol) mode
- Long provider list (shorter list for beginning)

### Subtask 002.3 — Sub-agent Requirements (DONE)

**Subagent Definition Format**: Compatible with PI, OpenCode, Claude Code
**Context Model**: Fresh context by default (subagents don't inherit parent conversation)
**Communication**: Parent ↔ child only, no subagent-to-subagent
**Side-task Pattern**: Ability to spin up one-time subagent with a question, sharing parent context on demand, to avoid polluting parent conversation. Results returned to parent without polluting main context.

### Subtask 002.4 — Skills Requirements (DONE)

**Format**: Agent Skills standard (agentskills.io) — SKILL.md with frontmatter (`name`, `description`) + markdown instructions. Compatible with PI, OpenCode, Claude Code.

**Directory structure**: `SKILL.md` + optional `scripts/`, `references/`, `assets/`.

**Progressive disclosure**: Only name + description in system prompt; full content loaded on-demand.

**Discovery locations**:
- Global: `~/.tau/skills/`
- Global cross-tool: `~/.agents/skills/`
- Project cross-tool: `.agents/skills/`
- No package system for MVP

**Commands**: `/skill:name` to explicitly load a skill.

**Built-in skills (MVP)**:
- `skill-builder` — creates new SKILL.md skills
- `subagent-builder` — creates new subagent definitions

### Subtask 002.5 — Provider Requirements (DONE)

**Providers (initial list)**:
- OpenAI
- Anthropic
- Google Gemini
- OpenCode Zen
- OpenCode Go
- OpenRouter
- Local models (Ollama, llama.cpp, LM Studio?)

### Subtask 002.8 — Non-Functional Requirements (DONE)

- Performance and lightweight focus
- Pragmatic decisions — performance drivers considered but not dogmatic
- Go binary with minimal dependencies

### Subtask 002.6 — Session Management Requirements (DONE)

- Session persistence: YES — sessions saved and resume-able across restarts
- Session naming: auto-generated
- Session branching/forking: NICE TO HAVE
- Storage: TBD (architecture decision)

### Subtask 002.7 — TUI Requirements (DEFERRED)

Deferred to deeper conversation during architecture/implementation phase.
Placeholder: chat-based interface with streaming output, file references, and command support.

### Subtask 002.9 — Out of Scope (DONE)

Explicitly NOT in scope:
- Extension system
- Package manager
- Web UI
- Multi-user support
- RCP mode
- Subagent-to-subagent communication

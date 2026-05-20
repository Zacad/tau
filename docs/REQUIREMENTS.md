# REQUIREMENTS

## 1. Product Vision

A minimalist agentic coding tool built on PI, designed for a single user who wants skills and subagents working out of the box — without installing or configuring extensions. The core agent serves primarily as an orchestrator, coordinating skills and subagents to execute defined workflows across coding, writing, research, and analysis tasks.

## 2. Target User

- **Primary user**: Single user (the author)
- **Not for distribution** — personal tool

## 3. Core Features

### 3.1 Orchestrator Model

The core agent acts as an orchestrator, coordinating skills and subagents to execute workflows. Both structured workflows (skill → subagent → skill) and free-form orchestration are supported.

### 3.2 Built-in Skills (MUST)

| Skill | Description |
|-------|-------------|
| `skill-builder` | Creates new SKILL.md skills following the Agent Skills standard |
| `subagent-builder` | Creates new subagent definitions |

### 3.3 Built-in Subagent Types (MUST)

- **Researcher** — research and gather information
- **Reviewer** — review code or content
- **Implementor** — implement features or changes
- **Security Reviewer** — security analysis
- **QA** — quality assurance and testing

### 3.4 Core Tools (MUST)

- File reading and editing
- Bash command execution
- Code execution
- Git operations
- Grep/search
- TUI chat interface

## 4. Sub-Agent Requirements

| Requirement | Detail |
|-------------|--------|
| Definition format | Compatible with PI, OpenCode, Claude Code |
| Context model | Fresh context by default |
| Side-task pattern | Spin up one-time subagent with parent context on demand to avoid polluting main conversation |
| Communication | Parent ↔ child only; no subagent-to-subagent |
| Results | Returned to parent without polluting main context |

## 5. Skills Requirements

| Requirement | Detail |
|-------------|--------|
| Format | Agent Skills standard (agentskills.io) — SKILL.md with frontmatter + markdown |
| Structure | `SKILL.md` + optional `scripts/`, `references/`, `assets/` |
| Discovery | Progressive disclosure — only name + description in system prompt |
| Global discovery | `~/.tau/skills/`, `~/.agents/skills/` |
| Project discovery | `.agents/skills/` |
| Commands | `/skill:name` to explicitly load |
| Cross-tool compatibility | YES — compatible with PI, OpenCode, Claude Code skill formats |

## 6. Provider Requirements

| Provider | Priority |
|----------|----------|
| OpenAI | MUST |
| Anthropic | MUST |
| Google Gemini | MUST |
| OpenCode Zen | SHOULD |
| OpenCode Go | SHOULD |
| OpenRouter | SHOULD |
| Local models (Ollama, llama.cpp, LM Studio) | SHOULD |

## 7. Session Management

| Requirement | Detail |
|-------------|--------|
| Persistence | Sessions saved and resume-able across restarts |
| Naming | Auto-generated |
| Branching/forking | SHOULD (nice to have) |

## 8. TUI Requirements

**DEFERRED** — requires deeper conversation during architecture/implementation phase.

Placeholder: Chat-based interface with streaming output, file references, and command support.

## 9. Non-Functional Requirements

| Requirement | Detail |
|-------------|--------|
| Language | Go |
| Dependencies | Minimal |
| Performance | Lightweight, fast startup — pragmatic decisions |

## 10. Out of Scope

The following are explicitly NOT in scope:

- Extension system
- Package manager
- Web UI
- Multi-user support
- RCP (Remote Command Protocol) mode
- Subagent-to-subagent communication
- Tree/branching sessions (MVP)

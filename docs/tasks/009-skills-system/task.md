# Task 009: Skills System

## Why

Skills define capabilities that the agent can discover and use. The skills system implements 3-tier discovery (built-in, global, project), SKILL.md parsing with YAML frontmatter, progressive disclosure for system prompts, and built-in skills. This task is a parallel track — it only depends on foundation (006).

## Comparison Analysis: Skills vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Discovery Paths | `~/.pi/agent/skills/`, `.agents/skills/`, `--skill` flag | `~/.tau/skills/`, `~/.agents/skills/`, `.agents/skills/` (project) |
| Skill Format | SKILL.md with YAML frontmatter + markdown | Same — Agent Skills standard compliant |
| Validation | Name must match dir, max 64 chars, lowercase/hyphens | Same rules |
| Prompt Integration | `formatSkillsForPrompt()` — progressive disclosure | Same pattern: only name+description in system prompt |
| Loading | Full content loaded on `/skill:name` or agent decision | Same behavior |
| Built-in Skills | None (skills are extension-based) | `skill-builder`, `subagent-builder` embedded in binary |
| Symlinks | Followed | Followed |
| Exclusions | `.gitignore`, `node_modules` skipped | Same |

## Main Constraints

- Must be compatible with Agent Skills standard (agentskills.io)
- Cross-tool compatibility: SKILL.md format must work with PI, OpenCode, Claude Code
- Only `gopkg.in/yaml.v3` allowed for frontmatter parsing
- Progressive disclosure must only expose name + description in system prompt
- Invalid skills silently skipped with warning — no crash

## Dependencies

- `internal/types/` (Task 006)

## Subtasks

- [x] **009.1** — `internal/skills/skill.go` — Skill struct
- [x] **009.2** — `internal/skills/discovery.go` — 3-tier directory scanning (built-in, global, project)
- [x] **009.3** — `internal/skills/parser.go` — SKILL.md YAML frontmatter parsing with validation
- [x] **009.4** — `internal/skills/prompt.go` — Progressive disclosure formatting
- [x] **009.5** — `internal/skills/builtin/skill-builder/SKILL.md` — Built-in skill for creating skills
- [x] **009.6** — `internal/skills/builtin/subagent-builder/SKILL.md` — Built-in skill for creating subagents
- [x] **009.7** — `internal/skills/embed.go` — `//go:embed` directive for built-in skills, embedded filesystem access
- [x] **009.8** — Unit tests for discovery, parsing, formatting

## Acceptance Criteria

- [x] 3-tier discovery finds skills in all paths (built-in, global `~/.tau/skills/`, `~/.agents/skills/`, project `.agents/skills/`)
- [x] SKILL.md parsing validates name, description, frontmatter
- [x] Name validation: lowercase, hyphens, 0-9, max 64 chars
- [x] Directory name must match skill name — enforced on load
- [x] Progressive disclosure output matches ARCHITECTURE.md §4.2 XML format
- [x] Symlinks followed, `node_modules` skipped, `.gitignore`/`.ignore` patterns respected
- [x] Built-in skills (skill-builder, subagent-builder) defined as valid SKILL.md files
- [x] Invalid skills silently skipped with warning logged
- [x] Unit tests with temp filesystem for discovery and parsing (use `testutil/` helpers)
- [x] No internal dependencies except `types`

## Testing & Verification Strategy

**Unit tests** (temp filesystem via `t.TempDir()`):
- Parser: valid SKILL.md (all fields), missing description (error), invalid name chars (error), name > 64 chars (error), malformed YAML frontmatter (error)
- Discovery: empty directory (no skills), valid skill dir, nested dirs (no recursion), symlinked skill dir
- Priority: same skill name in project and global — project wins (override)
- Exclusions: skill inside `node_modules/` (skipped), `.gitignore` pattern matching skill dir (skipped)
- go:embed: verify built-in skills accessible at runtime, content matches source files

**Integration tests**:
- Full discovery pipeline: create temp directory structure with skills at all 3 tiers, verify correct set with correct priority
- Progressive disclosure: feed discovered skills through formatter, verify XML output matches spec

**Quality gates**:
- `go test ./internal/skills/...` — all pass
- Built-in SKILL.md files validate against Agent Skills standard (yaml frontmatter + markdown body)
- No skill imports anything except `types` and stdlib

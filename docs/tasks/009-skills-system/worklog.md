# Worklog — Task 009: Skills System

## 2026-05-03

### Implementation

All subtasks completed in a single session.

**009.1 — `internal/skills/skill.go`** ✅
- `Skill` struct with all fields from ARCHITECTURE.md §4.2
- Plus unexported `dir` (filesystem root) and `fsys` (fs.FS for embedded/OS)
- `ValidateSkillName()`: lowercase, digits, hyphens, max 64 chars, must start with alphanumeric
- `ValidateDescription()`: required, max 1024 chars
- `Skill.Validate()`: combined validation
- `Skill.IsValidSource()`: checks source is builtin/global/project
- `Skill.ResolvePath(rel)`: resolves relative path for filesystem-based skills (empty for builtin)
- `Skill.ReadFile(rel)`: reads file from skill's filesystem (works for both embedded and OS)
- Sentinel errors: `errEmptyName`, `errNameTooLong`, `errInvalidNameChars`, `errEmptyDescription`, `errDescriptionTooLong`

**009.3 — `internal/skills/parser.go`** ✅
- `ParseSkillMD(reader, dirName)`: parses SKILL.md YAML frontmatter + markdown body
- Frontmatter delimited by `---` markers
- Validates name matches directory name
- Supports: name, description, disable_model_invocation, scripts, references, assets
- Uses `gopkg.in/yaml.v3` for YAML parsing (promoted to direct dependency)

**009.4 — `internal/skills/prompt.go`** ✅
- `FormatForPrompt([]*Skill)`: progressive disclosure XML output per ARCHITECTURE.md §4.2
- Excludes skills with `DisableModelInvocation=true`
- XML-escapes attribute values (`&`, `<`, `>`, `"`)
- Output format: `<skills>\n<skill name="..." description="..."/>\n</skills>`

**009.5 — `internal/skills/builtin/skill-builder/SKILL.md`** ✅
- Comprehensive skill creation guide
- Covers: SKILL.md structure, name rules, description guidelines, content guidelines
- Discovery tiers table, workflow for creating/updating skills, best practices

**009.6 — `internal/skills/builtin/subagent-builder/SKILL.md`** ✅
- Comprehensive subagent definition guide
- Covers: 5 built-in subagent types, context modes (fresh/fork), result handling
- Tool scoping, execution model, parallel patterns, workflow patterns
- Security considerations

**009.7 — `internal/skills/embed.go`** ✅
- `//go:embed builtin/*/SKILL.md` directive
- `BuiltinFS()`: returns embedded filesystem for built-in skills

**009.2 — `internal/skills/discovery.go`** ✅
- `DiscoverSkills(cwd)`: 3-tier discovery (builtin → global → project)
- `DiscoverSkillsWithFS(cwd, builtinFS)`: testable variant with explicit embedded FS
- Priority: project overrides global, global overrides builtin (map-based deduplication)
- Symlinks followed (uses `os.Stat` to resolve)
- `node_modules` skipped
- `.gitignore` and `.ignore` patterns respected (simple glob matching via `filepath.Match`)
- No recursion — only direct subdirectories of skill paths are scanned
- Invalid skills silently skipped with `slog.Warn`
- Global paths: `~/.tau/skills/`, `~/.agents/skills/`
- Project: `.agents/skills/` walking up from cwd through parent directories
- Reference `.md` files (direct children, excluding SKILL.md) loaded into `skill.References`

**009.8 — Tests** ✅
- `skill_test.go`: 22 tests covering name validation, description validation, skill.Validate, source validation
- `parser_test.go`: 13 tests covering valid SKILL.md, missing fields, invalid names, name mismatch, too-long values, malformed YAML, no frontmatter, empty file, disable_model_invocation, scripts/references/assets, multiline description
- `prompt_test.go`: 7 tests covering empty list, single skill, multiple skills, disabled exclusion, all disabled, XML escaping in name and description
- `discovery_test.go`: 15 tests covering empty dirs, builtin only, project overrides global, node_modules skip, .gitignore patterns, glob patterns, no recursion, symlink following, invalid skill skip, both global dirs, builtin content loading, parent directory walk, same-name override, .ignore file, reference files

### Quality Gates
- `go test ./internal/skills/...` — **68 tests, all pass** ✅
- `go vet ./internal/skills/...` — clean ✅
- `go build ./internal/skills/...` — clean ✅
- `go mod tidy` — clean ✅
- `go test -race ./internal/skills/...` — clean ✅
- `go build ./...` — full project builds ✅
- `go test ./...` — **all packages pass** ✅

### Design Decisions
1. **yaml.v3** promoted to direct dependency (was indirect via invopop/jsonschema)
2. **Builtin skills** moved from `skills/builtin/` to `internal/skills/builtin/` for clean `//go:embed` paths
3. **Skill struct** uses unexported `dir` and `fsys` fields for filesystem abstraction
4. **`Skill.ResolvePath()`** returns empty string for builtin skills (no OS filesystem)
5. **`Skill.ReadFile()`** works for both embedded (builtin) and OS (global/project) skills
6. **Content trimming**: leading whitespace after closing `---` is trimmed from SKILL.md body
7. **Discovery**: project skills override global, global override builtin (loaded last, map-based dedup)

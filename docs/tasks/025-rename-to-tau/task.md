# Task 025: Rename tau → tau

## Why

The project name "tau" is being renamed to "tau" for branding/direction reasons. All references — code, config paths, module paths, documentation, CLI binary name — must be updated consistently. A partial rename would break builds, tests, and user workflows.

## Scope Analysis

**Total references**: ~375 occurrences of "tau" across ~80 files.

### Categories of references

| Category | Count | Description |
|----------|-------|-------------|
| Go module path (`github.com/adam/tau`) | ~95 | Import paths in all `.go` files |
| Config directory (`~/.tau/`) | ~73 | Config, auth, sessions, skills paths |
| CLI binary (`cmd/tau/`) | ~82 | Binary name, directory, references in docs |
| Documentation | ~80+ | ARCHITECTURE.md, REQUIREMENTS.md, DECISIONS.md, task docs |
| Docker volume | 1 | `tau-ollama-data` in docker-compose.yml |
| Root directory | 1 | `/var/home/adam/Projects/tau` |

### Key rename mappings

| From | To | Scope |
|------|----|-------|
| `github.com/adam/tau` | `github.com/adam/tau` | Go module + all imports |
| `cmd/tau/` | `cmd/tau/` | CLI binary directory |
| `~/.tau/` | `~/.tau/` | Config/auth/sessions/skills paths |
| `tau-ollama-data` | `tau-ollama-data` | Docker volume name |
| Binary name `tau` | `tau` | go build, docs, help text |
| Root directory `tau/` | `tau/` | Filesystem |

## Constraints

1. **Must be atomic**: All references must be updated in one pass — partial rename breaks the build
2. **Go module path is critical**: Every `.go` file imports `github.com/adam/tau/...` — must be replaced with `github.com/adam/tau/...`
3. **Config directory migration**: Existing users with `~/.tau/` need their data accessible — decision needed on migration strategy
4. **Tests must pass**: All existing tests (200+) must pass after rename
5. **No functional changes**: Pure rename, no logic changes
6. **Git history**: Rename root directory last, after all file content changes

## Subtasks

### 025.1: Update Go module path and all imports

- Change `go.mod` module path from `github.com/adam/tau` to `github.com/adam/tau`
- Update all import paths in every `.go` file
- **AC**: `go vet ./...` and `go build ./...` pass

### 025.2: Rename cmd/tau/ → cmd/tau/

- Rename directory `cmd/tau/` to `cmd/tau/`
- Update any internal references to the binary name
- **AC**: `go build ./cmd/tau` produces `tau` binary

### 025.3: Update config directory paths (~/.tau/ → ~/.tau/)

- Update `internal/config/paths.go` and all references
- Update all tests that reference `.tau` directory
- **AC**: All config-related tests pass; paths resolve to `~/.tau/`

### 025.4: Update documentation

- ARCHITECTURE.md, REQUIREMENTS.md, DECISIONS.md, AGENTS.md
- All task docs in `docs/tasks/`
- **AC**: No remaining "tau" references in docs (except historical task descriptions)

### 025.5: Update Docker/infrastructure

- Update `ollama/docker-compose.yml` volume name
- **AC**: Docker compose references use `tau-ollama-data`

### 025.6: Run full test suite and verify

- `go test ./... -race`
- `go vet ./...`
- `go build ./...`
- Manual E2E verification with Ollama
- **AC**: All tests pass, binary works, sessions load correctly

### 025.7: Rename root directory

- Rename `/var/home/adam/Projects/tau` → `/var/home/adam/Projects/tau`
- Verify git still works, all tooling (IDE, etc.) adjusted
- **AC**: `git status` works from new directory, all tests still pass

## Acceptance Criteria

1. Zero remaining references to "tau" in code (except historical task docs where it refers to the old name in past context)
2. `go build ./...`, `go vet ./...`, `go test ./... -race` all pass
3. `tau` binary builds and runs correctly
4. Config directory resolves to `~/.tau/`
5. Existing sessions in `~/.tau/` are NOT auto-deleted (user migrates manually)
6. Documentation consistently uses "tau"
7. Root directory is `/var/home/adam/Projects/tau`

## Testing Strategy

1. **Automated**: Full `go test ./... -race` suite (200+ existing tests)
2. **Build verification**: `go build ./cmd/tau` produces working binary
3. **Search verification**: `rg "tau" --type go` returns zero results (after excluding task docs)
4. **Manual E2E**: Run `tau` with Ollama, verify session creation, tool calls, resume
5. **Config path verification**: Confirm `~/.tau/` is used for new sessions

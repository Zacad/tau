## Goal
Create minimalist agentic tau based on PI

## Basic Rules
- don't make assumptions, if something is not clear ask user
- always verify your claims
- write in go lang
- use idiomatic approach
- search for existing patterns and good practices for the problem you’re trying to solve
- subagents support out of the box
- always manually verify your work
- always write and run tests
- always document your work
- always verify your work
- user must confirm all work
- you can only start work on new task after user confirmation
- follow agile principles, use iterative and incremental approach
- must update documentation

## Documentation
- project documentation is in ./docs
- TRACKING.md is in ./docs
- tasks are in ./docs/tasks
- split work into tasks
- each task have own folder and task.md file
- task have acceptance criteria
- each task can have subtasks
- track subtasks in task.md file
- each task have worklog file documenting all work done on the task
- there is TRACKING.md file for tracking status of all tasks
- product requirements are described in REQUIREMENTS.md file
- architecture  is described in ARCHITECTURE.md file
- update ARCHITECTURE.md file when new decision is made
- document all decisions in DECISIONS.md file

## task definition
- start with why
- each task should have comparison analysis with PI
- define main constraints
- define subtasks
- define acceptance criteria
- define acceptance criteria for subtasks
- don't make assumptions
- don't write implementation details or any code
- describe drivers and constraints
- use subagent to critically challenge your design
- design testing strategy

## Task implementation
- follow TDD approach
- start with design
- follow idiomatic approach, use best practices
- be pragmatic
- consider security, performance
- consider edge cases
- use subagent to critically challenge your design
- after finish, rebuild binary in './' for manual testing

## Local Ollama for Testing

Ollama runs in Docker with Vulkan GPU acceleration on the host AMD iGPU (Radeon 8060S Graphics). Model data persists across container restarts via a named volume.

### Location
`./ollama/docker-compose.yml`

### Quick commands
```bash
cd ollama
docker compose up -d          # start (or restart) Ollama
docker compose down            # stop (data persists in volume)
docker compose logs -f         # follow logs
docker compose logs --tail 50  # last 50 log lines
```

### Running inference
```bash
curl -s http://localhost:11434/api/generate \
  -d '{"model": "ministral-3:14b", "prompt": "Your prompt here", "stream": false}'
```

### Using in tau tests
- Ollama API is available at `http://localhost:11434`
- Use `ministral-3:14b` as the default model for testing
- The model is pre-loaded on first container start and persists in `tau-ollama-data` volume
- To reset everything (clear models and start fresh): `docker compose down -v`

## Local SearXNG for Testing

SearXNG is a self-hosted metasearch engine running alongside Ollama in Docker. It provides a JSON search API with no API key required.

### Location
`./ollama/docker-compose.yml` (same compose file as Ollama)

### Quick commands
```bash
cd ollama
docker compose up -d searxng          # start SearXNG (also starts with `docker compose up -d`)
docker compose down                    # stop (Ollama data persists in volume)
docker compose logs -f searxng         # follow SearXNG logs
docker compose logs --tail 50 searxng  # last 50 log lines
```

### Running search
```bash
curl -s "http://localhost:8964/search?q=Go+programming&format=json" | jq '.results[:3] | .[] | {title, url, content}'
```

### Configuration
- Settings file: `./ollama/searxng-settings.yml` (mounted read-only into container)
- JSON API enabled (`formats: [html, json]` in settings)
- Limiter disabled for local use
- No persistent volume — config comes solely from bind mount

### Using in tau tests
- SearXNG API is available at `http://localhost:8964`
- Search endpoint: `GET /search?q=<query>&format=json`
- Health check: `GET /healthz`
- No API key required
- To reset: `docker compose down -v` (also clears Ollama data)

## Reference Implementations

When implementing new features or fixing bugs, always research existing solutions in OpenCode and PI first. Both are cloned locally and serve as the primary reference for design decisions.

### OpenCode
- **Location**: `~/Projects/opencode`
- **Key files**:
  - Provider system: `packages/opencode/src/provider/provider.ts`
  - Schema transforms: `packages/opencode/src/provider/transform.ts`
  - Model resolution: per-provider SDK setup in `provider.ts`
  - Zen provider: uses `@ai-sdk/openai`, `@ai-sdk/anthropic`, `@ai-sdk/google` packages

### PI
- **Location**: `~/Projects/pi`
- **Key files**:
  - Model resolver: `packages/coding-agent/src/core/model-resolver.ts`
  - Provider setup: per-provider factory functions mapping to AI SDK packages

### Task requirement
Every new task MUST start with researching the problem in OpenCode and PI. Look for:
1. How the feature is implemented in both codebases
2. What edge cases they handle
3. How they handle provider-specific quirks (schema sanitization, auth modes, etc.)
4. What SDK/packages they use and how they map to Go equivalents

# Task 007: Provider System

## Why

The provider system is the gateway to all LLM capabilities. Without it, the agent cannot communicate with any model. It implements the Provider interface, model registry, auth resolution, and all provider implementations. This task sits on the critical path — Task 012 (Agent Loop) cannot proceed without it.

## Comparison Analysis: Provider Architecture vs PI

| Dimension | PI Approach | Our Approach |
|-----------|-------------|--------------|
| Provider Interface | `AgentProvider` with `stream()` returning async iterable | `Provider` interface with `Stream()` returning `<-chan StreamEvent` |
| Provider Implementations | Per-provider files (25+ files across 9 API types) | Per-API-type — OpenAI-compat covers 6 providers |
| Auth Resolution | `resolveConfigValueOrThrow()`: env → config → OAuth | 4-step: CLI flag → auth.json → env → config file |
| Model Registry | Hardcoded model lists per provider | Central `Model` registry with pattern matching |
| Streaming | Async iterables, event types per provider | Go channels, unified `StreamEvent` type |
| Thinking Support | Varies by provider | `ThinkingLevel` enum in `StreamOptions` |

## Main Constraints

- Only two external dependencies total for entire project (yaml.v3, jsonschema)
- HTTP must use stdlib `net/http` — no external HTTP client
- Provider interface must support both streaming and non-streaming
- Auth resolution must support literal keys, env var references, and shell commands
- All providers must handle rate limits (429) with Retry-After parsing
- Error propagation with exponential backoff (max 2 retries)

## Dependencies

- `internal/types/` (Task 006)

## Subtasks

- [x] **007.1** — `internal/provider/provider.go` — Provider interface (`Stream()`, `Complete()`)
- [x] **007.2** — `internal/provider/model.go` — Model struct, model registry, pattern matching
- [x] **007.3** — `internal/provider/registry.go` — Provider registration, model resolution algorithm
- [x] **007.4** — `internal/provider/auth.go` — 4-step auth resolution (CLI flag → auth.json → env → config)
- [x] **007.5** — `internal/provider/stream.go` — streaming channel handling (StreamEvent type defined in `types/`, not here)
- [x] **007.6** — `internal/provider/openai.go` — OpenAI provider (openai-responses API)
- [x] **007.7** — `internal/provider/anthropic.go` — Anthropic provider (anthropic-messages API)
- [x] **007.8** — `internal/provider/google.go` — Google Gemini provider (google-generative-ai API)
- [x] **007.9** — `internal/provider/openai_compat.go` — OpenAI-compatible provider (OpenRouter, OpenCode, Ollama, llama.cpp, LM Studio)
- [x] **007.10** — Unit tests with mocked HTTP for all providers

## Acceptance Criteria

- [x] `Provider` interface satisfies ARCHITECTURE.md §6.1
- [x] Model registry supports exact ID and pattern matching (§7.2b)
- [x] Auth resolution chain works for all 3 key formats (literal, env ref, shell command)
- [x] `auth.json` keys resolved at runtime for `$ENV` and `!command` formats
- [x] OpenAI, Anthropic, Google providers stream correctly
- [x] OpenAI-compatible provider covers all 6 compat providers (OpenRouter, OpenCode Zen, OpenCode Go, Ollama, llama.cpp, LM Studio) via configuration
- [x] `StreamOptions` supports ThinkingLevel, MaxTokens, Temperature
- [x] Rate limit handling (429 Retry-After) implemented per provider
- [x] Error propagation with exponential backoff (max 2 retries)
- [x] Unit tests with mocked HTTP for all providers (use `testutil/` helpers)
- [x] No internal dependencies except `types`

## Results

**71 tests pass**. **80.7% coverage**. All quality gates clean.

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test -race ./...` — all pass, no data races
- `go mod tidy` — clean
- No internal dependencies except `types` and stdlib

## Testing & Verification Strategy

**Unit tests** (mock HTTP via `httptest.NewServer`):
- Each provider: verify correct request format (headers, body, endpoint), response parsing, streaming event sequence
- Provider interface: `Stream()` returns `<-chan StreamEvent` with correct order (start → text_delta → done)
- Model registry: exact ID match, substring match, multiple matches (return list), no match (error)
- Auth: literal key, `$ENV_VAR` resolution, `!command` execution, missing key (error), unset env var (error)
- Rate limiting: 429 with `Retry-After` header → exact wait; 429 without header → exponential backoff
- Error propagation: network timeout, malformed JSON, HTTP 500

**Integration tests**:
- Full stream per provider with mock server, verify all event types emitted in order
- Auth chain priority: CLI flag > auth.json > env > config — verify correct precedence

**Race detection**:
- `go test -race ./internal/provider/...` — no data races in channel handling

**Quality gates**:
- Each provider implementation has ≥80% line coverage
- No provider imports anything except `types` and stdlib

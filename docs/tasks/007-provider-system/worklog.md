# Task 007 Worklog: Provider System

## Subtask 007.1 — Provider Interface ✅

### Implementation
- Created `internal/provider/provider.go`
- `Provider` interface with `Name()`, `Stream()`, `Complete()` methods
- `baseProvider` struct with shared fields (name, httpClient, apiKey)
- `HTTPClient` interface for testability
- `Request`/`Response` wrapper types

### Tests
- Interface satisfaction test
- `apiKeyOrErr` validation test

---

## Subtask 007.2 — Model Registry ✅

### Implementation
- Created `internal/provider/model.go`
- `ModelRegistry` struct with `Find()`, `Get()`, `ListAll()`, `ListByProvider()`, `Register()`
- 10 built-in models: OpenAI (gpt-4o, gpt-4o-mini, o1, o3), Anthropic (claude-sonnet-4, claude-3-7-sonnet, claude-3-5-sonnet), Google (gemini-2.5-pro, gemini-2.5-flash, gemini-2.0-flash)
- Pattern matching: exact ID match → case-insensitive substring on ID/Name → multiple match error

### Tests
- Exact ID match (10 models)
- Pattern matching (substring, case-insensitive)
- Multiple matches error
- No match error
- ListByProvider filtering
- Custom model registration

---

## Subtask 007.3 — Provider Registry ✅

### Implementation
- Created `internal/provider/registry.go`
- `Registry` struct: `Register()`, `Get()`, `Models()`, `SetDefaultModel()`, `ResolveModel()`, `ListProviders()`
- Delegates model resolution to `ModelRegistry`

### Tests
- Register and Get
- ResolveModel with exact ID, default model, no model, no match
- ListProviders
- Models access

---

## Subtask 007.4 — Auth Resolution ✅

### Implementation
- Created `internal/provider/auth.go`
- 4-step chain: CLI flag → auth.json → env var → config file
- Key formats: literal, `$ENV_VAR`, `!command`
- `ResolveError` type with descriptive message
- `auth.json` file reading with JSON parsing

### Tests
- CLI flag (highest priority)
- Environment variable resolution
- `$ENV_VAR` format resolution
- `!command` shell execution
- Unset env var error
- auth.json reading (literal + env ref)
- Missing provider in auth.json
- Invalid JSON in auth.json
- Priority chain (CLI > auth.json > env)
- Standard env var name generation
- Empty key error

---

## Subtask 007.5 — Stream Helper ✅

### Implementation
- Created `internal/provider/stream.go`
- `streamToChannel()` — safely sends events with context cancellation
- `sendEvent()` — single event send with context check
- `closeWithError()` — sends error and closes channel
- `collectStream()` — consumes channel for testing/Complete()

### Tests
- `sendEvent` with cancelled context
- `streamToChannel` with cancelled context
- `closeWithError` behavior

---

## Subtask 007.6 — OpenAI Provider ✅

### Implementation
- Created `internal/provider/openai.go`
- OpenAI Responses API (`POST /v1/responses`)
- Bearer token authentication
- SSE response parsing (event: field for type)
- Text delta, tool call, usage events
- Thinking support (text format)

### Tests
- Stream with text delta events
- Complete with accumulated text
- Empty API key error
- Server error handling
- Rate limit error handling
- Header verification (Authorization, Content-Type, custom headers)
- Request format (model, stream, max_tokens)
- Tool definitions in request

---

## Subtask 007.7 — Anthropic Provider ✅

### Implementation
- Created `internal/provider/anthropic.go`
- Anthropic Messages API (`POST /v1/messages`)
- `x-api-key` + `anthropic-version` headers
- SSE response parsing
- Text delta, thinking delta, tool call events
- Thinking budget mapping (minimal→1024, xhigh→16384)
- Cache token tracking

### Tests
- Stream with message_start → content_block_delta → message_delta → message_stop
- Complete with accumulated text
- Empty API key error
- Thinking delta events
- Thinking budget function
- Tool definitions in request
- MaxTokens default from model

---

## Subtask 007.8 — Google Provider ✅

### Implementation
- Created `internal/provider/google.go`
- Google Generative AI API (`/models/{model}:streamGenerateContent?alt=sse&key=`)
- API key in URL query parameter
- JSON streaming (data: prefix per line)
- Text delta, function call, usage events
- System instruction support

### Tests
- Stream with text delta events
- Complete with accumulated text
- Empty API key error
- Tool definitions in request
- Usage metadata parsing

---

## Subtask 007.9 — OpenAI-Compatible Provider ✅

### Implementation
- Created `internal/provider/openai_compat.go`
- OpenAI Chat Completions API (`POST /chat/completions`)
- Configurable base URL, API path, extra headers
- Covers: OpenRouter, OpenCode Zen, OpenCode Go, Ollama, llama.cpp, LM Studio
- `[DONE]` marker handling
- Streaming delta with tool calls

### Tests
- Stream with text delta events
- `[DONE]` marker handling
- Complete with accumulated text
- Empty API key error
- Default config (provider name, API path)
- Custom headers (X-Provider)
- Tool definitions in request

---

## Subtask 007.10 — HTTP Client & Integration Tests ✅

### Implementation
- Created `internal/provider/http.go`
- `DefaultHTTPClient` using `net/http`
- Retry logic: max 2 retries, exponential backoff (1s, 2s, 4s, max 60s)
- Rate limit handling: parse `Retry-After` header (seconds or HTTP date)
- Server error (5xx) retry
- Request body replay for retries
- `SSELineReader` for parsing SSE streams

### Tests
- Retry on 500 (2 attempts)
- Retry on 429 with Retry-After (2 attempts)
- Exhausted retries (3 attempts total)
- Network error (connection refused)
- Success on first try
- POST with body
- `Retry-After` header parsing (seconds, empty)
- Backoff delay calculation
- String truncation

---

## Final Results

| Metric | Value |
|--------|-------|
| Tests | 71 |
| Coverage | 80.7% |
| Race detection | Clean |
| go vet | Clean |
| go build | Clean |
| go mod tidy | Clean |
| Internal dependencies | Only `types` + stdlib |
| External dependencies | None added |

## Files Created

| File | Purpose |
|------|---------|
| `provider.go` | Provider interface, base types |
| `model.go` | Model registry, 10 built-in models |
| `registry.go` | Provider registration, model resolution |
| `auth.go` | 4-step auth resolution |
| `stream.go` | Stream channel utilities |
| `http.go` | HTTP client with retry, SSE parser |
| `openai.go` | OpenAI Responses API provider |
| `anthropic.go` | Anthropic Messages API provider |
| `google.go` | Google Generative AI provider |
| `openai_compat.go` | OpenAI-compatible provider |
| `model_test.go` | Model registry tests |
| `auth_test.go` | Auth resolution tests |
| `http_test.go` | HTTP client + retry tests |
| `openai_test.go` | OpenAI provider tests |
| `provider_test.go` | Registry, Anthropic, Google, Compat tests |

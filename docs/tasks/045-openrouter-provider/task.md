# Task 045: OpenRouter Provider Support

## Why

OpenRouter provides a unified API gateway to 300+ AI models from dozens of providers (OpenAI, Anthropic, Google, Mistral, DeepSeek, etc.) through a single API key. Adding OpenRouter support gives tau users access to the broadest model selection with automatic provider routing, fallback, and cost optimization — all through one credential.

## Comparison with PI

| Feature | PI | Tau (current) | Tau (target) |
|---------|-----|---------------|--------------|
| Provider model | Direct OpenAI SDK | OpenAICompatProvider | Dedicated OpenRouterProvider |
| Model listing | Generated 216+ models | Hardcoded curated list | Curated list + user-configurable models |
| Thinking format | `{ reasoning: { effort } }` | Not supported | Mapped from ThinkingLevel |
| Attribution headers | `HTTP-Referer`, `X-OpenRouter-Title`, `X-OpenRouter-Categories` | None | All three headers |
| Provider routing | `openRouterRouting` compat field | Not supported | Via `model.Compat` |
| Error handling | Metadata extraction, cache normalization | Generic ClassifyAPIError | OpenRouter-specific patterns |
| Model variants | `:exacto`, `:extended`, `:free` suffixes | Not supported | Passed through as model ID |
| Cost data | Per-model pricing in generated code | Hardcoded | Hardcoded in curated list |

## Comparison with OpenCode

| Feature | OpenCode | Tau (target) |
|---------|----------|--------------|
| SDK approach | `@openrouter/ai-sdk-provider` npm package | Direct HTTP via OpenAICompatProvider composition |
| Attribution headers | Plugin system (`provider.update` hook) | Built into provider headers() |
| Reasoning transform | Excluded from generic interleaved transform | Handled natively by OpenRouter |
| Model filtering | Filters `gpt-5-chat` aliases | Not needed (curated list avoids broken aliases) |
| Caching | `prompt_cache_key` session-based | Not in scope for initial implementation |

## Constraints

- Must use dedicated `OpenRouterProvider` struct (not enhancing OpenAICompatProvider)
- Curated hardcoded model list of ~20 popular models
- User can add custom models via `config.json` `providers.openrouter.models` array
- Must reuse existing auth resolution chain (`ResolveKey("openrouter", "")`)
- Must integrate with existing TUI `/connect` flow (provider catalog)
- Must follow existing provider patterns (Name(), Stream(), Complete())
- OpenRouter API is OpenAI Chat Completions compatible — can delegate to OpenAICompatProvider internally

## Design

### OpenRouterProvider Structure

```go
// internal/provider/openrouter.go

type OpenRouterProvider struct {
    baseProvider
    compat *OpenAICompatProvider  // delegates streaming/parsing
}

func NewOpenRouterProvider(apiKey string) *OpenRouterProvider
func NewOpenRouterProviderWithClient(apiKey string, client HTTPClient) *OpenRouterProvider
```

The provider composes `OpenAICompatProvider` internally to reuse all SSE parsing, delta accumulation, and tool call handling. OpenRouter-specific behavior is layered on top:

1. **Headers** — adds attribution headers on top of Bearer auth
2. **Request body** — adds `reasoning` object and `provider` routing preferences
3. **Model ID** — passes through as-is (OpenRouter format: `author/model-name`)

### Thinking Level Mapping

OpenRouter normalizes reasoning across providers via a nested `reasoning` object:

| ThinkingLevel | OpenRouter param |
|---------------|-----------------|
| `off` | `{ "reasoning": { "effort": "none" } }` |
| `minimal` | `{ "reasoning": { "effort": "low" } }` |
| `low` | `{ "reasoning": { "effort": "low" } }` |
| `medium` | `{ "reasoning": { "effort": "medium" } }` |
| `high` | `{ "reasoning": { "effort": "high" } }` |
| `xhigh` | `{ "reasoning": { "effort": "xhigh" } }` |

Applied in `buildRequest()` as an additional field in the JSON body.

### Attribution Headers

Sent with every request per OpenRouter's app attribution spec:

```
HTTP-Referer: https://tau.example/
X-OpenRouter-Title: tau
X-OpenRouter-Categories: cli-agent
```

### Provider Routing Preferences

Read from `model.Compat["routing"]` and passed as `provider` object in request body:

```json
{
  "provider": {
    "order": ["anthropic", "openai"],
    "allow_fallbacks": true,
    "sort": "price"
  }
}
```

### Curated Model List

~20 popular models with accurate pricing, context windows, and reasoning capability.

### Config Extension

Add `Models` field to `ProviderConfig` for user-defined model IDs:

```go
type ProviderConfig struct {
    Model   string   `json:"model,omitempty"`
    Enabled *bool    `json:"enabled,omitempty"`
    BaseURL string   `json:"base_url,omitempty"`
    Models  []string `json:"models,omitempty"`  // NEW: user-defined model IDs
}
```

User config example:
```json
{
  "providers": {
    "openrouter": {
      "enabled": true,
      "models": ["anthropic/claude-sonnet-4", "openai/o3", "minimax/minimax-m2"]
    }
  }
}
```

### Error Handling Enhancements

OpenRouter-specific patterns already handled by existing `ClassifyAPIError`:
- 402 → `ErrorTypeCreditExhausted` (already detected via "credit"/"billing" patterns)
- 401/403 with "model" → `ErrorTypeModelUnavailable` (already detected)
- 503 → `ErrorTypeServerError` (already handled)

Context overflow pattern to add: `maximum context length is \d+ tokens`

## Subtasks

### 045.1: OpenRouterProvider Implementation
**File**: `internal/provider/openrouter.go`

- Create `OpenRouterProvider` struct embedding `baseProvider` and composing `OpenAICompatProvider`
- Implement `Name()` returning `"openrouter"`
- Implement `Stream()` and `Complete()` delegating to internal `OpenAICompatProvider`
- Override request building to add:
  - Thinking level → `{ "reasoning": { "effort" } }` mapping
  - Provider routing preferences from `model.Compat`
- Override headers to add attribution headers (`HTTP-Referer`, `X-OpenRouter-Title`, `X-OpenRouter-Categories`)
- Define curated model list (`openRouterModels`)
- Add `RegisterOpenRouterModels(registry *ModelRegistry)` function
- Add `DiscoverOpenRouterModels(apiKey string) ([]string, error)` for TUI model discovery (fetches full catalog from `GET /models`)

### 045.2: SDK Registration
**File**: `internal/sdk/sdk.go`

- Add `registerOpenRouter(reg *provider.Registry, cfg *config.Config)` function:
  1. Check provider enabled in config
  2. Resolve API key via `ResolveKey("openrouter", "")`
  3. Create `OpenRouterProvider` instance
  4. Register provider in registry
  5. Register curated models
  6. Register user-defined models from `cfg.Providers["openrouter"].Models`
- Add call in `CreateSession()` after existing provider registrations
- Add `openrouter` case to `RegisterProvider()` API type switch

### 045.3: Config Extension
**File**: `internal/config/config.go`

- Add `Models []string` field to `ProviderConfig` struct
- No migration needed — new field is optional with zero value

### 045.4: TUI Provider Catalog
**File**: `internal/tui/providers.go`

- Add OpenRouter entry to `providerCatalog`:
  - Name: `"openrouter"`
  - DisplayName: `"OpenRouter"`
  - Description: `"300+ AI models via unified API"`
  - RequiresAPIKey: `true`
  - BaseURL: `"https://openrouter.ai/api/v1"`
- Add `testOpenRouter(apiKey string) error` — `GET /models` with auth, 200 = success
- Add `discoverOpenRouterModels(apiKey string) ([]string, error)` — parse `/models` response, return all model IDs

### 045.5: Error Pattern Enhancement
**File**: `internal/types/errors.go`

- Add context overflow pattern detection: `maximum context length is \d+ tokens`
- This pattern is used by OpenRouter and several upstream providers

### 045.6: Tests
- `internal/provider/openrouter_test.go`:
  - Provider creation with/without API key
  - Headers include attribution headers
  - Request body includes `reasoning` object for thinking levels
  - Request body includes `provider` routing from model.Compat
  - Stream/Complete delegate to OpenAICompatProvider
- `internal/sdk/sdk_test.go`:
  - Registration with valid API key
  - Registration skipped when disabled in config
  - Curated models registered
  - User-defined models from config registered
- `internal/tui/providers_test.go`:
  - Connection test with valid/invalid key
  - Model discovery returns model IDs

### 045.7: Documentation
- Update `ARCHITECTURE.md` with OpenRouter provider section
- Add `DECISIONS.md` entry for OpenRouter design decisions
- Update `TRACKING.md`

## Acceptance Criteria

### Main AC
1. `OPENROUTER_API_KEY` env var or `~/.tau/auth.json` key enables OpenRouter provider at startup
2. Curated models (~20) appear in `/model` list when provider is connected
3. Thinking levels correctly map to `{ "reasoning": { "effort" } }` format in API requests
4. Attribution headers (`HTTP-Referer`, `X-OpenRouter-Title`, `X-OpenRouter-Categories`) sent with every request
5. Connection test works in TUI `/connect` flow
6. User can add custom model IDs via `config.json` `providers.openrouter.models`
7. Custom models appear alongside curated models in `/model` list
8. Provider routing preferences from `model.Compat` passed through to API
9. All existing tests pass, new tests added for OpenRouter-specific behavior
10. go vet / go build / go test -race all pass clean

### Subtask ACs

#### 045.1 AC
- `OpenRouterProvider` implements `Provider` interface
- `Name()` returns `"openrouter"`
- Attribution headers present in HTTP requests (verified in test)
- Thinking level `high` produces `{ "reasoning": { "effort": "high" } }` in request body
- Thinking level `off` produces `{ "reasoning": { "effort": "none" } }` in request body
- Curated models registered with correct pricing, context windows, reasoning flags
- `DiscoverOpenRouterModels` fetches from `GET /models` and returns model IDs

#### 045.2 AC
- `registerOpenRouter` called in `CreateSession`
- Provider registered when API key available and not disabled
- Curated models registered in model registry
- User-defined models from config also registered
- `openrouter` case in `RegisterProvider` API switch

#### 045.3 AC
- `ProviderConfig` has `Models []string` field
- Config JSON round-trips correctly with new field
- Existing configs without `models` field load without error

#### 045.4 AC
- OpenRouter appears in TUI provider list with correct display name and description
- `testOpenRouter` returns nil for valid key, error for invalid key
- `discoverOpenRouterModels` returns non-empty list of model IDs with valid key

#### 045.5 AC
- Context overflow error pattern detected and classified appropriately
- Existing error classification tests still pass

#### 045.6 AC
- All new tests pass
- Tests cover: provider creation, headers, thinking mapping, routing, SDK registration, TUI connection test, model discovery
- go test -race clean

#### 045.7 AC
- ARCHITECTURE.md has OpenRouter section
- DECISIONS.md has entry for dedicated provider vs enhancement decision
- TRACKING.md updated with task status

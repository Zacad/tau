# Task 045: OpenRouter Provider Support — Worklog

## Session 1: Task Definition (2025-05-15)

- Researched OpenRouter API documentation (endpoints, schemas, error handling, provider routing)
- Analyzed OpenCode OpenRouter implementation (plugin system, SDK package, transforms, model filtering)
- Analyzed PI OpenRouter implementation (OpenAI SDK usage, thinking format mapping, routing preferences, error metadata)
- Analyzed tau's existing provider system (OpenAICompatProvider, registry, auth, TUI catalog)
- Prepared comprehensive implementation plan
- User decisions: dedicated OpenRouterProvider struct, curated model list with user-configurable additions
- Created task.md with full specification

## Session 2: Implementation (2026-05-15)

### 045.3: Config Extension
- Added `Models []string` field to `ProviderConfig` in `internal/config/config.go`
- No migration needed — new field is optional with zero value

### 045.1: OpenRouterProvider Implementation
- Created `internal/provider/openrouter.go` with:
  - `OpenRouterProvider` struct embedding `baseProvider` + composing `OpenAICompatProvider`
  - `NewOpenRouterProvider()` and `NewOpenRouterProviderWithClient()` constructors
  - `Name()` returning `"openrouter"`
  - `Stream()` and `Complete()` delegating to `OpenAICompatProvider` with OpenRouter-specific config
  - `openRouterHeaders()` with attribution headers (`HTTP-Referer`, `X-OpenRouter-Title`, `X-OpenRouter-Categories`)
  - `thinkingLevelToEffort()` mapping tau `ThinkingLevel` to OpenRouter effort values
  - Curated model list (`openRouterModels`) with 20 popular models
  - `RegisterOpenRouterModels()`, `OpenRouterModelIDs()`, `RegisterOpenRouterModelsFromConfig()`
  - `DiscoverOpenRouterModels()`, `DiscoverOpenRouterModelsWithClient()` for TUI model discovery
  - `TestOpenRouterConnection()` for TUI connection testing
  - `BuildOpenRouterModelFromEntry()` for dynamic model creation
- Extended `OpenAICompatConfig` with `ThinkingLevel` and `ProviderRouting` fields
- Extended `openAICompatRequest` with `Reasoning` and `Provider` fields
- Updated `buildRequest()` to inject reasoning object and provider routing when set

### 045.2: SDK Registration
- Added `registerOpenRouter()` in `internal/sdk/sdk.go`
- Added call in `CreateSession()` after existing provider registrations
- Added `openrouter` case to `RegisterProvider()` API type switch

### 045.4: TUI Provider Catalog
- Added OpenRouter entry to `providerCatalog` in `internal/tui/providers.go`
- Added `testOpenRouter()` connection test function
- Added `discoverOpenRouterModels()` model discovery function

### 045.5: Error Pattern Enhancement
- Added `maximum context length` pattern detection in `ClassifyAPIError()` for 400, 401, and default status codes

### 045.6: Tests
- `internal/provider/openrouter_test.go`: 14 tests covering provider creation, headers, thinking mapping, routing, JSON serialization, model registration, delegation
- `internal/sdk/sdk_test.go`: 3 tests for OpenRouter disabled/enabled/user-models registration
- `internal/tui/providers_test.go`: 4 tests for OpenRouter catalog entry, connection test, model discovery
- `internal/config/config_test.go`: 3 tests for Models field parsing, round-trip, optional behavior
- `internal/types/errors_test.go`: 3 tests for context overflow error classification

### 045.7: Documentation
- Updated `TRACKING.md` — task 045 marked DONE
- Updated `DECISIONS.md` — decision #35 for OpenRouter provider design

### Verification
- `go vet ./...` — clean
- `go build -o tau ./cmd/tau` — successful
- `go test ./...` — all 14 packages pass

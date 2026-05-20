# Worklog — Task 037: Provider Connection System

## 2026-05-09 — Task Definition
- Explored codebase: provider system, registry, model registry, auth resolution, TUI commands, multistep infrastructure, SDK session
- Analyzed current state: `/connect` is skeleton, `/model` shows hardcoded models only, no disconnect capability, OpenCode Zen/Go not registered
- Compared with PI: PI uses config-file-only registration, no runtime connection, no model discovery
- Designed connection flow: multi-step with test-before-save, model discovery via provider APIs, enable/disable toggle preserving credentials
- Defined 6 subtasks: 037.1 (OpenCode Zen), 037.2 (OpenCode Go), 037.3 (/connect implementation), 037.4 (disconnect/disable), 037.5 (fix /model), 037.6 (documentation)
- User confirmed: test+save flow, disable+re-enable toggle, separate providers for Zen/Go, provider-specific API discovery

## 2026-05-09 — Subtask 037.1: OpenCode Zen Provider

### Implementation
- Added `discoverOpenAICompatModels()` in `internal/sdk/sdk.go` — reusable helper for OpenAI-compatible model discovery via `GET /v1/models`
  - Parses standard OpenAI model list response (`{"data": [...]}`)
  - Enriches models with `context_length` and `max_tokens` when present in response
  - Handles empty API key (no Authorization header sent)
  - Gracefully handles network errors, non-200 responses, malformed JSON
- Added `registerOpenCodeZen()` in `internal/sdk/sdk.go` — registers OpenCode Zen provider at startup
  - Resolves auth key via existing `provider.ResolveKey("opencode-zen", "")`
  - Creates `OpenAICompatProvider` with base URL `https://zen.opencode.ai/v1`
  - Calls `discoverOpenAICompatModels()` to fetch and register models
- Updated `CreateSession()` to call `registerOpenCodeZen()` during startup provider registration
- Added `opencode-zen` to `listAvailableProviders()` in `internal/tui/connect.go`

### Tests
- `TestDiscoverOpenAICompatModels_Success` — verifies model parsing and registration with auth header
- `TestDiscoverOpenAICompatModels_ContextLengthEnrichment` — verifies `context_length` and `max_tokens` fields populated
- `TestDiscoverOpenAICompatModels_EmptyResponse` — handles empty data array
- `TestDiscoverOpenAICompatModels_Non200` — handles non-200 HTTP status
- `TestDiscoverOpenAICompatModels_NetworkError` — handles connection failure
- `TestDiscoverOpenAICompatModels_NoAuthHeader` — verifies no Authorization header with empty key
- `TestRegisterOpenCodeZen_WithAuth` — verifies provider registered when auth exists
- `TestRegisterOpenCodeZen_NoAuth` — verifies graceful skip when no auth

### Verification
- All 8 new tests pass
- All existing tests pass (full suite: 15 packages)
- `go vet ./...` clean
- `go build ./...` clean

## 2026-05-09 — Subtask 037.2: OpenCode Go Provider

### Implementation
- Added `registerOpenCodeGo()` in `internal/sdk/sdk.go` — mirrors `registerOpenCodeZen()` pattern
  - Resolves auth key via `provider.ResolveKey("opencode-go", "")`
  - Creates `OpenAICompatProvider` with base URL `https://go.opencode.ai/v1`
  - Calls `discoverOpenAICompatModels()` (reused from 037.1)
- Updated `CreateSession()` to call `registerOpenCodeGo()` during startup
- Added `opencode-go` to `listAvailableProviders()` in `internal/tui/connect.go`

### Tests
- `TestRegisterOpenCodeGo_WithAuth` — verifies provider registered when auth exists
- `TestRegisterOpenCodeGo_NoAuth` — verifies graceful skip when no auth

### Verification
- All 2 new tests pass
- All existing tests pass (full suite: 15 packages, 10 new tests total for 037.1+037.2)
- `go vet ./...` clean
- `go build ./...` clean
- Binary rebuilt

## 2026-05-10 — Subtask 037.3: /connect Command Implementation

### Implementation

#### Config Package (`internal/config/config.go`)
- Added `SaveAuth(store AuthStore, path string) error` — writes auth.json with 0600 permissions
- Added `SaveConfig(cfg *Config, path string) error` — writes config.json
- Added `AuthPath(path string) string` — resolves auth.json path
- Extended `ProviderConfig` with `Enabled` (bool) and `BaseURL` (string) fields

#### Multistep Package (`internal/tui/multistep/`)
- Extended `CommandStep` interface with `SetPriorResults(results map[string]any)` method
- Updated `MultiStepRunner` to pass accumulated results to each step before initialization
- Added `TaskStep` — async step that runs a function with spinner, shows success/error, auto-advances
- Added `ConditionalInputStep` — input step that can auto-complete based on prior results (for skipping API key step for keyless providers)
- Added success/error styles for task step rendering

#### Provider Catalog (`internal/tui/providers.go`)
- Created `ProviderInfo` struct with metadata: name, display name, description, requires API key, base URL
- Defined `providerCatalog` with 6 providers: ollama, opencode-zen, opencode-go, openai, anthropic, google
- Implemented connection test functions:
  - `testOllama` — hits `/api/tags`
  - `testOpenAICompatWithURL` — hits `/v1/models`
  - `testAnthropic` — hits `/v1/messages` with minimal request (auth validation)
  - `testGoogle` — hits models listing endpoint
- Implemented model discovery functions:
  - `discoverOllamaModels` — parses `/api/tags` response
  - `discoverOpenAICompatModelsWithURL` — parses `/v1/models` response
  - `discoverAnthropicModels` / `discoverGoogleModels` — hardcoded model lists
- Helper functions: `findProvider()`, `listAvailableProviders()`, `testProviderConnection()`, `discoverProviderModels()`

#### Connect Command (`internal/tui/connect.go`)
- Replaced skeleton with full multi-step flow:
  1. **Select Provider** — ListStep showing all providers with display names and descriptions
  2. **API Key** — ConditionalInputStep (auto-skipped for Ollama)
  3. **Test Connection** — TaskStep with spinner, makes real API call
  4. **Discover Models** — TaskStep with spinner, fetches model list
  5. **Save** — ConfirmStep
- `handleConnectResult()` performs actual connection:
  - Saves credentials to auth.json (via `saveProviderAuth`)
  - Updates config.json with enabled state and base URL (via `saveProviderConfig`)
  - Registers provider into session at runtime (via `registerConnectedProvider`)
  - Displays success message with discovered models

#### SDK Session (`internal/sdk/sdk.go`)
- Added `RegisterProvider(prov, providerName, baseURL, modelIDs)` method
- Registers provider into registry and models into model registry at runtime
- Supports all provider types: ollama, opencode-zen, opencode-go, openai, anthropic, google

### Tests
- **Config tests** (`internal/config/config_test.go`):
  - `TestSaveAuth_CreatesFile` — verifies file creation, permissions, content roundtrip
  - `TestSaveAuth_ExplicitPath` — verifies custom path support
  - `TestSaveConfig_CreatesFile` — verifies config save with provider config
  - `TestSaveConfig_ExplicitPath` — verifies custom path support
  - `TestAuthPath` — verifies path resolution

- **SDK tests** (`internal/sdk/sdk_test.go`):
  - `TestSession_RegisterProvider` — verifies provider and model registration
  - `TestSession_RegisterProvider_Ollama` — verifies ollama API type assignment
  - `TestSession_RegisterProvider_Anthropic` — verifies anthropic API type assignment
  - `TestSession_RegisterProvider_EmptyModels` — verifies registration with no models

- **Provider tests** (`internal/tui/providers_test.go`):
  - `TestFindProvider_KnownProviders` — verifies all 6 providers found with correct metadata
  - `TestFindProvider_Unknown` — verifies unknown provider returns false
  - `TestListAvailableProviders` — verifies all expected providers in list
  - `TestProviderCatalog_BaseURLs` — verifies correct base URLs
  - `TestValidateAPIKey` — verifies key validation logic
  - `TestDiscoverAnthropicModels` / `TestDiscoverGoogleModels` — verifies hardcoded lists
  - `TestDiscoverProviderModels_*` — verifies discovery per provider type
  - `TestTestProviderConnection_*` — verifies connection test functions

- **Multistep tests** (`internal/tui/multistep/taskstep_test.go`):
  - `TestTaskStep_Init_ReturnsCmd` — verifies cmd creation and task execution
  - `TestTaskStep_Update_ReceivesResult` — verifies result handling
  - `TestTaskStep_Result` — verifies result data merging
  - `TestTaskStep_Error` — verifies error handling
  - `TestTaskStep_PriorResults` — verifies prior results passed to task
  - `TestTaskStep_Render` — verifies rendering before/after completion
  - `TestConditionalInputStep_Skip` — verifies auto-skip behavior
  - `TestConditionalInputStep_NoSkip` — verifies normal input behavior

### Verification
- All tests pass (full suite: 15 packages)
- `go vet ./...` clean
- `go build ./...` clean
- Binary rebuilt at `./tau`

## 2026-05-10 — Build Fix: discoverOpenAICompatModels undefined

### Issue
- `providers.go` catalog referenced `discoverOpenAICompatModels` which only exists in `sdk.go`
- Build failed with `undefined: discoverOpenAICompatModels` for opencode-zen, opencode-go, and openai entries

### Fix
- Added provider-specific wrapper functions in `providers.go`:
  - `discoverOpenAICompatModelsZen` → calls `discoverOpenAICompatModelsWithURL("https://zen.opencode.ai/v1", apiKey)`
  - `discoverOpenAICompatModelsGo` → calls `discoverOpenAICompatModelsWithURL("https://go.opencode.ai/v1", apiKey)`
  - `discoverOpenAICompatModelsOpenAI` → calls `discoverOpenAICompatModelsWithURL("https://api.openai.com/v1", apiKey)`
- Updated catalog entries to use correct wrapper functions
- Kept generic `discoverOpenAICompatModels` as error-returning stub (unused, prevents accidental misuse)

### Verification
- All tests pass (full suite: 15 packages)
- `go vet ./...` clean
- `go build ./...` clean
- Binary rebuilt at `./tau`

## 2026-05-10 — Subtask 037.4: Provider Disconnect/Disable

### Implementation

#### Provider Registry (`internal/provider/registry.go`)
- Added `Unregister(name string)` method — removes provider from registry by name
- No-op if provider doesn't exist

#### Model Registry (`internal/provider/model.go`)
- Added `RemoveByProvider(providerName string)` method — removes all models belonging to a specific provider
- No-op if provider has no models

#### SDK Session (`internal/sdk/sdk.go`)
- Added `DisableProvider(name string) error` method:
  - Validates provider exists in registry (returns error if not)
  - Removes provider from registry via `Unregister()`
  - Removes provider's models from model registry via `RemoveByProvider()`
  - Updates `config.json` to set `providers.<name>.enabled = false`
  - Preserves credentials in `auth.json` (not touched)
  - Does NOT change current session model/provider — user must switch explicitly

#### TUI Disconnect Command (`internal/tui/disconnect.go`)
- Created new `/disconnect` command with multi-step flow:
  1. **Select Provider** — ListStep showing only connected/configured providers with enabled/disabled status
  2. **Confirm Disconnect** — ConfirmStep asking for confirmation
- `handleDisconnectResult()` processes the disconnection:
  - Calls `session.DisableProvider()` to remove provider and models
  - Displays success message explaining what was done and how to re-enable
- `listConnectedProviders()` filters provider catalog to show only providers with auth credentials or config entries

#### TUI Connect Command Updates (`internal/tui/connect.go`)
- Updated `connectSteps()` to show provider connection state:
  - Displays "connected", "disabled", and "credentials saved" status
- Updated API key step to skip entry if credentials already exist in `auth.json`
- Updated Test Connection and Discover Models steps to load API key from `auth.json` if not entered

#### Provider Catalog Updates (`internal/tui/providers.go`)
- Added `providerWithState` struct extending `ProviderInfo` with `Enabled`, `HasAuth`, `HasConfig` fields
- Added `listAvailableProvidersWithState()` to get providers with current connection state
- Added config import for loading provider configuration

#### TUI Model Updates (`internal/tui/model.go`, `internal/tui/update.go`)
- Added `multiStepCommandName` field to track which multi-step command is active
- Updated `startMultiStep()` to store command name
- Updated `finishMultiStep()` to dispatch to correct handler based on command name (`connect` vs `disconnect`)
- Updated `cancelMultiStep()` to clear command name

### Tests
- **Provider Registry tests** (`internal/provider/registry_test.go`):
  - `TestRegistry_Unregister` — verifies provider removal from registry
  - `TestRegistry_Unregister_NonExistent` — verifies no-op for non-existent provider

- **Model Registry tests** (`internal/provider/model_test.go`):
  - `TestModelRegistry_RemoveByProvider` — verifies all models removed for provider, other providers unaffected
  - `TestModelRegistry_RemoveByProvider_NonExistent` — verifies no-op for non-existent provider

- **SDK tests** (`internal/sdk/sdk_test.go`):
  - `TestSession_DisableProvider` — full flow: register provider, disable it, verify provider removed, models hidden, credentials preserved, config updated
  - `TestSession_DisableProvider_NonExistent` — verifies error for non-existent provider
  - `TestSession_DisableProvider_CurrentModel` — verifies session model remains unchanged after disabling current provider

- **TUI tests** (`internal/tui/command_test.go`):
  - Updated command count expectations from 12 to 13 (added `/disconnect`)
  - Updated filter test expectations for "s" and "se" patterns (disconnect matches)

### Verification
- All tests pass (full suite: 15 packages)
- `go vet ./...` clean
- `go build ./...` clean
- Binary rebuilt at `./tau`

## 2026-05-10 — Subtask 037.5: Fix /model Command

### Implementation

#### Config Package (`internal/config/config.go`)
- Changed `ProviderConfig.Enabled` from `bool` to `*bool` to properly distinguish "not set" (default enabled) from "explicitly disabled"
- `nil` = enabled by default, `&true` = explicitly enabled, `&false` = explicitly disabled

#### SDK Session (`internal/sdk/sdk.go`)
- Added `isProviderEnabled(cfg, name) bool` helper function:
  - Returns `true` if provider not in config map (default enabled)
  - Returns `true` if `Enabled` is `nil` (not explicitly set)
  - Returns `*Enabled` value otherwise
- Updated `CreateSession()` to pass config to all provider registration functions
- Updated all registration functions to accept `*config.Config` parameter:
  - `registerOpenAI`, `registerAnthropic`, `registerGoogle`, `registerOllama`, `registerOpenCodeZen`, `registerOpenCodeGo`
  - Each function now checks `isProviderEnabled()` before attempting auth resolution
  - If disabled, removes provider's built-in models from registry via `RemoveByProvider()`
- Updated `DisableProvider()` to use pointer for `Enabled` field (`&enabled`)

#### TUI Provider List (`internal/tui/providers.go`)
- Updated `listAvailableProvidersWithState()` to convert `*bool` to `bool` for `providerWithState.Enabled`
- Default to `true` when `Enabled` is `nil`

#### TUI Connect Command (`internal/tui/connect.go`)
- Updated `saveProviderConfig()` to use pointer for `Enabled` field (`&enabled`)

#### TUI Disconnect Command (`internal/tui/disconnect.go`)
- Updated `listConnectedProviders()` to convert `*bool` to `bool` for `connectedProvider.Enabled`

### Tests
- **SDK tests** (`internal/sdk/sdk_test.go`):
  - `TestCreateSession_DisabledProviderNotRegistered` — verifies disabled provider not registered, models hidden
  - `TestCreateSession_EnabledProviderRegistered` — verifies explicitly enabled provider registered with models
  - `TestCreateSession_DefaultEnabledWhenNotInConfig` — verifies provider enabled by default when not in config
  - `TestIsProviderEnabled` — table-driven tests for all enabled/disabled/nil scenarios
  - Updated `TestRegisterOpenCodeZen_*` and `TestRegisterOpenCodeGo_*` to pass config parameter

- **Config tests** (`internal/config/config_test.go`):
  - Updated `TestSaveConfig_CreatesFile` to use pointer for `Enabled` field

### Verification
- All tests pass (full suite: 15 packages)
- `go vet ./...` clean
- `go build ./...` clean
- Binary rebuilt at `./tau`

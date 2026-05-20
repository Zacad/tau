# Task 037: Provider Connection System

## Why

Currently the `/connect` command is a skeleton proof-of-concept that collects provider + API key but doesn't actually test connections or save credentials. The `/model` command shows only hardcoded built-in models, not dynamically discovered models from connected providers. There's no ability to disconnect or toggle providers. OpenCode Zen and OpenCode Go providers are not registered at all.

Users need a proper connection flow to add providers at runtime, see all available models from active providers, and manage provider connections without restarting tau.

## Comparison with PI

| Feature | PI | Tau (current) | Tau (target) |
|---------|-----|---------------|--------------|
| Provider registration | Config file at startup | Config file + auto-discovery at startup | Runtime `/connect` + startup |
| Model discovery | Config-defined models | Hardcoded built-in + Ollama auto-discovery | Provider-specific API discovery |
| Connection testing | No explicit test | None | Test connection before saving |
| Credential storage | Config file | auth.json (existing) | auth.json (extended) |
| Provider toggle | Not applicable | Not available | Disable/re-enable with preserved credentials |
| Model selector | TUI picker with all models | TUI picker with hardcoded models only | TUI picker with all accessible models + provider info |

## Constraints

- Must use existing `auth.json` format (backward compatible)
- Must use existing `~/.tau/config.json` for provider enable/disable state
- Provider registry must support enable/disable toggle without removing credentials
- Model discovery must be async — don't block TUI while fetching models
- Must work with existing multi-step command infrastructure (`multistep` package)
- Ollama doesn't require API key — connect flow must handle keyless providers
- OpenCode Zen and OpenCode Go both use OpenAI-compatible API but with different base URLs

## Design

### Provider Connection State

Provider connection state tracked in two places:
- **Credentials**: `~/.tau/auth.json` — API keys (existing format, extended with new provider names)
- **Enable/disable**: `~/.tau/config.json` — `providers.<name>.enabled` boolean

```json
// ~/.tau/config.json (new section)
{
  "providers": {
    "opencode-zen": {
      "enabled": true,
      "base_url": "https://zen.opencode.ai/v1"
    },
    "opencode-go": {
      "enabled": true,
      "base_url": "https://go.opencode.ai/v1"
    },
    "ollama": {
      "enabled": true
    }
  }
}
```

### Model Discovery

Each provider type has its own discovery method:
- **Ollama**: `GET /api/tags` (existing, already implemented in `registerOllama`)
- **OpenAI-compat** (OpenCode Zen, OpenCode Go, OpenRouter): `GET /v1/models`
- **OpenAI**: `GET /v1/models` (requires API key)
- **Anthropic**: No public model listing API — use hardcoded list
- **Google**: No public model listing API — use hardcoded list

Discovered models registered into `ModelRegistry` with `Provider` field set.

### /connect Flow

Multi-step flow using existing `multistep` infrastructure:
1. **Select Provider** — ListStep showing all connectable providers (including already-connected ones for re-connect)
2. **Enter API Key** — InputStep (skipped for Ollama)
3. **Test Connection** — MessageStep with spinner, calls provider to verify connectivity
4. **Discover Models** — MessageStep with spinner, fetches model list from provider API
5. **Save Credentials** — ConfirmStep, writes to auth.json, updates config.json enabled state
6. **Register Provider** — Registers provider instance, adds discovered models to registry

### Disconnect/Disable Flow

- Toggle `providers.<name>.enabled` in config.json to `false`
- Remove provider from registry (unregister)
- Remove provider's models from model registry
- Credentials preserved in auth.json for quick re-enable via `/connect`

### /model Command Fix

- Show all models from **enabled** providers only
- Display format: `model-id  •  provider-name  •  context-window`
- Pre-select current active model
- Filter/search works across all models

## Subtasks

### 037.1: OpenCode Zen Provider
- Register OpenCode Zen as a named provider (`opencode-zen`)
- Use OpenAICompatProvider with base URL `https://zen.opencode.ai/v1`
- Implement model discovery via `GET /v1/models` endpoint
- Add to provider list in `/connect` command
- Register at startup if credentials exist and enabled in config

### 037.2: OpenCode Go Provider
- Register OpenCode Go as a named provider (`opencode-go`)
- Use OpenAICompatProvider with base URL `https://go.opencode.ai/v1`
- Implement model discovery via `GET /v1/models` endpoint
- Add to provider list in `/connect` command
- Register at startup if credentials exist and enabled in config

### 037.3: /connect Command Implementation
- Replace skeleton `connect.go` with full implementation
- Multi-step flow: Select Provider → Enter API Key → Test Connection → Discover Models → Save → Register
- Handle keyless providers (Ollama) — skip API key step
- Test connection by making a minimal API call
- Discover models via provider-specific API
- Save credentials to `auth.json` (existing format)
- Update `config.json` with enabled state and base URL
- Register provider instance and discovered models into registry
- Update SDK Session to support runtime provider registration

### 037.4: Provider Disconnect/Disable ✅
- Add provider enable/disable toggle to config loading
- Add `DisableProvider(name string)` to SDK Session
- Remove provider from registry and its models from model registry
- Preserve credentials in auth.json
- Add `/disconnect` command or integrate into `/connect` (re-connect flow)
- TUI: show disabled providers differently in connect list

### 037.5: Fix /model Command
- Update `ListModels()` to filter by enabled providers only
- Update model selector to show provider name for each model
- Ensure model resolution works with newly connected providers
- Update `cmdModel` handler to refresh model list from current registry state
- Verify model switching works after connecting new providers

### 037.6: Documentation
- Document provider connection pattern in ARCHITECTURE.md
- Document model discovery pattern for new providers
- Add DECISIONS.md entry for provider connection design
- Update TRACKING.md

## Acceptance Criteria

### Main AC
1. `/connect` command successfully connects to Ollama, OpenCode Zen, and OpenCode Go providers
2. Connection is tested before saving — user sees success/failure feedback
3. Credentials saved to `~/.tau/auth.json` in existing format
4. Provider enabled state saved to `~/.tau/config.json`
5. Models discovered from connected providers appear in `/model` selector
6. Provider can be disabled — removed from registry, models hidden, credentials preserved
7. Disabled provider can be re-enabled via `/connect` without re-entering credentials
8. `/model` shows provider name for each model in the list
9. Model switching works correctly after connecting/disconnecting providers
10. All existing tests pass, new tests added for connection flow

### Subtask ACs

#### 037.1 AC
- OpenCode Zen provider registered at startup when credentials exist
- Model discovery returns models from `GET /v1/models`
- Models appear in `/model` selector with `opencode-zen` provider label
- E2E test with Ollama verifies OpenAI-compat model discovery pattern

#### 037.2 AC
- OpenCode Go provider registered at startup when credentials exist
- Model discovery returns models from `GET /v1/models`
- Models appear in `/model` selector with `opencode-go` provider label

#### 037.3 AC
- `/connect` flow completes all 6 steps successfully
- API key step skipped for Ollama
- Connection test makes real API call and reports result
- Model discovery fetches and registers models
- Credentials persisted to auth.json
- Config updated with enabled state
- Provider registered and usable immediately after connect

#### 037.4 AC
- Provider can be disabled via command
- Disabled provider removed from registry
- Models from disabled provider hidden from `/model`
- Credentials remain in auth.json
- Re-connect skips API key entry if credentials exist

#### 037.5 AC
- `/model` shows all models from enabled providers
- Each model shows provider name in selector
- Current model pre-selected
- Model switching works after runtime provider changes
- Model resolution uses updated registry

#### 037.6 AC
- ARCHITECTURE.md updated with provider connection section
- DECISIONS.md has entry for connection design
- Pattern documented for adding future providers

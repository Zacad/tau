# Fix /connect Model Discovery — Wrong BaseURL

## Why
After `/connect` completes successfully for `opencode-go`, discovered models don't appear in `/model`. The provider registers but shows 0 models. Root cause: **BaseURL is wrong for both `opencode-go` and `opencode-zen` providers**.

## Proven Facts (No Assumptions)

### 1. BaseURL is incorrect
| Provider | Our Code | Correct (from Catwalk) | Verified |
|---|---|---|---|
| `opencode-go` | `https://go.opencode.ai/v1` | `https://opencode.ai/zen/go/v1` | DNS fails on our URL, correct URL returns 15 models |
| `opencode-zen` | `https://zen.opencode.ai/v1` | `https://opencode.ai/zen/v1` | Not yet tested but same pattern |

**Evidence:**
```
$ curl https://go.opencode.ai/v1/models
Could not resolve host: go.opencode.ai

$ curl -H "Authorization: Bearer $KEY" https://opencode.ai/zen/go/v1/models
{"data": [{"id": "minimax-m2.7"}, {"id": "minimax-m2.5"}, {"id": "kimi-k2.6"}, ...]}
# Returns 15 models successfully
```

Source: `https://catwalk.charm.land/v2/providers` (official Crush/Catwalk provider database)

### 2. Multi-step flow is correct
- `TestPalette_MultiStepFlow_ConnectSimulated` passes — `discover_models` propagates as `[]string` through the full flow
- `TestSession_RegisterProvider` passes — `RegisterProvider` → `ListModels` works end-to-end
- `HandleMultiStepDone()` correctly stores metadata (`_ok`, `_msg`, `_err`) without overwriting task data
- Task function stores `results["discover_models"] = models` directly; this persists through flow completion

### 3. `/connect` does verify connectivity
- Step 3 ("Test Connection") calls `testProviderConnection(info, apiKey)` which makes an HTTP GET to `{baseURL}/models`
- Step 4 ("Discover Models") calls `discoverProviderModels(info, apiKey)` which also hits `{baseURL}/models`
- Both use the same wrong BaseURL, so both would fail silently (return 0 models on non-200/network error)

### 4. Provider name consistency is correct
- `ProviderInfo.Name` = `"opencode-go"` matches `OpenAICompatConfig.ProviderName` = `"opencode-go"`
- `registry.Register()` uses `prov.Name()` which returns the config's `ProviderName`
- `ListModels()` filters by `m.Provider` which matches the registered provider name
- `TestRegisterOpenCodeGo_WithAuth` passes — provider name is `"opencode-go"`

### 5. Code locations with wrong URLs
- `internal/tui/providers.go:61` — `opencode-go` BaseURL: `https://go.opencode.ai/v1`
- `internal/tui/providers.go:51` — `opencode-zen` BaseURL: `https://zen.opencode.ai/v1`
- `internal/sdk/sdk.go:931` — `registerOpenCodeZen` BaseURL: `https://zen.opencode.ai/v1`
- `internal/sdk/sdk.go:955` — `registerOpenCodeGo` BaseURL: `https://go.opencode.ai/v1`
- `internal/tui/providers.go:211` — `testOpenAICompatZen` URL: `https://zen.opencode.ai/v1`
- `internal/tui/providers.go:215` — `testOpenAICompatGo` URL: `https://go.opencode.ai/v1`
- `internal/tui/providers.go:372` — `discoverOpenAICompatModelsZen` URL: `https://zen.opencode.ai/v1`
- `internal/tui/providers.go:376` — `discoverOpenAICompatModelsGo` URL: `https://go.opencode.ai/v1`

## Constraints
- Must not break existing working providers (ollama, openai, anthropic, google)
- Must update both startup registration (`sdk.go`) and runtime connection (`providers.go`)
- Debug logging added in previous session should be cleaned up or kept at Debug level

## Acceptance Criteria
1. `/connect opencode-go` discovers and displays all 15 models in `/model`
2. `/connect opencode-zen` discovers and displays models in `/model`
3. Startup auto-registration (`CreateSession`) works with correct URLs
4. All existing tests pass
5. No regressions in other providers

## Subtasks
- [x] Fix BaseURL in `providers.go` provider catalog (opencode-go, opencode-zen)
- [x] Fix BaseURL in `providers.go` test functions (testOpenAICompatZen, testOpenAICompatGo)
- [x] Fix BaseURL in `providers.go` discovery functions (discoverOpenAICompatModelsZen, discoverOpenAICompatModelsGo)
- [x] Fix BaseURL in `sdk.go` register functions (registerOpenCodeZen, registerOpenCodeGo)
- [x] Verify `opencode-zen` endpoint works: `curl https://opencode.ai/zen/v1/models`
- [x] Run all tests
- [x] Manual test: `/connect` → select opencode-go → paste key → verify models appear in `/model`
- [x] Clean up debug logging from previous session (keep at slog.Debug level)
- [x] Fix error classification: disabled models return HTTP 401 with "Model is disabled" message, misclassified as auth_failed. Added `ErrorTypeModelUnavailable` and check model keywords before defaulting to auth_failed for 401/403.

## Findings
- `glm-5` is disabled on opencode-go API but still listed in `/models` and Catwalk
- `glm-5.1` works correctly
- API returns `{"type":"error","error":{"type":"ModelError","message":"Model is disabled"}}` with HTTP 401 for disabled models
- Error was misclassified as "Authentication failed" — now shows "Model unavailable: Model is disabled. Try selecting a different model."

## Reference
- Catwalk provider database: `https://catwalk.charm.land/v2/providers`
- Crush (successor to opencode): `https://github.com/charmbracelet/crush`
- OpenCode Go models confirmed: minimax-m2.7, minimax-m2.5, kimi-k2.6, kimi-k2.5, glm-5.1, glm-5, deepseek-v4-pro, deepseek-v4-flash, mimo-v2-pro, mimo-v2-omni, coder-model, qwen3-235b, qwen3-30b, codestral-latest, plus more

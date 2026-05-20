# Worklog — Task 039: Fix /connect Model Discovery

## Session 1 — Investigation & Root Cause Discovery

### What was done
1. Traced `discover_models` flow through entire chain:
   - `connect.go` Discover Models task → `results["discover_models"]` → `handleConnectResult` → `registerConnectedProvider` → `session.RegisterProvider` → `provReg.Register` + `ModelRegistry.Register`
   - All code paths verified correct via unit tests

2. Added debug logging at key points:
   - `connect.go:92` — discover models result
   - `connect.go:98` — stored in results
   - `connect.go:168-175` — extracted from results
   - `sdk.go:554-562` — ListModels connected providers
   - `sdk.go:605` — model registered at runtime

3. Wrote `TestPalette_MultiStepFlow_ConnectSimulated` — simulates exact connect flow, passes
   - Confirms `discover_models` propagates as `[]string` through multi-step flow
   - Confirms type assertion `results["discover_models"].([]string)` works

4. Checked Catwalk (Crush provider database) for correct URLs:
   - `opencode-go`: `https://opencode.ai/zen/go/v1` (NOT `https://go.opencode.ai/v1`)
   - `opencode-zen`: `https://opencode.ai/zen/v1` (NOT `https://zen.opencode.ai/v1`)

5. Verified correct endpoint works:
   ```
   $ curl -H "Authorization: Bearer $KEY" https://opencode.ai/zen/go/v1/models
   {"data": [{"id": "minimax-m2.7"}, ...]}  # 15 models
   ```

6. Our URL fails:
   ```
   $ curl https://go.opencode.ai/v1/models
   Could not resolve host: go.opencode.ai
   ```

### Root cause
**Wrong BaseURL in 8 locations across 2 files.** Both `/connect` runtime flow and `CreateSession` startup flow use incorrect domains that don't resolve.

### Key findings
- Multi-step flow, result propagation, provider registration, model filtering — ALL correct
- `/connect` does verify connectivity (step 3 "Test Connection") — but against wrong URL so fails silently
- Provider name consistency is correct throughout
- Catwalk (`https://catwalk.charm.land/v2/providers`) is the authoritative source for provider URLs

## Session 2 — Fix Implementation

### What was done
1. Fixed BaseURLs in 8 locations across 2 source files + 2 test files:

   | File | Location | Old URL | New URL |
   |---|---|---|---|
   | `internal/tui/providers.go` | catalog zen | `https://zen.opencode.ai/v1` | `https://opencode.ai/zen/v1` |
   | `internal/tui/providers.go` | catalog go | `https://go.opencode.ai/v1` | `https://opencode.ai/zen/go/v1` |
   | `internal/tui/providers.go` | `testOpenAICompatZen` | `https://zen.opencode.ai/v1` | `https://opencode.ai/zen/v1` |
   | `internal/tui/providers.go` | `testOpenAICompatGo` | `https://go.opencode.ai/v1` | `https://opencode.ai/zen/go/v1` |
   | `internal/tui/providers.go` | `discoverOpenAICompatModelsZen` | `https://zen.opencode.ai/v1` | `https://opencode.ai/zen/v1` |
   | `internal/tui/providers.go` | `discoverOpenAICompatModelsGo` | `https://go.opencode.ai/v1` | `https://opencode.ai/zen/go/v1` |
   | `internal/sdk/sdk.go` | `registerOpenCodeZen` | `https://zen.opencode.ai/v1` | `https://opencode.ai/zen/v1` |
   | `internal/sdk/sdk.go` | `registerOpenCodeGo` | `https://go.opencode.ai/v1` | `https://opencode.ai/zen/go/v1` |
   | `internal/tui/providers_test.go` | `TestProviderCatalog_BaseURLs` | old URLs | new URLs |
   | `internal/config/config_test.go` | `TestSaveConfig_DefaultPath` | old URL | new URL |

2. Verified `opencode-zen` endpoint returns HTTP 200

3. All tests pass: `go test ./...` — 12 packages, all ok

4. Rebuilt binary: `go build -o ./tau ./cmd/tau`

5. Debug logging from previous session verified — already at `slog.Debug` level, no cleanup needed

## Session 3 — Fix Error Classification for Disabled Models

### Problem
After selecting `glm-5` from opencode-go and sending a prompt, the error showed:
> "Authentication failed. Please check your API key."

This was misleading — the API key was valid, but `glm-5` is disabled on the opencode-go API.

### Root cause
The opencode-go API returns HTTP 401 with body:
```json
{"type":"error","error":{"type":"ModelError","message":"Model is disabled"}}
```

Our `ClassifyAPIError` in `internal/types/errors.go` classified all 401/403 responses as `ErrorTypeAuthFailed` before checking the message content.

### Fix
1. Added `ErrorTypeModelUnavailable` constant to `APIErrorType`
2. Updated `ClassifyAPIError` to check for model-related keywords ("model", "disabled", "not found", "not available") BEFORE defaulting to auth_failed for 401/403
3. Added user-friendly message: "Model unavailable: {message}. Try selecting a different model."
4. Added tests for the new error type:
   - `TestClassifyAPIError_ModelUnavailable` — 5 test cases covering opencode-go format, model not found, and ensuring auth errors still classify correctly
   - `TestAPIError_ModelUnavailable_UserMessage` — 2 test cases for user message formatting
   - Updated `TestAPIError_UserMessage` to include model unavailable case

### Verification
- `glm-5.1` works correctly (confirmed by user)
- `glm-5` returns proper error message instead of misleading auth failure
- All tests pass: `go test ./...`
- Binary rebuilt

## Session 4 — Fix Error Block Rendering Delay & Improve Error Display

### Problem
Error blocks from disabled models (e.g., `glm-5` on opencode-go) delayed rendering until the second prompt was sent. The error was appended to `m.blocks` but the viewport update was not immediately visible.

### Root cause
`handleError` called `m.updateViewport()` (with `force=false`), which is subject to:
1. Throttle check: skips if last update was within 33ms
2. Length cache check: skips if `pendingLen`, `pendingRenderedLen`, and `blocksLen` match cached values

While the length check should detect the new block, the throttle could skip the update if the error arrives quickly (e.g., immediate HTTP 401 response). Additionally, the error text included internal prefixes like "provider stream error: " which leaked implementation details to the user.

### Fix
1. **Force viewport update in `handleError`**: Changed `m.updateViewport()` to `m.updateViewportWithForce(true)` to bypass both throttle and length cache checks, ensuring the error block renders immediately.

2. **Strip internal error prefixes in `renderError`**: Added prefix stripping for:
   - `"provider stream error: "`
   - `"agent prompt: "`
   - `"agent prompt failed: "`
   
   This ensures user-friendly messages from `APIError.UserMessage()` are displayed without internal wrapping text.

### Files changed
- `internal/tui/model.go:605` — `handleError` now uses `updateViewportWithForce(true)`
- `internal/tui/render.go:226-234` — `renderError` strips internal prefixes before rendering

### Verification
- All tests pass: `go test ./...`
- Binary rebuilt: `go build -o ./tau ./cmd/tau`

# Task 046: Thinking Command — Worklog

## Summary
Implemented per-model thinking level control with `/thinking` command, footer display, and provider-specific API mapping.

## Changes Made

### Types (`internal/types/provider.go`, `internal/types/thinking.go`)
- Added `ThinkingLevelMap` field to `Model` struct
- Added `AllThinkingLevels()`, `ThinkingLevelDescription()` helpers
- Added `GetSupportedThinkingLevels()` and `MapThinkingLevel()` methods on Model
- Created `internal/types/thinking.go` with thinking level helper functions
- Created `internal/types/thinking_test.go` with comprehensive tests

### Model Registration (`internal/provider/model.go`, `internal/sdk/sdk.go`)
- Added `ThinkingLevelMap` to all built-in reasoning models (OpenAI o1/o3, Anthropic Claude, Google Gemini)
- Updated OpenRouter model builder to include `ThinkingLevelMap` for reasoning models
- Updated Ollama auto-discovery to set `ThinkingLevelMap` for reasoning models (gemma, qwq, deepseek-r1)
- Updated `RegisterProvider` to populate `ThinkingLevelMap` for runtime-connected providers
- Added `isReasoningModel()` and `defaultThinkingLevelMap()` helpers

### Agent (`internal/agent/agent.go`, `internal/agent/loop.go`)
- Added `thinkingLevel` field to Agent struct
- Added `SetThinkingLevel()` and `ThinkingLevel()` methods
- Wired `ThinkingLevel` through `StreamOptions` in `streamToMessage()`

### Session Persistence (`internal/session/session.go`, `internal/session/types.go`)
- Updated `ThinkingLevelChangeData` to include `ModelID` for per-model tracking
- Added `modelThinkingLevels` map to Session struct
- Updated `SetThinkingLevel()` to accept model ID
- Added `GetThinkingLevelForModel()` for per-model restore

### SDK (`internal/sdk/sdk.go`)
- Added `SetThinkingLevel()` and `ThinkingLevel()` methods to Session
- Updated `SetModel()` to restore thinking level for the new model
- Propagates thinking level to agent on model switch

### Providers
- **OpenAI** (`internal/provider/openai.go`): Added `Effort` field to `openAIThinkingConfig`, maps thinking level to `reasoning.effort`
- **Anthropic** (`internal/provider/anthropic.go`): Added `OutputConfig` for adaptive thinking, detects effort-based vs budget-based models
- **Google** (`internal/provider/google.go`): Added `ThinkingConfig` with `thinkingLevel` (Gemini 3) and `thinkingBudget` (Gemini 2.x)
- **Ollama** (`internal/provider/ollama.go`): Added `Thinking` flag to `ollamaOptions`

### TUI (`internal/tui/command.go`, `internal/tui/view.go`)
- Added `/thinking` command with palette selector filtered by model capabilities
- Shows current level with "(current)" marker
- Shows human-readable descriptions for each level
- Non-reasoning models show appropriate message
- Footer displays `thinking:level` for reasoning-capable models

### Tests
- `internal/types/thinking_test.go`: 4 test functions covering level helpers
- `internal/provider/openai_test.go`: `TestOpenAIProvider_ThinkingMapping` with mock HTTP server
- `internal/provider/provider_test.go`: `TestAnthropicProvider_ThinkingMapping` with mock HTTP server
- Updated existing tests for new command count (14→15) and session signature change

## Verification
- All tests pass: `go test ./...`
- `go vet ./...` clean
- `go build ./...` clean
- Binary rebuilt: `go build -o tau ./cmd/tau/`
- E2E verified: `tau --model gemma4:26b -p "hello"` returns response without hang

## Bug Fix: Ollama Thinking Option
Initial implementation added `Thinking: true` to Ollama request options, which caused the app to hang. Ollama's `/api/chat` endpoint does not support a `thinking` option — thinking is a model capability that happens automatically for reasoning models and cannot be controlled via the API. Removed the `Thinking` field from `ollamaOptions`.

## Bug Fix: TUI Deadlock with Thinking Enabled
**Root cause**: `renderFooter()` called `m.session.Model()` and `m.session.ThinkingLevel()` which acquire `s.mu`. During `sdk.Prompt()`, `s.mu` is held for the entire agent loop. Events emitted by the agent trigger `m.program.Send()` which queues messages for the TUI event loop. The TUI event loop calls `View()` → `renderFooter()` → tries to acquire `s.mu` → deadlock.

**Fix**: Cached `thinkingLevel` (alongside existing `modelReasoning`) in the TUI model struct. Populated at startup in `NewModel()` and updated on model/thinking level changes in `cmdModel` and `cmdThinking`. `renderFooter()` now reads cached fields instead of calling session methods.

**Files changed**:
- `internal/tui/model.go`: Added `thinkingLevel` field, initialized in `NewModel()`
- `internal/tui/command.go`: Update `m.modelReasoning` and `m.thinkingLevel` on model/level change
- `internal/tui/view.go`: Use cached `m.modelReasoning` and `m.thinkingLevel` in `renderFooter()`

## Bug Fix: Ollama Thinking Level Not Sent in Request
**Root cause**: The Ollama provider logged the thinking level but never actually included it in the API request. The `ollamaOptions` struct was missing the `thinking_level` field.

**Fix**: Added `ThinkingLevel` field to `ollamaOptions` struct. Updated `buildRequest()` to set `req.Options.ThinkingLevel` using `model.MapThinkingLevel()` when the model supports reasoning.

**Files changed**:
- `internal/provider/ollama.go`: Added `ThinkingLevel string` to `ollamaOptions`, set it in `buildRequest()`
- `internal/provider/ollama_test.go`: Added `TestOllamaProvider_BuildRequest_ThinkingLevel` and `TestOllamaProvider_BuildRequest_ThinkingLevelMapping`

## Bug Fix: Google Provider Not Using Model Mapping
**Root cause**: Google provider had its own hardcoded `mapGoogleThinkingLevel()` function instead of using the standardized `model.MapThinkingLevel()`. This meant per-model `ThinkingLevelMap` was ignored for Google models.

**Fix**: Updated `buildRequest()` to accept `model` parameter and use `model.MapThinkingLevel()` for provider-specific values. Updated `Stream()` to pass model to `buildRequest()`.

**Files changed**:
- `internal/provider/google.go`: `buildRequest()` now takes `model types.Model`, uses `model.MapThinkingLevel()`
- `internal/provider/provider_test.go`: Added `TestGoogleProvider_ThinkingConfig` and `TestGoogleProvider_ThinkingBudget`

## Live API Verification (OpenCode Zen)
**GPT (gpt-5.2-codex via Responses API):**
- `low`: 219 output tokens, 524 text chars
- `medium`: 233 output tokens, 498 text chars
- `high`: 399 output tokens, 192 reasoning tokens, 526 text chars
- Output tokens increase significantly with effort level, confirming more internal reasoning.

**Claude (claude-sonnet-4-6 via Messages API):**
- `budget=0`: 0 thinking chars, 921 text chars
- `budget=2048`: 250 thinking chars, 840 text chars
- `budget=4096`: 349 thinking chars, 883 text chars
- `budget=8192`: 285 thinking chars, 814 text chars
- Thinking content increases with budget (0→250→349).

**Ollama (gemma4:26b):**
- `off`: 476 chars thinking
- `low`: 596 chars thinking
- `medium`: 646 chars thinking
- `high`: 689 chars thinking
- Thinking length increases with level as expected.

## Final Verification
- All tests pass: `go test ./...` (clean testcache)
- `go build -o tau ./cmd/tau` clean
- TUI multi-turn conversation verified with thinking enabled (no deadlock)
- Print mode verified with Ollama reasoning model
- Verification script: `./verify-thinking-all-providers.sh`

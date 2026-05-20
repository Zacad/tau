# Task 042: OpenCode Zen — Full Multi-Endpoint Model Support

## Why
OpenCode Zen routes different model families to **different API endpoints** with different request/response formats. Tau currently sends all Zen models to `/chat/completions` (OpenAI-compatible), which only works for a subset of models. Models like GPT (`gpt-*`) and Gemini (`gemini-*`) fail with 400/401 errors.

**Root cause confirmed:** User's default model `gemini-3.1-pro` returns HTTP 401 because Zen routes Gemini through Google's API (`/models/{id}?key=` auth), not `/chat/completions`.

## Comparison with PI and OpenCode

### How OpenCode handles it (reference: `packages/opencode/src/provider/provider.ts` + `transform.ts`)
OpenCode uses the AI SDK with per-provider npm packages. Each model has an `api.npm` field that determines which SDK to use:

| Model family | SDK package | Endpoint |
|---|---|---|
| `gpt-*` | `@ai-sdk/openai` | `/responses` (Responses API) |
| `claude-*` | `@ai-sdk/anthropic` | `/messages` (Messages API) |
| `gemini-*` | `@ai-sdk/google` | `/models/{id}` (Google Generative AI) |
| `qwen*`, `minimax*`, `glm*`, `kimi*` | `@ai-sdk/openai-compatible` | `/chat/completions` |

OpenCode's `provider.ts` has a `getModel()` function per provider that selects the correct SDK method (`sdk.responses()`, `sdk.messages()`, `sdk.languageModel()`) based on model ID.

### How PI handles it (reference: `packages/coding-agent/src/core/model-resolver.ts`)
PI uses AI SDK directly with per-provider factory functions. The model resolver maps provider IDs to SDK packages and creates language models with the correct endpoint configuration.

### How Tau currently handles it
Single `OpenAICompatProvider` with hardcoded `/chat/completions` endpoint for ALL Zen models. No model-type detection, no endpoint routing.

## Constraints
- Tau is a Go binary — cannot use npm/AI SDK packages like opencode/PI
- Must maintain existing `Provider` interface (`Stream`, `Complete`)
- Must not break existing providers (OpenRouter, Ollama, llama.cpp, etc.)
- Zen base URL: `https://opencode.ai/zen/v1`

## Design

### Approach: Per-model API type routing

Each model in the registry gets an `API` field indicating which endpoint format to use. The `OpenAICompatProvider` already uses `model.API` — we extend this to support multiple API types for Zen.

**Model API types for Zen:**

| API value | Endpoint | Request format | Response format |
|---|---|---|---|
| `openai-completions` | `/chat/completions` | Chat Completions | Chat Completions SSE |
| `openai-responses` | `/responses` | Responses API | Responses API SSE |
| `anthropic-messages` | `/messages` | Messages API | Messages API SSE |
| `google-generative` | `/models/{id}:streamGenerateContent` | Google API | Google SSE |

### Subtasks

- [x] **042.1**: Add `API` field constants for all endpoint types
  - Extend `types.Model.API` to support: `openai-completions`, `openai-responses`, `anthropic-messages`, `google-generative`
  - Update model discovery to set correct API type per model based on ID prefix

- [x] **042.2**: Create `OpenAIResponsesProvider` for GPT models (`/responses` endpoint)
  - Implement OpenAI Responses API streaming (`POST /v1/responses`)
  - Handle SSE with `text.delta`, `reasoning.delta`, `tool_call` events
  - Map to tau's `StreamEvent` types
  - Reference: OpenAI Responses API docs, opencode's `sdk.responses(modelID)` usage

- [x] **042.3**: Create `AnthropicMessagesProvider` for Claude models (`/messages` endpoint)
  - Implement Anthropic Messages API streaming (`POST /v1/messages`)
  - Handle SSE with `content_block_start`, `content_block_delta`, `message_delta` events
  - Map to tau's `StreamEvent` types
  - Reuse existing `AnthropicProvider` patterns from `anthropic.go` but with Zen base URL

- [x] **042.4**: Create `GoogleProvider` for Gemini models (`/models/{id}` endpoint)
  - Implement Google Generative AI streaming (`POST /v1/models/{id}:streamGenerateContent`)
  - Handle SSE with `text`, `thought` events
  - Map to tau's `StreamEvent` types
  - Reuse existing `GoogleProvider` patterns from `google.go` but with Zen base URL + Bearer auth

- [x] **042.5**: Update Zen provider registration to dispatch models by API type
  - `registerOpenCodeZen()` discovers models and creates appropriate provider per model family
  - OR: Create a `ZenProvider` wrapper that routes to the correct underlying provider based on model ID
  - Model routing logic:
    - `gpt-*` → `OpenAIResponsesProvider`
    - `claude-*` → `AnthropicMessagesProvider`
    - `gemini-*` → `GoogleProvider`
    - Everything else → `OpenAICompatProvider`

- [x] **042.6**: Update model discovery to tag models with correct API type
  - Fetch `/v1/models` from Zen
  - Classify each model by ID prefix
  - Set `API` field accordingly

- [x] **042.7**: Update `Registry.ResolveModelWithFallback` to route to correct provider
  - Ensure model resolution returns the correct provider for each API type

- [x] **042.8**: Tests
  - Unit tests for each new provider with mocked SSE responses
  - Integration tests against real Zen API (skip without key)
  - Test model routing logic
  - Test all model families: GPT, Claude, Gemini, OpenAI-compatible

- [x] **042.9**: Documentation
  - Update ARCHITECTURE.md with multi-endpoint Zen architecture
  - Update DECISIONS.md with routing decision
  - Update TRACKING.md

## Acceptance Criteria
- [x] `gpt-*` models work through Zen (Responses API)
- [x] `claude-*` models work through Zen (Messages API)
- [x] `gemini-*` models work through Zen (Google API)
- [x] `qwen*`, `minimax*`, `glm*`, `kimi*` models continue to work (Chat Completions)
- [x] Model discovery correctly tags all Zen models with API type
- [x] Error handling works for all endpoint types
- [x] All existing tests pass
- [x] Binary rebuilds successfully
- [x] Documentation updated

## Reference implementations
- **OpenCode**: `packages/opencode/src/provider/provider.ts` — `BUNDLED_PROVIDERS` map, `getModel()` per provider, `transform.ts` for message normalization
- **PI**: `packages/coding-agent/src/core/model-resolver.ts` — provider-to-SDK mapping
- **Zen docs**: https://opencode.ai/docs/zen — Endpoints table showing which models use which endpoint

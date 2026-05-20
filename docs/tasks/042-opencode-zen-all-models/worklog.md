# Worklog — Task 042: OpenCode Zen — Full Multi-Endpoint Model Support

## Summary
Implemented multi-endpoint routing for OpenCode Zen provider. GPT models now use Responses API, Claude models use Messages API, Gemini models use Google Generative API with Bearer auth, and all other models continue using Chat Completions API.

## Changes

### 1. GoogleProvider Bearer Auth Mode (`google.go`)
- Added `authMode` field to `GoogleProvider` struct
- Added `GoogleConfig` struct with `AuthMode` field
- Added `NewGoogleProviderWithConfig()` constructor
- Updated `Stream()` to use Bearer auth header when `authMode == "bearer"`
- Default auth mode remains `"key-param"` (URL parameter) for backward compatibility

### 2. OpenAI Responses API Enhancement (`openai.go`)
- Added `strings` import for text accumulation
- Added `openAIOutputItem` type for parsing output_item.added events
- Added `openAIThinkingConfig` type for thinking request parameter
- Replaced `Text` field with `Thinking` field in `openAIRequest`
- Enhanced `parseStreamResponse()` to handle:
  - `response.reasoning_summary_text.delta` → `EventThinkingDelta`
  - `response.output_item.added` → `EventToolCallStart` (for function_call type)
  - `response.function_call_arguments.delta` → argument accumulation
  - `response.function_call_arguments.done` → `EventToolCallEnd` with parsed JSON args
- Enhanced `collectFromStream()` to properly accumulate thinking, text, and tool call blocks
- Updated `buildStreamRequest()` to use `Thinking` config instead of `Text` format

### 3. ZenProvider Wrapper (`zen.go` — new file)
- Created `ZenProvider` struct holding 4 sub-providers
- `NewZenProvider()` and `NewZenProviderWithClient()` constructors
- `Stream()` and `Complete()` delegate to `routeProvider(model)`
- `routeProvider()` selects sub-provider based on `model.API`
- Helper functions:
  - `ClassifyZenModelAPI()` — classifies model by ID prefix
  - `ZenModelName()` — human-readable names for all Zen models
  - `ZenModelReasoning()` — reasoning support lookup
  - `ZenModelContextWindow()` — context window lookup
  - `ZenModelCost()` — cost info lookup ($/1M tokens)
  - `ZenMaxTokens()` — default max tokens lookup
  - `DiscoverZenModels()` — fetches and classifies models from `/v1/models`

### 4. SDK Registration Update (`sdk.go`)
- `registerOpenCodeZen()` now uses `NewZenProvider()` instead of `NewOpenAICompatProvider()`
- Uses `DiscoverZenModels()` instead of `discoverOpenAICompatModels()`

### 5. Tests (`zen_test.go` — new file)
- `TestClassifyZenModelAPI` — 16 test cases for all model prefixes
- `TestZenModelName` — 7 test cases for name mapping
- `TestZenModelReasoning` — reasoning support verification
- `TestZenModelContextWindow` — context window verification
- `TestZenModelCost` — cost info verification
- `TestZenProvider_RouteProvider` — routing logic verification
- `TestZenProvider_Stream_GPT` — GPT model streaming via Responses API
- `TestZenProvider_Stream_Claude` — Claude model streaming via Messages API
- `TestZenProvider_Stream_Gemini` — Gemini model streaming via Google API
- `TestZenProvider_Stream_OpenAICompat` — OpenAI-compatible model streaming
- `TestZenProvider_Complete` — complete response collection
- `TestZenProvider_EmptyAPIKey` — error handling for missing key
- `TestGoogleProvider_BearerAuth` — Bearer auth mode verification
- `TestGoogleProvider_KeyParamAuth` — key-param auth mode verification
- `TestOpenAIProvider_Thinking` — reasoning delta handling
- `TestOpenAIProvider_ToolCalls` — tool call event handling
- `TestOpenAIProvider_CollectWithThinkingAndTools` — complete message accumulation
- `TestDiscoverZenModels_Classification` — model discovery and classification

### 6. Documentation
- Updated `ARCHITECTURE.md`: Added section 6.4a describing ZenProvider multi-endpoint routing
- Updated `DECISIONS.md`: Added decision #33 documenting the wrapper approach
- Updated `TRACKING.md`: Marked task 042 as DONE

## Test Results
All 100+ tests pass. go vet/build/-race clean.

## Notes
- The existing `OpenAIProvider` Responses API implementation was minimal (text only). Enhanced to support thinking and tool calls for full GPT model compatibility through Zen.
- GoogleProvider Bearer auth mode is backward compatible — default remains key-param auth for direct Google API usage.
- Model classification uses simple prefix matching which covers all current Zen models.

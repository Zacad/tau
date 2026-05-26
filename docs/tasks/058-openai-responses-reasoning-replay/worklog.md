# Worklog: Task 058

## 2026-05-24
- Reproduced the failure shape from code inspection: OpenAI Responses `function_call` item IDs (`fc_...`) were replayed without paired native reasoning items (`rs_...`).
- Compared Tau behavior with OpenCode and PI patterns around provider-native reasoning metadata.
- Added regression tests proving replay should preserve `call_id` while omitting `function_call.id`.
- Changed `messageToOpenAI()` to stop replaying the `fc_...` item ID.
- Verified targeted regression tests pass after the fix.
- Verification:
  - `go test ./internal/provider -run 'TestMessageToOpenAI_AssistantWithToolCall|TestOpenAIProvider_RequestOmitsFunctionCallItemIDOnReplay'` passed.
  - `go test ./internal/provider -run 'TestOpenAIProvider|TestMessageToOpenAI|TestCodexModels'` passed.
  - `go test ./...` was run and failed in pre-existing live/Ollama-dependent tests (`TestOllamaProvider_WithThinkingLevel`, `TestRun_E2E_BuiltinType_*`).
  - `go build -o ./tau ./cmd/tau` passed.

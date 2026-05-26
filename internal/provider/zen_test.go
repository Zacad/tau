package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adam/tau/internal/types"
	"github.com/invopop/jsonschema"
)

func TestClassifyZenModelAPI(t *testing.T) {
	tests := []struct {
		modelID  string
		expected string
	}{
		{"gpt-5.5", zenAPIOpenAIResponses},
		{"gpt-5.4-pro", zenAPIOpenAIResponses},
		{"gpt-5-nano", zenAPIOpenAIResponses},
		{"claude-opus-4-7", zenAPIAnthropicMessages},
		{"claude-sonnet-4-6", zenAPIAnthropicMessages},
		{"claude-haiku-4-5", zenAPIAnthropicMessages},
		{"claude-3-5-haiku", zenAPIAnthropicMessages},
		{"gemini-3.1-pro", zenAPIGoogleGenerative},
		{"gemini-3-flash", zenAPIGoogleGenerative},
		{"qwen3.6-plus", zenAPIOpenAICompletions},
		{"minimax-m2.7", zenAPIOpenAICompletions},
		{"glm-5.1", zenAPIOpenAICompletions},
		{"kimi-k2.6", zenAPIOpenAICompletions},
		{"big-pickle", zenAPIOpenAICompletions},
		{"deepseek-v4-flash-free", zenAPIOpenAICompletions},
		{"", zenAPIOpenAICompletions},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			result := ClassifyZenModelAPI(tt.modelID)
			if result != tt.expected {
				t.Errorf("ClassifyZenModelAPI(%q) = %q, want %q", tt.modelID, result, tt.expected)
			}
		})
	}
}

func TestZenModelName(t *testing.T) {
	tests := []struct {
		modelID  string
		expected string
	}{
		{"gpt-5.5", "GPT 5.5"},
		{"gpt-5.5-pro", "GPT 5.5 Pro"},
		{"claude-opus-4-7", "Claude Opus 4.7"},
		{"claude-sonnet-4-6", "Claude Sonnet 4.6"},
		{"gemini-3.1-pro", "Gemini 3.1 Pro"},
		{"qwen3.6-plus", "Qwen3.6 Plus"},
		{"unknown-model", "unknown-model"},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			result := ZenModelName(tt.modelID)
			if result != tt.expected {
				t.Errorf("ZenModelName(%q) = %q, want %q", tt.modelID, result, tt.expected)
			}
		})
	}
}

func TestZenModelReasoning(t *testing.T) {
	reasoningModels := []string{
		"gpt-5.5", "gpt-5.4", "gpt-5",
		"claude-opus-4-7", "claude-sonnet-4-6", "claude-haiku-4-5",
		"gemini-3.1-pro", "gemini-3-flash",
	}
	nonReasoningModels := []string{
		"qwen3.6-plus", "minimax-m2.7", "glm-5.1", "kimi-k2.6",
		"gpt-5.4-nano",
	}

	for _, id := range reasoningModels {
		if !ZenModelReasoning(id) {
			t.Errorf("ZenModelReasoning(%q) = false, want true", id)
		}
	}
	for _, id := range nonReasoningModels {
		if ZenModelReasoning(id) {
			t.Errorf("ZenModelReasoning(%q) = true, want false", id)
		}
	}
}

func TestZenModelContextWindow(t *testing.T) {
	if cw := ZenModelContextWindow("gpt-5.5"); cw != 272000 {
		t.Errorf("ZenModelContextWindow(gpt-5.5) = %d, want 272000", cw)
	}
	if cw := ZenModelContextWindow("claude-sonnet-4-6"); cw != 200000 {
		t.Errorf("ZenModelContextWindow(claude-sonnet-4-6) = %d, want 200000", cw)
	}
	if cw := ZenModelContextWindow("unknown-model"); cw != 0 {
		t.Errorf("ZenModelContextWindow(unknown-model) = %d, want 0", cw)
	}
}

func TestZenModelCost(t *testing.T) {
	cost := ZenModelCost("gpt-5.5")
	if cost.Input != 5.00 || cost.Output != 30.00 {
		t.Errorf("ZenModelCost(gpt-5.5) = %+v, want Input=5.00 Output=30.00", cost)
	}

	freeCost := ZenModelCost("big-pickle")
	if freeCost.Input != 0 || freeCost.Output != 0 {
		t.Errorf("ZenModelCost(big-pickle) = %+v, want free", freeCost)
	}

	unknownCost := ZenModelCost("unknown-model")
	if unknownCost.Input != 0 || unknownCost.Output != 0 {
		t.Errorf("ZenModelCost(unknown-model) = %+v, want zero", unknownCost)
	}
}

func TestZenProvider_RouteProvider(t *testing.T) {
	provider := NewZenProvider("sk-test-key")

	tests := []struct {
		name     string
		api      string
		expected string
	}{
		{"GPT models", zenAPIOpenAIResponses, "*provider.OpenAIProvider"},
		{"Claude models", zenAPIAnthropicMessages, "*provider.AnthropicProvider"},
		{"Gemini models", zenAPIGoogleGenerative, "*provider.GoogleProvider"},
		{"Other models", zenAPIOpenAICompletions, "*provider.OpenAICompatProvider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := types.Model{
				ID:       "test-model",
				Provider: "opencode-zen",
				API:      tt.api,
				BaseURL:  "https://opencode.ai/zen/v1",
			}
			routed := provider.routeProvider(model)
			typeName := fmt.Sprintf("%T", routed)
			if typeName != tt.expected {
				t.Errorf("routeProvider(API=%q) = %s, want %s", tt.api, typeName, tt.expected)
			}
		})
	}
}

func TestZenProvider_Stream_GPT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("expected /responses path, got %s", r.URL.Path)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-zen-key" {
			t.Errorf("expected Bearer auth, got %s", auth)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\": \"Hello\"}\n\n")
		fmt.Fprint(w, "event: response.completed\ndata: {\"usage\": {\"input_tokens\": 10, \"output_tokens\": 1, \"total_tokens\": 11}}\n\n")
	}))
	defer server.Close()

	provider := NewZenProviderWithClient("sk-zen-key", &testHTTPClient{})
	model := types.Model{
		ID:       "gpt-5.5",
		Provider: "opencode-zen",
		API:      zenAPIOpenAIResponses,
		BaseURL:  server.URL,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}

	foundText := false
	foundDone := false
	for _, e := range events {
		if e.Type == types.EventTextDelta {
			foundText = true
		}
		if e.Type == types.EventDone {
			foundDone = true
		}
	}
	if !foundText {
		t.Error("expected text delta event")
	}
	if !foundDone {
		t.Error("expected done event")
	}
}

func TestZenProvider_Stream_Claude(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("expected /messages path, got %s", r.URL.Path)
		}
		if apiKey := r.Header.Get("x-api-key"); apiKey != "sk-zen-key" {
			t.Errorf("expected x-api-key header, got %s", apiKey)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\ndata: {\"type\": \"message_start\"}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\": \"content_block_start\", \"content_block\": {\"type\": \"text\", \"index\": 0}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\": \"content_block_delta\", \"delta\": {\"type\": \"text_delta\", \"text\": \"Hello\"}}\n\n")
		fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\": \"content_block_stop\"}\n\n")
		fmt.Fprint(w, "event: message_delta\ndata: {\"type\": \"message_delta\", \"usage\": {\"output_tokens\": 1}}\n\n")
	}))
	defer server.Close()

	provider := NewZenProviderWithClient("sk-zen-key", &testHTTPClient{})
	model := types.Model{
		ID:       "claude-sonnet-4-6",
		Provider: "opencode-zen",
		API:      zenAPIAnthropicMessages,
		BaseURL:  server.URL,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}

	foundText := false
	for _, e := range events {
		if e.Type == types.EventTextDelta {
			foundText = true
		}
	}
	if !foundText {
		t.Error("expected text delta event for Claude model")
	}
}

func TestZenProvider_Stream_Gemini(t *testing.T) {
	var capturedPath string
	var capturedAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAPIKey = r.Header.Get("x-goog-api-key")

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"candidates": [{"content": {"parts": [{"text": "Hello"}], "role": "model"}}]}`+"\n")
		fmt.Fprint(w, `data: {"candidates": [{"content": {"parts": [{"text": " World"}], "role": "model"}, "finishReason": "STOP"}], "usageMetadata": {"promptTokenCount": 5, "candidatesTokenCount": 2, "totalTokenCount": 7}}`+"\n")
	}))
	defer server.Close()

	provider := NewZenProviderWithClient("sk-zen-key", &testHTTPClient{})
	model := types.Model{
		ID:       "gemini-3.1-pro",
		Provider: "opencode-zen",
		API:      zenAPIGoogleGenerative,
		BaseURL:  server.URL,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}

	foundText := false
	for _, e := range events {
		if e.Type == types.EventTextDelta {
			foundText = true
		}
	}
	if !foundText {
		t.Error("expected text delta event for Gemini model")
	}
	if capturedPath != "/models/gemini-3.1-pro:streamGenerateContent" {
		t.Errorf("expected path /models/gemini-3.1-pro:streamGenerateContent, got %s", capturedPath)
	}
	if capturedAPIKey != "sk-zen-key" {
		t.Errorf("expected x-goog-api-key header 'sk-zen-key', got %s", capturedAPIKey)
	}
}

func TestZenProvider_Stream_OpenAICompat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions path, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\": [{\"delta\": {\"content\": \"Hello\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\": [{\"delta\": {}, \"finish_reason\": \"stop\"}]}\n\n")
	}))
	defer server.Close()

	provider := NewZenProviderWithClient("sk-zen-key", &testHTTPClient{})
	model := types.Model{
		ID:       "qwen3.6-plus",
		Provider: "opencode-zen",
		API:      zenAPIOpenAICompletions,
		BaseURL:  server.URL,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}
}

func TestZenProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\": \"Complete response\"}\n\n")
		fmt.Fprint(w, "event: response.completed\ndata: {\"usage\": {\"input_tokens\": 5, \"output_tokens\": 2, \"total_tokens\": 7}}\n\n")
	}))
	defer server.Close()

	provider := NewZenProviderWithClient("sk-zen-key", &testHTTPClient{})
	model := types.Model{
		ID:       "gpt-5.5",
		Provider: "opencode-zen",
		API:      zenAPIOpenAIResponses,
		BaseURL:  server.URL,
	}

	msg, err := provider.Complete(context.Background(), model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected message, got nil")
	}

	var text string
	for _, block := range msg.Content {
		if block.Type == types.BlockText {
			text += block.Text
		}
	}
	if text != "Complete response" {
		t.Fatalf("expected 'Complete response', got %q", text)
	}
}

func TestZenProvider_EmptyAPIKey(t *testing.T) {
	provider := NewZenProvider("")
	model := types.Model{
		ID:       "gpt-5.5",
		Provider: "opencode-zen",
		API:      zenAPIOpenAIResponses,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != types.EventError {
		t.Fatalf("expected error event, got %s", events[0].Type)
	}
}

func TestGoogleProvider_BearerAuth(t *testing.T) {
	var capturedAPIKey string
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAPIKey = r.Header.Get("x-goog-api-key")
		capturedURL = r.URL.Path

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"candidates": [{"content": {"parts": [{"text": "Hello"}], "role": "model"}, "finishReason": "STOP"}]}`+"\n")
	}))
	defer server.Close()

	provider := NewGoogleProviderWithConfig("sk-test-key", GoogleConfig{
		AuthMode: "bearer",
	}, &testHTTPClient{})
	model := types.Model{
		ID:       "gemini-3.1-pro",
		Provider: "google",
		API:      "google-generative-ai",
		BaseURL:  server.URL,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	if capturedAPIKey != "sk-test-key" {
		t.Errorf("expected x-goog-api-key header 'sk-test-key', got %q", capturedAPIKey)
	}
	if capturedURL == "" {
		t.Error("expected URL path to be set")
	}
}

func TestGoogleProvider_KeyParamAuth(t *testing.T) {
	var capturedAuth string
	var capturedURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedURL = r.URL.String()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"candidates": [{"content": {"parts": [{"text": "Hello"}], "role": "model"}, "finishReason": "STOP"}]}`+"\n")
	}))
	defer server.Close()

	provider := NewGoogleProviderWithClient("sk-test-key", &testHTTPClient{})
	model := types.Model{
		ID:       "gemini-2.5-pro",
		Provider: "google",
		API:      "google-generative-ai",
		BaseURL:  server.URL,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	if capturedAuth != "" {
		t.Errorf("expected no Authorization header for key-param auth, got %q", capturedAuth)
	}
	if capturedURL == "" {
		t.Error("expected URL to contain key param")
	}
}

func TestOpenAIProvider_Thinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.reasoning_summary_text.delta\ndata: {\"delta\": \"Let me think\"}\n\n")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\": \"The answer is 42\"}\n\n")
		fmt.Fprint(w, "event: response.completed\ndata: {\"usage\": {\"input_tokens\": 10, \"output_tokens\": 10, \"total_tokens\": 20}}\n\n")
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	foundThinking := false
	foundText := false
	for _, e := range events {
		if e.Type == types.EventThinkingDelta {
			foundThinking = true
			if e.Delta != "Let me think" {
				t.Errorf("expected thinking delta 'Let me think', got %q", e.Delta)
			}
		}
		if e.Type == types.EventTextDelta {
			foundText = true
		}
	}
	if !foundThinking {
		t.Error("expected thinking delta event")
	}
	if !foundText {
		t.Error("expected text delta event")
	}
}

func TestOpenAIProvider_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_item.added\ndata: {\"item\": {\"type\": \"function_call\", \"id\": \"fc_call123\", \"name\": \"get_weather\", \"call_id\": \"call_123\"}}\n\n")
		fmt.Fprint(w, "event: response.function_call_arguments.delta\ndata: {\"delta\": \"{\\\"location\\\": \\\"Warsaw\\\"}\"}\n\n")
		fmt.Fprint(w, "event: response.function_call_arguments.done\ndata: {\"arguments\": \"{\\\"location\\\": \\\"Warsaw\\\"}\"}\n\n")
		fmt.Fprint(w, "event: response.completed\ndata: {\"usage\": {\"input_tokens\": 10, \"output_tokens\": 5, \"total_tokens\": 15}}\n\n")
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	foundToolStart := false
	foundToolEnd := false
	for _, e := range events {
		if e.Type == types.EventToolCallStart {
			foundToolStart = true
			if e.Delta != "get_weather" {
				t.Errorf("expected tool call name 'get_weather', got %q", e.Delta)
			}
		}
		if e.Type == types.EventToolCallEnd {
			foundToolEnd = true
		}
	}
	if !foundToolStart {
		t.Error("expected tool call start event")
	}
	if !foundToolEnd {
		t.Error("expected tool call end event")
	}
}

func TestOpenAIProvider_CollectWithThinkingAndTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.reasoning_summary_text.delta\ndata: {\"delta\": \"thinking\"}\n\n")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"delta\": \"answer\"}\n\n")
		fmt.Fprint(w, "event: response.output_item.added\ndata: {\"item\": {\"type\": \"function_call\", \"id\": \"fc_call1\", \"name\": \"tool1\", \"call_id\": \"call_1\"}}\n\n")
		fmt.Fprint(w, "event: response.function_call_arguments.delta\ndata: {\"delta\": \"{\\\"a\\\": 1}\"}\n\n")
		fmt.Fprint(w, "event: response.function_call_arguments.done\ndata: {\"arguments\": \"{\\\"a\\\": 1}\"}\n\n")
		fmt.Fprint(w, "event: response.completed\ndata: {\"usage\": {\"input_tokens\": 5, \"output_tokens\": 5, \"total_tokens\": 10}}\n\n")
	}))
	defer server.Close()

	model := testModel(server.URL)
	provider := NewOpenAIProviderWithClient("sk-test-key", &testHTTPClient{})

	msg, err := provider.Complete(context.Background(), model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var hasThinking, hasText, hasToolCall bool
	for _, block := range msg.Content {
		switch block.Type {
		case types.BlockThinking:
			hasThinking = true
			if block.Text != "thinking" {
				t.Errorf("expected thinking text 'thinking', got %q", block.Text)
			}
		case types.BlockText:
			hasText = true
			if block.Text != "answer" {
				t.Errorf("expected text 'answer', got %q", block.Text)
			}
		case types.BlockToolCall:
			hasToolCall = true
			if block.ToolCall.Name != "tool1" {
				t.Errorf("expected tool call name 'tool1', got %q", block.ToolCall.Name)
			}
		}
	}

	if !hasThinking {
		t.Error("expected thinking block in collected message")
	}
	if !hasText {
		t.Error("expected text block in collected message")
	}
	if !hasToolCall {
		t.Error("expected tool call block in collected message")
	}
}

func TestDiscoverZenModels_Classification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("expected /models path, got %s", r.URL.Path)
		}

		models := zenModelsResponse{
			Data: []zenModelEntry{
				{ID: "gpt-5.5"},
				{ID: "claude-sonnet-4-6"},
				{ID: "gemini-3.1-pro"},
				{ID: "qwen3.6-plus"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models)
	}))
	defer server.Close()

	reg := NewRegistry()
	count := DiscoverZenModels(server.URL, "sk-test-key", reg)

	if count != 4 {
		t.Fatalf("expected 4 models, got %d", count)
	}

	models := reg.Models().ListByProvider("opencode-zen")
	if len(models) != 4 {
		t.Fatalf("expected 4 models in registry, got %d", len(models))
	}

	modelMap := make(map[string]types.Model)
	for _, m := range models {
		modelMap[m.ID] = m
	}

	if modelMap["gpt-5.5"].API != zenAPIOpenAIResponses {
		t.Errorf("gpt-5.5 API = %q, want %q", modelMap["gpt-5.5"].API, zenAPIOpenAIResponses)
	}
	if modelMap["claude-sonnet-4-6"].API != zenAPIAnthropicMessages {
		t.Errorf("claude-sonnet-4-6 API = %q, want %q", modelMap["claude-sonnet-4-6"].API, zenAPIAnthropicMessages)
	}
	if modelMap["gemini-3.1-pro"].API != zenAPIGoogleGenerative {
		t.Errorf("gemini-3.1-pro API = %q, want %q", modelMap["gemini-3.1-pro"].API, zenAPIGoogleGenerative)
	}
	if modelMap["qwen3.6-plus"].API != zenAPIOpenAICompletions {
		t.Errorf("qwen3.6-plus API = %q, want %q", modelMap["qwen3.6-plus"].API, zenAPIOpenAICompletions)
	}
}

func TestSanitizeGoogleSchema_StripsMetaFields(t *testing.T) {
	input := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://example.com/schema",
		"$ref":    "#/$defs/something",
		"$defs":   map[string]any{},
		"type":    "object",
		"properties": map[string]any{
			"name": map[string]any{
				"$schema": "https://json-schema.org/draft/2020-12/schema",
				"type":    "string",
			},
		},
		"required": []any{"name"},
	}

	result := sanitizeGoogleSchema(input)

	if _, exists := result["$schema"]; exists {
		t.Error("expected $schema to be stripped")
	}
	if _, exists := result["$id"]; exists {
		t.Error("expected $id to be stripped")
	}
	if _, exists := result["$ref"]; exists {
		t.Error("expected $ref to be stripped")
	}
	if _, exists := result["$defs"]; exists {
		t.Error("expected $defs to be stripped")
	}
	if result["type"] != "object" {
		t.Errorf("expected type=object, got %v", result["type"])
	}

	props := result["properties"].(map[string]any)
	nameProp := props["name"].(map[string]any)
	if _, exists := nameProp["$schema"]; exists {
		t.Error("expected nested $schema to be stripped")
	}
}

func TestSanitizeGoogleSchema_IntegerEnumsToString(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"level": map[string]any{
				"type": "integer",
				"enum": []any{1, 2, 3},
			},
		},
	}

	result := sanitizeGoogleSchema(input)
	props := result["properties"].(map[string]any)
	levelProp := props["level"].(map[string]any)

	if levelProp["type"] != "string" {
		t.Errorf("expected type=string for enum, got %v", levelProp["type"])
	}
	enums := levelProp["enum"].([]any)
	if enums[0] != "1" || enums[1] != "2" || enums[2] != "3" {
		t.Errorf("expected string enums, got %v", enums)
	}
}

func TestSanitizeGoogleSchema_FiltersRequired(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []any{"name", "nonexistent"},
	}

	result := sanitizeGoogleSchema(input)
	req := result["required"].([]any)

	if len(req) != 1 || req[0] != "name" {
		t.Errorf("expected required=[name], got %v", req)
	}
}

func TestSanitizeGoogleSchema_NilAndNonObject(t *testing.T) {
	if sanitizeGoogleSchema(nil) != nil {
		t.Error("expected nil input to return nil")
	}

	// Non-map input returns empty map (no properties to extract)
	result := sanitizeGoogleSchema(map[string]any{"not": "a schema"})
	if result["not"] != "a schema" {
		t.Error("expected non-object input to return unchanged")
	}
}

func TestSanitizeGoogleSchema_ArrayItems(t *testing.T) {
	input := map[string]any{
		"type": "array",
		"items": map[string]any{
			"$schema": "https://json-schema.org/draft/2020-12/schema",
			"type":    "string",
		},
	}

	result := sanitizeGoogleSchema(input)
	items := result["items"].(map[string]any)

	if _, exists := items["$schema"]; exists {
		t.Error("expected $schema in items to be stripped")
	}
	if items["type"] != "string" {
		t.Errorf("expected items type=string, got %v", items["type"])
	}
}

func TestSanitizeGoogleSchema_InlinesRef(t *testing.T) {
	input := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$ref":    "#/$defs/TestParams",
		"$defs": map[string]any{
			"TestParams": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"count": map[string]any{"type": "integer"},
				},
				"required": []any{"query"},
			},
		},
	}

	result := sanitizeGoogleSchema(input)

	// Meta fields should be stripped
	if _, exists := result["$schema"]; exists {
		t.Error("expected $schema to be stripped")
	}
	if _, exists := result["$ref"]; exists {
		t.Error("expected $ref to be stripped")
	}
	if _, exists := result["$defs"]; exists {
		t.Error("expected $defs to be stripped")
	}

	// Definition should be inlined
	if result["type"] != "object" {
		t.Errorf("expected type=object after inlining, got %v", result["type"])
	}
	if _, ok := result["properties"]; !ok {
		t.Error("expected properties after inlining")
	}
	if _, ok := result["required"]; !ok {
		t.Error("expected required after inlining")
	}
	props := result["properties"].(map[string]any)
	if _, ok := props["query"]; !ok {
		t.Error("expected query property after inlining")
	}
	if _, ok := props["count"]; !ok {
		t.Error("expected count property after inlining")
	}
	req := result["required"].([]any)
	if len(req) != 1 || req[0] != "query" {
		t.Errorf("expected required=[query], got %v", req)
	}
}

// TestGoogleProvider_SanitizesJsonSchema verifies that *jsonschema.Schema
// parameters are correctly sanitized before being sent to Google API.
// This is the core bug fix for Task 043: jsonschema.Schema generates
// $schema/$id/$ref/$defs meta fields that Google rejects.
func TestGoogleProvider_SanitizesJsonSchema(t *testing.T) {
	type TestParams struct {
		Query string `json:"query" jsonschema_description:"The search query"`
		Count int    `json:"count" jsonschema_description:"Number of results"`
	}

	schema := jsonschema.Reflect(&TestParams{})

	var capturedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(json.RawMessage{})
		// Capture the raw request body for inspection
		var reqMap map[string]any
		dec := json.NewDecoder(r.Body)
		dec.Decode(&reqMap)
		body, _ = json.Marshal(reqMap)
		capturedBody = body

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"candidates": [{"content": {"parts": [{"text": "Hello"}], "role": "model"}, "finishReason": "STOP"}]}`+"\n")
	}))
	defer server.Close()

	provider := NewGoogleProviderWithConfig("sk-test-key", GoogleConfig{
		AuthMode: "bearer",
	}, &testHTTPClient{})
	model := types.Model{
		ID:       "gemini-3.1-pro",
		Provider: "google",
		API:      "google-generative-ai",
		BaseURL:  server.URL,
	}

	tools := []types.ToolDefinition{
		{
			Name:        "search",
			Description: "Search for information",
			Parameters:  schema,
		},
	}

	ch := provider.Stream(context.Background(), model, []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}}},
	}, tools, types.StreamOptions{})
	collectStream(ch)

	// Verify meta fields are stripped from the captured request body
	bodyStr := string(capturedBody)
	for _, field := range []string{`"$schema"`, `"$id"`, `"$ref"`, `"$defs"`} {
		if strings.Contains(bodyStr, field) {
			t.Errorf("expected %s to be stripped from request body, got:\n%s", field, bodyStr)
		}
	}

	// Verify valid fields are preserved
	if !strings.Contains(bodyStr, `"type"`) {
		t.Errorf("expected 'type' field in request body, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"properties"`) {
		t.Errorf("expected 'properties' field in request body, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"search"`) {
		t.Errorf("expected tool name 'search' in request body, got:\n%s", bodyStr)
	}
}

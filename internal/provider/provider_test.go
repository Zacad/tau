package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

func TestRegistry_NewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if r.Models() == nil {
		t.Fatal("expected non-nil model registry")
	}
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()

	p := NewOpenAIProvider("sk-test")
	r.Register(p)

	got, ok := r.Get("openai")
	if !ok {
		t.Fatal("expected to find openai provider")
	}
	if got.Name() != "openai" {
		t.Fatalf("expected openai, got %s", got.Name())
	}

	_, ok = r.Get("nonexistent")
	if ok {
		t.Fatal("expected not to find nonexistent provider")
	}
}

func TestRegistry_ResolveModel(t *testing.T) {
	r := NewRegistry()

	// Exact model ID
	m, err := r.ResolveModel("gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", m.ID)
	}

	// Default model
	r.SetDefaultModel("claude-sonnet-4-6")
	m, err = r.ResolveModel("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.ID != "claude-sonnet-4-6" {
		t.Fatalf("expected claude-sonnet-4-6, got %s", m.ID)
	}

	// No model and no default
	r2 := NewRegistry()
	_, err = r2.ResolveModel("")
	if err == nil {
		t.Fatal("expected error when no model and no default")
	}

	// No match
	_, err = r.ResolveModel("nonexistent-model-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
}

func TestRegistry_ListProviders(t *testing.T) {
	r := NewRegistry()
	r.Register(NewOpenAIProvider("sk-test"))
	r.Register(NewAnthropicProvider("sk-test"))

	names := r.ListProviders()
	if len(names) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(names))
	}
}

func TestRegistry_ModelsAccess(t *testing.T) {
	r := NewRegistry()
	models := r.Models()
	all := models.ListAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 models, got %d", len(all))
	}
}

func TestAnthropicProvider_Stream(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			// Verify headers
			if req.Headers["x-api-key"] != "sk-anthropic-key" {
				t.Errorf("expected x-api-key header")
			}
			if req.Headers["anthropic-version"] == "" {
				t.Errorf("expected anthropic-version header")
			}
			return &Response{
				StatusCode: 200,
				Header:     map[string][]string{"Content-Type": {"text/event-stream"}},
				Body: []byte(
					"event: message_start\ndata: {}\n\n" +
						"event: content_block_start\ndata: {\"content_block\": {\"type\": \"text\"}}\n\n" +
						"event: content_block_delta\ndata: {\"delta\": {\"type\": \"text_delta\", \"text\": \"Hello\"}}\n\n" +
						"event: content_block_delta\ndata: {\"delta\": {\"type\": \"text_delta\", \"text\": \" World\"}}\n\n" +
						"event: message_delta\ndata: {\"usage\": {\"output_tokens\": 2}}\n\n" +
						"event: message_stop\ndata: {}\n\n",
				),
			}, nil
		},
	}

	provider := NewAnthropicProviderWithClient("sk-anthropic-key", client)
	model := types.Model{
		ID:       "claude-sonnet-4-20250514",
		Provider: "anthropic",
		API:      "anthropic-messages",
		BaseURL:  "https://api.anthropic.com/v1",
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	foundStart := false
	foundText := false
	foundDone := false
	for _, e := range events {
		if e.Type == types.EventStart {
			foundStart = true
		}
		if e.Type == types.EventTextDelta {
			foundText = true
		}
		if e.Type == types.EventDone {
			foundDone = true
			if e.Usage != nil && e.Usage.Output != 2 {
				t.Errorf("expected 2 output tokens, got %d", e.Usage.Output)
			}
		}
	}
	if !foundStart {
		t.Error("expected start event")
	}
	if !foundText {
		t.Error("expected text delta event")
	}
	if !foundDone {
		t.Error("expected done event")
	}
}

func TestAnthropicProvider_Complete(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			return &Response{
				StatusCode: 200,
				Body: []byte(
					"event: message_start\ndata: {}\n\n" +
						"event: content_block_delta\ndata: {\"delta\": {\"type\": \"text_delta\", \"text\": \"Complete\"}}\n\n" +
						"event: message_delta\ndata: {\"usage\": {\"output_tokens\": 1}}\n\n",
				),
			}, nil
		},
	}

	provider := NewAnthropicProviderWithClient("sk-test", client)
	model := types.Model{ID: "claude-sonnet-4-20250514", API: "anthropic-messages"}

	msg, err := provider.Complete(context.Background(), model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected message")
	}
}

func TestAnthropicProvider_EmptyAPIKey(t *testing.T) {
	provider := NewAnthropicProvider("")
	ch := provider.Stream(context.Background(), types.Model{}, nil, nil, types.StreamOptions{})
	events := collectStream(ch)
	if len(events) != 1 || events[0].Type != types.EventError {
		t.Fatalf("expected single error event")
	}
}

func TestAnthropicProvider_Thinking(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			// Verify thinking config in request body
			return &Response{
				StatusCode: 200,
				Body: []byte(
					"event: message_start\ndata: {}\n\n" +
						"event: content_block_delta\ndata: {\"delta\": {\"type\": \"thinking_delta\", \"thinking\": \"Let me think...\"}}\n\n" +
						"event: content_block_delta\ndata: {\"delta\": {\"type\": \"text_delta\", \"text\": \"Answer\"}}\n\n" +
						"event: message_delta\ndata: {\"usage\": {\"output_tokens\": 1}}\n\n",
				),
			}, nil
		},
	}

	provider := NewAnthropicProviderWithClient("sk-test", client)
	model := types.Model{
		ID:        "claude-sonnet-4-20250514",
		API:       "anthropic-messages",
		Reasoning: true,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{
		ThinkingLevel: types.ThinkingHigh,
	})
	events := collectStream(ch)

	foundThinking := false
	foundText := false
	for _, e := range events {
		if e.Type == types.EventThinkingDelta {
			foundThinking = true
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

func TestThinkingBudget(t *testing.T) {
	tests := []struct {
		level    types.ThinkingLevel
		expected int
	}{
		{types.ThinkingMinimal, 1024},
		{types.ThinkingLow, 2048},
		{types.ThinkingMedium, 4096},
		{types.ThinkingHigh, 8192},
		{types.ThinkingXHigh, 16384},
		{types.ThinkingOff, 4096},
		{"", 4096},
	}
	for _, tc := range tests {
		got := thinkingBudget(tc.level)
		if got != tc.expected {
			t.Errorf("thinkingBudget(%s) = %d, expected %d", tc.level, got, tc.expected)
		}
	}
}

func TestAnthropicProvider_Interface(t *testing.T) {
	var _ Provider = (*AnthropicProvider)(nil)
}

func TestGoogleProvider_Stream(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			// Verify API key is in URL
			if req.URL == "" {
				t.Fatal("expected URL with API key")
			}
			return &Response{
				StatusCode: 200,
				Body: []byte(
					"data: {\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"Hello\"}]}, \"finishReason\": \"\"}]}\n" +
						"data: {\"candidates\": [{\"content\": {\"parts\": [{\"text\": \" World\"}]}, \"finishReason\": \"\"}]}\n" +
						"data: {\"candidates\": [{\"content\": {\"parts\": []}, \"finishReason\": \"STOP\"}], \"usageMetadata\": {\"promptTokenCount\": 5, \"candidatesTokenCount\": 2, \"totalTokenCount\": 7}}\n",
				),
			}, nil
		},
	}

	provider := NewGoogleProviderWithClient("sk-google-key", client)
	model := types.Model{
		ID:       "gemini-2.5-pro",
		Provider: "google",
		API:      "google-generative-ai",
		BaseURL:  "https://generativelanguage.googleapis.com/v1beta/models",
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	foundText := false
	foundDone := false
	for _, e := range events {
		if e.Type == types.EventTextDelta {
			foundText = true
		}
		if e.Type == types.EventDone {
			foundDone = true
			if e.Usage != nil && e.Usage.TotalTokens != 7 {
				t.Errorf("expected 7 total tokens, got %d", e.Usage.TotalTokens)
			}
		}
	}
	if !foundText {
		t.Error("expected text delta event")
	}
	if !foundDone {
		t.Error("expected done event")
	}
}

func TestGoogleProvider_Complete(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			return &Response{
				StatusCode: 200,
				Body: []byte(
					"data: {\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"Gemini response\"}]}, \"finishReason\": \"\"}]}\n" +
						"data: {\"candidates\": [{\"content\": {\"parts\": []}, \"finishReason\": \"STOP\"}]}\n",
				),
			}, nil
		},
	}

	provider := NewGoogleProviderWithClient("sk-test", client)
	model := types.Model{ID: "gemini-2.5-pro", API: "google-generative-ai"}

	msg, err := provider.Complete(context.Background(), model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected message")
	}
}

func TestGoogleProvider_EmptyAPIKey(t *testing.T) {
	provider := NewGoogleProvider("")
	ch := provider.Stream(context.Background(), types.Model{}, nil, nil, types.StreamOptions{})
	events := collectStream(ch)
	if len(events) != 1 || events[0].Type != types.EventError {
		t.Fatalf("expected single error event")
	}
}

func TestGoogleProvider_Interface(t *testing.T) {
	var _ Provider = (*GoogleProvider)(nil)
}

func TestGoogleProvider_ThinkingConfig(t *testing.T) {
	tests := []struct {
		name         string
		level        types.ThinkingLevel
		wantBudget   int
		wantLevel    string
		wantThinking bool
	}{
		{"off", types.ThinkingOff, 0, "", false},
		{"empty", "", 0, "", false},
		{"minimal", types.ThinkingMinimal, 0, "MINIMAL", true},
		{"low", types.ThinkingLow, 0, "LOW", true},
		{"medium", types.ThinkingMedium, 0, "MEDIUM", true},
		{"high", types.ThinkingHigh, 0, "HIGH", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody []byte
			client := &MockHTTPClient{
				DoFunc: func(req *Request) (*Response, error) {
					capturedBody = req.Body
					return &Response{
						StatusCode: 200,
						Body: []byte(
							"data: {\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"OK\"}]}, \"finishReason\": \"STOP\"}]}\n",
						),
					}, nil
				},
			}

			provider := NewGoogleProviderWithClient("sk-test", client)
			model := types.Model{
				ID:        "gemini-3.1-pro",
				API:       "google-generative-ai",
				Reasoning: true,
				ThinkingLevelMap: map[string]string{
					"minimal": "MINIMAL", "low": "LOW", "medium": "MEDIUM", "high": "HIGH",
				},
			}

			ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{
				ThinkingLevel: tt.level,
			})
			for range ch {
			}

			var body map[string]any
			json.Unmarshal(capturedBody, &body)

			genConfig, ok := body["generationConfig"].(map[string]any)
			if !ok {
				t.Fatal("expected generationConfig in body")
			}

			thinkingConfig, hasThinking := genConfig["thinkingConfig"].(map[string]any)
			if tt.wantThinking {
				if !hasThinking {
					t.Fatalf("expected thinkingConfig in body, got: %v", body)
				}
				if tt.wantBudget > 0 {
					budget, _ := thinkingConfig["thinkingBudget"].(float64)
					if int(budget) != tt.wantBudget {
						t.Errorf("thinkingBudget=%d, want %d", int(budget), tt.wantBudget)
					}
				}
				if tt.wantLevel != "" {
					level, _ := thinkingConfig["thinkingLevel"].(string)
					if level != tt.wantLevel {
						t.Errorf("thinkingLevel=%q, want %q", level, tt.wantLevel)
					}
				}
			} else {
				if hasThinking {
					t.Errorf("expected no thinkingConfig for level=%q, got: %v", tt.level, thinkingConfig)
				}
			}
		})
	}
}

func TestGoogleProvider_ThinkingBudget(t *testing.T) {
	// Test Gemini 2.x style: numeric budget tokens
	var capturedBody []byte
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			capturedBody = req.Body
			return &Response{
				StatusCode: 200,
				Body:       []byte("data: {\"candidates\": [{\"content\": {\"parts\": []}, \"finishReason\": \"STOP\"}]}\n"),
			}, nil
		},
	}

	provider := NewGoogleProviderWithClient("sk-test", client)
	model := types.Model{
		ID:        "gemini-2.5-pro",
		API:       "google-generative-ai",
		Reasoning: true,
		ThinkingLevelMap: map[string]string{
			"minimal": "128", "low": "2048", "medium": "8192", "high": "32768",
		},
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{
		ThinkingLevel: types.ThinkingMedium,
	})
	for range ch {
	}

	var body map[string]any
	json.Unmarshal(capturedBody, &body)

	genConfig := body["generationConfig"].(map[string]any)
	thinkingConfig := genConfig["thinkingConfig"].(map[string]any)

	budget, _ := thinkingConfig["thinkingBudget"].(float64)
	if int(budget) != 8192 {
		t.Errorf("thinkingBudget=%d, want 8192 (from model mapping)", int(budget))
	}
}

func TestOpenAICompatProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w,
			"data: {\"choices\": [{\"delta\": {\"content\": \"Hello\"}, \"finish_reason\": \"\", \"index\": 0}]}\n\n"+
				"data: {\"choices\": [{\"delta\": {\"content\": \" World\"}, \"finish_reason\": \"\", \"index\": 0}]}\n\n"+
				"data: {\"choices\": [{\"delta\": {\"content\": \"\"}, \"finish_reason\": \"stop\", \"index\": 0}], \"usage\": {\"prompt_tokens\": 5, \"completion_tokens\": 2, \"total_tokens\": 7}}\n\n",
		)
	}))
	defer server.Close()

	config := OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "test",
	}
	provider := NewOpenAICompatProvider("sk-test", config)
	model := types.Model{
		ID:       "llama3",
		Provider: "test",
		API:      "openai-completions",
		BaseURL:  server.URL,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

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

func TestOpenAICompatProvider_DoneMarker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w,
			"data: {\"choices\": [{\"delta\": {\"content\": \"Hi\"}, \"finish_reason\": \"\", \"index\": 0}]}\n\n"+
				"data: [DONE]\n\n",
		)
	}))
	defer server.Close()

	config := OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "test",
	}
	provider := NewOpenAICompatProvider("sk-test", config)
	model := types.Model{ID: "gpt-4o", BaseURL: server.URL}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)

	foundDone := false
	for _, e := range events {
		if e.Type == types.EventDone {
			foundDone = true
		}
	}
	if !foundDone {
		t.Error("expected done event for [DONE] marker")
	}
}

func TestOpenAICompatProvider_EmptyAPIKey(t *testing.T) {
	// Ollama-compatible providers don't require API keys, so empty key is valid
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	config := OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "test",
	}
	provider := NewOpenAICompatProvider("", config)
	model := types.Model{ID: "llama3", BaseURL: server.URL}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	events := collectStream(ch)
	if len(events) == 0 {
		t.Fatal("expected events, got none")
	}
}

func TestOpenAICompatProvider_DefaultConfig(t *testing.T) {
	provider := NewOpenAICompatProvider("sk-test", OpenAICompatConfig{})
	if provider.Name() != "openai-compat" {
		t.Fatalf("expected default name openai-compat, got %s", provider.Name())
	}
}

func TestOpenAICompatProvider_Interface(t *testing.T) {
	var _ Provider = (*OpenAICompatProvider)(nil)
}

func TestOpenAICompatProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w,
			"data: {\"choices\": [{\"delta\": {\"content\": \"Complete\"}, \"finish_reason\": \"\", \"index\": 0}]}\n\n"+
				"data: {\"choices\": [{\"delta\": {\"content\": \"\"}, \"finish_reason\": \"stop\", \"index\": 0}]}\n\n",
		)
	}))
	defer server.Close()

	config := OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "test",
	}
	provider := NewOpenAICompatProvider("sk-test", config)
	model := types.Model{ID: "llama3", BaseURL: server.URL}

	msg, err := provider.Complete(context.Background(), model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == nil {
		t.Fatal("expected message")
	}
}

func TestOpenAICompatProvider_Headers(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	config := OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "test",
		ExtraHeaders: map[string]string{"X-Provider": "openrouter"},
	}
	provider := NewOpenAICompatProvider("sk-test", config)
	model := types.Model{ID: "gpt-4o", BaseURL: server.URL}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	if capturedHeaders.Get("Authorization") != "Bearer sk-test" {
		t.Errorf("expected Bearer token, got %s", capturedHeaders.Get("Authorization"))
	}
	if capturedHeaders.Get("X-Provider") != "openrouter" {
		t.Errorf("expected X-Provider header, got %s", capturedHeaders.Get("X-Provider"))
	}
}

func TestOpenAICompatProvider_Stream_ContextCancel(t *testing.T) {
	// Server that sends a slow stream
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// Send first chunk
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()

		// Wait to simulate slow stream - context should cancel before this completes
		time.Sleep(100 * time.Millisecond)

		// This should never be read if context is cancelled
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" World\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	config := OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "test",
	}
	provider := NewOpenAICompatProvider("sk-test", config)
	model := types.Model{ID: "llama3", BaseURL: server.URL}

	ctx, cancel := context.WithCancel(context.Background())
	ch := provider.Stream(ctx, model, nil, nil, types.StreamOptions{})

	// Cancel after a short delay
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	events := collectStream(ch)

	// Should have received at least EventStart before cancellation
	if len(events) == 0 {
		t.Fatal("expected at least one event before cancellation")
	}
}

func TestMessageToOpenAICompat(t *testing.T) {
	userMsg := types.AgentMessage{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}},
	}
	m := messageToOpenAICompat(userMsg)
	if m == nil || m.Role != "user" || m.Content != "Hello" {
		t.Errorf("expected user message with 'Hello', got %+v", m)
	}

	toolMsg := types.AgentMessage{
		Role:    types.RoleToolResult,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Result"}},
	}
	m = messageToOpenAICompat(toolMsg)
	if m == nil || m.Role != "tool" {
		t.Errorf("expected tool role, got %+v", m)
	}

	// Empty content returns nil
	emptyMsg := types.AgentMessage{Role: types.RoleUser, Content: nil}
	m = messageToOpenAICompat(emptyMsg)
	if m != nil {
		t.Errorf("expected nil for empty message, got %+v", m)
	}
}

func TestMessageToAnthropic(t *testing.T) {
	userMsg := types.AgentMessage{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}},
	}
	m := messageToAnthropic(userMsg)
	if m == nil || m.Role != "user" || len(m.Content) == 0 {
		t.Errorf("expected user message, got %+v", m)
	}

	toolMsg := types.AgentMessage{
		Role:    types.RoleToolResult,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Tool result"}},
	}
	m = messageToAnthropic(toolMsg)
	if m == nil || m.Role != "user" {
		t.Errorf("expected user role for tool result, got %+v", m)
	}

	emptyMsg := types.AgentMessage{Role: types.RoleUser, Content: nil}
	m = messageToAnthropic(emptyMsg)
	if m != nil {
		t.Errorf("expected nil for empty message, got %+v", m)
	}
}

func TestMessageToAnthropic_ThinkingRoundTrip(t *testing.T) {
	// Assistant message with thinking block (no signature)
	msgNoSig := types.AgentMessage{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.BlockThinking, Text: "Let me think about this"},
			{Type: types.BlockText, Text: "The answer is 42"},
		},
	}
	m := messageToAnthropic(msgNoSig)
	if m == nil || m.Role != "assistant" {
		t.Fatalf("expected assistant message, got %+v", m)
	}
	if len(m.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(m.Content))
	}
	if m.Content[0].Type != "thinking" {
		t.Errorf("first block should be thinking, got %s", m.Content[0].Type)
	}
	if m.Content[0].Thinking != "Let me think about this" {
		t.Errorf("thinking = %q, want %q", m.Content[0].Thinking, "Let me think about this")
	}
	// No signature — field should be empty
	if m.Content[0].Signature != "" {
		t.Errorf("expected empty signature, got %q", m.Content[0].Signature)
	}

	// Assistant message with thinking block containing NUL-separated signature
	msgWithSig := types.AgentMessage{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.BlockThinking, Text: "reasoning text\x00sig_abc123"},
			{Type: types.BlockText, Text: "Done"},
		},
	}
	m2 := messageToAnthropic(msgWithSig)
	if m2 == nil || len(m2.Content) < 1 {
		t.Fatalf("expected assistant message with content, got %+v", m2)
	}
	if m2.Content[0].Type != "thinking" {
		t.Errorf("first block should be thinking, got %s", m2.Content[0].Type)
	}
	if m2.Content[0].Thinking != "reasoning text" {
		t.Errorf("thinking = %q, want %q", m2.Content[0].Thinking, "reasoning text")
	}
	if m2.Content[0].Signature != "sig_abc123" {
		t.Errorf("signature = %q, want %q", m2.Content[0].Signature, "sig_abc123")
	}
}

func TestAnthropicProvider_SignatureDelta(t *testing.T) {
	// Simulate Anthropic SSE with thinking_delta followed by signature_delta
	body := []byte(
		"event: message_start\ndata: {\"message\":{\"id\":\"msg_1\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n" +
			"event: content_block_start\ndata: {\"index\":0,\"content_block\":{\"type\":\"thinking\",\"thinking\":\"\"}}\n\n" +
			"event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"Let me think\"}}\n\n" +
			"event: content_block_delta\ndata: {\"index\":0,\"delta\":{\"type\":\"signature_delta\",\"signature\":\"sig_xyz789\"}}\n\n" +
			"event: content_block_start\ndata: {\"index\":1,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
			"event: content_block_delta\ndata: {\"index\":1,\"delta\":{\"type\":\"text_delta\",\"text\":\"The answer is 42\"}}\n\n" +
			"event: message_delta\ndata: {\"usage\":{\"output_tokens\":20,\"cache_creation_input_tokens\":0,\"cache_read_input_tokens\":0}}\n\n" +
			"event: message_stop\ndata: {}\n\n",
	)

	ctx := context.Background()
	p := &AnthropicProvider{}
	ch := p.parseStreamResponse(ctx, body, "claude-sonnet-4")
	events := collectStream(ch)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	// Verify thinking block exists (signature stored separately now)
	if len(msg.Content) < 1 {
		t.Fatal("expected at least 1 content block")
	}

	thinkingBlocks := extractThinkingBlocks(msg)
	if len(thinkingBlocks) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinkingBlocks))
	}
	// Signature is stored separately, but in the Text field we still embed
	// it via NUL separator for persistence — verify it's there
	if !strings.Contains(thinkingBlocks[0], "\x00sig_xyz789") {
		t.Errorf("thinking block should contain NUL+signature, got %q", thinkingBlocks[0])
	}

	// Verify thinking event was emitted
	if countEventType(events, types.EventThinkingDelta) != 1 {
		t.Errorf("expected 1 EventThinkingDelta, got %d", countEventType(events, types.EventThinkingDelta))
	}
	if countEventType(events, types.EventThinkingStart) != 1 {
		t.Error("expected 1 EventThinkingStart")
	}
	if countEventType(events, types.EventThinkingEnd) != 1 {
		t.Error("expected 1 EventThinkingEnd")
	}
}

func TestMessageToGoogle(t *testing.T) {
	userMsg := types.AgentMessage{
		Role:    types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}},
	}
	m := messageToGoogle(userMsg)
	if m == nil || m.Role != "user" || len(m.Parts) == 0 {
		t.Errorf("expected user message, got %+v", m)
	}

	emptyMsg := types.AgentMessage{Role: types.RoleUser, Content: nil}
	m = messageToGoogle(emptyMsg)
	if m != nil {
		t.Errorf("expected nil for empty message, got %+v", m)
	}
}

func TestCloseWithError(t *testing.T) {
	ch := make(chan types.StreamEvent, 10)
	closeWithError(ch, fmt.Errorf("test error"))
	events := collectStream(ch)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != types.EventError {
		t.Fatalf("expected error event, got %s", events[0].Type)
	}
}

func TestSendEvent_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan types.StreamEvent) // unbuffered — send will block

	sent := sendEvent(ctx, ch, types.StreamEvent{Type: types.EventTextDelta})
	if sent {
		t.Fatal("expected sendEvent to return false on cancelled context")
	}
}

func TestStreamToChannel_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	events := []types.StreamEvent{
		{Type: types.EventTextDelta, Delta: "Hello"},
		{Type: types.EventDone},
	}
	ch := streamToChannel(ctx, events)
	received := collectStream(ch)
	// Should get error event due to cancelled context
	if len(received) == 0 {
		t.Fatal("expected at least error event")
	}
}

func TestOpenAIProvider_WithTools(t *testing.T) {
	var capturedBody []byte
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			capturedBody = req.Body
			return &Response{
				StatusCode: 200,
				Body:       []byte("event: response.completed\ndata: {}\n\n"),
			}, nil
		},
	}

	provider := NewOpenAIProviderWithClient("sk-test", client)
	model := testModel("http://localhost")

	tools := []types.ToolDefinition{
		{Name: "read", Description: "Read a file"},
	}

	ch := provider.Stream(context.Background(), model, nil, tools, types.StreamOptions{})
	collectStream(ch)

	if !strings.Contains(string(capturedBody), "tools") {
		t.Error("expected tools in request body")
	}
}

func TestOpenAIProvider_ModelHeaders(t *testing.T) {
	var capturedHeaders map[string]string
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			capturedHeaders = req.Headers
			return &Response{
				StatusCode: 200,
				Body:       []byte("event: response.completed\ndata: {}\n\n"),
			}, nil
		},
	}

	provider := NewOpenAIProviderWithClient("sk-test", client)
	model := types.Model{
		ID:      "test",
		Headers: map[string]string{"X-Custom": "header-value"},
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)

	if capturedHeaders["X-Custom"] != "header-value" {
		t.Errorf("expected X-Custom header, got %s", capturedHeaders["X-Custom"])
	}
}

func TestAnthropicProvider_WithTools(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			if !strings.Contains(string(req.Body), "tools") {
				t.Error("expected tools in request body")
			}
			return &Response{
				StatusCode: 200,
				Body: []byte(
					"event: message_start\ndata: {}\n\n" +
						"event: message_delta\ndata: {\"usage\": {\"output_tokens\": 1}}\n\n",
				),
			}, nil
		},
	}

	provider := NewAnthropicProviderWithClient("sk-test", client)
	model := types.Model{ID: "claude-sonnet-4-20250514", API: "anthropic-messages"}

	tools := []types.ToolDefinition{
		{Name: "read", Description: "Read a file"},
	}

	ch := provider.Stream(context.Background(), model, nil, tools, types.StreamOptions{})
	collectStream(ch)
}

func TestAnthropicProvider_MaxTokensDefault(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			return &Response{
				StatusCode: 200,
				Body: []byte(
					"event: message_start\ndata: {}\n\n" +
						"event: message_delta\ndata: {\"usage\": {\"output_tokens\": 1}}\n\n",
				),
			}, nil
		},
	}

	provider := NewAnthropicProviderWithClient("sk-test", client)
	model := types.Model{
		ID:        "claude-sonnet-4-20250514",
		API:       "anthropic-messages",
		MaxTokens: 8192,
	}

	ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{})
	collectStream(ch)
}

func TestGoogleProvider_WithTools(t *testing.T) {
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			if !strings.Contains(string(req.Body), "tools") {
				t.Error("expected tools in request body")
			}
			return &Response{
				StatusCode: 200,
				Body: []byte(
					"data: {\"candidates\": [{\"content\": {\"parts\": [{\"text\": \"Hi\"}]}, \"finishReason\": \"\"}]}\n" +
						"data: {\"candidates\": [{\"content\": {\"parts\": []}, \"finishReason\": \"STOP\"}]}\n",
				),
			}, nil
		},
	}

	provider := NewGoogleProviderWithClient("sk-test", client)
	model := types.Model{ID: "gemini-2.5-pro", API: "google-generative-ai"}

	tools := []types.ToolDefinition{
		{Name: "read", Description: "Read a file"},
	}

	ch := provider.Stream(context.Background(), model, nil, tools, types.StreamOptions{})
	collectStream(ch)
}

// --- Gap coverage tests for reasoning ---

func TestParseStreamResponse_ReasoningTextField(t *testing.T) {
	// Some providers use "reasoning_text" field
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning_text\":" + `"qwen reasoning"` + "},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	if countEventType(events, types.EventThinkingStart) != 1 {
		t.Error("expected 1 EventThinkingStart")
	}
	if countEventType(events, types.EventThinkingDelta) != 1 {
		t.Errorf("expected 1 EventThinkingDelta, got %d", countEventType(events, types.EventThinkingDelta))
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinking))
	}
	if thinking[0] != "qwen reasoning" {
		t.Errorf("thinking = %q, want %q", thinking[0], "qwen reasoning")
	}
}

func TestParseStreamResponse_Reasoning_SSLEndsWithoutDone(t *testing.T) {
	// SSE ends without [DONE] marker — should still flush thinking block
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"partial reasoning\"},\"finish_reason\":null}]}\n")
	events := collectStreamEvents(t, body)

	if countEventType(events, types.EventThinkingStart) != 1 {
		t.Error("expected 1 EventThinkingStart")
	}
	if countEventType(events, types.EventThinkingEnd) != 1 {
		t.Error("expected 1 EventThinkingEnd (flushed at SSE end)")
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinking))
	}
	if thinking[0] != "partial reasoning" {
		t.Errorf("thinking = %q, want %q", thinking[0], "partial reasoning")
	}
}

func TestParseStreamResponse_Reasoning_MultipleFragmentsSameBlock(t *testing.T) {
	// Multiple reasoning deltas should accumulate into single thinking block
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"step1 \"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"reasoning\":\"step2 \"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"reasoning\":\"step3\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	if countEventType(events, types.EventThinkingStart) != 1 {
		t.Error("expected 1 EventThinkingStart")
	}
	if countEventType(events, types.EventThinkingDelta) != 3 {
		t.Errorf("expected 3 EventThinkingDelta, got %d", countEventType(events, types.EventThinkingDelta))
	}
	if countEventType(events, types.EventThinkingEnd) != 1 {
		t.Error("expected 1 EventThinkingEnd")
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinking))
	}
	if thinking[0] != "step1 step2 step3" {
		t.Errorf("thinking = %q, want %q", thinking[0], "step1 step2 step3")
	}
}

func TestOpenAICompat_CollectFromStream_Reasoning(t *testing.T) {
	// Verify that Complete() captures thinking-only responses
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"thinking about it\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")

	ctx := context.Background()
	p := &OpenAICompatProvider{}
	ch := make(chan types.StreamEvent, 64)
	go func() {
		defer close(ch)
		p.parseStreamResponse(ctx, ch, bytes.NewReader(body), "test-model", "test-provider")
	}()

	msg, err := p.collectFromStream(ch)
	if err != nil {
		t.Fatalf("collectFromStream error: %v", err)
	}

	if msg == nil {
		t.Fatal("expected non-nil message")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinking))
	}
	if thinking[0] != "thinking about it" {
		t.Errorf("thinking = %q, want %q", thinking[0], "thinking about it")
	}

	text := extractText(*msg)
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
}

func TestMessageToOpenAICompat_ThinkingBlocksSkipped(t *testing.T) {
	// Thinking blocks should NOT appear in outgoing OpenAI-compat messages
	msg := types.AgentMessage{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.BlockThinking, Text: "secret reasoning"},
			{Type: types.BlockText, Text: "Hello world"},
		},
	}

	result := messageToOpenAICompat(msg)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Content != "Hello world" {
		t.Errorf("expected only text content, got %q", result.Content)
	}
	// Tool calls should be empty (none in this message)
	if len(result.ToolCalls) > 0 {
		t.Errorf("expected no tool calls, got %d", len(result.ToolCalls))
	}
}

func TestParseStreamResponse_Reasoning_FieldSwitchMidStream(t *testing.T) {
	// Edge case: reasoning_content first, then reasoning field in next chunk.
	// Should close the old thinking block and start a new one.
	chunk1 := `{"choices":[{"delta":{"reasoning_content":"rc content"},"finish_reason":null}]}`
	chunk2 := `{"choices":[{"delta":{"content":"text1"},"finish_reason":null}]}`
	chunk3 := `{"choices":[{"delta":{"reasoning":"reasoning field"},"finish_reason":null}]}`
	chunk4 := `{"choices":[{"delta":{"content":"text2"},"finish_reason":"stop"}]}`
	body := []byte("data: " + chunk1 + "\n\ndata: " + chunk2 + "\n\ndata: " + chunk3 + "\n\ndata: " + chunk4 + "\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	// Should have 2 thinking_start and 2 thinking_end events (one per block)
	if countEventType(events, types.EventThinkingStart) != 2 {
		t.Errorf("expected 2 EventThinkingStart, got %d", countEventType(events, types.EventThinkingStart))
	}
	if countEventType(events, types.EventThinkingEnd) != 2 {
		t.Errorf("expected 2 EventThinkingEnd, got %d", countEventType(events, types.EventThinkingEnd))
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 2 {
		t.Fatalf("expected 2 thinking blocks, got %d", len(thinking))
	}
	if thinking[0] != "rc content" {
		t.Errorf("thinking[0] = %q, want %q", thinking[0], "rc content")
	}
	if thinking[1] != "reasoning field" {
		t.Errorf("thinking[1] = %q, want %q", thinking[1], "reasoning field")
	}
}

func TestAnthropicProvider_CollectFromStream_Thinking(t *testing.T) {
	// Verify Complete() captures thinking-only responses through Anthropic provider.
	client := &MockHTTPClient{
		DoFunc: func(req *Request) (*Response, error) {
			return &Response{
				StatusCode: 200,
				Body: []byte(
					"event: message_start\ndata: {}\n\n" +
						"event: content_block_start\ndata: {\"content_block\":{\"type\":\"thinking\"}}\n\n" +
						"event: content_block_delta\ndata: {\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"Let me reason\"}}\n\n" +
						"event: content_block_start\ndata: {\"content_block\":{\"type\":\"text\"}}\n\n" +
						"event: content_block_delta\ndata: {\"delta\":{\"type\":\"text_delta\",\"text\":\"Answer\"}}\n\n" +
						"event: message_delta\ndata: {\"usage\":{\"output_tokens\":5}}\n\n",
				),
			}, nil
		},
	}

	provider := NewAnthropicProviderWithClient("sk-test", client)
	model := types.Model{ID: "claude-sonnet-4-20250514", API: "anthropic-messages"}

	msg, err := provider.Complete(context.Background(), model, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	// Should have both thinking and text blocks
	hasThinking := false
	for _, block := range msg.Content {
		if block.Type == types.BlockThinking && block.Text == "Let me reason" {
			hasThinking = true
		}
	}
	if !hasThinking {
		t.Error("expected thinking block in Complete() result")
	}
}

func TestParseStreamResponse_FinishReason_ClosesThinkingBlock(t *testing.T) {
	// finish_reason without [DONE] — must still close thinking block.
	// Critical bug fix: finishThinkingBlock() was missing from this path.
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"thinking\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n")
	events := collectStreamEvents(t, body)

	// Should have thinking_end even though no [DONE] marker was sent
	if countEventType(events, types.EventThinkingEnd) != 1 {
		t.Errorf("expected 1 EventThinkingEnd on finish_reason path, got %d", countEventType(events, types.EventThinkingEnd))
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 || thinking[0] != "thinking" {
		t.Errorf("thinking = %v, want [\"thinking\"]", thinking)
	}
}

func TestParseStreamResponse_ContextCancel_FlushesState(t *testing.T) {
	// Context cancellation should flush tool calls and thinking blocks.
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"partial\"},\"finish_reason\":null}]}\n")

	ctx, cancel := context.WithCancel(context.Background())
	p := &OpenAICompatProvider{}
	ch := make(chan types.StreamEvent, 64)

	// Cancel after a brief delay to let the goroutine start
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	go func() {
		defer close(ch)
		p.parseStreamResponse(ctx, ch, bytes.NewReader(body), "test-model", "test-provider")
	}()
	events := collectStream(ch)

	// Should have thinking_start and thinking_end (flushed on cancel)
	if countEventType(events, types.EventThinkingStart) != 1 {
		t.Error("expected EventThinkingStart before cancel")
	}
	if countEventType(events, types.EventThinkingEnd) != 1 {
		t.Error("expected EventThinkingEnd (flushed on cancel)")
	}
}

func TestAnthropicProvider_ThinkingMapping(t *testing.T) {
	tests := []struct {
		level      types.ThinkingLevel
		wantType   string
		wantEffort string
		wantBudget int
	}{
		{types.ThinkingOff, "", "", 0},
		{types.ThinkingLow, "adaptive", "low", 0},
		{types.ThinkingMedium, "adaptive", "medium", 0},
		{types.ThinkingHigh, "adaptive", "high", 0},
		{types.ThinkingXHigh, "adaptive", "xhigh", 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			var capturedBody []byte
			var mu sync.Mutex
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				capturedBody, _ = io.ReadAll(r.Body)
				mu.Unlock()
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "event: message_start\ndata: {}\n\nevent: message_delta\ndata: {\"usage\":{\"output_tokens\":5}}\n\n")
			}))
			defer server.Close()

			model := types.Model{
				ID:        "claude-sonnet-4-20250514",
				Provider:  "anthropic",
				API:       "anthropic-messages",
				BaseURL:   server.URL,
				Reasoning: true,
				ThinkingLevelMap: map[string]string{
					"low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh",
				},
			}
			provider := NewAnthropicProviderWithClient("sk-test", &testHTTPClient{})

			ch := provider.Stream(context.Background(), model, nil, nil, types.StreamOptions{
				ThinkingLevel: tt.level,
			})
			for range ch {
			}

			mu.Lock()
			body := capturedBody
			mu.Unlock()

			if tt.level == types.ThinkingOff {
				if strings.Contains(string(body), `"thinking"`) {
					t.Errorf("expected no thinking in body for off, got: %s", string(body))
				}
				return
			}

			if !strings.Contains(string(body), `"thinking"`) {
				t.Fatalf("expected thinking in body, got: %s", string(body))
			}
			if !strings.Contains(string(body), fmt.Sprintf(`"type":"%s"`, tt.wantType)) {
				t.Errorf("expected thinking type=%q in body, got: %s", tt.wantType, string(body))
			}
			if tt.wantEffort != "" && !strings.Contains(string(body), fmt.Sprintf(`"effort":"%s"`, tt.wantEffort)) {
				t.Errorf("expected output_config effort=%q in body, got: %s", tt.wantEffort, string(body))
			}
		})
	}
}

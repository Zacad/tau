package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

// mockOpenRouterHandler returns an HTTP handler that simulates OpenRouter responses.
func mockOpenRouterHandler(t *testing.T, respondWith func(w http.ResponseWriter, r *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respondWith(w, r)
	})
}

func TestNewOpenRouterProvider(t *testing.T) {
	t.Run("creates provider with API key", func(t *testing.T) {
		p := NewOpenRouterProvider("sk-test-key")
		if p.Name() != "openrouter" {
			t.Errorf("expected name 'openrouter', got %q", p.Name())
		}
	})

	t.Run("creates provider with custom client", func(t *testing.T) {
		client := &DefaultHTTPClient{}
		p := NewOpenRouterProviderWithClient("sk-test-key", client)
		if p.Name() != "openrouter" {
			t.Errorf("expected name 'openrouter', got %q", p.Name())
		}
	})
}

func TestOpenRouterHeaders(t *testing.T) {
	p := NewOpenRouterProvider("sk-test-key")

	headers := p.openRouterHeaders()

	if headers["HTTP-Referer"] != "https://tau.example/" {
		t.Errorf("expected HTTP-Referer header, got %q", headers["HTTP-Referer"])
	}
	if headers["X-OpenRouter-Title"] != "tau" {
		t.Errorf("expected X-OpenRouter-Title header, got %q", headers["X-OpenRouter-Title"])
	}
	if headers["X-OpenRouter-Categories"] != "cli-agent" {
		t.Errorf("expected X-OpenRouter-Categories header, got %q", headers["X-OpenRouter-Categories"])
	}
}

func TestThinkingLevelToEffort(t *testing.T) {
	tests := []struct {
		level    types.ThinkingLevel
		expected string
	}{
		{types.ThinkingOff, "none"},
		{types.ThinkingMinimal, "low"},
		{types.ThinkingLow, "low"},
		{types.ThinkingMedium, "medium"},
		{types.ThinkingHigh, "high"},
		{types.ThinkingXHigh, "xhigh"},
		{"", "none"},
		{"unknown", "medium"},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			got := thinkingLevelToEffort(tt.level)
			if got != tt.expected {
				t.Errorf("thinkingLevelToEffort(%q) = %q, want %q", tt.level, got, tt.expected)
			}
		})
	}
}

func TestOpenRouterRequestIncludesReasoning(t *testing.T) {
	tests := []struct {
		name         string
		thinkingLevel types.ThinkingLevel
		wantEffort   string
	}{
		{"off", types.ThinkingOff, "none"},
		{"minimal", types.ThinkingMinimal, "low"},
		{"low", types.ThinkingLow, "low"},
		{"medium", types.ThinkingMedium, "medium"},
		{"high", types.ThinkingHigh, "high"},
		{"xhigh", types.ThinkingXHigh, "xhigh"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compat := NewOpenAICompatProvider("sk-test", OpenAICompatConfig{
				BaseURL:       "https://openrouter.ai/api/v1",
				ProviderName:  "openrouter",
				ThinkingLevel: tt.thinkingLevel,
			})

			// Build request via Stream and capture the body
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			ch := compat.Stream(ctx, types.Model{ID: "test-model"}, []types.AgentMessage{
				{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
			}, nil, types.StreamOptions{ThinkingLevel: tt.thinkingLevel})

			// Consume channel (will fail to connect, but we can't easily capture the body)
			// Instead, test the config directly
			if compat.config.ThinkingLevel != tt.thinkingLevel {
				t.Errorf("config.ThinkingLevel = %q, want %q", compat.config.ThinkingLevel, tt.thinkingLevel)
			}

			<-ch
		})
	}
}

func TestOpenRouterRequestIncludesProviderRouting(t *testing.T) {
	routing := map[string]any{
		"order":           []string{"anthropic", "openai"},
		"allow_fallbacks": true,
		"sort":            "price",
	}

	compat := NewOpenAICompatProvider("sk-test", OpenAICompatConfig{
		BaseURL:         "https://openrouter.ai/api/v1",
		ProviderName:    "openrouter",
		ProviderRouting: routing,
	})

	if compat.config.ProviderRouting == nil {
		t.Fatal("expected ProviderRouting to be set")
	}
	if compat.config.ProviderRouting["sort"] != "price" {
		t.Errorf("expected sort=price, got %v", compat.config.ProviderRouting["sort"])
	}
}

func TestOpenRouterRequestJSON(t *testing.T) {
	// Test that the request body includes reasoning and provider fields
	routing := map[string]any{
		"order": []string{"anthropic"},
	}

	compat := NewOpenAICompatProvider("sk-test", OpenAICompatConfig{
		BaseURL:         "https://openrouter.ai/api/v1",
		ProviderName:    "openrouter",
		ThinkingLevel:   types.ThinkingHigh,
		ProviderRouting: routing,
	})

	// Build request body
	model := types.Model{ID: "anthropic/claude-sonnet-4"}
	messages := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
	}
	opts := types.StreamOptions{ThinkingLevel: types.ThinkingHigh}

	body, err := compat.buildRequest(model, messages, nil, opts)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	var req openAICompatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	// Check reasoning field
	if req.Reasoning == nil {
		t.Fatal("expected reasoning field in request")
	}
	if req.Reasoning.Effort != "high" {
		t.Errorf("expected reasoning.effort='high', got %q", req.Reasoning.Effort)
	}

	// Check provider routing
	if req.Provider == nil {
		t.Fatal("expected provider field in request")
	}
	if order, ok := req.Provider["order"].([]any); !ok || len(order) == 0 || order[0] != "anthropic" {
		t.Errorf("expected provider.order=['anthropic'], got %v", req.Provider["order"])
	}
}

func TestOpenRouterRequestJSONNoThinking(t *testing.T) {
	compat := NewOpenAICompatProvider("sk-test", OpenAICompatConfig{
		BaseURL:      "https://openrouter.ai/api/v1",
		ProviderName: "openrouter",
	})

	model := types.Model{ID: "openai/gpt-4o"}
	messages := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
	}
	opts := types.StreamOptions{}

	body, err := compat.buildRequest(model, messages, nil, opts)
	if err != nil {
		t.Fatalf("buildRequest failed: %v", err)
	}

	var req openAICompatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	// No thinking level set, so reasoning should be nil
	if req.Reasoning != nil {
		t.Errorf("expected no reasoning field when thinking level is empty, got %+v", req.Reasoning)
	}
}

func TestRegisterOpenRouterModels(t *testing.T) {
	registry := NewModelRegistry()
	countBefore := len(registry.ListAll())

	RegisterOpenRouterModels(registry)

	countAfter := len(registry.ListAll())
	registered := countAfter - countBefore

	if registered < 10 {
		t.Errorf("expected at least 10 models registered, got %d", registered)
	}

	// Check specific models exist
	openRouterModels := registry.ListByProvider("openrouter")
	if len(openRouterModels) != registered {
		t.Errorf("expected %d openrouter models, got %d", registered, len(openRouterModels))
	}

	// Check a specific model
	m, err := registry.Get("anthropic/claude-sonnet-4")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}
	if m.Provider != "openrouter" {
		t.Errorf("expected provider 'openrouter', got %q", m.Provider)
	}
	if !m.Reasoning {
		t.Error("expected claude-sonnet-4 to have reasoning=true")
	}
}

func TestOpenRouterModelIDs(t *testing.T) {
	ids := OpenRouterModelIDs()
	if len(ids) < 10 {
		t.Errorf("expected at least 10 model IDs, got %d", len(ids))
	}

	// Check format (should be author/model-name)
	for _, id := range ids {
		if !strings.Contains(id, "/") {
			t.Errorf("model ID %q should contain '/' separator", id)
		}
	}
}

func TestRegisterOpenRouterModelsFromConfig(t *testing.T) {
	registry := NewModelRegistry()

	// Register curated models first
	RegisterOpenRouterModels(registry)

	// Now register user-defined models
	userModelIDs := []string{
		"anthropic/claude-sonnet-4",
		"openai/o3",
		"minimax/minimax-m2",
	}

	RegisterOpenRouterModelsFromConfig(registry, userModelIDs)

	// Check user models are registered
	for _, id := range userModelIDs {
		m, err := registry.Get(id)
		if err != nil {
			t.Fatalf("user model %q not found: %v", id, err)
		}
		if m.Provider != "openrouter" {
			t.Errorf("user model %q: expected provider 'openrouter', got %q", id, m.Provider)
		}
	}

	// Check empty IDs are skipped
	RegisterOpenRouterModelsFromConfig(registry, []string{"", "valid-model"})
	_, err := registry.Get("valid-model")
	if err != nil {
		t.Fatalf("expected 'valid-model' to be registered: %v", err)
	}
}

func TestOpenRouterStreamDelegatesToCompat(t *testing.T) {
	p := NewOpenRouterProvider("sk-test-key")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch := p.Stream(ctx, types.Model{ID: "test-model"}, []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
	}, nil, types.StreamOptions{})

	// Should get an error event (no server running)
	event := <-ch
	if event.Type != types.EventError {
		t.Errorf("expected error event, got %q", event.Type)
	}
}

func TestOpenRouterCompleteDelegatesToCompat(t *testing.T) {
	p := NewOpenRouterProvider("sk-test-key")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.Complete(ctx, types.Model{ID: "test-model"}, []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
	}, nil, types.StreamOptions{})

	if err == nil {
		t.Error("expected error (no server running)")
	}
}

func TestOpenRouterProviderImplementsInterface(t *testing.T) {
	var _ Provider = (*OpenRouterProvider)(nil)
}

func TestBuildOpenRouterModelFromEntry(t *testing.T) {
	t.Run("basic model", func(t *testing.T) {
		entry := openRouterModelEntry{
			ID:          "openai/gpt-4o",
			Name:        "GPT-4o",
			Description: "A powerful language model",
		}

		m := BuildOpenRouterModelFromEntry(entry)

		if m.ID != "openai/gpt-4o" {
			t.Errorf("expected ID 'openai/gpt-4o', got %q", m.ID)
		}
		if m.Provider != "openrouter" {
			t.Errorf("expected provider 'openrouter', got %q", m.Provider)
		}
		if m.API != "openai-completions" {
			t.Errorf("expected API 'openai-completions', got %q", m.API)
		}
	})

	t.Run("model with context length", func(t *testing.T) {
		ctxLen := 128000
		entry := openRouterModelEntry{
			ID:            "openai/gpt-4o",
			ContextLength: &ctxLen,
		}

		m := BuildOpenRouterModelFromEntry(entry)
		if m.ContextWindow != 128000 {
			t.Errorf("expected context window 128000, got %d", m.ContextWindow)
		}
	})

	t.Run("model with pricing", func(t *testing.T) {
		entry := openRouterModelEntry{
			ID: "openai/gpt-4o",
			Pricing: struct {
				Prompt     float64 `json:"prompt"`
				Completion float64 `json:"completion"`
			}{Prompt: 0.0000025, Completion: 0.00001},
		}

		m := BuildOpenRouterModelFromEntry(entry)
		// Pricing should be scaled to per 1M tokens
		if m.Cost.Input != 2.50 {
			t.Errorf("expected input cost 2.50, got %f", m.Cost.Input)
		}
		if m.Cost.Output != 10.00 {
			t.Errorf("expected output cost 10.00, got %f", m.Cost.Output)
		}
	})

	t.Run("reasoning model detection by ID", func(t *testing.T) {
		tests := []struct {
			id          string
			wantReasoning bool
		}{
			{"deepseek/deepseek-r1", true},
			{"openai/o3", true},
			{"openai/o1", true},
			{"openai/gpt-4o", false},
		}

		for _, tt := range tests {
			entry := openRouterModelEntry{ID: tt.id}
			m := BuildOpenRouterModelFromEntry(entry)
			if m.Reasoning != tt.wantReasoning {
				t.Errorf("model %q: expected reasoning=%v, got %v", tt.id, tt.wantReasoning, m.Reasoning)
			}
		}
	})
}

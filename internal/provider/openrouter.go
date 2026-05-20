package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/adam/tau/internal/types"
)

// OpenRouterProvider implements the Provider interface for OpenRouter.
// OpenRouter provides a unified API gateway to 300+ AI models through
// a single API key, with automatic provider routing and fallback.
//
// It composes OpenAICompatProvider internally to reuse all SSE parsing,
// delta accumulation, and tool call handling. OpenRouter-specific behavior
// is layered on top: attribution headers, thinking level mapping, and
// provider routing preferences.
type OpenRouterProvider struct {
	baseProvider
	compat *OpenAICompatProvider
}

// NewOpenRouterProvider creates a new OpenRouter provider.
func NewOpenRouterProvider(apiKey string) *OpenRouterProvider {
	return NewOpenRouterProviderWithClient(apiKey, &DefaultHTTPClient{})
}

// NewOpenRouterProviderWithClient creates a new OpenRouter provider with a custom HTTP client.
func NewOpenRouterProviderWithClient(apiKey string, client HTTPClient) *OpenRouterProvider {
	compat := NewOpenAICompatProviderWithClient(apiKey, OpenAICompatConfig{
		BaseURL:      "https://openrouter.ai/api/v1",
		ProviderName: "openrouter",
	}, client)

	return &OpenRouterProvider{
		baseProvider: baseProvider{
			name:       "openrouter",
			httpClient: client,
			apiKey:     apiKey,
		},
		compat: compat,
	}
}

// Name returns the provider identifier.
func (p *OpenRouterProvider) Name() string {
	return "openrouter"
}

// Stream sends messages to OpenRouter and returns a channel of streaming events.
func (p *OpenRouterProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	// Build provider routing preferences from model.Compat
	var routing map[string]any
	if model.Compat != nil {
		if r, ok := model.Compat["routing"].(map[string]any); ok {
			routing = r
		}
	}

	// Create a compat provider with OpenRouter-specific config
	compat := NewOpenAICompatProviderWithClient(p.apiKey, OpenAICompatConfig{
		BaseURL:         "https://openrouter.ai/api/v1",
		ProviderName:    "openrouter",
		ThinkingLevel:   opts.ThinkingLevel,
		ProviderRouting: routing,
		ExtraHeaders:    p.openRouterHeaders(),
	}, p.httpClient)

	return compat.Stream(ctx, model, messages, tools, opts)
}

// Complete sends messages to OpenRouter and returns the full response.
func (p *OpenRouterProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	events := p.Stream(ctx, model, messages, tools, opts)
	return p.compat.collectFromStream(events)
}

// openRouterHeaders returns the HTTP headers for OpenRouter requests,
// including attribution headers per OpenRouter's app attribution spec.
func (p *OpenRouterProvider) openRouterHeaders() map[string]string {
	return map[string]string{
		"HTTP-Referer":          "https://tau.example/",
		"X-OpenRouter-Title":    "tau",
		"X-OpenRouter-Categories": "cli-agent",
	}
}

// Ensure OpenRouterProvider implements Provider
var _ Provider = (*OpenRouterProvider)(nil)

// openRouterModelResponse is the JSON response from OpenRouter's /models endpoint.
type openRouterModelResponse struct {
	Data []openRouterModelEntry `json:"data"`
}

type openRouterModelEntry struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	ContextLength *int    `json:"context_length,omitempty"`
	MaxTokens     *int    `json:"max_tokens,omitempty"`
	Pricing       struct {
		Prompt  float64 `json:"prompt"`
		Completion float64 `json:"completion"`
	} `json:"pricing,omitempty"`
}

// openRouterModels is the curated list of popular OpenRouter models.
var openRouterModels = []types.Model{
	// OpenAI — latest generation
	{
		ID:            "openai/gpt-5.5",
		Name:          "GPT-5.5",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 2.50, Output: 10.00},
		ContextWindow: 400000,
		MaxTokens:     128000,
	},
	{
		ID:            "openai/gpt-5.5-pro",
		Name:          "GPT-5.5 Pro",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 10.00, Output: 40.00},
		ContextWindow: 400000,
		MaxTokens:     128000,
	},
	{
		ID:            "openai/gpt-5.4",
		Name:          "GPT-5.4",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 1.25, Output: 5.00},
		ContextWindow: 400000,
		MaxTokens:     128000,
	},
	{
		ID:            "openai/gpt-5.4-mini",
		Name:          "GPT-5.4 Mini",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.25, Output: 1.00},
		ContextWindow: 400000,
		MaxTokens:     128000,
	},
	{
		ID:            "openai/gpt-5.4-pro",
		Name:          "GPT-5.4 Pro",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 5.00, Output: 20.00},
		ContextWindow: 400000,
		MaxTokens:     128000,
	},
	{
		ID:            "openai/gpt-5-chat",
		Name:          "GPT-5",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 1.25, Output: 10.00},
		ContextWindow: 400000,
		MaxTokens:     128000,
	},
	{
		ID:            "openai/gpt-5-mini",
		Name:          "GPT-5 Mini",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.25, Output: 2.00},
		ContextWindow: 400000,
		MaxTokens:     128000,
	},
	{
		ID:            "openai/gpt-5-nano",
		Name:          "GPT-5 Nano",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 0.05, Output: 0.40},
		ContextWindow: 400000,
		MaxTokens:     128000,
	},
	{
		ID:            "openai/gpt-4.1",
		Name:          "GPT-4.1",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 2.00, Output: 8.00},
		ContextWindow: 1047576,
		MaxTokens:     32768,
	},
	{
		ID:            "openai/gpt-4.1-mini",
		Name:          "GPT-4.1 Mini",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.40, Output: 1.60},
		ContextWindow: 1047576,
		MaxTokens:     32768,
	},
	{
		ID:            "openai/o3",
		Name:          "o3",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 10.00, Output: 40.00},
		ContextWindow: 200000,
		MaxTokens:     100000,
	},
	{
		ID:            "openai/o3-mini",
		Name:          "o3 Mini",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 1.10, Output: 4.40},
		ContextWindow: 200000,
		MaxTokens:     100000,
	},
	{
		ID:            "openai/o4-mini",
		Name:          "o4 Mini",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 1.10, Output: 4.40},
		ContextWindow: 200000,
		MaxTokens:     100000,
	},
	// Anthropic — Claude
	{
		ID:            "anthropic/claude-sonnet-4",
		Name:          "Claude Sonnet 4",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 3.00, Output: 15.00},
		ContextWindow: 200000,
		MaxTokens:     8192,
	},
	{
		ID:            "anthropic/claude-sonnet-4.6",
		Name:          "Claude Sonnet 4.6",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 3.00, Output: 15.00},
		ContextWindow: 200000,
		MaxTokens:     8192,
	},
	{
		ID:            "anthropic/claude-opus-4.6",
		Name:          "Claude Opus 4.6",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 15.00, Output: 75.00},
		ContextWindow: 200000,
		MaxTokens:     8192,
	},
	{
		ID:            "anthropic/claude-opus-4.7",
		Name:          "Claude Opus 4.7",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 15.00, Output: 75.00},
		ContextWindow: 200000,
		MaxTokens:     8192,
	},
	{
		ID:            "anthropic/claude-haiku-4.5",
		Name:          "Claude Haiku 4.5",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.25, Output: 1.25},
		ContextWindow: 200000,
		MaxTokens:     8192,
	},
	// Google — Gemini
	{
		ID:            "google/gemini-2.5-pro",
		Name:          "Gemini 2.5 Pro",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 1.25, Output: 10.00},
		ContextWindow: 1000000,
		MaxTokens:     65536,
	},
	{
		ID:            "google/gemini-2.5-flash",
		Name:          "Gemini 2.5 Flash",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.15, Output: 0.60},
		ContextWindow: 1000000,
		MaxTokens:     65536,
	},
	{
		ID:            "google/gemini-2.5-flash-lite",
		Name:          "Gemini 2.5 Flash Lite",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.0375, Output: 0.15},
		ContextWindow: 1000000,
		MaxTokens:     65536,
	},
	{
		ID:            "google/gemini-3.1-pro-preview",
		Name:          "Gemini 3.1 Pro",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 1.25, Output: 10.00},
		ContextWindow: 1000000,
		MaxTokens:     65536,
	},
	{
		ID:            "google/gemini-3.1-flash-lite",
		Name:          "Gemini 3.1 Flash Lite",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.0375, Output: 0.15},
		ContextWindow: 1000000,
		MaxTokens:     65536,
	},
	// DeepSeek
	{
		ID:            "deepseek/deepseek-chat",
		Name:          "DeepSeek V3",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 0.27, Output: 1.10},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	{
		ID:            "deepseek/deepseek-r1",
		Name:          "DeepSeek R1",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 0.55, Output: 2.19},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	{
		ID:            "deepseek/deepseek-v3.2",
		Name:          "DeepSeek V3.2",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 0.27, Output: 1.10},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	// Mistral
	{
		ID:            "mistralai/mistral-large-2411",
		Name:          "Mistral Large 2",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 2.00, Output: 6.00},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	{
		ID:            "mistralai/mistral-medium-3.1",
		Name:          "Mistral Medium 3.1",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 0.40, Output: 2.00},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	{
		ID:            "mistralai/ministral-8b-2512",
		Name:          "Ministral 8B",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 0.10, Output: 0.10},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	// Meta — Llama
	{
		ID:            "meta-llama/llama-3.3-70b-instruct",
		Name:          "Llama 3.3 70B",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 0.20, Output: 0.20},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	{
		ID:            "meta-llama/llama-4-maverick",
		Name:          "Llama 4 Maverick",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.20, Output: 0.60},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	{
		ID:            "meta-llama/llama-4-scout",
		Name:          "Llama 4 Scout",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.15, Output: 0.40},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	// Qwen
	{
		ID:            "qwen/qwen3-coder",
		Name:          "Qwen3 Coder",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 0.10, Output: 0.10},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	{
		ID:            "qwen/qwen3-max",
		Name:          "Qwen3 Max",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 1.60, Output: 6.40},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	// MiniMax
	{
		ID:            "minimax/minimax-m2.5",
		Name:          "MiniMax M2.5",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text"},
		Cost:          types.CostInfo{Input: 0.20, Output: 0.60},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	// xAI — Grok
	{
		ID:            "x-ai/grok-4",
		Name:          "Grok 4",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     true,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 3.00, Output: 15.00},
		ContextWindow: 128000,
		MaxTokens:     8192,
	},
	// Amazon — Nova
	{
		ID:            "amazon/nova-lite-v1",
		Name:          "Nova Lite",
		Provider:      "openrouter",
		API:           "openai-completions",
		BaseURL:       "https://openrouter.ai/api/v1",
		Reasoning:     false,
		InputTypes:    []string{"text", "image"},
		Cost:          types.CostInfo{Input: 0.06, Output: 0.24},
		ContextWindow: 300000,
		MaxTokens:     8192,
	},
}

// RegisterOpenRouterModels registers the curated OpenRouter model list
// into the model registry.
func RegisterOpenRouterModels(registry *ModelRegistry) {
	for _, m := range openRouterModels {
		registry.Register(m)
	}
}

// DiscoverOpenRouterModels fetches models from OpenRouter's /models endpoint,
// filters to popular providers, sorts by context length, and returns the top 30.
func DiscoverOpenRouterModels(apiKey string) ([]string, error) {
	const modelsURL = "https://openrouter.ai/api/v1/models"

	req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var modelsResp openRouterModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return filterAndSortModels(modelsResp.Data, 30), nil
}

// DiscoverOpenRouterModelsWithClient fetches models from OpenRouter's /models
// endpoint using a custom HTTP client, filters to popular providers, sorts by
// context length, and returns the top 30.
func DiscoverOpenRouterModelsWithClient(apiKey string, client HTTPClient) ([]string, error) {
	const modelsURL = "https://openrouter.ai/api/v1/models"

	httpReq, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(&Request{
		Method:  http.MethodGet,
		URL:     modelsURL,
		Headers: map[string]string{"Authorization": "Bearer " + apiKey},
	})
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(resp.Body))
	}

	var modelsResp openRouterModelResponse
	if err := json.Unmarshal(resp.Body, &modelsResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return filterAndSortModels(modelsResp.Data, 30), nil
}

// openRouterPopularPrefixes lists provider prefixes to include in model discovery.
var openRouterPopularPrefixes = []string{
	"openai/", "anthropic/", "google/", "mistralai/", "deepseek/",
	"meta-llama/", "qwen/", "x-ai/", "minimax/", "amazon/", "cohere/",
}

// filterAndSortModels filters models to popular providers, ensures diversity
// across providers, sorts by context length, and returns the top N model IDs.
func filterAndSortModels(entries []openRouterModelEntry, limit int) []string {
	type modelEntry struct {
		ID            string
		Provider      string
		ContextLength int
	}

	// Group by provider
	byProvider := make(map[string][]modelEntry)
	for _, e := range entries {
		for _, prefix := range openRouterPopularPrefixes {
			if len(e.ID) > len(prefix) && e.ID[:len(prefix)] == prefix {
				ctxLen := 0
				if e.ContextLength != nil {
					ctxLen = *e.ContextLength
				}
				provider := e.ID[:len(prefix)-1]
				byProvider[provider] = append(byProvider[provider], modelEntry{
					ID: e.ID, Provider: provider, ContextLength: ctxLen,
				})
				break
			}
		}
	}

	// Sort each provider's models by context_length descending
	for _, models := range byProvider {
		for i := 0; i < len(models); i++ {
			for j := i + 1; j < len(models); j++ {
				if models[j].ContextLength > models[i].ContextLength {
					models[i], models[j] = models[j], models[i]
				}
			}
		}
	}

	// Round-robin: take top model from each provider, then second from each, etc.
	var result []modelEntry
	maxPerProvider := 0
	for _, models := range byProvider {
		if len(models) > maxPerProvider {
			maxPerProvider = len(models)
		}
	}
	for i := 0; i < maxPerProvider; i++ {
		for _, provider := range openRouterPopularPrefixes {
			name := provider[:len(provider)-1]
			models := byProvider[name]
			if i < len(models) {
				result = append(result, models[i])
			}
		}
	}

	if limit > len(result) {
		limit = len(result)
	}

	modelIDs := make([]string, limit)
	for i := 0; i < limit; i++ {
		modelIDs[i] = result[i].ID
	}
	return modelIDs
}

// TestOpenRouterConnection tests connectivity to OpenRouter with the given API key.
func TestOpenRouterConnection(apiKey string) error {
	const modelsURL = "https://openrouter.ai/api/v1/models"

	req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		apiErr := types.ClassifyAPIError(resp.StatusCode, bodyBytes)
		return fmt.Errorf("%s", apiErr.UserMessage())
	}

	return nil
}

// OpenRouterModelIDs returns the curated model IDs for registration.
func OpenRouterModelIDs() []string {
	ids := make([]string, len(openRouterModels))
	for i, m := range openRouterModels {
		ids[i] = m.ID
	}
	return ids
}

// RegisterOpenRouterModelsFromConfig registers user-defined models from config
// into the model registry.
func RegisterOpenRouterModelsFromConfig(registry *ModelRegistry, modelIDs []string) {
	baseURL := "https://openrouter.ai/api/v1"
	for _, id := range modelIDs {
		if id == "" {
			continue
		}
		// Check if already registered (curated list)
		if _, err := registry.Get(id); err == nil {
			continue
		}
		registry.Register(types.Model{
			ID:       id,
			Name:     id,
			Provider: "openrouter",
			API:      "openai-completions",
			BaseURL:  baseURL,
		})
	}
}

// BuildOpenRouterModelFromEntry creates a types.Model from an OpenRouter model entry.
func BuildOpenRouterModelFromEntry(entry openRouterModelEntry) types.Model {
	m := types.Model{
		ID:       entry.ID,
		Name:     entry.Name,
		Provider: "openrouter",
		API:      "openai-completions",
		BaseURL:  "https://openrouter.ai/api/v1",
	}

	if entry.ContextLength != nil {
		m.ContextWindow = *entry.ContextLength
	}
	if entry.MaxTokens != nil {
		m.MaxTokens = *entry.MaxTokens
	}
	if entry.Pricing.Prompt > 0 || entry.Pricing.Completion > 0 {
		// OpenRouter pricing is per-token (not per 1M tokens), so multiply
		m.Cost = types.CostInfo{
			Input:  entry.Pricing.Prompt * 1_000_000,
			Output: entry.Pricing.Completion * 1_000_000,
		}
	}
	if strings.Contains(strings.ToLower(entry.Description), "reason") ||
		strings.Contains(strings.ToLower(entry.Description), "thinking") ||
		strings.Contains(strings.ToLower(entry.ID), "r1") ||
		strings.Contains(strings.ToLower(entry.ID), "o1") ||
		strings.Contains(strings.ToLower(entry.ID), "o3") {
		m.Reasoning = true
		m.ThinkingLevelMap = map[string]string{
			"off":    "none",
			"low":    "low",
			"medium": "medium",
			"high":   "high",
		}
	}

	return m
}

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/adam/tau/internal/types"
)

const (
	zenAPIOpenAICompletions = "openai-completions"
	zenAPIOpenAIResponses   = "openai-responses"
	zenAPIAnthropicMessages = "anthropic-messages"
	zenAPIGoogleGenerative  = "google-generative-ai"
)

// ZenProvider implements the Provider interface for OpenCode Zen.
// It routes models to the correct underlying provider based on model.API:
//   - gpt-* models → OpenAI Responses API (/responses)
//   - claude-* models → Anthropic Messages API (/messages)
//   - gemini-* models → Google Generative AI (/models/{id})
//   - all others → OpenAI Chat Completions (/chat/completions)
type ZenProvider struct {
	baseProvider
	openAI       *OpenAIProvider
	anthropic    *AnthropicProvider
	google       *GoogleProvider
	openAICompat *OpenAICompatProvider
}

// NewZenProvider creates a new OpenCode Zen provider with the given API key.
// All sub-providers are configured to use the Zen base URL.
func NewZenProvider(apiKey string) *ZenProvider {
	const zenBaseURL = "https://opencode.ai/zen/v1"

	return &ZenProvider{
		baseProvider: baseProvider{
			name:       "opencode-zen",
			httpClient: &DefaultHTTPClient{},
			apiKey:     apiKey,
		},
		openAI: NewOpenAIProviderWithClient(apiKey, &DefaultHTTPClient{}),
		anthropic: &AnthropicProvider{
			baseProvider: baseProvider{
				name:       "opencode-zen",
				httpClient: &DefaultHTTPClient{},
				apiKey:     apiKey,
			},
			apiVersion: "2023-06-01",
		},
		google: NewGoogleProviderWithConfig(apiKey, GoogleConfig{
			AuthMode: "bearer",
		}, &DefaultHTTPClient{}),
		openAICompat: NewOpenAICompatProvider(apiKey, OpenAICompatConfig{
			BaseURL:      zenBaseURL,
			APIPath:      "/chat/completions",
			ProviderName: "opencode-zen",
		}),
	}
}

// NewZenProviderWithClient creates a new Zen provider with a custom HTTP client.
func NewZenProviderWithClient(apiKey string, client HTTPClient) *ZenProvider {
	const zenBaseURL = "https://opencode.ai/zen/v1"

	return &ZenProvider{
		baseProvider: baseProvider{
			name:       "opencode-zen",
			httpClient: client,
			apiKey:     apiKey,
		},
		openAI: NewOpenAIProviderWithClient(apiKey, client),
		anthropic: &AnthropicProvider{
			baseProvider: baseProvider{
				name:       "opencode-zen",
				httpClient: client,
				apiKey:     apiKey,
			},
			apiVersion: "2023-06-01",
		},
		google: NewGoogleProviderWithConfig(apiKey, GoogleConfig{
			AuthMode: "bearer",
		}, client),
		openAICompat: NewOpenAICompatProviderWithClient(apiKey, OpenAICompatConfig{
			BaseURL:      zenBaseURL,
			APIPath:      "/chat/completions",
			ProviderName: "opencode-zen",
		}, client),
	}
}

// Stream sends messages to the appropriate Zen sub-provider based on model.API.
func (p *ZenProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	provider := p.routeProvider(model)
	return provider.Stream(ctx, model, messages, tools, opts)
}

// Complete sends messages to the appropriate Zen sub-provider based on model.API.
func (p *ZenProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	provider := p.routeProvider(model)
	return provider.Complete(ctx, model, messages, tools, opts)
}

// routeProvider selects the correct sub-provider based on the model's API type.
func (p *ZenProvider) routeProvider(model types.Model) Provider {
	switch model.API {
	case zenAPIOpenAIResponses:
		return p.openAI
	case zenAPIAnthropicMessages:
		return p.anthropic
	case zenAPIGoogleGenerative:
		return p.google
	default:
		return p.openAICompat
	}
}

// Ensure ZenProvider implements Provider
var _ Provider = (*ZenProvider)(nil)

// ClassifyZenModelAPI returns the correct API type for a Zen model based on its ID.
func ClassifyZenModelAPI(modelID string) string {
	switch {
	case len(modelID) >= 4 && modelID[:4] == "gpt-":
		return zenAPIOpenAIResponses
	case len(modelID) >= 7 && modelID[:7] == "claude-":
		return zenAPIAnthropicMessages
	case len(modelID) >= 7 && modelID[:7] == "gemini-":
		return zenAPIGoogleGenerative
	default:
		return zenAPIOpenAICompletions
	}
}

// ZenModelName returns a human-readable name for a Zen model.
func ZenModelName(modelID string) string {
	replacements := map[string]string{
		"gpt-5.5":          "GPT 5.5",
		"gpt-5.5-pro":      "GPT 5.5 Pro",
		"gpt-5.4":          "GPT 5.4",
		"gpt-5.4-pro":      "GPT 5.4 Pro",
		"gpt-5.4-mini":     "GPT 5.4 Mini",
		"gpt-5.4-nano":     "GPT 5.4 Nano",
		"gpt-5.3-codex":    "GPT 5.3 Codex",
		"gpt-5.3-codex-spark": "GPT 5.3 Codex Spark",
		"gpt-5.2":          "GPT 5.2",
		"gpt-5.2-codex":    "GPT 5.2 Codex",
		"gpt-5.1":          "GPT 5.1",
		"gpt-5.1-codex":    "GPT 5.1 Codex",
		"gpt-5.1-codex-max": "GPT 5.1 Codex Max",
		"gpt-5.1-codex-mini": "GPT 5.1 Codex Mini",
		"gpt-5":            "GPT 5",
		"gpt-5-codex":      "GPT 5 Codex",
		"gpt-5-nano":       "GPT 5 Nano",
		"claude-opus-4-7":  "Claude Opus 4.7",
		"claude-opus-4-6":  "Claude Opus 4.6",
		"claude-opus-4-5":  "Claude Opus 4.5",
		"claude-opus-4-1":  "Claude Opus 4.1",
		"claude-sonnet-4-6": "Claude Sonnet 4.6",
		"claude-sonnet-4-5": "Claude Sonnet 4.5",
		"claude-sonnet-4":  "Claude Sonnet 4",
		"claude-haiku-4-5": "Claude Haiku 4.5",
		"claude-3-5-haiku": "Claude Haiku 3.5",
		"gemini-3.1-pro":   "Gemini 3.1 Pro",
		"gemini-3-flash":   "Gemini 3 Flash",
		"qwen3.6-plus":     "Qwen3.6 Plus",
		"qwen3.5-plus":     "Qwen3.5 Plus",
		"minimax-m2.7":     "MiniMax M2.7",
		"minimax-m2.5":     "MiniMax M2.5",
		"minimax-m2.5-free": "MiniMax M2.5 Free",
		"glm-5.1":          "GLM 5.1",
		"glm-5":            "GLM 5",
		"kimi-k2.5":        "Kimi K2.5",
		"kimi-k2.6":        "Kimi K2.6",
		"big-pickle":       "Big Pickle",
		"deepseek-v4-flash-free": "DeepSeek V4 Flash Free",
		"ring-2.6-1t-free": "Ring 2.6 1T Free",
		"nemotron-3-super-free": "Nemotron 3 Super Free",
	}
	if name, ok := replacements[modelID]; ok {
		return name
	}
	return modelID
}

// ZenModelReasoning returns whether a Zen model supports reasoning/thinking.
func ZenModelReasoning(modelID string) bool {
	// GPT models with reasoning support
	gptReasoning := map[string]bool{
		"gpt-5.5":          true,
		"gpt-5.5-pro":      true,
		"gpt-5.4":          true,
		"gpt-5.4-pro":      true,
		"gpt-5.4-mini":     true,
		"gpt-5.3-codex":    true,
		"gpt-5.3-codex-spark": true,
		"gpt-5.2":          true,
		"gpt-5.2-codex":    true,
		"gpt-5.1":          true,
		"gpt-5.1-codex":    true,
		"gpt-5.1-codex-max": true,
		"gpt-5.1-codex-mini": true,
		"gpt-5":            true,
		"gpt-5-codex":      true,
	}
	// Claude models with reasoning support
	claudeReasoning := map[string]bool{
		"claude-opus-4-7":   true,
		"claude-opus-4-6":   true,
		"claude-opus-4-5":   true,
		"claude-opus-4-1":   true,
		"claude-sonnet-4-6": true,
		"claude-sonnet-4-5": true,
		"claude-sonnet-4":   true,
		"claude-haiku-4-5":  true,
	}
	// Gemini models with reasoning support
	geminiReasoning := map[string]bool{
		"gemini-3.1-pro": true,
		"gemini-3-flash": true,
	}

	return gptReasoning[modelID] || claudeReasoning[modelID] || geminiReasoning[modelID]
}

// ZenModelContextWindow returns the context window size for a Zen model.
func ZenModelContextWindow(modelID string) int {
	contextWindows := map[string]int{
		// GPT models
		"gpt-5.5":          272000,
		"gpt-5.5-pro":      272000,
		"gpt-5.4":          272000,
		"gpt-5.4-pro":      272000,
		"gpt-5.4-mini":     272000,
		"gpt-5.4-nano":     272000,
		"gpt-5.3-codex":    272000,
		"gpt-5.3-codex-spark": 272000,
		"gpt-5.2":          272000,
		"gpt-5.2-codex":    272000,
		"gpt-5.1":          272000,
		"gpt-5.1-codex":    272000,
		"gpt-5.1-codex-max": 272000,
		"gpt-5.1-codex-mini": 272000,
		"gpt-5":            272000,
		"gpt-5-codex":      272000,
		"gpt-5-nano":       272000,
		// Claude models
		"claude-opus-4-7":   200000,
		"claude-opus-4-6":   200000,
		"claude-opus-4-5":   200000,
		"claude-opus-4-1":   200000,
		"claude-sonnet-4-6": 200000,
		"claude-sonnet-4-5": 200000,
		"claude-sonnet-4":   200000,
		"claude-haiku-4-5":  200000,
		"claude-3-5-haiku":  200000,
		// Gemini models
		"gemini-3.1-pro":   200000,
		"gemini-3-flash":   200000,
		// OpenAI-compatible models
		"qwen3.6-plus":     256000,
		"qwen3.5-plus":     256000,
		"minimax-m2.7":     256000,
		"minimax-m2.5":     256000,
		"minimax-m2.5-free": 256000,
		"glm-5.1":          256000,
		"glm-5":            256000,
		"kimi-k2.5":        256000,
		"kimi-k2.6":        256000,
	}
	if cw, ok := contextWindows[modelID]; ok {
		return cw
	}
	return 0
}

// ZenModelCost returns the cost info for a Zen model ($ per 1M tokens).
func ZenModelCost(modelID string) types.CostInfo {
	costs := map[string]types.CostInfo{
		// Free models
		"big-pickle":            {Input: 0, Output: 0},
		"deepseek-v4-flash-free": {Input: 0, Output: 0},
		"minimax-m2.5-free":     {Input: 0, Output: 0},
		"ring-2.6-1t-free":      {Input: 0, Output: 0},
		"nemotron-3-super-free": {Input: 0, Output: 0},
		// MiniMax
		"minimax-m2.7": {Input: 0.30, Output: 1.20, CacheRead: 0.06, CacheWrite: 0.375},
		"minimax-m2.5": {Input: 0.30, Output: 1.20, CacheRead: 0.06, CacheWrite: 0.375},
		// GLM
		"glm-5.1": {Input: 1.40, Output: 4.40, CacheRead: 0.26},
		"glm-5":   {Input: 1.00, Output: 3.20, CacheRead: 0.20},
		// Kimi
		"kimi-k2.5": {Input: 0.60, Output: 3.00, CacheRead: 0.10},
		"kimi-k2.6": {Input: 0.95, Output: 4.00, CacheRead: 0.16},
		// Qwen
		"qwen3.6-plus": {Input: 0.50, Output: 3.00, CacheRead: 0.05, CacheWrite: 0.625},
		"qwen3.5-plus": {Input: 0.20, Output: 1.20, CacheRead: 0.02, CacheWrite: 0.25},
		// Claude Opus
		"claude-opus-4-7": {Input: 5.00, Output: 25.00, CacheRead: 0.50, CacheWrite: 6.25},
		"claude-opus-4-6": {Input: 5.00, Output: 25.00, CacheRead: 0.50, CacheWrite: 6.25},
		"claude-opus-4-5": {Input: 5.00, Output: 25.00, CacheRead: 0.50, CacheWrite: 6.25},
		"claude-opus-4-1": {Input: 15.00, Output: 75.00, CacheRead: 1.50, CacheWrite: 18.75},
		// Claude Sonnet (≤200K)
		"claude-sonnet-4-6": {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
		"claude-sonnet-4-5": {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
		"claude-sonnet-4":   {Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
		// Claude Haiku
		"claude-haiku-4-5": {Input: 1.00, Output: 5.00, CacheRead: 0.10, CacheWrite: 1.25},
		"claude-3-5-haiku": {Input: 1.00, Output: 5.00, CacheRead: 0.10, CacheWrite: 1.25},
		// Gemini
		"gemini-3.1-pro": {Input: 2.00, Output: 12.00, CacheRead: 0.20},
		"gemini-3-flash": {Input: 0.50, Output: 3.00, CacheRead: 0.05},
		// GPT
		"gpt-5.5":          {Input: 5.00, Output: 30.00, CacheRead: 0.50},
		"gpt-5.5-pro":      {Input: 30.00, Output: 180.00, CacheRead: 30.00},
		"gpt-5.4":          {Input: 2.50, Output: 15.00, CacheRead: 0.25},
		"gpt-5.4-pro":      {Input: 30.00, Output: 180.00, CacheRead: 30.00},
		"gpt-5.4-mini":     {Input: 0.75, Output: 4.50, CacheRead: 0.075},
		"gpt-5.4-nano":     {Input: 0.20, Output: 1.25, CacheRead: 0.02},
		"gpt-5.3-codex":    {Input: 1.75, Output: 14.00, CacheRead: 0.175},
		"gpt-5.3-codex-spark": {Input: 1.75, Output: 14.00, CacheRead: 0.175},
		"gpt-5.2":          {Input: 1.75, Output: 14.00, CacheRead: 0.175},
		"gpt-5.2-codex":    {Input: 1.75, Output: 14.00, CacheRead: 0.175},
		"gpt-5.1":          {Input: 1.07, Output: 8.50, CacheRead: 0.107},
		"gpt-5.1-codex":    {Input: 1.07, Output: 8.50, CacheRead: 0.107},
		"gpt-5.1-codex-max": {Input: 1.25, Output: 10.00, CacheRead: 0.125},
		"gpt-5.1-codex-mini": {Input: 0.25, Output: 2.00, CacheRead: 0.025},
		"gpt-5":            {Input: 1.07, Output: 8.50, CacheRead: 0.107},
		"gpt-5-codex":      {Input: 1.07, Output: 8.50, CacheRead: 0.107},
		"gpt-5-nano":       {Input: 0.05, Output: 0.40, CacheRead: 0.005},
	}
	if cost, ok := costs[modelID]; ok {
		return cost
	}
	return types.CostInfo{}
}

// ZenMaxTokens returns the default max tokens for a Zen model.
func ZenMaxTokens(modelID string) int {
	maxTokens := map[string]int{
		"gpt-5.5":          128000,
		"gpt-5.5-pro":      128000,
		"gpt-5.4":          128000,
		"gpt-5.4-pro":      128000,
		"gpt-5.4-mini":     64000,
		"gpt-5.4-nano":     32000,
		"gpt-5.3-codex":    128000,
		"gpt-5.3-codex-spark": 128000,
		"gpt-5.2":          128000,
		"gpt-5.2-codex":    128000,
		"gpt-5.1":          128000,
		"gpt-5.1-codex":    128000,
		"gpt-5.1-codex-max": 128000,
		"gpt-5.1-codex-mini": 64000,
		"gpt-5":            128000,
		"gpt-5-codex":      128000,
		"gpt-5-nano":       32000,
		"claude-opus-4-7":   64000,
		"claude-opus-4-6":   64000,
		"claude-opus-4-5":   64000,
		"claude-opus-4-1":   64000,
		"claude-sonnet-4-6": 64000,
		"claude-sonnet-4-5": 64000,
		"claude-sonnet-4":   64000,
		"claude-haiku-4-5":  64000,
		"claude-3-5-haiku":  64000,
		"gemini-3.1-pro":   64000,
		"gemini-3-flash":   64000,
		"qwen3.6-plus":     64000,
		"qwen3.5-plus":     64000,
		"minimax-m2.7":     64000,
		"minimax-m2.5":     64000,
		"minimax-m2.5-free": 64000,
		"glm-5.1":          64000,
		"glm-5":            64000,
		"kimi-k2.5":        64000,
		"kimi-k2.6":        64000,
	}
	if mt, ok := maxTokens[modelID]; ok {
		return mt
	}
	return 0
}

// DiscoverZenModels fetches models from the Zen /v1/models endpoint and registers
// them with the correct API type classification.
func DiscoverZenModels(baseURL, apiKey string, reg *Registry) int {
	// Use the same discovery mechanism as discoverOpenAICompatModels
	// but classify each model by API type
	count := discoverZenModelsInternal(baseURL, apiKey, "opencode-zen", reg)
	if count > 0 {
		// Log summary by API type
		var gptCount, claudeCount, geminiCount, compatCount int
		for _, m := range reg.Models().ListByProvider("opencode-zen") {
			switch m.API {
			case zenAPIOpenAIResponses:
				gptCount++
			case zenAPIAnthropicMessages:
				claudeCount++
			case zenAPIGoogleGenerative:
				geminiCount++
			default:
				compatCount++
			}
		}
	}
	return count
}

func discoverZenModelsInternal(baseURL, apiKey, providerName string, reg *Registry) int {
	models, err := fetchZenModels(baseURL, apiKey)
	if err != nil {
		return 0
	}

	count := 0
	for _, entry := range models {
		if entry.ID == "" {
			continue
		}

		apiType := ClassifyZenModelAPI(entry.ID)

		model := types.Model{
			ID:       entry.ID,
			Name:     ZenModelName(entry.ID),
			Provider: providerName,
			API:      apiType,
			BaseURL:  baseURL,
		}

		if entry.ContextLength != nil {
			model.ContextWindow = *entry.ContextLength
		} else if cw := ZenModelContextWindow(entry.ID); cw > 0 {
			model.ContextWindow = cw
		}
		if entry.MaxTokens != nil {
			model.MaxTokens = *entry.MaxTokens
		} else if mt := ZenMaxTokens(entry.ID); mt > 0 {
			model.MaxTokens = mt
		}

		model.Reasoning = ZenModelReasoning(entry.ID)
		model.Cost = ZenModelCost(entry.ID)

		reg.Models().Register(model)
		count++
	}

	return count
}

type zenModelEntry struct {
	ID            string `json:"id"`
	ContextLength *int   `json:"context_length,omitempty"`
	MaxTokens     *int   `json:"max_tokens,omitempty"`
}

type zenModelsResponse struct {
	Data []zenModelEntry `json:"data"`
}

func fetchZenModels(baseURL, apiKey string) ([]zenModelEntry, error) {
	modelsURL := strings.TrimRight(baseURL, "/") + "/models"

	req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create model discovery request: %w", err)
	}

	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("model discovery request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model discovery returned non-200: %d", resp.StatusCode)
	}

	var modelsResp zenModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("decode model discovery response: %w", err)
	}

	return modelsResp.Data, nil
}

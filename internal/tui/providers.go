package tui

import (
	"encoding/json"
	"io"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/types"
)

// ProviderInfo holds metadata about a connectable provider.
type ProviderInfo struct {
	// Name is the internal provider identifier (e.g., "opencode-zen").
	Name string
	// DisplayName is the human-readable name shown in the UI.
	DisplayName string
	// Description is shown below the name in the provider list.
	Description string
	// RequiresAPIKey is true if the provider needs an API key.
	RequiresAPIKey bool
	// BaseURL is the API base URL (for OpenAI-compatible providers).
	BaseURL string
	// APIPath is the API path for chat completions.
	APIPath string
	// TestConnection tests connectivity with the given API key.
	TestConnection func(apiKey string) error
	// DiscoverModels fetches model IDs from the provider.
	// Returns a list of model IDs and any error.
	DiscoverModels func(apiKey string) ([]string, error)
}

// providerCatalog lists all connectable providers.
var providerCatalog = []ProviderInfo{
	{
		Name:           "ollama",
		DisplayName:    "Ollama",
		Description:    "Local models (no API key required)",
		RequiresAPIKey: false,
		BaseURL:        "http://localhost:11434",
		TestConnection: testOllama,
		DiscoverModels: discoverOllamaModels,
	},
	{
		Name:           "opencode-zen",
		DisplayName:    "OpenCode Zen",
		Description:    "OpenCode Zen cloud provider",
		RequiresAPIKey: true,
		BaseURL:        "https://opencode.ai/zen/v1",
		TestConnection: testOpenAICompatZen,
		DiscoverModels: discoverOpenAICompatModelsZen,
	},
	{
		Name:           "opencode-go",
		DisplayName:    "OpenCode Go",
		Description:    "OpenCode Go cloud provider",
		RequiresAPIKey: true,
		BaseURL:        "https://opencode.ai/zen/go/v1",
		TestConnection: testOpenAICompatGo,
		DiscoverModels: discoverOpenAICompatModelsGo,
	},
	{
		Name:           "openai",
		DisplayName:    "OpenAI",
		Description:    "OpenAI cloud provider",
		RequiresAPIKey: true,
		BaseURL:        "https://api.openai.com/v1",
		TestConnection: testOpenAICompatOpenAI,
		DiscoverModels: discoverOpenAICompatModelsOpenAI,
	},
	{
		Name:           "openai-oauth",
		DisplayName:    "ChatGPT Plus/Pro (OAuth)",
		Description:    "Use your ChatGPT subscription (no API key required)",
		RequiresAPIKey: false,
		BaseURL:        "https://chatgpt.com/backend-api",
		TestConnection: testOpenAIOAuth,
		DiscoverModels: discoverOpenAIOAuthModels,
	},
	{
		Name:           "anthropic",
		DisplayName:    "Anthropic",
		Description:    "Anthropic Claude models",
		RequiresAPIKey: true,
		TestConnection: testAnthropic,
		DiscoverModels: discoverAnthropicModels,
	},
	{
		Name:           "google",
		DisplayName:    "Google",
		Description:    "Google Gemini models",
		RequiresAPIKey: true,
		TestConnection: testGoogle,
		DiscoverModels: discoverGoogleModels,
	},
	{
		Name:           "openrouter",
		DisplayName:    "OpenRouter",
		Description:    "300+ AI models via unified API",
		RequiresAPIKey: true,
		BaseURL:        "https://openrouter.ai/api/v1",
		TestConnection: testOpenRouter,
		DiscoverModels: discoverOpenRouterModels,
	},
}

// findProvider looks up a provider by name.
func findProvider(name string) (ProviderInfo, bool) {
	for _, p := range providerCatalog {
		if p.Name == name {
			return p, true
		}
	}
	return ProviderInfo{}, false
}

// listAvailableProviders returns the provider catalog entries.
func listAvailableProviders() []ProviderInfo {
	return providerCatalog
}

// providerWithState extends ProviderInfo with configuration state.
type providerWithState struct {
	ProviderInfo
	Enabled   bool
	HasAuth   bool
	HasConfig bool
}

// providerState holds the current state of a provider for connection flows.
type providerState struct {
	Enabled     bool
	HasAuth     bool
	HasConfig   bool
	APIKey      string
	IsConnected bool
}

// getProviderState returns the current state of a provider.
func getProviderState(providerName string) providerState {
	cfg, err := config.LoadConfig("")
	enabled := true
	hasConfig := false
	if err == nil {
		if providerCfg, exists := cfg.Providers[providerName]; exists {
			hasConfig = true
			if providerCfg.Enabled != nil {
				enabled = *providerCfg.Enabled
			}
		}
	}

	authPath := config.AuthPath("")
	store, err := config.LoadAuth(authPath)
	hasAuth := false
	apiKey := ""
	if err == nil {
		if authVal, exists := store[providerName]; exists {
			hasAuth = true
			apiKey = authVal.APIKey()
		}
	}

	return providerState{
		Enabled:     enabled,
		HasAuth:     hasAuth,
		HasConfig:   hasConfig,
		APIKey:      apiKey,
		IsConnected: enabled && hasAuth,
	}
}

// listAvailableProvidersWithState returns all providers with their current connection state.
func listAvailableProvidersWithState(m *Model) []providerWithState {
	cfg, err := config.LoadConfig("")
	if err != nil {
		slog.Debug("failed to load config for provider list", "error", err)
	}

	authPath := config.AuthPath("")
	store, err := config.LoadAuth(authPath)
	if err != nil {
		slog.Debug("failed to load auth for provider list", "error", err)
	}

	var result []providerWithState
	for _, info := range providerCatalog {
		_, hasAuth := store[info.Name]
		providerCfg, hasConfig := cfg.Providers[info.Name]

		enabled := true
		if hasConfig && providerCfg.Enabled != nil {
			enabled = *providerCfg.Enabled
		}

		result = append(result, providerWithState{
			ProviderInfo: info,
			Enabled:      enabled,
			HasAuth:      hasAuth,
			HasConfig:    hasConfig,
		})
	}

	return result
}

// --- Connection test functions ---

func testOllama(apiKey string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return fmt.Errorf("cannot reach Ollama at localhost:11434: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}
	return nil
}

func testOpenAICompat(apiKey string) error {
	return fmt.Errorf("testOpenAICompat called without BaseURL — use provider-specific test function")
}

func testOpenAICompatZen(apiKey string) error {
	return testOpenAICompatWithURL("https://opencode.ai/zen/v1", apiKey)
}

func testOpenAICompatGo(apiKey string) error {
	return testOpenAICompatWithURL("https://opencode.ai/zen/go/v1", apiKey)
}

func testOpenAICompatOpenAI(apiKey string) error {
	return testOpenAICompatWithURL("https://api.openai.com/v1", apiKey)
}

func testOpenAICompatWithURL(baseURL, apiKey string) error {
	modelsURL := strings.TrimRight(baseURL, "/") + "/models"

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

func testAnthropic(apiKey string) error {
	// Anthropic has no model listing API. Test by hitting /v1/messages
	// with a minimal request (will fail on model validation but proves auth works).
	reqBody := `{"model":"claude-sonnet-4-20250514","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`

	req, err := http.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	// 400 is acceptable — it means auth worked but the request was invalid.
	// 401/403 means auth failed.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("authentication failed (status %d)", resp.StatusCode)
	}
	return nil
}

func testGoogle(apiKey string) error {
	// Google has no model listing API. Test by hitting the models list endpoint
	// which only requires the API key in the URL.
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("authentication failed (status %d)", resp.StatusCode)
	}
	return nil
}

// --- Model discovery functions ---

func discoverOllamaModels(apiKey string) ([]string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		return nil, fmt.Errorf("cannot reach Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var models []string
	for _, m := range result.Models {
		if m.Name != "" {
			models = append(models, m.Name)
		}
	}
	return models, nil
}

func discoverOpenAICompatModelsWithURL(baseURL, apiKey string) ([]string, error) {
	modelsURL := strings.TrimRight(baseURL, "/") + "/models"

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
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var models []string
	for _, m := range result.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

// discoverOpenAICompatModels wrappers for specific providers.
func discoverOpenAICompatModels(apiKey string) ([]string, error) {
	// This is a generic wrapper — actual URL is set per-provider in the catalog.
	// Use discoverOpenAICompatModelsWithURL directly in discoverProviderModels.
	return nil, fmt.Errorf("use discoverOpenAICompatModelsWithURL instead")
}

func discoverOpenAICompatModelsZen(apiKey string) ([]string, error) {
	return discoverOpenAICompatModelsWithURL("https://opencode.ai/zen/v1", apiKey)
}

func discoverOpenAICompatModelsGo(apiKey string) ([]string, error) {
	return discoverOpenAICompatModelsWithURL("https://opencode.ai/zen/go/v1", apiKey)
}

func discoverOpenAICompatModelsOpenAI(apiKey string) ([]string, error) {
	return discoverOpenAICompatModelsWithURL("https://api.openai.com/v1", apiKey)
}

// Hardcoded model lists for providers without listing APIs.
var anthropicModels = []string{
	"claude-sonnet-4-20250514",
	"claude-3-7-sonnet-20250219",
	"claude-3-5-sonnet-20241022",
}

var googleModels = []string{
	"gemini-2.5-pro",
	"gemini-2.5-flash",
	"gemini-2.0-flash",
}

func discoverAnthropicModels(apiKey string) ([]string, error) {
	return anthropicModels, nil
}

func discoverGoogleModels(apiKey string) ([]string, error) {
	return googleModels, nil
}

func testOpenRouter(apiKey string) error {
	modelsURL := "https://openrouter.ai/api/v1/models"

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

func discoverOpenRouterModels(apiKey string) ([]string, error) {
	modelsURL := "https://openrouter.ai/api/v1/models"

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
		apiErr := types.ClassifyAPIError(resp.StatusCode, bodyBytes)
		return nil, fmt.Errorf("%s", apiErr.UserMessage())
	}

	var result struct {
		Data []struct {
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Filter to popular providers, group by provider
	type modelEntry struct {
		ID            string
		Provider      string
		ContextLength int
	}
	byProvider := make(map[string][]modelEntry)
	popularPrefixes := []string{
		"openai/", "anthropic/", "google/", "mistralai/", "deepseek/",
		"meta-llama/", "qwen/", "x-ai/", "minimax/", "amazon/", "cohere/",
	}
	for _, m := range result.Data {
		for _, prefix := range popularPrefixes {
			if len(m.ID) > len(prefix) && m.ID[:len(prefix)] == prefix {
				provider := prefix[:len(prefix)-1]
				byProvider[provider] = append(byProvider[provider], modelEntry{
					ID: m.ID, Provider: provider, ContextLength: m.ContextLength,
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
	var ordered []modelEntry
	maxPerProvider := 0
	for _, models := range byProvider {
		if len(models) > maxPerProvider {
			maxPerProvider = len(models)
		}
	}
	for i := 0; i < maxPerProvider; i++ {
		for _, prefix := range popularPrefixes {
			name := prefix[:len(prefix)-1]
			models := byProvider[name]
			if i < len(models) {
				ordered = append(ordered, models[i])
			}
		}
	}

	// Take top 30
	limit := 30
	if len(ordered) < limit {
		limit = len(ordered)
	}

	models := make([]string, limit)
	for i := 0; i < limit; i++ {
		models[i] = ordered[i].ID
	}

	return models, nil
}

// testOpenAIOAuth tests connectivity for OAuth provider.
// Since OAuth requires valid tokens, we just verify the endpoint is reachable.
func testOpenAIOAuth(apiKey string) error {
	// OAuth connectivity is verified during the OAuth flow itself.
	// This function is a no-op — actual connection test happens when tokens are obtained.
	if apiKey != "" {
		// If somehow an API key was passed, this isn't an OAuth provider
		return fmt.Errorf("OAuth provider does not use API keys")
	}
	return nil
}

// discoverOpenAIOAuthModels returns the hardcoded Codex model list.
// The Codex endpoint does not have a model listing API.
func discoverOpenAIOAuthModels(apiKey string) ([]string, error) {
	return []string{
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.3-codex",
		"gpt-5.3-codex-spark",
		"gpt-5.2",
	}, nil
}

// testProviderConnection tests connectivity for a specific provider.
func testProviderConnection(info ProviderInfo, apiKey string) error {
	if info.TestConnection == nil {
		return fmt.Errorf("no test function for provider: %s", info.Name)
	}
	return info.TestConnection(apiKey)
}

// discoverProviderModels fetches models for a specific provider.
func discoverProviderModels(info ProviderInfo, apiKey string) ([]string, error) {
	if info.DiscoverModels == nil {
		return nil, fmt.Errorf("no model discovery function for provider: %s", info.Name)
	}
	return info.DiscoverModels(apiKey)
}

// validateAPIKey checks if the API key looks valid (non-empty, reasonable length).
func validateAPIKey(key string) error {
	if key == "" {
		return fmt.Errorf("API key cannot be empty")
	}
	if len(key) < 4 {
		return fmt.Errorf("API key looks too short")
	}
	return nil
}

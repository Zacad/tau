package tui

import (
	"testing"
)

func TestFindProvider_KnownProviders(t *testing.T) {
	tests := []struct {
		name     string
		wantName string
		wantKey  bool
	}{
		{"ollama", "ollama", false},
		{"opencode-zen", "opencode-zen", true},
		{"opencode-go", "opencode-go", true},
		{"openai", "openai", true},
		{"anthropic", "anthropic", true},
		{"google", "google", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := findProvider(tt.name)
			if !ok {
				t.Fatalf("findProvider(%q) not found", tt.name)
			}
			if info.Name != tt.wantName {
				t.Errorf("Name: got %q, want %q", info.Name, tt.wantName)
			}
			if info.RequiresAPIKey != tt.wantKey {
				t.Errorf("RequiresAPIKey: got %v, want %v", info.RequiresAPIKey, tt.wantKey)
			}
			if info.DisplayName == "" {
				t.Error("DisplayName should not be empty")
			}
		})
	}
}

func TestFindProvider_Unknown(t *testing.T) {
	_, ok := findProvider("nonexistent-provider")
	if ok {
		t.Fatal("findProvider should return false for unknown provider")
	}
}

func TestListAvailableProviders(t *testing.T) {
	providers := listAvailableProviders()
	if len(providers) < 3 {
		t.Fatalf("expected at least 3 providers, got %d", len(providers))
	}

	// Verify all expected providers are present
	names := make(map[string]bool)
	for _, p := range providers {
		names[p.Name] = true
	}

	expected := []string{"ollama", "opencode-zen", "opencode-go", "openai", "anthropic", "google"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("expected provider %q in list", name)
		}
	}
}

func TestProviderCatalog_BaseURLs(t *testing.T) {
	tests := []struct {
		name    string
		wantURL string
	}{
		{"opencode-zen", "https://opencode.ai/zen/v1"},
		{"opencode-go", "https://opencode.ai/zen/go/v1"},
		{"openai", "https://api.openai.com/v1"},
		{"ollama", "http://localhost:11434"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := findProvider(tt.name)
			if !ok {
				t.Fatalf("provider %q not found", tt.name)
			}
			if info.BaseURL != tt.wantURL {
				t.Errorf("BaseURL: got %q, want %q", info.BaseURL, tt.wantURL)
			}
		})
	}
}

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		key     string
		wantErr bool
	}{
		{"sk-valid-key-12345", false},
		{"", true},
		{"abc", true},
		{"short", false}, // 5 chars, passes
	}

	for _, tt := range tests {
		err := validateAPIKey(tt.key)
		if (err != nil) != tt.wantErr {
			t.Errorf("validateAPIKey(%q): got err=%v, wantErr=%v", tt.key, err, tt.wantErr)
		}
	}
}

func TestDiscoverAnthropicModels(t *testing.T) {
	models, err := discoverAnthropicModels("test-key")
	if err != nil {
		t.Fatalf("discoverAnthropicModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected models from hardcoded list")
	}
	// Verify known models are present
	found := false
	for _, m := range models {
		if m == "claude-sonnet-4-20250514" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected claude-sonnet-4-20250514 in model list")
	}
}

func TestDiscoverGoogleModels(t *testing.T) {
	models, err := discoverGoogleModels("test-key")
	if err != nil {
		t.Fatalf("discoverGoogleModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected models from hardcoded list")
	}
	// Verify known models are present
	found := false
	for _, m := range models {
		if m == "gemini-2.5-pro" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected gemini-2.5-pro in model list")
	}
}

func TestTestProviderConnection_Ollama(t *testing.T) {
	info, _ := findProvider("ollama")
	// This will fail if Ollama is not running, which is expected in unit tests
	err := testProviderConnection(info, "")
	// We just verify it doesn't panic — actual success depends on Ollama being available
	_ = err
}

func TestDiscoverProviderModels_Anthropic(t *testing.T) {
	info, _ := findProvider("anthropic")
	models, err := discoverProviderModels(info, "test-key")
	if err != nil {
		t.Fatalf("discoverProviderModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected models for anthropic")
	}
}

func TestDiscoverProviderModels_Google(t *testing.T) {
	info, _ := findProvider("google")
	models, err := discoverProviderModels(info, "test-key")
	if err != nil {
		t.Fatalf("discoverProviderModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected models for google")
	}
}

func TestDiscoverProviderModels_Unknown(t *testing.T) {
	info := ProviderInfo{Name: "unknown-provider"}
	_, err := discoverProviderModels(info, "key")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestTestProviderConnection_Unknown(t *testing.T) {
	info := ProviderInfo{Name: "unknown-provider"}
	err := testProviderConnection(info, "key")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestFindProvider_OpenRouter(t *testing.T) {
	info, ok := findProvider("openrouter")
	if !ok {
		t.Fatal("findProvider('openrouter') not found")
	}
	if info.Name != "openrouter" {
		t.Errorf("Name: got %q, want %q", info.Name, "openrouter")
	}
	if info.DisplayName != "OpenRouter" {
		t.Errorf("DisplayName: got %q, want %q", info.DisplayName, "OpenRouter")
	}
	if info.Description != "300+ AI models via unified API" {
		t.Errorf("Description: got %q, want %q", info.Description, "300+ AI models via unified API")
	}
	if !info.RequiresAPIKey {
		t.Error("RequiresAPIKey: got false, want true")
	}
	if info.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("BaseURL: got %q, want %q", info.BaseURL, "https://openrouter.ai/api/v1")
	}
	if info.TestConnection == nil {
		t.Error("TestConnection should not be nil")
	}
	if info.DiscoverModels == nil {
		t.Error("DiscoverModels should not be nil")
	}
}

func TestListAvailableProviders_IncludesOpenRouter(t *testing.T) {
	providers := listAvailableProviders()

	found := false
	for _, p := range providers {
		if p.Name == "openrouter" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected openrouter provider in list")
	}
}

func TestTestOpenRouterConnection_InvalidKey(t *testing.T) {
	// OpenRouter allows model listing without auth, so this should succeed
	// even with an invalid key
	err := testOpenRouter("invalid-key")
	if err != nil {
		t.Fatalf("expected no error (OpenRouter models endpoint is public), got: %v", err)
	}
}

func TestDiscoverOpenRouterModels_ReturnsPopularModels(t *testing.T) {
	models, err := discoverOpenRouterModels("")
	if err != nil {
		t.Fatalf("discoverOpenRouterModels: %v", err)
	}
	if len(models) < 10 {
		t.Fatalf("expected at least 10 models, got %d", len(models))
	}
	if len(models) > 30 {
		t.Fatalf("expected at most 30 models, got %d", len(models))
	}
	// Check format (should be author/model-name)
	for _, m := range models[:10] {
		if m == "" {
			t.Error("expected non-empty model ID")
		}
	}
	// Verify models from known popular providers are present
	providers := make(map[string]bool)
	for _, m := range models {
		if idx := len(m) - len("/"); idx > 0 {
			for j := idx - 1; j >= 0; j-- {
				if m[j] == '/' {
					providers[m[:j]] = true
					break
				}
			}
		}
	}
	if !providers["openai"] {
		t.Error("expected openai models in list")
	}
	if !providers["anthropic"] {
		t.Error("expected anthropic models in list")
	}
	if !providers["google"] {
		t.Error("expected google models in list")
	}
}

func TestFindProvider_OpenAIOAuth(t *testing.T) {
	info, ok := findProvider("openai-oauth")
	if !ok {
		t.Fatal("findProvider('openai-oauth') not found")
	}
	if info.Name != "openai-oauth" {
		t.Errorf("Name: got %q, want %q", info.Name, "openai-oauth")
	}
	if info.DisplayName != "ChatGPT Plus/Pro (OAuth)" {
		t.Errorf("DisplayName: got %q, want %q", info.DisplayName, "ChatGPT Plus/Pro (OAuth)")
	}
	if info.RequiresAPIKey {
		t.Error("RequiresAPIKey: got true, want false")
	}
	if info.BaseURL != "https://chatgpt.com/backend-api" {
		t.Errorf("BaseURL: got %q, want %q", info.BaseURL, "https://chatgpt.com/backend-api")
	}
	if info.TestConnection == nil {
		t.Error("TestConnection should not be nil")
	}
	if info.DiscoverModels == nil {
		t.Error("DiscoverModels should not be nil")
	}
}

func TestListAvailableProviders_IncludesOpenAIOAuth(t *testing.T) {
	providers := listAvailableProviders()

	found := false
	for _, p := range providers {
		if p.Name == "openai-oauth" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected openai-oauth provider in list")
	}
}

func TestTestOpenAIOAuth_NoAPIKey(t *testing.T) {
	err := testOpenAIOAuth("")
	if err != nil {
		t.Fatalf("expected no error for OAuth provider without API key, got: %v", err)
	}
}

func TestTestOpenAIOAuth_WithAPIKey(t *testing.T) {
	err := testOpenAIOAuth("some-key")
	if err == nil {
		t.Fatal("expected error when API key is passed to OAuth provider")
	}
}

func TestDiscoverOpenAIOAuthModels(t *testing.T) {
	models, err := discoverOpenAIOAuthModels("")
	if err != nil {
		t.Fatalf("discoverOpenAIOAuthModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected Codex models")
	}

	expectedModels := []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2"}
	for _, expected := range expectedModels {
		found := false
		for _, m := range models {
			if m == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected model %q in OAuth model list", expected)
		}
	}
}

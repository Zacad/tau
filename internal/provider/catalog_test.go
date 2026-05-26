package provider

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

func TestTransformCatalog_BasicMapping(t *testing.T) {
	npmOpenAI := "@ai-sdk/openai"
	npmAnthropic := "@ai-sdk/anthropic"
	statusDeprecated := "deprecated"

	catalog := rawCatalog{
		"openai": rawProvider{
			ID:   "openai",
			Name: "OpenAI",
			NPM:  &npmOpenAI,
			API:  ptr("https://api.openai.com/v1"),
			Models: map[string]rawModel{
				"gpt-4o": {
					ID:          "gpt-4o",
					Name:        "GPT-4o",
					Reasoning:   false,
					ToolCall:    true,
					Temperature: true,
					Limit:       rawLimit{Context: 128000, Output: 16384},
					Cost:        &rawCost{Input: 2.50, Output: 10.00},
					Modalities:  &rawModalities{Input: []string{"text", "image"}},
				},
				"o3": {
					ID:          "o3",
					Name:        "o3",
					Reasoning:   true,
					ToolCall:    true,
					Temperature: false,
					Limit:       rawLimit{Context: 200000, Output: 100000},
					Cost:        &rawCost{Input: 10.00, Output: 40.00},
				},
			},
		},
		"anthropic": rawProvider{
			ID:   "anthropic",
			Name: "Anthropic",
			NPM:  &npmAnthropic,
			API:  ptr("https://api.anthropic.com/v1"),
			Models: map[string]rawModel{
				"claude-sonnet-4-20250514": {
					ID:          "claude-sonnet-4-20250514",
					Name:        "Claude Sonnet 4",
					Reasoning:   true,
					ToolCall:    true,
					Temperature: true,
					Limit:       rawLimit{Context: 200000, Output: 8192},
					Cost:        &rawCost{Input: 3.00, Output: 15.00, CacheRead: ptrFloat(0.30), CacheWrite: ptrFloat(3.75)},
					Modalities:  &rawModalities{Input: []string{"text", "image"}},
				},
				"old-model": {
					ID:          "old-model",
					Name:        "Old Model",
					Reasoning:   false,
					ToolCall:    true,
					Temperature: true,
					Limit:       rawLimit{Context: 100000, Output: 4096},
					Status:      &statusDeprecated,
				},
			},
		},
	}

	registry := NewModelRegistry()
	count := TransformCatalog(catalog, registry)

	// Should have registered 3 models (2 openai + 1 anthropic, deprecated skipped)
	if count != 3 {
		t.Fatalf("expected 3 models, got %d", count)
	}

	// Verify OpenAI models
	gpt4o, err := registry.Get("gpt-4o")
	if err != nil {
		t.Fatalf("gpt-4o not found: %v", err)
	}
	if gpt4o.API != "openai-responses" {
		t.Errorf("expected openai-responses API, got %s", gpt4o.API)
	}
	if gpt4o.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected base URL https://api.openai.com/v1, got %s", gpt4o.BaseURL)
	}
	if gpt4o.Reasoning {
		t.Error("gpt-4o should not have reasoning")
	}
	if gpt4o.ContextWindow != 128000 {
		t.Errorf("expected context window 128000, got %d", gpt4o.ContextWindow)
	}
	if gpt4o.Cost.Input != 2.50 {
		t.Errorf("expected cost input 2.50, got %f", gpt4o.Cost.Input)
	}

	// Verify reasoning model has thinking level map
	o3, err := registry.Get("o3")
	if err != nil {
		t.Fatalf("o3 not found: %v", err)
	}
	if !o3.Reasoning {
		t.Error("o3 should have reasoning")
	}
	if o3.ThinkingLevelMap == nil {
		t.Error("o3 should have thinking level map")
	}
	if o3.ThinkingLevelMap["off"] != "none" {
		t.Errorf("expected off->none, got %s", o3.ThinkingLevelMap["off"])
	}

	// Verify Anthropic model
	sonnet, err := registry.Get("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("claude-sonnet-4 not found: %v", err)
	}
	if sonnet.API != "anthropic-messages" {
		t.Errorf("expected anthropic-messages API, got %s", sonnet.API)
	}
	if sonnet.Cost.CacheRead != 0.30 {
		t.Errorf("expected cache read cost 0.30, got %f", sonnet.Cost.CacheRead)
	}

	// Verify deprecated model was skipped
	_, err = registry.Get("old-model")
	if err == nil {
		t.Error("deprecated model should not be registered")
	}
}

func TestTransformCatalog_SkipsNoToolCall(t *testing.T) {
	catalog := rawCatalog{
		"openai": rawProvider{
			ID:  "openai",
			NPM: ptr("@ai-sdk/openai"),
			API: ptr("https://api.openai.com/v1"),
			Models: map[string]rawModel{
				"dall-e-3": {
					ID:          "dall-e-3",
					Name:        "DALL-E 3",
					Reasoning:   false,
					ToolCall:    false,
					Temperature: false,
					Limit:       rawLimit{Context: 4096, Output: 4096},
				},
			},
		},
	}

	registry := NewModelRegistry()
	count := TransformCatalog(catalog, registry)

	if count != 0 {
		t.Fatalf("expected 0 models (no tool_call), got %d", count)
	}
}

func TestTransformCatalog_RemovesExistingProviderModels(t *testing.T) {
	registry := NewModelRegistry()

	// Verify minimal built-in models exist
	initialCount := len(registry.ListByProvider("openai"))
	if initialCount < 1 {
		t.Fatal("expected at least 1 built-in OpenAI model")
	}

	// Transform with empty catalog for openai
	catalog := rawCatalog{
		"openai": rawProvider{
			ID:     "openai",
			NPM:    ptr("@ai-sdk/openai"),
			API:    ptr("https://api.openai.com/v1"),
			Models: map[string]rawModel{},
		},
	}

	TransformCatalog(catalog, registry)

	// OpenAI models should be cleared
	openAI := registry.ListByProvider("openai")
	if len(openAI) != 0 {
		t.Fatalf("expected 0 OpenAI models after transform with empty catalog, got %d", len(openAI))
	}
}

func TestTransformCatalog_ModelProviderOverride(t *testing.T) {
	catalog := rawCatalog{
		"opencode": rawProvider{
			ID:   "opencode",
			Name: "OpenCode",
			NPM:  ptr("@ai-sdk/openai"),
			API:  ptr("https://opencode.ai/zen/v1"),
			Models: map[string]rawModel{
				"custom-model-v2": {
					ID:          "custom-model-v2",
					Name:        "Custom Model V2",
					Reasoning:   false,
					ToolCall:    true,
					Temperature: true,
					Limit:       rawLimit{Context: 128000, Output: 16384},
					Provider: &rawModelProvider{
						NPM: ptr("@ai-sdk/anthropic"),
						API: ptr("https://api.anthropic.com/v1"),
					},
				},
			},
		},
	}

	registry := NewModelRegistry()
	TransformCatalog(catalog, registry)

	m, err := registry.Get("custom-model-v2")
	if err != nil {
		t.Fatalf("custom-model-v2 not found: %v", err)
	}

	// Model-level provider override should take precedence
	if m.API != "anthropic-messages" {
		t.Errorf("expected anthropic-messages API (from model override), got %s", m.API)
	}
	if m.BaseURL != "https://api.anthropic.com/v1" {
		t.Errorf("expected anthropic base URL (from model override), got %s", m.BaseURL)
	}
}

func TestThinkingLevelMapFor_OpenAI(t *testing.T) {
	tests := []struct {
		modelID       string
		expectOff     string
		expectXHigh   string
		expectMinimal string
	}{
		{"o1", "none", "xhigh", "minimal"},
		{"o3", "none", "xhigh", "minimal"},
		{"o4-mini", "none", "xhigh", "minimal"},
		{"gpt-5", "none", "", ""},
		{"gpt-5.5", "none", "xhigh", "minimal"},
		{"gpt-5.4-pro", "none", "xhigh", "minimal"},
		{"gpt-4o", "", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.modelID, func(t *testing.T) {
			m := thinkingLevelMapFor("openai", tc.modelID)

			if tc.expectOff != "" {
				if m == nil || m["off"] != tc.expectOff {
					t.Errorf("expected off=%s, got %v", tc.expectOff, m)
				}
			}
			if tc.expectXHigh != "" {
				if m == nil || m["xhigh"] != tc.expectXHigh {
					t.Errorf("expected xhigh=%s, got %v", tc.expectXHigh, m)
				}
			}
			if tc.expectMinimal != "" {
				if m == nil || m["minimal"] != tc.expectMinimal {
					t.Errorf("expected minimal=%s, got %v", tc.expectMinimal, m)
				}
			}
			if tc.expectOff == "" && tc.expectXHigh == "" {
				if m != nil {
					t.Errorf("expected nil thinking level map, got %v", m)
				}
			}
		})
	}
}

func TestThinkingLevelMapFor_Anthropic(t *testing.T) {
	tests := []struct {
		modelID     string
		expectHigh  string
		expectXHigh string
	}{
		{"claude-sonnet-4-20250514", "8192", "16384"},
		{"claude-sonnet-4.5", "8192", "16384"},
		{"claude-opus-4-5", "8192", "16384"},
		{"claude-opus-4.6", "high", "max"},
		{"claude-opus-4.7", "high", "max"},
		{"claude-3-5-sonnet-20241022", "", ""},
	}

	for _, tc := range tests {
		t.Run(tc.modelID, func(t *testing.T) {
			m := thinkingLevelMapFor("anthropic", tc.modelID)

			if tc.expectHigh != "" {
				if m == nil || m["high"] != tc.expectHigh {
					t.Errorf("expected high=%s, got %v", tc.expectHigh, m)
				}
			}
			if tc.expectXHigh != "" {
				if m == nil || m["xhigh"] != tc.expectXHigh {
					t.Errorf("expected xhigh=%s, got %v", tc.expectXHigh, m)
				}
			}
			if tc.expectHigh == "" && tc.expectXHigh == "" {
				if m != nil {
					t.Errorf("expected nil thinking level map, got %v", m)
				}
			}
		})
	}
}

func TestThinkingLevelMapFor_Google(t *testing.T) {
	m := thinkingLevelMapFor("google", "gemini-2.5-pro")
	if m == nil {
		t.Fatal("expected thinking level map for gemini-2.5-pro")
	}
	if m["medium"] != "8192" {
		t.Errorf("expected medium=8192, got %s", m["medium"])
	}

	m = thinkingLevelMapFor("google", "gemma-3-27b")
	if m == nil {
		t.Fatal("expected thinking level map for gemma")
	}
	if m["high"] != "HIGH" {
		t.Errorf("expected high=HIGH, got %s", m["high"])
	}
}

func TestResolveAPIType(t *testing.T) {
	tests := []struct {
		npm      string
		expected string
	}{
		{"@ai-sdk/openai", "openai-responses"},
		{"@ai-sdk/anthropic", "anthropic-messages"},
		{"@ai-sdk/google", "google-generative-ai"},
		{"@ai-sdk/openai-compatible", "openai-completions"},
		{"@ai-sdk/alibaba", "openai-completions"},
		{"unknown-package", "openai-completions"},
	}

	for _, tc := range tests {
		t.Run(tc.npm, func(t *testing.T) {
			prov := rawProvider{NPM: &tc.npm}
			result := resolveAPIType(prov)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}

	// Nil NPM
	result := resolveAPIType(rawProvider{NPM: nil})
	if result != "openai-completions" {
		t.Errorf("expected openai-completions for nil NPM, got %s", result)
	}
}

func TestResolveBaseURL(t *testing.T) {
	tests := []struct {
		name     string
		prov     rawProvider
		expected string
	}{
		{
			name:     "explicit API field",
			prov:     rawProvider{API: ptr("https://custom.api.com")},
			expected: "https://custom.api.com",
		},
		{
			name:     "openai NPM",
			prov:     rawProvider{NPM: ptr("@ai-sdk/openai")},
			expected: "https://api.openai.com/v1",
		},
		{
			name:     "anthropic NPM",
			prov:     rawProvider{NPM: ptr("@ai-sdk/anthropic")},
			expected: "https://api.anthropic.com/v1",
		},
		{
			name:     "google NPM",
			prov:     rawProvider{NPM: ptr("@ai-sdk/google")},
			expected: "https://generativelanguage.googleapis.com/v1beta/models",
		},
		{
			name:     "unknown NPM",
			prov:     rawProvider{NPM: ptr("@ai-sdk/unknown")},
			expected: "",
		},
		{
			name:     "nil NPM",
			prov:     rawProvider{NPM: nil},
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := resolveBaseURL(tc.prov)
			if result != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestCachePath(t *testing.T) {
	path := CachePath()
	if path == "" {
		t.Fatal("expected non-empty cache path")
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".tau", "cache", "models.json")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}
}

func TestLoadFromCache_Fresh(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "models.json")

	catalog := rawCatalog{
		"test": rawProvider{
			ID:     "test",
			Models: map[string]rawModel{},
		},
	}
	data, _ := json.Marshal(catalog)
	os.WriteFile(cacheFile, data, 0644)

	result := loadFromCache(cacheFile)
	if result == nil {
		t.Fatal("expected cached catalog, got nil")
	}
	if _, ok := result["test"]; !ok {
		t.Error("expected test provider in cached catalog")
	}
}

func TestLoadFromCache_Stale(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "models.json")

	catalog := rawCatalog{
		"test": rawProvider{
			ID:     "test",
			Models: map[string]rawModel{},
		},
	}
	data, _ := json.Marshal(catalog)
	os.WriteFile(cacheFile, data, 0644)

	// Make file old
	oldTime := time.Now().Add(-10 * time.Minute)
	os.Chtimes(cacheFile, oldTime, oldTime)

	result := loadFromCache(cacheFile)
	if result != nil {
		t.Error("expected nil for stale cache, got catalog")
	}
}

func TestLoadFromCache_Missing(t *testing.T) {
	result := loadFromCache("/nonexistent/path/models.json")
	if result != nil {
		t.Error("expected nil for missing cache file")
	}
}

func TestLoadFromCache_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "models.json")

	os.WriteFile(cacheFile, []byte("not valid json"), 0644)

	result := loadFromCache(cacheFile)
	if result != nil {
		t.Error("expected nil for invalid JSON cache")
	}
}

func TestWriteCache(t *testing.T) {
	dir := t.TempDir()
	cacheFile := filepath.Join(dir, "subdir", "models.json")

	catalog := rawCatalog{
		"openai": rawProvider{
			ID:   "openai",
			Name: "OpenAI",
			Models: map[string]rawModel{
				"gpt-4o": {ID: "gpt-4o", Name: "GPT-4o", ToolCall: true, Limit: rawLimit{Context: 128000, Output: 16384}},
			},
		},
	}

	err := writeCache(cacheFile, catalog)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file exists and is valid JSON
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		t.Fatalf("failed to read cache file: %v", err)
	}

	var readBack rawCatalog
	if err := json.Unmarshal(data, &readBack); err != nil {
		t.Fatalf("cache file contains invalid JSON: %v", err)
	}

	if _, ok := readBack["openai"]; !ok {
		t.Error("expected openai provider in written cache")
	}
}

func TestTransformCatalog_InputTypes(t *testing.T) {
	catalog := rawCatalog{
		"openai": rawProvider{
			ID:  "openai",
			NPM: ptr("@ai-sdk/openai"),
			API: ptr("https://api.openai.com/v1"),
			Models: map[string]rawModel{
				"with-modalities": {
					ID:       "with-modalities",
					Name:     "With Modalities",
					ToolCall: true,
					Limit:    rawLimit{Context: 100000, Output: 8192},
					Modalities: &rawModalities{
						Input:  []string{"text", "image", "audio"},
						Output: []string{"text"},
					},
				},
				"no-modalities": {
					ID:       "no-modalities",
					Name:     "No Modalities",
					ToolCall: true,
					Limit:    rawLimit{Context: 100000, Output: 8192},
				},
			},
		},
	}

	registry := NewModelRegistry()
	TransformCatalog(catalog, registry)

	withMod, _ := registry.Get("with-modalities")
	if len(withMod.InputTypes) != 3 {
		t.Errorf("expected 3 input types, got %d", len(withMod.InputTypes))
	}

	noMod, _ := registry.Get("no-modalities")
	if len(noMod.InputTypes) != 1 || noMod.InputTypes[0] != "text" {
		t.Errorf("expected [text] input types, got %v", noMod.InputTypes)
	}
}

// Helper functions

func ptr(s string) *string {
	return &s
}

func ptrFloat(f float64) *float64 {
	return &f
}

func TestFetchCatalog_Disabled(t *testing.T) {
	// Set env var to disable fetch
	os.Setenv("TAU_DISABLE_MODELS_FETCH", "1")
	defer os.Unsetenv("TAU_DISABLE_MODELS_FETCH")

	ctx := context.Background()
	result := FetchCatalog(ctx, "/nonexistent/cache/models.json")

	if result != nil {
		t.Error("expected nil when fetch is disabled")
	}
}

func TestTransformCatalog_CostWithCacheFields(t *testing.T) {
	cacheRead := 0.50
	cacheWrite := 5.00

	catalog := rawCatalog{
		"anthropic": rawProvider{
			ID:  "anthropic",
			NPM: ptr("@ai-sdk/anthropic"),
			API: ptr("https://api.anthropic.com/v1"),
			Models: map[string]rawModel{
				"claude-opus-4.7": {
					ID:       "claude-opus-4.7",
					Name:     "Claude Opus 4.7",
					ToolCall: true,
					Reasoning: true,
					Limit:    rawLimit{Context: 200000, Output: 32000},
					Cost: &rawCost{
						Input:      15.00,
						Output:     75.00,
						CacheRead:  &cacheRead,
						CacheWrite: &cacheWrite,
					},
				},
			},
		},
	}

	registry := NewModelRegistry()
	TransformCatalog(catalog, registry)

	m, err := registry.Get("claude-opus-4.7")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}

	if m.Cost.Input != 15.00 {
		t.Errorf("expected input cost 15.00, got %f", m.Cost.Input)
	}
	if m.Cost.Output != 75.00 {
		t.Errorf("expected output cost 75.00, got %f", m.Cost.Output)
	}
	if m.Cost.CacheRead != 0.50 {
		t.Errorf("expected cache read cost 0.50, got %f", m.Cost.CacheRead)
	}
	if m.Cost.CacheWrite != 5.00 {
		t.Errorf("expected cache write cost 5.00, got %f", m.Cost.CacheWrite)
	}
}

func TestTransformCatalog_ReasoningModelGetsThinkingMap(t *testing.T) {
	catalog := rawCatalog{
		"openai": rawProvider{
			ID:  "openai",
			NPM: ptr("@ai-sdk/openai"),
			API: ptr("https://api.openai.com/v1"),
			Models: map[string]rawModel{
				"gpt-5.5": {
					ID:        "gpt-5.5",
					Name:      "GPT-5.5",
					ToolCall:  true,
					Reasoning: true,
					Limit:     rawLimit{Context: 272000, Output: 128000},
				},
				"gpt-4o": {
					ID:       "gpt-4o",
					Name:     "GPT-4o",
					ToolCall: true,
					Limit:    rawLimit{Context: 128000, Output: 16384},
				},
			},
		},
	}

	registry := NewModelRegistry()
	TransformCatalog(catalog, registry)

	gpt55, _ := registry.Get("gpt-5.5")
	if gpt55.ThinkingLevelMap == nil {
		t.Error("gpt-5.5 should have thinking level map (reasoning model)")
	}

	gpt4o, _ := registry.Get("gpt-4o")
	if gpt4o.ThinkingLevelMap != nil {
		t.Error("gpt-4o should not have thinking level map (non-reasoning model)")
	}
}

func TestTransformCatalog_RegistersSameModelUnderMultipleProviders(t *testing.T) {
	// Same model ID from multiple providers should all be registered
	catalog := rawCatalog{
		"openai": rawProvider{
			ID:  "openai",
			NPM: ptr("@ai-sdk/openai"),
			API: ptr("https://api.openai.com/v1"),
			Models: map[string]rawModel{
				"gpt-4o": {
					ID:       "gpt-4o",
					Name:     "GPT-4o",
					ToolCall: true,
					Limit:    rawLimit{Context: 128000, Output: 16384},
				},
			},
		},
		"poe": rawProvider{
			ID:  "poe",
			NPM: ptr("@ai-sdk/openai-compatible"),
			API: ptr("https://poe.com/api"),
			Models: map[string]rawModel{
				"gpt-4o": {
					ID:       "gpt-4o",
					Name:     "GPT-4o (via Poe)",
					ToolCall: true,
					Limit:    rawLimit{Context: 128000, Output: 16384},
				},
			},
		},
		"llmgateway": rawProvider{
			ID:  "llmgateway",
			NPM: ptr("@ai-sdk/openai-compatible"),
			API: ptr("https://llmgateway.io/api"),
			Models: map[string]rawModel{
				"gpt-4o": {
					ID:       "gpt-4o",
					Name:     "GPT-4o (via LLMGateway)",
					ToolCall: true,
					Limit:    rawLimit{Context: 128000, Output: 16384},
				},
			},
		},
	}

	registry := NewModelRegistry()
	count := TransformCatalog(catalog, registry)

	// Should have 3 models (one per provider, no deduplication)
	if count != 3 {
		t.Fatalf("expected 3 models (one per provider), got %d", count)
	}

	// Each provider should have its own gpt-4o
	openaiModels := registry.ListByProvider("openai")
	if len(openaiModels) != 1 || openaiModels[0].ID != "gpt-4o" {
		t.Errorf("expected openai to have gpt-4o, got %v", openaiModels)
	}
	if openaiModels[0].BaseURL != "https://api.openai.com/v1" {
		t.Errorf("expected openai base URL, got %s", openaiModels[0].BaseURL)
	}

	poeModels := registry.ListByProvider("poe")
	if len(poeModels) != 1 || poeModels[0].ID != "gpt-4o" {
		t.Errorf("expected poe to have gpt-4o, got %v", poeModels)
	}
	if poeModels[0].BaseURL != "https://poe.com/api" {
		t.Errorf("expected poe base URL, got %s", poeModels[0].BaseURL)
	}
}

func TestProviderPriority(t *testing.T) {
	tests := []struct {
		provider string
		expected int
	}{
		{"openai", 100},
		{"anthropic", 100},
		{"google", 100},
		{"amazon-bedrock", 80},
		{"github-copilot", 80},
		{"groq", 70},
		{"opencode", 60},
		{"poe", 0},
		{"llmgateway", 0},
		{"unknown-proxy", 0},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			result := providerPriority(tc.provider)
			if result != tc.expected {
				t.Errorf("expected priority %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestReassignToCanonicalProvider_NonStandardIDNotReassigned(t *testing.T) {
	// Proxy providers use non-standard model IDs (e.g., gpt-5-5 with hyphens
	// instead of gpt-5.5 with dots). These should NOT be reassigned to the
	// canonical provider because the canonical provider doesn't have that model.
	tests := []struct {
		name          string
		modelID       string
		origProvider  string
		wantProvider  string
		wantAPI       string
		wantBaseURL   string
	}{
		// Proxy-specific IDs — should NOT be reassigned
		{
			name:         "gpt-5-5 from frogbot should stay as frogbot",
			modelID:      "gpt-5-5",
			origProvider: "frogbot",
			wantProvider: "frogbot",
		},
		{
			name:         "databricks-gpt-5-5 should stay as databricks",
			modelID:      "databricks-gpt-5-5",
			origProvider: "databricks",
			wantProvider: "databricks",
		},
		{
			name:         "openai-gpt-55 from venice should stay as venice",
			modelID:      "openai-gpt-55",
			origProvider: "venice",
			wantProvider: "venice",
		},
		{
			name:         "gpt-5-4-mini from frogbot should stay as frogbot",
			modelID:      "gpt-5-4-mini",
			origProvider: "frogbot",
			wantProvider: "frogbot",
		},
		{
			name:         "duo-chat-gpt-5-5 from gitlab should stay as gitlab",
			modelID:      "duo-chat-gpt-5-5",
			origProvider: "gitlab",
			wantProvider: "gitlab",
		},
		// Official IDs — should be reassigned even if canonical provider not in catalog
		{
			name:         "gpt-5.5 from github-copilot should be reassigned to openai",
			modelID:      "gpt-5.5",
			origProvider: "github-copilot",
			wantProvider: "openai",
			wantAPI:      "openai-responses",
			wantBaseURL:  "https://api.openai.com/v1",
		},
		{
			name:         "gpt-5.4 from poe should be reassigned to openai",
			modelID:      "gpt-5.4",
			origProvider: "poe",
			wantProvider: "openai",
			wantAPI:      "openai-responses",
			wantBaseURL:  "https://api.openai.com/v1",
		},
		{
			name:         "gpt-5.5-pro from llmgateway should be reassigned to openai",
			modelID:      "gpt-5.5-pro",
			origProvider: "llmgateway",
			wantProvider: "openai",
			wantAPI:      "openai-responses",
			wantBaseURL:  "https://api.openai.com/v1",
		},
		{
			name:         "gpt-5-codex from opencode should be reassigned to openai",
			modelID:      "gpt-5-codex",
			origProvider: "opencode",
			wantProvider: "openai",
			wantAPI:      "openai-responses",
			wantBaseURL:  "https://api.openai.com/v1",
		},
		{
			name:         "gpt-5 from poe should be reassigned to openai",
			modelID:      "gpt-5",
			origProvider: "poe",
			wantProvider: "openai",
			wantAPI:      "openai-responses",
			wantBaseURL:  "https://api.openai.com/v1",
		},
		{
			name:         "claude-sonnet-4 from github-copilot should be reassigned to anthropic",
			modelID:      "claude-sonnet-4",
			origProvider: "github-copilot",
			wantProvider: "anthropic",
			wantAPI:      "anthropic-messages",
			wantBaseURL:  "https://api.anthropic.com/v1",
		},
		{
			name:         "gemini-2.5-pro from poe should be reassigned to google",
			modelID:      "gemini-2.5-pro",
			origProvider: "poe",
			wantProvider: "google",
			wantAPI:      "google-generative-ai",
			wantBaseURL:  "https://generativelanguage.googleapis.com/v1beta/models",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := types.Model{
				ID:       tc.modelID,
				Name:     tc.modelID,
				Provider: tc.origProvider,
			}

			// Empty registry — simulates stale catalog where canonical provider
			// doesn't have this model. Reassignment should still happen for official IDs.
			registry := NewModelRegistry()
			registry.RemoveByProvider("openai")
			registry.RemoveByProvider("anthropic")
			registry.RemoveByProvider("google")

			result := reassignToCanonicalProvider(model, registry)

			if result.Provider != tc.wantProvider {
				t.Errorf("provider = %q, want %q", result.Provider, tc.wantProvider)
			}
			if tc.wantAPI != "" && result.API != tc.wantAPI {
				t.Errorf("API = %q, want %q", result.API, tc.wantAPI)
			}
			if tc.wantBaseURL != "" && result.BaseURL != tc.wantBaseURL {
				t.Errorf("BaseURL = %q, want %q", result.BaseURL, tc.wantBaseURL)
			}
		})
	}
}

func TestTransformCatalog_NonStandardProxyModelNotReassigned(t *testing.T) {
	// When a proxy provider has a non-standard model ID (gpt-5-5 instead of
	// gpt-5.5), it should NOT be reassigned to the canonical provider.
	npmOpenAI := "@ai-sdk/openai"

	catalog := rawCatalog{
		"openai": rawProvider{
			ID:   "openai",
			Name: "OpenAI",
			NPM:  &npmOpenAI,
			API:  ptr("https://api.openai.com/v1"),
			Models: map[string]rawModel{
				"gpt-5.5": {
					ID:       "gpt-5.5",
					Name:     "GPT 5.5",
					ToolCall: true,
					Limit:    rawLimit{Context: 272000, Output: 128000},
				},
			},
		},
		"frogbot": rawProvider{
			ID:   "frogbot",
			Name: "FrogBot",
			API:  ptr("https://frogbot.ai/api"),
			Models: map[string]rawModel{
				"gpt-5-5": {
					ID:       "gpt-5-5",
					Name:     "GPT 5-5",
					ToolCall: true,
					Limit:    rawLimit{Context: 272000, Output: 128000},
				},
			},
		},
	}

	registry := NewModelRegistry()
	count := TransformCatalog(catalog, registry)

	// Should have 2 models: gpt-5.5 (openai) and gpt-5-5 (frogbot)
	if count != 2 {
		t.Fatalf("expected 2 models, got %d", count)
	}

	// gpt-5.5 should be from openai
	m1, err := registry.Get("gpt-5.5")
	if err != nil {
		t.Fatalf("gpt-5.5 not found: %v", err)
	}
	if m1.Provider != "openai" {
		t.Errorf("gpt-5.5 provider = %q, want %q", m1.Provider, "openai")
	}

	// gpt-5-5 should stay as frogbot, NOT reassigned to openai
	m2, err := registry.Get("gpt-5-5")
	if err != nil {
		t.Fatalf("gpt-5-5 not found: %v", err)
	}
	if m2.Provider != "frogbot" {
		t.Errorf("gpt-5-5 provider = %q, want %q (should not be reassigned to openai)", m2.Provider, "frogbot")
	}
	if m2.BaseURL == "https://api.openai.com/v1" {
		t.Errorf("gpt-5-5 BaseURL = %q, should not point to openai API", m2.BaseURL)
	}
}

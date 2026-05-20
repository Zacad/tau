package provider

import (
	"fmt"
	"strings"

	"github.com/adam/tau/internal/types"
)

// ModelRegistry holds a catalog of known models and supports lookup by exact ID
// or pattern matching (case-insensitive substring).
type ModelRegistry struct {
	models map[string]types.Model
}

// NewModelRegistry creates a registry pre-loaded with built-in models.
func NewModelRegistry() *ModelRegistry {
	r := &ModelRegistry{
		models: make(map[string]types.Model),
	}
	r.loadBuiltIn()
	return r
}

// Register adds or overwrites a model in the registry.
func (r *ModelRegistry) Register(m types.Model) {
	r.models[m.ID] = m
}

// Find looks up a model by pattern.
//
// 1. If pattern is an exact ID match, returns the model.
// 2. If pattern matches a single model by case-insensitive substring
//    on ID or Name, returns that model.
// 3. If pattern matches multiple models, returns ErrMultipleMatches
//    with the list of candidates.
// 4. If no match, returns ErrModelNotFound.
func (r *ModelRegistry) Find(pattern string) (types.Model, error) {
	if pattern == "" {
		return types.Model{}, fmt.Errorf("empty model pattern")
	}

	// Exact match
	if m, ok := r.models[pattern]; ok {
		return m, nil
	}

	// Substring match
	lower := strings.ToLower(pattern)
	var matches []types.Model
	for _, m := range r.models {
		if strings.Contains(strings.ToLower(m.ID), lower) ||
			strings.Contains(strings.ToLower(m.Name), lower) {
			matches = append(matches, m)
		}
	}

	if len(matches) == 0 {
		return types.Model{}, fmt.Errorf("no model matching %q", pattern)
	}

	if len(matches) > 1 {
		var ids []string
		for _, m := range matches {
			ids = append(ids, m.ID)
		}
		return types.Model{}, fmt.Errorf("multiple models matching %q: %s", pattern, strings.Join(ids, ", "))
	}

	return matches[0], nil
}

// Get returns a model by exact ID.
func (r *ModelRegistry) Get(id string) (types.Model, error) {
	m, ok := r.models[id]
	if !ok {
		return types.Model{}, fmt.Errorf("model %q not found", id)
	}
	return m, nil
}

// ListAll returns all registered models.
func (r *ModelRegistry) ListAll() []types.Model {
	result := make([]types.Model, 0, len(r.models))
	for _, m := range r.models {
		result = append(result, m)
	}
	return result
}

// ListByProvider returns models for a specific provider.
func (r *ModelRegistry) ListByProvider(providerName string) []types.Model {
	var result []types.Model
	for _, m := range r.models {
		if m.Provider == providerName {
			result = append(result, m)
		}
	}
	return result
}

// RemoveByProvider removes all models belonging to a specific provider.
func (r *ModelRegistry) RemoveByProvider(providerName string) {
	for id, m := range r.models {
		if m.Provider == providerName {
			delete(r.models, id)
		}
	}
}

// loadBuiltIn populates the registry with known models.
func (r *ModelRegistry) loadBuiltIn() {
	// OpenAI models
	openAIModels := []types.Model{
		{
			ID:            "gpt-4o",
			Name:          "GPT-4o",
			Provider:      "openai",
			API:           "openai-responses",
			BaseURL:       "https://api.openai.com/v1",
			Reasoning:     false,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 2.50, Output: 10.00, CacheRead: 1.25, CacheWrite: 5.00},
			ContextWindow: 128000,
			MaxTokens:     16384,
		},
		{
			ID:            "gpt-4o-mini",
			Name:          "GPT-4o Mini",
			Provider:      "openai",
			API:           "openai-responses",
			BaseURL:       "https://api.openai.com/v1",
			Reasoning:     false,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 0.15, Output: 0.60, CacheRead: 0.075, CacheWrite: 0.30},
			ContextWindow: 128000,
			MaxTokens:     16384,
		},
		{
			ID:            "o1",
			Name:          "o1",
			Provider:      "openai",
			API:           "openai-responses",
			BaseURL:       "https://api.openai.com/v1",
			Reasoning:     true,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 15.00, Output: 60.00},
			ContextWindow: 200000,
			MaxTokens:     100000,
			ThinkingLevelMap: map[string]string{
				"off":     "none",
				"minimal": "minimal",
				"low":     "low",
				"medium":  "medium",
				"high":    "high",
				"xhigh":   "xhigh",
			},
		},
		{
			ID:            "o3",
			Name:          "o3",
			Provider:      "openai",
			API:           "openai-responses",
			BaseURL:       "https://api.openai.com/v1",
			Reasoning:     true,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 10.00, Output: 40.00},
			ContextWindow: 200000,
			MaxTokens:     100000,
			ThinkingLevelMap: map[string]string{
				"off":     "none",
				"minimal": "minimal",
				"low":     "low",
				"medium":  "medium",
				"high":    "high",
				"xhigh":   "xhigh",
			},
		},
	}

	// Anthropic models
	anthropicModels := []types.Model{
		{
			ID:            "claude-sonnet-4-20250514",
			Name:          "Claude Sonnet 4",
			Provider:      "anthropic",
			API:           "anthropic-messages",
			BaseURL:       "https://api.anthropic.com/v1",
			Reasoning:     true,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
			ContextWindow: 200000,
			MaxTokens:     8192,
			ThinkingLevelMap: map[string]string{
				"low":   "low",
				"medium": "medium",
				"high":  "high",
				"xhigh": "xhigh",
			},
		},
		{
			ID:            "claude-3-7-sonnet-20250219",
			Name:          "Claude 3.7 Sonnet",
			Provider:      "anthropic",
			API:           "anthropic-messages",
			BaseURL:       "https://api.anthropic.com/v1",
			Reasoning:     true,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
			ContextWindow: 200000,
			MaxTokens:     8192,
			ThinkingLevelMap: map[string]string{
				"low":   "low",
				"medium": "medium",
				"high":  "high",
				"xhigh": "xhigh",
			},
		},
		{
			ID:            "claude-3-5-sonnet-20241022",
			Name:          "Claude 3.5 Sonnet",
			Provider:      "anthropic",
			API:           "anthropic-messages",
			BaseURL:       "https://api.anthropic.com/v1",
			Reasoning:     false,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
			ContextWindow: 200000,
			MaxTokens:     8192,
		},
	}

	// Google models
	googleModels := []types.Model{
		{
			ID:            "gemini-2.5-pro",
			Name:          "Gemini 2.5 Pro",
			Provider:      "google",
			API:           "google-generative-ai",
			BaseURL:       "https://generativelanguage.googleapis.com/v1beta/models",
			Reasoning:     true,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 1.25, Output: 10.00},
			ContextWindow: 1000000,
			MaxTokens:     65536,
			ThinkingLevelMap: map[string]string{
				"minimal": "128",
				"low":     "2048",
				"medium":  "8192",
				"high":    "32768",
			},
		},
		{
			ID:            "gemini-2.5-flash",
			Name:          "Gemini 2.5 Flash",
			Provider:      "google",
			API:           "google-generative-ai",
			BaseURL:       "https://generativelanguage.googleapis.com/v1beta/models",
			Reasoning:     true,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 0.15, Output: 0.60},
			ContextWindow: 1000000,
			MaxTokens:     65536,
			ThinkingLevelMap: map[string]string{
				"minimal": "128",
				"low":     "2048",
				"medium":  "8192",
				"high":    "24576",
			},
		},
		{
			ID:            "gemini-2.0-flash",
			Name:          "Gemini 2.0 Flash",
			Provider:      "google",
			API:           "google-generative-ai",
			BaseURL:       "https://generativelanguage.googleapis.com/v1beta/models",
			Reasoning:     false,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 0.10, Output: 0.40},
			ContextWindow: 1000000,
			MaxTokens:     8192,
		},
	}

	for _, m := range openAIModels {
		r.Register(m)
	}
	for _, m := range anthropicModels {
		r.Register(m)
	}
	for _, m := range googleModels {
		r.Register(m)
	}
}

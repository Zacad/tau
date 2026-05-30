package provider

import (
	"fmt"
	"strings"

	"github.com/adam/tau/internal/types"
)

// ParseModelRef splits a model reference into provider and model ID.
// Uses first-slash split so model IDs containing slashes are preserved
// (e.g., "openrouter/openai/gpt-4o" → provider="openrouter", modelID="openai/gpt-4o").
// Returns empty provider for bare model IDs (backward compatibility).
func ParseModelRef(ref string) (provider, modelID string) {
	if ref == "" {
		return "", ""
	}
	slashIdx := strings.Index(ref, "/")
	if slashIdx == -1 {
		return "", ref
	}
	return ref[:slashIdx], ref[slashIdx+1:]
}

// ModelKey returns the canonical compound key for a model.
func ModelKey(provider, modelID string) string {
	return provider + "/" + modelID
}

// ModelRegistry holds a catalog of known models and supports lookup by exact ID
// or pattern matching (case-insensitive substring).
//
// Internally keyed by compound "provider/modelID" to support the same model ID
// under multiple providers.
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
// Uses compound key "provider/modelID" to support same ID across providers.
func (r *ModelRegistry) Register(m types.Model) {
	key := ModelKey(m.Provider, m.ID)
	r.models[key] = m
}

// Find looks up a model by pattern.
//
// 1. If pattern is "provider/modelID", tries exact compound key match.
// 2. If pattern is an exact bare ID match (unique across providers), returns the model.
// 3. If pattern matches a single model by case-insensitive substring on ID or Name, returns that model.
// 4. If pattern matches multiple models, returns ErrMultipleMatches with the list of candidates.
// 5. If no match, returns ErrModelNotFound.
func (r *ModelRegistry) Find(pattern string) (types.Model, error) {
	if pattern == "" {
		return types.Model{}, fmt.Errorf("empty model pattern")
	}

	// Try exact compound key match (provider/modelID)
	if m, ok := r.models[pattern]; ok {
		return m, nil
	}

	// Try exact bare ID match
	prov, modelID := ParseModelRef(pattern)
	if prov != "" {
		// provider/modelID format — try exact match on that compound key
		if m, ok := r.models[pattern]; ok {
			return m, nil
		}
		// Also try matching just the modelID part under the specified provider
		for _, m := range r.models {
			if strings.EqualFold(m.Provider, prov) && strings.EqualFold(m.ID, modelID) {
				return m, nil
			}
		}
		return types.Model{}, fmt.Errorf("model %q not found", pattern)
	}

	// Bare model ID — exact match (must be unique across providers)
	var exactMatches []types.Model
	for _, m := range r.models {
		if strings.EqualFold(m.ID, pattern) {
			exactMatches = append(exactMatches, m)
		}
	}
	if len(exactMatches) == 1 {
		return exactMatches[0], nil
	}
	if len(exactMatches) > 1 {
		var ids []string
		for _, m := range exactMatches {
			ids = append(ids, ModelKey(m.Provider, m.ID))
		}
		return types.Model{}, fmt.Errorf("ambiguous model %q, use provider/modelID format: %s", pattern, strings.Join(ids, ", "))
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
			ids = append(ids, ModelKey(m.Provider, m.ID))
		}
		return types.Model{}, fmt.Errorf("multiple models matching %q: %s", pattern, strings.Join(ids, ", "))
	}

	return matches[0], nil
}

// Get returns a model by exact ID or compound key.
func (r *ModelRegistry) Get(id string) (types.Model, error) {
	// Try compound key first
	if m, ok := r.models[id]; ok {
		return m, nil
	}
	// Try bare ID (must be unique)
	for _, m := range r.models {
		if strings.EqualFold(m.ID, id) {
			return m, nil
		}
	}
	return types.Model{}, fmt.Errorf("model %q not found", id)
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

// loadBuiltIn populates the registry with a minimal set of known models.
// This serves as an offline fallback when the models.dev catalog is unavailable.
// The full catalog is loaded dynamically via LoadFromCatalog().
func (r *ModelRegistry) loadBuiltIn() {
	// Minimal OpenAI fallback
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
	}

	// Minimal Anthropic fallback
	anthropicModels := []types.Model{
		{
			ID:            "claude-sonnet-4-6",
			Name:          "Claude Sonnet 4.6",
			Provider:      "anthropic",
			API:           "anthropic-messages",
			BaseURL:       "https://api.anthropic.com/v1",
			Reasoning:     true,
			InputTypes:    []string{"text", "image"},
			Cost:          types.CostInfo{Input: 3.00, Output: 15.00, CacheRead: 0.30, CacheWrite: 3.75},
			ContextWindow: 200000,
			MaxTokens:     8192,
			ThinkingLevelMap: map[string]string{
				"low":    "low",
				"medium": "medium",
				"high":   "high",
				"xhigh":  "xhigh",
			},
		},
	}

	// Minimal Google fallback
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

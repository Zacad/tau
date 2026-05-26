package provider

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adam/tau/internal/types"
)

const (
	defaultModelsURL = "https://models.dev/api.json"
	cacheTTL         = 5 * time.Minute
	fetchTimeout     = 10 * time.Second
)

// rawCatalog is the top-level models.dev response keyed by provider ID.
type rawCatalog map[string]rawProvider

type rawProvider struct {
	ID     string                  `json:"id"`
	Name   string                  `json:"name"`
	Env    []string                `json:"env"`
	NPM    *string                 `json:"npm,omitempty"`
	API    *string                 `json:"api,omitempty"`
	Models map[string]rawModel     `json:"models"`
}

type rawModel struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Family       *string             `json:"family,omitempty"`
	ReleaseDate  string              `json:"release_date"`
	Attachment   bool                `json:"attachment"`
	Reasoning    bool                `json:"reasoning"`
	Temperature  bool                `json:"temperature"`
	ToolCall     bool                `json:"tool_call"`
	Cost         *rawCost            `json:"cost,omitempty"`
	Limit        rawLimit            `json:"limit"`
	Modalities   *rawModalities      `json:"modalities,omitempty"`
	Status       *string             `json:"status,omitempty"`
	Provider     *rawModelProvider   `json:"provider,omitempty"`
	Experimental *rawExperimental    `json:"experimental,omitempty"`
}

type rawCost struct {
	Input         float64          `json:"input"`
	Output        float64         `json:"output"`
	CacheRead     *float64        `json:"cache_read,omitempty"`
	CacheWrite    *float64        `json:"cache_write,omitempty"`
	ContextOver200K *rawCost      `json:"context_over_200k,omitempty"`
}

type rawLimit struct {
	Context int `json:"context"`
	Input   *int `json:"input,omitempty"`
	Output  int `json:"output"`
}

type rawModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type rawModelProvider struct {
	NPM *string `json:"npm,omitempty"`
	API *string `json:"api,omitempty"`
}

type rawExperimental struct {
	Modes map[string]rawMode `json:"modes,omitempty"`
}

type rawMode struct {
	Cost     *rawCost           `json:"cost,omitempty"`
	Provider *rawModelProvider  `json:"provider,omitempty"`
}

// FetchCatalog fetches the models.dev catalog with disk caching.
// Returns the raw catalog data. On failure, returns nil (caller should use fallback).
func FetchCatalog(ctx context.Context, cachePath string) rawCatalog {
	// Check disk cache
	if cached := loadFromCache(cachePath); cached != nil {
		return cached
	}

	// Fetch from network
	data := fetchFromNetwork(ctx)
	if data == nil {
		return nil
	}

	// Write to cache (best-effort)
	if err := writeCache(cachePath, data); err != nil {
		slog.Debug("failed to write models cache", "path", cachePath, "error", err)
	}

	return data
}

// loadFromCache reads the cached catalog if it's fresh (< TTL).
func loadFromCache(cachePath string) rawCatalog {
	info, err := os.Stat(cachePath)
	if err != nil {
		return nil
	}

	// Check TTL
	if time.Since(info.ModTime()) > cacheTTL {
		return nil
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}

	var catalog rawCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		slog.Debug("failed to parse cached models", "error", err)
		return nil
	}

	slog.Debug("loaded models from cache", "path", cachePath, "age", time.Since(info.ModTime()).Round(time.Second))
	return catalog
}

// fetchFromNetwork fetches the catalog from models.dev with timeout and retry.
func fetchFromNetwork(ctx context.Context) rawCatalog {
	url := os.Getenv("TAU_MODELS_URL")
	if url == "" {
		url = defaultModelsURL
	}

	// Check if fetch is disabled
	if os.Getenv("TAU_DISABLE_MODELS_FETCH") != "" {
		slog.Debug("models fetch disabled via TAU_DISABLE_MODELS_FETCH")
		return nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(fetchCtx, http.MethodGet, url, nil)
	if err != nil {
		slog.Debug("failed to create models fetch request", "error", err)
		return nil
	}
	req.Header.Set("User-Agent", "tau/1.0")

	client := &http.Client{Timeout: fetchTimeout}

	var resp *http.Response
	var lastErr error

	// 2 retries with exponential backoff
	for attempt := 0; attempt <= 2; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(200*(1<<uint(attempt-1))) * time.Millisecond
			slog.Debug("retrying models fetch", "attempt", attempt, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-fetchCtx.Done():
				return nil
			}
		}

		resp, lastErr = client.Do(req)
		if lastErr == nil {
			break
		}
	}

	if lastErr != nil {
		slog.Debug("models fetch failed after retries", "error", lastErr)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Debug("models fetch returned non-200", "status", resp.StatusCode)
		return nil
	}

	var catalog rawCatalog
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		slog.Debug("failed to decode models response", "error", err)
		return nil
	}

	return catalog
}

// writeCache writes the catalog to disk.
func writeCache(path string, catalog rawCatalog) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.Marshal(catalog)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// LoadFromCatalog fetches the models.dev catalog and registers models into the registry.
// If the fetch fails, falls back to the built-in minimal models.
func LoadFromCatalog(ctx context.Context, registry *ModelRegistry, cachePath string) {
	catalog := FetchCatalog(ctx, cachePath)
	if catalog == nil {
		slog.Info("using built-in model fallback (catalog unavailable)")
		return
	}

	count := TransformCatalog(catalog, registry)
	slog.Info("loaded models from catalog", "count", count)
}

// TransformCatalog transforms raw models.dev data into types.Model and registers them.
// Returns the number of models registered.
//
// Models are registered under their respective provider without deduplication by ID.
// The same model ID can exist under multiple providers (e.g., gpt-4o from openai, poe, llmgateway).
// The registry uses compound "provider/modelID" keys internally.
func TransformCatalog(catalog rawCatalog, registry *ModelRegistry) int {
	// Clear existing provider models before repopulating
	for providerID := range catalog {
		registry.RemoveByProvider(providerID)
	}

	count := 0
	for providerID, rawProv := range catalog {
		apiType := resolveAPIType(rawProv)
		baseURL := resolveBaseURL(rawProv)

		for modelID, rawModel := range rawProv.Models {
			// Skip deprecated models
			if rawModel.Status != nil && *rawModel.Status == "deprecated" {
				continue
			}

			// Skip models without tool_call support (tau requires tools)
			if !rawModel.ToolCall {
				continue
			}

			model := transformModel(providerID, modelID, rawModel, apiType, baseURL)

			// Register model under its original provider.
			// No reassignment needed — models are now keyed by provider/modelID,
			// so the same model ID can exist under multiple providers.
			registry.Register(model)
			count++
		}
	}

	return count
}

// reassignToCanonicalProvider reassigns a model to its canonical provider based
// on model ID pattern. This handles models.dev CDN inconsistency where canonical
// models (claude-*, gpt-*, gemini-*) sometimes appear only under proxy providers.
//
// Proxy-specific IDs (e.g., gpt-5-5 with hyphens instead of gpt-5.5 with dots,
// openai-gpt-55, databricks-gpt-5-5) are NOT reassigned because they don't match
// the canonical provider's official naming format.
//
// Returns the model unchanged if already canonical or if no canonical provider is known.
func reassignToCanonicalProvider(model types.Model, registry *ModelRegistry) types.Model {
	canonical := canonicalProviderForModel(model.ID)
	if canonical == "" || model.Provider == canonical {
		return model
	}

	// Check if the canonical provider already has this exact model ID.
	// If so, reassign (handles dedup scenarios where proxy won priority).
	for _, m := range registry.ListByProvider(canonical) {
		if m.ID == model.ID {
			model.Provider = canonical
			model.API, model.BaseURL = canonicalProviderAPI(canonical)
			return model
		}
	}

	// Canonical provider doesn't have this exact ID in the catalog (may be stale).
	// Only reassign if the model ID matches the canonical provider's official
	// naming format. This prevents proxy-specific IDs from being reassigned.
	if !isOfficialModelID(model.ID, canonical) {
		return model
	}

	model.Provider = canonical
	model.API, model.BaseURL = canonicalProviderAPI(canonical)
	return model
}

// canonicalProviderAPI returns the API type and base URL for a canonical provider.
func canonicalProviderAPI(provider string) (string, string) {
	switch provider {
	case "openai":
		return "openai-responses", "https://api.openai.com/v1"
	case "anthropic":
		return "anthropic-messages", "https://api.anthropic.com/v1"
	case "google":
		return "google-generative-ai", "https://generativelanguage.googleapis.com/v1beta/models"
	}
	return "", ""
}

// isOfficialModelID checks if a model ID matches the canonical provider's
// official naming format. Proxy providers often use non-standard IDs:
//   - Replace dots with hyphens: gpt-5-5 instead of gpt-5.5
//   - Add prefixes: openai-gpt-55, databricks-gpt-5-5
//   - Concatenate versions: openai-gpt-55-pro
//
// Official IDs use dots for version numbers (gpt-5.5, gemini-2.5-pro) or
// standard suffixes (gpt-5-codex, claude-sonnet-4).
func isOfficialModelID(modelID, canonical string) bool {
	id := strings.ToLower(modelID)
	switch canonical {
	case "openai":
		// Official: gpt-X.Y[-suffix], gpt-X[-suffix], o1/o3/o4[-suffix]
		// Reject: gpt-X-Y (hyphen instead of dot), openai-gpt-*, databricks-gpt-*
		if strings.HasPrefix(id, "gpt-") {
			// Must have dot in version OR be a known base model pattern
			// gpt-5.5, gpt-5.4, gpt-5.1, etc. → official
			// gpt-5, gpt-5-codex, gpt-5-mini, gpt-5-pro → official
			// gpt-5-5, gpt-5-4 → NOT official (hyphen instead of dot)
			// Strip "gpt-" prefix
			rest := id[4:]
			// Check if it starts with digit.digit (official version format)
			if len(rest) >= 3 && rest[1] == '.' && rest[0] >= '0' && rest[0] <= '9' && rest[2] >= '0' && rest[2] <= '9' {
				return true
			}
			// Check if it's digit followed by hyphen or end (base model like gpt-5, gpt-5-codex)
			if len(rest) >= 1 && rest[0] >= '0' && rest[0] <= '9' {
				if len(rest) == 1 || rest[1] == '-' || rest[1] == '.' {
					// gpt-5, gpt-5-codex, gpt-5-mini → official
					// But NOT gpt-5-5 (rest = "5-5", rest[1] = '-', but rest[0]='5' is digit, rest[2]='5' is digit)
					// Need to check: after the first digit, if there's a hyphen, the next char should NOT be a digit
					if len(rest) >= 3 && rest[1] == '-' && rest[2] >= '0' && rest[2] <= '9' {
						return false // gpt-5-5, gpt-5-4, etc.
					}
					return true
				}
			}
			return false
		}
		// o-series: o1, o3, o4 with optional suffixes
		if len(id) >= 2 && (id[:2] == "o1" || id[:2] == "o3" || id[:2] == "o4") {
			if len(id) == 2 || id[2] == '-' {
				return true
			}
		}
		return false

	case "anthropic":
		// Official: claude-{name}-X.Y[-suffix] or claude-{name}-X[-suffix]
		// Reject: claude-*-X-Y where Y looks like a dot replacement
		if strings.HasPrefix(id, "claude-") {
			// Check for hyphen-digit-hyphen-digit pattern (proxy-specific)
			// Official: claude-sonnet-4, claude-sonnet-4-5 (date suffix -YYYYMMDD)
			// Proxy: claude-sonnet-4-5 where 4-5 means 4.5
			// Distinguish by checking if the "digit-digit" part looks like a date
			parts := strings.Split(id, "-")
			for i := 2; i < len(parts)-1; i++ {
				// Check for pattern: name-digit-digit where both are single digits
				if len(parts[i]) == 1 && parts[i][0] >= '0' && parts[i][0] <= '9' &&
					len(parts[i+1]) == 1 && parts[i+1][0] >= '0' && parts[i+1][0] <= '9' {
					// This looks like a dot replacement (e.g., claude-sonnet-4-5 → 4.5)
					// But it could also be a date suffix like -20250514
					// Date suffixes are 8+ digits, single digits are version numbers
					return false
				}
			}
			return true
		}
		return false

	case "google":
		// Official: gemini-X.Y[-suffix], gemma-X[-suffix]
		// Reject: gemini-XY (concatenated), gemini-X-Y (hyphen version)
		if strings.HasPrefix(id, "gemini-") {
			rest := id[7:]
			if len(rest) >= 3 && rest[1] == '.' && rest[0] >= '0' && rest[0] <= '9' && rest[2] >= '0' && rest[2] <= '9' {
				return true
			}
			if len(rest) >= 1 && rest[0] >= '0' && rest[0] <= '9' {
				if len(rest) == 1 || rest[1] == '-' {
					if len(rest) >= 3 && rest[1] == '-' && rest[2] >= '0' && rest[2] <= '9' {
						return false
					}
					return true
				}
			}
			return false
		}
		if strings.HasPrefix(id, "gemma-") {
			return true
		}
		return false
	}
	return false
}

// canonicalProviderForModel returns the canonical provider for a model ID based
// on naming patterns. Returns empty string if no canonical provider is known.
func canonicalProviderForModel(modelID string) string {
	id := strings.ToLower(modelID)
	switch {
	case strings.HasPrefix(id, "gpt-") || strings.HasPrefix(id, "o1") ||
		strings.HasPrefix(id, "o3") || strings.HasPrefix(id, "o4"):
		return "openai"
	case strings.HasPrefix(id, "claude-"):
		return "anthropic"
	case strings.HasPrefix(id, "gemini-") || strings.HasPrefix(id, "gemma-"):
		return "google"
	case strings.HasPrefix(id, "llama-") || strings.HasPrefix(id, "codellama"):
		return "" // Meta models — no single canonical API provider
	default:
		return ""
	}
}

// providerPriority returns a priority score for a provider.
// Higher scores are preferred when deduplicating models with the same ID.
// Official API providers get the highest priority.
func providerPriority(providerID string) int {
	switch providerID {
	case "openai":
		return 100
	case "anthropic":
		return 100
	case "google":
		return 100
	case "amazon-bedrock", "azure-openai-responses", "github-copilot":
		return 80
	case "groq", "cerebras", "mistral", "xai", "cohere", "fireworks",
		"together", "huggingface", "cloudflare", "perplexity", "minimax", "moonshotai",
		"zai", "deepseek":
		return 70
	case "opencode", "opencode-go":
		return 60
	default:
		return 0 // Proxy/aggregator providers
	}
}

// resolveAPIType determines the API type from provider NPM package.
func resolveAPIType(rawProv rawProvider) string {
	if rawProv.NPM == nil {
		return "openai-completions"
	}

	switch *rawProv.NPM {
	case "@ai-sdk/openai":
		return "openai-responses"
	case "@ai-sdk/anthropic":
		return "anthropic-messages"
	case "@ai-sdk/google":
		return "google-generative-ai"
	case "@ai-sdk/openai-compatible":
		return "openai-completions"
	case "@ai-sdk/alibaba":
		return "openai-completions"
	default:
		return "openai-completions"
	}
}

// resolveBaseURL determines the base URL from provider data.
func resolveBaseURL(rawProv rawProvider) string {
	if rawProv.API != nil {
		return *rawProv.API
	}

	if rawProv.NPM != nil {
		switch *rawProv.NPM {
		case "@ai-sdk/openai":
			return "https://api.openai.com/v1"
		case "@ai-sdk/anthropic":
			return "https://api.anthropic.com/v1"
		case "@ai-sdk/google":
			return "https://generativelanguage.googleapis.com/v1beta/models"
		}
	}

	return ""
}

// transformModel converts a raw model to types.Model.
func transformModel(providerID, modelID string, raw rawModel, apiType, baseURL string) types.Model {
	model := types.Model{
		ID:            modelID,
		Name:          raw.Name,
		Provider:      providerID,
		API:           apiType,
		BaseURL:       baseURL,
		Reasoning:     raw.Reasoning,
		ContextWindow: raw.Limit.Context,
		MaxTokens:     raw.Limit.Output,
	}

	// Cost info
	if raw.Cost != nil {
		model.Cost = types.CostInfo{
			Input:  raw.Cost.Input,
			Output: raw.Cost.Output,
		}
		if raw.Cost.CacheRead != nil {
			model.Cost.CacheRead = *raw.Cost.CacheRead
		}
		if raw.Cost.CacheWrite != nil {
			model.Cost.CacheWrite = *raw.Cost.CacheWrite
		}
	}

	// Input types from modalities
	if raw.Modalities != nil && len(raw.Modalities.Input) > 0 {
		model.InputTypes = raw.Modalities.Input
	} else {
		model.InputTypes = []string{"text"}
	}

	// Thinking level map for reasoning models
	if raw.Reasoning {
		model.ThinkingLevelMap = thinkingLevelMapFor(providerID, modelID)
	}

	// Model-level provider override
	if raw.Provider != nil {
		if raw.Provider.NPM != nil {
			model.API = resolveAPIType(rawProvider{NPM: raw.Provider.NPM})
		}
		if raw.Provider.API != nil {
			model.BaseURL = *raw.Provider.API
		}
	}

	return model
}

// thinkingLevelMapFor returns the thinking level map for a provider/model combination.
// models.dev doesn't provide this data, so we maintain it here (like PI does).
func thinkingLevelMapFor(providerID, modelID string) map[string]string {
	id := strings.ToLower(modelID)

	switch providerID {
	case "openai":
		// o-series models
		if strings.HasPrefix(id, "o1") || strings.HasPrefix(id, "o3") || strings.HasPrefix(id, "o4") {
			return map[string]string{
				"off": "none", "minimal": "minimal", "low": "low",
				"medium": "medium", "high": "high", "xhigh": "xhigh",
			}
		}
		// gpt-5.x models
		if strings.HasPrefix(id, "gpt-5") {
			base := map[string]string{
				"off": "none",
			}
			// Newer gpt-5.x support effort-based thinking
			if strings.HasPrefix(id, "gpt-5.2") || strings.HasPrefix(id, "gpt-5.3") ||
				strings.HasPrefix(id, "gpt-5.4") || strings.HasPrefix(id, "gpt-5.5") {
				base["minimal"] = "minimal"
				base["low"] = "low"
				base["medium"] = "medium"
				base["high"] = "high"
				base["xhigh"] = "xhigh"
			}
			return base
		}
		return nil

	case "anthropic":
		// Opus 4.6+ uses adaptive thinking (effort-based)
		if strings.Contains(id, "opus-4.6") || strings.Contains(id, "opus-4.7") {
			return map[string]string{
				"low":    "low",
				"medium": "medium",
				"high":   "high",
				"xhigh":  "max",
			}
		}
		// Sonnet 4.x, Opus 4.5, and 3-7-sonnet use budget-based thinking
		if strings.Contains(id, "sonnet-4") || strings.Contains(id, "opus-4") || strings.Contains(id, "3-7-sonnet") {
			return map[string]string{
				"minimal": "1024",
				"low":     "2048",
				"medium":  "4096",
				"high":    "8192",
				"xhigh":   "16384",
			}
		}
		return nil

	case "google":
		if strings.Contains(id, "gemini-2.5") || strings.Contains(id, "gemini-3") {
			// Gemini uses token budgets
			return map[string]string{
				"minimal": "128",
				"low":     "2048",
				"medium":  "8192",
				"high":    "32768",
			}
		}
		if strings.Contains(id, "gemma") {
			return map[string]string{
				"minimal": "MINIMAL",
				"low":     "LOW",
				"medium":  "MEDIUM",
				"high":    "HIGH",
			}
		}
		return nil

	default:
		// Generic fallback for other providers
		if strings.Contains(id, "r1") || strings.Contains(id, "reasoning") {
			return map[string]string{
				"low":    "low",
				"medium": "medium",
				"high":   "high",
			}
		}
		return nil
	}
}

// CachePath returns the default cache path for the models catalog.
func CachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".tau", "cache", "models.json")
}

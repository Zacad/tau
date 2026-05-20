package tui

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/provider"
	"github.com/adam/tau/internal/tui/palette"
	"github.com/adam/tau/internal/types"
)

// connectSteps returns the multi-step flow for the /connect command.
func connectSteps(m *Model) []palette.Step {
	providers := listAvailableProvidersWithState(m)
	opts := make([]palette.ListOption, len(providers))
	for i, p := range providers {
		status := ""
		if p.Enabled {
			status = "connected"
		} else if p.HasConfig {
			status = "disabled"
		}
		if p.HasAuth {
			if status != "" {
				status += " • credentials saved"
			} else {
				status = "credentials saved"
			}
		}

		desc := p.Description
		if status != "" {
			desc = fmt.Sprintf("%s • %s", p.Description, status)
		}
		opts[i] = palette.ListOption{
			Title:       p.DisplayName,
			Description: desc,
			Value:       p.Name,
		}
	}

	return []palette.Step{
		palette.ListStep("Select Provider", "Choose a provider to connect to:", opts),
		palette.InputStep(
			"API Key",
			"Enter your API key (leave empty to use saved credentials):",
			"sk-...",
		),
		palette.TaskStep("Test Connection", func(results map[string]any) (bool, string, error) {
			providerName, _ := results["select_provider"].(string)
			apiKey, _ := results["api_key"].(string)

			// If API key is empty, try to load from auth.json
			if apiKey == "" {
				providerState := getProviderState(providerName)
				apiKey = providerState.APIKey
			}

			info, ok := findProvider(providerName)
			if !ok {
				return false, "", fmt.Errorf("unknown provider: %s", providerName)
			}

			// Validate API key before testing
			if info.RequiresAPIKey && apiKey == "" {
				return false, "API key is required but not provided", fmt.Errorf("missing API key for %s", info.DisplayName)
			}

			if err := testProviderConnection(info, apiKey); err != nil {
				return false, fmt.Sprintf("Connection to %s failed: %v", info.DisplayName, err), err
			}
			return true, fmt.Sprintf("Connected to %s successfully", info.DisplayName), nil
		}),
		palette.TaskStep("Discover Models", func(results map[string]any) (bool, string, error) {
			providerName, _ := results["select_provider"].(string)
			apiKey, _ := results["api_key"].(string)

			// If API key is empty, try to load from auth.json
			if apiKey == "" {
				providerState := getProviderState(providerName)
				apiKey = providerState.APIKey
			}

			info, ok := findProvider(providerName)
			if !ok {
				return false, "", fmt.Errorf("unknown provider: %s", providerName)
			}

			models, err := discoverProviderModels(info, apiKey)
			slog.Debug("discover models result", "provider", info.Name, "models", models, "err", err)
			if err != nil {
				return false, fmt.Sprintf("Model discovery failed: %v", err), err
			}
			// Store models in results for handleConnectResult to use.
			results["discover_models"] = models
			slog.Debug("stored discover_models in results", "type", fmt.Sprintf("%T", models), "count", len(models))
			if len(models) == 0 {
				return true, fmt.Sprintf("No models found for %s", info.DisplayName), nil
			}
			return true, fmt.Sprintf("Discovered %d model(s) from %s", len(models), info.DisplayName), nil
		}),
		palette.ConfirmStep("Save", "Save credentials and register provider?"),
	}
}

// handleConnectResult processes the collected results and performs the actual connection.
func handleConnectResult(m *Model, results map[string]any) {
	providerName, _ := results["select_provider"].(string)
	apiKey, _ := results["api_key"].(string)
	saveConfirmed, _ := results["save"].(bool)

	if !saveConfirmed {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "Connection cancelled — credentials not saved.",
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

	info, ok := findProvider(providerName)
	if !ok {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Unknown provider: %s", providerName),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

	// Determine if we're using a new key or existing one
	state := getProviderState(providerName)
	usingExistingKey := apiKey == "" && state.HasAuth
	if usingExistingKey {
		apiKey = state.APIKey
	}

	// Save credentials to auth.json (only if new key provided)
	if apiKey != "" {
		if err := saveProviderAuth(providerName, apiKey); err != nil {
			m.blocks = append(m.blocks, messageBlock{
				kind: blockError,
				text: fmt.Sprintf("Failed to save credentials: %v", err),
			})
			m.invalidateRenderedCache()
			m.updateViewport()
			return
		}
	}

	// Update config.json with enabled state and base URL
	if err := saveProviderConfig(providerName, info.BaseURL); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Failed to update config: %v", err),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

	// Register provider into session's registry
	modelsRaw, hasModels := results["discover_models"]
	models, ok := modelsRaw.([]string)
	slog.Debug("handleConnectResult: extracted models",
		"has_key", hasModels,
		"type", fmt.Sprintf("%T", modelsRaw),
		"type_assert_ok", ok,
		"count", len(models),
		"models", models,
	)
	if err := registerConnectedProvider(m, info, apiKey, models); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Provider registered but model registration failed: %v", err),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

	// Build success message
	var lines []string
	lines = append(lines, fmt.Sprintf("✓ Successfully connected to **%s**", info.DisplayName))
	lines = append(lines, "")

	if usingExistingKey {
		lines = append(lines, "- Using existing credentials from `~/.tau/auth.json`")
	} else if info.RequiresAPIKey {
		lines = append(lines, "- Credentials saved to `~/.tau/auth.json`")
	} else {
		lines = append(lines, "- Connected without API key")
	}
	lines = append(lines, "- Provider enabled in `~/.tau/config.json`")

	if len(models) > 0 {
		lines = append(lines, fmt.Sprintf("- %d model(s) discovered and registered:", len(models)))
		for _, model := range models {
			lines = append(lines, fmt.Sprintf("  - `%s`", model))
		}
	} else {
		lines = append(lines, "- No models discovered (using default model list)")
	}

	lines = append(lines, "")
	lines = append(lines, "You can now use `/model` to select from available models.")

	m.blocks = append(m.blocks, messageBlock{
		kind: blockAssistantText,
		text: strings.Join(lines, "\n"),
	})
	m.invalidateRenderedCache()
	m.updateViewport()
}

// saveProviderAuth saves the API key to auth.json.
func saveProviderAuth(providerName, apiKey string) error {
	if apiKey == "" {
		return nil // Keyless provider
	}

	authPath := config.AuthPath("")
	store, err := config.LoadAuth(authPath)
	if err != nil {
		return fmt.Errorf("load existing auth: %w", err)
	}

	store[providerName] = apiKey

	if err := config.SaveAuth(store, authPath); err != nil {
		return fmt.Errorf("save auth: %w", err)
	}

	slog.Info("provider auth saved", "provider", providerName)
	return nil
}

// saveProviderConfig updates config.json with provider enabled state and base URL.
func saveProviderConfig(providerName, baseURL string) error {
	cfg, err := config.LoadConfig("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]config.ProviderConfig)
	}

	// Store provider config with enabled state
	pc := cfg.Providers[providerName]
	enabled := true
	pc.Enabled = &enabled
	if baseURL != "" {
		pc.BaseURL = baseURL
	}
	cfg.Providers[providerName] = pc

	if err := config.SaveConfig(cfg, ""); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	slog.Info("provider config saved", "provider", providerName, "enabled", true)
	return nil
}

// registerConnectedProvider registers a newly connected provider into the session.
func registerConnectedProvider(m *Model, info ProviderInfo, apiKey string, models []string) error {
	session := m.session
	if session == nil {
		return fmt.Errorf("no active session")
	}

	// Create provider instance based on type
	var prov provider.Provider
	switch info.Name {
	case "ollama":
		prov = provider.NewOllamaProvider(info.BaseURL)
	case "opencode-zen", "opencode-go", "openai":
		prov = provider.NewOpenAICompatProvider(apiKey, provider.OpenAICompatConfig{
			BaseURL:      info.BaseURL,
			ProviderName: info.Name,
		})
	case "anthropic":
		prov = provider.NewAnthropicProvider(apiKey)
	case "google":
		prov = provider.NewGoogleProvider(apiKey)
	case "openrouter":
		prov = provider.NewOpenRouterProvider(apiKey)
	default:
		return fmt.Errorf("unsupported provider: %s", info.Name)
	}

	// Register provider and models into session
	if err := session.RegisterProvider(prov, info.Name, info.BaseURL, models); err != nil {
		return fmt.Errorf("register provider: %w", err)
	}

	slog.Info("provider registered at runtime", "provider", info.Name, "models", len(models))
	return nil
}

// buildModelEntry creates a types.Model entry for a discovered model.
func buildModelEntry(modelID, providerName, baseURL string) types.Model {
	api := "openai-completions"
	switch providerName {
	case "anthropic":
		api = "anthropic-messages"
	case "google":
		api = "google-generative-ai"
	case "ollama":
		api = "ollama-chat"
	}

	return types.Model{
		ID:       modelID,
		Name:     modelID,
		Provider: providerName,
		API:      api,
		BaseURL:  baseURL,
	}
}

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

	authMethodOpts := []palette.ListOption{
		{Title: "Browser (recommended)", Description: "Open browser to authenticate with ChatGPT", Value: "browser"},
		{Title: "Device code (headless)", Description: "Get a code to enter on another device", Value: "device"},
		{Title: "Manual paste", Description: "Open URL manually and paste the redirect URL", Value: "manual"},
	}

	return []palette.Step{
		palette.ListStep("Select Provider", "Choose a provider to connect to:", opts),
		palette.ConditionalListStep(
			"Auth Method",
			"Choose how to authenticate:",
			authMethodOpts,
			func(results map[string]any) bool {
				providerName, _ := results["select_provider"].(string)
				return providerName != "openai-oauth"
			},
			"",
		),
		palette.ConditionalTaskStep(
			"Generate Authorization URL",
			func(results map[string]any) (bool, string, error) {
				verifier, challenge, err := provider.GeneratePKCE()
				if err != nil {
					return false, "", fmt.Errorf("generating PKCE: %w", err)
				}
				results["pkce_verifier"] = verifier
				results["pkce_challenge"] = challenge
				return true, "", nil
			},
			func(results map[string]any) bool {
				providerName, _ := results["select_provider"].(string)
				authMethod, _ := results["auth_method"].(string)
				return !(providerName == "openai-oauth" && authMethod == "manual")
			},
			"",
		),
		palette.ConditionalInputStep(
			"Paste Authorization URL",
			"Open the URL below in your browser, then paste the full redirect URL or authorization code:\n\n(Complete authentication in your browser first)",
			"https://auth.openai.com/oauth/authorize?... or raw code",
			func(results map[string]any) bool {
				providerName, _ := results["select_provider"].(string)
				authMethod, _ := results["auth_method"].(string)
				return !(providerName == "openai-oauth" && authMethod == "manual")
			},
			"",
		),
		palette.ConditionalTaskStep(
			"Exchange Code for Tokens",
			func(results map[string]any) (bool, string, error) {
				pasteInput, _ := results["paste_authorization_url"].(string)
				verifier, _ := results["pkce_verifier"].(string)

				code, _, err := provider.ParseCallbackInput(pasteInput)
				if err != nil {
					return false, fmt.Sprintf("Invalid input: %v", err), err
				}

				access, refresh, expires, err := provider.ExchangeCodeForTokens(code, verifier)
				if err != nil {
					return false, fmt.Sprintf("Token exchange failed: %v", err), err
				}

				accountID, err := provider.ExtractAccountID(access)
				if err != nil {
					return false, fmt.Sprintf("Failed to extract account ID: %v", err), err
				}

				creds := provider.OAuthCredentials{
					AccessToken:  access,
					RefreshToken: refresh,
					Expires:      expires,
					AccountID:    accountID,
				}
				results["oauth_credentials"] = creds

				return true, fmt.Sprintf("Authenticated as account %s", accountID), nil
			},
			func(results map[string]any) bool {
				providerName, _ := results["select_provider"].(string)
				authMethod, _ := results["auth_method"].(string)
				return !(providerName == "openai-oauth" && authMethod == "manual")
			},
			"",
		),
		palette.ConditionalTaskStep(
			"Browser Authentication",
			func(results map[string]any) (bool, string, error) {
				creds, err := provider.BrowserFlow()
				if err != nil {
					return false, fmt.Sprintf("Browser authentication failed: %v", err), err
				}
				results["oauth_credentials"] = creds
				return true, fmt.Sprintf("Authenticated as account %s", creds.AccountID), nil
			},
			func(results map[string]any) bool {
				providerName, _ := results["select_provider"].(string)
				authMethod, _ := results["auth_method"].(string)
				return !(providerName == "openai-oauth" && authMethod == "browser")
			},
			"",
		),
		palette.ConditionalTaskStep(
			"Device Authorization",
			func(results map[string]any) (bool, string, error) {
				creds, err := provider.DeviceFlow()
				if err != nil {
					return false, fmt.Sprintf("Device authorization failed: %v", err), err
				}
				results["oauth_credentials"] = creds
				return true, fmt.Sprintf("Authenticated as account %s", creds.AccountID), nil
			},
			func(results map[string]any) bool {
				providerName, _ := results["select_provider"].(string)
				authMethod, _ := results["auth_method"].(string)
				return !(providerName == "openai-oauth" && authMethod == "device")
			},
			"",
		),
		palette.ConditionalInputStep(
			"API Key",
			"Enter your API key (leave empty to use saved credentials):",
			"sk-...",
			func(results map[string]any) bool {
				providerName, _ := results["select_provider"].(string)
				return providerName == "openai-oauth"
			},
			"",
		),
		palette.ConditionalTaskStep("Test Connection", func(results map[string]any) (bool, string, error) {
			providerName, _ := results["select_provider"].(string)
			apiKey, _ := results["api_key"].(string)

			info, ok := findProvider(providerName)
			if !ok {
				return false, "", fmt.Errorf("unknown provider: %s", providerName)
			}

			if apiKey == "" {
				providerState := getProviderState(providerName)
				apiKey = providerState.APIKey
			}

			if info.RequiresAPIKey && apiKey == "" {
				return false, "API key is required but not provided", fmt.Errorf("missing API key for %s", info.DisplayName)
			}

			if err := testProviderConnection(info, apiKey); err != nil {
				return false, fmt.Sprintf("Connection to %s failed: %v", info.DisplayName, err), err
			}
			return true, fmt.Sprintf("Connected to %s successfully", info.DisplayName), nil
		}, func(results map[string]any) bool {
			providerName, _ := results["select_provider"].(string)
			return providerName == "openai-oauth"
		}, ""),
		palette.TaskStep("Discover Models", func(results map[string]any) (bool, string, error) {
			providerName, _ := results["select_provider"].(string)
			apiKey, _ := results["api_key"].(string)

			info, ok := findProvider(providerName)
			if !ok {
				return false, "", fmt.Errorf("unknown provider: %s", providerName)
			}

			var models []string
			var err error

			if providerName == "openai-oauth" {
				models, err = discoverOpenAIOAuthModels("")
			} else {
				if apiKey == "" {
					providerState := getProviderState(providerName)
					apiKey = providerState.APIKey
				}
				models, err = discoverProviderModels(info, apiKey)
			}

			slog.Debug("discover models result", "provider", info.Name, "models", models, "err", err)
			if err != nil {
				return false, fmt.Sprintf("Model discovery failed: %v", err), err
			}
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

	if providerName == "openai-oauth" {
		handleOAuthConnectResult(m, info, results)
		return
	}

	// API key provider flow
	state := getProviderState(providerName)
	usingExistingKey := apiKey == "" && state.HasAuth
	if usingExistingKey {
		apiKey = state.APIKey
	}

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

	if err := saveProviderConfig(providerName, info.BaseURL); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Failed to update config: %v", err),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

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

// handleOAuthConnectResult processes OAuth connection results.
func handleOAuthConnectResult(m *Model, info ProviderInfo, results map[string]any) {
	authMethod, _ := results["auth_method"].(string)

	authMethodLabel := "OAuth"
	switch authMethod {
	case "browser":
		authMethodLabel = "Browser"
	case "device":
		authMethodLabel = "Device code"
	case "manual":
		authMethodLabel = "Manual"
	}

	if err := saveProviderOAuthAuth("openai-oauth", results); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Failed to save OAuth credentials: %v", err),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

	if err := saveProviderConfig("openai-oauth", info.BaseURL); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Failed to update config: %v", err),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

	modelsRaw, _ := results["discover_models"]
	models, _ := modelsRaw.([]string)

	persistFunc := func(creds provider.OAuthCredentials) error {
		return saveProviderOAuthAuth("openai-oauth", map[string]any{
			"oauth_credentials": creds,
		})
	}

	if err := registerOAuthProvider(m, info, persistFunc); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Provider registered but provider registration failed: %v", err),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("✓ Successfully connected to **%s** via %s authentication", info.DisplayName, authMethodLabel))
	lines = append(lines, "")
	lines = append(lines, "- OAuth credentials saved to `~/.tau/auth.json`")
	lines = append(lines, "- Provider enabled in `~/.tau/config.json`")
	lines = append(lines, "- Tokens will be automatically refreshed when expired")

	if len(models) > 0 {
		lines = append(lines, fmt.Sprintf("- %d Codex model(s) available:", len(models)))
		for _, model := range models {
			lines = append(lines, fmt.Sprintf("  - `%s`", model))
		}
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

	store[providerName] = config.AuthValue{Value: apiKey}

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

// saveProviderOAuthAuth saves OAuth credentials to auth.json.
func saveProviderOAuthAuth(providerName string, results map[string]any) error {
	creds, ok := results["oauth_credentials"].(provider.OAuthCredentials)
	if !ok {
		return fmt.Errorf("OAuth credentials not found in results")
	}

	authPath := config.AuthPath("")
	store, err := config.LoadAuth(authPath)
	if err != nil {
		return fmt.Errorf("load existing auth: %w", err)
	}

	store[providerName] = config.AuthValue{
		Type:      "oauth",
		Access:    creds.AccessToken,
		Refresh:   creds.RefreshToken,
		Expires:   creds.Expires,
		AccountID: creds.AccountID,
	}

	if err := config.SaveAuth(store, authPath); err != nil {
		return fmt.Errorf("save auth: %w", err)
	}

	slog.Info("OAuth credentials saved", "provider", providerName, "account_id", creds.AccountID)
	return nil
}

// registerOAuthProvider registers an OAuth provider into the session.
func registerOAuthProvider(m *Model, info ProviderInfo, persistFunc provider.PersistFunc) error {
	session := m.session
	if session == nil {
		return fmt.Errorf("no active session")
	}

	authPath := config.AuthPath("")
	store, err := config.LoadAuth(authPath)
	if err != nil {
		return fmt.Errorf("load auth for OAuth provider: %w", err)
	}

	authVal, exists := store[info.Name]
	if !exists || !authVal.IsOAuth() {
		return fmt.Errorf("OAuth credentials not found for %s", info.Name)
	}

	creds := provider.OAuthCredentials{
		AccessToken:  authVal.Access,
		RefreshToken: authVal.Refresh,
		Expires:      authVal.Expires,
		AccountID:    authVal.AccountID,
	}

	persist := func(creds provider.OAuthCredentials) error {
		store, err := config.LoadAuth(authPath)
		if err != nil {
			return fmt.Errorf("load auth for persistence: %w", err)
		}
		store[info.Name] = config.AuthValue{
			Type:      "oauth",
			Access:    creds.AccessToken,
			Refresh:   creds.RefreshToken,
			Expires:   creds.Expires,
			AccountID: creds.AccountID,
		}
		return config.SaveAuth(store, authPath)
	}

	prov := provider.NewOpenAIOAuthProviderWithPersist(creds, persist)

	models, err := discoverOpenAIOAuthModels("")
	if err != nil {
		return fmt.Errorf("discover OAuth models: %w", err)
	}

	if err := session.RegisterProvider(prov, info.Name, info.BaseURL, models); err != nil {
		return fmt.Errorf("register provider: %w", err)
	}

	slog.Info("OAuth provider registered at runtime", "provider", info.Name, "models", len(models))
	return nil
}

// registerConnectedProvider registers a newly connected provider into the session.
func registerConnectedProvider(m *Model, info ProviderInfo, apiKey string, models []string) error {
	session := m.session
	if session == nil {
		return fmt.Errorf("no active session")
	}

	var prov provider.Provider
	switch info.Name {
	case "ollama":
		prov = provider.NewOllamaProvider(info.BaseURL)
	case "opencode-zen", "opencode-go", "openai":
		prov = provider.NewOpenAICompatProvider(apiKey, provider.OpenAICompatConfig{
			BaseURL:      info.BaseURL,
			ProviderName: info.Name,
		})
	case "openai-oauth":
		authPath := config.AuthPath("")
		store, err := config.LoadAuth(authPath)
		if err != nil {
			return fmt.Errorf("load auth for OAuth provider: %w", err)
		}
		authVal, exists := store[info.Name]
		if !exists || !authVal.IsOAuth() {
			return fmt.Errorf("OAuth credentials not found for %s", info.Name)
		}
		creds := provider.OAuthCredentials{
			AccessToken:  authVal.Access,
			RefreshToken: authVal.Refresh,
			Expires:      authVal.Expires,
			AccountID:    authVal.AccountID,
		}
		persist := func(creds provider.OAuthCredentials) error {
			store, err := config.LoadAuth(authPath)
			if err != nil {
				return fmt.Errorf("load auth for persistence: %w", err)
			}
			store[info.Name] = config.AuthValue{
				Type:      "oauth",
				Access:    creds.AccessToken,
				Refresh:   creds.RefreshToken,
				Expires:   creds.Expires,
				AccountID: creds.AccountID,
			}
			return config.SaveAuth(store, authPath)
		}
		prov = provider.NewOpenAIOAuthProviderWithPersist(creds, persist)
	case "anthropic":
		prov = provider.NewAnthropicProvider(apiKey)
	case "google":
		prov = provider.NewGoogleProvider(apiKey)
	case "openrouter":
		prov = provider.NewOpenRouterProvider(apiKey)
	default:
		return fmt.Errorf("unsupported provider: %s", info.Name)
	}

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

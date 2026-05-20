package tui

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/tui/palette"
)

// disconnectSteps returns the multi-step flow for the /disconnect command.
func disconnectSteps(m *Model) []palette.Step {
	providers := listConnectedProviders(m)
	if len(providers) == 0 {
		return nil
	}

	opts := make([]palette.ListOption, len(providers))
	for i, p := range providers {
		status := "connected"
		if !p.Enabled {
			status = "disabled"
		}
		opts[i] = palette.ListOption{
			Title:       p.DisplayName,
			Description: fmt.Sprintf("%s • %s", p.Description, status),
			Value:       p.Name,
		}
	}

	return []palette.Step{
		palette.ListStep("Select Provider", "Choose a provider to disconnect:", opts),
		palette.ConfirmStep("Confirm Disconnect", "Disable this provider? (credentials will be preserved)"),
	}
}

// handleDisconnectResult processes the collected results and performs the disconnection.
func handleDisconnectResult(m *Model, results map[string]any) {
	providerName, _ := results["select_provider"].(string)
	disconnectConfirmed, _ := results["confirm_disconnect"].(bool)

	if !disconnectConfirmed {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "Disconnect cancelled.",
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

	// Disable provider in session
	session := m.session
	if session == nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: "No active session",
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

	// Check if disconnecting active model's provider
	currentModel := session.Model()
	activeModelWarning := ""
	if currentModel.Provider == providerName {
		activeModelWarning = fmt.Sprintf("\n⚠️  **Warning**: Your current model `%s` uses this provider. You will need to switch to another model after disconnecting.", currentModel.ID)
	}

	if err := session.DisableProvider(providerName); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Failed to disable provider: %v", err),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return
	}

	// Build success message
	var lines []string
	lines = append(lines, fmt.Sprintf("✓ **%s** has been disabled", info.DisplayName))
	lines = append(lines, "")
	lines = append(lines, "- Provider removed from registry")
	lines = append(lines, "- Models hidden from `/model` selector")
	lines = append(lines, "- Credentials preserved in `~/.tau/auth.json`")
	if activeModelWarning != "" {
		lines = append(lines, activeModelWarning)
	}
	lines = append(lines, "")
	lines = append(lines, "Use `/connect` to re-enable this provider without re-entering credentials.")

	m.blocks = append(m.blocks, messageBlock{
		kind: blockAssistantText,
		text: strings.Join(lines, "\n"),
	})
	m.invalidateRenderedCache()
	m.updateViewport()
}

// connectedProvider extends ProviderInfo with enabled state from config.
type connectedProvider struct {
	ProviderInfo
	Enabled bool
}

// listConnectedProviders returns providers that have been explicitly connected via /connect.
// A provider is considered "connected" if it has credentials in auth.json AND a config entry.
// This excludes providers auto-registered at startup from environment variables.
func listConnectedProviders(m *Model) []connectedProvider {
	cfg, err := config.LoadConfig("")
	if err != nil {
		slog.Debug("failed to load config for disconnect list", "error", err)
		return nil
	}

	authPath := config.AuthPath("")
	store, err := config.LoadAuth(authPath)
	if err != nil {
		slog.Debug("failed to load auth for disconnect list", "error", err)
	}

	var result []connectedProvider
	for _, info := range providerCatalog {
		_, hasAuth := store[info.Name]
		providerCfg, hasConfig := cfg.Providers[info.Name]

		// Provider is considered connected only if it has both auth AND config
		// (meaning it was explicitly connected via /connect, not auto-registered)
		if hasAuth && hasConfig {
			enabled := true
			if providerCfg.Enabled != nil {
				enabled = *providerCfg.Enabled
			}
			result = append(result, connectedProvider{
				ProviderInfo: info,
				Enabled:      enabled,
			})
		}
	}

	return result
}

// Package config provides configuration loading and path resolution for Tau.
//
// It loads settings from ~/.tau/config.json with sensible defaults,
// parses auth.json for API credentials, and computes file search paths
// for context files (AGENTS.md/CLAUDE.md).
//
// This package has no internal dependencies on other internal/ packages.
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var logger = slog.Default().With(slog.String("pkg", "config"))

// Constants for Tau directory structure.
const (
	TauDirName = ".tau"
	SkillsDirName = "skills"
	AgentsDirName = ".agents"
	ConfigFileName = "config.json"
	AuthFileName  = "auth.json"
	SessionsDirName = "sessions"
)

// Config holds Tau configuration settings.
type Config struct {
	Providers        map[string]ProviderConfig `json:"providers,omitempty"`
	DefaultModel     string                    `json:"default_model,omitempty"`
	Compaction       CompactionConfig          `json:"compaction,omitempty"`
	SubagentTimeout  time.Duration             `json:"subagent_timeout,omitempty"`
	ToolAllowlist    []string                  `json:"tool_allowlist,omitempty"`
	ReadOnly         bool                      `json:"read_only,omitempty"`
	LoadContextFiles bool                      `json:"load_context_files,omitempty"`
	Search           SearchConfig              `json:"search,omitempty"`
}

// ProviderConfig holds provider-specific configuration.
type ProviderConfig struct {
	Model   string   `json:"model,omitempty"`
	Enabled *bool    `json:"enabled,omitempty"`
	BaseURL string   `json:"base_url,omitempty"`
	Models  []string `json:"models,omitempty"`
}

// CompactionConfig holds compaction trigger settings.
type CompactionConfig struct {
	ReserveTokens  int `json:"reserve_tokens,omitempty"`
	KeepRecentTokens int `json:"keep_recent_tokens,omitempty"`
}

// SearchConfig holds search backend configuration.
type SearchConfig struct {
	Backend   string `json:"backend,omitempty"`
	SearXNGURL string `json:"searxng_url,omitempty"`
}

// AuthStore holds API keys for providers.
// Values can be literal keys, environment variable references ("$VAR"),
// or shell commands ("!command").
type AuthStore map[string]string

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Providers:        make(map[string]ProviderConfig),
		DefaultModel:     "",
		Compaction: CompactionConfig{
			ReserveTokens:  16384,
			KeepRecentTokens: 20000,
		},
		SubagentTimeout:  5 * time.Minute,
		ToolAllowlist:    nil,
		ReadOnly:         false,
		LoadContextFiles: true,
	}
}

// LoadConfig loads configuration from the specified path.
// If path is empty, it resolves to ~/.tau/config.json.
// If the file does not exist, returns DefaultConfig with nil error.
// If the file exists but contains invalid JSON, returns an error.
// Partial configs are merged with defaults (JSON unmarshal overlays on defaults).
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Warn("could not determine home directory, using defaults", "error", err)
			cfg := DefaultConfig()
			return &cfg, nil
		}
		path = filepath.Join(home, TauDirName, ConfigFileName)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			return &cfg, nil
		}
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	// Ensure non-nil maps
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}

	logger.Info("config loaded", "path", path)
	return &cfg, nil
}

// LoadAuth loads API keys from auth.json at the given path.
// If path is empty, resolves to ~/.tau/auth.json.
// Returns an empty AuthStore if the file does not exist.
// Warns if file permissions are too open (should be 0600).
func LoadAuth(path string) (AuthStore, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Warn("could not determine home directory, returning empty auth", "error", err)
			return make(AuthStore), nil
		}
		path = filepath.Join(home, TauDirName, AuthFileName)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(AuthStore), nil
		}
		return nil, fmt.Errorf("reading auth file %s: %w", path, err)
	}

	// Check permissions
	info, err := os.Stat(path)
	if err == nil {
		perm := info.Mode().Perm()
		if perm&0077 != 0 {
			logger.Warn("auth.json has overly permissive file permissions (should be 0600)",
				"path", path, "permissions", perm.String())
		}
	}

	var store AuthStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parsing auth file %s: %w", path, err)
	}

	if store == nil {
		store = make(AuthStore)
	}

	return store, nil
}

// ResolveAuthKey resolves a single auth key value by interpreting its format:
// - Literal: "sk-actual-key-value" → returned as-is
// - Environment variable: "$MY_VAR" → value of env var MY_VAR
// - Shell command: "!command" → stdout of command execution
// If an env var or command fails, returns the original value.
func ResolveAuthKey(value string) string {
	if strings.HasPrefix(value, "$") {
		envName := strings.TrimPrefix(value, "$")
		if envVal, ok := os.LookupEnv(envName); ok {
			return envVal
		}
		logger.Warn("environment variable not found, returning raw auth value", "env", envName)
		return value
	}

	if strings.HasPrefix(value, "!") {
		cmd := strings.TrimPrefix(value, "!")
		output, err := execShellCommand(cmd)
		if err != nil {
			logger.Warn("shell command failed, returning raw auth value", "command", cmd, "error", err)
			return value
		}
		return strings.TrimSpace(output)
	}

	return value
}

// ConfigPath returns the path to config.json.
// If path is empty, resolves to ~/.tau/config.json.
func ConfigPath(path string) string {
	if path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, TauDirName, ConfigFileName)
}

// AuthPath returns the path to auth.json.
// If path is empty, resolves to ~/.tau/auth.json.
func AuthPath(path string) string {
	if path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, TauDirName, AuthFileName)
}

// SaveAuth writes API keys to auth.json at the given path.
// If path is empty, resolves to ~/.tau/auth.json.
// Creates the .tau directory if it doesn't exist.
// File permissions are set to 0600.
func SaveAuth(store AuthStore, path string) error {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not determine home directory: %w", err)
		}
		path = filepath.Join(home, TauDirName, AuthFileName)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling auth data: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing auth file %s: %w", path, err)
	}

	logger.Info("auth saved", "path", path)
	return nil
}

// SaveConfig writes configuration to config.json at the given path.
// If path is empty, resolves to ~/.tau/config.json.
// Creates the .tau directory if it doesn't exist.
func SaveConfig(cfg *Config, path string) error {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not determine home directory: %w", err)
		}
		path = filepath.Join(home, TauDirName, ConfigFileName)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config data: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file %s: %w", path, err)
	}

	logger.Info("config saved", "path", path)
	return nil
}

// execShellCommand executes a shell command and returns trimmed stdout.
func execShellCommand(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).Output()
	return string(out), err
}

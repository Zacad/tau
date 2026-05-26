package provider

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/adam/tau/internal/config"
)

// KeyFormat describes the format of an API key value.
type KeyFormat int

const (
	// KeyFormatLiteral is a plain API key string.
	KeyFormatLiteral KeyFormat = iota
	// KeyFormatEnvRef references an environment variable ($VAR_NAME).
	KeyFormatEnvRef
	// KeyFormatShellCmd runs a shell command to retrieve the key (!command).
	KeyFormatShellCmd
)

// ResolveKey resolves an API key using the 4-step chain:
// 1. CLI flag (--api-key)
// 2. auth.json file
// 3. Environment variable (PROVIDER_API_KEY)
// 4. Config file (future, currently returns empty)
//
// The raw key value may be in one of three formats:
// - Literal: used as-is
// - $ENV_VAR: resolved from environment
// - !command: executed as shell command, stdout used as key
func ResolveKey(providerName string, cliKey string) (string, error) {
	// Step 1: CLI flag (highest priority)
	if cliKey != "" {
		return resolveKeyFormat(cliKey)
	}

	// Step 2: auth.json file
	authPath := authJSONPath()
	if authPath != "" {
		key, err := readAuthKey(authPath, providerName)
		if err == nil && key != "" {
			return resolveKeyFormat(key)
		}
		// If file doesn't exist or key not found, continue to next step
	}

	// Step 3: Environment variable
	envKey := resolveEnvKey(providerName)
	if envKey != "" {
		return envKey, nil
	}

	// Step 4: Config file (not yet implemented — returns empty)
	// TODO: implement config file key resolution

	return "", &ResolveError{
		Provider:     providerName,
		EnvVarName:   standardEnvVar(providerName),
		AuthFilePath: authPath,
	}
}

// resolveKeyFormat interprets a raw key value based on its format prefix.
func resolveKeyFormat(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("empty key value")
	}

	// Shell command: !command
	if strings.HasPrefix(raw, "!") {
		cmd := strings.TrimPrefix(raw, "!")
		return execShellCommand(cmd)
	}

	// Environment variable: $VAR_NAME
	if strings.HasPrefix(raw, "$") {
		envName := strings.TrimPrefix(raw, "$")
		val := os.Getenv(envName)
		if val == "" {
			return "", fmt.Errorf("environment variable %q is not set", envName)
		}
		return val, nil
	}

	// Literal key
	return raw, nil
}

// readAuthKey reads a specific provider key from auth.json.
func readAuthKey(path string, providerName string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var keys map[string]config.AuthValue
	if err := json.Unmarshal(data, &keys); err != nil {
		return "", err
	}

	authVal, ok := keys[providerName]
	if !ok {
		return "", fmt.Errorf("provider %q not found in auth.json", providerName)
	}

	return authVal.APIKey(), nil
}

// resolveEnvKey looks up the standard environment variable for a provider.
func resolveEnvKey(providerName string) string {
	return os.Getenv(standardEnvVar(providerName))
}

// standardEnvVar returns the standard environment variable name for a provider.
func standardEnvVar(providerName string) string {
	return strings.ToUpper(providerName) + "_API_KEY"
}

// execShellCommand runs a shell command and trims the output.
func execShellCommand(cmd string) (string, error) {
	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		return "", fmt.Errorf("shell command %q failed: %w", cmd, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// authJSONPath returns the path to the auth.json file.
// Returns empty string if the file doesn't exist.
func authJSONPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".tau", "auth.json")
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// ResolveError is returned when no API key could be found.
type ResolveError struct {
	Provider     string
	EnvVarName   string
	AuthFilePath string
}

func (e *ResolveError) Error() string {
	msg := fmt.Sprintf("no API key found for provider %q. Set it via: ", e.Provider)
	msg += fmt.Sprintf("--api-key flag, auth.json (%s), or %s env var", e.AuthFilePath, e.EnvVarName)
	return msg
}

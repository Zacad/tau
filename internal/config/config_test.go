package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/testutil"
)

func TestLoadConfig_DefaultWhenMissing(t *testing.T) {
	testutil.SetHomeEnv(t, testutil.TempDir(t))

	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultModel != "" {
		t.Errorf("DefaultModel: got %q, want empty", cfg.DefaultModel)
	}
	if cfg.Compaction.ReserveTokens != 16384 {
		t.Errorf("ReserveTokens: got %d, want 16384", cfg.Compaction.ReserveTokens)
	}
	if cfg.SubagentTimeout == 0 {
		t.Error("SubagentTimeout should have default value")
	}
	if !cfg.LoadContextFiles {
		t.Error("LoadContextFiles should default to true")
	}
}

func TestLoadConfig_ValidJSON(t *testing.T) {
	home := testutil.SetupTauHome(t, `{
		"default_model": "claude-sonnet-4-20250514",
		"compaction": {"reserve_tokens": 8192},
		"providers": {"anthropic": {"model": "claude-sonnet-4"}}
	}`, "")
	testutil.SetHomeEnv(t, home)

	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultModel != "claude-sonnet-4-20250514" {
		t.Errorf("DefaultModel: got %q, want %q", cfg.DefaultModel, "claude-sonnet-4-20250514")
	}
	if cfg.Compaction.ReserveTokens != 8192 {
		t.Errorf("ReserveTokens: got %d, want 8192", cfg.Compaction.ReserveTokens)
	}
	if cfg.Compaction.KeepRecentTokens != 20000 {
		t.Errorf("KeepRecentTokens: got %d, want 20000 (should use default)", cfg.Compaction.KeepRecentTokens)
	}
	if _, ok := cfg.Providers["anthropic"]; !ok {
		t.Error("providers should contain anthropic")
	}
}

func TestLoadConfig_EmptyJSON(t *testing.T) {
	home := testutil.SetupTauHome(t, `{}`, "")
	testutil.SetHomeEnv(t, home)

	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Compaction.ReserveTokens != 16384 {
		t.Errorf("ReserveTokens should use default: got %d", cfg.Compaction.ReserveTokens)
	}
}

func TestLoadConfig_MalformedJSON(t *testing.T) {
	home := testutil.SetupTauHome(t, `{not valid json}`, "")
	testutil.SetHomeEnv(t, home)

	_, err := config.LoadConfig("")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestLoadConfig_ExplicitPath(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "custom.json")
	if err := os.WriteFile(path, []byte(`{"default_model": "gpt-4o"}`), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DefaultModel != "gpt-4o" {
		t.Errorf("DefaultModel: got %q, want %q", cfg.DefaultModel, "gpt-4o")
	}
}

func TestLoadAuth_DefaultWhenMissing(t *testing.T) {
	home := testutil.TempDir(t)
	testutil.SetHomeEnv(t, home)

	store, err := config.LoadAuth("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store) != 0 {
		t.Errorf("expected empty auth store, got %d entries", len(store))
	}
}

func TestLoadAuth_ValidJSON(t *testing.T) {
	authJSON := `{
		"openai": "sk-test-key-123",
		"anthropic": "$ANTHROPIC_API_KEY",
		"google": "!echo secret"
	}`
	home := testutil.SetupTauHome(t, "", authJSON)
	testutil.SetHomeEnv(t, home)

	store, err := config.LoadAuth("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store) != 3 {
		t.Errorf("expected 3 entries, got %d", len(store))
	}
}

func TestLoadAuth_MalformedJSON(t *testing.T) {
	home := testutil.SetupTauHome(t, "", `{bad json`)
	testutil.SetHomeEnv(t, home)

	_, err := config.LoadAuth("")
	if err == nil {
		t.Fatal("expected error for malformed auth JSON")
	}
}

func TestResolveAuthKey_Literal(t *testing.T) {
	got := config.ResolveAuthKey("sk-actual-key-value")
	if got != "sk-actual-key-value" {
		t.Errorf("got %q, want %q", got, "sk-actual-key-value")
	}
}

func TestResolveAuthKey_EnvVar(t *testing.T) {
	testKey := "test-env-key-123"
	t.Setenv("TEST_AUTH_KEY", testKey)

	got := config.ResolveAuthKey("$TEST_AUTH_KEY")
	if got != testKey {
		t.Errorf("got %q, want %q", got, testKey)
	}
}

func TestResolveAuthKey_EnvVarMissing(t *testing.T) {
	// Non-existent env should return raw value
	got := config.ResolveAuthKey("$NONEXISTENT_ENV_VAR_12345")
	if got != "$NONEXISTENT_ENV_VAR_12345" {
		t.Errorf("expected raw value, got %q", got)
	}
}

func TestResolveAuthKey_ShellCommand(t *testing.T) {
	got := config.ResolveAuthKey("!echo shell-key")
	if got != "shell-key" {
		t.Errorf("got %q, want %q", got, "shell-key")
	}
}

func TestResolveAuthKey_ShellCommandFailure(t *testing.T) {
	// Command that fails should return raw value
	got := config.ResolveAuthKey("!exit 1")
	if got != "!exit 1" {
		t.Errorf("expected raw value on failure, got %q", got)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.Providers == nil {
		t.Error("Providers should be initialized to non-nil map")
	}
	if cfg.Compaction.ReserveTokens != 16384 {
		t.Errorf("ReserveTokens: got %d, want 16384", cfg.Compaction.ReserveTokens)
	}
	if cfg.Compaction.KeepRecentTokens != 20000 {
		t.Errorf("KeepRecentTokens: got %d, want 20000", cfg.Compaction.KeepRecentTokens)
	}
}

func TestSaveAuth_CreatesFile(t *testing.T) {
	dir := testutil.TempDir(t)
	home := filepath.Join(dir, "home")
	tauDir := filepath.Join(home, ".tau")

	testutil.SetHomeEnv(t, home)

	store := config.AuthStore{
		"openai":       "sk-test-key",
		"opencode-zen": "zen-key-123",
	}

	err := config.SaveAuth(store, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was created
	authPath := filepath.Join(tauDir, "auth.json")
	if _, err := os.Stat(authPath); err != nil {
		t.Fatalf("auth.json should exist: %v", err)
	}

	// Verify file permissions
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatalf("stat auth.json: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}

	// Verify content can be read back
	loaded, err := config.LoadAuth(authPath)
	if err != nil {
		t.Fatalf("load saved auth: %v", err)
	}
	if loaded["openai"] != "sk-test-key" {
		t.Errorf("openai: got %q, want %q", loaded["openai"], "sk-test-key")
	}
	if loaded["opencode-zen"] != "zen-key-123" {
		t.Errorf("opencode-zen: got %q, want %q", loaded["opencode-zen"], "zen-key-123")
	}
}

func TestSaveAuth_ExplicitPath(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "custom-auth.json")

	store := config.AuthStore{"test-provider": "test-key"}
	err := config.SaveAuth(store, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.LoadAuth(path)
	if err != nil {
		t.Fatalf("load saved auth: %v", err)
	}
	if loaded["test-provider"] != "test-key" {
		t.Errorf("test-provider: got %q, want %q", loaded["test-provider"], "test-key")
	}
}

func TestSaveConfig_CreatesFile(t *testing.T) {
	dir := testutil.TempDir(t)
	home := filepath.Join(dir, "home")

	testutil.SetHomeEnv(t, home)

	cfg := config.DefaultConfig()
	cfg.DefaultModel = "gpt-4o"
	enabled := true
	cfg.Providers["opencode-zen"] = config.ProviderConfig{
		Enabled: &enabled,
		BaseURL: "https://opencode.ai/zen/v1",
	}

	err := config.SaveConfig(&cfg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify file was created and can be read back
	loaded, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if loaded.DefaultModel != "gpt-4o" {
		t.Errorf("DefaultModel: got %q, want %q", loaded.DefaultModel, "gpt-4o")
	}
	pc, ok := loaded.Providers["opencode-zen"]
	if !ok {
		t.Fatal("providers should contain opencode-zen")
	}
	if pc.Enabled == nil || !*pc.Enabled {
		t.Error("opencode-zen should be enabled")
	}
	if pc.BaseURL != "https://opencode.ai/zen/v1" {
		t.Errorf("BaseURL: got %q, want %q", pc.BaseURL, "https://opencode.ai/zen/v1")
	}
}

func TestSaveConfig_ExplicitPath(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "custom-config.json")

	cfg := config.DefaultConfig()
	cfg.DefaultModel = "claude-sonnet-4-20250514"

	err := config.SaveConfig(&cfg, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.LoadConfig(path)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if loaded.DefaultModel != "claude-sonnet-4-20250514" {
		t.Errorf("DefaultModel: got %q, want %q", loaded.DefaultModel, "claude-sonnet-4-20250514")
	}
}

func TestAuthPath(t *testing.T) {
	home := testutil.TempDir(t)
	testutil.SetHomeEnv(t, home)

	got := config.AuthPath("")
	want := filepath.Join(home, ".tau", "auth.json")
	if got != want {
		t.Errorf("AuthPath(\"\") = %q, want %q", got, want)
	}

	// Explicit path should be returned as-is
	explicit := "/custom/path/auth.json"
	if config.AuthPath(explicit) != explicit {
		t.Errorf("AuthPath(%q) = %q, want %q", explicit, config.AuthPath(explicit), explicit)
	}
}

func TestProviderConfig_ModelsField(t *testing.T) {
	home := testutil.SetupTauHome(t, `{
		"providers": {
			"openrouter": {
				"enabled": true,
				"models": ["anthropic/claude-sonnet-4", "openai/o3", "minimax/minimax-m2"]
			}
		}
	}`, "")
	testutil.SetHomeEnv(t, home)

	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pc, ok := cfg.Providers["openrouter"]
	if !ok {
		t.Fatal("providers should contain openrouter")
	}
	if len(pc.Models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(pc.Models))
	}
	if pc.Models[0] != "anthropic/claude-sonnet-4" {
		t.Errorf("Models[0]: got %q, want %q", pc.Models[0], "anthropic/claude-sonnet-4")
	}
}

func TestProviderConfig_ModelsFieldRoundTrip(t *testing.T) {
	dir := testutil.TempDir(t)
	home := filepath.Join(dir, "home")
	testutil.SetHomeEnv(t, home)

	cfg := config.DefaultConfig()
	enabled := true
	cfg.Providers["openrouter"] = config.ProviderConfig{
		Enabled: &enabled,
		Models:  []string{"custom/model-1", "custom/model-2"},
	}

	err := config.SaveConfig(&cfg, "")
	if err != nil {
		t.Fatalf("save config: %v", err)
	}

	loaded, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	pc, ok := loaded.Providers["openrouter"]
	if !ok {
		t.Fatal("providers should contain openrouter")
	}
	if len(pc.Models) != 2 {
		t.Fatalf("expected 2 models after round-trip, got %d", len(pc.Models))
	}
	if pc.Models[0] != "custom/model-1" {
		t.Errorf("Models[0]: got %q, want %q", pc.Models[0], "custom/model-1")
	}
}

func TestProviderConfig_ModelsFieldOptional(t *testing.T) {
	// Config without models field should load without error
	home := testutil.SetupTauHome(t, `{
		"providers": {
			"openrouter": {
				"enabled": true
			}
		}
	}`, "")
	testutil.SetHomeEnv(t, home)

	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pc := cfg.Providers["openrouter"]
	if pc.Models != nil {
		t.Errorf("expected nil Models when not in config, got %v", pc.Models)
	}
}

package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestLoadConfig_SubagentTimeoutString(t *testing.T) {
	home := testutil.SetupTauHome(t, `{
		"subagent_timeout": "10m"
	}`, "")
	testutil.SetHomeEnv(t, home)

	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SubagentTimeout != 10*time.Minute {
		t.Errorf("SubagentTimeout: got %v, want 10m", cfg.SubagentTimeout)
	}
}

func TestLoadConfig_SubagentTimeoutStringInvalid(t *testing.T) {
	home := testutil.SetupTauHome(t, `{
		"subagent_timeout": "soon"
	}`, "")
	testutil.SetHomeEnv(t, home)

	_, err := config.LoadConfig("")
	if err == nil {
		t.Fatal("expected error for invalid subagent_timeout")
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
	if store["openai"].APIKey() != "sk-test-key-123" {
		t.Errorf("openai: got %q, want %q", store["openai"].APIKey(), "sk-test-key-123")
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
		"openai":       config.AuthValue{Value: "sk-test-key"},
		"opencode-zen": config.AuthValue{Value: "zen-key-123"},
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
	if loaded["openai"].APIKey() != "sk-test-key" {
		t.Errorf("openai: got %q, want %q", loaded["openai"].APIKey(), "sk-test-key")
	}
	if loaded["opencode-zen"].APIKey() != "zen-key-123" {
		t.Errorf("opencode-zen: got %q, want %q", loaded["opencode-zen"].APIKey(), "zen-key-123")
	}
}

func TestSaveAuth_ExplicitPath(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "custom-auth.json")

	store := config.AuthStore{"test-provider": config.AuthValue{Value: "test-key"}}
	err := config.SaveAuth(store, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.LoadAuth(path)
	if err != nil {
		t.Fatalf("load saved auth: %v", err)
	}
	if loaded["test-provider"].APIKey() != "test-key" {
		t.Errorf("test-provider: got %q, want %q", loaded["test-provider"].APIKey(), "test-key")
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

func TestAuthValue_MarshalJSON_APIKey(t *testing.T) {
	av := config.AuthValue{Value: "sk-test-key"}
	data, err := av.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should serialize as plain string for backward compatibility
	if string(data) != `"sk-test-key"` {
		t.Errorf("got %q, want %q", string(data), `"sk-test-key"`)
	}
}

func TestAuthValue_MarshalJSON_OAuth(t *testing.T) {
	av := config.AuthValue{
		Type:      "oauth",
		Access:    "access-token",
		Refresh:   "refresh-token",
		Expires:   1234567890,
		AccountID: "user-123",
	}
	data, err := av.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should serialize as object
	var parsed map[string]any
	if err := config.UnmarshalJSONForTest(data, &parsed); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if parsed["type"] != "oauth" {
		t.Errorf("type: got %v, want %q", parsed["type"], "oauth")
	}
	if parsed["access"] != "access-token" {
		t.Errorf("access: got %v, want %q", parsed["access"], "access-token")
	}
	if parsed["refresh"] != "refresh-token" {
		t.Errorf("refresh: got %v, want %q", parsed["refresh"], "refresh-token")
	}
}

func TestAuthValue_UnmarshalJSON_String(t *testing.T) {
	var av config.AuthValue
	err := av.UnmarshalJSONForTest([]byte(`"sk-test-key"`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if av.APIKey() != "sk-test-key" {
		t.Errorf("APIKey: got %q, want %q", av.APIKey(), "sk-test-key")
	}
	if av.IsAPIKey() != true {
		t.Error("IsAPIKey should be true")
	}
	if av.IsOAuth() != false {
		t.Error("IsOAuth should be false")
	}
}

func TestAuthValue_UnmarshalJSON_Object(t *testing.T) {
	var av config.AuthValue
	err := av.UnmarshalJSONForTest([]byte(`{"type":"oauth","access":"acc","refresh":"ref","expires":1234567890,"account_id":"user-123"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if av.Type != "oauth" {
		t.Errorf("Type: got %q, want %q", av.Type, "oauth")
	}
	if av.Access != "acc" {
		t.Errorf("Access: got %q, want %q", av.Access, "acc")
	}
	if av.Refresh != "ref" {
		t.Errorf("Refresh: got %q, want %q", av.Refresh, "ref")
	}
	if av.Expires != 1234567890 {
		t.Errorf("Expires: got %d, want %d", av.Expires, 1234567890)
	}
	if av.AccountID != "user-123" {
		t.Errorf("AccountID: got %q, want %q", av.AccountID, "user-123")
	}
	if av.IsOAuth() != true {
		t.Error("IsOAuth should be true")
	}
	if av.IsAPIKey() != false {
		t.Error("IsAPIKey should be false")
	}
}

func TestAuthValue_RoundTrip_APIKey(t *testing.T) {
	original := config.AuthValue{Value: "sk-test-key"}
	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored config.AuthValue
	if err := restored.UnmarshalJSONForTest(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.APIKey() != original.APIKey() {
		t.Errorf("round-trip failed: got %q, want %q", restored.APIKey(), original.APIKey())
	}
	if restored.IsAPIKey() != original.IsAPIKey() {
		t.Error("round-trip IsAPIKey mismatch")
	}
}

func TestAuthValue_RoundTrip_OAuth(t *testing.T) {
	original := config.AuthValue{
		Type:      "oauth",
		Access:    "access-token",
		Refresh:   "refresh-token",
		Expires:   1234567890,
		AccountID: "user-123",
	}
	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored config.AuthValue
	if err := restored.UnmarshalJSONForTest(data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if restored.Type != original.Type {
		t.Errorf("Type: got %q, want %q", restored.Type, original.Type)
	}
	if restored.Access != original.Access {
		t.Errorf("Access: got %q, want %q", restored.Access, original.Access)
	}
	if restored.Refresh != original.Refresh {
		t.Errorf("Refresh: got %q, want %q", restored.Refresh, original.Refresh)
	}
	if restored.Expires != original.Expires {
		t.Errorf("Expires: got %d, want %d", restored.Expires, original.Expires)
	}
	if restored.AccountID != original.AccountID {
		t.Errorf("AccountID: got %q, want %q", restored.AccountID, original.AccountID)
	}
}

func TestLoadAuth_MixedFormat(t *testing.T) {
	authJSON := `{
		"openai": "sk-api-key-123",
		"anthropic": {"type":"api_key","key":"sk-ant-456"},
		"openai-codex": {"type":"oauth","access":"jwt-token","refresh":"refresh-token","expires":1234567890,"account_id":"user-abc"}
	}`
	home := testutil.SetupTauHome(t, "", authJSON)
	testutil.SetHomeEnv(t, home)

	store, err := config.LoadAuth("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(store))
	}

	// Plain string API key
	if store["openai"].APIKey() != "sk-api-key-123" {
		t.Errorf("openai: got %q, want %q", store["openai"].APIKey(), "sk-api-key-123")
	}
	if !store["openai"].IsAPIKey() {
		t.Error("openai should be API key type")
	}

	// Object API key (PI/OpenCode format) — should parse but Value will be empty
	// since we don't have a "value" field, this tests graceful handling
	if store["anthropic"].Type != "api_key" {
		t.Errorf("anthropic type: got %q, want %q", store["anthropic"].Type, "api_key")
	}

	// OAuth credentials
	if !store["openai-codex"].IsOAuth() {
		t.Error("openai-codex should be OAuth type")
	}
	if store["openai-codex"].Access != "jwt-token" {
		t.Errorf("openai-codex access: got %q, want %q", store["openai-codex"].Access, "jwt-token")
	}
	if store["openai-codex"].AccountID != "user-abc" {
		t.Errorf("openai-codex account_id: got %q, want %q", store["openai-codex"].AccountID, "user-abc")
	}
}

func TestSaveAuth_BackwardCompatOutput(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "auth.json")

	store := config.AuthStore{
		"openai": config.AuthValue{Value: "sk-test-key"},
	}
	err := config.SaveAuth(store, path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read raw file content to verify backward compat format
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	// Verify it's a plain string, not an object
	var raw map[string]any
	if err := config.UnmarshalJSONForTest(data, &raw); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	openaiVal := raw["openai"]
	if str, ok := openaiVal.(string); !ok {
		t.Errorf("openai should be string in JSON, got %T: %v", openaiVal, openaiVal)
	} else if str != "sk-test-key" {
		t.Errorf("openai: got %q, want %q", str, "sk-test-key")
	}
}

func TestAuthValue_IsEmpty(t *testing.T) {
	empty := config.AuthValue{}
	if !empty.IsEmpty() {
		t.Error("empty AuthValue should report IsEmpty=true")
	}

	apiKey := config.AuthValue{Value: "sk-key"}
	if apiKey.IsEmpty() {
		t.Error("API key AuthValue should report IsEmpty=false")
	}

	oauth := config.AuthValue{Type: "oauth", Access: "token"}
	if oauth.IsEmpty() {
		t.Error("OAuth AuthValue should report IsEmpty=false")
	}
}

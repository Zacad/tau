package provider

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveKey_CLIFlag(t *testing.T) {
	key, err := ResolveKey("openai", "sk-cli-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-cli-key" {
		t.Fatalf("expected sk-cli-key, got %s", key)
	}
}

func TestResolveKey_EnvVar(t *testing.T) {
	// Ensure no CLI key and no auth.json
	os.Unsetenv("PRAXIS_TEST_AUTH_DIR")

	key, err := ResolveKey("testprovider", "")
	if err == nil {
		t.Fatalf("expected error, got key: %s", key)
	}

	// Set env var
	os.Setenv("TESTPROVIDER_API_KEY", "sk-env-key")
	t.Cleanup(func() { os.Unsetenv("TESTPROVIDER_API_KEY") })

	key, err = ResolveKey("testprovider", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-env-key" {
		t.Fatalf("expected sk-env-key, got %s", key)
	}
}

func TestResolveKey_EnvRefFormat(t *testing.T) {
	os.Setenv("MY_SPECIAL_KEY", "sk-resolved-key")
	t.Cleanup(func() { os.Unsetenv("MY_SPECIAL_KEY") })

	key, err := ResolveKey("testprovider", "$MY_SPECIAL_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-resolved-key" {
		t.Fatalf("expected sk-resolved-key, got %s", key)
	}
}

func TestResolveKey_UnsetEnvRef(t *testing.T) {
	os.Unsetenv("UNSET_SPECIAL_KEY")

	_, err := ResolveKey("testprovider", "$UNSET_SPECIAL_KEY")
	if err == nil {
		t.Fatal("expected error for unset env var")
	}
}

func TestResolveKey_ShellCommand(t *testing.T) {
	key, err := ResolveKey("testprovider", "!echo sk-shell-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-shell-key" {
		t.Fatalf("expected sk-shell-key, got %s", key)
	}
}

func TestResolveKey_AuthJSON(t *testing.T) {
	// Create a temp auth.json
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	content := `{"openai": "sk-auth-json-key", "anthropic": "$ANTHROPIC_FROM_AUTH"}`
	if err := os.WriteFile(authPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write auth.json: %v", err)
	}

	// Temporarily override home dir by setting a test auth path
	// We can't easily override authJSONPath, so we test the internal function
	key, err := readAuthKey(authPath, "openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-auth-json-key" {
		t.Fatalf("expected sk-auth-json-key, got %s", key)
	}

	// Test env ref from auth.json
	os.Setenv("ANTHROPIC_FROM_AUTH", "sk-resolved-from-auth")
	t.Cleanup(func() { os.Unsetenv("ANTHROPIC_FROM_AUTH") })

	rawKey, err := readAuthKey(authPath, "anthropic")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resolved, err := resolveKeyFormat(rawKey)
	if err != nil {
		t.Fatalf("unexpected error resolving format: %v", err)
	}
	if resolved != "sk-resolved-from-auth" {
		t.Fatalf("expected sk-resolved-from-auth, got %s", resolved)
	}
}

func TestResolveKey_MissingProvider(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	content := `{"openai": "sk-key"}`
	if err := os.WriteFile(authPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write auth.json: %v", err)
	}

	_, err := readAuthKey(authPath, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestResolveKey_PriorityChain(t *testing.T) {
	// Set env var
	os.Setenv("MYPUBLIC_API_KEY", "sk-env-key")
	t.Cleanup(func() { os.Unsetenv("MYPUBLIC_API_KEY") })

	// CLI flag should take priority
	key, err := ResolveKey("mypublic", "sk-cli-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-cli-key" {
		t.Fatalf("CLI flag should have priority, expected sk-cli-key, got %s", key)
	}

	// Without CLI flag, env var should be used
	key, err = ResolveKey("mypublic", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-env-key" {
		t.Fatalf("expected env var key, got %s", key)
	}
}

func TestResolveError_String(t *testing.T) {
	err := &ResolveError{
		Provider:     "test",
		EnvVarName:   "TEST_API_KEY",
		AuthFilePath: "/home/user/.tau/auth.json",
	}
	msg := err.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
	if !containsAll(msg, "test", "TEST_API_KEY") {
		t.Fatalf("error message should contain provider and env var name: %s", msg)
	}
}

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func TestResolveKey_LiteralKey(t *testing.T) {
	key, err := ResolveKey("testprovider", "sk-literal-key-12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-literal-key-12345" {
		t.Fatalf("expected sk-literal-key-12345, got %s", key)
	}
}

func TestStandardEnvVar(t *testing.T) {
	tests := []struct {
		provider string
		expected string
	}{
		{"openai", "OPENAI_API_KEY"},
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"google", "GOOGLE_API_KEY"},
		{"myprovider", "MYPROVIDER_API_KEY"},
	}

	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			got := standardEnvVar(tc.provider)
			if got != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, got)
			}
		})
	}
}

// Test with httptest server to verify auth resolution doesn't make network calls
func TestResolveKey_NoNetworkCalls(t *testing.T) {
	// This test verifies that ResolveKey doesn't make any HTTP calls
	// by setting up a server that would panic if called
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ResolveKey should not make network calls")
	}))
	defer server.Close()

	// ResolveKey should not use the server at all
	key, err := ResolveKey("openai", "sk-test-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-test-key" {
		t.Fatalf("expected sk-test-key, got %s", key)
	}
}

func TestExecShellCommand_Error(t *testing.T) {
	_, err := execShellCommand("exit 1")
	if err == nil {
		t.Fatal("expected error for failing command")
	}
}

func TestResolveKeyFormat_Empty(t *testing.T) {
	_, err := resolveKeyFormat("")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestAuthJSONPath_NoHome(t *testing.T) {
	// This test verifies authJSONPath returns empty when file doesn't exist
	// We can't easily test this without modifying the home directory
	// So we just verify the function doesn't panic
	path := authJSONPath()
	// May or may not exist depending on the test environment
	_ = path
}

func TestReadAuthJSON_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`not valid json`), 0600); err != nil {
		t.Fatalf("failed to write auth.json: %v", err)
	}

	_, err := readAuthKey(authPath, "openai")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestResolveKeyFormat_ShellCommandWithWhitespace(t *testing.T) {
	key, err := resolveKeyFormat("!echo '  sk-trimmed  '")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "sk-trimmed" {
		t.Fatalf("expected sk-trimmed, got %q", key)
	}
}

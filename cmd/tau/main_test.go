package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestParseFlags_InteractiveDefault(t *testing.T) {
	os.Args = []string{"tau"}
	cfg := parseFlags()

	if cfg.Output != modeInteractive {
		t.Errorf("expected interactive mode, got %q", cfg.Output)
	}
	if cfg.Prompt != "" {
		t.Errorf("expected empty prompt, got %q", cfg.Prompt)
	}
}

func TestParseFlags_PrintMode(t *testing.T) {
	os.Args = []string{"tau", "-p", "hello"}
	cfg := parseFlags()

	if cfg.Output != modePrint {
		t.Errorf("expected print mode, got %q", cfg.Output)
	}
	if cfg.Prompt != "hello" {
		t.Errorf("expected prompt 'hello', got %q", cfg.Prompt)
	}
}

func TestParseFlags_PrintModeLongFlag(t *testing.T) {
	os.Args = []string{"tau", "--print", "world"}
	cfg := parseFlags()

	if cfg.Output != modePrint {
		t.Errorf("expected print mode, got %q", cfg.Output)
	}
	if cfg.Prompt != "world" {
		t.Errorf("expected prompt 'world', got %q", cfg.Prompt)
	}
}

func TestParseFlags_JSONMode(t *testing.T) {
	os.Args = []string{"tau", "--mode", "json", "-p", "hello"}
	cfg := parseFlags()

	if cfg.Output != modeJSON {
		t.Errorf("expected json mode, got %q", cfg.Output)
	}
}

func TestParseFlags_ModelOverride(t *testing.T) {
	os.Args = []string{"tau", "-p", "hi", "--model", "gpt-4"}
	cfg := parseFlags()

	if cfg.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", cfg.Model)
	}
}

func TestParseFlags_SessionFlags(t *testing.T) {
	os.Args = []string{"tau", "-c"}
	cfg := parseFlags()

	if !cfg.Continue {
		t.Error("expected Continue=true")
	}

	os.Args = []string{"tau", "--no-session"}
	cfg = parseFlags()

	if !cfg.Ephemeral {
		t.Error("expected Ephemeral=true")
	}
}

// TestParseFlags_ErrorCases verifies that invalid flag combinations cause
// the binary to exit with code 2. These must be tested via subprocess
// because parseFlags calls os.Exit(2).
func TestParseFlags_ErrorCases(t *testing.T) {
	bin := buildTestBinary(t)

	tests := []struct {
		name string
		args []string
	}{
		{"unknown_flag", []string{"--bogus"}},
		{"invalid_mode", []string{"--mode", "yaml"}},
		{"json_without_prompt", []string{"--mode", "json"}},
		{"session_flag_in_print_mode", []string{"-p", "hello", "-c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(bin, tt.args...)
			err := cmd.Run()
			if err == nil {
				t.Fatal("expected non-zero exit code")
			}
			var exitErr *exec.ExitError
			if !isExitError(err, &exitErr) {
				t.Fatalf("expected exit error, got: %v", err)
			}
			if exitErr.ExitCode() != 2 {
				t.Errorf("expected exit code 2, got %d", exitErr.ExitCode())
			}
		})
	}
}

func buildTestBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := dir + "/tau"
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "." // build from module root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build test binary: %v\n%s", err, out)
	}
	return bin
}

func isExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}

func TestParseFlags_MockURL(t *testing.T) {
	os.Args = []string{"tau", "--mock", "http://localhost:9999"}
	cfg := parseFlags()

	if cfg.MockURL != "http://localhost:9999" {
		t.Errorf("expected mock URL 'http://localhost:9999', got %q", cfg.MockURL)
	}
	if os.Getenv("TAU_MOCK_URL") != "http://localhost:9999" {
		t.Errorf("expected TAU_MOCK_URL env var set, got %q", os.Getenv("TAU_MOCK_URL"))
	}
	os.Unsetenv("TAU_MOCK_URL")
}

func TestFriendlyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains string
	}{
		{"no_model", fmt.Errorf("no model specified, no default configured"), "No model configured"},
		{"resolve_model", fmt.Errorf("resolve model: not found"), "No model available"},
		{"no_provider", fmt.Errorf("no provider registered for openai"), "No provider available"},
		{"empty_api_key", fmt.Errorf("API key is empty for provider openai"), "API key not configured"},
		{"no_sessions", fmt.Errorf("no sessions found for /tmp"), "No previous sessions found"},
		{"open_session", fmt.Errorf("open session: corrupted"), "Failed to open session file"},
		{"agent_prompt", fmt.Errorf("agent prompt: connection refused"), "Request failed"},
		{"unknown", fmt.Errorf("some random error"), "some random error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := friendlyError(tt.err)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("friendlyError(%q) = %q, should contain %q", tt.err, got, tt.contains)
			}
		})
	}
}

func TestFriendlyError_Nil(t *testing.T) {
	if got := friendlyError(nil); got != "" {
		t.Errorf("friendlyError(nil) = %q, want empty", got)
	}
}

func TestExitConstants(t *testing.T) {
	if exitSuccess != 0 {
		t.Errorf("exitSuccess = %d, want 0", exitSuccess)
	}
	if exitRuntime != 1 {
		t.Errorf("exitRuntime = %d, want 1", exitRuntime)
	}
	if exitUsage != 2 {
		t.Errorf("exitUsage = %d, want 2", exitUsage)
	}
}

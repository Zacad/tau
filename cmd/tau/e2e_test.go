//go:build e2e

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// sseHandler returns an HTTP handler that serves canned SSE responses.
func sseHandler(events string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, events)
	}
}

const streamingTextResponse = `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":""}]}

data: {"choices":[{"delta":{"content":" world"},"finish_reason":""}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}

`

const streamingWithToolCall = `data: {"choices":[{"delta":{"content":"Let me check that."},"finish_reason":""}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{\"path\":\"test.txt\"}"}}]},"finish_reason":""}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}

`

func buildE2EBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := dir + "/tau"
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build test binary: %v\n%s", err, out)
	}
	return bin
}

func TestE2E_PrintMode(t *testing.T) {
	server := httptest.NewServer(sseHandler(streamingTextResponse))
	defer server.Close()

	bin := buildE2EBinary(t)
	cmd := exec.Command(bin, "--mock", server.URL, "-p", "hello")
	cmd.Env = append(os.Environ(), "TAU_MOCK_URL="+server.URL)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Hello world") {
		t.Errorf("expected 'Hello world' in output, got: %s", out)
	}
}

func TestE2E_PrintModeExitCode(t *testing.T) {
	server := httptest.NewServer(sseHandler(streamingTextResponse))
	defer server.Close()

	bin := buildE2EBinary(t)
	cmd := exec.Command(bin, "--mock", server.URL, "-p", "hello")
	cmd.Env = append(os.Environ(), "TAU_MOCK_URL="+server.URL)
	err := cmd.Run()
	if err != nil {
		t.Fatalf("expected exit code 0, got error: %v", err)
	}
}

func TestE2E_JSONMode(t *testing.T) {
	server := httptest.NewServer(sseHandler(streamingTextResponse))
	defer server.Close()

	bin := buildE2EBinary(t)
	cmd := exec.Command(bin, "--mock", server.URL, "--mode", "json", "-p", "hello")
	cmd.Env = append(os.Environ(), "TAU_MOCK_URL="+server.URL)
	out, err := cmd.Output() // stdout only; logs go to stderr
	if err != nil {
		// Get stderr for debugging
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Logf("stderr: %s", exitErr.Stderr)
		}
		t.Fatalf("command failed: %v\n%s", err, out)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		t.Fatal("expected JSONL output, got empty")
	}

	for i, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d is not valid JSON: %v\n  line: %s", i, err, line)
		}
	}
}

func TestE2E_JSONModeValidStructure(t *testing.T) {
	server := httptest.NewServer(sseHandler(streamingTextResponse))
	defer server.Close()

	bin := buildE2EBinary(t)
	cmd := exec.Command(bin, "--mock", server.URL, "--mode", "json", "-p", "hello")
	cmd.Env = append(os.Environ(), "TAU_MOCK_URL="+server.URL)
	out, err := cmd.Output() // stdout only
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Logf("stderr: %s", exitErr.Stderr)
		}
		t.Fatalf("command failed: %v\n%s", err, out)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var foundTextDelta bool
	for _, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("invalid JSON: %v\n  line: %s", err, line)
		}
		if eventType, ok := obj["type"].(string); ok && eventType == "text_delta" {
			foundTextDelta = true
		}
	}
	if !foundTextDelta {
		t.Errorf("expected text_delta event in JSONL output, got: %s", out)
	}
}

func TestE2E_StdinPiping(t *testing.T) {
	server := httptest.NewServer(sseHandler(streamingTextResponse))
	defer server.Close()

	bin := buildE2EBinary(t)
	cmd := exec.Command(bin, "--mock", server.URL)
	cmd.Env = append(os.Environ(), "TAU_MOCK_URL="+server.URL)
	cmd.Stdin = strings.NewReader("hello from stdin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Hello world") {
		t.Errorf("expected 'Hello world' in output, got: %s", out)
	}
}

func TestE2E_InvalidFlagExitCode(t *testing.T) {
	bin := buildE2EBinary(t)
	cmd := exec.Command(bin, "--bogus-flag")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit code for invalid flag")
	}
	var exitErr *exec.ExitError
	if !isExitError(err, &exitErr) {
		t.Fatalf("expected exit error, got: %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Errorf("expected exit code 2, got %d", exitErr.ExitCode())
	}
}

func TestE2E_VersionFlag(t *testing.T) {
	bin := buildE2EBinary(t)
	cmd := exec.Command(bin, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "tau") {
		t.Errorf("expected 'tau' in version output, got: %s", out)
	}
}

func TestE2E_HelpFlag(t *testing.T) {
	bin := buildE2EBinary(t)
	cmd := exec.Command(bin, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command failed: %v\n%s", err, out)
	}
	output := string(out)
	required := []string{"Usage:", "Flags:", "Examples:", "--model", "--version"}
	for _, r := range required {
		if !strings.Contains(output, r) {
			t.Errorf("expected %q in help output", r)
		}
	}
}

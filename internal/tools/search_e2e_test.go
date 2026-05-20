package tools

import (
	"context"
	"testing"
	"time"
)

func TestWebSearchTool_E2E_SearXNG(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	searxng := NewSearXNGBackend("http://localhost:8964", 10*time.Second)
	if !searxng.Available() {
		t.Skip("SearXNG not running at localhost:8964")
	}

	tool := NewWebSearchTool([]SearchBackend{searxng}, time.Now())
	if tool == nil {
		t.Fatal("NewWebSearchTool should not return nil with SearXNG available")
	}

	result, err := tool.Execute(context.Background(), &WebSearchParams{
		Query:      "Go programming language",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if len(text) == 0 {
		t.Fatal("Result should not be empty")
	}
	t.Logf("websearch result:\n%s", text)
}

func TestWebFetchTool_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	tool := NewWebFetchTool()

	result, err := tool.Execute(context.Background(), &WebFetchParams{
		URL:     "https://go.dev",
		Format:  "markdown",
		Timeout: 30,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute() returned error: %s", result.Content[0].Text)
	}

	text := result.Content[0].Text
	if len(text) == 0 {
		t.Fatal("Result should not be empty")
	}
	if len(text) < 50 {
		t.Fatalf("Result seems too short, got: %s", text)
	}
	t.Logf("webfetch result (first 200 chars): %s", truncateStr(text, 200))
}

func TestWebSearchTool_E2E_RegistrationNoBackends(t *testing.T) {
	tool := NewWebSearchTool(nil, time.Now())
	if tool != nil {
		t.Error("Should return nil when no backends")
	}

	tool = NewWebSearchTool([]SearchBackend{}, time.Now())
	if tool != nil {
		t.Error("Should return nil when empty backends")
	}
}

func TestWebFetchTool_E2E_SSRF(t *testing.T) {
	tool := NewWebFetchTool()

	tests := []string{
		"http://127.0.0.1/",
		"http://localhost/",
		"http://10.0.0.1/",
		"http://169.254.169.254/",
	}

	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), &WebFetchParams{URL: url})
			if err != nil {
				t.Fatalf("Execute() error: %v", err)
			}
			if !result.IsError {
				t.Errorf("SSRF: %s should be blocked", url)
			}
		})
	}
}

func TestWebSearchTool_E2E_FallbackChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	unreachable := NewSearXNGBackend("http://localhost:1", 100*time.Millisecond)
	searxng := NewSearXNGBackend("http://localhost:8964", 10*time.Second)

	backends := []SearchBackend{unreachable, searxng}
	tool := NewWebSearchTool(backends, time.Now())

	result, err := tool.Execute(context.Background(), &WebSearchParams{
		Query:      "Go programming",
		MaxResults: 3,
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Should succeed via fallback: %s", result.Content[0].Text)
	}
	t.Logf("Fallback chain result:\n%s", result.Content[0].Text)
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

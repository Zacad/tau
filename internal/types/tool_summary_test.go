package types_test

import (
	"strings"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestSummarizeToolArgs_RedactsSensitiveFields(t *testing.T) {
	summary := types.SummarizeToolArgs("webfetch", map[string]any{
		"url":           "https://example.com",
		"authorization": "Bearer secret-token",
		"api_key":       "secret-key",
	})

	if strings.Contains(summary, "secret") || strings.Contains(summary, "Bearer") {
		t.Fatalf("summary leaked sensitive data: %q", summary)
	}
	if !strings.Contains(summary, "https://example.com") {
		t.Fatalf("summary missing safe URL: %q", summary)
	}
}

func TestSummarizeToolArgs_OmitsLargeContentFields(t *testing.T) {
	summary := types.SummarizeToolArgs("write", map[string]any{
		"path":    "main.go",
		"content": strings.Repeat("x", 1000),
	})

	if strings.Contains(summary, strings.Repeat("x", 20)) {
		t.Fatalf("summary leaked content: %q", summary)
	}
	if !strings.Contains(summary, "path: main.go") {
		t.Fatalf("summary missing path: %q", summary)
	}
}

func TestSummarizeToolArgs_FindUsesPattern(t *testing.T) {
	summary := types.SummarizeToolArgs("find", map[string]any{
		"pattern": "*.go",
		"path":    "internal",
	})

	if !strings.Contains(summary, "pattern: *.go") || !strings.Contains(summary, "path: internal") {
		t.Fatalf("unexpected find summary: %q", summary)
	}
	if strings.Contains(summary, "name:") {
		t.Fatalf("find summary used legacy name key: %q", summary)
	}
}

func TestSummarizeToolArgs_Subagent(t *testing.T) {
	summary := types.SummarizeToolArgs("subagent", map[string]any{
		"type":    "researcher",
		"task":    strings.Repeat("research ", 40),
		"timeout": "5m",
	})

	if !strings.Contains(summary, "type: researcher") || !strings.Contains(summary, "timeout: 5m") {
		t.Fatalf("summary missing subagent metadata: %q", summary)
	}
	if len([]rune(summary)) > 180 {
		t.Fatalf("summary too long: %d runes %q", len([]rune(summary)), summary)
	}
}

func TestSummarizeToolArgs_RuneSafeTruncation(t *testing.T) {
	summary := types.SummarizeToolArgs("bash", map[string]any{
		"command": strings.Repeat("ąć", 100),
	})

	if !strings.HasSuffix(summary, "…") {
		t.Fatalf("summary should be truncated with ellipsis: %q", summary)
	}
	if !strings.Contains(summary, "ąć") {
		t.Fatalf("summary lost unicode content: %q", summary)
	}
}

func TestSummarizeToolArgsJSON_MalformedFallbackIsSafe(t *testing.T) {
	summary := types.SummarizeToolArgsJSON("unknown", []byte(`{"token":"secret-value",`))
	if strings.Contains(summary, "secret-value") {
		t.Fatalf("malformed fallback leaked sensitive value: %q", summary)
	}
	if summary == "" {
		t.Fatal("expected non-empty malformed fallback summary")
	}
}

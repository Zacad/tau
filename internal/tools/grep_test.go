package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newGrepTool(t *testing.T) (*GrepTool, string) {
	t.Helper()
	dir := t.TempDir()
	return NewGrepTool(dir, DefaultMaxOutputChars), dir
}

func TestGrepTool_FindsMatch(t *testing.T) {
	tool, dir := newGrepTool(t)
	tree := map[string]string{
		"a.txt": "hello world\nfoo bar",
		"b.txt": "no match here",
	}
	setupDirTree(t, dir, tree)

	result, err := tool.Execute(context.Background(), &GrepParams{
		Pattern: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "a.txt") {
		t.Errorf("expected a.txt in results: %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "hello world") {
		t.Errorf("expected matching line in results: %q", result.Content[0].Text)
	}
}

func TestGrepTool_NoMatch(t *testing.T) {
	tool, dir := newGrepTool(t)
	setupDirTree(t, dir, map[string]string{"a.txt": "nothing special"})

	result, err := tool.Execute(context.Background(), &GrepParams{
		Pattern: "xyz123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "No matches") {
		t.Errorf("expected 'No matches' in output: %q", result.Content[0].Text)
	}
}

func TestGrepTool_GlobFilter(t *testing.T) {
	tool, dir := newGrepTool(t)
	tree := map[string]string{
		"a.go": "package tools",
		"a.md": "package tools",
	}
	setupDirTree(t, dir, tree)

	result, err := tool.Execute(context.Background(), &GrepParams{
		Pattern: "package tools",
		Glob:    "*.go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	// Should only match .go file
	if strings.Contains(result.Content[0].Text, "a.md") {
		t.Errorf("a.md should be filtered out: %q", result.Content[0].Text)
	}
}

func TestGrepTool_CaseSensitive(t *testing.T) {
	tool, dir := newGrepTool(t)
	setupDirTree(t, dir, map[string]string{"a.txt": "Hello World"})

	// Case insensitive (default)
	result, err := tool.Execute(context.Background(), &GrepParams{
		Pattern: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError || strings.Contains(result.Content[0].Text, "No matches") {
		t.Errorf("case-insensitive should match: %q", result.Content[0].Text)
	}

	// Case sensitive
	result, err = tool.Execute(context.Background(), &GrepParams{
		Pattern:       "hello",
		CaseSensitive: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError || !strings.Contains(result.Content[0].Text, "No matches") {
		t.Errorf("case-sensitive should not match: %q", result.Content[0].Text)
	}
}

func TestGrepTool_InvalidRegex(t *testing.T) {
	tool, dir := newGrepTool(t)
	setupDirTree(t, dir, map[string]string{"a.txt": "test"})

	result, err := tool.Execute(context.Background(), &GrepParams{
		Pattern: "[invalid",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for invalid regex")
	}
}

func setupDirTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newFindTool(t *testing.T) (*FindTool, string) {
	t.Helper()
	dir := t.TempDir()
	return NewFindTool(dir, DefaultMaxOutputChars), dir
}

func TestFindTool_FindsFiles(t *testing.T) {
	tool, dir := newFindTool(t)
	tree := map[string]string{
		"a.go":  "package main",
		"b.go":  "package tools",
		"README.md": "# readme",
	}
	setupDirTree(t, dir, tree)

	result, err := tool.Execute(context.Background(), &FindParams{
		Pattern: "*.go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "a.go") {
		t.Errorf("expected a.go in results: %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "b.go") {
		t.Errorf("expected b.go in results: %q", result.Content[0].Text)
	}
	if strings.Contains(result.Content[0].Text, "README.md") {
		t.Errorf("README.md should not match *.go: %q", result.Content[0].Text)
	}
}

func TestFindTool_NoMatch(t *testing.T) {
	tool, dir := newFindTool(t)
	setupDirTree(t, dir, map[string]string{"a.txt": "test"})

	result, err := tool.Execute(context.Background(), &FindParams{
		Pattern: "*.xyz",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "No matches") {
		t.Errorf("expected 'No matches': %q", result.Content[0].Text)
	}
}

func TestFindTool_PathNotFound(t *testing.T) {
	tool, _ := newFindTool(t)

	result, err := tool.Execute(context.Background(), &FindParams{
		Pattern: "*.go",
		Path:    "/nonexistent/path",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for nonexistent path")
	}
}

func TestFindTool_Subdirectories(t *testing.T) {
	tool, dir := newFindTool(t)
	tree := map[string]string{
		"sub/a.go":    "package sub",
		"sub/deep/b.go": "package deep",
	}
	setupDirTree(t, dir, tree)

	result, err := tool.Execute(context.Background(), &FindParams{
		Pattern: "*.go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	// Results should include paths with subdirectories
	if !strings.Contains(result.Content[0].Text, "sub") {
		t.Errorf("expected sub directory in results: %q", result.Content[0].Text)
	}
}

func TestFindTool_SingleFile(t *testing.T) {
	tool, dir := newFindTool(t)
	path := filepath.Join(dir, "single.go")
	os.WriteFile(path, []byte("package main"), 0644)

	result, err := tool.Execute(context.Background(), &FindParams{
		Pattern: "*.go",
		Path:    path,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "single.go") {
		t.Errorf("expected single.go in results: %q", result.Content[0].Text)
	}
}

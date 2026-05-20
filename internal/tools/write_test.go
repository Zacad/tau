package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func newWriteTool(t *testing.T) (*WriteTool, string) {
	t.Helper()
	dir := t.TempDir()
	return NewWriteTool(dir, DefaultMaxOutputChars), dir
}

func TestWriteTool_CreatesFile(t *testing.T) {
	tool, dir := newWriteTool(t)

	result, err := tool.Execute(context.Background(), &WriteParams{
		Path:    "test.txt",
		Content: "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}

	content, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("got %q, want %q", string(content), "hello world")
	}
}

func TestWriteTool_CreatesParentDirs(t *testing.T) {
	tool, dir := newWriteTool(t)

	result, err := tool.Execute(context.Background(), &WriteParams{
		Path:    "a/b/c/test.txt",
		Content: "nested",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}

	content, err := os.ReadFile(filepath.Join(dir, "a", "b", "c", "test.txt"))
	if err != nil {
		t.Fatalf("nested file not created: %v", err)
	}
	if string(content) != "nested" {
		t.Errorf("got %q, want %q", string(content), "nested")
	}
}

func TestWriteTool_Overwrites(t *testing.T) {
	tool, dir := newWriteTool(t)
	existing := filepath.Join(dir, "existing.txt")
	os.WriteFile(existing, []byte("old content"), 0644)

	result, err := tool.Execute(context.Background(), &WriteParams{
		Path:    "existing.txt",
		Content: "new content",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}

	content, err := os.ReadFile(existing)
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if string(content) != "new content" {
		t.Errorf("got %q, want %q", string(content), "new content")
	}
}

func TestWriteTool_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteTool("/different", DefaultMaxOutputChars)
	path := filepath.Join(dir, "abs.txt")

	result, err := tool.Execute(context.Background(), &WriteParams{
		Path:    path,
		Content: "absolute",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(content) != "absolute" {
		t.Errorf("got %q, want %q", string(content), "absolute")
	}
}

func TestWriteTool_FilePaths(t *testing.T) {
	tool := NewWriteTool("/work", 1000)
	paths := tool.FilePaths(&WriteParams{Path: "target.txt"})
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0] != "/work/target.txt" {
		t.Errorf("got %q, want %q", paths[0], "/work/target.txt")
	}
}

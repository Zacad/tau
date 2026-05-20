package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newReadTool(t *testing.T) (*ReadTool, string) {
	t.Helper()
	dir := t.TempDir()
	return NewReadTool(dir, DefaultMaxOutputChars), dir
}

func TestReadTool_Success(t *testing.T) {
	tool, dir := newReadTool(t)
	path := filepath.Join(dir, "hello.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	result, err := tool.Execute(context.Background(), &ReadParams{Path: "hello.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if result.Content[0].Text != "hello world" {
		t.Errorf("got %q, want %q", result.Content[0].Text, "hello world")
	}
}

func TestReadTool_FileNotFound(t *testing.T) {
	tool, _ := newReadTool(t)

	result, err := tool.Execute(context.Background(), &ReadParams{Path: "nonexistent.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing file")
	}
}

func TestReadTool_EmptyFile(t *testing.T) {
	tool, dir := newReadTool(t)
	path := filepath.Join(dir, "empty.txt")
	os.WriteFile(path, []byte(""), 0644)

	result, err := tool.Execute(context.Background(), &ReadParams{Path: "empty.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if result.Content[0].Text != "" {
		t.Errorf("got %q, want empty", result.Content[0].Text)
	}
}

func TestReadTool_LineOffsetAndLimit(t *testing.T) {
	tool, dir := newReadTool(t)
	path := filepath.Join(dir, "lines.txt")
	content := "line1\nline2\nline3\nline4\nline5\n"
	os.WriteFile(path, []byte(content), 0644)

	result, err := tool.Execute(context.Background(), &ReadParams{Path: "lines.txt", Offset: 2, Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if !strings.Contains(result.Content[0].Text, "line2") || !strings.Contains(result.Content[0].Text, "line3") {
		t.Errorf("expected lines 2-3, got %q", result.Content[0].Text)
	}
}

func TestReadTool_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadTool("/different", DefaultMaxOutputChars)
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("absolute path test"), 0644)

	result, err := tool.Execute(context.Background(), &ReadParams{Path: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if result.Content[0].Text != "absolute path test" {
		t.Errorf("got %q, want %q", result.Content[0].Text, "absolute path test")
	}
}

func TestReadTool_Truncation(t *testing.T) {
	tool, dir := newReadTool(t)
	tool.maxChars = 50
	path := filepath.Join(dir, "big.txt")
	os.WriteFile(path, []byte(strings.Repeat("x", 200)), 0644)

	result, err := tool.Execute(context.Background(), &ReadParams{Path: "big.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result")
	}
	if !strings.Contains(result.Content[0].Text, "truncated") {
		t.Errorf("expected truncation notice in output: %q", result.Content[0].Text)
	}
	// Clean up temp file
	if result.Details != nil {
		if fp, ok := result.Details.(string); ok {
			os.Remove(fp)
		}
	}
}

func TestReadTool_FilePaths(t *testing.T) {
	tool := NewReadTool("/work", 1000)
	paths := tool.FilePaths(&ReadParams{Path: "sub/file.txt"})
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	expected := filepath.Join("/work", "sub", "file.txt")
	if paths[0] != expected {
		t.Errorf("got %q, want %q", paths[0], expected)
	}
}

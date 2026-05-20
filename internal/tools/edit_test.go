package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newEditTool(t *testing.T) (*EditTool, string) {
	t.Helper()
	dir := t.TempDir()
	return NewEditTool(dir, DefaultMaxOutputChars), dir
}

func TestEditTool_Success(t *testing.T) {
	tool, dir := newEditTool(t)
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("hello world\nsecond line"), 0644)

	result, err := tool.Execute(context.Background(), &EditParams{
		Path:    "edit.txt",
		OldText: "hello world",
		NewText: "goodbye world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not found: %v", err)
	}
	if !strings.Contains(string(content), "goodbye world") {
		t.Errorf("file not edited: %q", string(content))
	}
	if strings.Contains(string(content), "hello world") {
		t.Error("old text still present")
	}
}

func TestEditTool_NoMatch(t *testing.T) {
	tool, dir := newEditTool(t)
	path := filepath.Join(dir, "nomatch.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	result, err := tool.Execute(context.Background(), &EditParams{
		Path:    "nomatch.txt",
		OldText: "something else",
		NewText: "replacement",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for no match")
	}
}

func TestEditTool_MultipleMatches(t *testing.T) {
	tool, dir := newEditTool(t)
	path := filepath.Join(dir, "multi.txt")
	os.WriteFile(path, []byte("foo\nbar\nfoo\n"), 0644)

	result, err := tool.Execute(context.Background(), &EditParams{
		Path:    "multi.txt",
		OldText: "foo",
		NewText: "baz",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for multiple matches")
	}
	// Should mention line numbers
	if !strings.Contains(result.Content[0].Text, "2 matches") {
		t.Errorf("expected '2 matches' in error: %q", result.Content[0].Text)
	}
}

func TestEditTool_FileNotFound(t *testing.T) {
	tool, _ := newEditTool(t)

	result, err := tool.Execute(context.Background(), &EditParams{
		Path:    "nonexistent.txt",
		OldText: "x",
		NewText: "y",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for missing file")
	}
}

func TestEditTool_FilePaths(t *testing.T) {
	tool := NewEditTool("/work", 1000)
	paths := tool.FilePaths(&EditParams{Path: "target.txt"})
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(paths))
	}
	if paths[0] != "/work/target.txt" {
		t.Errorf("got %q, want %q", paths[0], "/work/target.txt")
	}
}

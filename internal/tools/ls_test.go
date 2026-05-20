package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newLsTool(t *testing.T) (*LsTool, string) {
	t.Helper()
	dir := t.TempDir()
	return NewLsTool(dir, DefaultMaxOutputChars), dir
}

func TestLsTool_ListDirectory(t *testing.T) {
	tool, dir := newLsTool(t)
	tree := map[string]string{
		"a.txt": "hello",
		"b.go":  "package main",
	}
	setupDirTree(t, dir, tree)

	result, err := tool.Execute(context.Background(), &LsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "a.txt") {
		t.Errorf("expected a.txt in listing: %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "b.go") {
		t.Errorf("expected b.go in listing: %q", result.Content[0].Text)
	}
}

func TestLsTool_LongFormat(t *testing.T) {
	tool, dir := newLsTool(t)
	setupDirTree(t, dir, map[string]string{"test.txt": "hello"})

	result, err := tool.Execute(context.Background(), &LsParams{LongFormat: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	// Long format should include file type and size
	if !strings.Contains(result.Content[0].Text, "file") {
		t.Errorf("expected file type in long format: %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "5") {
		t.Errorf("expected file size in long format: %q", result.Content[0].Text)
	}
}

func TestLsTool_HiddenFiles(t *testing.T) {
	tool, dir := newLsTool(t)
	tree := map[string]string{
		"visible.txt": "hi",
		".hidden":     "secret",
	}
	setupDirTree(t, dir, tree)

	// Without AllFiles
	result, err := tool.Execute(context.Background(), &LsParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if strings.Contains(result.Content[0].Text, ".hidden") {
		t.Errorf("hidden files should be excluded by default: %q", result.Content[0].Text)
	}

	// With AllFiles
	result, err = tool.Execute(context.Background(), &LsParams{AllFiles: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content[0].Text, ".hidden") {
		t.Errorf("hidden files should be included with AllFiles: %q", result.Content[0].Text)
	}
}

func TestLsTool_PathNotFound(t *testing.T) {
	tool, _ := newLsTool(t)

	result, err := tool.Execute(context.Background(), &LsParams{
		Path: "/nonexistent/path",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for nonexistent path")
	}
}

func TestLsTool_EmptyDirectory(t *testing.T) {
	tool, dir := newLsTool(t)
	emptyDir := filepath.Join(dir, "empty")
	os.MkdirAll(emptyDir, 0755)

	result, err := tool.Execute(context.Background(), &LsParams{Path: "empty"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "empty directory") {
		t.Errorf("expected 'empty directory': %q", result.Content[0].Text)
	}
}

func TestLsTool_SingleFile(t *testing.T) {
	tool, dir := newLsTool(t)
	path := filepath.Join(dir, "single.txt")
	os.WriteFile(path, []byte("test"), 0644)

	result, err := tool.Execute(context.Background(), &LsParams{Path: "single.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "single.txt") {
		t.Errorf("expected filename: %q", result.Content[0].Text)
	}
}

func TestLsTool_DirectoryFirst(t *testing.T) {
	tool, dir := newLsTool(t)
	tree := map[string]string{
		"a_file.txt": "hi",
		"z_dir/x.go": "package z",
	}
	setupDirTree(t, dir, tree)

	result, err := tool.Execute(context.Background(), &LsParams{AllFiles: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	// Directories should appear before files
	lines := strings.Split(result.Content[0].Text, "\n")
	dirIdx := -1
	fileIdx := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "dir") || strings.Contains(line, "z_dir") {
			if dirIdx == -1 {
				dirIdx = i
			}
		}
		if strings.Contains(line, "a_file") {
			fileIdx = i
		}
	}
	if dirIdx != -1 && fileIdx != -1 && dirIdx > fileIdx {
		t.Errorf("directories should appear before files")
	}
}

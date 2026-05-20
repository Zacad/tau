package tools

import (
	"os"
	"strings"
	"testing"
)

func TestTruncate_NoTruncation(t *testing.T) {
	text := "hello world"
	result, err := Truncate(text, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != text {
		t.Errorf("got %q, want %q", result.Output, text)
	}
	if result.Truncated {
		t.Error("should not be truncated")
	}
	if result.FullOutput != "" {
		t.Error("FullOutput should be empty when not truncated")
	}
	if result.FullOutputPath != "" {
		t.Error("FullOutputPath should be empty when not truncated")
	}
}

func TestTruncate_Truncated(t *testing.T) {
	text := strings.Repeat("a", 1000)
	result, err := Truncate(text, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Truncated {
		t.Fatal("expected truncation")
	}
	if result.FullOutput != text {
		t.Error("FullOutput should contain the original text")
	}
	if result.FullOutputPath == "" {
		t.Fatal("FullOutputPath should be set when truncated")
	}
	// Verify temp file exists and contains the full output
	content, err := os.ReadFile(result.FullOutputPath)
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}
	if string(content) != text {
		t.Error("temp file should contain full output")
	}
	// Cleanup
	os.Remove(result.FullOutputPath)
}

func TestTruncate_DefaultLimit(t *testing.T) {
	text := strings.Repeat("a", DefaultMaxOutputChars+1)
	result, err := Truncate(text, 0) // 0 means use default
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Truncated {
		t.Fatal("expected truncation with default limit")
	}
	if len(result.Output) > DefaultMaxOutputChars+200 {
		t.Errorf("output too long: %d chars", len(result.Output))
	}
	os.Remove(result.FullOutputPath)
}

func TestTruncate_ExactBoundary(t *testing.T) {
	text := strings.Repeat("a", 100)
	result, err := Truncate(text, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Truncated {
		t.Error("should not truncate at exact boundary")
	}
}

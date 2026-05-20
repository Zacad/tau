package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adam/tau/internal/types"
)

func newBashTool(t *testing.T, readOnly bool) (*BashTool, string) {
	t.Helper()
	dir := t.TempDir()
	return NewBashTool(dir, DefaultMaxOutputChars, readOnly), dir
}

func TestBashTool_SimpleCommand(t *testing.T) {
	tool, _ := newBashTool(t, false)

	result, err := tool.Execute(context.Background(), &BashParams{
		Command: "echo hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "hello") {
		t.Errorf("expected 'hello' in output: %q", result.Content[0].Text)
	}
}

func TestBashTool_ExitCode(t *testing.T) {
	tool, _ := newBashTool(t, false)

	result, err := tool.Execute(context.Background(), &BashParams{
		Command: "exit 42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-zero exit")
	}
	if result.Details == nil {
		t.Fatal("expected details")
	}
	details, ok := result.Details.(types.BashExecution)
	if !ok {
		t.Fatalf("expected BashExecution details, got %T", result.Details)
	}
	if details.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", details.ExitCode)
	}
}

func TestBashTool_CombinedOutput(t *testing.T) {
	tool, _ := newBashTool(t, false)

	result, err := tool.Execute(context.Background(), &BashParams{
		Command: "echo stdout; echo stderr >&2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "stdout") {
		t.Errorf("expected stdout in output: %q", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "stderr") {
		t.Errorf("expected stderr in output: %q", result.Content[0].Text)
	}
}

func TestBashTool_WorkingDirectory(t *testing.T) {
	tool, dir := newBashTool(t, false)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)

	result, err := tool.Execute(context.Background(), &BashParams{
		Command: "pwd",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result.Content[0].Text)
	}
	// pwd should return the working dir
	if !strings.Contains(result.Content[0].Text, dir) {
		t.Errorf("expected %q in output: %q", dir, result.Content[0].Text)
	}
}

func TestBashTool_ContextCancellation(t *testing.T) {
	tool, _ := newBashTool(t, false)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := tool.Execute(ctx, &BashParams{
		Command: "sleep 10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for cancelled context")
	}
}

func TestBashTool_ReadOnlyMode_MutatingBlocked(t *testing.T) {
	tool, _ := newBashTool(t, true)

	tests := []string{
		"rm -rf /",
		"touch newfile",
		"mkdir test",
	}
	for _, cmd := range tests {
		result, err := tool.Execute(context.Background(), &BashParams{Command: cmd})
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", cmd, err)
		}
		if !result.IsError {
			t.Errorf("expected blocking for mutating command: %q", cmd)
		}
	}
}

func TestBashTool_ReadOnlyMode_ReadAllowed(t *testing.T) {
	tool, _ := newBashTool(t, true)

	result, err := tool.Execute(context.Background(), &BashParams{
		Command: "ls /",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("read-only command should not be blocked: %v", result.Content[0].Text)
	}
}

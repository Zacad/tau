package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/adam/tau/internal/types"
)

// mockTool is a test double for Tool.
type mockTool struct {
	name        string
	desc        string
	params      any
	execMode    types.ExecutionMode
	execFn      func(ctx context.Context, params any) (*types.ToolResult, error)
	filePathsFn func(params any) []string
}

func (m *mockTool) Name() string                            { return m.name }
func (m *mockTool) Description() string                     { return m.desc }
func (m *mockTool) Parameters() any                         { return m.params }
func (m *mockTool) ExecutionMode() types.ExecutionMode      { return m.execMode }
func (m *mockTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	return m.execFn(ctx, params)
}
func (m *mockTool) FilePaths(params any) []string {
	if m.filePathsFn != nil {
		return m.filePathsFn(params)
	}
	return nil
}

type mockParams struct {
	Value string `json:"value"`
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test", desc: "test tool", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{}, nil
		}}
	r.Register(tool)

	got := r.Get("test")
	if got == nil {
		t.Fatal("expected tool to be registered")
	}
	if got.Name() != "test" {
		t.Errorf("got name %q, want %q", got.Name(), "test")
	}
}

func TestRegistry_RegisterDuplicatePanics(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "dup", desc: "dup", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{}, nil
		}}
	r.Register(tool)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate register")
		}
	}()
	r.Register(tool)
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "b", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) { return &types.ToolResult{}, nil }})
	r.Register(&mockTool{name: "a", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) { return &types.ToolResult{}, nil }})

	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "a" || names[1] != "b" {
		t.Errorf("expected sorted names [a, b], got %v", names)
	}
}

func TestRegistry_ToolDefinitions(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "test", desc: "A test tool", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) { return &types.ToolResult{}, nil }})

	defs := r.ToolDefinitions()
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	if defs[0].Name != "test" {
		t.Errorf("got name %q, want %q", defs[0].Name, "test")
	}
	if defs[0].Description != "A test tool" {
		t.Errorf("got desc %q, want %q", defs[0].Description, "A test tool")
	}
	if defs[0].Parameters == nil {
		t.Error("parameters schema should not be nil")
	}
}

func TestRegistry_ExecuteBatch_UnknownTool(t *testing.T) {
	r := NewRegistry()
	calls := []ToolCallRequest{
		{ID: "1", Name: "nonexistent"},
	}
	results := r.ExecuteBatch(context.Background(), calls)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Result.IsError {
		t.Error("expected error for unknown tool")
	}
}

func TestRegistry_ExecuteBatch_Allowlist(t *testing.T) {
	r := NewRegistry(WithAllowlist([]string{"read"}))
	r.Register(&mockTool{name: "read", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "ok"}}}, nil
		}})
	r.Register(&mockTool{name: "write", desc: "", params: &mockParams{}, execMode: types.ExecutionSequential,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "written"}}}, nil
		}})

	calls := []ToolCallRequest{
		{ID: "1", Name: "read", Arguments: []byte(`{"value":"x"}`)},
		{ID: "2", Name: "write", Arguments: []byte(`{"value":"y"}`)},
	}
	results := r.ExecuteBatch(context.Background(), calls)

	if results[0].Result.IsError {
		t.Errorf("read should be allowed: %v", results[0].Result.Content[0].Text)
	}
	if !results[1].Result.IsError {
		t.Error("write should be blocked by allowlist")
	}
}

func TestRegistry_ExecuteBatch_ReadOnly(t *testing.T) {
	r := NewRegistry(WithReadOnly(true))
	r.Register(&mockTool{name: "read", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "ok"}}}, nil
		}})
	r.Register(&mockTool{name: "write", desc: "", params: &mockParams{}, execMode: types.ExecutionSequential,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "written"}}}, nil
		}})
	r.Register(&mockTool{name: "bash", desc: "", params: &mockParams{}, execMode: types.ExecutionExclusive,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "ran"}}}, nil
		}})

	calls := []ToolCallRequest{
		{ID: "1", Name: "read", Arguments: []byte(`{"value":"x"}`)},
		{ID: "2", Name: "write", Arguments: []byte(`{"value":"y"}`)},
		{ID: "3", Name: "bash", Arguments: []byte(`{"value":"z"}`)},
	}
	results := r.ExecuteBatch(context.Background(), calls)

	if results[0].Result.IsError {
		t.Error("read should work in read-only mode")
	}
	if !results[1].Result.IsError {
		t.Error("write should be blocked in read-only mode")
	}
	if !results[2].Result.IsError {
		t.Error("bash should be blocked in read-only mode")
	}
}

func TestRegistry_ExecuteBatch_InvalidArgs(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "test", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{}, nil
		}})

	calls := []ToolCallRequest{
		{ID: "1", Name: "test", Arguments: []byte(`{invalid json`)},
	}
	results := r.ExecuteBatch(context.Background(), calls)
	if !results[0].Result.IsError {
		t.Error("expected error for invalid JSON arguments")
	}
}

func TestRegistry_ExecuteBatch_BeforeHook(t *testing.T) {
	r := NewRegistry(WithBeforeToolCall(func(ctx types.BeforeToolCallContext) (*types.BeforeToolCallResult, error) {
		if ctx.ToolName == "blocked" {
			return &types.BeforeToolCallResult{Allowed: false, BlockReason: "not allowed"}, nil
		}
		return &types.BeforeToolCallResult{Allowed: true}, nil
	}))
	r.Register(&mockTool{name: "ok", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "ok"}}}, nil
		}})
	r.Register(&mockTool{name: "blocked", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "should not run"}}}, nil
		}})

	calls := []ToolCallRequest{
		{ID: "1", Name: "ok", Arguments: []byte(`{"value":"x"}`)},
		{ID: "2", Name: "blocked", Arguments: []byte(`{"value":"y"}`)},
	}
	results := r.ExecuteBatch(context.Background(), calls)

	if results[0].Result.IsError {
		t.Error("ok tool should succeed")
	}
	if !results[1].Result.IsError {
		t.Error("blocked tool should be rejected by hook")
	}
}

func TestRegistry_ExecuteBatch_AfterHook(t *testing.T) {
	r := NewRegistry(WithAfterToolCall(func(ctx types.AfterToolCallContext) (*types.AfterToolCallResult, error) {
		return &types.AfterToolCallResult{
			Result: &types.ToolResult{
				Content: []types.ContentBlock{{Type: "text", Text: "modified by hook"}},
			},
		}, nil
	}))
	r.Register(&mockTool{name: "test", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "original"}}}, nil
		}})

	calls := []ToolCallRequest{
		{ID: "1", Name: "test", Arguments: []byte(`{"value":"x"}`)},
	}
	results := r.ExecuteBatch(context.Background(), calls)

	if results[0].Result.Content[0].Text != "modified by hook" {
		t.Errorf("got %q, want %q", results[0].Result.Content[0].Text, "modified by hook")
	}
}

func TestRegistry_ExecuteBatch_OrderPreserved(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "a", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "result-a"}}}, nil
		}})
	r.Register(&mockTool{name: "b", desc: "", params: &mockParams{}, execMode: types.ExecutionExclusive,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "result-b"}}}, nil
		}})

	calls := []ToolCallRequest{
		{ID: "1", Name: "a", Arguments: []byte(`{"value":"1"}`)},
		{ID: "2", Name: "b", Arguments: []byte(`{"value":"2"}`)},
	}
	results := r.ExecuteBatch(context.Background(), calls)

	if len(results) != 2 {
		t.Fatalf("expected 2 results")
	}
	// Results should be in source order
	if results[0].ID != "1" || results[1].ID != "2" {
		t.Errorf("results out of order: got IDs %s, %s", results[0].ID, results[1].ID)
	}
	if results[0].Result.Content[0].Text != "result-a" {
		t.Errorf("result[0] text = %q, want result-a", results[0].Result.Content[0].Text)
	}
	if results[1].Result.Content[0].Text != "result-b" {
		t.Errorf("result[1] text = %q, want result-b", results[1].Result.Content[0].Text)
	}
}

func TestIsMutatingTool(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"write", true},
		{"edit", true},
		{"bash", true},
		{"read", false},
		{"grep", false},
		{"find", false},
		{"ls", false},
	}
	for _, tt := range tests {
		tool := &mockTool{name: tt.name, execMode: types.ExecutionParallel,
			execFn: func(ctx context.Context, params any) (*types.ToolResult, error) { return nil, nil }}
		if got := isMutatingTool(tool); got != tt.expected {
			t.Errorf("isMutatingTool(%s) = %v, want %v", tt.name, got, tt.expected)
		}
	}
}

func TestRegistry_ExecuteBatch_SequentialSerialization(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "counter.txt")
	os.WriteFile(filePath, []byte("0"), 0644)

	var mu sync.Mutex
	writeTool := NewWriteTool(dir, DefaultMaxOutputChars)
	editTool := &mockTool{name: "edit", desc: "", params: &EditParams{},
		execMode: types.ExecutionSequential,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			mu.Lock()
			mu.Unlock()
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "edited"}}}, nil
		},
		filePathsFn: func(params any) []string { return []string{filePath} },
	}

	r := NewRegistry()
	r.Register(writeTool)
	r.Register(editTool)

	calls := []ToolCallRequest{
		{ID: "1", Name: "write", Arguments: []byte(`{"path":"counter.txt","content":"1"}`)},
		{ID: "2", Name: "edit", Arguments: []byte(`{"path":"counter.txt","oldText":"1","newText":"2"}`)},
	}
	results := r.ExecuteBatch(context.Background(), calls)

	for i, res := range results {
		if res.Result.IsError {
			t.Errorf("result[%d] should not be error: %v", i, res.Result.Content[0].Text)
		}
	}
}

func TestRegistry_ExecuteBatch_MixedExecutionModes(t *testing.T) {
	dir := t.TempDir()

	r := NewRegistry()
	r.Register(&mockTool{name: "read", desc: "", params: &mockParams{}, execMode: types.ExecutionParallel,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "read"}}}, nil
		}})
	r.Register(&mockTool{name: "bash", desc: "", params: &mockParams{}, execMode: types.ExecutionExclusive,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "bash"}}}, nil
		}})
	r.Register(&mockTool{name: "write", desc: "", params: &mockParams{}, execMode: types.ExecutionSequential,
		execFn: func(ctx context.Context, params any) (*types.ToolResult, error) {
			return &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "write"}}}, nil
		},
		filePathsFn: func(params any) []string { return []string{filepath.Join(dir, "f.txt")} }})

	calls := []ToolCallRequest{
		{ID: "1", Name: "read", Arguments: []byte(`{"value":"r"}`)},
		{ID: "2", Name: "bash", Arguments: []byte(`{"value":"b"}`)},
		{ID: "3", Name: "write", Arguments: []byte(`{"value":"w"}`)},
	}
	results := r.ExecuteBatch(context.Background(), calls)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, res := range results {
		if res.Result.IsError {
			t.Errorf("result[%d] error: %v", i, res.Result.Content[0].Text)
		}
	}
}

func TestTruncate_EmptyInput(t *testing.T) {
	result, err := Truncate("", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "" {
		t.Errorf("expected empty output, got %q", result.Output)
	}
	if result.Truncated {
		t.Error("empty input should not be truncated")
	}
}

func TestTruncate_NegativeLimit(t *testing.T) {
	text := strings.Repeat("x", 50)
	result, err := Truncate(text, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Truncated {
		t.Error("50 chars should not be truncated with default limit")
	}
	os.Remove(result.FullOutputPath)
}

func TestBashTool_RedirectBlocked(t *testing.T) {
	tool, _ := newBashTool(t, true)

	result, err := tool.Execute(context.Background(), &BashParams{
		Command: "echo test > /tmp/file",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("redirect should be blocked in read-only mode")
	}
}

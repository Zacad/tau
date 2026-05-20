package testutil_test

import (
	"context"
	"sync"
	"testing"

	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/types"
)

func TestMockProvider_Stream(t *testing.T) {
	events := []types.StreamEvent{
		{Type: types.EventStart},
		{Type: types.EventTextDelta, Delta: "Hello"},
		{Type: types.EventDone},
	}

	provider := &testutil.MockProvider{
		Events: events,
	}

	ch := provider.Stream(context.Background(), types.Model{}, nil, nil, types.StreamOptions{})

	var received []types.StreamEvent
	for e := range ch {
		received = append(received, e)
	}

	if len(received) != len(events) {
		t.Fatalf("received %d events, want %d", len(received), len(events))
	}

	for i, e := range received {
		if e.Type != events[i].Type {
			t.Errorf("event[%d].Type: got %q, want %q", i, e.Type, events[i].Type)
		}
	}
}

func TestMockProvider_StreamEmpty(t *testing.T) {
	provider := &testutil.MockProvider{}
	ch := provider.Stream(context.Background(), types.Model{}, nil, nil, types.StreamOptions{})

	count := 0
	for range ch {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 events from empty provider, got %d", count)
	}
}

func TestMockProvider_Name(t *testing.T) {
	p1 := &testutil.MockProvider{}
	if p1.Name() != "mock" {
		t.Errorf("default name: got %q, want %q", p1.Name(), "mock")
	}

	p2 := &testutil.MockProvider{ProviderName: "my-provider"}
	if p2.Name() != "my-provider" {
		t.Errorf("custom name: got %q, want %q", p2.Name(), "my-provider")
	}
}

func TestMockProvider_Complete(t *testing.T) {
	msg := &types.AgentMessage{ID: "msg-1", Role: types.RoleAssistant}
	provider := &testutil.MockProvider{
		CompleteResult: msg,
	}

	result, err := provider.Complete(context.Background(), types.Model{}, nil, nil, types.StreamOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ID != "msg-1" {
		t.Errorf("result ID: got %q, want %q", result.ID, "msg-1")
	}
}

func TestMockProvider_CompleteError(t *testing.T) {
	provider := &testutil.MockProvider{
		CompleteErr: types.NewProviderError("complete", nil),
	}

	_, err := provider.Complete(context.Background(), types.Model{}, nil, nil, types.StreamOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockTool_Execute(t *testing.T) {
	expectedResult := &types.ToolResult{
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "result"}},
	}

	tool := &testutil.MockTool{
		ToolName:        "test-tool",
		ToolDescription: "A test tool",
		Result:          expectedResult,
	}

	result, err := tool.Execute(context.Background(), map[string]any{"path": "file.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content[0].Text != "result" {
		t.Errorf("result text: got %q, want %q", result.Content[0].Text, "result")
	}

	if tool.CallCount() != 1 {
		t.Errorf("call count: got %d, want 1", tool.CallCount())
	}

	calls := tool.CallsSnapshot()
	if len(calls) != 1 {
		t.Fatalf("calls: got %d, want 1", len(calls))
	}
}

func TestMockTool_ExecuteError(t *testing.T) {
	tool := &testutil.MockTool{
		Err: types.NewToolError("test-tool", nil),
	}

	_, err := tool.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockTool_ConcurrentCalls(t *testing.T) {
	tool := &testutil.MockTool{
		Result: &types.ToolResult{},
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tool.Execute(context.Background(), nil)
		}()
	}
	wg.Wait()

	if tool.CallCount() != 10 {
		t.Errorf("call count: got %d, want 10", tool.CallCount())
	}
}

func TestMockTool_Reset(t *testing.T) {
	tool := &testutil.MockTool{Result: &types.ToolResult{}}
	tool.Execute(context.Background(), nil)
	tool.Execute(context.Background(), nil)

	if tool.CallCount() != 2 {
		t.Fatalf("before reset: got %d, want 2", tool.CallCount())
	}

	tool.Reset()
	if tool.CallCount() != 0 {
		t.Errorf("after reset: got %d, want 0", tool.CallCount())
	}
}

func TestMockTool_NameAndDescription(t *testing.T) {
	tool := &testutil.MockTool{
		ToolName:        "custom-tool",
		ToolDescription: "does custom things",
	}
	if tool.Name() != "custom-tool" {
		t.Errorf("name: got %q, want %q", tool.Name(), "custom-tool")
	}
	if tool.Description() != "does custom things" {
		t.Errorf("description: got %q, want %q", tool.Description(), "does custom things")
	}
}

func TestMockTool_Parameters(t *testing.T) {
	tool := &testutil.MockTool{}
	params := tool.Parameters()
	if params == nil {
		t.Fatal("mock tool parameters should not be nil (needed for jsonschema.Reflect)")
	}
	// Verify it returns a pointer to a struct (valid for jsonschema)
	_, ok := params.(*testutil.MockToolParams)
	if !ok {
		t.Errorf("expected *MockToolParams, got %T", params)
	}
}

func TestMockTool_ExecutionMode(t *testing.T) {
	tool := &testutil.MockTool{}
	if tool.ExecutionMode() != types.ExecutionParallel {
		t.Errorf("execution mode: got %q, want %q", tool.ExecutionMode(), types.ExecutionParallel)
	}
}

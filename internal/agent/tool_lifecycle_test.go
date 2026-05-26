package agent

import (
	"context"
	"testing"

	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/tools"
	"github.com/adam/tau/internal/types"
)

func TestRun_EmitsTypedInferredFinalizedLifecycleFromFinalAssistantMessage(t *testing.T) {
	a := newTestAgent()
	a.tools = tools.NewRegistry()
	a.tools.Register(&testutil.MockTool{
		ToolName:        "read",
		ToolDescription: "Read a file",
		Result: &types.ToolResult{
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "file content"}},
		},
	})

	turns := 0
	a.provider = &countingProvider{fn: func() []types.StreamEvent {
		turns++
		if turns == 1 {
			return []types.StreamEvent{{Type: types.EventDone, Message: &types.AgentMessage{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{{Type: types.BlockToolCall, ToolCall: &types.ToolCallBlock{
					ID:        "tc1",
					Name:      "read",
					Arguments: map[string]any{"path": "test.txt"},
				}}},
			}}}
		}
		return []types.StreamEvent{{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "done"}},
		}}}
	}}

	var lifecycle []types.ToolLifecycleEvent
	a.Subscribe(func(e types.AgentEvent) {
		if e.Type == types.AgentEventToolExecEnd {
			if payload, ok := e.Data.(types.ToolLifecycleEvent); ok {
				lifecycle = append(lifecycle, payload)
			}
		}
	})

	if err := a.Prompt(context.Background(), "read test.txt"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	if len(lifecycle) != 1 {
		t.Fatalf("got %d typed finalized lifecycle events, want 1: %#v", len(lifecycle), lifecycle)
	}
	got := lifecycle[0]
	if got.CallID != "tc1" || got.ToolName != "read" {
		t.Fatalf("correlation = (%q,%q), want (tc1,read)", got.CallID, got.ToolName)
	}
	if got.Phase != types.ToolLifecycleFinalized {
		t.Fatalf("phase = %q, want finalized", got.Phase)
	}
	if got.Source != types.ToolLifecycleSourceInferred {
		t.Fatalf("source = %q, want inferred", got.Source)
	}
	if !got.ArgsComplete {
		t.Fatal("ArgsComplete = false, want true")
	}
	if string(got.ArgsJSON) != `{"path":"test.txt"}` {
		t.Fatalf("ArgsJSON = %s", got.ArgsJSON)
	}
}

func TestRun_EmitsTypedProgressAndResultWithCallID(t *testing.T) {
	a := newTestAgent()
	a.tools = tools.NewRegistry()
	a.tools.Register(&testutil.MockTool{
		ToolName:        "read",
		ToolDescription: "Read a file",
		Result: &types.ToolResult{
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "file content"}},
		},
	})

	turns := 0
	a.provider = &countingProvider{fn: func() []types.StreamEvent {
		turns++
		if turns == 1 {
			return []types.StreamEvent{{Type: types.EventDone, Message: &types.AgentMessage{
				Role: types.RoleAssistant,
				Content: []types.ContentBlock{{Type: types.BlockToolCall, ToolCall: &types.ToolCallBlock{
					ID:        "tc1",
					Name:      "read",
					Arguments: map[string]any{"path": "test.txt"},
				}}},
			}}}
		}
		return []types.StreamEvent{{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "done"}},
		}}}
	}}

	var results []types.ToolResultEvent
	a.Subscribe(func(e types.AgentEvent) {
		if e.Type == types.AgentEventToolResult {
			if payload, ok := e.Data.(types.ToolResultEvent); ok {
				results = append(results, payload)
			}
		}
	})

	if err := a.Prompt(context.Background(), "read test.txt"); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d typed result events, want 1: %#v", len(results), results)
	}
	if results[0].CallID != "tc1" || results[0].ToolName != "read" {
		t.Fatalf("result correlation = (%q,%q), want (tc1,read)", results[0].CallID, results[0].ToolName)
	}
	if results[0].IsError {
		t.Fatal("IsError = true, want false")
	}
	if results[0].Content != "file content" {
		t.Fatalf("Content = %q, want file content", results[0].Content)
	}
}

package tui

import (
	"encoding/json"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestModel_ProcessEvent_ToolExecEndWithoutStartCreatesCompletedBlock(t *testing.T) {
	m := newTestModel()

	m.processEvent(testEvent(types.AgentEventToolExecEnd, types.ToolLifecycleEvent{
		CallID:       "tc1",
		ToolName:     "read",
		Phase:        types.ToolLifecycleFinalized,
		Source:       types.ToolLifecycleSourceInferred,
		ArgsJSON:     json.RawMessage(`{"path":"main.go"}`),
		ArgsComplete: true,
	}))

	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockToolCall || m.blocks[0].toolSt != toolSuccess {
		t.Fatalf("expected successful tool call block, got kind=%v status=%v", m.blocks[0].kind, m.blocks[0].toolSt)
	}
	if m.blocks[0].toolID != "tc1" || m.blocks[0].toolName != "read" {
		t.Fatalf("tool identity = (%q,%q), want (tc1,read)", m.blocks[0].toolID, m.blocks[0].toolName)
	}
}

func TestModel_ProcessEvent_MultiplePendingToolCallsCorrelateByID(t *testing.T) {
	m := newTestModel()

	m.processEvent(testEvent(types.AgentEventToolExecStart, types.ToolLifecycleEvent{CallID: "tc1", ToolName: "read", Phase: types.ToolLifecycleRequested, Source: types.ToolLifecycleSourceNative}))
	m.processEvent(testEvent(types.AgentEventToolExecStart, types.ToolLifecycleEvent{CallID: "tc2", ToolName: "bash", Phase: types.ToolLifecycleRequested, Source: types.ToolLifecycleSourceNative}))
	m.processEvent(testEvent(types.AgentEventToolExecEnd, types.ToolLifecycleEvent{CallID: "tc1", ToolName: "read", Phase: types.ToolLifecycleFinalized, Source: types.ToolLifecycleSourceNative, ArgsJSON: json.RawMessage(`{"path":"a.go"}`), ArgsComplete: true}))
	m.processEvent(testEvent(types.AgentEventToolExecEnd, types.ToolLifecycleEvent{CallID: "tc2", ToolName: "bash", Phase: types.ToolLifecycleFinalized, Source: types.ToolLifecycleSourceNative, ArgsJSON: json.RawMessage(`{"command":"go test ./..."}`), ArgsComplete: true}))

	if len(m.blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(m.blocks))
	}
	if m.blocks[0].toolID != "tc1" || m.blocks[0].toolName != "read" || m.blocks[0].toolSt != toolSuccess {
		t.Fatalf("first block = id %q name %q status %v", m.blocks[0].toolID, m.blocks[0].toolName, m.blocks[0].toolSt)
	}
	if m.blocks[1].toolID != "tc2" || m.blocks[1].toolName != "bash" || m.blocks[1].toolSt != toolSuccess {
		t.Fatalf("second block = id %q name %q status %v", m.blocks[1].toolID, m.blocks[1].toolName, m.blocks[1].toolSt)
	}
}

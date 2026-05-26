package types_test

import (
	"encoding/json"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestAgentEvent_LegacyDataAdaptsTypedToolPayloads(t *testing.T) {
	event := types.AgentEvent{Data: types.ToolLifecycleEvent{
		CallID:       "tc1",
		ToolName:     "read",
		Phase:        types.ToolLifecycleFinalized,
		Source:       types.ToolLifecycleSourceNative,
		ArgsJSON:     json.RawMessage(`{"path":"main.go"}`),
		ArgsComplete: true,
	}}

	legacy, ok := event.LegacyData().(map[string]any)
	if !ok {
		t.Fatalf("LegacyData() = %T, want map[string]any", event.LegacyData())
	}
	if legacy["id"] != "tc1" || legacy["tool"] != "read" || legacy["args"] != `{"path":"main.go"}` {
		t.Fatalf("unexpected legacy data: %#v", legacy)
	}
}

func TestAgentEvent_LegacyDataLeavesNonToolPayloadsUnchanged(t *testing.T) {
	event := types.AgentEvent{Data: "hello"}
	if event.LegacyData() != "hello" {
		t.Fatalf("LegacyData() = %#v, want hello", event.LegacyData())
	}
}

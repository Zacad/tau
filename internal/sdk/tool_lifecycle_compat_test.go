package sdk

import (
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestSessionSubscribe_DocumentsTypedToolPayloadAndLegacyAdapter(t *testing.T) {
	event := types.AgentEvent{
		Type: types.AgentEventToolExecEnd,
		Data: types.ToolLifecycleEvent{
			CallID:       "tc1",
			ToolName:     "read",
			Phase:        types.ToolLifecycleFinalized,
			Source:       types.ToolLifecycleSourceInferred,
			ArgsSummary:  "path: main.go",
			ArgsComplete: true,
		},
	}

	if _, ok := event.Data.(types.ToolLifecycleEvent); !ok {
		t.Fatalf("tool event data is not typed payload: %T", event.Data)
	}
	legacy, ok := event.LegacyData().(map[string]any)
	if !ok {
		t.Fatalf("LegacyData() = %T, want map[string]any", event.LegacyData())
	}
	if legacy["id"] != "tc1" || legacy["tool"] != "read" || legacy["argsSummary"] != "path: main.go" {
		t.Fatalf("unexpected legacy payload: %#v", legacy)
	}
}

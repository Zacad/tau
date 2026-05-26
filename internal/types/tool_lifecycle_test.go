package types_test

import (
	"encoding/json"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestToolLifecyclePayloads_CarryStableCorrelationAndPhase(t *testing.T) {
	requested := types.ToolLifecycleEvent{
		CallID:       "call-1",
		ToolName:     "read",
		Phase:        types.ToolLifecycleRequested,
		Source:       types.ToolLifecycleSourceNative,
		ArgsComplete: false,
	}

	if requested.CallID != "call-1" {
		t.Fatalf("CallID = %q, want stable tool call id", requested.CallID)
	}
	if requested.ToolName != "read" {
		t.Fatalf("ToolName = %q, want read", requested.ToolName)
	}
	if requested.Phase != types.ToolLifecycleRequested {
		t.Fatalf("Phase = %q, want requested", requested.Phase)
	}
	if requested.Source != types.ToolLifecycleSourceNative {
		t.Fatalf("Source = %q, want native", requested.Source)
	}
}

func TestToolLifecyclePayloads_JSONShapeIsStable(t *testing.T) {
	payload := types.ToolLifecycleEvent{
		CallID:       "call-1",
		ToolName:     "bash",
		Phase:        types.ToolLifecycleExecuting,
		Source:       types.ToolLifecycleSourceInferred,
		ArgsJSON:     json.RawMessage(`{"command":"go test ./..."}`),
		ArgsSummary:  "go test ./...",
		ArgsComplete: true,
	}

	got, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := map[string]any{
		"call_id":       "call-1",
		"tool":          "bash",
		"phase":         "executing",
		"source":        "inferred",
		"args_summary":  "go test ./...",
		"args_complete": true,
	}
	for key, wantVal := range want {
		if decoded[key] != wantVal {
			t.Fatalf("decoded[%q] = %#v, want %#v in %#v", key, decoded[key], wantVal, decoded)
		}
	}
	if _, ok := decoded["args"]; !ok {
		t.Fatalf("encoded payload missing args in %s", got)
	}
}

func TestToolLifecyclePayloads_PartialArgsUseSummaryWithoutRawJSON(t *testing.T) {
	payload := types.ToolLifecycleEvent{
		CallID:       "call-1",
		ToolName:     "bash",
		Phase:        types.ToolLifecycleRequested,
		Source:       types.ToolLifecycleSourceNative,
		ArgsSummary:  "command: go test",
		ArgsComplete: false,
	}

	got, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if _, ok := decoded["args"]; ok {
		t.Fatalf("partial payload should omit raw args JSON: %#v", decoded)
	}
	if decoded["args_complete"] != false {
		t.Fatalf("args_complete = %#v, want false", decoded["args_complete"])
	}
}

func TestToolResultPayload_JSONShapeIsStable(t *testing.T) {
	payload := types.ToolResultEvent{
		CallID:   "call-1",
		ToolName: "read",
		IsError:  false,
		Content:  "hello",
	}

	got, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := map[string]any{
		"call_id":  "call-1",
		"tool":     "read",
		"is_error": false,
		"content":  "hello",
	}
	for key, wantVal := range want {
		if decoded[key] != wantVal {
			t.Fatalf("decoded[%q] = %#v, want %#v in %#v", key, decoded[key], wantVal, decoded)
		}
	}
}

func TestLegacyToolLifecycleMapAdapter(t *testing.T) {
	payload := types.ToolLifecycleEvent{
		CallID:       "call-1",
		ToolName:     "read",
		Phase:        types.ToolLifecycleFinalized,
		Source:       types.ToolLifecycleSourceNative,
		ArgsJSON:     json.RawMessage(`{"path":"main.go"}`),
		ArgsSummary:  "main.go",
		ArgsComplete: true,
	}

	legacy := payload.LegacyMap()
	want := map[string]any{
		"id":           "call-1",
		"tool":         "read",
		"phase":        "finalized",
		"source":       "native",
		"args":         `{"path":"main.go"}`,
		"args_json":    `{"path":"main.go"}`,
		"argsSummary":  "main.go",
		"argsComplete": true,
	}
	for key, wantVal := range want {
		if legacy[key] != wantVal {
			t.Fatalf("legacy[%q] = %#v, want %#v in %#v", key, legacy[key], wantVal, legacy)
		}
	}
}

func TestLegacyToolProgressMapAdapter(t *testing.T) {
	legacy := types.ToolProgressEvent{
		CallID:   "call-1",
		ToolName: "bash",
		Message:  "running",
	}.LegacyMap()

	want := map[string]any{"id": "call-1", "tool": "bash", "message": "running"}
	for key, wantVal := range want {
		if legacy[key] != wantVal {
			t.Fatalf("legacy[%q] = %#v, want %#v", key, legacy[key], wantVal)
		}
	}
}

func TestLegacyToolResultMapAdapter(t *testing.T) {
	legacy := types.ToolResultEvent{
		CallID:   "call-1",
		ToolName: "read",
		IsError:  true,
		Content:  "failed",
	}.LegacyMap()

	want := map[string]any{"id": "call-1", "tool": "read", "isError": true, "content": "failed"}
	for key, wantVal := range want {
		if legacy[key] != wantVal {
			t.Fatalf("legacy[%q] = %#v, want %#v", key, legacy[key], wantVal)
		}
	}
}

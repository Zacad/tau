package types_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

func TestAgentMessage_JSONRoundTrip(t *testing.T) {
	msg := types.AgentMessage{
		ID:        "msg-001",
		Role:      types.RoleAssistant,
		Timestamp: time.Date(2026, 5, 2, 14, 30, 0, 0, time.UTC),
		API:       "anthropic-messages",
		Model:     "claude-sonnet-4-20250514",
		Content: []types.ContentBlock{
			{
				Type: types.BlockText,
				Text: "Hello, world!",
			},
			{
				Type: types.BlockToolCall,
				ToolCall: &types.ToolCallBlock{
					ID:   "tc-001",
					Name: "read",
					Arguments: map[string]any{
						"path": "file.go",
					},
				},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded types.AgentMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != msg.ID {
		t.Errorf("ID: got %q, want %q", decoded.ID, msg.ID)
	}
	if decoded.Role != msg.Role {
		t.Errorf("Role: got %q, want %q", decoded.Role, msg.Role)
	}
	if decoded.API != msg.API {
		t.Errorf("API: got %q, want %q", decoded.API, msg.API)
	}
	if len(decoded.Content) != len(msg.Content) {
		t.Fatalf("Content length: got %d, want %d", len(decoded.Content), len(msg.Content))
	}
	if decoded.Content[0].Type != msg.Content[0].Type {
		t.Errorf("Content[0].Type: got %q, want %q", decoded.Content[0].Type, msg.Content[0].Type)
	}
	if decoded.Content[1].ToolCall == nil {
		t.Fatal("Content[1].ToolCall is nil")
	}
	if decoded.Content[1].ToolCall.Name != msg.Content[1].ToolCall.Name {
		t.Errorf("ToolCall.Name: got %q, want %q", decoded.Content[1].ToolCall.Name, msg.Content[1].ToolCall.Name)
	}
}

func TestAgentMessage_ZeroValue(t *testing.T) {
	var msg types.AgentMessage
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal zero value: %v", err)
	}
	if string(data) == "" {
		t.Error("zero value marshal produced empty string")
	}
}

func TestContentBlock_NilPointersOmitEmpty(t *testing.T) {
	block := types.ContentBlock{
		Type: types.BlockText,
		Text: "hello",
	}

	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// ToolCall and Image should not be present (omitempty with nil pointers)
	if _, ok := raw["tool_call"]; ok {
		t.Error("tool_call should be omitted when nil")
	}
	if _, ok := raw["image"]; ok {
		t.Error("image should be omitted when nil")
	}
}

func TestToolResult_JSONRoundTrip(t *testing.T) {
	result := types.ToolResult{
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "file contents here"},
		},
		IsError:   false,
		Terminate: false,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded types.ToolResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Content) != 1 {
		t.Fatalf("Content length: got %d, want 1", len(decoded.Content))
	}
	if decoded.Content[0].Text != "file contents here" {
		t.Errorf("Content[0].Text: got %q, want %q", decoded.Content[0].Text, "file contents here")
	}
}

func TestToolResult_ErrorFlag(t *testing.T) {
	result := types.ToolResult{
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "error message"},
		},
		IsError: true,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if raw["is_error"] != true {
		t.Errorf("is_error: got %v, want true", raw["is_error"])
	}
}

func TestSessionEntry_JSONRoundTrip(t *testing.T) {
	msgData, err := json.Marshal(types.AgentMessage{
		ID:   "msg-001",
		Role: types.RoleUser,
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}

	entry := types.SessionEntry{
		Type:      types.EntryMessage,
		ID:        "entry-001",
		ParentID:  "parent-001",
		Data:      msgData,
		Timestamp: time.Date(2026, 5, 2, 14, 30, 0, 0, time.UTC),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded types.SessionEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != entry.Type {
		t.Errorf("Type: got %q, want %q", decoded.Type, entry.Type)
	}
	if decoded.ID != entry.ID {
		t.Errorf("ID: got %q, want %q", decoded.ID, entry.ID)
	}

	// Verify Data round-trip via UnmarshalData helper
	var decodedMsg types.AgentMessage
	if err := decoded.UnmarshalData(&decodedMsg); err != nil {
		t.Fatalf("UnmarshalData: %v", err)
	}
	if decodedMsg.ID != "msg-001" {
		t.Errorf("decoded message ID: got %q, want %q", decodedMsg.ID, "msg-001")
	}
}

func TestSessionEntry_UnknownType(t *testing.T) {
	data := []byte(`{"type":"unknown_type","id":"e-1","timestamp":"2026-05-02T14:30:00Z"}`)
	var entry types.SessionEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Type != "unknown_type" {
		t.Errorf("Type: got %q, want %q", entry.Type, "unknown_type")
	}
}

func TestSessionHeader_JSON(t *testing.T) {
	header := types.SessionHeader{
		Type:      "session",
		Version:   1,
		ID:        "a3f7b2c1",
		Timestamp: time.Date(2026, 5, 2, 14, 30, 0, 0, time.UTC),
		Cwd:       "/home/adam/Projects/tau",
		Name:      "test-session",
	}

	data, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded types.SessionHeader
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Version != 1 {
		t.Errorf("Version: got %d, want 1", decoded.Version)
	}
	if decoded.Cwd != header.Cwd {
		t.Errorf("Cwd: got %q, want %q", decoded.Cwd, header.Cwd)
	}
}

func TestStreamEvent_JSONRoundTrip(t *testing.T) {
	event := types.StreamEvent{
		Type:  types.EventTextDelta,
		Delta: "Hello",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded types.StreamEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Type != event.Type {
		t.Errorf("Type: got %q, want %q", decoded.Type, event.Type)
	}
	if decoded.Delta != event.Delta {
		t.Errorf("Delta: got %q, want %q", decoded.Delta, event.Delta)
	}
}

func TestStreamEvent_ErrorString(t *testing.T) {
	event := types.StreamEvent{
		Type:  types.EventError,
		Error: "connection refused",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if raw["error"] != "connection refused" {
		t.Errorf("error: got %v, want %q", raw["error"], "connection refused")
	}
}

func TestUsage_JSONRoundTrip(t *testing.T) {
	usage := types.Usage{
		Input:       1000,
		Output:      500,
		CacheRead:   200,
		CacheWrite:  100,
		TotalTokens: 1800,
		Cost: types.CostDollars{
			Input:  0.015,
			Output: 0.0075,
			Total:  0.0225,
		},
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded types.Usage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Input != usage.Input {
		t.Errorf("Input: got %d, want %d", decoded.Input, usage.Input)
	}
	if decoded.Cost.Total != usage.Cost.Total {
		t.Errorf("Cost.Total: got %f, want %f", decoded.Cost.Total, usage.Cost.Total)
	}
}

func TestModel_JSONRoundTrip(t *testing.T) {
	model := types.Model{
		ID:            "claude-sonnet-4-20250514",
		Name:          "Claude Sonnet 4",
		Provider:      "anthropic",
		API:           "anthropic-messages",
		Reasoning:     true,
		InputTypes:    []string{"text"},
		ContextWindow: 200000,
		MaxTokens:     8192,
		Cost: types.CostInfo{
			Input:  3.0,
			Output: 15.0,
		},
	}

	data, err := json.Marshal(model)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded types.Model
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != model.ID {
		t.Errorf("ID: got %q, want %q", decoded.ID, model.ID)
	}
	if decoded.Reasoning != model.Reasoning {
		t.Errorf("Reasoning: got %v, want %v", decoded.Reasoning, model.Reasoning)
	}
	if decoded.Cost.Input != model.Cost.Input {
		t.Errorf("Cost.Input: got %f, want %f", decoded.Cost.Input, model.Cost.Input)
	}
}

func TestTypedConstants(t *testing.T) {
	// Verify typed constants have expected values
	tests := []struct {
		name  string
		got   string
		want  string
	}{
		{"RoleUser", string(types.RoleUser), "user"},
		{"RoleAssistant", string(types.RoleAssistant), "assistant"},
		{"RoleToolResult", string(types.RoleToolResult), "tool_result"},
		{"BlockText", string(types.BlockText), "text"},
		{"BlockThinking", string(types.BlockThinking), "thinking"},
		{"BlockToolCall", string(types.BlockToolCall), "tool_call"},
		{"BlockImage", string(types.BlockImage), "image"},
		{"ExecutionParallel", string(types.ExecutionParallel), "parallel"},
		{"ExecutionSequential", string(types.ExecutionSequential), "sequential"},
		{"ExecutionExclusive", string(types.ExecutionExclusive), "exclusive_per_file"},
		{"ThinkingOff", string(types.ThinkingOff), "off"},
		{"ThinkingHigh", string(types.ThinkingHigh), "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestEntryTypeConstants(t *testing.T) {
	expected := map[types.EntryType]string{
		types.EntrySession:            "session",
		types.EntryMessage:            "message",
		types.EntryModelChange:        "model_change",
		types.EntryThinkingLevelChange: "thinking_level_change",
		types.EntryCompaction:         "compaction",
		types.EntryCustomEntry:        "custom_entry",
		types.EntryCustomMessage:      "custom_message",
		types.EntrySessionInfo:        "session_info",
	}

	for entryType, want := range expected {
		if string(entryType) != want {
			t.Errorf("%s: got %q, want %q", entryType, string(entryType), want)
		}
	}
}

func TestAgentEvent_TypedConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"AgentEventStart", string(types.AgentEventStart), "agent_start"},
		{"AgentEventMessageStart", string(types.AgentEventMessageStart), "message_start"},
		{"AgentEventTextDelta", string(types.AgentEventTextDelta), "text_delta"},
		{"AgentEventMessageEnd", string(types.AgentEventMessageEnd), "message_end"},
		{"AgentEventTurnEnd", string(types.AgentEventTurnEnd), "turn_end"},
		{"AgentEventAgentEnd", string(types.AgentEventAgentEnd), "agent_end"},
		{"AgentEventError", string(types.AgentEventError), "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestStreamEventTypeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"EventStart", string(types.EventStart), "start"},
		{"EventTextDelta", string(types.EventTextDelta), "text_delta"},
		{"EventThinkingDelta", string(types.EventThinkingDelta), "thinking_delta"},
		{"EventToolCallStart", string(types.EventToolCallStart), "toolcall_start"},
		{"EventToolCallEnd", string(types.EventToolCallEnd), "toolcall_end"},
		{"EventDone", string(types.EventDone), "done"},
		{"EventError", string(types.EventError), "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

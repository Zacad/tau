package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestOllamaProvider_BuildRequest(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	msgs := []types.AgentMessage{
		{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "cześć"}},
		},
	}

	opts := types.StreamOptions{
		SystemPrompt: "You are helpful.",
		MaxTokens:    4096,
	}

	body, err := p.buildRequest(types.Model{ID: "gemma4:e4b"}, msgs, nil, opts)
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	var req ollamaChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if req.Model != "gemma4:e4b" {
		t.Errorf("expected model=gemma4:e4b, got %q", req.Model)
	}
	if !req.Stream {
		t.Error("expected stream=true")
	}
	if req.Options.NumPredict != 4096 {
		t.Errorf("expected num_predict=4096, got %d", req.Options.NumPredict)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" {
		t.Errorf("expected first message role=system, got %q", req.Messages[0].Role)
	}
	if req.Messages[0].Content != "You are helpful." {
		t.Errorf("expected system content, got %q", req.Messages[0].Content)
	}
	if req.Messages[1].Role != "user" {
		t.Errorf("expected second message role=user, got %q", req.Messages[1].Role)
	}
	if req.Messages[1].Content != "cześć" {
		t.Errorf("expected user content, got %q", req.Messages[1].Content)
	}
}

func TestOllamaProvider_BuildRequest_DefaultTokens(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	msgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}},
	}

	opts := types.StreamOptions{
		SystemPrompt: "test",
		MaxTokens:    0, // not set
	}

	body, err := p.buildRequest(types.Model{ID: "test"}, msgs, nil, opts)
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	var req ollamaChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if req.Options.NumPredict != 32768 {
		t.Errorf("expected default num_predict=32768, got %d", req.Options.NumPredict)
	}
}

func TestOllamaProvider_BuildRequest_WithTools(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	msgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "list files"}}},
	}

	tools := []types.ToolDefinition{
		{
			Name:        "bash",
			Description: "Run a bash command",
			Parameters:  nil,
		},
	}

	opts := types.StreamOptions{
		SystemPrompt: "test",
		MaxTokens:    1000,
	}

	body, err := p.buildRequest(types.Model{ID: "test"}, msgs, tools, opts)
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	var req ollamaChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Tools[0].Type != "function" {
		t.Errorf("expected tool type=function, got %q", req.Tools[0].Type)
	}
	if req.Tools[0].Function.Name != "bash" {
		t.Errorf("expected tool name=bash, got %q", req.Tools[0].Function.Name)
	}
	if req.Tools[0].Function.Description != "Run a bash command" {
		t.Errorf("expected tool description, got %q", req.Tools[0].Function.Description)
	}
}

func TestOllamaProvider_BuildRequest_WithToolCalls(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	msgs := []types.AgentMessage{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{Type: types.BlockToolCall, ToolCall: &types.ToolCallBlock{
					ID:   "call_0",
					Name: "bash",
					Arguments: map[string]any{"command": "ls"},
				}},
			},
		},
		{
			Role:       types.RoleToolResult,
			Content:    []types.ContentBlock{{Type: types.BlockText, Text: "file1.txt\nfile2.txt"}},
			ToolCallID: "call_0",
		},
	}

	opts := types.StreamOptions{
		SystemPrompt: "test",
		MaxTokens:    1000,
	}

	body, err := p.buildRequest(types.Model{ID: "test"}, msgs, nil, opts)
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	var req ollamaChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	assistantMsg := req.Messages[1]
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call in assistant message, got %d", len(assistantMsg.ToolCalls))
	}
	if assistantMsg.ToolCalls[0].Function.Name != "bash" {
		t.Errorf("expected tool call name=bash, got %q", assistantMsg.ToolCalls[0].Function.Name)
	}

	toolMsg := req.Messages[2]
	if toolMsg.Role != "tool" {
		t.Errorf("expected tool message role=tool, got %q", toolMsg.Role)
	}
}

func TestOllamaProvider_BuildRequest_WithThinking(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	msgs := []types.AgentMessage{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{Type: types.BlockThinking, Text: "Let me think..."},
				{Type: types.BlockText, Text: "OK."},
			},
		},
		{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "thanks"}},
		},
	}

	opts := types.StreamOptions{
		SystemPrompt: "test",
		MaxTokens:    1000,
	}

	body, err := p.buildRequest(types.Model{ID: "test"}, msgs, nil, opts)
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	var req ollamaChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	assistantMsg := req.Messages[1]
	if assistantMsg.Thinking != "" {
		t.Errorf("thinking content should NOT be sent back in multi-turn for gemma4, got %q", assistantMsg.Thinking)
	}
	if assistantMsg.Content != "OK." {
		t.Errorf("expected text content, got %q", assistantMsg.Content)
	}
}

func TestOllamaProvider_BuildRequest_WithThinking_ReasoningModel(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	msgs := []types.AgentMessage{
		{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{Type: types.BlockThinking, Text: "Let me think..."},
				{Type: types.BlockText, Text: "OK."},
			},
		},
		{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "thanks"}},
		},
	}

	opts := types.StreamOptions{
		SystemPrompt: "test",
		MaxTokens:    1000,
	}

	model := types.Model{
		ID:        "gemma4:26b",
		Reasoning: true,
	}

	body, err := p.buildRequest(model, msgs, nil, opts)
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	var req ollamaChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	// Messages: [system, assistant, user]
	if len(req.Messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(req.Messages))
	}
	assistantMsg := req.Messages[1]
	if assistantMsg.Thinking != "Let me think..." {
		t.Errorf("thinking content SHOULD be sent for reasoning models, got %q", assistantMsg.Thinking)
	}
	if assistantMsg.Content != "OK." {
		t.Errorf("expected text content, got %q", assistantMsg.Content)
	}
}

func TestOllamaProvider_BuildRequest_ThinkingLevel(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	msgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}},
	}

	tests := []struct {
		name          string
		reasoning     bool
		thinkingLevel types.ThinkingLevel
		wantLevel     string
	}{
		{"non-reasoning model", false, types.ThinkingMedium, ""},
		{"reasoning model off", true, types.ThinkingOff, ""},
		{"reasoning model empty", true, "", ""},
		{"reasoning model low", true, types.ThinkingLow, "low"},
		{"reasoning model medium", true, types.ThinkingMedium, "medium"},
		{"reasoning model high", true, types.ThinkingHigh, "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := types.Model{
				ID:        "gemma4:26b",
				Reasoning: tt.reasoning,
				ThinkingLevelMap: map[string]string{
					"low":    "low",
					"medium": "medium",
					"high":   "high",
				},
			}
			opts := types.StreamOptions{
				SystemPrompt:  "test",
				ThinkingLevel: tt.thinkingLevel,
			}

			body, err := p.buildRequest(model, msgs, nil, opts)
			if err != nil {
				t.Fatalf("buildRequest error: %v", err)
			}

			var req ollamaChatRequest
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("failed to unmarshal request: %v", err)
			}

			if req.Options.ThinkingLevel != tt.wantLevel {
				t.Errorf("thinking_level: got %q, want %q", req.Options.ThinkingLevel, tt.wantLevel)
			}
		})
	}
}

func TestOllamaProvider_BuildRequest_ThinkingLevelMapping(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	msgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}},
	}

	// Test that model.MapThinkingLevel is used (provider-specific values)
	model := types.Model{
		ID:        "gemma4:26b",
		Reasoning: true,
		ThinkingLevelMap: map[string]string{
			"low":    "minimal",
			"medium": "moderate",
			"high":   "extensive",
		},
	}

	opts := types.StreamOptions{
		SystemPrompt:  "test",
		ThinkingLevel: types.ThinkingMedium,
	}

	body, err := p.buildRequest(model, msgs, nil, opts)
	if err != nil {
		t.Fatalf("buildRequest error: %v", err)
	}

	var req ollamaChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}

	if req.Options.ThinkingLevel != "moderate" {
		t.Errorf("thinking_level: got %q, want %q (should use model mapping)", req.Options.ThinkingLevel, "moderate")
	}
}

func TestOllamaProvider_ParseStream_ThinkingThenText(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	// Simulate Ollama's streaming response: thinking first, then content.
	streamData := []byte(`{"model":"gemma4:26b","message":{"role":"assistant","content":"","thinking":"Let me think about this..."},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":"","thinking":"More thinking."},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":"Hello!","thinking":""},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":" How can I help?","thinking":""},"done":true}
`)

	ch := make(chan types.StreamEvent, 64)
	go func() {
		p.parseStreamResponse(context.Background(), ch, bytes.NewReader(streamData), "gemma4:26b")
		close(ch)
	}()

	events := collectStream(ch)

	// Verify event sequence
	var eventTypes []types.StreamEventType
	var thinkingDeltas []string
	var textDeltas []string

	for _, e := range events {
		eventTypes = append(eventTypes, e.Type)
		switch e.Type {
		case types.EventThinkingDelta:
			thinkingDeltas = append(thinkingDeltas, e.Delta)
		case types.EventTextDelta:
			textDeltas = append(textDeltas, e.Delta)
		}
	}

	// Check event order
	wantStart := []types.StreamEventType{
		types.EventStart,
		types.EventThinkingStart,
		types.EventThinkingDelta,
		types.EventThinkingDelta,
		types.EventThinkingEnd,
		types.EventTextDelta,
		types.EventTextDelta,
		types.EventDone,
	}

	if len(eventTypes) != len(wantStart) {
		t.Errorf("expected %d events, got %d: %v", len(wantStart), len(eventTypes), eventTypes)
	}

	for i, want := range wantStart {
		if i >= len(eventTypes) {
			t.Errorf("missing event at index %d: expected %s", i, want)
			continue
		}
		if eventTypes[i] != want {
			t.Errorf("event[%d]: expected %s, got %s", i, want, eventTypes[i])
		}
	}

	// Check thinking deltas
	if len(thinkingDeltas) != 2 {
		t.Errorf("expected 2 thinking deltas, got %d", len(thinkingDeltas))
	}
	if thinkingDeltas[0] != "Let me think about this..." {
		t.Errorf("unexpected first thinking delta: %q", thinkingDeltas[0])
	}
	if thinkingDeltas[1] != "More thinking." {
		t.Errorf("unexpected second thinking delta: %q", thinkingDeltas[1])
	}

	// Check text deltas
	if len(textDeltas) != 2 {
		t.Errorf("expected 2 text deltas, got %d", len(textDeltas))
	}
	if textDeltas[0] != "Hello!" {
		t.Errorf("unexpected first text delta: %q", textDeltas[0])
	}
	if textDeltas[1] != " How can I help?" {
		t.Errorf("unexpected second text delta: %q", textDeltas[1])
	}
}

func TestOllamaProvider_ParseStream_ThinkingOnly(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	// Simulate a response where the model only produces thinking (no content).
	streamData := []byte(`{"model":"gemma4:26b","message":{"role":"assistant","content":"","thinking":"Hmm, let me see..."},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":"","thinking":""},"done":true}
`)

	ch := make(chan types.StreamEvent, 64)
	go func() {
		p.parseStreamResponse(context.Background(), ch, bytes.NewReader(streamData), "gemma4:26b")
		close(ch)
	}()

	events := collectStream(ch)

	var eventTypes []types.StreamEventType
	for _, e := range events {
		eventTypes = append(eventTypes, e.Type)
	}

	wantTypes := []types.StreamEventType{
		types.EventStart,
		types.EventThinkingStart,
		types.EventThinkingDelta,
		types.EventThinkingEnd,
		types.EventDone,
	}

	if len(eventTypes) != len(wantTypes) {
		t.Errorf("expected %d events, got %d: %v", len(wantTypes), len(eventTypes), eventTypes)
	}

	for i, want := range wantTypes {
		if i >= len(eventTypes) {
			t.Errorf("missing event at index %d: expected %s", i, want)
			continue
		}
		if eventTypes[i] != want {
			t.Errorf("event[%d]: expected %s, got %s", i, want, eventTypes[i])
		}
	}
}

func TestOllamaProvider_ParseStream_Interleaved(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	// Simulate interleaved thinking and content (thinking → content → thinking → content).
	streamData := []byte(`{"model":"gemma4:26b","message":{"role":"assistant","content":"","thinking":"Let me think..."},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":"Sure,","thinking":""},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":"","thinking":"Wait, actually..."},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":" here's the answer.","thinking":""},"done":true}
`)

	ch := make(chan types.StreamEvent, 64)
	go func() {
		p.parseStreamResponse(context.Background(), ch, bytes.NewReader(streamData), "gemma4:26b")
		close(ch)
	}()

	events := collectStream(ch)

	var eventTypes []types.StreamEventType
	for _, e := range events {
		eventTypes = append(eventTypes, e.Type)
	}

	// Should see: start, thinking_start, thinking_delta, thinking_end, text_delta,
	// thinking_start, thinking_delta, thinking_end, text_delta, done
	wantTypes := []types.StreamEventType{
		types.EventStart,
		types.EventThinkingStart,
		types.EventThinkingDelta,
		types.EventThinkingEnd,
		types.EventTextDelta,
		types.EventThinkingStart,
		types.EventThinkingDelta,
		types.EventThinkingEnd,
		types.EventTextDelta,
		types.EventDone,
	}

	if len(eventTypes) != len(wantTypes) {
		t.Errorf("expected %d events, got %d: %v", len(wantTypes), len(eventTypes), eventTypes)
	}

	for i, want := range wantTypes {
		if i >= len(eventTypes) {
			t.Errorf("missing event at index %d: expected %s", i, want)
			continue
		}
		if eventTypes[i] != want {
			t.Errorf("event[%d]: expected %s, got %s", i, want, eventTypes[i])
		}
	}
}

func TestOllamaProvider_ParseStream_ToolCalls(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	streamData := []byte(`{"model":"gemma4:26b","message":{"role":"assistant","content":"","thinking":"Let me check the files..."},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":"","thinking":""},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"bash","arguments":{"command":"ls -la"}}}]},"done":false}
{"model":"gemma4:26b","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}
`)

	ch := make(chan types.StreamEvent, 64)
	go func() {
		p.parseStreamResponse(context.Background(), ch, bytes.NewReader(streamData), "gemma4:26b")
		close(ch)
	}()

	events := collectStream(ch)

	var eventTypes []types.StreamEventType
	var toolCallNames []string
	for _, e := range events {
		eventTypes = append(eventTypes, e.Type)
		if e.Type == types.EventToolCallStart || e.Type == types.EventToolCallEnd {
			toolCallNames = append(toolCallNames, e.Delta)
		}
	}

	hasToolCallStart := false
	hasToolCallEnd := false
	for _, et := range eventTypes {
		if et == types.EventToolCallStart {
			hasToolCallStart = true
		}
		if et == types.EventToolCallEnd {
			hasToolCallEnd = true
		}
	}

	if !hasToolCallStart {
		t.Error("missing EventToolCallStart")
	}
	if !hasToolCallEnd {
		t.Error("missing EventToolCallEnd")
	}
	if len(toolCallNames) != 2 {
		t.Fatalf("expected 2 tool call name events (start+end), got %d", len(toolCallNames))
	}
	if toolCallNames[0] != "bash" {
		t.Errorf("expected tool name 'bash', got %q", toolCallNames[0])
	}
}

func TestOllamaProvider_CollectFromStream_Error(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	ch := make(chan types.StreamEvent, 1)
	ch <- types.StreamEvent{Type: types.EventError, Error: "connection refused"}
	close(ch)

	_, err := p.collectFromStream(ch)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOllamaProvider_CollectFromStream_NoResponse(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	ch := make(chan types.StreamEvent, 1)
	ch <- types.StreamEvent{Type: types.EventStart}
	close(ch)

	_, err := p.collectFromStream(ch)
	if err == nil {
		t.Fatal("expected error for empty response, got nil")
	}
}

// TestOllamaProvider_ParseStream_PolishInput_ThinkingThenText is a regression
// test for the Polish input bug: gemma4:26b previously showed thinking but
// no response text for Polish inputs like "cześć". This test verifies that
// the native Ollama provider correctly separates thinking from content for
// non-ASCII/Polish inputs using actual captured SSE data.
func TestOllamaProvider_ParseStream_PolishInput_ThinkingThenText(t *testing.T) {
	p := NewOllamaProvider("http://localhost:11434")

	// Real SSE capture from Ollama gemma4:26b for input "cześć".
	// Key: many thinking chunks first, then content chunks, no overlap.
	streamData := []byte(`{"model":"gemma4:26b","created_at":"2026-05-04T17:43:04.45834355Z","message":{"role":"assistant","content":"","thinking":"*"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:04.540052633Z","message":{"role":"assistant","content":"","thinking":"   Input"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:04.579245607Z","message":{"role":"assistant","content":"","thinking":":"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:04.618488895Z","message":{"role":"assistant","content":"","thinking":" \""},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:04.658258651Z","message":{"role":"assistant","content":"","thinking":"cze"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:04.697701474Z","message":{"role":"assistant","content":"","thinking":"ść"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.300331941Z","message":{"role":"assistant","content":"C"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.346187547Z","message":{"role":"assistant","content":"ze"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.392391456Z","message":{"role":"assistant","content":"ść"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.443891433Z","message":{"role":"assistant","content":"!"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.493202994Z","message":{"role":"assistant","content":" W"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.54077456Z","message":{"role":"assistant","content":" czym"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.586364207Z","message":{"role":"assistant","content":" mogę"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.632984578Z","message":{"role":"assistant","content":" Ci"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.680589146Z","message":{"role":"assistant","content":" dz"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.728819377Z","message":{"role":"assistant","content":"isiaj"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.775986454Z","message":{"role":"assistant","content":" pom"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.822250837Z","message":{"role":"assistant","content":"óc"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.866507332Z","message":{"role":"assistant","content":"?"},"done":false}
{"model":"gemma4:26b","created_at":"2026-05-04T17:43:12.915086619Z","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop"}
`)

	ch := make(chan types.StreamEvent, 64)
	go func() {
		p.parseStreamResponse(context.Background(), ch, bytes.NewReader(streamData), "gemma4:26b")
		close(ch)
	}()

	events := collectStream(ch)

	var eventTypes []types.StreamEventType
	var thinkingDeltas []string
	var textDeltas []string
	var hasThinkingStart, hasThinkingEnd, hasDone bool

	for _, e := range events {
		eventTypes = append(eventTypes, e.Type)
		switch e.Type {
		case types.EventThinkingStart:
			hasThinkingStart = true
		case types.EventThinkingDelta:
			thinkingDeltas = append(thinkingDeltas, e.Delta)
		case types.EventThinkingEnd:
			hasThinkingEnd = true
		case types.EventTextDelta:
			textDeltas = append(textDeltas, e.Delta)
		case types.EventDone:
			hasDone = true
		}
	}

	// Must have thinking lifecycle events
	if !hasThinkingStart {
		t.Error("missing EventThinkingStart")
	}
	if !hasThinkingEnd {
		t.Error("missing EventThinkingEnd")
	}
	if !hasDone {
		t.Error("missing EventDone")
	}

	// Must have thinking deltas (Polish input triggers thinking)
	if len(thinkingDeltas) == 0 {
		t.Fatal("expected thinking deltas, got 0")
	}

	// CRITICAL: must have text deltas (this was the original bug)
	if len(textDeltas) == 0 {
		t.Fatal("expected text deltas, got 0 — response text is missing!")
	}

	// Verify the full response text
	fullText := ""
	for _, d := range textDeltas {
		fullText += d
	}
	wantText := "Cześć! W czym mogę Ci dzisiaj pomóc?"
	if fullText != wantText {
		t.Errorf("text mismatch:\ngot:  %q\nwant: %q", fullText, wantText)
	}

	// Verify event ordering: thinking must complete before text starts
	thinkingEndIdx := -1
	firstTextIdx := -1
	for i, et := range eventTypes {
		if et == types.EventThinkingEnd && thinkingEndIdx == -1 {
			thinkingEndIdx = i
		}
		if et == types.EventTextDelta && firstTextIdx == -1 {
			firstTextIdx = i
		}
	}
	if thinkingEndIdx >= 0 && firstTextIdx >= 0 && thinkingEndIdx > firstTextIdx {
		t.Errorf("thinking_end (idx %d) should come before first text_delta (idx %d)", thinkingEndIdx, firstTextIdx)
	}
}

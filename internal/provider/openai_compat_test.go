package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

// helper: collect all events from parseStreamResponse
func collectStreamEvents(t *testing.T, body []byte) []types.StreamEvent {
	t.Helper()
	ctx := context.Background()
	p := &OpenAICompatProvider{}
	ch := make(chan types.StreamEvent, 64)
	go func() {
		defer close(ch)
		p.parseStreamResponse(ctx, ch, bytes.NewReader(body), "test-model", "test-provider")
	}()
	return collectStream(ch)
}

// helper: find the final message in events
func findDoneMessage(events []types.StreamEvent) *types.AgentMessage {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == types.EventDone && events[i].Message != nil {
			return events[i].Message
		}
	}
	return nil
}

// helper: extract all ToolCallBlocks from a message
func extractToolCallBlocks(msg *types.AgentMessage) []*types.ToolCallBlock {
	var blocks []*types.ToolCallBlock
	for _, cb := range msg.Content {
		if cb.Type == types.BlockToolCall && cb.ToolCall != nil {
			blocks = append(blocks, cb.ToolCall)
		}
	}
	return blocks
}

// helper: extract all BlockThinking text from a message
func extractThinkingBlocks(msg *types.AgentMessage) []string {
	var texts []string
	for _, block := range msg.Content {
		if block.Type == types.BlockThinking {
			texts = append(texts, block.Text)
		}
	}
	return texts
}

// helper: count events of a given type
func countEventType(events []types.StreamEvent, typ types.StreamEventType) int {
	n := 0
	for _, e := range events {
		if e.Type == typ {
			n++
		}
	}
	return n
}

// --- repairJSON tests ---

func TestRepairJSON_EscapesControlCharacters(t *testing.T) {
	// Input has raw control characters; repairJSON should escape them
	inputNewline := "{\"text\":\"hello\nworld\"}"
	expectedNewline := "{\"text\":\"hello\\nworld\"}"
	result := repairJSON(inputNewline)
	if result != expectedNewline {
		t.Errorf("repairJSON newline = %q, want %q", result, expectedNewline)
	}

	inputTab := "{\"text\":\"hello\tworld\"}"
	expectedTab := "{\"text\":\"hello\\tworld\"}"
	result = repairJSON(inputTab)
	if result != expectedTab {
		t.Errorf("repairJSON tab = %q, want %q", result, expectedTab)
	}

	// Already escaped — should pass through unchanged
	inputEscaped := `{"text":"hello\nworld"}`
	result = repairJSON(inputEscaped)
	if result != inputEscaped {
		t.Errorf("repairJSON already-escaped = %q, want %q", result, inputEscaped)
	}
}

func TestRepairJSON_FixesBadBackslash(t *testing.T) {
	input := `{"path":"C:\Users\test"}`
	result := repairJSON(input)
	if result == input {
		t.Error("repairJSON should have modified input with bad backslashes")
	}
	var m map[string]any
	if err := repairAndParse(result, &m); err != nil {
		t.Errorf("repaired JSON should be parseable: %v", err)
	}
}

func TestRepairJSON_PassThrough(t *testing.T) {
	valid := `{"path":"test.txt","mode":"read"}`
	result := repairJSON(valid)
	if result != valid {
		t.Errorf("repairJSON(%q) = %q, should be unchanged", valid, result)
	}
}

// --- parseArgs tests ---

func TestParseArgs_ValidJSON(t *testing.T) {
	input := `{"path":"test.txt","mode":"read"}`
	result, err := parseArgs(input)
	if err != nil {
		t.Fatalf("parseArgs(%q) error: %v", input, err)
	}
	if result["path"] != "test.txt" {
		t.Errorf("path = %v, want %q", result["path"], "test.txt")
	}
	if result["mode"] != "read" {
		t.Errorf("mode = %v, want %q", result["mode"], "read")
	}
}

func TestParseArgs_EmptyString(t *testing.T) {
	result, err := parseArgs("")
	if err != nil {
		t.Fatalf("parseArgs(\"\") error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("parseArgs(\"\") = %v, want empty map", result)
	}
}

func TestParseArgs_NestedJSON(t *testing.T) {
	input := `{"query":{"path":"src","pattern":"func"},"maxResults":10,"includeHidden":false}`
	result, err := parseArgs(input)
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	query, ok := result["query"].(map[string]any)
	if !ok {
		t.Fatalf("query should be map, got %T", result["query"])
	}
	if query["path"] != "src" {
		t.Errorf("query.path = %v, want %q", query["path"], "src")
	}
	if result["maxResults"].(float64) != 10 {
		t.Errorf("maxResults = %v, want 10", result["maxResults"])
	}
	if result["includeHidden"].(bool) != false {
		t.Errorf("includeHidden = %v, want false", result["includeHidden"])
	}
}

// --- SSE streaming tests ---
// NOTE: SSE events must be separated by blank lines.

func TestParseStreamResponse_TextOnly(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	hasStart := false
	hasDone := false
	for _, e := range events {
		if e.Type == types.EventStart {
			hasStart = true
		}
		if e.Type == types.EventDone {
			hasDone = true
		}
	}
	if !hasStart {
		t.Error("expected EventStart")
	}
	if !hasDone {
		t.Error("expected EventDone")
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}
	text := extractText(*msg)
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
}

func TestParseStreamResponse_SingleToolCall_ArgsInOneChunk(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"id\":\"call_1\",\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"test.txt\\\"}\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	var toolCallStart, toolCallEnd bool
	for _, e := range events {
		switch e.Type {
		case types.EventToolCallStart:
			toolCallStart = true
		case types.EventToolCallEnd:
			toolCallEnd = true
		}
	}
	if !toolCallStart {
		t.Error("expected EventToolCallStart")
	}
	if !toolCallEnd {
		t.Error("expected EventToolCallEnd")
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	var toolCallBlock *types.ToolCallBlock
	for _, block := range msg.Content {
		if block.Type == types.BlockToolCall {
			toolCallBlock = block.ToolCall
			break
		}
	}
	if toolCallBlock == nil {
		t.Fatal("no tool call block in message")
	}
	if toolCallBlock.Name != "read" {
		t.Errorf("tool name = %q, want %q", toolCallBlock.Name, "read")
	}
	if toolCallBlock.ID != "call_1" {
		t.Errorf("tool ID = %q, want %q", toolCallBlock.ID, "call_1")
	}
	if toolCallBlock.Arguments["path"] != "test.txt" {
		t.Errorf("arguments[path] = %v, want %q", toolCallBlock.Arguments["path"], "test.txt")
	}
}

func TestParseStreamResponse_SingleToolCall_ArgsSplitAcrossChunks(t *testing.T) {
	// Build from explicit JSON strings to avoid Go escaping issues
	chunk1 := `{"choices":[{"delta":{"role":"assistant","tool_calls":[{"id":"call_1","index":0,"type":"function","function":{"name":"read"}}]},"finish_reason":null}]}`
	chunk2 := `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\""}}]},"finish_reason":null}]}`
	chunk3 := `{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"test.txt\"}"}}]},"finish_reason":null}]}`
	chunk4 := `{"choices":[{"delta":{"content":""},"finish_reason":"tool_calls"}]}`
	body := []byte("data: " + chunk1 + "\n\ndata: " + chunk2 + "\n\ndata: " + chunk3 + "\n\ndata: " + chunk4 + "\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	var toolCallBlock *types.ToolCallBlock
	for _, block := range msg.Content {
		if block.Type == types.BlockToolCall {
			toolCallBlock = block.ToolCall
			break
		}
	}
	if toolCallBlock == nil {
		t.Fatal("no tool call block in message")
	}
	if toolCallBlock.Arguments["path"] != "test.txt" {
		t.Errorf("arguments[path] = %v, want %q", toolCallBlock.Arguments["path"], "test.txt")
	}

	toolCallEndCount := 0
	for _, e := range events {
		if e.Type == types.EventToolCallEnd {
			toolCallEndCount++
		}
	}
	if toolCallEndCount != 1 {
		t.Errorf("EventToolCallEnd count = %d, want 1", toolCallEndCount)
	}
}

func TestParseStreamResponse_MultipleToolCalls(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"id\":\"call_1\",\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"a.txt\\\"}\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_2\",\"index\":1,\"type\":\"function\",\"function\":{\"name\":\"grep\",\"arguments\":\"{\\\"pattern\\\":\\\"func\\\"}\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	toolCalls := extractToolCallBlocks(msg)
	if len(toolCalls) != 2 {
		t.Fatalf("tool call count = %d, want 2", len(toolCalls))
	}
	if toolCalls[0].Name != "read" || toolCalls[0].Arguments["path"] != "a.txt" {
		t.Errorf("first tool call wrong: %+v", toolCalls[0])
	}
	if toolCalls[1].Name != "grep" || toolCalls[1].Arguments["pattern"] != "func" {
		t.Errorf("second tool call wrong: %+v", toolCalls[1])
	}

	toolCallEndCount := 0
	for _, e := range events {
		if e.Type == types.EventToolCallEnd {
			toolCallEndCount++
		}
	}
	if toolCallEndCount != 2 {
		t.Errorf("EventToolCallEnd count = %d, want 2", toolCallEndCount)
	}
}

func TestParseStreamResponse_FinishReasonStop_WithToolCalls(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"id\":\"call_1\",\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"test.txt\\\"}\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	toolCalls := extractToolCallBlocks(msg)
	if len(toolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1 (should process even with stop reason)", len(toolCalls))
	}
	if toolCalls[0].Name != "read" {
		t.Errorf("tool name = %q, want %q", toolCalls[0].Name, "read")
	}

	toolCallEndCount := 0
	for _, e := range events {
		if e.Type == types.EventToolCallEnd {
			toolCallEndCount++
		}
	}
	if toolCallEndCount != 1 {
		t.Errorf("EventToolCallEnd count = %d, want 1", toolCallEndCount)
	}
}

func TestParseStreamResponse_NoToolCalls_TextResponse(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"I can help with that.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	toolCallStartCount := 0
	toolCallEndCount := 0
	for _, e := range events {
		if e.Type == types.EventToolCallStart {
			toolCallStartCount++
		}
		if e.Type == types.EventToolCallEnd {
			toolCallEndCount++
		}
	}
	if toolCallStartCount != 0 {
		t.Errorf("unexpected EventToolCallStart count = %d", toolCallStartCount)
	}
	if toolCallEndCount != 0 {
		t.Errorf("unexpected EventToolCallEnd count = %d", toolCallEndCount)
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}
	text := extractText(*msg)
	if text != "I can help with that." {
		t.Errorf("text = %q, want %q", text, "I can help with that.")
	}
}

func TestParseStreamResponse_MalformedJSONArguments(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"id\":\"call_1\",\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{bad json\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	toolCalls := extractToolCallBlocks(msg)
	if len(toolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].Name != "read" {
		t.Errorf("tool name = %q, want %q", toolCalls[0].Name, "read")
	}
	if _, ok := toolCalls[0].Arguments["_parse_error"]; !ok {
		t.Errorf("expected _parse_error key in arguments for malformed JSON, got: %v", toolCalls[0].Arguments)
	}
}

func TestParseStreamResponse_EmptyToolCallArgs(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"id\":\"call_1\",\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"list_tools\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	toolCalls := extractToolCallBlocks(msg)
	if len(toolCalls) != 1 {
		t.Fatalf("tool call count = %d, want 1", len(toolCalls))
	}
	if toolCalls[0].Name != "list_tools" {
		t.Errorf("tool name = %q, want %q", toolCalls[0].Name, "list_tools")
	}
	if len(toolCalls[0].Arguments) != 0 {
		t.Errorf("expected empty arguments, got: %v", toolCalls[0].Arguments)
	}
}

func TestParseStreamResponse_TextThenToolCall(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Let me check that.\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_1\",\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"test.txt\\\"}\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	if len(msg.Content) < 2 {
		t.Fatalf("expected at least 2 content blocks, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != types.BlockText {
		t.Errorf("first block should be text, got %s", msg.Content[0].Type)
	}
	if msg.Content[1].Type != types.BlockToolCall {
		t.Errorf("second block should be tool_call, got %s", msg.Content[1].Type)
	}
}

func TestParseStreamResponse_Usage(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":5,\"total_tokens\":15}}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	var usage *types.Usage
	for _, e := range events {
		if e.Type == types.EventDone && e.Usage != nil {
			usage = e.Usage
			break
		}
	}
	if usage == nil {
		t.Fatal("expected usage in EventDone")
	}
	if usage.Input != 10 {
		t.Errorf("usage.Input = %d, want 10", usage.Input)
	}
	if usage.Output != 5 {
		t.Errorf("usage.Output = %d, want 5", usage.Output)
	}
	if usage.TotalTokens != 15 {
		t.Errorf("usage.TotalTokens = %d, want 15", usage.TotalTokens)
	}
}

// --- Reasoning streaming tests ---

func TestParseStreamResponse_ReasoningBeforeContent(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"Let me think\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"reasoning\":\" about this\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	if countEventType(events, types.EventThinkingStart) != 1 {
		t.Error("expected 1 EventThinkingStart")
	}
	if countEventType(events, types.EventThinkingDelta) != 2 {
		t.Errorf("expected 2 EventThinkingDelta, got %d", countEventType(events, types.EventThinkingDelta))
	}
	if countEventType(events, types.EventThinkingEnd) != 1 {
		t.Error("expected 1 EventThinkingEnd")
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinking))
	}
	if thinking[0] != "Let me think about this" {
		t.Errorf("thinking = %q, want %q", thinking[0], "Let me think about this")
	}

	text := extractText(*msg)
	if text != "Hello" {
		t.Errorf("text = %q, want %q", text, "Hello")
	}

	// Verify block order: thinking before text
	if msg.Content[0].Type != types.BlockThinking {
		t.Errorf("first block should be thinking, got %s", msg.Content[0].Type)
	}
	if msg.Content[1].Type != types.BlockText {
		t.Errorf("second block should be text, got %s", msg.Content[1].Type)
	}
}

func TestParseStreamResponse_ReasoningInterleavedWithContent(t *testing.T) {
	// reasoning → content → reasoning → content (two thinking blocks)
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"thinking1\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"text1\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"reasoning\":\"thinking2\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"text2\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	if countEventType(events, types.EventThinkingStart) != 2 {
		t.Errorf("expected 2 EventThinkingStart, got %d", countEventType(events, types.EventThinkingStart))
	}
	if countEventType(events, types.EventThinkingDelta) != 2 {
		t.Errorf("expected 2 EventThinkingDelta, got %d", countEventType(events, types.EventThinkingDelta))
	}
	if countEventType(events, types.EventThinkingEnd) != 2 {
		t.Errorf("expected 2 EventThinkingEnd, got %d", countEventType(events, types.EventThinkingEnd))
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 2 {
		t.Fatalf("expected 2 thinking blocks, got %d", len(thinking))
	}
	if thinking[0] != "thinking1" {
		t.Errorf("thinking[0] = %q, want %q", thinking[0], "thinking1")
	}
	if thinking[1] != "thinking2" {
		t.Errorf("thinking[1] = %q, want %q", thinking[1], "thinking2")
	}

	// Block order: thinking, text, thinking, text
	expectedOrder := []types.ContentBlockType{
		types.BlockThinking, types.BlockText,
		types.BlockThinking, types.BlockText,
	}
	if len(msg.Content) != len(expectedOrder) {
		t.Fatalf("expected %d blocks, got %d", len(expectedOrder), len(msg.Content))
	}
	for i, want := range expectedOrder {
		if msg.Content[i].Type != want {
			t.Errorf("block[%d] = %s, want %s", i, msg.Content[i].Type, want)
		}
	}
}

func TestParseStreamResponse_ReasoningBeforeToolCall(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"need to read the file\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call_1\",\"index\":0,\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"test.txt\\\"}\"}}]},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	if countEventType(events, types.EventThinkingStart) != 1 {
		t.Error("expected 1 EventThinkingStart")
	}
	if countEventType(events, types.EventThinkingEnd) != 1 {
		t.Error("expected 1 EventThinkingEnd")
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	// Block order: thinking, tool_call
	if len(msg.Content) < 2 {
		t.Fatalf("expected at least 2 content blocks, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != types.BlockThinking {
		t.Errorf("first block should be thinking, got %s", msg.Content[0].Type)
	}
	if msg.Content[1].Type != types.BlockToolCall {
		t.Errorf("second block should be tool_call, got %s", msg.Content[1].Type)
	}

	thinking := extractThinkingBlocks(msg)
	if thinking[0] != "need to read the file" {
		t.Errorf("thinking = %q, want %q", thinking[0], "need to read the file")
	}
}

func TestParseStreamResponse_ReasoningOnly_NoContent(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"Just thinking, no answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinking))
	}
	if thinking[0] != "Just thinking, no answer" {
		t.Errorf("thinking = %q, want %q", thinking[0], "Just thinking, no answer")
	}

	text := extractText(*msg)
	if text != "" {
		t.Errorf("expected empty text, got %q", text)
	}
}

func TestParseStreamResponse_NoReasoning_Baseline(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello world\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	// No thinking events should be emitted
	if countEventType(events, types.EventThinkingStart) != 0 {
		t.Error("unexpected EventThinkingStart")
	}
	if countEventType(events, types.EventThinkingDelta) != 0 {
		t.Error("unexpected EventThinkingDelta")
	}
	if countEventType(events, types.EventThinkingEnd) != 0 {
		t.Error("unexpected EventThinkingEnd")
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}
	text := extractText(*msg)
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
}

func TestParseStreamResponse_EmptyReasoning_NoOp(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning\":\"\",\"content\":\"Hi\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	if countEventType(events, types.EventThinkingStart) != 0 {
		t.Error("unexpected EventThinkingStart for empty reasoning")
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}
	text := extractText(*msg)
	if text != "Hi" {
		t.Errorf("text = %q, want %q", text, "Hi")
	}
}

func TestParseStreamResponse_ReasoningContentField(t *testing.T) {
	// llama.cpp uses "reasoning_content" instead of "reasoning"
	body := []byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"llama thinking\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	if countEventType(events, types.EventThinkingStart) != 1 {
		t.Error("expected 1 EventThinkingStart")
	}
	if countEventType(events, types.EventThinkingDelta) != 1 {
		t.Errorf("expected 1 EventThinkingDelta, got %d", countEventType(events, types.EventThinkingDelta))
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinking))
	}
	if thinking[0] != "llama thinking" {
		t.Errorf("thinking = %q, want %q", thinking[0], "llama thinking")
	}
}

func TestParseStreamResponse_DuplicateReasoningFields_FirstWins(t *testing.T) {
	// chutes.ai returns both reasoning_content and reasoning with same content
	// We should use the first non-empty field (reasoning_content) only once
	chunk1 := `{"choices":[{"delta":{"reasoning_content":"thinking","reasoning":"thinking"},"finish_reason":null}]}`
	chunk2 := `{"choices":[{"delta":{"content":"Hi"},"finish_reason":"stop"}]}`
	body := []byte("data: " + chunk1 + "\n\ndata: " + chunk2 + "\n\ndata: [DONE]\n")
	events := collectStreamEvents(t, body)

	// Should only emit 1 thinking delta (from reasoning_content), not 2
	if countEventType(events, types.EventThinkingDelta) != 1 {
		t.Errorf("expected 1 EventThinkingDelta (first field wins), got %d", countEventType(events, types.EventThinkingDelta))
	}

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinking))
	}
	if thinking[0] != "thinking" {
		t.Errorf("thinking = %q, want %q", thinking[0], "thinking")
	}
}

// --- Streaming accumulation tests ---

func TestParseStreamResponse_SingleTextBlock_MultipleDeltas(t *testing.T) {
	// 5 text deltas should produce exactly 1 text block
	body := []byte(
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\" \"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"world\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"!\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\" How are you?\"},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n",
	)
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	textBlocks := 0
	for _, block := range msg.Content {
		if block.Type == types.BlockText {
			textBlocks++
		}
	}
	if textBlocks != 1 {
		t.Errorf("expected 1 text block, got %d", textBlocks)
	}

	text := extractText(*msg)
	if text != "Hello world! How are you?" {
		t.Errorf("text = %q, want %q", text, "Hello world! How are you?")
	}

	// Verify event lifecycle
	if countEventType(events, types.EventTextStart) != 1 {
		t.Errorf("expected 1 EventTextStart, got %d", countEventType(events, types.EventTextStart))
	}
	if countEventType(events, types.EventTextDelta) != 5 {
		t.Errorf("expected 5 EventTextDelta, got %d", countEventType(events, types.EventTextDelta))
	}
	if countEventType(events, types.EventTextEnd) != 1 {
		t.Errorf("expected 1 EventTextEnd, got %d", countEventType(events, types.EventTextEnd))
	}
}

func TestParseStreamResponse_SingleThinkingBlock_MultipleDeltas(t *testing.T) {
	// 3 reasoning deltas should produce exactly 1 thinking block
	body := []byte(
		"data: {\"choices\":[{\"delta\":{\"reasoning\":\"Let me\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"reasoning\":\" think\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"reasoning\":\" about this\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"Done\"},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n",
	)
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	thinking := extractThinkingBlocks(msg)
	if len(thinking) != 1 {
		t.Fatalf("expected 1 thinking block, got %d", len(thinking))
	}
	if thinking[0] != "Let me think about this" {
		t.Errorf("thinking = %q, want %q", thinking[0], "Let me think about this")
	}

	if countEventType(events, types.EventThinkingStart) != 1 {
		t.Errorf("expected 1 EventThinkingStart, got %d", countEventType(events, types.EventThinkingStart))
	}
	if countEventType(events, types.EventThinkingDelta) != 3 {
		t.Errorf("expected 3 EventThinkingDelta, got %d", countEventType(events, types.EventThinkingDelta))
	}
	if countEventType(events, types.EventThinkingEnd) != 1 {
		t.Errorf("expected 1 EventThinkingEnd, got %d", countEventType(events, types.EventThinkingEnd))
	}
}

func TestParseStreamResponse_InterleavedTextThinking_TextAccumulates(t *testing.T) {
	// reasoning → content → reasoning → content
	// Should produce: 2 thinking blocks, 2 text blocks
	body := []byte(
		"data: {\"choices\":[{\"delta\":{\"reasoning\":\"thinking1\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"text1a\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"text1b\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"reasoning\":\"thinking2\"},\"finish_reason\":null}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"text2\"},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n",
	)
	events := collectStreamEvents(t, body)

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	// Verify block count and order
	expectedOrder := []types.ContentBlockType{
		types.BlockThinking, types.BlockText,
		types.BlockThinking, types.BlockText,
	}
	if len(msg.Content) != len(expectedOrder) {
		t.Fatalf("expected %d blocks, got %d", len(expectedOrder), len(msg.Content))
	}
	for i, want := range expectedOrder {
		if msg.Content[i].Type != want {
			t.Errorf("block[%d] = %s, want %s", i, msg.Content[i].Type, want)
		}
	}

	// Verify text accumulation
	if msg.Content[1].Text != "text1atext1b" {
		t.Errorf("text block 1 = %q, want %q", msg.Content[1].Text, "text1atext1b")
	}
	if msg.Content[3].Text != "text2" {
		t.Errorf("text block 2 = %q, want %q", msg.Content[3].Text, "text2")
	}

	// Verify event counts
	if countEventType(events, types.EventTextStart) != 2 {
		t.Errorf("expected 2 EventTextStart, got %d", countEventType(events, types.EventTextStart))
	}
	if countEventType(events, types.EventTextEnd) != 2 {
		t.Errorf("expected 2 EventTextEnd, got %d", countEventType(events, types.EventTextEnd))
	}
	if countEventType(events, types.EventThinkingStart) != 2 {
		t.Errorf("expected 2 EventThinkingStart, got %d", countEventType(events, types.EventThinkingStart))
	}
	if countEventType(events, types.EventThinkingEnd) != 2 {
		t.Errorf("expected 2 EventThinkingEnd, got %d", countEventType(events, types.EventThinkingEnd))
	}
}

func TestParseStreamResponse_LargeContent_NoQuadraticBehavior(t *testing.T) {
	// Simulate 1000 text deltas — should produce 1 text block, not 1000
	var body string
	for i := 0; i < 1000; i++ {
		body += "data: {\"choices\":[{\"delta\":{\"content\":\"x\"},\"finish_reason\":null}]}\n\n"
	}
	body += "data: {\"choices\":[{\"delta\":{\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"

	events := collectStreamEvents(t, []byte(body))

	msg := findDoneMessage(events)
	if msg == nil {
		t.Fatal("no message in EventDone")
	}

	textBlocks := 0
	for _, block := range msg.Content {
		if block.Type == types.BlockText {
			textBlocks++
		}
	}
	if textBlocks != 1 {
		t.Errorf("expected 1 text block for 1000 deltas, got %d", textBlocks)
	}

	text := extractText(*msg)
	if len(text) != 1000 {
		t.Errorf("text length = %d, want 1000", len(text))
	}

	// Verify event counts
	if countEventType(events, types.EventTextStart) != 1 {
		t.Errorf("expected 1 EventTextStart, got %d", countEventType(events, types.EventTextStart))
	}
	if countEventType(events, types.EventTextDelta) != 1000 {
		t.Errorf("expected 1000 EventTextDelta, got %d", countEventType(events, types.EventTextDelta))
	}
	if countEventType(events, types.EventTextEnd) != 1 {
		t.Errorf("expected 1 EventTextEnd, got %d", countEventType(events, types.EventTextEnd))
	}
}

// --- streaming error handling tests ---

func TestParseStreamResponse_SSEErrorEvent(t *testing.T) {
	body := []byte("event: error\ndata: Rate limit exceeded. Please retry after 60 seconds.\n\n")

	ctx := context.Background()
	p := &OpenAICompatProvider{}
	ch := make(chan types.StreamEvent, 64)

	go func() {
		defer close(ch)
		p.parseStreamResponse(ctx, ch, bytes.NewReader(body), "test-model", "opencode-zen")
	}()

	events := collectStream(ch)

	var errorEvent *types.StreamEvent
	for i := range events {
		if events[i].Type == types.EventError {
			errorEvent = &events[i]
			break
		}
	}

	if errorEvent == nil {
		t.Fatal("expected EventError for SSE error event")
	}
	if !strings.Contains(errorEvent.Error, "opencode-zen") {
		t.Errorf("error should contain provider name, got: %s", errorEvent.Error)
	}
	if !strings.Contains(errorEvent.Error, "Rate limit exceeded") {
		t.Errorf("error should contain SSE error message, got: %s", errorEvent.Error)
	}
}

// --- DeepSeek-specific handling tests ---

func TestOpenAICompatProvider_DeepSeek_ReasoningContentInAssistantMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		messages := body["messages"].([]any)
		// Check that assistant messages have reasoning_content field
		for _, m := range messages {
			msg := m.(map[string]any)
			if msg["role"] == "assistant" {
				if _, ok := msg["reasoning_content"]; !ok {
					t.Error("assistant message should have reasoning_content field for DeepSeek")
				}
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	p := NewOpenAICompatProvider("test-key", OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "test-provider",
	})

	model := types.Model{ID: "deepseek-v4-flash-free", BaseURL: server.URL}
	messages := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hi"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: types.BlockThinking, Text: "Let me think"},
			{Type: types.BlockText, Text: "Hello!"},
		}},
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Bye"}}},
	}

	ctx := context.Background()
	ch := p.Stream(ctx, model, messages, nil, types.StreamOptions{})
	collectStream(ch)
}

func TestOpenAICompatProvider_DeepSeek_EmptyReasoningForNoThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		messages := body["messages"].([]any)
		for _, m := range messages {
			msg := m.(map[string]any)
			if msg["role"] == "assistant" {
				rc, ok := msg["reasoning_content"]
				if !ok {
					t.Error("assistant message should have reasoning_content field for DeepSeek")
				} else if rc != "" {
					t.Errorf("reasoning_content should be empty for assistant without thinking, got: %v", rc)
				}
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	p := NewOpenAICompatProvider("test-key", OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "test-provider",
	})

	model := types.Model{ID: "deepseek-chat", BaseURL: server.URL}
	messages := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hi"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello!"}}},
	}

	ctx := context.Background()
	ch := p.Stream(ctx, model, messages, nil, types.StreamOptions{})
	collectStream(ch)
}

func TestOpenAICompatProvider_NonDeepSeek_NoReasoningContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		messages := body["messages"].([]any)
		for _, m := range messages {
			msg := m.(map[string]any)
			if msg["role"] == "assistant" {
				if _, ok := msg["reasoning_content"]; ok {
					t.Error("non-DeepSeek assistant messages should NOT have reasoning_content field")
				}
			}
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n"))
	}))
	defer server.Close()

	p := NewOpenAICompatProvider("test-key", OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "test-provider",
	})

	model := types.Model{ID: "qwen3.6-plus", BaseURL: server.URL}
	messages := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hi"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{
			{Type: types.BlockThinking, Text: "Let me think"},
			{Type: types.BlockText, Text: "Hello!"},
		}},
	}

	ctx := context.Background()
	ch := p.Stream(ctx, model, messages, nil, types.StreamOptions{})
	collectStream(ch)
}

func TestParseStreamResponse_JSONParseError_WithErrorContent(t *testing.T) {
	body := []byte("data: {\"error\": \"Quota exceeded\", \"message\": \"Your quota has been exceeded\"}\n\n")
	events := collectStreamEvents(t, body)

	hasError := false
	for _, e := range events {
		if e.Type == types.EventError {
			hasError = true
		}
	}
	if !hasError {
		t.Fatal("expected EventError when JSON parse fails and data contains 'error'")
	}
}

func TestParseStreamResponse_JSONParseError_NoErrorContent(t *testing.T) {
	body := []byte("data: this is not valid json\n\n")
	events := collectStreamEvents(t, body)

	hasError := false
	for _, e := range events {
		if e.Type == types.EventError {
			hasError = true
		}
	}
	if hasError {
		t.Error("should not emit EventError for non-error JSON parse failures")
	}
}

func TestParseStreamResponse_ContextCancel_StopsStreaming(t *testing.T) {
	// Verify that parseStreamResponse returns when context is cancelled
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")

	ctx, cancel := context.WithCancel(context.Background())
	p := &OpenAICompatProvider{}
	ch := make(chan types.StreamEvent, 64)

	done := make(chan struct{})
	go func() {
		defer close(ch)
		defer close(done)
		p.parseStreamResponse(ctx, ch, bytes.NewReader(body), "test-model", "test-provider")
	}()

	// Cancel after a brief delay
	time.Sleep(5 * time.Millisecond)
	cancel()

	// Wait for goroutine to finish (with timeout)
	select {
	case <-done:
		// Success - goroutine returned
	case <-time.After(100 * time.Millisecond):
		t.Fatal("parseStreamResponse did not return after context cancellation")
	}
}

func TestOpenAICompatProvider_Stream_AuthFailure(t *testing.T) {
	// Simulate Zen API auth failure response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "Invalid API key provided"}}`))
	}))
	defer server.Close()

	p := NewOpenAICompatProvider("invalid-key", OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "opencode-zen",
	})

	ctx := context.Background()
	model := types.Model{ID: "test-model", BaseURL: server.URL}
	ch := p.Stream(ctx, model, nil, nil, types.StreamOptions{})

	events := collectStream(ch)

	var errorEvent *types.StreamEvent
	for i := range events {
		if events[i].Type == types.EventError {
			errorEvent = &events[i]
			break
		}
	}

	if errorEvent == nil {
		t.Fatal("expected EventError for auth failure")
	}
	if !strings.Contains(errorEvent.Error, "opencode-zen") {
		t.Errorf("error should contain provider name, got: %s", errorEvent.Error)
	}
}

func TestOpenAICompatProvider_Stream_QuotaExceeded(t *testing.T) {
	// Simulate Zen API quota exceeded response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": {"message": "Weekly quota exceeded. Resets on 2026-05-21"}}`))
	}))
	defer server.Close()

	p := NewOpenAICompatProvider("valid-key", OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "opencode-zen",
	})

	ctx := context.Background()
	model := types.Model{ID: "test-model", BaseURL: server.URL}
	ch := p.Stream(ctx, model, nil, nil, types.StreamOptions{})

	events := collectStream(ch)

	var errorEvent *types.StreamEvent
	for i := range events {
		if events[i].Type == types.EventError {
			errorEvent = &events[i]
			break
		}
	}

	if errorEvent == nil {
		t.Fatal("expected EventError for quota exceeded")
	}
	if !strings.Contains(errorEvent.Error, "Weekly quota exceeded") {
		t.Errorf("error should contain provider message, got: %s", errorEvent.Error)
	}
}

func TestOpenAICompatProvider_Stream_ModelUnavailable(t *testing.T) {
	// Simulate Zen API model unavailable response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error": {"message": "Model test-model is not available for your account"}}`))
	}))
	defer server.Close()

	p := NewOpenAICompatProvider("valid-key", OpenAICompatConfig{
		BaseURL:      server.URL,
		ProviderName: "opencode-zen",
	})

	ctx := context.Background()
	model := types.Model{ID: "test-model", BaseURL: server.URL}
	ch := p.Stream(ctx, model, nil, nil, types.StreamOptions{})

	events := collectStream(ch)

	var errorEvent *types.StreamEvent
	for i := range events {
		if events[i].Type == types.EventError {
			errorEvent = &events[i]
			break
		}
	}

	if errorEvent == nil {
		t.Fatal("expected EventError for model unavailable")
	}
	if !strings.Contains(errorEvent.Error, "opencode-zen") {
		t.Errorf("error should contain provider name, got: %s", errorEvent.Error)
	}
}

func TestOpenAICompatProvider_Stream_SSEErrorEvent(t *testing.T) {
	// Simulate Zen API SSE error event
	sseResponse := "event: error\ndata: Rate limit exceeded. Please retry after 60 seconds.\n\n"

	ctx := context.Background()
	p := &OpenAICompatProvider{}
	ch := make(chan types.StreamEvent, 64)

	go func() {
		defer close(ch)
		p.parseStreamResponse(ctx, ch, bytes.NewReader([]byte(sseResponse)), "test-model", "opencode-zen")
	}()

	events := collectStream(ch)

	var errorEvent *types.StreamEvent
	for i := range events {
		if events[i].Type == types.EventError {
			errorEvent = &events[i]
			break
		}
	}

	if errorEvent == nil {
		t.Fatal("expected EventError for SSE error event")
	}
	if !strings.Contains(errorEvent.Error, "opencode-zen") {
		t.Errorf("error should contain provider name, got: %s", errorEvent.Error)
	}
	if !strings.Contains(errorEvent.Error, "Rate limit exceeded") {
		t.Errorf("error should contain SSE error message, got: %s", errorEvent.Error)
	}
}

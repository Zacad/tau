package session

import (
	"testing"

	"github.com/adam/tau/internal/types"
)

func makeTextMsg(id, text string) types.AgentMessage {
	return types.AgentMessage{
		ID:        id,
		Role:      types.RoleUser,
		Content:   []types.ContentBlock{{Type: types.BlockText, Text: text}},
	}
}

func makeAssistantMsg(id, text string) types.AgentMessage {
	return types.AgentMessage{
		ID:        id,
		Role:      types.RoleAssistant,
		Content:   []types.ContentBlock{{Type: types.BlockText, Text: text}},
	}
}

func makeAssistantWithToolCalls(id, toolName string) types.AgentMessage {
	return types.AgentMessage{
		ID:   id,
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "I'll use a tool"},
			{
				Type: types.BlockToolCall,
				ToolCall: &types.ToolCallBlock{
					ID:   "tc-001",
					Name: toolName,
				},
			},
		},
	}
}

func makeToolResultMsg(id, output string) types.AgentMessage {
	return types.AgentMessage{
		ID:      id,
		Role:    types.RoleToolResult,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: output}},
	}
}

func makeThinkingMsg(id, text string) types.AgentMessage {
	return types.AgentMessage{
		ID:        id,
		Role:      types.RoleAssistant,
		Content:   []types.ContentBlock{{Type: types.BlockThinking, Text: text}},
	}
}

// --- EstimateTokens ---

func TestEstimateTokens_Text(t *testing.T) {
	// 4 chars → 1 token (chars/4)
	msg := makeTextMsg("1", "abcd")
	tokens := EstimateTokens(msg)
	if tokens != 1 {
		t.Errorf("4 text chars → expected 1 token, got %d", tokens)
	}

	// 100 chars → 25 tokens
	msg = makeTextMsg("1", string(make([]byte, 100)))
	tokens = EstimateTokens(msg)
	if tokens != 25 {
		t.Errorf("100 text chars → expected 25 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_ToolResult(t *testing.T) {
	// Tool results use chars/3
	msg := makeToolResultMsg("1", "abcdef") // 6 chars
	tokens := EstimateTokens(msg)
	// 6/3 = 2 tokens from tool result heuristic
	if tokens != 2 {
		t.Errorf("6 tool result chars → expected 2 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_Thinking(t *testing.T) {
	// Thinking uses chars/3.5
	msg := makeThinkingMsg("1", "abcdefg") // 7 chars
	tokens := EstimateTokens(msg)
	// 7/3.5 = 2 tokens
	if tokens != 2 {
		t.Errorf("7 thinking chars → expected 2 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_MixedContent(t *testing.T) {
	msg := types.AgentMessage{
		ID:   "1",
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "abcd"},          // 1 token
			{Type: types.BlockThinking, Text: "abcdefg"},    // 2 tokens
		},
	}
	tokens := EstimateTokens(msg)
	if tokens != 3 {
		t.Errorf("mixed content → expected 3 tokens, got %d", tokens)
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	msg := types.AgentMessage{ID: "1", Role: types.RoleAssistant}
	tokens := EstimateTokens(msg)
	if tokens != 0 {
		t.Errorf("empty message → expected 0 tokens, got %d", tokens)
	}
}

// --- ShouldCompact ---

func TestShouldCompact_True(t *testing.T) {
	// Create messages that exceed contextWindow - reserveTokens
	// Each message: 2000 chars = 500 tokens; 100 messages = 50,000 tokens
	// Threshold = 50,000 - 16,384 = 33,616 → compaction needed
	var msgs []types.AgentMessage
	for i := 0; i < 100; i++ {
		msgs = append(msgs, makeTextMsg(string(rune('a'+i%26)), string(make([]byte, 2000))))
	}
	if !ShouldCompact(msgs, 50000, ReserveTokens) {
		t.Error("expected ShouldCompact to return true for large messages")
	}
}

func TestShouldCompact_False(t *testing.T) {
	msgs := []types.AgentMessage{
		makeTextMsg("1", "hello"), // ~1 token
	}
	if ShouldCompact(msgs, 50000, ReserveTokens) {
		t.Error("expected ShouldCompact to return false for small messages")
	}
}

func TestShouldCompact_ZeroContextWindow(t *testing.T) {
	msgs := []types.AgentMessage{makeTextMsg("1", "hello")}
	if ShouldCompact(msgs, 0, ReserveTokens) {
		t.Error("expected false for unknown context window")
	}
}

func TestShouldCompact_EmptyMessages(t *testing.T) {
	if ShouldCompact(nil, 50000, ReserveTokens) {
		t.Error("expected false for empty messages")
	}
}

// --- FindCutPoint ---

func TestFindCutPoint_FitsInBudget(t *testing.T) {
	msgs := []types.AgentMessage{
		makeTextMsg("1", "hello"), // ~1 token
		makeTextMsg("2", "world"), // ~1 token
	}
	cut := FindCutPoint(msgs, 100) // budget of 100 tokens
	if cut != -1 {
		t.Errorf("expected -1 (all fit), got %d", cut)
	}
}

func TestFindCutPoint_ExceedsBudget(t *testing.T) {
	// Create messages with known token counts
	// 1000 chars = 250 tokens each
	large := makeTextMsg("1", string(make([]byte, 1000))) // 250 tokens
	small := makeTextMsg("2", "hello")                     // 1 token

	msgs := []types.AgentMessage{large, small}

	// Budget of 50 tokens — only the small message fits
	cut := FindCutPoint(msgs, 50)
	if cut != 0 {
		t.Errorf("expected cut at index 0, got %d", cut)
	}
}

func TestFindCutPoint_EmptyMessages(t *testing.T) {
	cut := FindCutPoint(nil, 100)
	if cut != -1 {
		t.Errorf("expected -1 for empty messages, got %d", cut)
	}
}

// --- AdjustToTurnBoundary ---

func TestAdjustToTurnBoundary_BeforeUserMessage(t *testing.T) {
	msgs := []types.AgentMessage{
		makeTextMsg("u1", "hello"),
		makeAssistantMsg("a1", "hi"),
		makeTextMsg("u2", "follow up"),
	}
	// Cut at index 1 (assistant message) — should advance to index 2 (user message)
	adjusted := AdjustToTurnBoundary(msgs, 1)
	if adjusted != 2 {
		t.Errorf("expected index 2 (user message), got %d", adjusted)
	}
}

func TestAdjustToTurnBoundary_AfterToolResults(t *testing.T) {
	msgs := []types.AgentMessage{
		makeTextMsg("u1", "read this file"),
		makeAssistantWithToolCalls("a1", "read"),
		makeToolResultMsg("t1", "file contents here"),
		makeTextMsg("u2", "now edit it"),
	}
	// Cut at index 2 (tool result) — should advance past all tool results to index 3
	adjusted := AdjustToTurnBoundary(msgs, 2)
	if adjusted != 3 {
		t.Errorf("expected index 3 (user message), got %d", adjusted)
	}
}

func TestAdjustToTurnBoundary_MidToolCalls_AdvancesPastAllResults(t *testing.T) {
	msgs := []types.AgentMessage{
		makeTextMsg("u1", "read two files"),
		makeAssistantWithToolCalls("a1", "read"),
		makeToolResultMsg("t1", "file 1 contents"),
		makeToolResultMsg("t2", "file 2 contents"),
		makeTextMsg("u2", "now compare"),
	}
	// Cut at index 1 (assistant with tool calls) — should skip past tool results to index 4
	adjusted := AdjustToTurnBoundary(msgs, 1)
	if adjusted != 4 {
		t.Errorf("expected index 4 (user message after tool results), got %d", adjusted)
	}
}

func TestAdjustToTurnBoundary_NoValidBoundary(t *testing.T) {
	msgs := []types.AgentMessage{
		makeTextMsg("u1", "hello"),
		makeAssistantMsg("a1", "hi"),
	}
	// Cut at index 1 (last message) — no valid boundary after it
	adjusted := AdjustToTurnBoundary(msgs, 1)
	if adjusted != -1 {
		t.Errorf("expected -1 (no valid boundary), got %d", adjusted)
	}
}

func TestAdjustToTurnBoundary_NegativeIndex(t *testing.T) {
	msgs := []types.AgentMessage{makeTextMsg("u1", "hello")}
	adjusted := AdjustToTurnBoundary(msgs, -1)
	if adjusted != -1 {
		t.Errorf("expected -1 for negative index, got %d", adjusted)
	}
}

func TestAdjustToTurnBoundary_OutOfRange(t *testing.T) {
	msgs := []types.AgentMessage{makeTextMsg("u1", "hello")}
	adjusted := AdjustToTurnBoundary(msgs, 10)
	if adjusted != -1 {
		t.Errorf("expected -1 for out-of-range index, got %d", adjusted)
	}
}

func TestAdjustToTurnBoundary_AlreadyAtUserMessage(t *testing.T) {
	msgs := []types.AgentMessage{
		makeTextMsg("u1", "hello"),
		makeAssistantMsg("a1", "hi"),
		makeTextMsg("u2", "follow up"),
	}
	// Cut at index 0 which is already a user message
	adjusted := AdjustToTurnBoundary(msgs, 0)
	if adjusted != 0 {
		t.Errorf("expected index 0 (already user message), got %d", adjusted)
	}
}

func TestAdjustToTurnBoundary_AssistantWithoutToolCalls(t *testing.T) {
	msgs := []types.AgentMessage{
		makeTextMsg("u1", "tell me a joke"),
		makeAssistantMsg("a1", "Why did the chicken cross the road?"),
		makeTextMsg("u2", "haha"),
	}
	// Cut at index 1 (assistant without tool calls) — safe to cut at index 2
	adjusted := AdjustToTurnBoundary(msgs, 1)
	if adjusted != 2 {
		t.Errorf("expected index 2 (user message), got %d", adjusted)
	}
}

// --- BuildCompactionEntry ---

func TestBuildCompactionEntry(t *testing.T) {
	entry := BuildCompactionEntry("msg-005", 50000, "## Summary\nThis is a test summary")

	if entry.Type != types.EntryCompaction {
		t.Errorf("entry type mismatch: got %s, want %s", entry.Type, types.EntryCompaction)
	}
	if entry.ID == "" {
		t.Error("entry ID should not be empty")
	}

	var data CompactionData
	if err := entry.UnmarshalData(&data); err != nil {
		t.Fatalf("UnmarshalData: %v", err)
	}
	if data.FirstKeptEntryID != "msg-005" {
		t.Errorf("firstKeptEntryID mismatch: got %s", data.FirstKeptEntryID)
	}
	if data.TokensBefore != 50000 {
		t.Errorf("tokensBefore mismatch: got %d", data.TokensBefore)
	}
	if data.Summary != "## Summary\nThis is a test summary" {
		t.Errorf("summary mismatch: got %s", data.Summary)
	}
}

// --- PlanCompaction ---

func TestPlanCompaction_NoCompactionNeeded(t *testing.T) {
	msgs := []types.AgentMessage{
		makeTextMsg("u1", "hello"),
		makeAssistantMsg("a1", "hi there"),
	}

	plan := PlanCompaction(msgs, 50000)
	if plan.ShouldCompact {
		t.Error("expected no compaction needed for small messages")
	}
}

func TestPlanCompaction_CompactionNeeded(t *testing.T) {
	// Create enough messages to trigger compaction
	// With ReserveTokens=16384 and contextWindow=50000, threshold=33616
	// Each message: 2000 bytes = 500 tokens; 100 messages = 50,000 tokens
	var msgs []types.AgentMessage
	for i := 0; i < 100; i++ {
		msgs = append(msgs, makeTextMsg(string(rune('a'+i%26)), string(make([]byte, 2000))))
	}

	plan := PlanCompaction(msgs, 50000)
	if !plan.ShouldCompact {
		t.Error("expected compaction needed for large message set")
	}
	if plan.CutIndex <= 0 {
		t.Errorf("expected cut index > 0, got %d", plan.CutIndex)
	}
	if plan.FirstKeptID == "" {
		t.Error("expected firstKeptID to be set")
	}
}

func TestPlanCompaction_EmptyMessages(t *testing.T) {
	plan := PlanCompaction(nil, 50000)
	if plan.ShouldCompact {
		t.Error("expected no compaction for empty messages")
	}
}

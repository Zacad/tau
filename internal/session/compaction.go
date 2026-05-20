package session

import (
	"github.com/adam/tau/internal/types"
)

// Compaction settings matching ARCHITECTURE.md §7.5.
const (
	// ReserveTokens is the buffer kept for tool results + new turn.
	ReserveTokens = 16384
	// KeepRecentTokens is the recent context kept without summarization.
	KeepRecentTokens = 20000
)

// EstimateTokens estimates token count for a single message using per-type heuristics:
//   - Text:       chars / 4
//   - Tool calls: chars / 3  (more token-dense due to structured formatting)
//   - Thinking:   chars / 3.5
//   - Tool results: chars / 3
func EstimateTokens(msg types.AgentMessage) int {
	// For tool_result messages, use the tool heuristic on all text content
	if msg.Role == types.RoleToolResult {
		var textLen int
		for _, block := range msg.Content {
			textLen += len(block.Text)
		}
		return textLen / 3
	}

	var total int
	for _, block := range msg.Content {
		switch block.Type {
		case types.BlockText:
			total += len(block.Text) / 4
		case types.BlockToolCall:
			// Estimate from tool call name + ID
			nameChars := len(block.ToolCall.Name)
			argsChars := len(block.ToolCall.ID) // rough proxy
			total += (nameChars + argsChars) / 3
		case types.BlockThinking:
			total += int(float64(len(block.Text)) / 3.5)
		}
	}
	return total
}

// ShouldCompact checks if total estimated tokens exceed the compaction threshold.
// threshold = contextWindow - reserveTokens.
// Returns false if contextWindow is 0 (unknown) or messages fit within threshold.
func ShouldCompact(messages []types.AgentMessage, contextWindow int, reserveTokens int) bool {
	if contextWindow == 0 {
		return false
	}
	threshold := contextWindow - reserveTokens
	if threshold <= 0 {
		return false
	}
	total := 0
	for _, msg := range messages {
		total += EstimateTokens(msg)
	}
	return total > threshold
}

// FindCutPoint walks backwards from the end of messages, accumulating token
// estimates. Returns the index where keeping messages from that index onward
// would fit within budget. Returns -1 if all messages fit within budget.
//
// The budget is typically KeepRecentTokens — the amount of recent context
// to keep without summarization.
func FindCutPoint(messages []types.AgentMessage, budget int) int {
	if len(messages) == 0 {
		return -1
	}

	total := 0
	for i := len(messages) - 1; i >= 0; i-- {
		tokens := EstimateTokens(messages[i])
		total += tokens
		if total > budget {
			// messages[i] doesn't fit; cut point is at i
			// but we need to constrain to turn boundary
			return i
		}
	}
	// All messages fit within budget
	return -1
}

// AdjustToTurnBoundary adjusts cutIndex to never split a tool call from its result.
//
// Turn boundary rules:
//   - Safe to cut before a user message (start of new turn)
//   - Safe to cut after a tool_result that follows an assistant message with tool calls
//   - Never cut between an assistant message with tool calls and its tool results
//   - Never cut mid-tool-call-batch (between tool results)
//
// Returns adjusted index, or -1 if no valid cut point exists.
func AdjustToTurnBoundary(messages []types.AgentMessage, cutIndex int) int {
	if cutIndex < 0 || cutIndex >= len(messages) {
		return -1
	}

	// Walk forward from cutIndex to find the next safe boundary
	for i := cutIndex; i < len(messages); i++ {
		msg := messages[i]

		// Safe: cutting before a user message
		if msg.Role == types.RoleUser {
			return i
		}

		// If this is a tool_result, check if we're at the end of a tool batch
		if msg.Role == types.RoleToolResult {
			// Check if the next message is NOT a tool_result
			// (meaning we're at the end of the tool result batch)
			if i+1 >= len(messages) || messages[i+1].Role != types.RoleToolResult {
				// Safe: end of tool result batch, cut before next message (i+1)
				if i+1 < len(messages) {
					return i + 1
				}
				// Last message is a tool result — no valid cut point after it
				return -1
			}
			// Still in the middle of tool results — continue
			continue
		}

		// If this is an assistant message with tool calls, we must skip past all results
		if msg.Role == types.RoleAssistant {
			hasToolCalls := false
			for _, block := range msg.Content {
				if block.Type == types.BlockToolCall {
					hasToolCalls = true
					break
				}
			}
			if hasToolCalls {
				// Skip past all following tool results
				j := i + 1
				for j < len(messages) && messages[j].Role == types.RoleToolResult {
					j++
				}
				if j < len(messages) {
					return j
				}
				return -1
			}
			// Assistant message without tool calls — safe to cut before next message
			if i+1 < len(messages) {
				return i + 1
			}
			return -1
		}
	}

	return -1
}

// BuildCompactionEntry creates a compaction SessionEntry.
//
// Parameters:
//   - firstKeptEntryID: ID of the first message kept after compaction
//   - tokensBefore: estimated token count before compaction
//   - summary: structured summary text (provided by caller/SDK)
//
// This is a pure function — it does NOT call any provider.
func BuildCompactionEntry(firstKeptEntryID string, tokensBefore int, summary string) types.SessionEntry {
	data, _ := MarshalEntryData(types.EntryCompaction, CompactionData{
		FirstKeptEntryID: firstKeptEntryID,
		TokensBefore:     tokensBefore,
		Summary:          summary,
	})

	return types.SessionEntry{
		Type:      types.EntryCompaction,
		ID:        GenerateID(),
		Timestamp: now(),
		Data:      data,
	}
}

// CompactionPlan describes what should be compacted.
type CompactionPlan struct {
	// ShouldCompact is true if compaction is needed.
	ShouldCompact bool
	// CutIndex is the message index where compaction should start (after turn boundary adjustment).
	CutIndex int
	// TokensTotal is the total estimated token count.
	TokensTotal int
	// FirstKeptID is the ID of the first message to keep (after compaction).
	FirstKeptID string
}

// PlanCompaction analyzes messages and determines if/where compaction should occur.
// Returns a CompactionPlan with all details.
//
// This is a pure function — no I/O, no provider calls.
func PlanCompaction(messages []types.AgentMessage, contextWindow int) CompactionPlan {
	totalTokens := 0
	for _, msg := range messages {
		totalTokens += EstimateTokens(msg)
	}

	plan := CompactionPlan{
		TokensTotal: totalTokens,
		CutIndex:    -1,
	}

	if !ShouldCompact(messages, contextWindow, ReserveTokens) {
		return plan
	}

	// Find cut point based on keepRecentTokens budget
	cutIndex := FindCutPoint(messages, KeepRecentTokens)
	if cutIndex < 0 {
		return plan
	}

	// Adjust to turn boundary
	adjustedIndex := AdjustToTurnBoundary(messages, cutIndex)
	if adjustedIndex < 0 {
		// No valid turn boundary found — can't compact safely
		return plan
	}

	plan.ShouldCompact = true
	plan.CutIndex = adjustedIndex
	if adjustedIndex < len(messages) {
		plan.FirstKeptID = messages[adjustedIndex].ID
	}

	return plan
}

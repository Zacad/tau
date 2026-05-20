package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/adam/tau/internal/tools"
	"github.com/adam/tau/internal/types"
)

// run executes the agent loop state machine.
//
// State transitions:
//
//	IDLE → STREAMING        Prompt/Continue called
//	STREAMING → TURN_END    Provider finishes streaming
//	TURN_END → EXECUTING    Assistant message contains tool calls
//	EXECUTING → TURN_END    All tool results collected
//	TURN_END → STREAMING    Steering/follow-up queue has messages
//	TURN_END → DONE         No tools, no steering, no follow-up
func (a *Agent) run(ctx context.Context) error {
	slog.Debug("agent: run() entered")
	runCtx, cancel := context.WithCancel(ctx)

	a.mu.Lock()
	a.ctx = runCtx
	a.cancel = cancel
	a.mu.Unlock()

	defer func() {
		cancel()
		a.mu.Lock()
		a.state = StateDone
		a.mu.Unlock()
		a.emit(types.AgentEvent{Type: types.AgentEventAgentEnd})
	}()

	slog.Debug("agent: run() emitting EventStart")
	a.emit(types.AgentEvent{Type: types.AgentEventStart})
	slog.Debug("agent: run() emitted EventStart")

	// Safety guard: prevent infinite agent loops
	const maxIterations = 200
	iteration := 0

	for {
		iteration++
		if iteration > maxIterations {
			a.mu.Lock()
			a.runErr = fmt.Errorf("agent loop exceeded max iterations (%d) — possible tool-use loop", maxIterations)
			a.mu.Unlock()
			slog.Error("agent loop exceeded max iterations", "iterations", iteration)
			return a.runErr
		}
		// Check abort
		select {
		case <-runCtx.Done():
			a.mu.Lock()
			a.runErr = runCtx.Err()
			a.mu.Unlock()
			return runCtx.Err()
		default:
		}

		// --- STREAMING ---
		a.setState(StateStreaming)

		assistantMsg, err := a.streamToMessage(runCtx)
		if err != nil {
			a.mu.Lock()
			a.runErr = err
			a.mu.Unlock()
			return err
		}

		// Context cancelled during streaming (provider may return empty on cancel)
		if runCtx.Err() != nil {
			a.mu.Lock()
			a.runErr = runCtx.Err()
			a.mu.Unlock()
			return runCtx.Err()
		}

		// Append assistant message to transcript
		a.addMessage(*assistantMsg)
		a.emit(types.AgentEvent{Type: types.AgentEventMessageEnd})

		// --- TURN_END ---
		a.setState(StateTurnEnd)
		a.emit(types.AgentEvent{Type: types.AgentEventTurnEnd})

		// Extract tool calls from assistant message
		toolCalls := extractToolCalls(assistantMsg)

		if len(toolCalls) > 0 {
			// --- EXECUTING_TOOLS ---
			a.setState(StateExecuting)

			// Run tool execution in background and emit progress events
			resultsCh := make(chan []*tools.ToolCallResult, 1)
			go func() {
				resultsCh <- a.tools.ExecuteBatch(runCtx, toolCalls)
			}()

			// Emit periodic progress events to keep TUI responsive
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			var results []*tools.ToolCallResult
			execDone := false
			for !execDone {
				select {
				case <-runCtx.Done():
					a.mu.Lock()
					a.runErr = runCtx.Err()
					a.mu.Unlock()
					return runCtx.Err()
				case results = <-resultsCh:
					execDone = true
				case <-ticker.C:
					// Emit progress for each in-flight tool call
					for _, tc := range toolCalls {
						a.emit(types.AgentEvent{
							Type: types.AgentEventToolProgress,
							Data: map[string]any{
								"tool": tc.Name,
								"id":   tc.ID,
							},
						})
					}
				}
			}

			for i, result := range results {
				resultMsg := buildToolResultMessage(toolCalls[i], result)
				a.addMessage(resultMsg)

				a.emit(types.AgentEvent{
					Type: types.AgentEventToolResult,
					Data: map[string]any{
						"tool":    toolCalls[i].Name,
						"args":    string(toolCalls[i].Arguments),
						"isError": result.Result != nil && result.Result.IsError,
						"content": toolResultText(result),
					},
				})
			}

			// After tool execution, check steering queue
			a.drainSteerQueueToTranscript()

			continue
		}

		// No tool calls — check steering queue
		if a.drainSteerQueueToTranscript() > 0 {
			continue
		}

		// Check follow-up queue (only when agent would otherwise stop)
		if a.drainFollowUpQueueToTranscript() > 0 {
			continue
		}

		// --- DONE ---
		return nil
	}
}

// streamToMessage consumes the provider stream channel and builds
// an assistant AgentMessage. Emits agent events for each stream event.
func (a *Agent) streamToMessage(ctx context.Context) (*types.AgentMessage, error) {
	a.mu.RLock()
	thinkingLevel := a.thinkingLevel
	model := a.model
	a.mu.RUnlock()

	slog.Debug("agent: streamToMessage", "model", model.ID, "thinking_level", thinkingLevel, "reasoning", model.Reasoning)

	opts := types.StreamOptions{
		SystemPrompt:  a.buildSystemPrompt(),
		Tools:         a.tools.ToolDefinitions(),
		ThinkingLevel: thinkingLevel,
	}

	slog.Debug("agent: calling provider.Stream", "model", model.ID, "provider", a.provider.Name(), "thinking_level", thinkingLevel)
	events := a.provider.Stream(ctx, model, a.messages, opts.Tools, opts)
	slog.Debug("agent: provider.Stream returned, iterating events", "model", model.ID)

	var lastMsg *types.AgentMessage
	eventCount := 0
	for event := range events {
		eventCount++
		slog.Debug("agent: received event", "type", event.Type, "delta_len", len(event.Delta), "event_count", eventCount)
		switch event.Type {
		case types.EventStart:
			a.emit(types.AgentEvent{Type: types.AgentEventMessageStart})
		case types.EventTextDelta:
			a.emit(types.AgentEvent{Type: types.AgentEventTextDelta, Data: event.Delta})
		case types.EventThinkingDelta:
			a.emit(types.AgentEvent{Type: types.AgentEventThinkingDelta, Data: event.Delta})
		case types.EventToolCallStart:
			// event.Delta contains the tool name
			a.emit(types.AgentEvent{
				Type: types.AgentEventToolExecStart,
				Data: map[string]any{
					"tool": event.Delta,
				},
			})
		case types.EventToolCallEnd:
			// event.Message contains the full tool call with arguments
			args := ""
			if event.Message != nil {
				for _, block := range event.Message.Content {
					if block.Type == types.BlockToolCall && block.ToolCall != nil {
						argsJSON, _ := json.Marshal(block.ToolCall.Arguments)
						args = string(argsJSON)
						break
					}
				}
			}
			a.emit(types.AgentEvent{
				Type: types.AgentEventToolExecEnd,
				Data: map[string]any{
					"tool": event.Delta,
					"args": args,
				},
			})
		case types.EventDone:
			lastMsg = event.Message
			if lastMsg != nil {
				if lastMsg.API == "" {
					lastMsg.API = a.model.API
				}
				if lastMsg.Model == "" {
					lastMsg.Model = a.model.ID
				}
			}
			// Capture usage from this turn
			if event.Usage != nil {
				a.mu.Lock()
				a.lastTurnUsage = *event.Usage
				a.mu.Unlock()
			}
		case types.EventError:
			return nil, fmt.Errorf("provider stream error: %s", event.Error)
		}
	}

	slog.Debug("agent: event loop finished", "model", model.ID, "event_count", eventCount)

	if lastMsg == nil {
		// Provider returned empty response — create minimal assistant message
		lastMsg = &types.AgentMessage{
			ID:        newID(),
			Role:      types.RoleAssistant,
			Content:   []types.ContentBlock{{Type: types.BlockText, Text: ""}},
			Timestamp: now(),
			API:       a.model.API,
			Model:     a.model.ID,
		}
	}

	return lastMsg, nil
}

// extractToolCalls pulls ToolCallBlock entries from an assistant message
// and returns them as ToolCallRequests for the tools registry.
func extractToolCalls(msg *types.AgentMessage) []tools.ToolCallRequest {
	var calls []tools.ToolCallRequest
	for _, block := range msg.Content {
		if block.Type == types.BlockToolCall && block.ToolCall != nil {
			argsJSON, _ := json.Marshal(block.ToolCall.Arguments)
			slog.Debug("agent: extracting tool call", "tool", block.ToolCall.Name, "args", string(argsJSON))
			calls = append(calls, tools.ToolCallRequest{
				ID:        block.ToolCall.ID,
				Name:      block.ToolCall.Name,
				Arguments: argsJSON,
			})
		}
	}
	return calls
}

// buildToolResultMessage creates a tool_result AgentMessage from a tool call
// and its execution result.
func buildToolResultMessage(call tools.ToolCallRequest, result *tools.ToolCallResult) types.AgentMessage {
	content := []types.ContentBlock{
		{
			Type: types.BlockText,
			Text: fmt.Sprintf("[%s] tool call %s", call.Name, call.ID),
		},
	}
	if result != nil && result.Result != nil {
		content = result.Result.Content
	}
	return types.AgentMessage{
		ID:         newID(),
		Role:       types.RoleToolResult,
		Content:    content,
		Timestamp:  now(),
		ToolCallID: call.ID,
	}
}

// toolResultText extracts the text content from a tool call result.
func toolResultText(result *tools.ToolCallResult) string {
	if result == nil || result.Result == nil {
		return ""
	}
	var parts []string
	for _, block := range result.Result.Content {
		if block.Type == types.BlockText && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// drainSteerQueueToTranscript drains the steering queue and appends messages
// to the transcript. Returns the number of messages drained.
func (a *Agent) drainSteerQueueToTranscript() int {
	msgs := a.drainSteerQueue()
	for _, msg := range msgs {
		a.addMessage(msg)
	}
	return len(msgs)
}

// drainFollowUpQueueToTranscript drains the follow-up queue and appends
// messages to the transcript. Returns the number of messages drained.
func (a *Agent) drainFollowUpQueueToTranscript() int {
	msgs := a.drainFollowUpQueue()
	for _, msg := range msgs {
		a.addMessage(msg)
	}
	return len(msgs)
}

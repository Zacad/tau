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

		assistantMsg, nativeFinalizedToolCalls, err := a.streamToMessage(runCtx)
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
			for _, call := range toolCalls {
				if nativeFinalizedToolCalls[call.ID] {
					continue
				}
				a.emit(types.AgentEvent{
					Type: types.AgentEventToolExecEnd,
					Data: types.ToolLifecycleEvent{
						CallID:       call.ID,
						ToolName:     call.Name,
						Phase:        types.ToolLifecycleFinalized,
						Source:       types.ToolLifecycleSourceInferred,
						ArgsJSON:     append([]byte(nil), call.Arguments...),
						ArgsSummary:  types.SummarizeToolArgsJSON(call.Name, call.Arguments),
						ArgsComplete: true,
					},
				})
			}

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
					for _, call := range toolCalls {
						resultMsg := buildInterruptedToolResultMessage(call, runCtx.Err())
						a.addMessage(resultMsg)

						a.emit(types.AgentEvent{
							Type: types.AgentEventToolResult,
							Data: types.ToolResultEvent{
								CallID:   call.ID,
								ToolName: call.Name,
								IsError:  true,
								Content:  toolResultTextFromMessage(resultMsg),
							},
						})
					}
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
							Data: types.ToolProgressEvent{
								CallID:   tc.ID,
								ToolName: tc.Name,
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
					Data: types.ToolResultEvent{
						CallID:   toolCalls[i].ID,
						ToolName: toolCalls[i].Name,
						IsError:  result.Result != nil && result.Result.IsError,
						Content:  toolResultText(result),
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
func (a *Agent) streamToMessage(ctx context.Context) (*types.AgentMessage, map[string]bool, error) {
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
	nativeFinalizedToolCalls := make(map[string]bool)
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
			// event.Delta contains the tool name. Some providers do not expose a
			// stable call ID until the final message; use native/source metadata now
			// and emit inferred finalized events with IDs after EventDone.
			a.emit(types.AgentEvent{
				Type: types.AgentEventToolExecStart,
				Data: types.ToolLifecycleEvent{
					ToolName:     event.Delta,
					Phase:        types.ToolLifecycleRequested,
					Source:       types.ToolLifecycleSourceNative,
					ArgsSummary:  types.SummarizeToolArgs(event.Delta, nil),
					ArgsComplete: false,
				},
			})
		case types.EventToolCallEnd:
			// event.Message contains the full tool call with arguments for providers
			// that stream symmetric native end events.
			var payload *types.ToolLifecycleEvent
			if event.Message != nil {
				for _, block := range event.Message.Content {
					if block.Type == types.BlockToolCall && block.ToolCall != nil {
						argsJSON, _ := json.Marshal(block.ToolCall.Arguments)
						payload = &types.ToolLifecycleEvent{
							CallID:       block.ToolCall.ID,
							ToolName:     block.ToolCall.Name,
							Phase:        types.ToolLifecycleFinalized,
							Source:       types.ToolLifecycleSourceNative,
							ArgsJSON:     argsJSON,
							ArgsSummary:  types.SummarizeToolArgs(block.ToolCall.Name, block.ToolCall.Arguments),
							ArgsComplete: true,
						}
						nativeFinalizedToolCalls[block.ToolCall.ID] = true
						break
					}
				}
			}
			if payload == nil {
				// A native end event without a stable call ID cannot satisfy the
				// canonical finalized contract. Wait for EventDone/final assistant
				// message normalization to emit one inferred finalized event with ID.
				continue
			}
			a.emit(types.AgentEvent{Type: types.AgentEventToolExecEnd, Data: *payload})
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
			return nil, nativeFinalizedToolCalls, fmt.Errorf("provider stream error: %s", event.Error)
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

	return lastMsg, nativeFinalizedToolCalls, nil
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

// buildInterruptedToolResultMessage creates a synthetic tool_result for a tool
// call that was in-flight when the agent was interrupted. Provider APIs such as
// OpenAI Responses require every assistant tool call in history to have a
// matching tool output before the next request.
func buildInterruptedToolResultMessage(call tools.ToolCallRequest, err error) types.AgentMessage {
	msg := "Tool execution interrupted"
	if err != nil {
		msg = fmt.Sprintf("%s: %v", msg, err)
	}
	return buildToolResultMessage(call, &tools.ToolCallResult{
		ID:   call.ID,
		Name: call.Name,
		Result: &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: msg}},
		},
	})
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

func toolResultTextFromMessage(msg types.AgentMessage) string {
	var parts []string
	for _, block := range msg.Content {
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

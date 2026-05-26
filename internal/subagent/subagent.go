// Package subagent implements the sub-agent lifecycle for Tau.
//
// Sub-agents run synchronously within the parent agent loop, with isolated
// context and optional tool access. They are spawned by the parent agent
// to delegate focused tasks.
//
// Import rules: this package depends on internal/types, internal/provider,
// and the Go standard library. It does NOT import internal/tools to avoid
// import cycles (tools imports subagent for SubAgentTool).
package subagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/adam/tau/internal/provider"
	"github.com/adam/tau/internal/types"
	"github.com/invopop/jsonschema"
)

// ContextMode defines how a sub-agent's message context is initialized.
type ContextMode string

const (
	// ContextFresh starts with an empty message slice, inheriting only
	// the system prompt. This is the default and recommended mode.
	ContextFresh ContextMode = "fresh"

	// ContextFork starts with a shallow copy of parent messages,
	// giving the sub-agent visibility into the parent conversation.
	// Modifications to the forked context do not affect the parent.
	ContextFork ContextMode = "fork"

	// DefaultTimeout is the default execution timeout for sub-agents.
	DefaultTimeout = 5 * time.Minute

	// maxToolIterations prevents infinite tool-use loops.
	maxToolIterations = 50

	// eventsBufferSize is the buffer size for the internal event channel.
	// Matches the TUI event channel pattern for consistency.
	eventsBufferSize = 256

	// defaultConcurrency is the default max concurrent subagents.
	defaultConcurrency = 4

	// defaultMaxTasks is the default max tasks accepted.
	defaultMaxTasks = 8
)

// SubAgentTask wraps a SubAgent for parallel execution.
type SubAgentTask struct {
	SubAgent *SubAgent
}

// ParallelOpts configures parallel execution.
type ParallelOpts struct {
	Concurrency int
	MaxTasks    int
	Events      chan types.AgentEvent
}

// ParallelResult holds aggregated results from parallel execution.
type ParallelResult struct {
	Results      []SubAgentResult
	SuccessCount int
	FailureCount int
	TotalUsage   types.Usage
	Error        error
}

// Tool is the interface for tools available to a sub-agent.
// It mirrors tools.Tool to avoid an import cycle.
type Tool interface {
	Name() string
	Description() string
	Parameters() any
	ExecutionMode() types.ExecutionMode
	Execute(ctx context.Context, params any) (*types.ToolResult, error)
}

// ToolCallRequest represents a single tool invocation from the LLM.
type ToolCallRequest struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolCallResult holds the outcome of a single tool invocation.
type ToolCallResult struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Result *types.ToolResult `json:"result"`
}

// Executor executes a batch of tool calls and returns results in order.
// The caller (typically the tools package or agent) provides this to
// bridge subagent's local types with the actual tool registry.
type Executor func(ctx context.Context, calls []ToolCallRequest) []*ToolCallResult

// SubAgent is a delegated agent that runs synchronously within the parent.
// It has its own context (fresh or forked), optional tool access, and
// returns a structured result upon completion.
type SubAgent struct {
	ID             string
	Type           Type
	Task           string
	ContextMode    ContextMode
	SystemPrompt   string
	Model          types.Model
	Provider       provider.Provider
	Timeout        time.Duration
	ParentMessages []types.AgentMessage
	Tools          []Tool
	Executor       Executor
	Events         chan types.AgentEvent
}

// SubAgentOpts configures a new SubAgent via the constructor.
type SubAgentOpts struct {
	Type            Type
	Task            string
	ContextMode     ContextMode
	SystemPrompt    string
	Model           types.Model
	ID              string
	Timeout         time.Duration
	ParentMessages  []types.AgentMessage
	Tools           []Tool
	ParentToolNames []string
	Executor        Executor
	Events          chan types.AgentEvent
}

// SubAgentResult holds the outcome of a completed sub-agent execution.
type SubAgentResult struct {
	Success    bool
	Timeout    bool
	Output     string
	Error      error
	Duration   time.Duration
	Usage      types.Usage
	LLMVisible bool
	Artifacts  []string
}

// NewSubAgent creates a new SubAgent with the given provider and options.
// Panics if provider is nil. Auto-generates ID if not provided.
// Defaults ContextMode to ContextFresh if not specified.
// Panics if any subagent tool is not in the parent tool set (hard ceiling).
func NewSubAgent(p provider.Provider, opts SubAgentOpts) *SubAgent {
	if p == nil {
		panic("subagent: provider must not be nil")
	}

	parentSet := make(map[string]bool, len(opts.ParentToolNames))
	for _, n := range opts.ParentToolNames {
		parentSet[n] = true
	}

	for _, t := range opts.Tools {
		if len(parentSet) > 0 && !parentSet[t.Name()] {
			panic(fmt.Sprintf("subagent: tool %q not in parent tool set (hard ceiling violation)", t.Name()))
		}
	}

	sa := &SubAgent{
		ID:             opts.ID,
		Type:           opts.Type,
		Task:           opts.Task,
		ContextMode:    opts.ContextMode,
		SystemPrompt:   opts.SystemPrompt,
		Model:          opts.Model,
		Provider:       p,
		Timeout:        opts.Timeout,
		ParentMessages: opts.ParentMessages,
		Tools:          opts.Tools,
		Executor:       opts.Executor,
		Events:         opts.Events,
	}

	if sa.ID == "" {
		sa.ID = generateID()
	}

	if sa.ContextMode == "" {
		sa.ContextMode = ContextFresh
	}

	if sa.Timeout <= 0 {
		sa.Timeout = DefaultTimeout
	}

	return sa
}

// Run executes the sub-agent synchronously until completion or context cancellation.
// Returns a SubAgentResult with the output, duration, and usage.
func (sa *SubAgent) Run(ctx context.Context) SubAgentResult {
	start := time.Now()
	result := SubAgentResult{LLMVisible: true}

	timeoutCtx, cancel := context.WithTimeout(ctx, sa.Timeout)
	defer cancel()

	internalEvents := make(chan types.StreamEvent, eventsBufferSize)
	done := make(chan struct{})
	go sa.forwardEvents(internalEvents, done)

	sa.emitEvent(types.AgentEvent{Type: types.AgentEventStart})

	var messages []types.AgentMessage
	switch sa.ContextMode {
	case ContextFork:
		messages = make([]types.AgentMessage, len(sa.ParentMessages)+1)
		copy(messages, sa.ParentMessages)
		messages[len(sa.ParentMessages)] = types.AgentMessage{
			Role:    types.RoleUser,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: sa.Task}},
		}
	default:
		messages = []types.AgentMessage{
			{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: sa.Task}}},
		}
	}

	toolDefs := sa.buildToolDefinitions()

	systemPrompt := sa.SystemPrompt
	if len(toolDefs) > 0 {
		systemPrompt += "\n\n## Tool Usage Guidelines\n" +
			"- After gathering sufficient information, stop using tools and provide your final answer\n" +
			"- Do not repeat the same tool call with identical parameters\n" +
			"- If a tool returns an error, try a different approach rather than retrying the same call\n" +
			"- You must provide a final text response — do not end with a tool call"
		switch sa.Type {
		case TypeResearcher:
			systemPrompt += "\n- For research tasks, use websearch and webfetch aggressively — gather information from multiple sources"
		case TypeSecurityReviewer, TypeReviewer:
			systemPrompt += "\n- Use read and grep thoroughly to examine all relevant code paths"
		default:
			systemPrompt += "\n- Use tools when they help complete the task more effectively"
		}
	}

	slog.Debug("subagent.Run: start",
		"id", sa.ID,
		"model", sa.Model.ID,
		"provider", sa.Provider.Name(),
		"system_prompt_len", len(systemPrompt),
		"task_len", len(sa.Task),
		"timeout", sa.Timeout,
		"tools", len(sa.Tools),
	)

	var textAccum string
	var lastMsg *types.AgentMessage
	iteration := 0
	var artifacts []string

	for {
		iteration++
		if iteration > maxToolIterations {
			result.Success = false
			result.Duration = time.Since(start)
			result.Error = fmt.Errorf("subagent: exceeded max tool iterations (%d)", maxToolIterations)
			result.Output = textAccum
			result.Artifacts = artifacts
			slog.Warn("subagent.Run: max iterations exceeded",
				"id", sa.ID,
				"iterations", iteration,
			)
			close(internalEvents)
			<-done
			return result
		}

		streamOpts := types.StreamOptions{
			SystemPrompt: systemPrompt,
			Tools:        toolDefs,
		}

		providerEvents := sa.Provider.Stream(timeoutCtx, sa.Model, messages, toolDefs, streamOpts)

		var turnTextAccum string
		var turnLastMsg *types.AgentMessage
		toolCalls := sa.consumeStream(timeoutCtx, providerEvents, internalEvents, &result, &turnTextAccum, &turnLastMsg, start)

		textAccum += turnTextAccum
		if turnLastMsg != nil {
			lastMsg = turnLastMsg
			messages = append(messages, *turnLastMsg)
		}

		if toolCalls == nil {
			break
		}

		if len(toolCalls) == 0 {
			break
		}

		if sa.Executor == nil {
			break
		}

		slog.Debug("subagent.Run: executing tool calls",
			"id", sa.ID,
			"count", len(toolCalls),
			"iteration", iteration,
		)

		results := sa.Executor(timeoutCtx, toolCalls)

		for i, call := range toolCalls {
			r := results[i]
			resultMsg := buildToolResultMessage(call, r)
			messages = append(messages, resultMsg)

			sa.emitEvent(types.AgentEvent{
				Type: types.AgentEventToolResult,
				Data: types.ToolResultEvent{
					CallID:   call.ID,
					ToolName: call.Name,
					IsError:  r != nil && r.Result != nil && r.Result.IsError,
					Content:  toolResultText(r),
				},
			})

			if artifact := extractArtifact(call, r); artifact != "" {
				artifacts = append(artifacts, artifact)
			}
		}
	}

	close(internalEvents)
	<-done

	if timeoutCtx.Err() != nil || result.Error != nil {
		result.Success = false
		result.Duration = time.Since(start)
		result.Artifacts = artifacts
		if result.Error == nil {
			if timeoutCtx.Err() == context.DeadlineExceeded {
				result.Timeout = true
				result.Error = fmt.Errorf("subagent: execution timed out after %s: %w", sa.Timeout, context.DeadlineExceeded)
			} else if timeoutCtx.Err() != nil {
				result.Error = fmt.Errorf("subagent: execution cancelled: %w", timeoutCtx.Err())
			}
		}
		slog.Warn("subagent.Run: context error",
			"id", sa.ID,
			"timeout", result.Timeout,
			"error", result.Error,
		)
		return result
	}

	output := textAccum
	if lastMsg != nil {
		for _, block := range lastMsg.Content {
			if block.Type == types.BlockText {
				output += block.Text
			}
		}
	}

	result.Success = true
	result.Output = output
	result.Duration = time.Since(start)
	result.Artifacts = artifacts

	sa.emitEvent(types.AgentEvent{Type: types.AgentEventMessageEnd})
	sa.emitEvent(types.AgentEvent{Type: types.AgentEventAgentEnd})

	slog.Debug("subagent.Run: complete",
		"id", sa.ID,
		"success", result.Success,
		"output_len", len(result.Output),
		"duration", result.Duration,
	)

	return result
}

// SubAgentStep defines a single step in a chain execution.
type SubAgentStep struct {
	Task           string
	Type           Type
	SystemPrompt   string
	Model          types.Model
	Timeout        time.Duration
	ContextMode    ContextMode
	ParentMessages []types.AgentMessage
	Tools          []Tool
	Executor       Executor
	Events         chan types.AgentEvent
}

// ChainResult holds the outcome of a chain execution.
type ChainResult struct {
	Output         string
	Error          error
	Duration       time.Duration
	TotalUsage     types.Usage
	Steps          []SubAgentResult
	CompletedSteps int
	FailedStep     int // -1 if all succeeded
}

// RunChain executes steps sequentially, passing the output of each step
// to the next via {previous} placeholder substitution.
// Stops at the first failure. Usage is aggregated across all steps.
func RunChain(ctx context.Context, provider provider.Provider, steps []SubAgentStep, parentToolNames []string) ChainResult {
	if len(steps) == 0 {
		return ChainResult{Steps: []SubAgentResult{}, FailedStep: -1}
	}

	start := time.Now()
	var results []SubAgentResult
	var previousOutput string
	var totalUsage types.Usage

	for i, step := range steps {
		task := strings.ReplaceAll(step.Task, "{previous}", previousOutput)

		sa := NewSubAgent(provider, SubAgentOpts{
			Type:            step.Type,
			Task:            task,
			SystemPrompt:    step.SystemPrompt,
			Model:           step.Model,
			Timeout:         step.Timeout,
			ContextMode:     step.ContextMode,
			ParentMessages:  step.ParentMessages,
			Tools:           step.Tools,
			ParentToolNames: parentToolNames,
			Executor:        step.Executor,
			Events:          step.Events,
		})

		result := sa.Run(ctx)
		results = append(results, result)

		totalUsage.Input += result.Usage.Input
		totalUsage.Output += result.Usage.Output
		totalUsage.CacheRead += result.Usage.CacheRead
		totalUsage.CacheWrite += result.Usage.CacheWrite
		totalUsage.TotalTokens += result.Usage.TotalTokens
		totalUsage.Cost.Input += result.Usage.Cost.Input
		totalUsage.Cost.Output += result.Usage.Cost.Output
		totalUsage.Cost.CacheRead += result.Usage.Cost.CacheRead
		totalUsage.Cost.CacheWrite += result.Usage.Cost.CacheWrite
		totalUsage.Cost.Total += result.Usage.Cost.Total

		if !result.Success {
			return ChainResult{
				Output:         result.Output,
				Error:          result.Error,
				Duration:       time.Since(start),
				TotalUsage:     totalUsage,
				Steps:          results,
				CompletedSteps: i,
				FailedStep:     i,
			}
		}

		previousOutput = result.Output
	}

	return ChainResult{
		Output:         previousOutput,
		Duration:       time.Since(start),
		TotalUsage:     totalUsage,
		Steps:          results,
		CompletedSteps: len(steps),
		FailedStep:     -1,
	}
}

// RunParallel executes multiple sub-agents concurrently with configurable concurrency.
// Returns results in the same order as input tasks.
// Rejects entirely if len(tasks) > opts.MaxTasks (default 8).
func RunParallel(ctx context.Context, tasks []SubAgentTask, opts ParallelOpts) ParallelResult {
	if len(tasks) == 0 {
		return ParallelResult{Results: []SubAgentResult{}}
	}

	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	maxTasks := opts.MaxTasks
	if maxTasks <= 0 {
		maxTasks = defaultMaxTasks
	}

	if len(tasks) > maxTasks {
		return ParallelResult{
			Error: fmt.Errorf("subagent: too many tasks (%d), max is %d", len(tasks), maxTasks),
		}
	}

	results := make([]SubAgentResult, len(tasks))
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var fwdWg sync.WaitGroup
	var mu sync.Mutex
	taskEventsChans := make([]chan types.AgentEvent, len(tasks))

	for i, task := range tasks {
		wg.Add(1)
		go func(idx int, t SubAgentTask) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if t.SubAgent.Events == nil && opts.Events != nil {
				taskEvents := make(chan types.AgentEvent, eventsBufferSize)
				taskEventsChans[idx] = taskEvents
				t.SubAgent.Events = taskEvents

				fwdWg.Add(1)
				go func() {
					defer fwdWg.Done()
					for evt := range taskEvents {
						select {
						case opts.Events <- evt:
						default:
						}
					}
				}()
			}

			result := t.SubAgent.Run(ctx)

			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, task)
	}

	wg.Wait()

	for _, ch := range taskEventsChans {
		if ch != nil {
			close(ch)
		}
	}
	fwdWg.Wait()

	var pr ParallelResult
	pr.Results = results
	for _, r := range results {
		if r.Success {
			pr.SuccessCount++
		} else {
			pr.FailureCount++
		}
		pr.TotalUsage.Input += r.Usage.Input
		pr.TotalUsage.Output += r.Usage.Output
		pr.TotalUsage.CacheRead += r.Usage.CacheRead
		pr.TotalUsage.CacheWrite += r.Usage.CacheWrite
		pr.TotalUsage.TotalTokens += r.Usage.TotalTokens
		pr.TotalUsage.Cost.Input += r.Usage.Cost.Input
		pr.TotalUsage.Cost.Output += r.Usage.Cost.Output
		pr.TotalUsage.Cost.CacheRead += r.Usage.Cost.CacheRead
		pr.TotalUsage.Cost.CacheWrite += r.Usage.Cost.CacheWrite
		pr.TotalUsage.Cost.Total += r.Usage.Cost.Total
	}

	return pr
}

// forwardEvents reads from the internal provider event channel and forwards
// converted AgentEvent to the parent Events channel. Runs in a goroutine.
// Closes when internalEvents is closed, then signals via done.
func (sa *SubAgent) forwardEvents(internalEvents <-chan types.StreamEvent, done chan<- struct{}) {
	defer close(done)

	if sa.Events == nil {
		// Drain the channel to prevent provider goroutine leak
		for range internalEvents {
		}
		return
	}

	for event := range internalEvents {
		agentEvent := sa.convertStreamEvent(event)
		select {
		case sa.Events <- agentEvent:
		default:
			// Drop event if channel is full — don't block Run()
		}
	}
}

// convertStreamEvent converts a provider StreamEvent to an AgentEvent.
func (sa *SubAgent) convertStreamEvent(event types.StreamEvent) types.AgentEvent {
	subAgentID := sa.ID
	ae := types.AgentEvent{SubAgentID: &subAgentID}

	switch event.Type {
	case types.EventStart:
		ae.Type = types.AgentEventStart
	case types.EventTextStart:
		ae.Type = types.AgentEventMessageStart
	case types.EventTextDelta:
		ae.Type = types.AgentEventTextDelta
		ae.Data = event.Delta
	case types.EventTextEnd:
		ae.Type = types.AgentEventMessageEnd
	case types.EventThinkingStart:
		ae.Type = types.AgentEventThinkingDelta
	case types.EventThinkingDelta:
		ae.Type = types.AgentEventThinkingDelta
		ae.Data = event.Delta
	case types.EventThinkingEnd:
		ae.Type = types.AgentEventMessageEnd
	case types.EventToolCallStart:
		ae.Type = types.AgentEventToolExecStart
		ae.Data = types.ToolLifecycleEvent{
			ToolName:     event.Delta,
			Phase:        types.ToolLifecycleRequested,
			Source:       types.ToolLifecycleSourceNative,
			ArgsSummary:  types.SummarizeToolArgs(event.Delta, nil),
			ArgsComplete: false,
		}
	case types.EventToolCallEnd:
		ae.Type = types.AgentEventToolExecEnd
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
					break
				}
			}
		}
		if payload == nil {
			// Native end without a stable ID is not canonical finalized metadata.
			// consumeStream will emit an inferred finalized event from EventDone.
			ae.Type = types.AgentEventToolProgress
			ae.Data = types.ToolProgressEvent{ToolName: event.Delta}
		} else {
			ae.Data = *payload
		}
	case types.EventDone:
		ae.Type = types.AgentEventMessageEnd
	case types.EventError:
		ae.Type = types.AgentEventError
		ae.Data = event.Error
	default:
		ae.Type = types.AgentEventError
		ae.Data = fmt.Sprintf("unknown stream event type: %s", event.Type)
	}

	return ae
}

// emitEvent sends an AgentEvent to the Events channel if configured.
func (sa *SubAgent) emitEvent(event types.AgentEvent) {
	if sa.Events == nil {
		return
	}
	subAgentID := sa.ID
	event.SubAgentID = &subAgentID
	select {
	case sa.Events <- event:
	default:
		// Drop event if channel is full — don't block Run()
	}
}

// consumeStream processes a stream of events, accumulating text and extracting tool calls.
// Returns the tool calls found (nil if stream errored or was cancelled).
func (sa *SubAgent) consumeStream(ctx context.Context, events <-chan types.StreamEvent, internalEvents chan<- types.StreamEvent, result *SubAgentResult, textAccum *string, lastMsg **types.AgentMessage, start time.Time) []ToolCallRequest {
	nativeFinalizedToolCalls := make(map[string]bool)
	for event := range events {
		// Forward to internal channel for the goroutine forwarder
		internalEvents <- event
		if event.Type == types.EventToolCallEnd && event.Message != nil {
			for _, block := range event.Message.Content {
				if block.Type == types.BlockToolCall && block.ToolCall != nil && block.ToolCall.ID != "" {
					nativeFinalizedToolCalls[block.ToolCall.ID] = true
				}
			}
		}

		switch event.Type {
		case types.EventTextDelta:
			*textAccum += event.Delta
		case types.EventDone:
			*lastMsg = event.Message
			if event.Usage != nil {
				result.Usage.Input += event.Usage.Input
				result.Usage.Output += event.Usage.Output
				result.Usage.CacheRead += event.Usage.CacheRead
				result.Usage.CacheWrite += event.Usage.CacheWrite
				result.Usage.TotalTokens += event.Usage.TotalTokens
				result.Usage.Cost.Input += event.Usage.Cost.Input
				result.Usage.Cost.Output += event.Usage.Cost.Output
				result.Usage.Cost.CacheRead += event.Usage.Cost.CacheRead
				result.Usage.Cost.CacheWrite += event.Usage.Cost.CacheWrite
				result.Usage.Cost.Total += event.Usage.Cost.Total
			}
		case types.EventError:
			result.Success = false
			result.Duration = time.Since(start)
			if ctx.Err() == context.DeadlineExceeded {
				result.Timeout = true
				result.Error = fmt.Errorf("subagent: execution timed out after %s: %w", sa.Timeout, context.DeadlineExceeded)
			} else {
				result.Error = fmt.Errorf("provider stream error: %s", event.Error)
			}
			slog.Warn("subagent.Run: stream error",
				"id", sa.ID,
				"timeout", result.Timeout,
				"error", result.Error,
			)
			return nil
		}
	}

	if *lastMsg == nil {
		return nil
	}

	toolCalls := extractToolCalls(*lastMsg)
	for _, call := range toolCalls {
		if nativeFinalizedToolCalls[call.ID] {
			continue
		}
		sa.emitEvent(types.AgentEvent{
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
	return toolCalls
}

// buildToolDefinitions converts the tool set to provider ToolDefinition slices.
func (sa *SubAgent) buildToolDefinitions() []types.ToolDefinition {
	if len(sa.Tools) == 0 {
		return nil
	}

	defs := make([]types.ToolDefinition, 0, len(sa.Tools))
	for _, t := range sa.Tools {
		schema := jsonschema.Reflect(t.Parameters())
		defs = append(defs, types.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  schema,
		})
	}
	return defs
}

// extractToolCalls pulls ToolCallBlock entries from an assistant message
// and returns them as ToolCallRequests.
func extractToolCalls(msg *types.AgentMessage) []ToolCallRequest {
	var calls []ToolCallRequest
	for _, block := range msg.Content {
		if block.Type == types.BlockToolCall && block.ToolCall != nil {
			argsJSON, _ := json.Marshal(block.ToolCall.Arguments)
			calls = append(calls, ToolCallRequest{
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
func buildToolResultMessage(call ToolCallRequest, result *ToolCallResult) types.AgentMessage {
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
		ID:         generateID(),
		Role:       types.RoleToolResult,
		Content:    content,
		Timestamp:  time.Now(),
		ToolCallID: call.ID,
	}
}

// extractArtifact extracts a file path artifact from a tool call and its result.
// Tracks files created or modified by write, edit, or bash tools.
func extractArtifact(call ToolCallRequest, result *ToolCallResult) string {
	if result == nil || result.Result == nil {
		return ""
	}

	switch call.Name {
	case "write", "edit":
		var args struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err == nil && args.Path != "" {
			return filepath.Clean(args.Path)
		}
	case "bash":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err == nil && args.Command != "" {
			// Extract file paths from bash output (e.g., "Created file: /path/to/file")
			content := resultText(result.Result)
			if strings.Contains(content, "Created file:") || strings.Contains(content, "Written to:") {
				for _, line := range strings.Split(content, "\n") {
					line = strings.TrimSpace(line)
					if idx := strings.Index(line, ": "); idx != -1 {
						path := strings.TrimSpace(line[idx+2:])
						if filepath.IsAbs(path) {
							return filepath.Clean(path)
						}
					}
				}
			}
		}
	}
	return ""
}

// resultText extracts text content from a ToolResult.
func resultText(r *types.ToolResult) string {
	var sb strings.Builder
	for _, block := range r.Content {
		if block.Type == types.BlockText {
			sb.WriteString(block.Text)
		}
	}
	return sb.String()
}

// FormatForLLM formats the SubAgentResult as a structured, LLM-consumable string.
// Suitable for injection as a tool result message in the parent context.
func (r *SubAgentResult) FormatForLLM(task string) string {
	var sb strings.Builder
	sb.WriteString("## Subagent Result\n\n")
	sb.WriteString(fmt.Sprintf("**Task**: %s\n\n", task))

	if r.Success {
		sb.WriteString(fmt.Sprintf("**Status**: Success\n"))
	} else if r.Timeout {
		sb.WriteString(fmt.Sprintf("**Status**: Timeout\n"))
	} else {
		sb.WriteString(fmt.Sprintf("**Status**: Failed\n"))
	}

	sb.WriteString(fmt.Sprintf("**Duration**: %s\n", r.Duration.Round(time.Millisecond)))

	if r.Usage.TotalTokens > 0 {
		sb.WriteString(fmt.Sprintf("**Tokens**: %d input, %d output, %d total\n\n", r.Usage.Input, r.Usage.Output, r.Usage.TotalTokens))
	} else {
		sb.WriteString("\n")
	}

	if r.Success {
		sb.WriteString(fmt.Sprintf("**Output**:\n%s\n", r.Output))
	} else {
		sb.WriteString(fmt.Sprintf("**Error**: %s\n", r.Error))
	}

	if len(r.Artifacts) > 0 {
		sb.WriteString("\n**Artifacts**:\n")
		for _, a := range r.Artifacts {
			sb.WriteString(fmt.Sprintf("- %s\n", a))
		}
	}

	return sb.String()
}

// InjectResult injects a SubAgentResult into the parent message list.
// If LLMVisible is true, appends a tool result message with formatted output.
// If LLMVisible is false, returns messages unchanged (result stored separately).
func InjectResult(messages []types.AgentMessage, result SubAgentResult, task string) []types.AgentMessage {
	if !result.LLMVisible {
		return messages
	}

	formatted := result.FormatForLLM(task)
	return append(messages, types.AgentMessage{
		ID:        generateID(),
		Role:      types.RoleToolResult,
		Content:   []types.ContentBlock{{Type: types.BlockText, Text: formatted}},
		Timestamp: time.Now(),
	})
}

// generateID creates a unique sub-agent ID.
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

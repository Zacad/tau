// Package agent implements the Tau agent loop: streaming conversation,
// tool execution orchestration, steering/follow-up queues, and event emission.
//
// Import rules: this package depends on internal/types, internal/provider,
// internal/tools, internal/config, and the Go standard library.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/provider"
	"github.com/adam/tau/internal/tools"
	"github.com/adam/tau/internal/types"
)

// AgentState represents the current state of the agent loop.
type AgentState string

const (
	StateIdle      AgentState = "idle"
	StateStreaming AgentState = "streaming"
	StateTurnEnd   AgentState = "turn_end"
	StateExecuting AgentState = "executing_tools"
	StateDone      AgentState = "done"
)

const queueSize = 10

// Agent is the core orchestrator. It manages the conversation transcript,
// streams from the provider, executes tools, and handles steering/follow-up.
//
// The agent receives a pre-composed system prompt from the SDK (including
// skill progressive disclosure). It independently loads context files
// (AGENTS.md/CLAUDE.md) and prepends them to the system prompt.
type Agent struct {
	mu sync.RWMutex

	// Transcript
	messages     []types.AgentMessage
	systemPrompt string // Pre-composed by SDK (includes skills)
	cwd          string

	// Tools
	tools *tools.Registry

	// Provider
	provider provider.Provider
	model    types.Model

	// Thinking
	thinkingLevel types.ThinkingLevel

	// Steering/follow-up queues (drop oldest on overflow)
	steerMu     sync.Mutex
	steerQueue  []types.AgentMessage
	followUpMu  sync.Mutex
	followUpQueue []types.AgentMessage

	// Events
	listeners []listenerEntry

	// Abort
	cancel context.CancelFunc
	ctx    context.Context

	// State
	state AgentState
	runErr error

	// Usage from the last completed LLM turn.
	lastTurnUsage types.Usage
}

// Options configures a new Agent.
type Options struct {
	// SystemPrompt is the pre-composed system prompt from SDK (includes skills).
	SystemPrompt string
	// WorkingDir is the cwd for context file discovery and tool execution.
	WorkingDir string
	// Provider for LLM calls.
	Provider provider.Provider
	// Model to use for LLM calls.
	Model types.Model
	// ToolRegistry for tool execution.
	ToolRegistry *tools.Registry
}

// New creates a new Agent with the given options.
func New(opts Options) *Agent {
	a := &Agent{
		systemPrompt: opts.SystemPrompt,
		cwd:          opts.WorkingDir,
		provider:     opts.Provider,
		model:        opts.Model,
		tools:        opts.ToolRegistry,
		state:        StateIdle,
	}
	if a.tools == nil {
		a.tools = tools.NewRegistry()
	}
	return a
}

// Prompt adds a user message to the transcript and runs the agent loop
// until completion. Blocks until the agent reaches DONE or context is cancelled.
func (a *Agent) Prompt(ctx context.Context, message string) error {
	a.mu.Lock()
	a.messages = append(a.messages, types.AgentMessage{
		ID:        newID(),
		Role:      types.RoleUser,
		Content:   []types.ContentBlock{{Type: types.BlockText, Text: message}},
		Timestamp: now(),
	})
	a.mu.Unlock()

	return a.run(ctx)
}

// Continue runs the agent loop without adding a new user message.
// Used for follow-up tasks after the agent has reached DONE.
func (a *Agent) Continue(ctx context.Context) error {
	return a.run(ctx)
}

// Steer adds a message to the steering queue. The message will be
// delivered after the current tool call batch completes, before
// the next LLM call. Non-blocking — drops oldest if queue is full.
func (a *Agent) Steer(message string) error {
	msg := types.AgentMessage{
		ID:        newID(),
		Role:      types.RoleUser,
		Content:   []types.ContentBlock{{Type: types.BlockText, Text: message}},
		Timestamp: now(),
	}

	a.steerMu.Lock()
	defer a.steerMu.Unlock()

	if len(a.steerQueue) >= queueSize {
		a.steerQueue = a.steerQueue[1:] // drop oldest
		slog.Warn("steer queue overflow, dropped oldest message")
	}
	a.steerQueue = append(a.steerQueue, msg)
	return nil
}

// FollowUp adds a message to the follow-up queue. The message will be
// delivered only when the agent would otherwise stop (no tool calls,
// no steering). Non-blocking — drops oldest if queue is full.
func (a *Agent) FollowUp(message string) error {
	msg := types.AgentMessage{
		ID:        newID(),
		Role:      types.RoleUser,
		Content:   []types.ContentBlock{{Type: types.BlockText, Text: message}},
		Timestamp: now(),
	}

	a.followUpMu.Lock()
	defer a.followUpMu.Unlock()

	if len(a.followUpQueue) >= queueSize {
		a.followUpQueue = a.followUpQueue[1:] // drop oldest
		slog.Warn("follow-up queue overflow, dropped oldest message")
	}
	a.followUpQueue = append(a.followUpQueue, msg)
	return nil
}

// Abort cancels the current agent loop. Safe to call from any state.
// Partial results from the current provider call are discarded.
func (a *Agent) Abort() {
	a.mu.RLock()
	cancel := a.cancel
	a.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
}

// ClearMessages resets the conversation transcript to empty.
func (a *Agent) ClearMessages() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messages = nil
}

// SetMessages replaces the conversation transcript. Used when resuming a session.
func (a *Agent) SetMessages(msgs []types.AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messages = make([]types.AgentMessage, len(msgs))
	copy(a.messages, msgs)
}

// Messages returns a deep copy of the current conversation transcript.
func (a *Agent) Messages() []types.AgentMessage {
	a.mu.RLock()
	defer a.mu.RUnlock()

	cpy := make([]types.AgentMessage, len(a.messages))
	for i, msg := range a.messages {
		cpy[i] = msg
		// Deep-copy content blocks
		cpy[i].Content = make([]types.ContentBlock, len(msg.Content))
		for j, block := range msg.Content {
			cpy[i].Content[j] = block
			if block.ToolCall != nil {
				tc := *block.ToolCall
				cpy[i].Content[j].ToolCall = &tc
			}
			if block.Image != nil {
				img := *block.Image
				cpy[i].Content[j].Image = &img
			}
		}
	}
	return cpy
}

// State returns the current agent state.
func (a *Agent) State() AgentState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.state
}

// LastUsage returns the token usage from the most recent completed
// LLM turn. Returns an empty Usage struct if no turns have completed.
func (a *Agent) LastUsage() types.Usage {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastTurnUsage
}

// Error returns the error from the last run, if any.
func (a *Agent) Error() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.runErr
}

// newID generates a unique message ID.
func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// now returns the current time. Package-level variable for testing overrides.
var now = time.Now

// buildSystemPrompt combines context files with the SDK-provided system prompt.
// Context files are loaded by discovering paths via config.ContextFileSearchList,
// reading each existing file, and prepending their content.
func (a *Agent) buildSystemPrompt() string {
	if a.cwd == "" {
		return a.systemPrompt
	}

	paths, err := config.ContextFileSearchList(a.cwd)
	if err != nil {
		return a.systemPrompt
	}

	var ctxContent string
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip non-existent files
		}
		ctxContent += fmt.Sprintf("## %s\n\n%s\n\n---\n\n", path, string(data))
	}

	if ctxContent == "" {
		return a.systemPrompt
	}

	return "<context_files>\n" + ctxContent + "</context_files>\n\n" + a.systemPrompt
}

// setState updates the agent state thread-safely.
func (a *Agent) setState(s AgentState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = s
}

// drainSteerQueue removes and returns all messages from the steering queue.
func (a *Agent) drainSteerQueue() []types.AgentMessage {
	a.steerMu.Lock()
	defer a.steerMu.Unlock()

	msgs := a.steerQueue
	a.steerQueue = nil
	return msgs
}

// drainFollowUpQueue removes and returns all messages from the follow-up queue.
func (a *Agent) drainFollowUpQueue() []types.AgentMessage {
	a.followUpMu.Lock()
	defer a.followUpMu.Unlock()

	msgs := a.followUpQueue
	a.followUpQueue = nil
	return msgs
}

// addMessage appends a message to the transcript thread-safely.
func (a *Agent) addMessage(msg types.AgentMessage) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.messages = append(a.messages, msg)
}

// SetThinkingLevel sets the thinking level for the agent.
func (a *Agent) SetThinkingLevel(level types.ThinkingLevel) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.thinkingLevel = level
}

// ThinkingLevel returns the current thinking level.
func (a *Agent) ThinkingLevel() types.ThinkingLevel {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.thinkingLevel
}

// SetModel swaps the provider and model on the existing agent without
// clearing the conversation transcript. This allows mid-conversation model
// switching while preserving full context — matching PI's architecture
// where model is a mutable state property, not an agent recreation.
func (a *Agent) SetModel(prov provider.Provider, model types.Model) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.provider = prov
	a.model = model
}

// Model returns the current model.
func (a *Agent) Model() types.Model {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.model
}

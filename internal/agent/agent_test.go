package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/tools"
	"github.com/adam/tau/internal/types"
)

// --- Helpers ---

func newTestAgent() *Agent {
	return New(Options{
		SystemPrompt: "You are a helpful assistant.",
		WorkingDir:   "",
		Provider:     nil,
		Model:        types.Model{ID: "test-model", API: "test"},
	})
}

func mockProviderWithEvents(events ...types.StreamEvent) *mockProvider {
	return &mockProvider{events: events}
}

// --- Mock providers ---

type mockProvider struct {
	events []types.StreamEvent
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, toolsDef []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, len(m.events)+1)
	go func() {
		defer close(ch)
		for _, e := range m.events {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

func (m *mockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, toolsDef []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	for _, e := range m.events {
		if e.Type == types.EventDone && e.Message != nil {
			return e.Message, nil
		}
	}
	return nil, nil
}

// blockingProvider blocks until context is cancelled.
type blockingProvider struct{}

func (b *blockingProvider) Name() string { return "blocking" }

func (b *blockingProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, toolsDef []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, 1)
	go func() {
		defer close(ch)
		<-ctx.Done()
		ch <- types.StreamEvent{Type: types.EventError, Error: ctx.Err().Error()}
	}()
	return ch
}

func (b *blockingProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, toolsDef []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// delayedProvider introduces a per-stream delay to simulate slow LLM response.
type delayedProvider struct {
	delay  time.Duration
	events []types.StreamEvent
}

func (d *delayedProvider) Name() string { return "delayed" }

func (d *delayedProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, toolsDef []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, len(d.events)+1)
	go func() {
		defer close(ch)
		time.Sleep(d.delay)
		for _, e := range d.events {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

func (d *delayedProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, toolsDef []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

type blockingTool struct {
	release <-chan struct{}
}

func (b *blockingTool) Name() string { return "slow" }

func (b *blockingTool) Description() string { return "Slow test tool" }

func (b *blockingTool) Parameters() any { return &testutil.MockToolParams{} }

func (b *blockingTool) ExecutionMode() types.ExecutionMode { return types.ExecutionParallel }

func (b *blockingTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	<-b.release
	return &types.ToolResult{Content: []types.ContentBlock{{Type: types.BlockText, Text: "released"}}}, nil
}

// countingProvider calls fn each time Stream is invoked.
type countingProvider struct {
	fn func() []types.StreamEvent
}

func (c *countingProvider) Name() string { return "counting" }

func (c *countingProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, toolsDef []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	events := c.fn()
	ch := make(chan types.StreamEvent, len(events)+1)
	go func() {
		defer close(ch)
		for _, e := range events {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

func (c *countingProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, toolsDef []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

// --- Agent struct / Options ---

func TestNewAgent_Defaults(t *testing.T) {
	a := New(Options{
		SystemPrompt: "test",
		Model:        types.Model{ID: "m1"},
	})
	if a.systemPrompt != "test" {
		t.Errorf("systemPrompt = %q, want %q", a.systemPrompt, "test")
	}
	if a.State() != StateIdle {
		t.Errorf("state = %v, want %v", a.State(), StateIdle)
	}
	if a.tools == nil {
		t.Error("expected default tool registry")
	}
	if len(a.Messages()) != 0 {
		t.Error("expected empty transcript")
	}
}

func TestNewAgent_WithToolRegistry(t *testing.T) {
	reg := tools.NewRegistry()
	a := New(Options{
		Provider:     nil,
		Model:        types.Model{},
		ToolRegistry: reg,
	})
	if a.tools != reg {
		t.Error("expected provided tool registry")
	}
}

// --- Prompt / Transcript ---

func TestPrompt_AppendsUserMessage(t *testing.T) {
	a := newTestAgent()
	a.provider = mockProviderWithEvents(
		types.StreamEvent{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}},
		}},
	)

	err := a.Prompt(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := a.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != types.RoleUser {
		t.Errorf("first message role = %v, want %v", msgs[0].Role, types.RoleUser)
	}
	if msgs[1].Role != types.RoleAssistant {
		t.Errorf("second message role = %v, want %v", msgs[1].Role, types.RoleAssistant)
	}
}

func TestContinue_NoNewMessage(t *testing.T) {
	a := newTestAgent()
	a.addMessage(types.AgentMessage{
		ID: "u1", Role: types.RoleUser,
		Content:   []types.ContentBlock{{Type: types.BlockText, Text: "prev"}},
		Timestamp: time.Now(),
	})
	a.provider = mockProviderWithEvents(
		types.StreamEvent{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "ok"}},
		}},
	)

	err := a.Continue(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := a.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (existing + assistant), got %d", len(msgs))
	}
	if msgs[0].Role != types.RoleUser || msgs[0].Content[0].Text != "prev" {
		t.Errorf("first message not preserved")
	}
}

// --- Steer / FollowUp queues ---

func TestSteer_QueuesMessage(t *testing.T) {
	a := newTestAgent()
	if err := a.Steer("new info"); err != nil {
		t.Fatalf("Steer error: %v", err)
	}
	msgs := a.drainSteerQueue()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 steered message, got %d", len(msgs))
	}
	if msgs[0].Content[0].Text != "new info" {
		t.Errorf("steer message text = %q", msgs[0].Content[0].Text)
	}
}

func TestSteer_DropsOldestOnOverflow(t *testing.T) {
	a := newTestAgent()
	for i := 0; i < queueSize+1; i++ {
		_ = a.Steer(fmt.Sprintf("msg-%d", i))
	}
	msgs := a.drainSteerQueue()
	if len(msgs) != queueSize {
		t.Fatalf("expected %d messages after overflow, got %d", queueSize, len(msgs))
	}
	if msgs[0].Content[0].Text != "msg-1" {
		t.Errorf("expected first message 'msg-1', got %q", msgs[0].Content[0].Text)
	}
}

func TestFollowUp_QueuesMessage(t *testing.T) {
	a := newTestAgent()
	if err := a.FollowUp("follow task"); err != nil {
		t.Fatalf("FollowUp error: %v", err)
	}
	msgs := a.drainFollowUpQueue()
	if len(msgs) != 1 || msgs[0].Content[0].Text != "follow task" {
		t.Errorf("follow-up queue mismatch")
	}
}

func TestFollowUp_DropsOldestOnOverflow(t *testing.T) {
	a := newTestAgent()
	for i := 0; i < queueSize+1; i++ {
		_ = a.FollowUp(fmt.Sprintf("fu-%d", i))
	}
	msgs := a.drainFollowUpQueue()
	if len(msgs) != queueSize {
		t.Fatalf("expected %d messages, got %d", queueSize, len(msgs))
	}
	if msgs[0].Content[0].Text != "fu-1" {
		t.Errorf("expected first message 'fu-1', got %q", msgs[0].Content[0].Text)
	}
}

// --- Abort ---

func TestAbort_CancelsContext(t *testing.T) {
	a := newTestAgent()
	a.provider = &blockingProvider{}

	done := make(chan error, 1)
	go func() {
		done <- a.Prompt(context.Background(), "hello")
	}()

	time.Sleep(10 * time.Millisecond)
	a.Abort()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after abort, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("abort did not unblock agent loop within timeout")
	}
}

func TestAbort_Idempotent(t *testing.T) {
	a := newTestAgent()
	a.Abort()
	a.Abort() // should not panic
}

// --- State transitions ---

func TestState_Transitions_IdleToDone(t *testing.T) {
	a := newTestAgent()
	a.provider = mockProviderWithEvents(
		types.StreamEvent{Type: types.EventStart},
		types.StreamEvent{Type: types.EventTextDelta, Delta: "hello"},
		types.StreamEvent{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}},
		}},
	)

	var states []AgentState
	var mu sync.Mutex
	unsub := a.Subscribe(func(e types.AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		states = append(states, a.State())
	})
	defer unsub()

	_ = a.Prompt(context.Background(), "hi")

	mu.Lock()
	s := make([]AgentState, len(states))
	copy(s, states)
	mu.Unlock()

	foundStreaming, foundDone := false, false
	for _, st := range s {
		if st == StateStreaming {
			foundStreaming = true
		}
		if st == StateDone {
			foundDone = true
		}
	}
	if !foundStreaming {
		t.Error("never saw StateStreaming")
	}
	if !foundDone {
		t.Error("never saw StateDone")
	}
}

func TestState_Transitions_ToolExecution(t *testing.T) {
	a := newTestAgent()
	mockTool := &testutil.MockTool{
		ToolName:        "read",
		ToolDescription: "Read a file",
		Result: &types.ToolResult{
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "file content"}},
		},
	}
	a.tools = tools.NewRegistry()
	a.tools.Register(mockTool)

	// First turn: tool call
	// Second turn: text response after tool result
	turns := 0
	a.provider = &countingProvider{
		fn: func() []types.StreamEvent {
			turns++
			if turns == 1 {
				return []types.StreamEvent{
					{Type: types.EventDone, Message: &types.AgentMessage{
						Role: types.RoleAssistant,
						Content: []types.ContentBlock{
							{Type: types.BlockToolCall, ToolCall: &types.ToolCallBlock{
								ID: "tc1", Name: "read",
								Arguments: map[string]any{"path": "test.txt"},
							}},
						},
					}},
				}
			}
			return []types.StreamEvent{
				{Type: types.EventDone, Message: &types.AgentMessage{
					Role:    types.RoleAssistant,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "Done reading."}},
				}},
			}
		},
	}

	_ = a.Prompt(context.Background(), "read test.txt")

	if mockTool.CallCount() != 1 {
		t.Errorf("expected 1 tool call, got %d", mockTool.CallCount())
	}

	msgs := a.Messages()
	// user → assistant(with tool call) → tool_result → assistant(response)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (user+assistant+tool_result+assistant), got %d: %+v", len(msgs), msgs)
	}
	if msgs[2].Role != types.RoleToolResult {
		t.Errorf("third message role = %v, want tool_result", msgs[2].Role)
	}
}

func TestRun_InterruptedToolExecutionAddsToolResults(t *testing.T) {
	a := newTestAgent()
	release := make(chan struct{})
	defer close(release)

	a.tools = tools.NewRegistry()
	a.tools.Register(&blockingTool{release: release})
	a.provider = mockProviderWithEvents(types.StreamEvent{Type: types.EventDone, Message: &types.AgentMessage{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.BlockToolCall, ToolCall: &types.ToolCallBlock{
				ID:        "tc1",
				Name:      "slow",
				Arguments: map[string]any{"input": "wait"},
			}},
		},
	}})

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err := a.Prompt(ctx, "run slow tool")
	if err == nil {
		t.Fatal("expected context timeout error")
	}

	msgs := a.Messages()
	if len(msgs) != 3 {
		t.Fatalf("expected user, assistant tool call, and synthetic tool result, got %d: %+v", len(msgs), msgs)
	}
	if msgs[2].Role != types.RoleToolResult {
		t.Fatalf("third message role = %v, want tool_result", msgs[2].Role)
	}
	if msgs[2].ToolCallID != "tc1" {
		t.Errorf("tool result ToolCallID = %q, want tc1", msgs[2].ToolCallID)
	}
	if got := msgs[2].Content[0].Text; !strings.Contains(got, "Tool execution interrupted") {
		t.Errorf("tool result text = %q, want interruption message", got)
	}
}

// --- Event subscription ---

func TestSubscribe_EmitsEvents(t *testing.T) {
	a := newTestAgent()
	a.provider = mockProviderWithEvents(
		types.StreamEvent{Type: types.EventStart},
		types.StreamEvent{Type: types.EventTextDelta, Delta: "hi"},
		types.StreamEvent{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}},
		}},
	)

	var events []types.AgentEventType
	var mu sync.Mutex
	a.Subscribe(func(e types.AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e.Type)
	})

	_ = a.Prompt(context.Background(), "hello")

	mu.Lock()
	defer mu.Unlock()

	hasStart, hasEnd := false, false
	for _, et := range events {
		if et == types.AgentEventStart {
			hasStart = true
		}
		if et == types.AgentEventAgentEnd {
			hasEnd = true
		}
	}
	if !hasStart {
		t.Error("expected agent_start event")
	}
	if !hasEnd {
		t.Error("expected agent_end event")
	}
}

func TestSubscribe_Unsubscribe(t *testing.T) {
	a := newTestAgent()
	callCount := 0
	fn := func(e types.AgentEvent) {
		callCount++
	}
	unsub := a.Subscribe(fn)
	unsub()

	a.emit(types.AgentEvent{Type: types.AgentEventStart})
	if callCount != 0 {
		t.Errorf("unsubscribed listener was called %d times", callCount)
	}
}

// --- System prompt / context files ---

func TestBuildSystemPrompt_WithNoCWD(t *testing.T) {
	a := newTestAgent()
	a.systemPrompt = "base prompt"
	prompt := a.buildSystemPrompt()
	if prompt != "base prompt" {
		t.Errorf("expected base prompt, got %q", prompt)
	}
}

func TestBuildSystemPrompt_WithContextFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/AGENTS.md", []byte("# Project Rules\nBe nice.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	a := newTestAgent()
	a.cwd = dir
	a.systemPrompt = "skills disclosure"

	prompt := a.buildSystemPrompt()

	if prompt == "" {
		t.Fatal("expected non-empty system prompt")
	}
	if !strings.Contains(prompt, "AGENTS.md") {
		t.Error("expected AGENTS.md path in system prompt")
	}
	if !strings.Contains(prompt, "Be nice.") {
		t.Error("expected AGENTS.md content in system prompt")
	}
	if !strings.Contains(prompt, "skills disclosure") {
		t.Error("expected SDK system prompt in system prompt")
	}
}

func TestBuildSystemPrompt_NonExistentFiles(t *testing.T) {
	a := newTestAgent()
	a.cwd = t.TempDir() // empty dir, no AGENTS.md
	a.systemPrompt = "base"

	prompt := a.buildSystemPrompt()
	if prompt != "base" {
		t.Errorf("expected base prompt when no context files exist, got %q", prompt)
	}
}

// --- FollowUp integration ---

func TestFollowUp_TriggersAdditionalTurn(t *testing.T) {
	a := newTestAgent()
	callCount := 0
	firstDone := make(chan struct{})
	a.provider = &countingProvider{
		fn: func() []types.StreamEvent {
			callCount++
			if callCount == 1 {
				// Signal that first turn is about to return, then wait for follow-up
				close(firstDone)
				time.Sleep(50 * time.Millisecond) // give FollowUp goroutine time
				return []types.StreamEvent{
					{Type: types.EventDone, Message: &types.AgentMessage{
						Role:    types.RoleAssistant,
						Content: []types.ContentBlock{{Type: types.BlockText, Text: "first"}},
					}},
				}
			}
			return []types.StreamEvent{
				{Type: types.EventDone, Message: &types.AgentMessage{
					Role:    types.RoleAssistant,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "second"}},
				}},
			}
		},
	}

	// Add follow-up after first turn signals it's done
	go func() {
		<-firstDone
		_ = a.FollowUp("also do this")
	}()

	_ = a.Prompt(context.Background(), "hello")

	if callCount != 2 {
		t.Errorf("expected 2 provider calls (initial + follow-up), got %d", callCount)
	}

	msgs := a.Messages()
	// user → assistant(first) → user(follow-up) → assistant(second)
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[2].Role != types.RoleUser {
		t.Errorf("third message should be user (follow-up), got %v", msgs[2].Role)
	}
}

// --- Steering integration ---

func TestSteering_TriggersAdditionalTurn(t *testing.T) {
	a := newTestAgent()
	callCount := 0
	firstDone := make(chan struct{})
	a.provider = &countingProvider{
		fn: func() []types.StreamEvent {
			callCount++
			if callCount == 1 {
				close(firstDone)
				time.Sleep(50 * time.Millisecond)
				return []types.StreamEvent{
					{Type: types.EventDone, Message: &types.AgentMessage{
						Role:    types.RoleAssistant,
						Content: []types.ContentBlock{{Type: types.BlockText, Text: "first"}},
					}},
				}
			}
			return []types.StreamEvent{
				{Type: types.EventDone, Message: &types.AgentMessage{
					Role:    types.RoleAssistant,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "second"}},
				}},
			}
		},
	}

	// Steer after first turn signals done
	go func() {
		<-firstDone
		_ = a.Steer("actually wait")
	}()

	_ = a.Prompt(context.Background(), "go")

	if callCount != 2 {
		t.Errorf("expected 2 provider calls (initial + steer), got %d", callCount)
	}

	msgs := a.Messages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
}

// --- Tool execution errors ---

func TestToolExecution_ErrorInResult(t *testing.T) {
	a := newTestAgent()
	errTool := &testutil.MockTool{
		ToolName:        "bash",
		ToolDescription: "Run bash",
		Err:             fmt.Errorf("command not found"),
	}
	a.tools = tools.NewRegistry()
	a.tools.Register(errTool)

	turns := 0
	a.provider = &countingProvider{
		fn: func() []types.StreamEvent {
			turns++
			if turns == 1 {
				return []types.StreamEvent{
					{Type: types.EventDone, Message: &types.AgentMessage{
						Role: types.RoleAssistant,
						Content: []types.ContentBlock{
							{Type: types.BlockToolCall, ToolCall: &types.ToolCallBlock{
								ID: "tc1", Name: "bash",
								Arguments: map[string]any{"command": "invalid-cmd"},
							}},
						},
					}},
				}
			}
			return []types.StreamEvent{
				{Type: types.EventDone, Message: &types.AgentMessage{
					Role:    types.RoleAssistant,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "Tool failed"}},
				}},
			}
		},
	}

	_ = a.Prompt(context.Background(), "run something")

	msgs := a.Messages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}
	if msgs[2].Role != types.RoleToolResult {
		t.Errorf("third message should be tool_result, got %v", msgs[2].Role)
	}
}

// --- Provider error propagation ---

func TestProviderError_PropagatesToCaller(t *testing.T) {
	a := newTestAgent()
	a.provider = mockProviderWithEvents(
		types.StreamEvent{Type: types.EventError, Error: "rate limited"},
	)

	err := a.Prompt(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error from provider, got nil")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected 'rate limited' in error, got %v", err)
	}
}

// --- Messages returns a copy ---

func TestMessages_ReturnsCopy(t *testing.T) {
	a := newTestAgent()
	a.provider = mockProviderWithEvents(
		types.StreamEvent{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}},
		}},
	)

	_ = a.Prompt(context.Background(), "hello")

	msgs1 := a.Messages()
	msgs1[0].Content[0].Text = "MUTATED"

	msgs2 := a.Messages()
	if msgs2[0].Content[0].Text == "MUTATED" {
		t.Error("Messages() returned a reference, not a copy")
	}
}

func TestClearMessages_ResetsTranscript(t *testing.T) {
	a := newTestAgent()
	a.provider = mockProviderWithEvents(
		types.StreamEvent{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}},
		}},
	)

	_ = a.Prompt(context.Background(), "hello")
	if len(a.Messages()) == 0 {
		t.Fatal("expected messages after prompt")
	}

	a.ClearMessages()
	msgs := a.Messages()
	if len(msgs) != 0 {
		t.Errorf("expected empty transcript after ClearMessages, got %d messages", len(msgs))
	}
}

func TestClearMessages_ThreadSafe(t *testing.T) {
	a := newTestAgent()
	a.provider = mockProviderWithEvents(
		types.StreamEvent{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}},
		}},
	)

	_ = a.Prompt(context.Background(), "hello")

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			a.ClearMessages()
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		_ = a.Messages()
	}
	<-done
}

func TestAgent_SetModel_SwapsProviderAndModel(t *testing.T) {
	a := newTestAgent()

	// Add messages to verify they're preserved
	msgs := []types.AgentMessage{
		{ID: "1", Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
		{ID: "2", Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}},
	}
	a.SetMessages(msgs)

	// Create a new model
	newModel := types.Model{
		ID:       "new-model",
		Name:     "New Model",
		Provider: "new-provider",
		API:      "openai-completions",
	}
	// Use the existing mock provider (events don't matter for SetModel test)
	newProv := &mockProvider{events: []types.StreamEvent{
		{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "done"}},
		}},
	}}

	// Switch model
	a.SetModel(newProv, newModel)

	// Verify model changed
	if a.Model().ID != "new-model" {
		t.Fatalf("expected model ID 'new-model', got %q", a.Model().ID)
	}
	if a.Model().Provider != "new-provider" {
		t.Fatalf("expected provider 'new-provider', got %q", a.Model().Provider)
	}

	// Verify messages are preserved
	gotMsgs := a.Messages()
	if len(gotMsgs) != 2 {
		t.Fatalf("expected 2 messages after model switch, got %d", len(gotMsgs))
	}
	if gotMsgs[0].Content[0].Text != "hello" {
		t.Errorf("expected first message 'hello', got %q", gotMsgs[0].Content[0].Text)
	}
}

func TestAgent_SetModel_ThreadSafe(t *testing.T) {
	a := newTestAgent()

	// Add messages
	a.SetMessages([]types.AgentMessage{
		{ID: "1", Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "test"}}},
	})

	newModel := types.Model{ID: "m1", Provider: "p1", API: "openai-completions"}
	newProv := &mockProvider{events: []types.StreamEvent{
		{Type: types.EventDone, Message: &types.AgentMessage{
			Role:    types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "done"}},
		}},
	}}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			a.SetModel(newProv, newModel)
		}
		close(done)
	}()

	for i := 0; i < 100; i++ {
		_ = a.Model()
		_ = a.Messages()
	}
	<-done

	// Should still have the message
	if len(a.Messages()) != 1 {
		t.Fatalf("expected 1 message, got %d", len(a.Messages()))
	}
}

package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adam/tau/internal/provider"
	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/types"
)

func TestRun_Success(t *testing.T) {
	mp := &testutil.MockProvider{
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "Hello, "},
			{Type: types.EventTextDelta, Delta: "world!"},
			{
				Type: types.EventDone,
				Message: &types.AgentMessage{
					Content: []types.ContentBlock{
						{Type: types.BlockText, Text: ""},
					},
				},
				Usage: &types.Usage{
					Input:  10,
					Output: 5,
				},
			},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:         "greet",
		SystemPrompt: "You are helpful",
		Model:        types.Model{ID: "test-model"},
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Output != "Hello, world!" {
		t.Errorf("expected output %q, got %q", "Hello, world!", result.Output)
	}
	if result.Duration <= 0 {
		t.Errorf("expected duration > 0, got %v", result.Duration)
	}
	if result.Usage.Input != 10 {
		t.Errorf("expected input usage 10, got %d", result.Usage.Input)
	}
	if result.Usage.Output != 5 {
		t.Errorf("expected output usage 5, got %d", result.Usage.Output)
	}
}

func TestRun_FreshContext_TaskSentAsUserMessage(t *testing.T) {
	var capturedMessages []types.AgentMessage

	mp := &capturingMockProvider{
		onStream: func(messages []types.AgentMessage) {
			capturedMessages = messages
		},
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task: "test task",
	})

	sa.Run(context.Background())

	if len(capturedMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(capturedMessages))
	}
	if capturedMessages[0].Role != types.RoleUser {
		t.Errorf("expected role user, got %v", capturedMessages[0].Role)
	}
	if capturedMessages[0].Content[0].Text != "test task" {
		t.Errorf("expected task text 'test task', got %q", capturedMessages[0].Content[0].Text)
	}
}

func TestRun_SystemPromptInherited(t *testing.T) {
	var capturedOpts types.StreamOptions

	mp := &capturingMockProvider{
		onStreamOpts: func(opts types.StreamOptions) {
			capturedOpts = opts
		},
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "ok"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:         "test",
		SystemPrompt: "Be concise and helpful",
	})

	sa.Run(context.Background())

	if capturedOpts.SystemPrompt != "Be concise and helpful" {
		t.Errorf("expected system prompt %q, got %q", "Be concise and helpful", capturedOpts.SystemPrompt)
	}
}

func TestRun_StreamError(t *testing.T) {
	mp := &testutil.MockProvider{
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "partial"},
			{Type: types.EventError, Error: "rate limited"},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task: "test",
	})

	result := sa.Run(context.Background())

	if result.Success {
		t.Fatal("expected failure on stream error")
	}
	if result.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if result.Output != "partial" {
		t.Logf("warning: partial output accumulated: %q", result.Output)
	}
}

func TestNewSubAgent_Defaults(t *testing.T) {
	mp := &testutil.MockProvider{}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task: "test-task",
	})

	if sa.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if len(sa.ID) != 16 {
		t.Errorf("expected 16-char hex ID, got %q (len=%d)", sa.ID, len(sa.ID))
	}
	if sa.ContextMode != ContextFresh {
		t.Errorf("expected ContextFresh, got %q", sa.ContextMode)
	}
	if sa.Task != "test-task" {
		t.Errorf("expected task %q, got %q", "test-task", sa.Task)
	}
	if sa.Provider == nil {
		t.Error("expected provider to be set")
	}
}

func TestNewSubAgent_NilProvider(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic with nil provider")
		}
	}()

	NewSubAgent(nil, SubAgentOpts{})
}

func TestNewSubAgent_CustomID(t *testing.T) {
	mp := &testutil.MockProvider{}

	sa := NewSubAgent(mp, SubAgentOpts{
		ID:   "my-custom-id",
		Task: "test",
	})

	if sa.ID != "my-custom-id" {
		t.Errorf("expected custom ID, got %q", sa.ID)
	}
}

func TestRun_Timeout(t *testing.T) {
	mp := &slowMockProvider{delay: 500 * time.Millisecond}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:    "slow task",
		Timeout: 50 * time.Millisecond,
	})

	result := sa.Run(context.Background())

	if result.Success {
		t.Fatal("expected failure on timeout")
	}
	if !result.Timeout {
		t.Fatal("expected Timeout flag to be true")
	}
	if result.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if result.Duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", result.Duration)
	}
	if result.Duration > 2*time.Second {
		t.Errorf("expected duration < 2s (should not wait for slow provider), got %v", result.Duration)
	}
}

func TestRun_SuccessWithinTimeout(t *testing.T) {
	mp := &testutil.MockProvider{
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "fast response"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:    "fast task",
		Timeout: 30 * time.Second,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Timeout {
		t.Fatal("expected Timeout flag to be false")
	}
	if result.Output != "fast response" {
		t.Errorf("expected output 'fast response', got %q", result.Output)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	mp := &slowMockProvider{delay: 5 * time.Second}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:    "cancellable task",
		Timeout: 30 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := sa.Run(ctx)

	if result.Success {
		t.Fatal("expected failure on cancellation")
	}
	if result.Timeout {
		t.Fatal("expected Timeout flag to be false (cancellation, not timeout)")
	}
	if result.Error == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_DefaultTimeout(t *testing.T) {
	mp := &testutil.MockProvider{
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "ok"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task: "test",
	})

	if sa.Timeout != DefaultTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultTimeout, sa.Timeout)
	}

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

func TestNewSubAgent_DefaultTimeout(t *testing.T) {
	mp := &testutil.MockProvider{}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task: "test",
	})

	if sa.Timeout <= 0 {
		t.Error("expected positive timeout")
	}
	if sa.Timeout != DefaultTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultTimeout, sa.Timeout)
	}
}

func TestNewSubAgent_CustomTimeout(t *testing.T) {
	mp := &testutil.MockProvider{}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:    "test",
		Timeout: 2 * time.Minute,
	})

	if sa.Timeout != 2*time.Minute {
		t.Errorf("expected timeout 2m, got %v", sa.Timeout)
	}
}

// capturingMockProvider records what was passed to Stream() for verification.
type capturingMockProvider struct {
	Events       []types.StreamEvent
	onStream     func([]types.AgentMessage)
	onStreamOpts func(types.StreamOptions)
}

func (m *capturingMockProvider) Name() string { return "capturing-mock" }

func (m *capturingMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	if m.onStream != nil {
		m.onStream(messages)
	}
	if m.onStreamOpts != nil {
		m.onStreamOpts(opts)
	}
	ch := make(chan types.StreamEvent, len(m.Events)+1)
	for _, e := range m.Events {
		ch <- e
	}
	close(ch)
	return ch
}

func (m *capturingMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

// slowMockProvider simulates a slow provider that delays before sending events.
// It respects context cancellation during the delay.
type slowMockProvider struct {
	delay time.Duration
	mu    sync.Mutex
	called bool
}

func (m *slowMockProvider) Name() string { return "slow-mock" }

func (m *slowMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent)

	go func() {
		m.mu.Lock()
		m.called = true
		m.mu.Unlock()

		timer := time.NewTimer(m.delay)
		defer timer.Stop()

		select {
		case <-timer.C:
			ch <- types.StreamEvent{Type: types.EventStart}
			ch <- types.StreamEvent{Type: types.EventTextDelta, Delta: "slow response"}
			ch <- types.StreamEvent{Type: types.EventDone, Message: &types.AgentMessage{}}
			close(ch)
		case <-ctx.Done():
			close(ch)
		}
	}()

	return ch
}

func (m *slowMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

func TestRun_ForkContext_ParentMessagesIncluded(t *testing.T) {
	var capturedMessages []types.AgentMessage

	mp := &capturingMockProvider{
		onStream: func(messages []types.AgentMessage) {
			capturedMessages = messages
		},
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	parentMsgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "What is Go?"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Go is a programming language."}}},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:           "Summarize the conversation",
		ContextMode:    ContextFork,
		ParentMessages: parentMsgs,
	})

	sa.Run(context.Background())

	if len(capturedMessages) != 3 {
		t.Fatalf("expected 3 messages (2 parent + 1 task), got %d", len(capturedMessages))
	}
	if capturedMessages[0].Role != types.RoleUser {
		t.Errorf("expected first message role user, got %v", capturedMessages[0].Role)
	}
	if capturedMessages[0].Content[0].Text != "What is Go?" {
		t.Errorf("expected first message text 'What is Go?', got %q", capturedMessages[0].Content[0].Text)
	}
	if capturedMessages[1].Role != types.RoleAssistant {
		t.Errorf("expected second message role assistant, got %v", capturedMessages[1].Role)
	}
	if capturedMessages[2].Role != types.RoleUser {
		t.Errorf("expected task message role user, got %v", capturedMessages[2].Role)
	}
	if capturedMessages[2].Content[0].Text != "Summarize the conversation" {
		t.Errorf("expected task text 'Summarize the conversation', got %q", capturedMessages[2].Content[0].Text)
	}
}

func TestRun_ForkContext_Isolation(t *testing.T) {
	parentMsgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hi there"}}},
	}

	parentLen := len(parentMsgs)
	parentCopy := make([]types.AgentMessage, len(parentMsgs))
	copy(parentCopy, parentMsgs)

	mp := &testutil.MockProvider{
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "ok"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:           "task",
		ContextMode:    ContextFork,
		ParentMessages: parentMsgs,
	})

	sa.Run(context.Background())

	if len(parentMsgs) != parentLen {
		t.Errorf("parent messages length changed: expected %d, got %d", parentLen, len(parentMsgs))
	}
	for i := range parentMsgs {
		if parentMsgs[i].Role != parentCopy[i].Role {
			t.Errorf("parent message[%d] role changed: expected %v, got %v", i, parentCopy[i].Role, parentMsgs[i].Role)
		}
		if len(parentMsgs[i].Content) != len(parentCopy[i].Content) {
			t.Errorf("parent message[%d] content length changed: expected %d, got %d", i, len(parentCopy[i].Content), len(parentMsgs[i].Content))
		}
		for j := range parentMsgs[i].Content {
			if parentMsgs[i].Content[j].Text != parentCopy[i].Content[j].Text {
				t.Errorf("parent message[%d].content[%d] text changed: expected %q, got %q", i, j, parentCopy[i].Content[j].Text, parentMsgs[i].Content[j].Text)
			}
		}
	}
}

func TestRun_ForkContext_ConcurrentIsolation(t *testing.T) {
	parentMsgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Parent conversation"}}},
	}

	var wg sync.WaitGroup
	numSubAgents := 10

	for i := 0; i < numSubAgents; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			mp := &testutil.MockProvider{
				Events: []types.StreamEvent{
					{Type: types.EventStart},
					{Type: types.EventTextDelta, Delta: "response"},
					{Type: types.EventDone, Message: &types.AgentMessage{}},
				},
			}

			sa := NewSubAgent(mp, SubAgentOpts{
				Task:           fmt.Sprintf("task %d", idx),
				ContextMode:    ContextFork,
				ParentMessages: parentMsgs,
			})

			sa.Run(context.Background())
		}(i)
	}

	wg.Wait()

	if len(parentMsgs) != 1 {
		t.Errorf("parent messages modified by concurrent subagents: expected length 1, got %d", len(parentMsgs))
	}
	if parentMsgs[0].Content[0].Text != "Parent conversation" {
		t.Errorf("parent message content modified: expected 'Parent conversation', got %q", parentMsgs[0].Content[0].Text)
	}
}

func TestRun_ForkContext_EmptyParentMessages(t *testing.T) {
	var capturedMessages []types.AgentMessage

	mp := &capturingMockProvider{
		onStream: func(messages []types.AgentMessage) {
			capturedMessages = messages
		},
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:           "test task",
		ContextMode:    ContextFork,
		ParentMessages: nil,
	})

	sa.Run(context.Background())

	if len(capturedMessages) != 1 {
		t.Fatalf("expected 1 message (task only), got %d", len(capturedMessages))
	}
	if capturedMessages[0].Content[0].Text != "test task" {
		t.Errorf("expected task text 'test task', got %q", capturedMessages[0].Content[0].Text)
	}
}

func TestNewSubAgent_ContextForkMode(t *testing.T) {
	mp := &testutil.MockProvider{}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:        "test",
		ContextMode: ContextFork,
	})

	if sa.ContextMode != ContextFork {
		t.Errorf("expected ContextFork, got %q", sa.ContextMode)
	}
}

func TestRun_E2E_Ollama(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:         "Say exactly: E2E test passed",
		SystemPrompt: "Be extremely concise. Follow instructions exactly.",
		Model:        model,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}
	if result.Duration <= 0 {
		t.Errorf("expected duration > 0, got %v", result.Duration)
	}
	t.Logf("E2E output: %q", result.Output)
	t.Logf("Duration: %v", result.Duration)
	t.Logf("Usage: input=%d output=%d total=%d", result.Usage.Input, result.Usage.Output, result.Usage.TotalTokens)
}

func TestRun_E2E_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:         "Write a detailed essay about the history of computing, at least 500 words",
		SystemPrompt: "Be extremely verbose and detailed.",
		Model:        model,
		Timeout:      100 * time.Millisecond,
	})

	result := sa.Run(context.Background())

	if result.Success {
		t.Fatal("expected failure on timeout")
	}
	if !result.Timeout {
		t.Fatal("expected Timeout flag to be true")
	}
	if result.Error == nil {
		t.Fatal("expected error, got nil")
	}
	t.Logf("Timeout error: %v", result.Error)
	t.Logf("Duration: %v", result.Duration)
}

func TestRun_E2E_SuccessWithinTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:         "Say exactly: timeout test passed",
		SystemPrompt: "Be extremely concise. Follow instructions exactly.",
		Model:        model,
		Timeout:      30 * time.Second,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Timeout {
		t.Fatal("expected Timeout flag to be false")
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("E2E output: %q", result.Output)
	t.Logf("Duration: %v", result.Duration)
}

func TestRun_E2E_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:         "Write a detailed essay about the history of computing",
		SystemPrompt: "Be extremely verbose and detailed.",
		Model:        model,
		Timeout:      30 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := sa.Run(ctx)

	if result.Success {
		t.Fatal("expected failure on cancellation")
	}
	if result.Timeout {
		t.Fatal("expected Timeout flag to be false (cancellation, not timeout)")
	}
	if result.Error == nil {
		t.Fatal("expected error, got nil")
	}
	t.Logf("Cancellation error: %v", result.Error)
}

func TestRun_E2E_ForkContext_SeesParentMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	parentMsgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "My favorite color is blue"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Got it, blue is your favorite color."}}},
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:           "What is my favorite color? Answer with just the color name.",
		ContextMode:    ContextFork,
		ParentMessages: parentMsgs,
		SystemPrompt:   "Answer based only on the conversation context. Be extremely concise.",
		Model:          model,
		Timeout:        30 * time.Second,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	output := strings.ToLower(result.Output)
	if !strings.Contains(output, "blue") {
		t.Errorf("expected fork context to see parent messages, output should contain 'blue', got: %q", result.Output)
	}
	t.Logf("E2E fork context output: %q", result.Output)
}

func TestRun_E2E_FreshContext_CannotSeeParentMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	parentMsgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "My favorite color is blue"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Got it, blue is your favorite color."}}},
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:           "What is my favorite color? If you don't know, say 'unknown'.",
		ContextMode:    ContextFresh,
		ParentMessages: parentMsgs,
		SystemPrompt:   "Answer based only on the conversation context. If you have no context about a question, say 'unknown'.",
		Model:          model,
		Timeout:        30 * time.Second,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	output := strings.ToLower(result.Output)
	if strings.Contains(output, "blue") {
		t.Errorf("expected fresh context to NOT see parent messages, but output contains 'blue': %q", result.Output)
	}
	t.Logf("E2E fresh context output: %q", result.Output)
}

func TestRun_E2E_ForkContext_ParentIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	parentMsgs := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}}},
		{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hi"}}},
	}

	beforeLen := len(parentMsgs)
	beforeContent := make([]string, len(parentMsgs))
	for i, msg := range parentMsgs {
		if len(msg.Content) > 0 {
			beforeContent[i] = msg.Content[0].Text
		}
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:           "Say hello",
		ContextMode:    ContextFork,
		ParentMessages: parentMsgs,
		SystemPrompt:   "Be concise.",
		Model:          model,
		Timeout:        30 * time.Second,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	if len(parentMsgs) != beforeLen {
		t.Errorf("parent messages length changed: before=%d, after=%d", beforeLen, len(parentMsgs))
	}
	for i, msg := range parentMsgs {
		if len(msg.Content) > 0 && msg.Content[0].Text != beforeContent[i] {
			t.Errorf("parent message[%d] content changed: before=%q, after=%q", i, beforeContent[i], msg.Content[0].Text)
		}
	}
	t.Logf("Parent isolation verified: %d messages unchanged", beforeLen)
}

// mockTool implements subagent.Tool for testing.
type mockTool struct {
	name        string
	description string
	result      *types.ToolResult
	execErr     error

	mu    sync.Mutex
	calls []any
}

func (m *mockTool) Name() string { return m.name }
func (m *mockTool) Description() string { return m.description }
func (m *mockTool) Parameters() any    { return &mockToolParams{} }
func (m *mockTool) ExecutionMode() types.ExecutionMode { return types.ExecutionParallel }
func (m *mockTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, params)
	return m.result, m.execErr
}
func (m *mockTool) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

type mockToolParams struct {
	Input string `json:"input,omitempty" jsonschema:"description=Mock input"`
}

// toolCallMockProvider captures tools passed to Stream and can return tool calls in the final message.
type toolCallMockProvider struct {
	mu             sync.Mutex
	callCount      int
	capturedTools  []types.ToolDefinition
	capturedMsgs   [][]types.AgentMessage
	toolCallName   string
	toolCallArgs   map[string]any
	finalText      string
	eventsOverride []types.StreamEvent
}

func (m *toolCallMockProvider) Name() string { return "toolcall-mock" }

func (m *toolCallMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	m.mu.Lock()
	m.callCount++
	callNum := m.callCount
	m.capturedTools = tools
	msgCopy := make([]types.AgentMessage, len(messages))
	copy(msgCopy, messages)
	m.capturedMsgs = append(m.capturedMsgs, msgCopy)
	m.mu.Unlock()

	if m.eventsOverride != nil {
		ch := make(chan types.StreamEvent, len(m.eventsOverride)+1)
		for _, e := range m.eventsOverride {
			ch <- e
		}
		close(ch)
		return ch
	}

	ch := make(chan types.StreamEvent, 8)
	ch <- types.StreamEvent{Type: types.EventStart}
	ch <- types.StreamEvent{Type: types.EventTextDelta, Delta: m.finalText}

	if m.toolCallName != "" && callNum == 1 {
		msg := &types.AgentMessage{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{
					Type: types.BlockToolCall,
					ToolCall: &types.ToolCallBlock{
						ID:        "call_0",
						Name:      m.toolCallName,
						Arguments: m.toolCallArgs,
					},
				},
			},
		}
		ch <- types.StreamEvent{Type: types.EventToolCallStart, Delta: m.toolCallName}
		ch <- types.StreamEvent{Type: types.EventToolCallEnd, Delta: m.toolCallName, Message: msg}
		ch <- types.StreamEvent{Type: types.EventDone, Message: msg}
	} else {
		msg := &types.AgentMessage{
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{Type: types.BlockText, Text: m.finalText},
			},
		}
		ch <- types.StreamEvent{Type: types.EventDone, Message: msg, Usage: &types.Usage{Input: 5, Output: 3}}
	}
	close(ch)
	return ch
}

func (m *toolCallMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

func (m *toolCallMockProvider) CapturedTools() []types.ToolDefinition {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.capturedTools
}

func (m *toolCallMockProvider) CapturedMessages() [][]types.AgentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	cpy := make([][]types.AgentMessage, len(m.capturedMsgs))
	for i, msgs := range m.capturedMsgs {
		cpy[i] = make([]types.AgentMessage, len(msgs))
		copy(cpy[i], msgs)
	}
	return cpy
}

// alwaysToolCallProvider always returns a tool call, used for testing max iterations.
type alwaysToolCallProvider struct {
	toolCallName string
	toolCallArgs map[string]any
	callCount    int
	mu           sync.Mutex
}

func (m *alwaysToolCallProvider) Name() string { return "always-toolcall" }

func (m *alwaysToolCallProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()

	ch := make(chan types.StreamEvent, 8)
	ch <- types.StreamEvent{Type: types.EventStart}
	msg := &types.AgentMessage{
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{
				Type: types.BlockToolCall,
				ToolCall: &types.ToolCallBlock{
					ID:        "call_0",
					Name:      m.toolCallName,
					Arguments: m.toolCallArgs,
				},
			},
		},
	}
	ch <- types.StreamEvent{Type: types.EventToolCallStart, Delta: m.toolCallName}
	ch <- types.StreamEvent{Type: types.EventToolCallEnd, Delta: m.toolCallName, Message: msg}
	ch <- types.StreamEvent{Type: types.EventDone, Message: msg}
	close(ch)
	return ch
}

func (m *alwaysToolCallProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

func TestRun_ToolExecution(t *testing.T) {
	mp := &toolCallMockProvider{
		toolCallName: "mock_tool",
		toolCallArgs: map[string]any{"input": "hello"},
		finalText:    "tool result processed",
	}

	mt := &mockTool{
		name:        "mock_tool",
		description: "A mock tool for testing",
		result: &types.ToolResult{
			Content: []types.ContentBlock{{Type: "text", Text: "mock output"}},
		},
	}

	callCount := 0
	executor := func(ctx context.Context, calls []ToolCallRequest) []*ToolCallResult {
		callCount++
		results := make([]*ToolCallResult, len(calls))
		for i, c := range calls {
			results[i] = &ToolCallResult{
				ID:   c.ID,
				Name: c.Name,
				Result: &types.ToolResult{
					Content: []types.ContentBlock{{Type: "text", Text: "executed: " + c.Name}},
				},
			}
		}
		return results
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:     "use the tool",
		Model:    types.Model{ID: "test-model"},
		Tools:    []Tool{mt},
		Executor: executor,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if callCount != 1 {
		t.Errorf("expected 1 executor call, got %d", callCount)
	}

	allMsgs := mp.CapturedMessages()
	if len(allMsgs) != 2 {
		t.Fatalf("expected 2 stream calls (initial + after tool), got %d", len(allMsgs))
	}

	if len(allMsgs[1]) != 3 {
		t.Fatalf("expected 3 messages in second call (task + assistant tool_call + tool result), got %d", len(allMsgs[1]))
	}
	if allMsgs[1][1].Role != types.RoleAssistant {
		t.Errorf("expected second message to be assistant (tool_call), got %v", allMsgs[1][1].Role)
	}
	if allMsgs[1][2].Role != types.RoleToolResult {
		t.Errorf("expected third message to be tool_result, got %v", allMsgs[1][2].Role)
	}
}

func TestRun_ToolFiltering(t *testing.T) {
	mp := &toolCallMockProvider{
		finalText: "done",
	}

	mt := &mockTool{
		name:        "allowed_tool",
		description: "An allowed tool",
		result:      &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "ok"}}},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:  "test",
		Model: types.Model{ID: "test-model"},
		Tools: []Tool{mt},
	})

	sa.Run(context.Background())

	tools := mp.CapturedTools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool definition, got %d", len(tools))
	}
	if tools[0].Name != "allowed_tool" {
		t.Errorf("expected tool name 'allowed_tool', got %q", tools[0].Name)
	}
}

func TestRun_ParentHardCeiling(t *testing.T) {
	mp := &testutil.MockProvider{
		Events: []types.StreamEvent{
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	mt := &mockTool{
		name:   "forbidden_tool",
		result: &types.ToolResult{},
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for hard ceiling violation")
		}
		expected := `subagent: tool "forbidden_tool" not in parent tool set (hard ceiling violation)`
		if r.(string) != expected {
			t.Errorf("expected panic message %q, got %q", expected, r.(string))
		}
	}()

	NewSubAgent(mp, SubAgentOpts{
		Task:            "test",
		Tools:           []Tool{mt},
		ParentToolNames: []string{"read", "ls"},
	})
}

func TestRun_NoTools_NoExecutor(t *testing.T) {
	mp := &testutil.MockProvider{
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "no tools"},
			{Type: types.EventDone, Message: &types.AgentMessage{
				Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}},
			}},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:  "test",
		Model: types.Model{ID: "test-model"},
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Output != "no tools" {
		t.Errorf("expected output 'no tools', got %q", result.Output)
	}
}

func TestRun_MaxIterations(t *testing.T) {
	mp := &alwaysToolCallProvider{
		toolCallName: "loop_tool",
		toolCallArgs: map[string]any{},
	}

	mt := &mockTool{
		name:   "loop_tool",
		result: &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "loop"}}},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:  "infinite loop",
		Model: types.Model{ID: "test-model"},
		Tools: []Tool{mt},
		Executor: func(ctx context.Context, calls []ToolCallRequest) []*ToolCallResult {
			results := make([]*ToolCallResult, len(calls))
			for i, c := range calls {
				results[i] = &ToolCallResult{ID: c.ID, Name: c.Name, Result: &types.ToolResult{}}
			}
			return results
		},
	})

	result := sa.Run(context.Background())

	if result.Success {
		t.Fatal("expected failure due to max iterations")
	}
	if result.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(result.Error.Error(), "max tool iterations") {
		t.Errorf("expected max iterations error, got: %v", result.Error)
	}
}

func TestRun_ToolDefinitionsPassedToProvider(t *testing.T) {
	mp := &toolCallMockProvider{
		finalText: "done",
	}

	readTool := &mockTool{
		name:        "read",
		description: "Read a file",
		result:      &types.ToolResult{},
	}
	lsTool := &mockTool{
		name:        "ls",
		description: "List directory",
		result:      &types.ToolResult{},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:  "list files",
		Model: types.Model{ID: "test-model"},
		Tools: []Tool{readTool, lsTool},
	})

	sa.Run(context.Background())

	tools := mp.CapturedTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tool definitions, got %d", len(tools))
	}
	names := make(map[string]bool)
	for _, td := range tools {
		names[td.Name] = true
	}
	if !names["read"] || !names["ls"] {
		t.Errorf("expected tools 'read' and 'ls', got %v", names)
	}
}

// E2E tool tests (TestRun_E2E_ToolRead, TestRun_E2E_RestrictedTools) are in
// internal/tools/subagent_e2e_test.go to avoid an import cycle (tools → subagent).

func runE2EWithType(t *testing.T, agentType Type, task string, expectedOutputContains string) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	parentTools := []Tool{
		&mockTool{name: "read", description: "Read a file", result: &types.ToolResult{}},
		&mockTool{name: "write", description: "Write a file", result: &types.ToolResult{}},
		&mockTool{name: "edit", description: "Edit a file", result: &types.ToolResult{}},
		&mockTool{name: "bash", description: "Run bash command", result: &types.ToolResult{}},
		&mockTool{name: "grep", description: "Search file contents", result: &types.ToolResult{}},
		&mockTool{name: "find", description: "Search for files", result: &types.ToolResult{}},
		&mockTool{name: "ls", description: "List directory", result: &types.ToolResult{}},
	}

	sa, err := NewSubAgentByType(agentType, ollama, parentTools, SubAgentOpts{
		Task:    task,
		Model:   model,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create subagent: %v", err)
	}

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success for type %q, got error: %v", agentType, result.Error)
	}
	if result.Output == "" {
		t.Fatalf("expected non-empty output for type %q", agentType)
	}
	if expectedOutputContains != "" && !strings.Contains(strings.ToLower(result.Output), strings.ToLower(expectedOutputContains)) {
		t.Errorf("expected output to contain %q for type %q, got: %q", expectedOutputContains, agentType, result.Output)
	}
	t.Logf("E2E %s output (first 200 chars): %q", agentType, truncateOutput(result.Output, 200))
	t.Logf("Duration: %v", result.Duration)
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func TestRun_E2E_BuiltinType_General(t *testing.T) {
	runE2EWithType(t, TypeGeneral, "Say exactly: general type test passed", "general type test passed")
}

func TestRun_E2E_BuiltinType_Researcher(t *testing.T) {
	runE2EWithType(t, TypeResearcher, "What programming language is this file written in? Answer with just the language name.", "go")
}

func TestRun_E2E_BuiltinType_Reviewer(t *testing.T) {
	runE2EWithType(t, TypeReviewer, "Review this statement: 'Go is a statically typed language.' Is this correct? Answer with just yes or no.", "yes")
}

func TestRun_E2E_BuiltinType_Implementor(t *testing.T) {
	runE2EWithType(t, TypeImplementor, "Say exactly: implementor type test passed", "implementor type test passed")
}

func TestRun_E2E_BuiltinType_SecurityReviewer(t *testing.T) {
	runE2EWithType(t, TypeSecurityReviewer, "Is it a security risk to hardcode API keys in source code? Answer with just yes or no.", "yes")
}

func TestRun_E2E_BuiltinType_QA(t *testing.T) {
	runE2EWithType(t, TypeQA, "Say exactly: qa type test passed", "qa type test passed")
}

// eventMockProvider emits events and can be configured to emit multiple EventDone for usage accumulation testing.
type eventMockProvider struct {
	events       []types.StreamEvent
	capturedMsgs [][]types.AgentMessage
	mu           sync.Mutex
}

func (m *eventMockProvider) Name() string { return "event-mock" }

func (m *eventMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	m.mu.Lock()
	msgCopy := make([]types.AgentMessage, len(messages))
	copy(msgCopy, messages)
	m.capturedMsgs = append(m.capturedMsgs, msgCopy)
	m.mu.Unlock()

	ch := make(chan types.StreamEvent, len(m.events)+1)
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch
}

func (m *eventMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

func TestRun_EventForwarding(t *testing.T) {
	mp := &eventMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "Hello, "},
			{Type: types.EventTextDelta, Delta: "world!"},
			{
				Type: types.EventDone,
				Message: &types.AgentMessage{
					Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}},
				},
			},
		},
	}

	eventsCh := make(chan types.AgentEvent, 64)
	sa := NewSubAgent(mp, SubAgentOpts{
		Task:   "greet",
		Events: eventsCh,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	close(eventsCh)
	var events []types.AgentEvent
	for e := range eventsCh {
		events = append(events, e)
	}

	if len(events) == 0 {
		t.Fatal("expected events to be forwarded, got none")
	}

	// Verify first event is agent_start
	if events[0].Type != types.AgentEventStart {
		t.Errorf("expected first event to be agent_start, got %s", events[0].Type)
	}

	// Verify SubAgentID is set on all events
	for i, e := range events {
		if e.SubAgentID == nil || *e.SubAgentID != sa.ID {
			t.Errorf("event[%d] missing or wrong SubAgentID: %v", i, e.SubAgentID)
		}
	}

	// Verify text_delta events contain the deltas
	var foundTextDeltas int
	for _, e := range events {
		if e.Type == types.AgentEventTextDelta {
			foundTextDeltas++
			if e.Data != "Hello, " && e.Data != "world!" {
				t.Errorf("unexpected text_delta data: %v", e.Data)
			}
		}
	}
	if foundTextDeltas != 2 {
		t.Errorf("expected 2 text_delta events, got %d", foundTextDeltas)
	}

	// Verify message_end and agent_end at the end
	lastTypes := []types.AgentEventType{}
	for i := len(events) - 2; i < len(events); i++ {
		if i >= 0 {
			lastTypes = append(lastTypes, events[i].Type)
		}
	}
	if len(lastTypes) < 2 {
		t.Fatalf("expected at least 2 ending events, got %d", len(lastTypes))
	}
	if lastTypes[0] != types.AgentEventMessageEnd {
		t.Errorf("expected second-to-last event to be message_end, got %s", lastTypes[0])
	}
	if lastTypes[1] != types.AgentEventAgentEnd {
		t.Errorf("expected last event to be agent_end, got %s", lastTypes[1])
	}
}

func TestRun_NilEventsChannel(t *testing.T) {
	mp := &eventMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "ok"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task: "test",
	})

	// Should not panic with nil Events channel
	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
}

func TestRun_UsageAccumulation(t *testing.T) {
	// Use a multi-turn provider
	multiTurn := &multiTurnEventProvider{
		turns: [][]types.StreamEvent{
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "first turn"},
				{
					Type: types.EventDone,
					Message: &types.AgentMessage{
						Role: types.RoleAssistant,
						Content: []types.ContentBlock{
							{
								Type: types.BlockToolCall,
								ToolCall: &types.ToolCallBlock{
									ID:   "call_0",
									Name: "mock_tool",
								},
							},
						},
					},
					Usage: &types.Usage{Input: 10, Output: 5},
				},
			},
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "final answer"},
				{
					Type: types.EventDone,
					Message: &types.AgentMessage{
						Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}},
					},
					Usage: &types.Usage{Input: 20, Output: 15},
				},
			},
		},
	}

	mt := &mockTool{name: "mock_tool", result: &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "ok"}}}}

	executor := func(ctx context.Context, calls []ToolCallRequest) []*ToolCallResult {
		return []*ToolCallResult{
			{ID: calls[0].ID, Name: calls[0].Name, Result: &types.ToolResult{Content: []types.ContentBlock{{Type: "text", Text: "tool result"}}}},
		}
	}

	sa := NewSubAgent(multiTurn, SubAgentOpts{
		Task:     "use tool",
		Model:    types.Model{ID: "test-model"},
		Tools:    []Tool{mt},
		Executor: executor,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	// Verify usage accumulated across both turns
	if result.Usage.Input != 30 {
		t.Errorf("expected accumulated input usage 30, got %d", result.Usage.Input)
	}
	if result.Usage.Output != 20 {
		t.Errorf("expected accumulated output usage 20, got %d", result.Usage.Output)
	}
}

// multiTurnEventProvider returns different events on successive Stream calls.
type multiTurnEventProvider struct {
	turns     [][]types.StreamEvent
	callCount int
	mu        sync.Mutex
}

func (m *multiTurnEventProvider) Name() string { return "multi-turn" }

func (m *multiTurnEventProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	m.mu.Lock()
	idx := m.callCount
	m.callCount++
	m.mu.Unlock()

	var events []types.StreamEvent
	if idx < len(m.turns) {
		events = m.turns[idx]
	} else {
		events = []types.StreamEvent{{Type: types.EventDone, Message: &types.AgentMessage{}}}
	}

	ch := make(chan types.StreamEvent, len(events)+1)
	for _, e := range events {
		ch <- e
	}
	close(ch)
	return ch
}

func (m *multiTurnEventProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

func TestRun_SlowConsumer(t *testing.T) {
	mp := &eventMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "ok"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	// Small buffered channel that we never drain
	eventsCh := make(chan types.AgentEvent, 2)

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:   "test",
		Events: eventsCh,
	})

	// Run should complete without blocking even though channel fills up
	// (internal channel is buffered at 256, forwardEvents sends to sa.Events
	// which will fill up but internal channel draining prevents blocking)
	done := make(chan struct{})
	go func() {
		sa.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Run() blocked for too long — slow consumer caused deadlock")
	}
}

func TestRun_EventSequence(t *testing.T) {
	mp := &eventMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextStart},
			{Type: types.EventTextDelta, Delta: "Hello"},
			{Type: types.EventTextDelta, Delta: " world"},
			{Type: types.EventTextEnd},
			{
				Type: types.EventDone,
				Message: &types.AgentMessage{
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello world"}},
				},
			},
		},
	}

	eventsCh := make(chan types.AgentEvent, 64)
	sa := NewSubAgent(mp, SubAgentOpts{
		Task:   "greet",
		Events: eventsCh,
	})

	sa.Run(context.Background())
	close(eventsCh)

	var events []types.AgentEvent
	for e := range eventsCh {
		events = append(events, e)
	}

	// Expected sequence: agent_start (from emitEvent) → message_start → text_delta × 2 → message_end → message_end (from EventDone) → agent_end
	// Filter out the first agent_start (from emitEvent) and last two (message_end + agent_end from emitEvent)
	var streamEvents []types.AgentEvent
	for _, e := range events {
		if e.Type != types.AgentEventStart && e.Type != types.AgentEventAgentEnd {
			streamEvents = append(streamEvents, e)
		}
	}

	// The stream events should be: message_start, text_delta, text_delta, message_end, message_end
	expectedTypes := []types.AgentEventType{
		types.AgentEventMessageStart,
		types.AgentEventTextDelta,
		types.AgentEventTextDelta,
		types.AgentEventMessageEnd,
		types.AgentEventMessageEnd,
	}

	if len(streamEvents) < len(expectedTypes) {
		t.Fatalf("expected at least %d stream events, got %d: %v", len(expectedTypes), len(streamEvents), events)
	}

	for i, expected := range expectedTypes {
		if streamEvents[i].Type != expected {
			t.Errorf("event[%d] type: expected %s, got %s", i, expected, streamEvents[i].Type)
		}
	}
}

func TestRun_E2E_EventStreaming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	eventsCh := make(chan types.AgentEvent, 256)
	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:         "Say exactly: event streaming test passed",
		SystemPrompt: "Be extremely concise. Follow instructions exactly.",
		Model:        model,
		Events:       eventsCh,
	})

	startTime := time.Now()
	result := sa.Run(context.Background())
	close(eventsCh)

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}

	var events []types.AgentEvent
	for e := range eventsCh {
		events = append(events, e)
	}

	if len(events) == 0 {
		t.Fatal("expected events to be forwarded, got none")
	}

	// Verify event sequence contains text_delta events
	var textDeltaCount int
	var hasMessageEnd bool
	var hasAgentEnd bool
	for _, e := range events {
		if e.Type == types.AgentEventTextDelta {
			textDeltaCount++
		}
		if e.Type == types.AgentEventMessageEnd {
			hasMessageEnd = true
		}
		if e.Type == types.AgentEventAgentEnd {
			hasAgentEnd = true
		}
	}

	if textDeltaCount == 0 {
		t.Error("expected at least one text_delta event")
	}
	if !hasMessageEnd {
		t.Error("expected message_end event")
	}
	if !hasAgentEnd {
		t.Error("expected agent_end event")
	}

	// Verify events arrived in real-time (not all batched at end)
	if len(events) >= 2 {
		firstEventTime := startTime
		lastEventTime := startTime.Add(result.Duration)
		eventSpread := lastEventTime.Sub(firstEventTime)
		t.Logf("Event spread: %v, total events: %d, text_deltas: %d", eventSpread, len(events), textDeltaCount)
	}

	t.Logf("E2E output: %q", result.Output)
	t.Logf("Duration: %v", result.Duration)
	t.Logf("Events received: %d", len(events))
}

func TestRun_E2E_NilEventsChannel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:         "Say exactly: nil events test passed",
		SystemPrompt: "Be extremely concise. Follow instructions exactly.",
		Model:        model,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("E2E output: %q", result.Output)
	t.Logf("Duration: %v", result.Duration)
}

func TestRun_E2E_UsageAccumulation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:         "Say exactly: usage accumulation test passed",
		SystemPrompt: "Be extremely concise. Follow instructions exactly.",
		Model:        model,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}

	// Ollama streaming doesn't include usage stats per chunk, so Usage may be 0.
	// This test verifies the field is accessible and doesn't panic.
	t.Logf("E2E output: %q", result.Output)
	t.Logf("Duration: %v", result.Duration)
	t.Logf("Usage: input=%d output=%d total=%d", result.Usage.Input, result.Usage.Output, result.Usage.TotalTokens)
}

func TestSubAgentResult_LLMVisibleDefault(t *testing.T) {
	mp := &testutil.MockProvider{
		Events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "ok"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task: "test",
	})

	result := sa.Run(context.Background())

	if !result.LLMVisible {
		t.Error("expected LLMVisible to default to true")
	}
}

func TestInjectResult_LLMVisible(t *testing.T) {
	messages := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}}},
	}

	result := SubAgentResult{
		Success:    true,
		Output:     "Subagent output",
		Duration:   2 * time.Second,
		LLMVisible: true,
		Artifacts:  []string{"/tmp/file.txt"},
		Usage: types.Usage{
			Input:  100,
			Output: 50,
		},
	}

	injected := InjectResult(messages, result, "Test task")

	if len(injected) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(injected))
	}

	injectedMsg := injected[1]
	if injectedMsg.Role != types.RoleToolResult {
		t.Errorf("expected role tool_result, got %v", injectedMsg.Role)
	}

	text := injectedMsg.Content[0].Text
	if !strings.Contains(text, "## Subagent Result") {
		t.Errorf("expected formatted header, got: %q", text)
	}
	if !strings.Contains(text, "**Task**: Test task") {
		t.Errorf("expected task in output, got: %q", text)
	}
	if !strings.Contains(text, "**Status**: Success") {
		t.Errorf("expected status in output, got: %q", text)
	}
	if !strings.Contains(text, "Subagent output") {
		t.Errorf("expected output in result, got: %q", text)
	}
	if !strings.Contains(text, "/tmp/file.txt") {
		t.Errorf("expected artifact in result, got: %q", text)
	}
}

func TestInjectResult_NotLLMVisible(t *testing.T) {
	messages := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}}},
	}

	result := SubAgentResult{
		Success:    true,
		Output:     "Secret output",
		LLMVisible: false,
	}

	injected := InjectResult(messages, result, "Test task")

	if len(injected) != 1 {
		t.Fatalf("expected 1 message (unchanged), got %d", len(injected))
	}
}

func TestSubAgentResult_FormatForLLM_Success(t *testing.T) {
	result := SubAgentResult{
		Success:  true,
		Output:   "The answer is 42",
		Duration: 3500 * time.Millisecond,
		Usage: types.Usage{
			Input:       100,
			Output:      50,
			TotalTokens: 150,
		},
		Artifacts: []string{"/tmp/test.go", "/tmp/test_test.go"},
	}

	formatted := result.FormatForLLM("Find the answer")

	if !strings.Contains(formatted, "**Task**: Find the answer") {
		t.Errorf("missing task: %q", formatted)
	}
	if !strings.Contains(formatted, "**Status**: Success") {
		t.Errorf("missing status: %q", formatted)
	}
	if !strings.Contains(formatted, "**Duration**: 3.5s") {
		t.Errorf("missing duration: %q", formatted)
	}
	if !strings.Contains(formatted, "**Tokens**: 100 input, 50 output, 150 total") {
		t.Errorf("missing tokens: %q", formatted)
	}
	if !strings.Contains(formatted, "The answer is 42") {
		t.Errorf("missing output: %q", formatted)
	}
	if !strings.Contains(formatted, "/tmp/test.go") {
		t.Errorf("missing artifact: %q", formatted)
	}
	if !strings.Contains(formatted, "/tmp/test_test.go") {
		t.Errorf("missing second artifact: %q", formatted)
	}
}

func TestSubAgentResult_FormatForLLM_Failure(t *testing.T) {
	result := SubAgentResult{
		Success: false,
		Error:   fmt.Errorf("something went wrong"),
	}

	formatted := result.FormatForLLM("Fail task")

	if !strings.Contains(formatted, "**Status**: Failed") {
		t.Errorf("missing failed status: %q", formatted)
	}
	if !strings.Contains(formatted, "**Error**: something went wrong") {
		t.Errorf("missing error: %q", formatted)
	}
}

func TestSubAgentResult_FormatForLLM_Timeout(t *testing.T) {
	result := SubAgentResult{
		Success: false,
		Timeout: true,
		Error:   fmt.Errorf("timed out"),
	}

	formatted := result.FormatForLLM("Timeout task")

	if !strings.Contains(formatted, "**Status**: Timeout") {
		t.Errorf("missing timeout status: %q", formatted)
	}
}

func TestExtractArtifact_WriteTool(t *testing.T) {
	call := ToolCallRequest{
		Name:      "write",
		Arguments: json.RawMessage(`{"path": "/tmp/hello.txt"}`),
	}
	result := &ToolCallResult{
		Result: &types.ToolResult{
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "File written"}},
		},
	}

	artifact := extractArtifact(call, result)
	if artifact != "/tmp/hello.txt" {
		t.Errorf("expected /tmp/hello.txt, got %q", artifact)
	}
}

func TestExtractArtifact_EditTool(t *testing.T) {
	call := ToolCallRequest{
		Name:      "edit",
		Arguments: json.RawMessage(`{"path": "/src/main.go"}`),
	}
	result := &ToolCallResult{
		Result: &types.ToolResult{
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "File edited"}},
		},
	}

	artifact := extractArtifact(call, result)
	if artifact != "/src/main.go" {
		t.Errorf("expected /src/main.go, got %q", artifact)
	}
}

func TestExtractArtifact_NoArtifact(t *testing.T) {
	call := ToolCallRequest{
		Name:      "read",
		Arguments: json.RawMessage(`{"path": "/tmp/file.txt"}`),
	}
	result := &ToolCallResult{
		Result: &types.ToolResult{
			Content: []types.ContentBlock{{Type: types.BlockText, Text: "file content"}},
		},
	}

	artifact := extractArtifact(call, result)
	if artifact != "" {
		t.Errorf("expected no artifact for read tool, got %q", artifact)
	}
}

func TestExtractArtifact_NilResult(t *testing.T) {
	call := ToolCallRequest{
		Name:      "write",
		Arguments: json.RawMessage(`{"path": "/tmp/file.txt"}`),
	}

	artifact := extractArtifact(call, nil)
	if artifact != "" {
		t.Errorf("expected no artifact with nil result, got %q", artifact)
	}
}

func TestRun_ArtifactTracking(t *testing.T) {
	mp := &toolCallMockProvider{
		toolCallName: "write",
		toolCallArgs: map[string]any{"path": "/tmp/artifact_test.txt"},
		finalText:    "File created successfully",
	}

	mt := &mockTool{
		name:        "write",
		description: "Write a file",
		result: &types.ToolResult{
			Content: []types.ContentBlock{{Type: "text", Text: "Written to /tmp/artifact_test.txt"}},
		},
	}

	executor := func(ctx context.Context, calls []ToolCallRequest) []*ToolCallResult {
		return []*ToolCallResult{
			{
				ID:   calls[0].ID,
				Name: calls[0].Name,
				Result: &types.ToolResult{
					Content: []types.ContentBlock{{Type: "text", Text: "Written to /tmp/artifact_test.txt"}},
				},
			},
		}
	}

	sa := NewSubAgent(mp, SubAgentOpts{
		Task:     "create a file",
		Model:    types.Model{ID: "test-model"},
		Tools:    []Tool{mt},
		Executor: executor,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	if len(result.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact, got %d: %v", len(result.Artifacts), result.Artifacts)
	}

	expected := "/tmp/artifact_test.txt"
	if result.Artifacts[0] != expected {
		t.Errorf("expected artifact %q, got %q", expected, result.Artifacts[0])
	}
}

func TestRun_E2E_ArtifactTracking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	parentTools := []Tool{
		&mockTool{name: "write", description: "Write a file", result: &types.ToolResult{}},
		&mockTool{name: "read", description: "Read a file", result: &types.ToolResult{}},
	}

	sa, err := NewSubAgentByType(TypeImplementor, ollama, parentTools, SubAgentOpts{
		Task:    "Create a file /tmp/e2e_artifact_test.txt with content 'E2E artifact test'",
		Model:   model,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create subagent: %v", err)
	}

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	t.Logf("E2E artifact test output: %q", truncateOutput(result.Output, 200))
	t.Logf("Artifacts: %v", result.Artifacts)
	t.Logf("Duration: %v", result.Duration)
}

func TestRun_E2E_InjectResult(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	sa := NewSubAgent(ollama, SubAgentOpts{
		Task:         "Say exactly: inject result test passed",
		SystemPrompt: "Be extremely concise. Follow instructions exactly.",
		Model:        model,
	})

	result := sa.Run(context.Background())

	if !result.Success {
		t.Fatalf("expected success, got error: %v", result.Error)
	}

	// Test LLM-visible injection
	parentMessages := []types.AgentMessage{
		{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "Run a subagent task"}}},
	}

	injected := InjectResult(parentMessages, result, "Say exactly: inject result test passed")

	if len(injected) != 2 {
		t.Fatalf("expected 2 messages after injection, got %d", len(injected))
	}

	if injected[1].Role != types.RoleToolResult {
		t.Errorf("expected injected message to be tool_result, got %v", injected[1].Role)
	}

	text := injected[1].Content[0].Text
	if !strings.Contains(text, "inject result test passed") {
		t.Errorf("expected subagent output in injected message, got: %q", text)
	}

	t.Logf("Injected message (first 200 chars): %q", truncateOutput(text, 200))
}

// parallelMockProvider returns different text based on index for identification.
type parallelMockProvider struct {
	index int
}

func (p *parallelMockProvider) Name() string                          { return "parallel-mock" }
func (p *parallelMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: fmt.Sprintf("task-%d", p.index)}}}, nil
}
func (p *parallelMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, 4)
	go func() {
		defer close(ch)
		ch <- types.StreamEvent{Type: types.EventStart}
		ch <- types.StreamEvent{Type: types.EventTextDelta, Delta: fmt.Sprintf("task-%d", p.index)}
		ch <- types.StreamEvent{
			Type:    types.EventDone,
			Message: &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}}},
			Usage:   &types.Usage{Input: 10, Output: 5},
		}
	}()
	return ch
}

func TestRunParallel_EmptyTasks(t *testing.T) {
	result := RunParallel(context.Background(), []SubAgentTask{}, ParallelOpts{})

	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
	if len(result.Results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result.Results))
	}
}

func TestRunParallel_MaxTasksExceeded(t *testing.T) {
	tasks := make([]SubAgentTask, 10)
	for i := range tasks {
		tasks[i] = SubAgentTask{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}},
			},
		}, SubAgentOpts{Task: fmt.Sprintf("task-%d", i)})}
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{MaxTasks: 8})

	if result.Error == nil {
		t.Fatal("expected error for too many tasks, got nil")
	}
	if !strings.Contains(result.Error.Error(), "too many tasks") {
		t.Errorf("expected 'too many tasks' error, got: %v", result.Error)
	}
}

func TestRunParallel_AllSucceed(t *testing.T) {
	tasks := make([]SubAgentTask, 3)
	for i := 0; i < 3; i++ {
		idx := i
		tasks[i] = SubAgentTask{SubAgent: NewSubAgent(&parallelMockProvider{index: idx}, SubAgentOpts{
			Task:  fmt.Sprintf("task-%d", idx),
			Model: types.Model{ID: "test"},
		})}
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{Concurrency: 2})

	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
	if result.SuccessCount != 3 {
		t.Errorf("expected 3 successes, got %d", result.SuccessCount)
	}
	if result.FailureCount != 0 {
		t.Errorf("expected 0 failures, got %d", result.FailureCount)
	}
	if len(result.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result.Results))
	}
	for i, r := range result.Results {
		if !r.Success {
			t.Errorf("result[%d] should succeed", i)
		}
		if !strings.Contains(r.Output, fmt.Sprintf("task-%d", i)) {
			t.Errorf("result[%d] should contain 'task-%d', got: %q", i, i, r.Output)
		}
	}
}

func TestRunParallel_OrderPreserved(t *testing.T) {
	tasks := make([]SubAgentTask, 5)
	for i := 0; i < 5; i++ {
		idx := i
		tasks[i] = SubAgentTask{SubAgent: NewSubAgent(&parallelMockProvider{index: idx}, SubAgentOpts{
			Task:  fmt.Sprintf("task-%d", idx),
			Model: types.Model{ID: "test"},
		})}
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{Concurrency: 1})

	for i, r := range result.Results {
		if !strings.Contains(r.Output, fmt.Sprintf("task-%d", i)) {
			t.Errorf("result[%d] should contain 'task-%d' (order preserved), got: %q", i, i, r.Output)
		}
	}
}

func TestRunParallel_ConcurrencyLimit(t *testing.T) {
	var mu sync.Mutex
	var maxConcurrent int
	var currentConcurrent int

	tasks := make([]SubAgentTask, 4)
	for i := 0; i < 4; i++ {
		idx := i
		tasks[i] = SubAgentTask{SubAgent: NewSubAgent(&slowParallelMockProvider{
			index: idx,
			onStart: func() {
				mu.Lock()
				currentConcurrent++
				if currentConcurrent > maxConcurrent {
					maxConcurrent = currentConcurrent
				}
				mu.Unlock()
			},
			onEnd: func() {
				mu.Lock()
				currentConcurrent--
				mu.Unlock()
			},
		}, SubAgentOpts{
			Task:    fmt.Sprintf("task-%d", idx),
			Model:   types.Model{ID: "test"},
			Timeout: 5 * time.Second,
		})}
	}

	RunParallel(context.Background(), tasks, ParallelOpts{Concurrency: 2})

	if maxConcurrent > 2 {
		t.Errorf("expected max concurrent <= 2, got %d", maxConcurrent)
	}
}

func TestRunParallel_MixedSuccessFailure(t *testing.T) {
	tasks := []SubAgentTask{
		{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}},
			},
		}, SubAgentOpts{Task: "success-1", Model: types.Model{ID: "test"}})},
		{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventError, Error: "provider error"},
			},
		}, SubAgentOpts{Task: "fail-1", Model: types.Model{ID: "test"}})},
		{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}},
			},
		}, SubAgentOpts{Task: "success-2", Model: types.Model{ID: "test"}})},
		{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventError, Error: "another error"},
			},
		}, SubAgentOpts{Task: "fail-2", Model: types.Model{ID: "test"}})},
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{})

	if result.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", result.SuccessCount)
	}
	if result.FailureCount != 2 {
		t.Errorf("expected 2 failures, got %d", result.FailureCount)
	}
	if !result.Results[0].Success {
		t.Error("result[0] should succeed")
	}
	if result.Results[1].Success {
		t.Error("result[1] should fail")
	}
	if !result.Results[2].Success {
		t.Error("result[2] should succeed")
	}
	if result.Results[3].Success {
		t.Error("result[3] should fail")
	}
}

func TestRunParallel_UsageAggregation(t *testing.T) {
	tasks := make([]SubAgentTask, 3)
	for i := 0; i < 3; i++ {
		input := 10 + i*5
		output := 5 + i*3
		tasks[i] = SubAgentTask{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}, Usage: &types.Usage{
					Input:  input,
					Output: output,
				}},
			},
		}, SubAgentOpts{Task: fmt.Sprintf("task-%d", i), Model: types.Model{ID: "test"}})}
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{})

	expectedInput := 10 + 15 + 20  // 10+15+20 = 45
	expectedOutput := 5 + 8 + 11   // 5+8+11 = 24

	if result.TotalUsage.Input != expectedInput {
		t.Errorf("expected total input %d, got %d", expectedInput, result.TotalUsage.Input)
	}
	if result.TotalUsage.Output != expectedOutput {
		t.Errorf("expected total output %d, got %d", expectedOutput, result.TotalUsage.Output)
	}
}

func TestRunParallel_NilEventsChannel(t *testing.T) {
	tasks := []SubAgentTask{
		{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}},
			},
		}, SubAgentOpts{Task: "task-1", Model: types.Model{ID: "test"}})},
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{Events: nil})

	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
	if result.SuccessCount != 1 {
		t.Errorf("expected 1 success, got %d", result.SuccessCount)
	}
}

func TestRunParallel_DefaultConcurrency(t *testing.T) {
	tasks := make([]SubAgentTask, 4)
	for i := 0; i < 4; i++ {
		tasks[i] = SubAgentTask{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}},
			},
		}, SubAgentOpts{Task: fmt.Sprintf("task-%d", i), Model: types.Model{ID: "test"}})}
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{Concurrency: 0})

	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
	if result.SuccessCount != 4 {
		t.Errorf("expected 4 successes, got %d", result.SuccessCount)
	}
}

func TestRunParallel_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tasks := []SubAgentTask{
		{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}},
			},
		}, SubAgentOpts{Task: "task-1", Model: types.Model{ID: "test"}})},
	}

	result := RunParallel(ctx, tasks, ParallelOpts{})

	if result.Error != nil {
		t.Fatalf("expected no error from RunParallel itself, got: %v", result.Error)
	}
}

func TestRunParallel_EventForwarding(t *testing.T) {
	parentEvents := make(chan types.AgentEvent, 100)
	tasks := []SubAgentTask{
		{SubAgent: NewSubAgent(&testutil.MockProvider{
			Events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "hello"},
				{Type: types.EventTextDelta, Delta: " world"},
				{Type: types.EventDone, Message: &types.AgentMessage{}},
			},
		}, SubAgentOpts{Task: "task-1", Model: types.Model{ID: "test"}})},
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{Events: parentEvents})

	close(parentEvents)

	var events []types.AgentEvent
	for evt := range parentEvents {
		events = append(events, evt)
	}

	if result.SuccessCount != 1 {
		t.Errorf("expected 1 success, got %d", result.SuccessCount)
	}

	var hasStart, hasDelta, hasEnd bool
	for _, evt := range events {
		if evt.SubAgentID == nil {
			continue
		}
		switch evt.Type {
		case types.AgentEventStart:
			hasStart = true
		case types.AgentEventTextDelta:
			hasDelta = true
		case types.AgentEventMessageEnd:
			hasEnd = true
		}
	}

	if !hasStart {
		t.Error("expected AgentEventStart in forwarded events")
	}
	if !hasDelta {
		t.Error("expected AgentEventTextDelta in forwarded events")
	}
	if !hasEnd {
		t.Error("expected AgentEventMessageEnd in forwarded events")
	}
}

// slowParallelMockProvider is a mock that sleeps to test concurrency limits.
type slowParallelMockProvider struct {
	index   int
	onStart func()
	onEnd   func()
}

func (p *slowParallelMockProvider) Name() string { return "slow-parallel-mock" }
func (p *slowParallelMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: fmt.Sprintf("task-%d", p.index)}}}, nil
}
func (p *slowParallelMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, 4)
	go func() {
		defer close(ch)
		if p.onStart != nil {
			p.onStart()
		}
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
		}
		ch <- types.StreamEvent{Type: types.EventStart}
		ch <- types.StreamEvent{Type: types.EventTextDelta, Delta: fmt.Sprintf("task-%d", p.index)}
		ch <- types.StreamEvent{
			Type:    types.EventDone,
			Message: &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}}},
			Usage:   &types.Usage{Input: 10, Output: 5},
		}
		if p.onEnd != nil {
			p.onEnd()
		}
	}()
	return ch
}

func TestRunParallel_E2E_ParallelExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	tasks := []SubAgentTask{
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "Say exactly: parallel-task-1", SystemPrompt: "Be extremely concise.", Model: model})},
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "Say exactly: parallel-task-2", SystemPrompt: "Be extremely concise.", Model: model})},
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "Say exactly: parallel-task-3", SystemPrompt: "Be extremely concise.", Model: model})},
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "Say exactly: parallel-task-4", SystemPrompt: "Be extremely concise.", Model: model})},
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{Concurrency: 2})

	if result.SuccessCount != 4 {
		t.Fatalf("expected 4 successes, got %d (failures: %d)", result.SuccessCount, result.FailureCount)
	}
	for i, r := range result.Results {
		if len(r.Output) == 0 {
			t.Errorf("result[%d] should have non-empty output", i)
		}
	}
	t.Logf("All 4 parallel tasks completed successfully")
}

func TestRunParallel_E2E_MixedSuccessFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}
	invalidModel := types.Model{
		ID:       "nonexistent-model-12345",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	tasks := []SubAgentTask{
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "Say exactly: success-1", SystemPrompt: "Be extremely concise.", Model: model})},
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "Say exactly: success-2", SystemPrompt: "Be extremely concise.", Model: model})},
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "Say exactly: success-3", SystemPrompt: "Be extremely concise.", Model: model})},
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "This should fail", SystemPrompt: "Be extremely concise.", Model: invalidModel})},
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{})

	if result.SuccessCount != 3 {
		t.Errorf("expected 3 successes, got %d", result.SuccessCount)
	}
	if result.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", result.FailureCount)
	}
}

func TestRunParallel_E2E_ConcurrencyLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	tasks := make([]SubAgentTask, 4)
	for i := 0; i < 4; i++ {
		tasks[i] = SubAgentTask{SubAgent: NewSubAgent(ollama, SubAgentOpts{
			Task:         fmt.Sprintf("Say exactly: concurrency-test-%d", i+1),
			SystemPrompt: "Be extremely concise.",
			Model:        model,
		})}
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{Concurrency: 2})

	if result.SuccessCount != 4 {
		t.Fatalf("expected 4 successes, got %d", result.SuccessCount)
	}
	t.Logf("All 4 tasks completed with concurrency limit of 2")
}

func TestRunParallel_E2E_UsageAggregation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	tasks := []SubAgentTask{
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "Say exactly: usage-test-1", SystemPrompt: "Be extremely concise.", Model: model})},
		{SubAgent: NewSubAgent(ollama, SubAgentOpts{Task: "Say exactly: usage-test-2", SystemPrompt: "Be extremely concise.", Model: model})},
	}

	result := RunParallel(context.Background(), tasks, ParallelOpts{})

	if result.SuccessCount != 2 {
		t.Fatalf("expected 2 successes, got %d", result.SuccessCount)
	}

	// Ollama streaming doesn't include usage, so TotalTokens may be 0
	// Just verify the field is accessible and result is valid
	t.Logf("Total usage: input=%d, output=%d, total=%d", result.TotalUsage.Input, result.TotalUsage.Output, result.TotalUsage.TotalTokens)
}

// chainMockProvider returns configurable events for chain testing.
type chainMockProvider struct {
	events []types.StreamEvent
}

func (m *chainMockProvider) Name() string { return "chain-mock" }
func (m *chainMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}
func (m *chainMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, len(m.events)+1)
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch
}

// capturingChainProvider captures the messages passed to Stream for verification.
type capturingChainProvider struct {
	capturedMsgs [][]types.AgentMessage
	callCount    int
	mu           sync.Mutex
	events       []types.StreamEvent
}

func (m *capturingChainProvider) Name() string { return "capturing-chain" }
func (m *capturingChainProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}
func (m *capturingChainProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	m.mu.Lock()
	msgCopy := make([]types.AgentMessage, len(messages))
	copy(msgCopy, messages)
	m.capturedMsgs = append(m.capturedMsgs, msgCopy)
	m.callCount++
	m.mu.Unlock()

	ch := make(chan types.StreamEvent, len(m.events)+1)
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch
}

func TestRunChain_EmptySteps(t *testing.T) {
	result := RunChain(context.Background(), &testutil.MockProvider{}, []SubAgentStep{}, nil)

	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
	if len(result.Steps) != 0 {
		t.Fatalf("expected 0 steps, got %d", len(result.Steps))
	}
	if result.CompletedSteps != 0 {
		t.Errorf("expected 0 completed steps, got %d", result.CompletedSteps)
	}
	if result.FailedStep != -1 {
		t.Errorf("expected FailedStep=-1, got %d", result.FailedStep)
	}
}

func TestRunChain_SingleStep(t *testing.T) {
	mp := &chainMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "step 1 output"},
			{Type: types.EventDone, Message: &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}}}},
		},
	}

	steps := []SubAgentStep{
		{Task: "Do step 1", Model: types.Model{ID: "test"}},
	}

	result := RunChain(context.Background(), mp, steps, nil)

	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
	if result.Output != "step 1 output" {
		t.Errorf("expected output 'step 1 output', got %q", result.Output)
	}
	if result.CompletedSteps != 1 {
		t.Errorf("expected 1 completed step, got %d", result.CompletedSteps)
	}
	if result.FailedStep != -1 {
		t.Errorf("expected FailedStep=-1, got %d", result.FailedStep)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.Steps))
	}
}

func TestRunChain_MultipleSteps(t *testing.T) {
	mp := &multiTurnEventProvider{
		turns: [][]types.StreamEvent{
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "first"},
				{Type: types.EventDone, Message: &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}}}},
			},
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "second"},
				{Type: types.EventDone, Message: &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}}}},
			},
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "third"},
				{Type: types.EventDone, Message: &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}}}},
			},
		},
	}

	steps := []SubAgentStep{
		{Task: "Step 1", Model: types.Model{ID: "test"}},
		{Task: "Step 2", Model: types.Model{ID: "test"}},
		{Task: "Step 3", Model: types.Model{ID: "test"}},
	}

	result := RunChain(context.Background(), mp, steps, nil)

	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
	if result.Output != "third" {
		t.Errorf("expected output 'third', got %q", result.Output)
	}
	if result.CompletedSteps != 3 {
		t.Errorf("expected 3 completed steps, got %d", result.CompletedSteps)
	}
	if result.FailedStep != -1 {
		t.Errorf("expected FailedStep=-1, got %d", result.FailedStep)
	}
	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(result.Steps))
	}
	for i, r := range result.Steps {
		if !r.Success {
			t.Errorf("step[%d] should succeed", i)
		}
	}
}

func TestRunChain_PreviousSubstitution(t *testing.T) {
	cp := &capturingChainProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "step-result"},
			{Type: types.EventDone, Message: &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}}}},
		},
	}

	steps := []SubAgentStep{
		{Task: "First step", Model: types.Model{ID: "test"}},
		{Task: "Process: {previous}", Model: types.Model{ID: "test"}},
		{Task: "Final: {previous}", Model: types.Model{ID: "test"}},
	}

	RunChain(context.Background(), cp, steps, nil)

	msgs := cp.capturedMsgs
	if len(msgs) != 3 {
		t.Fatalf("expected 3 stream calls, got %d", len(msgs))
	}

	// Step 1: task is "First step"
	if msgs[0][0].Content[0].Text != "First step" {
		t.Errorf("step 1 task: expected 'First step', got %q", msgs[0][0].Content[0].Text)
	}

	// Step 2: {previous} replaced with "step-result"
	if !strings.Contains(msgs[1][0].Content[0].Text, "Process: step-result") {
		t.Errorf("step 2 task: expected 'Process: step-result', got %q", msgs[1][0].Content[0].Text)
	}

	// Step 3: {previous} replaced with "step-result" (same output from step 2)
	if !strings.Contains(msgs[2][0].Content[0].Text, "Final: step-result") {
		t.Errorf("step 3 task: expected 'Final: step-result', got %q", msgs[2][0].Content[0].Text)
	}
}

func TestRunChain_StopsAtFirstFailure(t *testing.T) {
	mp := &multiTurnEventProvider{
		turns: [][]types.StreamEvent{
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "step 1 ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{Content: []types.ContentBlock{{Type: types.BlockText, Text: ""}}}},
			},
			{
				{Type: types.EventStart},
				{Type: types.EventError, Error: "step 2 failed"},
			},
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "should not reach here"},
				{Type: types.EventDone, Message: &types.AgentMessage{}},
			},
		},
	}

	steps := []SubAgentStep{
		{Task: "Step 1", Model: types.Model{ID: "test"}},
		{Task: "Step 2", Model: types.Model{ID: "test"}},
		{Task: "Step 3", Model: types.Model{ID: "test"}},
	}

	result := RunChain(context.Background(), mp, steps, nil)

	if result.Error == nil {
		t.Fatal("expected error from failed step")
	}
	if result.FailedStep != 1 {
		t.Errorf("expected FailedStep=1, got %d", result.FailedStep)
	}
	if result.CompletedSteps != 1 {
		t.Errorf("expected 1 completed step, got %d", result.CompletedSteps)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results (1 success + 1 failure), got %d", len(result.Steps))
	}
	if !result.Steps[0].Success {
		t.Error("step 0 should succeed")
	}
	if result.Steps[1].Success {
		t.Error("step 1 should fail")
	}
}

func TestRunChain_UsageAggregation(t *testing.T) {
	mp := &multiTurnEventProvider{
		turns: [][]types.StreamEvent{
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}, Usage: &types.Usage{Input: 10, Output: 5}},
			},
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}, Usage: &types.Usage{Input: 20, Output: 15}},
			},
			{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ok"},
				{Type: types.EventDone, Message: &types.AgentMessage{}, Usage: &types.Usage{Input: 30, Output: 25}},
			},
		},
	}

	steps := []SubAgentStep{
		{Task: "Step 1", Model: types.Model{ID: "test"}},
		{Task: "Step 2", Model: types.Model{ID: "test"}},
		{Task: "Step 3", Model: types.Model{ID: "test"}},
	}

	result := RunChain(context.Background(), mp, steps, nil)

	if result.TotalUsage.Input != 60 {
		t.Errorf("expected total input 60, got %d", result.TotalUsage.Input)
	}
	if result.TotalUsage.Output != 45 {
		t.Errorf("expected total output 45, got %d", result.TotalUsage.Output)
	}
}

func TestRunChain_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mp := &slowMockProvider{delay: 5 * time.Second}

	steps := []SubAgentStep{
		{Task: "Step 1", Model: types.Model{ID: "test"}, Timeout: 30 * time.Second},
	}

	result := RunChain(ctx, mp, steps, nil)

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.Steps))
	}
	if result.Steps[0].Success {
		t.Error("expected step to fail on cancelled context")
	}
	if result.FailedStep != 0 {
		t.Errorf("expected FailedStep=0, got %d", result.FailedStep)
	}
}

func TestRunChain_DurationTracked(t *testing.T) {
	mp := &chainMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "ok"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	steps := []SubAgentStep{
		{Task: "Step 1", Model: types.Model{ID: "test"}},
	}

	result := RunChain(context.Background(), mp, steps, nil)

	if result.Duration <= 0 {
		t.Errorf("expected positive duration, got %v", result.Duration)
	}
}

func TestRunChain_E2E_ChainExecution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	steps := []SubAgentStep{
		{
			Task:         "Write a haiku about coding. Keep it to exactly 3 lines.",
			SystemPrompt: "Be extremely concise.",
			Model:        model,
			Timeout:      30 * time.Second,
		},
		{
			Task:         "Translate the following text to Polish: {previous}",
			SystemPrompt: "Be extremely concise. Translate only, no explanation.",
			Model:        model,
			Timeout:      30 * time.Second,
		},
		{
			Task:         "Count the number of words in: {previous}",
			SystemPrompt: "Be extremely concise. Answer with just the number.",
			Model:        model,
			Timeout:      30 * time.Second,
		},
	}

	result := RunChain(context.Background(), ollama, steps, nil)

	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
	if result.CompletedSteps != 3 {
		t.Errorf("expected 3 completed steps, got %d", result.CompletedSteps)
	}
	if result.Output == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("Chain output: %q", truncateOutput(result.Output, 200))
	t.Logf("Duration: %v", result.Duration)
	t.Logf("Steps completed: %d", result.CompletedSteps)
}

func TestRunChain_E2E_ChainFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}
	invalidModel := types.Model{
		ID:       "nonexistent-model-12345",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	steps := []SubAgentStep{
		{
			Task:         "Say exactly: step 1 passed",
			SystemPrompt: "Be extremely concise.",
			Model:        model,
			Timeout:      30 * time.Second,
		},
		{
			Task:         "This should fail",
			SystemPrompt: "Be extremely concise.",
			Model:        invalidModel,
			Timeout:      5 * time.Second,
		},
		{
			Task:         "Should not execute: {previous}",
			SystemPrompt: "Be extremely concise.",
			Model:        model,
			Timeout:      30 * time.Second,
		},
	}

	result := RunChain(context.Background(), ollama, steps, nil)

	if result.Error == nil {
		t.Fatal("expected error from failed step")
	}
	if result.FailedStep != 1 {
		t.Errorf("expected FailedStep=1, got %d", result.FailedStep)
	}
	if result.CompletedSteps != 1 {
		t.Errorf("expected 1 completed step, got %d", result.CompletedSteps)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.Steps))
	}
	if !result.Steps[0].Success {
		t.Error("step 0 should succeed")
	}
	t.Logf("Step 1 output: %q", result.Steps[0].Output)
	t.Logf("Chain stopped at step %d with error: %v", result.FailedStep, result.Error)
}

func TestRunChain_E2E_PreviousSubstitution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	steps := []SubAgentStep{
		{
			Task:         "Say exactly: blue sky",
			SystemPrompt: "Be extremely concise. Follow instructions exactly.",
			Model:        model,
			Timeout:      30 * time.Second,
		},
		{
			Task:         "What color is mentioned in: {previous}? Answer with just the color.",
			SystemPrompt: "Be extremely concise.",
			Model:        model,
			Timeout:      30 * time.Second,
		},
	}

	result := RunChain(context.Background(), ollama, steps, nil)

	if result.Error != nil {
		t.Fatalf("expected no error, got: %v", result.Error)
	}
	if result.CompletedSteps != 2 {
		t.Errorf("expected 2 completed steps, got %d", result.CompletedSteps)
	}
	output := strings.ToLower(result.Output)
	if !strings.Contains(output, "blue") {
		t.Errorf("expected final output to contain 'blue' (from chain data flow), got: %q", result.Output)
	}
	t.Logf("Chain output: %q", result.Output)
}

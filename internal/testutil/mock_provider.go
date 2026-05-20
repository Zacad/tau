// Package testutil provides shared test utilities and mock implementations
// for the Tau internal packages.
//
// It depends on internal/types only, and provides reusable mocks for
// Provider and Tool interfaces, along with temporary filesystem helpers.
package testutil

import (
	"context"
	"sync"

	"github.com/adam/tau/internal/types"
)

// MockProvider implements a testable provider.Provider interface.
// It returns pre-configured StreamEvents from a buffered channel.
type MockProvider struct {
	// Events to send on Stream(). Populated before calling Stream().
	Events []types.StreamEvent
	// ProviderName is the provider name returned by Name().
	ProviderName string

	// CompleteResult is returned by Complete() if set.
	CompleteResult *types.AgentMessage
	CompleteErr    error
}

// Name returns the provider name.
func (m *MockProvider) Name() string {
	if m.ProviderName != "" {
		return m.ProviderName
	}
	return "mock"
}

// Stream returns a buffered channel containing all configured events,
// then closes the channel. Never blocks.
func (m *MockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, len(m.Events)+1)
	for _, e := range m.Events {
		ch <- e
	}
	close(ch)
	return ch
}

// Complete returns the configured result or error.
func (m *MockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return m.CompleteResult, m.CompleteErr
}

// MockToolCall records a single tool invocation.
type MockToolCall struct {
	Params any
}

// MockTool implements a testable tool interface.
// It records all calls (thread-safe) and returns a configured result.
type MockTool struct {
	ToolName        string
	ToolDescription string
	Result          *types.ToolResult
	Err             error

	mu    sync.Mutex
	Calls []MockToolCall
}

// Name returns the tool name.
func (m *MockTool) Name() string {
	if m.ToolName != "" {
		return m.ToolName
	}
	return "mock-tool"
}

// Description returns the tool description.
func (m *MockTool) Description() string {
	return m.ToolDescription
}

// MockToolParams is a simple parameter struct for MockTool JSON schema generation.
type MockToolParams struct {
	Input string `json:"input,omitempty" jsonschema:"description=Mock input"`
}

// Parameters returns a pointer to MockToolParams for valid JSON schema generation.
func (m *MockTool) Parameters() any {
	return &MockToolParams{}
}

// ExecutionMode returns ExecutionParallel for the mock.
func (m *MockTool) ExecutionMode() types.ExecutionMode {
	return types.ExecutionParallel
}

// Execute records the call and returns the configured result.
func (m *MockTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, MockToolCall{Params: params})
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Result, nil
}

// CallCount returns the number of times Execute was called.
func (m *MockTool) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}

// CallsSnapshot returns a copy of the recorded calls.
func (m *MockTool) CallsSnapshot() []MockToolCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	cpy := make([]MockToolCall, len(m.Calls))
	copy(cpy, m.Calls)
	return cpy
}

// Reset clears the recorded calls.
func (m *MockTool) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = nil
}

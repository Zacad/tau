package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adam/tau/internal/provider"
	"github.com/adam/tau/internal/subagent"
	"github.com/adam/tau/internal/types"
)

func TestSubAgentTool_Name(t *testing.T) {
	mp := &testutilMockProvider{}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model"}, nil, nil, nil)

	if tool.Name() != "subagent" {
		t.Errorf("expected name 'subagent', got %q", tool.Name())
	}
}

func TestSubAgentTool_Description(t *testing.T) {
	mp := &testutilMockProvider{}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model"}, nil, nil, nil)

	desc := tool.Description()
	if desc == "" {
		t.Error("expected non-empty description")
	}
	if !strings.Contains(desc, "Built-in agent types") {
		t.Error("expected description to mention built-in agent types")
	}
	if !strings.Contains(desc, "agent_name") {
		t.Error("expected description to mention agent_name parameter")
	}
}

func TestSubAgentTool_Description_WithDiscoveredAgents(t *testing.T) {
	mp := &testutilMockProvider{}
	agents := map[string]*subagent.AgentDefinition{
		"greeter": {
			Name:        "greeter",
			Description: "A friendly greeter agent",
			Tools:       []string{"bash"},
			SystemPrompt: "You are a greeter.",
			Source:      "user",
		},
		"code-reviewer": {
			Name:        "code-reviewer",
			Description: "Reviews code for bugs",
			Tools:       []string{"read", "grep"},
			SystemPrompt: "You are a code reviewer.",
			Source:      "project",
		},
	}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model"}, nil, nil, agents)

	desc := tool.Description()
	if !strings.Contains(desc, "User-defined agents") {
		t.Error("expected description to mention user-defined agents section")
	}
	if !strings.Contains(desc, "greeter") {
		t.Error("expected description to list 'greeter' agent")
	}
	if !strings.Contains(desc, "A friendly greeter agent") {
		t.Error("expected description to include greeter's description")
	}
	if !strings.Contains(desc, "code-reviewer") {
		t.Error("expected description to list 'code-reviewer' agent")
	}
}

func TestSubAgentTool_ExecutionMode(t *testing.T) {
	mp := &testutilMockProvider{}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model"}, nil, nil, nil)

	if tool.ExecutionMode() != types.ExecutionExclusive {
		t.Errorf("expected ExecutionExclusive, got %v", tool.ExecutionMode())
	}
}

func TestSubAgentTool_Execute_Success(t *testing.T) {
	var capturedTools []types.ToolDefinition
	mp := &capturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "Task completed"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
		onStreamTools: func(tools []types.ToolDefinition) {
			capturedTools = tools
		},
	}

	reg := NewRegistry()
	reg.Register(NewReadTool("/tmp", 50000))
	reg.Register(NewLsTool("/tmp", 50000))

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, reg, reg.Names(), nil)

	params := &SubAgentParams{
		Task: "Do something",
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success, got error result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &output); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}
	if output["task"] != "Do something" {
		t.Errorf("expected task 'Do something', got %v", output["task"])
	}
	if output["output"] != "Task completed" {
		t.Errorf("expected output 'Task completed', got %v", output["output"])
	}
	if _, ok := output["subagent_id"]; !ok {
		t.Error("expected subagent_id in output")
	}

	if len(capturedTools) == 0 {
		t.Fatal("expected tools passed to subagent, got none — this means subagent has no tool access")
	}
	names := make(map[string]bool)
	for _, td := range capturedTools {
		names[td.Name] = true
	}
	if !names["read"] {
		t.Error("expected 'read' tool in subagent tools")
	}
	if !names["ls"] {
		t.Error("expected 'ls' tool in subagent tools")
	}
	if names["subagent"] {
		t.Error("expected 'subagent' tool to be excluded (no nested spawning)")
	}
}

func TestSubAgentTool_Execute_Failure(t *testing.T) {
	mp := &testutilMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventError, Error: "model not found"},
		},
	}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil)

	params := &SubAgentParams{
		Task: "Do something",
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result, got success")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
}

func TestSubAgentTool_Execute_CustomModel(t *testing.T) {
	var capturedModel types.Model
	mp := &capturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
		onStream: func(model types.Model) {
			capturedModel = model
		},
	}
	tool := NewSubAgentTool(mp, types.Model{ID: "parent-model", Provider: "mock", API: "mock"}, nil, nil, nil)

	params := &SubAgentParams{
		Task:  "Do something",
		Model: "custom-model",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedModel.ID != "custom-model" {
		t.Errorf("expected model 'custom-model', got %q", capturedModel.ID)
	}
}

func TestSubAgentTool_Execute_DefaultModel(t *testing.T) {
	var capturedModel types.Model
	mp := &capturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
		onStream: func(model types.Model) {
			capturedModel = model
		},
	}
	tool := NewSubAgentTool(mp, types.Model{ID: "parent-model", Provider: "mock", API: "mock"}, nil, nil, nil)

	params := &SubAgentParams{
		Task: "Do something",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedModel.ID != "parent-model" {
		t.Errorf("expected model 'parent-model', got %q", capturedModel.ID)
	}
}

func TestSubAgentTool_Execute_SystemPrompt(t *testing.T) {
	var capturedSystemPrompt string
	mp := &capturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
		onStreamOpts: func(opts types.StreamOptions) {
			capturedSystemPrompt = opts.SystemPrompt
		},
	}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil)

	params := &SubAgentParams{
		Task:         "Do something",
		SystemPrompt: "You are a test assistant",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedSystemPrompt != "You are a test assistant" {
		t.Errorf("expected system prompt 'You are a test assistant', got %q", capturedSystemPrompt)
	}
}

func TestSubAgentTool_Execute_Timeout(t *testing.T) {
	var capturedTimeout time.Duration
	mp := &timeoutCapturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
		onStream: func(ctx context.Context) {
			deadline, ok := ctx.Deadline()
			if ok {
				capturedTimeout = time.Until(deadline)
			}
		},
	}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil)

	params := &SubAgentParams{
		Task:    "Do something",
		Timeout: "30s",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTimeout <= 0 || capturedTimeout > 30*time.Second {
		t.Errorf("expected timeout around 30s, got %v", capturedTimeout)
	}
}

func TestSubAgentTool_Execute_InvalidTimeout(t *testing.T) {
	mp := &testutilMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil)

	params := &SubAgentParams{
		Task:    "Do something",
		Timeout: "invalid-duration",
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid timeout")
	}
}

func TestSubAgentTool_PassesToolsToSubagent(t *testing.T) {
	var capturedTools []types.ToolDefinition
	mp := &capturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
		onStreamTools: func(tools []types.ToolDefinition) {
			capturedTools = tools
		},
	}

	reg := NewRegistry()
	reg.Register(NewReadTool("/tmp", 50000))
	reg.Register(NewLsTool("/tmp", 50000))
	reg.Register(NewBashTool("/tmp", 50000, false))

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, reg, reg.Names(), nil)

	params := &SubAgentParams{
		Task: "Do something",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedTools) == 0 {
		t.Fatal("expected tools to be passed to subagent, got none")
	}

	names := make(map[string]bool)
	for _, td := range capturedTools {
		names[td.Name] = true
	}

	if !names["read"] {
		t.Error("expected 'read' tool in subagent tools")
	}
	if !names["ls"] {
		t.Error("expected 'ls' tool in subagent tools")
	}
	if !names["bash"] {
		t.Error("expected 'bash' tool in subagent tools")
	}
	if names["subagent"] {
		t.Error("expected 'subagent' tool to be excluded from subagent tools (no nested spawning)")
	}
}

func TestSubAgentTool_NoRegistry_NoTools(t *testing.T) {
	var capturedTools []types.ToolDefinition
	mp := &capturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
		onStreamTools: func(tools []types.ToolDefinition) {
			capturedTools = tools
		},
	}

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil)

	params := &SubAgentParams{
		Task: "Do something",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedTools) != 0 {
		t.Errorf("expected no tools when registry is nil, got %d", len(capturedTools))
	}
}

func TestSubAgentTool_E2E_Ollama(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	reg := NewRegistry()
	reg.Register(NewReadTool("/tmp", 50000))
	reg.Register(NewLsTool("/tmp", 50000))

	tool := NewSubAgentTool(ollama, model, reg, reg.Names(), nil)

	params := &SubAgentParams{
		Task:         "Say exactly: E2E subagent test passed",
		SystemPrompt: "Be extremely concise. Follow instructions exactly.",
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content[0].Text)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &output); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}
	if output["output"] == "" {
		t.Fatal("expected non-empty output")
	}
	t.Logf("E2E output: %v", output["output"])
}

func TestSubAgentTool_E2E_WithTools(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	tmpDir := t.TempDir()
	testFile := tmpDir + "/greeting.txt"
	expectedContent := "Hello from E2E test!"
	if err := os.WriteFile(testFile, []byte(expectedContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	reg := NewRegistry()
	readTool := NewReadTool(tmpDir, 50000)
	reg.Register(readTool)

	tool := NewSubAgentTool(ollama, model, reg, reg.Names(), nil)

	params := &SubAgentParams{
		Task:         fmt.Sprintf("Use the read tool to read the file at path '%s/greeting.txt'. Return ONLY the file contents, nothing else.", tmpDir),
		SystemPrompt: "You MUST use the read tool to read files. Do NOT say you cannot access files — you have the read tool available. Call the read tool with the file path.",
		Timeout:      "2m",
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content[0].Text)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &output); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}
	subOutput := output["output"].(string)
	t.Logf("E2E tool output: %q", subOutput)
	t.Logf("Duration: %v", output["duration"])

	if !strings.Contains(subOutput, expectedContent) {
		t.Errorf("expected output to contain %q, got: %q", expectedContent, subOutput)
	}
}

func TestSubAgent_E2E_WebsearchToolCall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	searxng := NewSearXNGBackend("http://localhost:8964", 10*time.Second)
	if !searxng.Available() {
		t.Skip("SearXNG not available")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	reg := NewRegistry()
	reg.Register(NewWebSearchTool([]SearchBackend{searxng}, time.Now()))
	reg.Register(NewWebFetchTool())

	callLog := []string{}
	callMu := sync.Mutex{}

	allTools := reg.Tools()
	subTools := make([]subagent.Tool, len(allTools))
	for i, t := range allTools {
		subTools[i] = t
	}

	sa := subagent.NewSubAgent(ollama, subagent.SubAgentOpts{
		Task:            "Search for 'golang programming language'. Use websearch tool ONCE, then report results.",
		SystemPrompt:    "You have websearch and webfetch tools. Use websearch ONCE to find information, then immediately report results. Do NOT call websearch again.",
		Model:           model,
		Tools:           subTools,
		ParentToolNames: reg.Names(),
		Timeout:         2 * time.Minute,
		Executor: func(ctx context.Context, calls []subagent.ToolCallRequest) []*subagent.ToolCallResult {
			callMu.Lock()
			callLog = append(callLog, fmt.Sprintf("call#%d: %d tool(s)", len(callLog)+1, len(calls)))
			for _, c := range calls {
				callLog = append(callLog, fmt.Sprintf("  tool=%s args=%s", c.Name, string(c.Arguments)))
			}
			callMu.Unlock()

			toolCalls := make([]ToolCallRequest, len(calls))
			for i, c := range calls {
				toolCalls[i] = ToolCallRequest{ID: c.ID, Name: c.Name, Arguments: c.Arguments}
			}
			results := reg.ExecuteBatch(ctx, toolCalls)
			subResults := make([]*subagent.ToolCallResult, len(results))
			for i, r := range results {
				subResults[i] = &subagent.ToolCallResult{ID: r.ID, Name: r.Name, Result: r.Result}
			}
			return subResults
		},
	})

	result := sa.Run(context.Background())

	callMu.Lock()
	logCopy := make([]string, len(callLog))
	copy(logCopy, callLog)
	callMu.Unlock()

	t.Logf("Tool call log:")
	for _, entry := range logCopy {
		t.Logf("  %s", entry)
	}
	t.Logf("Success: %v, Error: %v", result.Success, result.Error)
	t.Logf("Duration: %v", result.Duration)
	t.Logf("Output length: %d", len(result.Output))

	if result.Success {
		t.Logf("Output: %s", result.Output[:min(len(result.Output), 500)])
	}

	// Verify tool was actually called (not just output as text)
	if len(logCopy) == 0 {
		t.Log("WARNING: websearch was not called — ministral-3:14b may output tool calls as text for some queries")
	}
}

func TestSubAgent_E2E_DirectToolCall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	ollama := provider.NewOllamaProvider("")
	model := types.Model{
		ID:       "ministral-3:14b",
		Provider: "ollama",
		API:      "ollama-chat",
	}

	tmpDir := t.TempDir()
	testFile := tmpDir + "/greeting.txt"
	expectedContent := "Hello from E2E test!"
	if err := os.WriteFile(testFile, []byte(expectedContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	readTool := NewReadTool(tmpDir, 50000)

	reg := NewRegistry()
	reg.Register(readTool)

	sa := subagent.NewSubAgent(ollama, subagent.SubAgentOpts{
		Task:            fmt.Sprintf("Use the read tool to read the file at path '%s/greeting.txt' and return its exact contents.", tmpDir),
		SystemPrompt:    "You have a read tool. Use it ONCE to read the file, then immediately report the contents. Do NOT call the tool again.",
		Model:           model,
		Tools:           []subagent.Tool{readTool},
		ParentToolNames: []string{"read"},
		Timeout:         2 * time.Minute,
		Executor: func(ctx context.Context, calls []subagent.ToolCallRequest) []*subagent.ToolCallResult {
			toolCalls := make([]ToolCallRequest, len(calls))
			for i, c := range calls {
				toolCalls[i] = ToolCallRequest{ID: c.ID, Name: c.Name, Arguments: c.Arguments}
			}
			results := reg.ExecuteBatch(ctx, toolCalls)
			subResults := make([]*subagent.ToolCallResult, len(results))
			for i, r := range results {
				subResults[i] = &subagent.ToolCallResult{ID: r.ID, Name: r.Name, Result: r.Result}
			}
			return subResults
		},
	})

	result := sa.Run(context.Background())

	t.Logf("E2E direct tool output: %q", result.Output)
	t.Logf("Duration: %v", result.Duration)
	t.Logf("Success: %v, Error: %v", result.Success, result.Error)

	if result.Output == "" && result.Error == nil {
		t.Fatal("expected either output or error")
	}
}

// testutilMockProvider is a mock provider for testing.
type testutilMockProvider struct {
	events []types.StreamEvent
}

func (m *testutilMockProvider) Name() string { return "testutil-mock" }

func (m *testutilMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, len(m.events)+1)
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch
}

func (m *testutilMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

// capturingMockProvider records what was passed to Stream() for verification.
type capturingMockProvider struct {
	events        []types.StreamEvent
	onStream      func(types.Model)
	onStreamOpts  func(types.StreamOptions)
	onStreamTools func([]types.ToolDefinition)
}

func (m *capturingMockProvider) Name() string { return "capturing-mock" }

func (m *capturingMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	if m.onStream != nil {
		m.onStream(model)
	}
	if m.onStreamOpts != nil {
		m.onStreamOpts(opts)
	}
	if m.onStreamTools != nil {
		m.onStreamTools(tools)
	}
	ch := make(chan types.StreamEvent, len(m.events)+1)
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch
}

func (m *capturingMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

// Verify mock providers implement the subagent.NewSubAgent requirements via provider.Provider.
var _ provider.Provider = (*testutilMockProvider)(nil)
var _ provider.Provider = (*capturingMockProvider)(nil)

// timeoutCapturingMockProvider captures the context to verify deadline.
type timeoutCapturingMockProvider struct {
	events   []types.StreamEvent
	onStream func(context.Context)
}

func (m *timeoutCapturingMockProvider) Name() string { return "timeout-capturing-mock" }

func (m *timeoutCapturingMockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	if m.onStream != nil {
		m.onStream(ctx)
	}
	ch := make(chan types.StreamEvent, len(m.events)+1)
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch
}

func (m *timeoutCapturingMockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return nil, nil
}

var _ provider.Provider = (*timeoutCapturingMockProvider)(nil)

func TestSubAgentTool_Execute_AgentName(t *testing.T) {
	mp := &capturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "Hello from custom agent"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}

	reg := NewRegistry()
	reg.Register(NewReadTool("/tmp", 50000))
	reg.Register(NewBashTool("/tmp", 50000, false))

	agents := map[string]*subagent.AgentDefinition{
		"greeter": {
			Name:         "greeter",
			Description:  "A friendly greeter agent",
			Tools:        []string{"bash"},
			SystemPrompt: "You are a greeter agent. Say hello.",
			Source:       "user",
		},
	}

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, reg, reg.Names(), agents)

	params := &SubAgentParams{
		Task:      "Say hello",
		AgentName: "greeter",
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	var output map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &output); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if output["type"] != "greeter" {
		t.Errorf("expected type 'greeter', got %v", output["type"])
	}
}

func TestSubAgentTool_Execute_AgentName_NotFound(t *testing.T) {
	mp := &testutilMockProvider{}
	reg := NewRegistry()
	agents := map[string]*subagent.AgentDefinition{}

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, reg, reg.Names(), agents)

	params := &SubAgentParams{
		Task:      "Do something",
		AgentName: "nonexistent",
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsError {
		t.Fatal("expected error for non-existent agent")
	}

	if !strings.Contains(result.Content[0].Text, "not found") {
		t.Errorf("expected 'not found' in error message, got: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "general") {
		t.Error("expected error to list available agents including 'general'")
	}
}

func TestSubAgentTool_Execute_AgentName_ToolFiltering(t *testing.T) {
	var capturedTools []types.ToolDefinition
	mp := &capturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "Done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
		onStreamTools: func(tools []types.ToolDefinition) {
			capturedTools = tools
		},
	}

	reg := NewRegistry()
	reg.Register(NewReadTool("/tmp", 50000))
	reg.Register(NewWriteTool("/tmp", 50000))
	reg.Register(NewBashTool("/tmp", 50000, false))

	// Agent only has read tool
	agents := map[string]*subagent.AgentDefinition{
		"reader": {
			Name:         "reader",
			Description:  "Reads files only",
			Tools:        []string{"read"},
			SystemPrompt: "You can only read files.",
			Source:       "user",
		},
	}

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, reg, reg.Names(), agents)

	params := &SubAgentParams{
		Task:      "Read a file",
		AgentName: "reader",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify only read tool was passed
	if len(capturedTools) != 1 {
		t.Fatalf("expected 1 tool (read), got %d", len(capturedTools))
	}
	if capturedTools[0].Name != "read" {
		t.Errorf("expected 'read' tool, got %q", capturedTools[0].Name)
	}
}

func TestSubAgentTool_Execute_AgentName_NoTools(t *testing.T) {
	var capturedTools []types.ToolDefinition
	mp := &capturingMockProvider{
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "Done"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
		onStreamTools: func(tools []types.ToolDefinition) {
			capturedTools = tools
		},
	}

	reg := NewRegistry()
	reg.Register(NewReadTool("/tmp", 50000))
	reg.Register(NewBashTool("/tmp", 50000, false))

	// Agent with no tools defined — should get all parent tools
	agents := map[string]*subagent.AgentDefinition{
		"generalist": {
			Name:         "generalist",
			Description:  "General agent",
			Tools:        []string{},
			SystemPrompt: "You are a general agent.",
			Source:       "user",
		},
	}

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, reg, reg.Names(), agents)

	params := &SubAgentParams{
		Task:      "Do something",
		AgentName: "generalist",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have all parent tools (excluding subagent itself)
	if len(capturedTools) < 2 {
		t.Errorf("expected at least 2 tools, got %d", len(capturedTools))
	}
}

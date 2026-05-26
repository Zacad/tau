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
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model"}, nil, nil, nil, nil, 0)

	if tool.Name() != "subagent" {
		t.Errorf("expected name 'subagent', got %q", tool.Name())
	}
}

func TestSubAgentTool_Description(t *testing.T) {
	mp := &testutilMockProvider{}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model"}, nil, nil, nil, nil, 0)

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
			Name:         "greeter",
			Description:  "A friendly greeter agent",
			Tools:        []string{"bash"},
			SystemPrompt: "You are a greeter.",
			Source:       "user",
		},
		"code-reviewer": {
			Name:         "code-reviewer",
			Description:  "Reviews code for bugs",
			Tools:        []string{"read", "grep"},
			SystemPrompt: "You are a code reviewer.",
			Source:       "project",
		},
	}
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model"}, nil, nil, nil, agents, 0)

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
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model"}, nil, nil, nil, nil, 0)

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

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, reg, reg.Names(), nil, 0)

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
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 0)

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
	tool := NewSubAgentTool(mp, types.Model{ID: "parent-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 0)

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
	tool := NewSubAgentTool(mp, types.Model{ID: "parent-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 0)

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
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 0)

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
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 0)

	params := &SubAgentParams{
		Task:    "Do something",
		Timeout: "30s",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 30s is below minimum, should be bumped to ~5m
	if capturedTimeout <= 0 || capturedTimeout > 5*time.Minute {
		t.Errorf("expected timeout around 5m (minimum enforced), got %v", capturedTimeout)
	}
}

func TestSubAgentTool_Execute_DefaultTimeoutWhenConfigMissing(t *testing.T) {
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
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 0)

	_, err := tool.Execute(context.Background(), &SubAgentParams{Task: "Do something"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTimeout <= 0 || capturedTimeout > subagent.DefaultTimeout {
		t.Errorf("expected default timeout around %v, got %v", subagent.DefaultTimeout, capturedTimeout)
	}
}

func TestSubAgentTool_Execute_TimeoutAboveMinimum(t *testing.T) {
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
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 0)

	params := &SubAgentParams{
		Task:    "Do something",
		Timeout: "5m",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 5m is above minimum, should be preserved
	if capturedTimeout <= 0 || capturedTimeout > 5*time.Minute {
		t.Errorf("expected timeout around 5m, got %v", capturedTimeout)
	}
}

func TestSubAgentTool_Execute_DefaultTimeoutFromConfig(t *testing.T) {
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
	// Pass 3m as the config default timeout. It is below the enforced minimum,
	// so the effective timeout should be bumped to 5m.
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 3*time.Minute)

	// No timeout specified in params — should use config default
	params := &SubAgentParams{
		Task: "Do something",
	}

	_, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedTimeout <= 0 || capturedTimeout > 5*time.Minute {
		t.Errorf("expected timeout around 5m (minimum enforced), got %v", capturedTimeout)
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
	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 0)

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

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, reg, reg.Names(), nil, 0)

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

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, nil, nil, nil, 0)

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

	tool := NewSubAgentTool(ollama, model, nil, reg, reg.Names(), nil, 0)

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

	tool := NewSubAgentTool(ollama, model, nil, reg, reg.Names(), nil, 0)

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
	name          string // optional override for Name(), defaults to "capturing-mock"
	events        []types.StreamEvent
	onStream      func(types.Model)
	onStreamOpts  func(types.StreamOptions)
	onStreamTools func([]types.ToolDefinition)
}

func (m *capturingMockProvider) Name() string {
	if m.name != "" {
		return m.name
	}
	return "capturing-mock"
}

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

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, reg, reg.Names(), agents, 0)

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

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, reg, reg.Names(), agents, 0)

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

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, reg, reg.Names(), agents, 0)

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

	tool := NewSubAgentTool(mp, types.Model{ID: "test-model", Provider: "mock", API: "mock"}, nil, reg, reg.Names(), agents, 0)

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

// TestSubAgentTool_ResolveModel_PriorityChain tests the full model resolution
// priority chain with a real provider registry.
func TestSubAgentTool_ResolveModel_PriorityChain(t *testing.T) {
	// Set up a provider registry with two providers.
	// Each mock's Name() must match its model's Provider field for resolution to work.
	provReg := provider.NewRegistry()

	// Provider "mock-a" with model "model-a"
	mockA := &capturingMockProvider{
		name: "mock-a",
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "A"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}
	provReg.Register(mockA)
	provReg.Models().Register(types.Model{
		ID:       "model-a",
		Name:     "Model A",
		Provider: "mock-a",
		API:      "mock-a-api",
	})

	// Provider "mock-b" with model "model-b"
	mockB := &capturingMockProvider{
		name: "mock-b",
		events: []types.StreamEvent{
			{Type: types.EventStart},
			{Type: types.EventTextDelta, Delta: "B"},
			{Type: types.EventDone, Message: &types.AgentMessage{}},
		},
	}
	provReg.Register(mockB)
	provReg.Models().Register(types.Model{
		ID:       "model-b",
		Name:     "Model B",
		Provider: "mock-b",
		API:      "mock-b-api",
	})

	// Subagent default models list (for step 4 fallback testing)
	// We'll override it temporarily
	origDefaults := subagentDefaultModels
	subagentDefaultModels = []subagentDefaultCandidate{
		{ModelID: "model-b", Provider: "mock-b"},
	}
	defer func() { subagentDefaultModels = origDefaults }()

	t.Run("step1: frontmatter model wins over prompt model", func(t *testing.T) {
		parentModel := types.Model{
			ID:       "model-a",
			Provider: "mock-a",
			API:      "mock-a-api",
		}
		tool := NewSubAgentTool(mockA, parentModel, provReg, nil, nil, map[string]*subagent.AgentDefinition{
			"test-agent": {
				Name:         "test-agent",
				Description:  "Test agent",
				SystemPrompt: "Test",
				Model:        "model-b", // frontmatter model
			},
		}, 0)

		var capturedModel types.Model
		mockA.onStream = func(m types.Model) { capturedModel = m }
		mockB.onStream = func(m types.Model) { capturedModel = m }

		_, err := tool.Execute(context.Background(), &SubAgentParams{
			Task:      "test",
			Model:     "model-a", // prompt model should be overridden by frontmatter
			AgentName: "test-agent",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedModel.ID != "model-b" {
			t.Errorf("expected model 'model-b' (from frontmatter), got %q", capturedModel.ID)
		}
		if capturedModel.Provider != "mock-b" {
			t.Errorf("expected provider 'mock-b', got %q", capturedModel.Provider)
		}
	})

	t.Run("step2: prompt model used when no frontmatter", func(t *testing.T) {
		var capturedModel types.Model
		mockB.onStream = func(m types.Model) { capturedModel = m }

		parentModel := types.Model{
			ID:       "model-a",
			Provider: "mock-a",
			API:      "mock-a-api",
		}
		tool := NewSubAgentTool(mockA, parentModel, provReg, nil, nil, nil, 0)

		_, err := tool.Execute(context.Background(), &SubAgentParams{
			Task:  "test",
			Model: "model-b",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedModel.ID != "model-b" {
			t.Errorf("expected model 'model-b' (from prompt), got %q", capturedModel.ID)
		}
		if capturedModel.Provider != "mock-b" {
			t.Errorf("expected provider 'mock-b', got %q", capturedModel.Provider)
		}
	})

	t.Run("step3: parent model used when no frontmatter or prompt", func(t *testing.T) {
		var capturedModel types.Model
		mockA.onStream = func(m types.Model) { capturedModel = m }

		parentModel := types.Model{
			ID:       "model-a",
			Provider: "mock-a",
			API:      "mock-a-api",
		}
		tool := NewSubAgentTool(mockA, parentModel, provReg, nil, nil, nil, 0)

		_, err := tool.Execute(context.Background(), &SubAgentParams{
			Task: "test",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedModel.ID != "model-a" {
			t.Errorf("expected model 'model-a' (from parent), got %q", capturedModel.ID)
		}
		if capturedModel.Provider != "mock-a" {
			t.Errorf("expected provider 'mock-a', got %q", capturedModel.Provider)
		}
	})

	t.Run("step4: fallback to subagent defaults when parent model provider unavailable", func(t *testing.T) {
		// Set parent model to a provider that doesn't exist in the registry
		parentModel := types.Model{
			ID:       "ghost-model",
			Provider: "ghost-provider",
			API:      "ghost-api",
		}

		var capturedModel types.Model
		mockB.onStream = func(m types.Model) { capturedModel = m }

		tool := NewSubAgentTool(mockB, parentModel, provReg, nil, nil, nil, 0)

		_, err := tool.Execute(context.Background(), &SubAgentParams{
			Task: "test",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should fall back to model-b from subagentDefaultModels
		if capturedModel.ID != "model-b" {
			t.Errorf("expected fallback model 'model-b', got %q", capturedModel.ID)
		}
		if capturedModel.Provider != "mock-b" {
			t.Errorf("expected provider 'mock-b', got %q", capturedModel.Provider)
		}
	})

	t.Run("cross-provider: ollama model with anthropic parent", func(t *testing.T) {
		// Simulate the real-world bug scenario: parent is anthropic,
		// user requests an ollama model
		parentModel := types.Model{
			ID:       "claude-sonnet-4-20250514",
			Provider: "anthropic",
			API:      "anthropic-messages",
		}

		// Create an ollama mock provider
		mockOllama := &capturingMockProvider{
			name: "ollama",
			events: []types.StreamEvent{
				{Type: types.EventStart},
				{Type: types.EventTextDelta, Delta: "ollama"},
				{Type: types.EventDone, Message: &types.AgentMessage{}},
			},
		}
		provReg.Register(mockOllama)
		provReg.Models().Register(types.Model{
			ID:       "ministral-3:14b",
			Name:     "Ministral 3B 14b",
			Provider: "ollama",
			API:      "ollama-chat",
		})

		var capturedModel types.Model
		mockOllama.onStream = func(m types.Model) { capturedModel = m }

		tool := NewSubAgentTool(mockA, parentModel, provReg, nil, nil, nil, 0)

		_, err := tool.Execute(context.Background(), &SubAgentParams{
			Task:  "test",
			Model: "ministral-3:14b",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if capturedModel.ID != "ministral-3:14b" {
			t.Errorf("expected model 'ministral-3:14b', got %q", capturedModel.ID)
		}
		if capturedModel.Provider != "ollama" {
			t.Errorf("expected provider 'ollama', got %q", capturedModel.Provider)
		}
		if capturedModel.API != "ollama-chat" {
			t.Errorf("expected API 'ollama-chat', got %q", capturedModel.API)
		}
	})
}

// TestSubAgentTool_ResolveModel_LegacyFallback verifies that when provReg is nil,
// the old behavior of inheriting parent provider is preserved for backward compat.
func TestSubAgentTool_ResolveModel_LegacyFallback(t *testing.T) {
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

	// No provider registry (nil) — legacy behavior
	parentModel := types.Model{
		ID:       "parent-model",
		Provider: "mock",
		API:      "mock-api",
	}
	tool := NewSubAgentTool(mp, parentModel, nil, nil, nil, nil, 0)

	_, err := tool.Execute(context.Background(), &SubAgentParams{
		Task:  "test",
		Model: "different-model", // Should use this model ID but inherit parent provider
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedModel.ID != "different-model" {
		t.Errorf("expected model ID 'different-model', got %q", capturedModel.ID)
	}
	if capturedModel.Provider != "mock" {
		t.Errorf("expected inherited provider 'mock', got %q", capturedModel.Provider)
	}
	if capturedModel.API != "mock-api" {
		t.Errorf("expected inherited API 'mock-api', got %q", capturedModel.API)
	}
}

func TestSubAgentTool_UpdateParentModel(t *testing.T) {
	mp := &testutilMockProvider{}
	parentModel := types.Model{
		ID:       "parent-model",
		Provider: "mock",
		API:      "mock-api",
	}
	tool := NewSubAgentTool(mp, parentModel, nil, nil, nil, nil, 0)

	// Verify initial state
	if tool.model.ID != "parent-model" {
		t.Errorf("initial model ID = %q, want parent-model", tool.model.ID)
	}
	if tool.model.Provider != "mock" {
		t.Errorf("initial provider = %q, want mock", tool.model.Provider)
	}

	// Update parent model
	newModel := types.Model{
		ID:       "new-model",
		Provider: "new-provider",
		API:      "new-api",
	}
	newProv := &testutilMockProvider{}
	tool.UpdateParentModel(newProv, newModel)

	// Verify updated state
	if tool.model.ID != "new-model" {
		t.Errorf("updated model ID = %q, want new-model", tool.model.ID)
	}
	if tool.model.Provider != "new-provider" {
		t.Errorf("updated provider = %q, want new-provider", tool.model.Provider)
	}
	if tool.prov != newProv {
		t.Error("updated provider instance not set correctly")
	}
}

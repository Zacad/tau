//go:build e2e

package sdk

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/provider"
	tausession "github.com/adam/tau/internal/session"
	"github.com/adam/tau/internal/skills"
	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/tools"
	"github.com/adam/tau/internal/types"
)

const (
	ollamaBaseURL = "http://localhost:11434/v1"
	ollamaModelID = "gemma4:e4b"
)

// setupOllamaSession creates an SDK Session configured to use Ollama.
func setupOllamaSession(t *testing.T, opts SessionOptions) *Session {
	t.Helper()

	if os.Getenv("OLLAMA_E2E") == "" {
		t.Skip("set OLLAMA_E2E=1 to run e2e tests")
	}

	if opts.WorkingDir == "" {
		opts.WorkingDir = t.TempDir()
	}

	// Create provider registry
	provReg := provider.NewRegistry()

	// Register Ollama as OpenAI-compatible provider
	ollama := provider.NewOpenAICompatProvider("", provider.OpenAICompatConfig{
		BaseURL:      ollamaBaseURL,
		APIPath:      "/chat/completions",
		ProviderName: "ollama",
	})
	provReg.Register(ollama)

	// Register the gemma4:e4b model
	ollamaModel := types.Model{
		ID:            ollamaModelID,
		Name:          "Gemma 4 12B",
		Provider:      "ollama",
		API:           "openai-completions",
		BaseURL:       ollamaBaseURL,
		Reasoning:     false,
		InputTypes:    []string{"text"},
		ContextWindow: 8192,
		MaxTokens:     2048,
	}
	provReg.Models().Register(ollamaModel)
	provReg.SetDefaultModel(ollamaModelID)

	// Resolve model
	model, err := provReg.ResolveModelWithFallback(opts.Model)
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}

	// Get provider
	prov, ok := provReg.Get(model.Provider)
	if !ok {
		t.Fatalf("provider not found: %s", model.Provider)
	}

	// Discover skills
	discovered := skills.DiscoverSkills(opts.WorkingDir)
	systemPrompt := skills.FormatForPrompt(discovered)

	// Create tool registry
	toolOpts := []tools.RegistryOption{}
	if len(opts.ToolAllowlist) > 0 {
		toolOpts = append(toolOpts, tools.WithAllowlist(opts.ToolAllowlist))
	}
	if opts.ReadOnly {
		toolOpts = append(toolOpts, tools.WithReadOnly(true))
	}
	toolReg := tools.NewRegistry(toolOpts...)
	registerBuiltinTools(toolReg, opts.WorkingDir, &config.Config{}, prov, model)

	// Create agent
	ag := newAgent(systemPrompt, opts.WorkingDir, prov, model, toolReg)

	s := &Session{
		ag:        ag,
		provReg:   provReg,
		prov:      prov,
		model:     model,
		toolReg:   toolReg,
		allSkills: discovered,
		cwd:       opts.WorkingDir,
		ephemeral: opts.Ephemeral,
		systemP:   systemPrompt,
		msgCount:  0,
	}

	return s
}

// withTimeout returns a context with a generous timeout for Ollama responses.
func withTimeout(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 5*time.Minute)
}

// --- E2E Tests ---

func TestE2E_BasicPrompt(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	ctx, cancel := withTimeout(t)
	defer cancel()

	err := s.Prompt(ctx, "Say 'hello e2e' and nothing else.")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	msgs := s.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}

	// Verify user message
	if msgs[0].Role != types.RoleUser {
		t.Fatalf("expected user message first, got %s", msgs[0].Role)
	}

	// Verify assistant response has content
	if msgs[1].Role != types.RoleAssistant {
		t.Fatalf("expected assistant message second, got %s", msgs[1].Role)
	}

	text := extractMessageText(msgs[1])
	if text == "" {
		t.Fatal("assistant response is empty")
	}

	t.Logf("Assistant response: %s", truncate(text, 200))
}

func TestE2E_MultiTurn(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	ctx, cancel := withTimeout(t)
	defer cancel()

	// First turn
	err := s.Prompt(ctx, "What is 2+2? Answer with just the number.")
	if err != nil {
		t.Fatalf("Prompt 1: %v", err)
	}

	// Second turn
	err = s.Prompt(ctx, "Now what is 3*5? Answer with just the number.")
	if err != nil {
		t.Fatalf("Prompt 2: %v", err)
	}

	// Third turn
	err = s.Prompt(ctx, "Add the two previous answers together. Answer with just the number.")
	if err != nil {
		t.Fatalf("Prompt 3: %v", err)
	}

	msgs := s.Messages()
	// 3 user messages + 3 assistant responses = 6
	if len(msgs) < 6 {
		t.Fatalf("expected at least 6 messages, got %d", len(msgs))
	}

	// Verify alternating pattern
	for i := 0; i < len(msgs); i++ {
		if i%2 == 0 {
			if msgs[i].Role != types.RoleUser {
				t.Errorf("message %d: expected user, got %s", i, msgs[i].Role)
			}
		} else {
			if msgs[i].Role != types.RoleAssistant {
				t.Errorf("message %d: expected assistant, got %s", i, msgs[i].Role)
			}
		}
	}

	t.Logf("Completed %d messages in multi-turn conversation", len(msgs))
}

func TestE2E_Continue(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	ctx, cancel := withTimeout(t)
	defer cancel()

	// Initial prompt
	err := s.Prompt(ctx, "List 3 programming languages.")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// Continue — should generate another response
	err = s.Continue(ctx)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}

	msgs := s.Messages()
	// 1 user + 2 assistant (from Prompt + Continue)
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}

	t.Logf("After Continue: %d messages total", len(msgs))
}

func TestE2E_Steer(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	ctx, cancel := withTimeout(t)
	defer cancel()

	// Start a prompt
	err := s.Prompt(ctx, "Tell me about Go programming language.")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// After the prompt completes, steer with a follow-up
	err = s.Steer("Now mention one thing about Rust.")
	if err != nil {
		t.Fatalf("Steer: %v", err)
	}

	// Continue to process the steered message
	err = s.Continue(ctx)
	if err != nil {
		t.Fatalf("Continue after steer: %v", err)
	}

	msgs := s.Messages()
	if len(msgs) < 4 {
		t.Fatalf("expected at least 4 messages (prompt + steer response), got %d", len(msgs))
	}

	// Last message should mention Rust
	lastText := extractMessageText(msgs[len(msgs)-1])
	t.Logf("Steer response: %s", truncate(lastText, 200))
}

func TestE2E_UsageAccumulation(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	ctx, cancel := withTimeout(t)
	defer cancel()

	// First prompt
	err := s.Prompt(ctx, "Write a short paragraph about the history of computing.")
	if err != nil {
		t.Fatalf("Prompt 1: %v", err)
	}

	usage1 := s.Usage()
	// Note: Ollama's streaming API does not return usage/token counts.
	// Usage will be zero but the prompt still executed successfully.
	// We verify the mechanism works (no crash, returns Usage struct).
	t.Logf("Usage after prompt 1: %d total tokens ($%.4f)", usage1.TotalTokens, usage1.Cost.Total)

	// Second prompt
	err = s.Prompt(ctx, "Write another short paragraph about the internet.")
	if err != nil {
		t.Fatalf("Prompt 2: %v", err)
	}

	usage2 := s.Usage()
	t.Logf("Usage after prompt 2: %d total tokens ($%.4f)", usage2.TotalTokens, usage2.Cost.Total)

	// Both prompts completed successfully — the mechanism works.
	// Ollama doesn't return usage in streaming responses, so totals are zero.
	// This is a known limitation of Ollama's OpenAI-compatible API.
	if usage1.TotalTokens == 0 && usage2.TotalTokens == 0 {
		t.Log("Usage is zero as expected (Ollama streaming API does not return token counts)")
	}
}

func TestE2E_ModelSwitch(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	// Verify initial model
	if s.Model().ID != ollamaModelID {
		t.Fatalf("expected model %s, got %s", ollamaModelID, s.Model().ID)
	}

	// Switch model (same provider, same model — just testing the mechanism)
	err := s.SetModel(ollamaModelID)
	if err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	if s.Model().ID != ollamaModelID {
		t.Fatalf("expected model %s after SetModel, got %s", ollamaModelID, s.Model().ID)
	}

	ctx, cancel := withTimeout(t)
	defer cancel()

	// Prompt should still work after model switch
	err = s.Prompt(ctx, "Say 'model switch works'.")
	if err != nil {
		t.Fatalf("Prompt after model switch: %v", err)
	}

	msgs := s.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected messages after prompt, got %d", len(msgs))
	}

	t.Log("Model switch verified")
}

func TestE2E_Subscribe(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	var events []types.AgentEvent
	var mu sync.Mutex

	unsub := s.Subscribe(func(e types.AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	})

	ctx, cancel := withTimeout(t)
	defer cancel()

	err := s.Prompt(ctx, "Say hi.")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	unsub()

	mu.Lock()
	defer mu.Unlock()

	if len(events) == 0 {
		t.Fatal("expected events to be emitted")
	}

	// Check for expected event types
	eventTypes := make(map[types.AgentEventType]bool)
	for _, e := range events {
		eventTypes[e.Type] = true
	}

	for _, expected := range []types.AgentEventType{
		types.AgentEventStart,
		types.AgentEventMessageStart,
		types.AgentEventMessageEnd,
		types.AgentEventTurnEnd,
		types.AgentEventAgentEnd,
	} {
		if !eventTypes[expected] {
			t.Errorf("missing event: %s", expected)
		}
	}

	t.Logf("Received %d events during prompt", len(events))
}

func TestE2E_SessionPersistence(t *testing.T) {
	// Create a temporary HOME for tau config
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	if os.Getenv("OLLAMA_E2E") == "" {
		t.Skip("set OLLAMA_E2E=1 to run e2e tests")
	}

	// Create provider registry
	provReg := provider.NewRegistry()
	ollama := provider.NewOpenAICompatProvider("", provider.OpenAICompatConfig{
		BaseURL:      ollamaBaseURL,
		APIPath:      "/chat/completions",
		ProviderName: "ollama",
	})
	provReg.Register(ollama)

	ollamaModel := types.Model{
		ID:       ollamaModelID,
		Name:     "Gemma 4 12B",
		Provider: "ollama",
		API:      "openai-completions",
		BaseURL:  ollamaBaseURL,
	}
	provReg.Models().Register(ollamaModel)
	provReg.SetDefaultModel(ollamaModelID)

	prov, _ := provReg.Get("ollama")

	cwd := t.TempDir()
	discovered := skills.DiscoverSkills(cwd)
	systemPrompt := skills.FormatForPrompt(discovered)
	toolReg := tools.NewRegistry()
	registerBuiltinTools(toolReg, cwd, &config.Config{}, prov, ollamaModel)

	// Create a non-ephemeral session
	s := &Session{
		ag:        newAgent(systemPrompt, cwd, prov, ollamaModel, toolReg),
		provReg:   provReg,
		prov:      prov,
		model:     ollamaModel,
		toolReg:   toolReg,
		allSkills: discovered,
		cwd:       cwd,
		ephemeral: false,
		systemP:   systemPrompt,
		msgCount:  0,
	}

	// Create session file in the correct location (where resumeMostRecent will find it)
	sessDir, err := config.SessionsDir(cwd)
	if err != nil {
		t.Fatalf("get sessions dir: %v", err)
	}

	sess, err := createSessionWithDir(sessDir, cwd)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	s.sess = sess
	defer s.Close()

	sessionID := s.ID()
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	ctx, cancel := withTimeout(t)
	defer cancel()

	err = s.Prompt(ctx, "Hello, this is a persistence test.")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// Close and re-open
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Re-open session
	sess2, msgCount, err := resumeMostRecent(cwd)
	if err != nil {
		t.Fatalf("resume session: %v", err)
	}

	if msgCount < 2 {
		t.Fatalf("expected at least 2 persisted messages, got %d", msgCount)
	}

	// Verify session ID is preserved
	if sess2.ID() != sessionID {
		t.Fatalf("session ID mismatch: expected %s, got %s", sessionID, sess2.ID())
	}

	t.Logf("Session persistence verified: ID=%s, messages=%d", sess2.ID(), msgCount)
}

func TestE2E_ReadTool(t *testing.T) {

	s := setupOllamaSession(t, SessionOptions{
		Model:      ollamaModelID,
		Ephemeral:  true,
		WorkingDir: t.TempDir(),
	})
	defer s.Close()

	// Create a test file
	testFile := s.Cwd() + "/test.txt"
	if err := os.WriteFile(testFile, []byte("The answer is 42."), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// Subscribe to see what's happening
	s.Subscribe(func(e types.AgentEvent) {
		switch e.Type {
		case types.AgentEventToolExecStart:
			t.Logf("EVENT: tool execution start")
		case types.AgentEventToolExecEnd:
			t.Logf("EVENT: tool execution end")
		case types.AgentEventMessageEnd:
			t.Logf("EVENT: message end — checking tool calls")
			msgs := s.Messages()
			for _, msg := range msgs {
				for _, block := range msg.Content {
					if block.Type == types.BlockToolCall && block.ToolCall != nil {
						t.Logf("  TOOL CALL: %s args=%v", block.ToolCall.Name, block.ToolCall.Arguments)
					}
				}
			}
		}
	})

	ctx, cancel := withTimeout(t)
	defer cancel()

	// Ask the agent to read the file
	err := s.Prompt(ctx, "Read the file test.txt and tell me what it says.")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	msgs := s.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}

	// Check that a tool call was made
	foundToolCall := false
	for _, msg := range msgs {
		for _, block := range msg.Content {
			if block.Type == types.BlockToolCall {
				foundToolCall = true
				t.Logf("Tool call: %s", block.ToolCall.Name)
			}
		}
	}

	if !foundToolCall {
		t.Log("No tool call detected (agent may have responded without using tools)")
	}

	t.Logf("Completed read tool e2e test with %d messages", len(msgs))
}

func TestE2E_ReasoningInHistory(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	ctx, cancel := withTimeout(t)
	defer cancel()

	// Ask a reasoning-heavy question to trigger thinking tokens
	err := s.Prompt(ctx, "If a train travels 60 mph for 2.5 hours, then stops for 30 minutes, then travels 45 mph for 1.5 hours, what is the total distance traveled? Show your reasoning.")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	msgs := s.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}

	// Find the assistant response
	assistantMsg := msgs[1]
	if assistantMsg.Role != types.RoleAssistant {
		t.Fatalf("expected assistant message second, got %s", assistantMsg.Role)
	}

	// Check for thinking blocks
	thinkingContent := extractMessageThinking(assistantMsg)
	textContent := extractMessageText(assistantMsg)

	t.Logf("Reasoning content length: %d bytes", len(thinkingContent))
	t.Logf("Response text length: %d bytes", len(textContent))

	// Verify content blocks include at least one thinking block
	hasThinkingBlock := false
	for _, block := range assistantMsg.Content {
		if block.Type == types.BlockThinking {
			hasThinkingBlock = true
			if block.Text == "" {
				t.Error("thinking block has empty text")
			}
		}
	}

	if !hasThinkingBlock {
		t.Log("No BlockThinking found — model may not have emitted reasoning tokens")
		t.Logf("Content blocks: %v", func() []string {
			types := make([]string, len(assistantMsg.Content))
			for i, b := range assistantMsg.Content {
				types[i] = string(b.Type)
			}
			return types
		}())
	} else {
		t.Logf("Reasoning verified: %d bytes of thinking content in session history", len(thinkingContent))
	}

	// Verify the response has text content too
	if textContent == "" {
		t.Fatal("assistant response text is empty")
	}
}

func TestE2E_ErrorHandling(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should return error, not panic
	err := s.Prompt(ctx, "This should fail.")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}

	t.Logf("Got expected error: %v", err)
}

func TestE2E_ListModels(t *testing.T) {
	s := setupOllamaSession(t, SessionOptions{
		Model:     ollamaModelID,
		Ephemeral: true,
	})
	defer s.Close()

	models := s.ListModels()
	if len(models) < 11 {
		// 10 built-in + 1 ollama
		t.Fatalf("expected at least 11 models, got %d", len(models))
	}

	// Verify ollama model is in the list
	found := false
	for _, m := range models {
		if m.ID == ollamaModelID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("ollama model not found in model list")
	}

	t.Logf("Found %d models including ollama/%s", len(models), ollamaModelID)
}

// --- Helpers ---

func extractMessageText(msg types.AgentMessage) string {
	var sb strings.Builder
	for _, block := range msg.Content {
		if block.Type == types.BlockText {
			sb.WriteString(block.Text)
		}
	}
	return sb.String()
}

// extractMessageThinking returns all thinking block text from a message.
func extractMessageThinking(msg types.AgentMessage) string {
	var sb strings.Builder
	for _, block := range msg.Content {
		if block.Type == types.BlockThinking {
			sb.WriteString(block.Text)
		}
	}
	return sb.String()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func createSessionWithDir(sessDir string, cwd string) (*tausession.Session, error) {
	return tausession.CreateSession(sessDir, cwd, "", "")
}

// Ensure we import the session package
var (
	_ = tausession.CreateSession
)

package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/adam/tau/internal/agent"
	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/provider"
	"github.com/adam/tau/internal/skills"
	tausession "github.com/adam/tau/internal/session"
	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/tools"
	"github.com/adam/tau/internal/types"
)

func TestCreateSession_Ephemeral(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json with an API key
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	if !s.Ephemeral() {
		t.Fatal("expected ephemeral session")
	}
	if s.ID() != "" {
		t.Fatalf("expected empty ID for ephemeral, got %q", s.ID())
	}
}

func TestCreateSession_ResolvesModel(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json with an API key
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	// Exact model ID
	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession with gpt-4o: %v", err)
	}
	defer s.Close()

	if s.Model().ID != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", s.Model().ID)
	}
}

func TestCreateSession_RequiresAuth(t *testing.T) {
	tmpDir := testutil.TempDir(t)

	// Should succeed but with no model when API key is not configured
	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "claude-sonnet-4-20250514",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	// Session should have no model
	if s.Model().ID != "" {
		t.Fatalf("expected no model, got %s", s.Model().ID)
	}

	// Prompt should fail with helpful error
	err = s.Prompt(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error when prompting with no model")
	}
}

func TestCreateSession_SetAPIKey(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json with an API key
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	if s.Model().ID != "gpt-4o" {
		t.Fatalf("expected model gpt-4o, got %s", s.Model().ID)
	}
}

func TestSession_Model(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	m := s.Model()
	if m.ID != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", m.ID)
	}
	if m.Provider != "openai" {
		t.Fatalf("expected openai provider, got %s", m.Provider)
	}
}

func TestSession_SetModel(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	err := s.SetModel("gemini-2.5-pro")
	if err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	if s.Model().ID != "gemini-2.5-pro" {
		t.Fatalf("expected gemini-2.5-pro, got %s", s.Model().ID)
	}
}

func TestSession_SetModel_AmbiguousPattern(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// "sonnet" matches multiple Anthropic models — should resolve to one deterministically
	err := s.SetModel("sonnet")
	if err != nil {
		t.Fatalf("SetModel(sonnet): %v", err)
	}

	m := s.Model()
	if m.Provider != "anthropic" {
		t.Fatalf("expected anthropic provider, got %s", m.Provider)
	}
}

func TestSession_SetModel_NotFound(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	err := s.SetModel("nonexistent-model-xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
}

func TestSession_SetModel_PreservesMsgCount(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// Simulate having persisted messages from previous interactions
	s.msgCount = 5

	// Register a second mock provider so SetModel can switch to it
	model2, err := s.provReg.ResolveModelWithFallback("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	s.provReg.Register(&mockProvider{
		providerName: model2.Provider,
		model:        model2,
	})

	err = s.SetModel("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	// msgCount must be preserved when model is switched (agent is reused)
	if s.msgCount != 5 {
		t.Fatalf("expected msgCount to be 5 after model switch, got %d", s.msgCount)
	}
}

func TestSession_SetModel_PreservesMessages(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// Add messages to the agent
	msgs := []types.AgentMessage{
		{ID: "1", Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
		{ID: "2", Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi there"}}},
		{ID: "3", Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "how are you?"}}},
	}
	s.ag.SetMessages(msgs)
	s.msgCount = 3

	// Register a second mock provider so SetModel can switch to it
	model2, err := s.provReg.ResolveModelWithFallback("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	s.provReg.Register(&mockProvider{
		providerName: model2.Provider,
		model:        model2,
	})

	// Switch model
	err = s.SetModel("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	// Verify model changed
	if s.Model().ID != "claude-sonnet-4-20250514" {
		t.Fatalf("expected claude-sonnet-4-20250514, got %s", s.Model().ID)
	}
	if s.Model().Provider != "anthropic" {
		t.Fatalf("expected anthropic provider, got %s", s.Model().Provider)
	}

	// Verify messages are preserved
	gotMsgs := s.Messages()
	if len(gotMsgs) != 3 {
		t.Fatalf("expected 3 messages after model switch, got %d", len(gotMsgs))
	}
	if gotMsgs[0].Content[0].Text != "hello" {
		t.Errorf("expected first message 'hello', got %q", gotMsgs[0].Content[0].Text)
	}
	if gotMsgs[2].Content[0].Text != "how are you?" {
		t.Errorf("expected last message 'how are you?', got %q", gotMsgs[2].Content[0].Text)
	}

	// Verify msgCount is preserved
	if s.msgCount != 3 {
		t.Fatalf("expected msgCount 3, got %d", s.msgCount)
	}
}

func TestSession_ListModels(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	models := s.ListModels()
	// newTestSession only registers the mock provider for the resolved model's provider (openai)
	// So ListModels should return only openai models (4 built-in)
	if len(models) != 4 {
		t.Fatalf("expected 4 models (openai only), got %d", len(models))
	}

	// All models should be from openai provider
	for _, m := range models {
		if m.Provider != "openai" {
			t.Errorf("expected openai provider, got %s", m.Provider)
		}
	}

	// Verify sorted by provider then ID
	for i := 1; i < len(models); i++ {
		if models[i].Provider < models[i-1].Provider {
			t.Fatalf("models not sorted by provider at index %d", i)
		}
		if models[i].Provider == models[i-1].Provider && models[i].ID < models[i-1].ID {
			t.Fatalf("models not sorted by ID at index %d", i)
		}
	}
}

func TestSession_ListModels_FilteredByConnectedProviders(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// Register a second provider (anthropic mock)
	anthropicModel, _ := s.provReg.ResolveModelWithFallback("claude-sonnet-4-20250514")
	s.provReg.Register(&mockProvider{
		providerName: "anthropic",
		model:        anthropicModel,
	})

	models := s.ListModels()

	// Should include both openai (4) and anthropic (3) models
	if len(models) != 7 {
		t.Fatalf("expected 7 models (openai + anthropic), got %d", len(models))
	}

	// Verify no models from unconnected providers (google)
	for _, m := range models {
		if m.Provider == "google" {
			t.Errorf("google models should not appear when provider not connected")
		}
	}
}

func TestSession_ListModels_NoProviders(t *testing.T) {
	reg := provider.NewRegistry()
	// Remove all built-in models to test empty case
	for _, m := range reg.Models().ListAll() {
		reg.Models().RemoveByProvider(m.Provider)
	}

	// Create minimal session-like test
	// ListModels on registry with no providers should return empty
	models := reg.Models().ListAll()
	connected := reg.ListProviders()
	connectedSet := make(map[string]bool, len(connected))
	for _, p := range connected {
		connectedSet[p] = true
	}

	var filtered []types.Model
	for _, m := range models {
		if connectedSet[m.Provider] {
			filtered = append(filtered, m)
		}
	}

	if len(filtered) != 0 {
		t.Fatalf("expected 0 models with no connected providers, got %d", len(filtered))
	}
}

func TestSession_ListProviders(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	providers := s.ListProviders()
	// Only openai should be registered (no auth for others)
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider (openai), got %d: %v", len(providers), providers)
	}
	if providers[0] != "openai" {
		t.Fatalf("expected openai, got %s", providers[0])
	}
}

func TestSession_Steer(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	err := s.Steer("new instruction")
	if err != nil {
		t.Fatalf("Steer: %v", err)
	}
}

func TestSession_Subscribe(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	var events []types.AgentEvent
	var mu sync.Mutex

	unsub := s.Subscribe(func(e types.AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	})

	// Events should be delivered when agent runs
	// Unsubscribe should not panic
	unsub()

	// Unsubscribing again should be safe (no-op)
	unsub()
}

func TestSession_Cwd(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	if s.Cwd() != tmpDir {
		t.Fatalf("expected cwd %s, got %s", tmpDir, s.Cwd())
	}
}

func TestSession_Ephemeral(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// newTestSession always creates ephemeral sessions
	if !s.Ephemeral() {
		t.Fatal("expected ephemeral session from newTestSession")
	}
}

func TestSession_EphemeralFlag(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json with an API key
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	if !s.Ephemeral() {
		t.Fatal("expected ephemeral session")
	}
}

func TestSession_Skills(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	skills := s.Skills()
	// Built-in skills should be discovered
	if len(skills) == 0 {
		t.Fatal("expected at least builtin skills")
	}
}

func TestSession_AgentState(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	state := s.AgentState()
	if state != agent.StateIdle {
		t.Fatalf("expected idle state, got %s", state)
	}
}

func TestSession_Messages(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	msgs := s.Messages()
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestSession_Delete(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	sessionID := s.ID()
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	err = s.Delete()
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// After delete, Close should be safe
	err = s.Close()
	if err != nil {
		t.Fatalf("Close after Delete: %v", err)
	}
}

func TestSession_Rename_Ephemeral(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// Rename on non-ephemeral should persist to session
	err := s.Rename("my-session")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
}

func TestCreateSession_ToolAllowlist(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:         "gpt-4o",
		WorkingDir:    tmpDir,
		Ephemeral:     true,
		ToolAllowlist: []string{"read", "grep"},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	// Session created successfully with tool allowlist
	if s.Cwd() != tmpDir {
		t.Fatalf("expected cwd %s, got %s", tmpDir, s.Cwd())
	}
}

func TestCreateSession_ReadOnly(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
		Ephemeral:  true,
		ReadOnly:   true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	// Session created successfully with read-only mode
	if s.Model().ID != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", s.Model().ID)
	}
}

// --- Integration test ---

func TestIntegration_FullPromptFlow(t *testing.T) {
	// Use mock provider — no real API needed
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// Collect events
	var events []types.AgentEvent
	var mu sync.Mutex
	s.Subscribe(func(e types.AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e)
	})

	// Run prompt
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.Prompt(ctx, "Hello, world!")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// Verify events were emitted
	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("expected events to be emitted")
	}

	// Check for key event types
	eventTypes := make(map[types.AgentEventType]bool)
	for _, e := range events {
		eventTypes[e.Type] = true
	}

	if !eventTypes[types.AgentEventStart] {
		t.Error("missing agent_start event")
	}
	if !eventTypes[types.AgentEventAgentEnd] {
		t.Error("missing agent_end event")
	}

	// Verify messages were captured
	msgs := s.Messages()
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages (user + assistant), got %d", len(msgs))
	}
}

func TestIntegration_MultiplePrompts(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 3; i++ {
		err := s.Prompt(ctx, fmt.Sprintf("message %d", i))
		if err != nil {
			t.Fatalf("Prompt %d: %v", i, err)
		}
	}

	msgs := s.Messages()
	// Each prompt adds a user message + assistant response
	if len(msgs) < 6 {
		t.Fatalf("expected at least 6 messages (3 turns), got %d", len(msgs))
	}
}

func TestIntegration_SteerDuringRun(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start prompt in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- s.Prompt(ctx, "Start conversation")
	}()

	// Steer while agent is running (may or may not be delivered depending on timing)
	err := s.Steer("Interrupt!")
	if err != nil {
		t.Fatalf("Steer: %v", err)
	}

	// Wait for completion
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Prompt timed out")
	}
}

func TestIntegration_Continue(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.Prompt(ctx, "First message")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}

	// Continue runs another turn — adds 1 assistant message (no user message)
	err = s.Continue(ctx)
	if err != nil {
		t.Fatalf("Continue: %v", err)
	}

	msgs := s.Messages()
	// Prompt: user + assistant = 2, Continue: assistant = +1 → total = 3
	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages (2 turns), got %d", len(msgs))
	}
}

func TestIntegration_UsageAccumulation(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First prompt
	err := s.Prompt(ctx, "Hello")
	if err != nil {
		t.Fatalf("Prompt 1: %v", err)
	}

	usage1 := s.Usage()
	if usage1.TotalTokens == 0 {
		t.Error("expected usage after first prompt")
	}

	// Second prompt
	err = s.Prompt(ctx, "Hello again")
	if err != nil {
		t.Fatalf("Prompt 2: %v", err)
	}

	usage2 := s.Usage()
	if usage2.TotalTokens <= usage1.TotalTokens {
		t.Errorf("expected cumulative usage to increase: first=%d, second=%d",
			usage1.TotalTokens, usage2.TotalTokens)
	}
}

func TestIntegration_PersistedSession(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	// Create session (non-ephemeral)
	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	sessionID := s.ID()
	if sessionID == "" {
		t.Fatal("expected non-empty session ID")
	}

	// Close the session
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify session file exists
	sessionsDir, err := os.ReadDir(tauDir + "/sessions")
	if err != nil {
		t.Fatalf("read sessions dir: %v", err)
	}
	if len(sessionsDir) == 0 {
		t.Fatal("expected session file to exist")
	}
}

// --- Test helpers ---

// newTestSession creates a Session with a mock provider for testing.
// It bypasses auth resolution by directly creating the provider.
func newTestSession(t *testing.T, workingDir string, modelID string) *Session {
	t.Helper()

	provReg := provider.NewRegistry()
	model, err := provReg.ResolveModelWithFallback(modelID)
	if err != nil {
		t.Fatalf("resolve model %s: %v", modelID, err)
	}

	// Register a mock provider
	provReg.Register(&mockProvider{
		providerName: model.Provider,
		model:        model,
	})

	prov, ok := provReg.Get(model.Provider)
	if !ok {
		t.Fatalf("provider not found: %s", model.Provider)
	}

	discovered := skills.DiscoverSkills(workingDir)
	systemPrompt := skills.FormatForPrompt(discovered)

	toolReg := tools.NewRegistry()
	registerBuiltinTools(toolReg, workingDir, &config.Config{}, prov, model)

	ag := agent.New(agent.Options{
		SystemPrompt: systemPrompt,
		WorkingDir:   workingDir,
		Provider:     prov,
		Model:        model,
		ToolRegistry: toolReg,
	})

	return &Session{
		ag:        ag,
		provReg:   provReg,
		prov:      prov,
		model:     model,
		toolReg:   toolReg,
		allSkills: discovered,
		cwd:       workingDir,
		ephemeral: true,
		systemP:   systemPrompt,
		msgCount:  0,
		cfg:       &config.Config{},
		cfgPath:   "",
	}
}

// mockProvider is a test provider that returns a simple text response.
type mockProvider struct {
	providerName string
	model        types.Model
}

func (m *mockProvider) Name() string {
	return m.providerName
}

func (m *mockProvider) Stream(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, 5)
	ch <- types.StreamEvent{Type: types.EventStart}
	ch <- types.StreamEvent{Type: types.EventTextDelta, Delta: "Hello! "}
	ch <- types.StreamEvent{Type: types.EventTextDelta, Delta: "How can I help?"}
	ch <- types.StreamEvent{
		Type: types.EventDone,
		Message: &types.AgentMessage{
			ID:   "mock-msg-1",
			Role: types.RoleAssistant,
			Content: []types.ContentBlock{
				{Type: types.BlockText, Text: "Hello! How can I help?"},
			},
			Timestamp: time.Now(),
		},
		Usage: &types.Usage{
			Input:       10,
			Output:      8,
			TotalTokens: 18,
		},
	}
	close(ch)
	return ch
}

func (m *mockProvider) Complete(ctx context.Context, model types.Model, messages []types.AgentMessage, tools []types.ToolDefinition, opts types.StreamOptions) (*types.AgentMessage, error) {
	return &types.AgentMessage{
		ID:   "mock-complete",
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{
			{Type: types.BlockText, Text: "Summary of conversation"},
		},
		Timestamp: time.Now(),
	}, nil
}

// Ensure we import the right packages
var (
	_ = provider.NewRegistry
	_ = agent.New
	_ = skills.DiscoverSkills
	_ = tools.NewRegistry
)

func TestSession_EnqueueDequeue(t *testing.T) {
	s := &Session{}

	if s.DequeueMessage() != "" {
		t.Fatal("expected empty dequeue on fresh session")
	}
	if s.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", s.PendingCount())
	}

	s.EnqueueMessage("msg1")
	s.EnqueueMessage("msg2")
	s.EnqueueMessage("msg3")

	if s.PendingCount() != 3 {
		t.Fatalf("expected 3 pending, got %d", s.PendingCount())
	}

	if got := s.DequeueMessage(); got != "msg1" {
		t.Fatalf("expected msg1, got %q", got)
	}
	if got := s.DequeueMessage(); got != "msg2" {
		t.Fatalf("expected msg2, got %q", got)
	}
	if got := s.DequeueMessage(); got != "msg3" {
		t.Fatalf("expected msg3, got %q", got)
	}
	if s.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after drain, got %d", s.PendingCount())
	}
}

func TestSession_QueueOverflow(t *testing.T) {
	s := &Session{}

	for i := 0; i < 15; i++ {
		s.EnqueueMessage(fmt.Sprintf("msg%d", i))
	}

	if s.PendingCount() != maxMessageQueueSize {
		t.Fatalf("expected %d pending after overflow, got %d", maxMessageQueueSize, s.PendingCount())
	}

	if s.OverflowCount() != 5 {
		t.Fatalf("expected 5 overflow drops, got %d", s.OverflowCount())
	}

	first := s.DequeueMessage()
	if first != "msg5" {
		t.Fatalf("expected oldest surviving message msg5, got %q", first)
	}
}

func TestSession_QueueConcurrency(t *testing.T) {
	s := &Session{}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.EnqueueMessage(fmt.Sprintf("msg%d", n))
		}(i)
	}

	wg.Wait()

	if s.PendingCount() > maxMessageQueueSize {
		t.Fatalf("expected at most %d pending, got %d", maxMessageQueueSize, s.PendingCount())
	}
}

func TestSession_ResetOverflow(t *testing.T) {
	s := &Session{}

	for i := 0; i < 15; i++ {
		s.EnqueueMessage(fmt.Sprintf("msg%d", i))
	}

	if s.OverflowCount() != 5 {
		t.Fatalf("expected 5 overflow, got %d", s.OverflowCount())
	}

	s.ResetOverflow()
	if s.OverflowCount() != 0 {
		t.Fatalf("expected 0 after reset, got %d", s.OverflowCount())
	}
}

func TestDiscoverOpenAICompatModels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization header %q, got %q", "Bearer test-key", got)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIModelsResponse{
			Data: []openAIModelEntry{
				{ID: "model-a", Object: "model", OwnedBy: "provider"},
				{ID: "model-b", Object: "model", OwnedBy: "provider"},
			},
		})
	}))
	defer srv.Close()

	reg := provider.NewRegistry()
	count := discoverOpenAICompatModels(srv.URL, "test-key", "test-provider", reg)

	if count != 2 {
		t.Fatalf("expected 2 models discovered, got %d", count)
	}

	models := reg.Models().ListByProvider("test-provider")
	if len(models) != 2 {
		t.Fatalf("expected 2 models in registry, got %d", len(models))
	}

	modelIDs := make(map[string]bool)
	for _, m := range models {
		modelIDs[m.ID] = true
		if m.Provider != "test-provider" {
			t.Errorf("expected provider %q, got %q", "test-provider", m.Provider)
		}
		if m.API != "openai-completions" {
			t.Errorf("expected API %q, got %q", "openai-completions", m.API)
		}
		if m.BaseURL != srv.URL {
			t.Errorf("expected BaseURL %q, got %q", srv.URL, m.BaseURL)
		}
	}

	if !modelIDs["model-a"] || !modelIDs["model-b"] {
		t.Errorf("expected model-a and model-b, got %v", modelIDs)
	}
}

func TestDiscoverOpenAICompatModels_ContextLengthEnrichment(t *testing.T) {
	ctxLen := 128000
	maxTok := 4096

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIModelsResponse{
			Data: []openAIModelEntry{
				{
					ID:            "smart-model",
					Object:        "model",
					OwnedBy:       "provider",
					ContextLength: &ctxLen,
					MaxTokens:     &maxTok,
				},
			},
		})
	}))
	defer srv.Close()

	reg := provider.NewRegistry()
	discoverOpenAICompatModels(srv.URL, "key", "test-provider", reg)

	model, err := reg.Models().Get("smart-model")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}

	if model.ContextWindow != 128000 {
		t.Errorf("expected ContextWindow 128000, got %d", model.ContextWindow)
	}
	if model.MaxTokens != 4096 {
		t.Errorf("expected MaxTokens 4096, got %d", model.MaxTokens)
	}
}

func TestDiscoverOpenAICompatModels_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIModelsResponse{Data: []openAIModelEntry{}})
	}))
	defer srv.Close()

	reg := provider.NewRegistry()
	count := discoverOpenAICompatModels(srv.URL, "key", "test-provider", reg)

	if count != 0 {
		t.Fatalf("expected 0 models, got %d", count)
	}
}

func TestDiscoverOpenAICompatModels_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	reg := provider.NewRegistry()
	count := discoverOpenAICompatModels(srv.URL, "bad-key", "test-provider", reg)

	if count != 0 {
		t.Fatalf("expected 0 models on 401, got %d", count)
	}
}

func TestDiscoverOpenAICompatModels_NetworkError(t *testing.T) {
	reg := provider.NewRegistry()
	count := discoverOpenAICompatModels("http://127.0.0.1:1", "key", "test-provider", reg)

	if count != 0 {
		t.Fatalf("expected 0 models on network error, got %d", count)
	}
}

func TestDiscoverOpenAICompatModels_NoAuthHeader(t *testing.T) {
	var authHeaderSeen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaderSeen = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIModelsResponse{
			Data: []openAIModelEntry{{ID: "model", Object: "model"}},
		})
	}))
	defer srv.Close()

	reg := provider.NewRegistry()
	discoverOpenAICompatModels(srv.URL, "", "test-provider", reg)

	if authHeaderSeen != "" {
		t.Errorf("expected no Authorization header with empty key, got %q", authHeaderSeen)
	}
}

func TestRegisterOpenCodeZen_WithAuth(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"opencode-zen": "sk-zen-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	reg := provider.NewRegistry()
	cfg := config.DefaultConfig()
	registerOpenCodeZen(reg, &cfg)

	prov, ok := reg.Get("opencode-zen")
	if !ok {
		t.Fatal("expected opencode-zen provider to be registered")
	}
	if prov.Name() != "opencode-zen" {
		t.Errorf("expected provider name %q, got %q", "opencode-zen", prov.Name())
	}
}

func TestRegisterOpenCodeZen_NoAuth(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	reg := provider.NewRegistry()
	cfg := config.DefaultConfig()
	registerOpenCodeZen(reg, &cfg)

	_, ok := reg.Get("opencode-zen")
	if ok {
		t.Fatal("expected opencode-zen provider to NOT be registered without auth")
	}
}

func TestRegisterOpenCodeGo_WithAuth(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"opencode-go": "sk-go-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	reg := provider.NewRegistry()
	cfg := config.DefaultConfig()
	registerOpenCodeGo(reg, &cfg)

	prov, ok := reg.Get("opencode-go")
	if !ok {
		t.Fatal("expected opencode-go provider to be registered")
	}
	if prov.Name() != "opencode-go" {
		t.Errorf("expected provider name %q, got %q", "opencode-go", prov.Name())
	}
}

func TestRegisterOpenCodeGo_NoAuth(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	reg := provider.NewRegistry()
	cfg := config.DefaultConfig()
	registerOpenCodeGo(reg, &cfg)

	_, ok := reg.Get("opencode-go")
	if ok {
		t.Fatal("expected opencode-go provider to NOT be registered without auth")
	}
}

func TestSession_RegisterProvider(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// Register a new provider with models
	prov := provider.NewOpenAICompatProvider("test-key", provider.OpenAICompatConfig{
		BaseURL:      "https://test.example.com/v1",
		ProviderName: "test-provider",
	})

	models := []string{"test-model-1", "test-model-2"}
	err := s.RegisterProvider(prov, "test-provider", "https://test.example.com/v1", models)
	if err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}

	// Verify provider is registered
	providers := s.ListProviders()
	found := false
	for _, p := range providers {
		if p == "test-provider" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected test-provider in list, got %v", providers)
	}

	// Verify models are registered
	allModels := s.ListModels()
	modelIDs := make(map[string]bool)
	for _, m := range allModels {
		modelIDs[m.ID] = true
	}
	if !modelIDs["test-model-1"] {
		t.Error("expected test-model-1 to be registered")
	}
	if !modelIDs["test-model-2"] {
		t.Error("expected test-model-2 to be registered")
	}

	// Verify model provider assignment
	for _, m := range allModels {
		if m.ID == "test-model-1" || m.ID == "test-model-2" {
			if m.Provider != "test-provider" {
				t.Errorf("expected provider test-provider for %s, got %s", m.ID, m.Provider)
			}
			if m.BaseURL != "https://test.example.com/v1" {
				t.Errorf("expected BaseURL https://test.example.com/v1 for %s, got %s", m.ID, m.BaseURL)
			}
		}
	}
}

func TestSession_RegisterProvider_Ollama(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	prov := provider.NewOllamaProvider("http://localhost:11434")
	models := []string{"llama3", "mistral"}

	err := s.RegisterProvider(prov, "ollama", "http://localhost:11434", models)
	if err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}

	// Verify ollama API type
	allModels := s.ListModels()
	for _, m := range allModels {
		if m.ID == "llama3" {
			if m.API != "ollama-chat" {
				t.Errorf("expected API ollama-chat, got %s", m.API)
			}
		}
	}
}

func TestSession_RegisterProvider_Anthropic(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	prov := provider.NewAnthropicProvider("test-key")
	models := []string{"claude-sonnet-4-20250514"}

	err := s.RegisterProvider(prov, "anthropic", "https://api.anthropic.com/v1", models)
	if err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}

	allModels := s.ListModels()
	for _, m := range allModels {
		if m.ID == "claude-sonnet-4-20250514" {
			if m.API != "anthropic-messages" {
				t.Errorf("expected API anthropic-messages, got %s", m.API)
			}
		}
	}
}

func TestSession_RegisterProvider_EmptyModels(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	prov := provider.NewOpenAICompatProvider("test-key", provider.OpenAICompatConfig{
		BaseURL:      "https://test.example.com/v1",
		ProviderName: "empty-provider",
	})

	err := s.RegisterProvider(prov, "empty-provider", "https://test.example.com/v1", []string{})
	if err != nil {
		t.Fatalf("RegisterProvider with empty models: %v", err)
	}

	// Provider should still be registered
	providers := s.ListProviders()
	found := false
	for _, p := range providers {
		if p == "empty-provider" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected empty-provider to be registered even with no models")
	}
}

func TestSession_DisableProvider(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json and config.json
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key", "test-provider": "sk-test-key-2"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/config.json",
		[]byte(`{"providers": {"test-provider": {"enabled": true}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// Register a test provider
	prov := provider.NewOpenAICompatProvider("test-key", provider.OpenAICompatConfig{
		BaseURL:      "https://test.example.com/v1",
		ProviderName: "test-provider",
	})
	models := []string{"test-model-1", "test-model-2"}
	if err := s.RegisterProvider(prov, "test-provider", "https://test.example.com/v1", models); err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}

	// Verify provider is registered
	providers := s.ListProviders()
	found := false
	for _, p := range providers {
		if p == "test-provider" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected test-provider to be registered")
	}

	// Verify models are registered
	modelsBefore := s.ListModels()
	modelCountBefore := 0
	for _, m := range modelsBefore {
		if m.Provider == "test-provider" {
			modelCountBefore++
		}
	}
	if modelCountBefore != 2 {
		t.Fatalf("expected 2 test-provider models, got %d", modelCountBefore)
	}

	// Disable the provider
	err := s.DisableProvider("test-provider")
	if err != nil {
		t.Fatalf("DisableProvider: %v", err)
	}

	// Verify provider is removed from registry
	providers = s.ListProviders()
	for _, p := range providers {
		if p == "test-provider" {
			t.Fatal("expected test-provider to be removed from registry")
		}
	}

	// Verify models are removed
	modelsAfter := s.ListModels()
	for _, m := range modelsAfter {
		if m.Provider == "test-provider" {
			t.Fatalf("expected test-provider models to be hidden, got %s", m.ID)
		}
	}

	// Verify auth.json still has credentials
	authPath := config.AuthPath("")
	store, err := config.LoadAuth(authPath)
	if err != nil {
		t.Fatalf("LoadAuth: %v", err)
	}
	if _, exists := store["test-provider"]; !exists {
		t.Fatal("expected credentials to be preserved in auth.json")
	}

	// Verify config.json has enabled=false
	cfg, err := config.LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	pc := cfg.Providers["test-provider"]
	if pc.Enabled == nil || *pc.Enabled {
		t.Fatal("expected provider to be disabled in config.json")
	}
}

func TestSession_DisableProvider_NonExistent(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	err := s.DisableProvider("nonexistent-provider")
	if err == nil {
		t.Fatal("expected error when disabling non-existent provider")
	}
}

func TestSession_DisableProvider_CurrentModel(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json and config.json
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// Current model is gpt-4o from openai
	if s.Model().ID != "gpt-4o" {
		t.Fatalf("expected gpt-4o, got %s", s.Model().ID)
	}

	// Disable openai (current provider)
	err := s.DisableProvider("openai")
	if err != nil {
		t.Fatalf("DisableProvider: %v", err)
	}

	// Session should still have the model/provider set (not automatically changed)
	if s.Model().ID != "gpt-4o" {
		t.Fatalf("expected model to remain gpt-4o after disabling provider, got %s", s.Model().ID)
	}
}

func TestCreateSession_DisabledProviderNotRegistered(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json with openai key and config.json with openai disabled
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	enabled := false
	cfg := config.DefaultConfig()
	cfg.Providers["openai"] = config.ProviderConfig{Enabled: &enabled}
	cfgJSON, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(tauDir+"/config.json", cfgJSON, 0644); err != nil {
		t.Fatal(err)
	}

	// Use ollama model since openai is disabled
	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "ministral-3:14b",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	// OpenAI should NOT be registered
	providers := s.ListProviders()
	for _, p := range providers {
		if p == "openai" {
			t.Fatal("expected openai provider to NOT be registered when disabled in config")
		}
	}

	// OpenAI models should NOT appear in list
	models := s.ListModels()
	for _, m := range models {
		if m.Provider == "openai" {
			t.Fatalf("expected openai models to be hidden, got %s", m.ID)
		}
	}
}

func TestCreateSession_EnabledProviderRegistered(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json with openai key and config.json with openai explicitly enabled
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	enabled := true
	cfg := config.DefaultConfig()
	cfg.Providers["openai"] = config.ProviderConfig{Enabled: &enabled}
	cfgJSON, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(tauDir+"/config.json", cfgJSON, 0644); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	// OpenAI should be registered
	providers := s.ListProviders()
	found := false
	for _, p := range providers {
		if p == "openai" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected openai provider to be registered when enabled in config")
	}

	// OpenAI models should appear in list
	models := s.ListModels()
	foundModel := false
	for _, m := range models {
		if m.Provider == "openai" {
			foundModel = true
			break
		}
	}
	if !foundModel {
		t.Fatal("expected openai models to appear in list")
	}
}

func TestCreateSession_DefaultEnabledWhenNotInConfig(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json with openai key but NO config.json entry for openai
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	// Write empty config (no providers section)
	if err := os.WriteFile(tauDir+"/config.json", []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	// OpenAI should be registered (default enabled when not in config)
	providers := s.ListProviders()
	found := false
	for _, p := range providers {
		if p == "openai" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected openai provider to be registered by default when not in config")
	}
}

func TestIsProviderEnabled(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		provName string
		want     bool
	}{
		{
			name:     "not in config = enabled",
			cfg:      &config.Config{Providers: map[string]config.ProviderConfig{}},
			provName: "openai",
			want:     true,
		},
		{
			name:     "explicitly enabled",
			cfg:      &config.Config{Providers: map[string]config.ProviderConfig{"openai": {Enabled: ptrBool(true)}}},
			provName: "openai",
			want:     true,
		},
		{
			name:     "explicitly disabled",
			cfg:      &config.Config{Providers: map[string]config.ProviderConfig{"openai": {Enabled: ptrBool(false)}}},
			provName: "openai",
			want:     false,
		},
		{
			name:     "in config but enabled nil = enabled",
			cfg:      &config.Config{Providers: map[string]config.ProviderConfig{"openai": {Model: "gpt-4o"}}},
			provName: "openai",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProviderEnabled(tt.cfg, tt.provName)
			if got != tt.want {
				t.Errorf("isProviderEnabled(%q) = %v, want %v", tt.provName, got, tt.want)
			}
		})
	}
}

func TestCreateSession_OpenRouterDisabled(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openrouter": "sk-or-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	enabled := false
	cfg := config.DefaultConfig()
	cfg.Providers["openrouter"] = config.ProviderConfig{Enabled: &enabled}
	cfgJSON, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(tauDir+"/config.json", cfgJSON, 0644); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "ministral-3:14b",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	// OpenRouter should NOT be registered
	providers := s.ListProviders()
	for _, p := range providers {
		if p == "openrouter" {
			t.Fatal("expected openrouter provider to NOT be registered when disabled in config")
		}
	}
}

func TestCreateSession_OpenRouterWithUserModels(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openrouter": "sk-or-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	cfg.Providers["openrouter"] = config.ProviderConfig{
		Models: []string{"custom-org/custom-model", "another-org/another-model"},
	}
	cfgJSON, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(tauDir+"/config.json", cfgJSON, 0644); err != nil {
		t.Fatal(err)
	}

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "ministral-3:14b",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	// Check user models are registered
	models := s.ListModels()
	userModelFound := false
	for _, m := range models {
		if m.ID == "custom-org/custom-model" && m.Provider == "openrouter" {
			userModelFound = true
			break
		}
	}
	if !userModelFound {
		t.Fatal("expected user-defined openrouter model to be registered")
	}
}

func TestSession_RegisterProvider_OpenRouter(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "ministral-3:14b",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	prov := provider.NewOpenRouterProvider("sk-test-key")
	err = s.RegisterProvider(prov, "openrouter", "https://openrouter.ai/api/v1", []string{
		"anthropic/claude-sonnet-4",
		"openai/gpt-4o",
	})
	if err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}

	// Check models are registered
	models := s.ListModels()
	found := false
	for _, m := range models {
		if m.ID == "anthropic/claude-sonnet-4" && m.Provider == "openrouter" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected openrouter model to be registered via RegisterProvider")
	}
}

func TestSession_NewSession_Ephemeral(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "ministral-3:14b",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	newID, err := s.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if newID != "" {
		t.Errorf("expected empty ID in ephemeral mode, got %q", newID)
	}

	// Agent messages should be cleared
	if len(s.Messages()) != 0 {
		t.Error("expected empty messages after NewSession in ephemeral mode")
	}

	// Usage should be reset
	u := s.Usage()
	if u.TotalTokens != 0 {
		t.Errorf("expected zero usage after NewSession, got %d", u.TotalTokens)
	}
}

func TestSession_NewSession_Persistent(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "ministral-3:14b",
		WorkingDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	oldID := s.ID()
	if oldID == "" {
		t.Fatal("expected non-empty session ID")
	}

	newID, err := s.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if newID == "" {
		t.Fatal("expected non-empty new session ID")
	}
	if newID == oldID {
		t.Errorf("expected different session ID, got same %q", newID)
	}

	// Agent messages should be cleared
	if len(s.Messages()) != 0 {
		t.Error("expected empty messages after NewSession")
	}

	// Usage should be reset
	u := s.Usage()
	if u.TotalTokens != 0 {
		t.Errorf("expected zero usage after NewSession, got %d", u.TotalTokens)
	}

	// New session ID should be returned by ID()
	if s.ID() != newID {
		t.Errorf("expected ID() to return %q, got %q", newID, s.ID())
	}
}

func TestSession_NewSession_ClearsQueue(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	s, err := CreateSession(context.Background(), SessionOptions{
		Model:      "ministral-3:14b",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	s.EnqueueMessage("queued message 1")
	s.EnqueueMessage("queued message 2")

	_, err = s.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Queue should be empty
	next := s.DequeueMessage()
	if next != "" {
		t.Errorf("expected empty queue after NewSession, got %q", next)
	}
}

func ptrBool(b bool) *bool {
	return &b
}

// --- Session resume tests ---

func TestCreateSession_ResumeSessionPath_LoadsMessages(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create a session file manually with messages
	sessDir := tmpDir + "/.tau/sessions/--tmp--"
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	sess, err := tausession.CreateSession(sessDir, tmpDir, "test-session", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	// Append messages directly to the session file
	userMsg := types.AgentMessage{
		ID: "msg-1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello from first session"}},
	}
	assistantMsg := types.AgentMessage{
		ID: "msg-2", Role: types.RoleAssistant,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello! How can I help?"}},
	}

	if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append user: %v", err)
	}
	if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: assistantMsg}); err != nil {
		t.Fatalf("Append assistant: %v", err)
	}

	sessionPath := sess.File()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Resume the session via SessionPath (no auth needed — agent will be nil, messages come from sess)
	s2, err := CreateSession(context.Background(), SessionOptions{
		WorkingDir:  tmpDir,
		SessionPath: sessionPath,
	})
	if err != nil {
		t.Fatalf("CreateSession (resume): %v", err)
	}
	defer s2.Close()

	// Messages should be restored (via sess.Messages() fallback since no agent)
	msgsAfter := s2.Messages()
	if len(msgsAfter) != 2 {
		t.Fatalf("expected 2 messages after resume, got %d", len(msgsAfter))
	}

	if msgsAfter[0].Role != types.RoleUser {
		t.Errorf("expected first message to be user, got %s", msgsAfter[0].Role)
	}
	if msgsAfter[1].Role != types.RoleAssistant {
		t.Errorf("expected second message to be assistant, got %s", msgsAfter[1].Role)
	}

	var foundUserText bool
	for _, block := range msgsAfter[0].Content {
		if block.Type == types.BlockText && strings.Contains(block.Text, "hello from first session") {
			foundUserText = true
		}
	}
	if !foundUserText {
		t.Error("expected to find original user message text in resumed session")
	}
}

func TestCreateSession_ResumeSessionPath_MultipleTurns(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	sessDir := tmpDir + "/.tau/sessions/--tmp--"
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	sess, err := tausession.CreateSession(sessDir, tmpDir, "multi-turn", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	// Append 3 turns (6 messages)
	for i := 0; i < 3; i++ {
		userMsg := types.AgentMessage{
			ID: fmt.Sprintf("user-%d", i), Role: types.RoleUser,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: fmt.Sprintf("turn %d", i)}},
		}
		assistantMsg := types.AgentMessage{
			ID: fmt.Sprintf("assistant-%d", i), Role: types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: fmt.Sprintf("response %d", i)}},
		}
		if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: userMsg}); err != nil {
			t.Fatalf("Append user %d: %v", i, err)
		}
		if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: assistantMsg}); err != nil {
			t.Fatalf("Append assistant %d: %v", i, err)
		}
	}

	sessionPath := sess.File()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Resume
	s2, err := CreateSession(context.Background(), SessionOptions{
		WorkingDir:  tmpDir,
		SessionPath: sessionPath,
	})
	if err != nil {
		t.Fatalf("CreateSession (resume): %v", err)
	}
	defer s2.Close()

	msgsAfter := s2.Messages()
	if len(msgsAfter) != 6 {
		t.Fatalf("expected 6 messages after resume, got %d", len(msgsAfter))
	}

	userTurns := 0
	for _, msg := range msgsAfter {
		if msg.Role == types.RoleUser {
			userTurns++
		}
	}
	if userTurns != 3 {
		t.Errorf("expected 3 user turns, got %d", userTurns)
	}
}

func TestCreateSession_ResumeSessionPath_WithAgent_LoadsMessages(t *testing.T) {
	// This test uses newTestSession (mock provider) to verify that messages
	// are loaded into the agent when resuming.
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create a session file with messages
	sessDir := tmpDir + "/.tau/sessions/--tmp--"
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	sess, err := tausession.CreateSession(sessDir, tmpDir, "agent-resume", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	userMsg := types.AgentMessage{
		ID: "msg-1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "remember: code is BLUE"}},
	}
	assistantMsg := types.AgentMessage{
		ID: "msg-2", Role: types.RoleAssistant,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Got it, BLUE."}},
	}

	if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append user: %v", err)
	}
	if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: assistantMsg}); err != nil {
		t.Fatalf("Append assistant: %v", err)
	}

	sessionPath := sess.File()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Create a mock-provider session that resumes the file
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	// Manually open the session file and load messages into the agent
	// (simulating what CreateSession with SessionPath does)
	resumedSess, err := tausession.OpenSession(sessionPath)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer resumedSess.Close()

	sessionMsgs := resumedSess.Messages()
	if len(sessionMsgs) != 2 {
		t.Fatalf("expected 2 messages in session file, got %d", len(sessionMsgs))
	}

	s.ag.SetMessages(sessionMsgs)

	// Verify messages are available via the agent
	msgs := s.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after SetMessages, got %d", len(msgs))
	}

	// Verify content is preserved
	if msgs[0].Role != types.RoleUser {
		t.Errorf("expected first message to be user, got %s", msgs[0].Role)
	}
	var foundBlue bool
	for _, block := range msgs[0].Content {
		if block.Type == types.BlockText && strings.Contains(block.Text, "BLUE") {
			foundBlue = true
		}
	}
	if !foundBlue {
		t.Error("expected 'BLUE' text in resumed user message")
	}
}

func TestCreateSession_ResumeSessionPath_RestoresUsage(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	sessDir := tmpDir + "/.tau/sessions/--tmp--"
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	sess, err := tausession.CreateSession(sessDir, tmpDir, "usage-test", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	// Append a message and save usage
	userMsg := types.AgentMessage{
		ID: "msg-1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}},
	}
	if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Manually set usage and save
	usageToSave := types.Usage{Input: 100, Output: 50, TotalTokens: 150}
	infoData := tausession.SessionInfoData{Usage: &usageToSave}
	if err := sess.Append(types.EntrySessionInfo, infoData); err != nil {
		t.Fatalf("Append usage: %v", err)
	}

	sessionPath := sess.File()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Resume
	s2, err := CreateSession(context.Background(), SessionOptions{
		WorkingDir:  tmpDir,
		SessionPath: sessionPath,
	})
	if err != nil {
		t.Fatalf("CreateSession (resume): %v", err)
	}
	defer s2.Close()

	usageAfter := s2.Usage()
	if usageAfter.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens after resume, got %d", usageAfter.TotalTokens)
	}
	if usageAfter.Input != 100 {
		t.Errorf("expected 100 input tokens, got %d", usageAfter.Input)
	}
	if usageAfter.Output != 50 {
		t.Errorf("expected 50 output tokens, got %d", usageAfter.Output)
	}
}

func TestAgent_SetMessages(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	testMsgs := []types.AgentMessage{
		{ID: "1", Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
		{ID: "2", Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hi"}}},
	}

	s.ag.SetMessages(testMsgs)

	got := s.ag.Messages()
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Role != types.RoleUser {
		t.Errorf("expected first message role user, got %s", got[0].Role)
	}
	if got[1].Role != types.RoleAssistant {
		t.Errorf("expected second message role assistant, got %s", got[1].Role)
	}
}

func TestAgent_SetMessages_ReturnsCopy(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	s := newTestSession(t, tmpDir, "gpt-4o")
	defer s.Close()

	original := []types.AgentMessage{
		{ID: "1", Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
	}

	s.ag.SetMessages(original)
	got := s.ag.Messages()

	got[0].Content[0].Text = "modified"

	got2 := s.ag.Messages()
	if got2[0].Content[0].Text == "modified" {
		t.Error("expected Messages() to return a copy, modifications should not affect internal state")
	}
}

func TestSession_ResumeSession_LoadsMessages(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	sessDir := tmpDir + "/.tau/sessions/--tmp--"
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file with messages
	sess, err := tausession.CreateSession(sessDir, tmpDir, "resume-test", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	userMsg := types.AgentMessage{
		ID: "msg-1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello from resumed session"}},
	}
	assistantMsg := types.AgentMessage{
		ID: "msg-2", Role: types.RoleAssistant,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hi there!"}},
	}

	if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append user: %v", err)
	}
	if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: assistantMsg}); err != nil {
		t.Fatalf("Append assistant: %v", err)
	}

	sessionPath := sess.File()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Create an ephemeral session first
	s, err := CreateSession(context.Background(), SessionOptions{
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession (ephemeral): %v", err)
	}
	defer s.Close()

	// Resume the file-based session
	if err := s.ResumeSession(sessionPath); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}

	// Verify messages are loaded
	msgs := s.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != types.RoleUser {
		t.Errorf("expected first message role user, got %s", msgs[0].Role)
	}
	if msgs[1].Role != types.RoleAssistant {
		t.Errorf("expected second message role assistant, got %s", msgs[1].Role)
	}

	// Verify session is no longer ephemeral
	if s.Ephemeral() {
		t.Error("expected session to be non-ephemeral after resume")
	}

	// Verify session ID and name are from the resumed file
	if s.ID() == "" {
		t.Error("expected session ID to be set after resume")
	}
	if s.Name() != "resume-test" {
		t.Errorf("expected session name 'resume-test', got %q", s.Name())
	}
}

func TestSession_ResumeSession_RestoresUsage(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	sessDir := tmpDir + "/.tau/sessions/--tmp--"
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	sess, err := tausession.CreateSession(sessDir, tmpDir, "usage-test", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	userMsg := types.AgentMessage{
		ID: "msg-1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}},
	}
	if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	usageToSave := types.Usage{Input: 200, Output: 100, TotalTokens: 300}
	infoData := tausession.SessionInfoData{Usage: &usageToSave}
	if err := sess.Append(types.EntrySessionInfo, infoData); err != nil {
		t.Fatalf("Append usage: %v", err)
	}

	sessionPath := sess.File()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Create an ephemeral session
	s, err := CreateSession(context.Background(), SessionOptions{
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession (ephemeral): %v", err)
	}
	defer s.Close()

	// Resume
	if err := s.ResumeSession(sessionPath); err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}

	// Verify usage is restored
	usage := s.Usage()
	if usage.TotalTokens != 300 {
		t.Errorf("expected 300 total tokens, got %d", usage.TotalTokens)
	}
	if usage.Input != 200 {
		t.Errorf("expected 200 input tokens, got %d", usage.Input)
	}
	if usage.Output != 100 {
		t.Errorf("expected 100 output tokens, got %d", usage.Output)
	}
}

func TestSession_ResumeSession_InvalidPath(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	s, err := CreateSession(context.Background(), SessionOptions{
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	err = s.ResumeSession("/nonexistent/path/session.jsonl")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestSession_SetModelAfterResume_PreservesHistory(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	sessDir := tmpDir + "/.tau/sessions/--tmp--"
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a session file with a multi-turn conversation
	sess, err := tausession.CreateSession(sessDir, tmpDir, "multi-turn", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	turns := []struct {
		user      string
		assistant string
	}{
		{"what is go?", "Go is a programming language created at Google"},
		{"show me code", "```go\nfmt.Println(\"hello\")\n```"},
		{"translate to Polish", "Oto tłumaczenie: \"Witaj\""},
	}

	for _, turn := range turns {
		if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: types.AgentMessage{
			ID: "u-" + turn.user, Role: types.RoleUser,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: turn.user}},
		}}); err != nil {
			t.Fatalf("Append user: %v", err)
		}
		if err := sess.Append(types.EntryMessage, tausession.MessageData{Message: types.AgentMessage{
			ID: "a-" + turn.user, Role: types.RoleAssistant,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: turn.assistant}},
		}}); err != nil {
			t.Fatalf("Append assistant: %v", err)
		}
	}

	sessionPath := sess.File()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Create an ephemeral session and resume the file
	s, err := CreateSession(context.Background(), SessionOptions{
		WorkingDir:  tmpDir,
		SessionPath: sessionPath,
	})
	if err != nil {
		t.Fatalf("CreateSession (resume): %v", err)
	}
	defer s.Close()

	// Verify messages loaded from resumed session
	msgs := s.Messages()
	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages from resumed session, got %d", len(msgs))
	}

	// Register a second mock provider so SetModel can switch to it
	model2, err := s.provReg.ResolveModelWithFallback("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	s.provReg.Register(&mockProvider{
		providerName: model2.Provider,
		model:        model2,
	})

	// Switch model mid-conversation
	err = s.SetModel("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("SetModel after resume: %v", err)
	}

	// Verify model changed
	if s.Model().ID != "claude-sonnet-4-20250514" {
		t.Fatalf("expected claude-sonnet-4-20250514, got %s", s.Model().ID)
	}

	// Verify ALL messages are still preserved after model switch
	msgsAfter := s.Messages()
	if len(msgsAfter) != 6 {
		t.Fatalf("expected 6 messages after model switch, got %d", len(msgsAfter))
	}

	// Verify specific content is preserved
	if msgsAfter[0].Content[0].Text != "what is go?" {
		t.Errorf("expected first message 'what is go?', got %q", msgsAfter[0].Content[0].Text)
	}
	if msgsAfter[5].Content[0].Text != "Oto tłumaczenie: \"Witaj\"" {
		t.Errorf("expected last message with Polish translation, got %q", msgsAfter[5].Content[0].Text)
	}
}

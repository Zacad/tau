package session

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/types"
)

func TestCreateSession(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s, err := CreateSession(sessionDir, "/test/cwd", "test-session", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	if s.ID() == "" {
		t.Error("session ID should not be empty")
	}
	if s.Name() != "test-session" {
		t.Errorf("name mismatch: got %s, want test-session", s.Name())
	}
	if s.Cwd() != "/test/cwd" {
		t.Errorf("cwd mismatch: got %s, want /test/cwd", s.Cwd())
	}
	if s.File() == "" {
		t.Error("file path should not be empty")
	}

	// Verify file exists
	if _, err := os.Stat(s.File()); err != nil {
		t.Fatalf("session file not created: %v", err)
	}

	// Verify header is readable
	h, entries, err := ReadEntries(s.File())
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if h.ID != s.ID() {
		t.Errorf("header ID mismatch")
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for new session, got %d", len(entries))
	}
}

func TestCreateSession_WithExplicitID(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s, err := CreateSession(sessionDir, "/test", "test", "my-custom-id")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	if s.ID() != "my-custom-id" {
		t.Errorf("ID mismatch: got %s, want my-custom-id", s.ID())
	}
}

func TestSession_AppendMessage(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s, err := CreateSession(sessionDir, "/test", "test", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	msg := types.AgentMessage{
		ID:        "msg-001",
		Role:      types.RoleUser,
		Content:   []types.ContentBlock{{Type: types.BlockText, Text: "Hello world"}},
		Timestamp: time.Now(),
	}

	if err := s.Append(types.EntryMessage, MessageData{Message: msg}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Verify in-memory
	msgs := s.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].ID != "msg-001" {
		t.Errorf("message ID mismatch: got %s, want msg-001", msgs[0].ID)
	}

	// Verify on disk
	s.Close()
	h, entries, err := ReadEntries(s.File())
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if h.ID != s.ID() {
		t.Errorf("header ID mismatch")
	}
	// 1 entry (the message) + header line
	if len(entries) != 1 {
		t.Errorf("expected 1 entry on disk, got %d", len(entries))
	}
}

func TestSession_AppendMultiple(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s, err := CreateSession(sessionDir, "/test", "test", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	// Append a user message
	userMsg := types.AgentMessage{
		ID:   "user-1",
		Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}},
	}
	if err := s.Append(types.EntryMessage, MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append user: %v", err)
	}

	// Append model change
	if err := s.Append(types.EntryModelChange, ModelChangeData{ModelID: "gpt-4o"}); err != nil {
		t.Fatalf("Append model_change: %v", err)
	}

	// Append thinking level change
	if err := s.Append(types.EntryThinkingLevelChange, ThinkingLevelChangeData{ModelID: "gpt-4o", Level: types.ThinkingMedium}); err != nil {
		t.Fatalf("Append thinking_change: %v", err)
	}

	// Verify in-memory state
	if s.CurrentModel() != "gpt-4o" {
		t.Errorf("model mismatch: got %s, want gpt-4o", s.CurrentModel())
	}
	if s.CurrentThinkingLevel() != types.ThinkingMedium {
		t.Errorf("thinking level mismatch: got %s, want %s",
			s.CurrentThinkingLevel(), types.ThinkingMedium)
	}

	msgs := s.Messages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
}

func TestSession_Resume(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	// Create session and add messages
	s1, err := CreateSession(sessionDir, "/test", "test", "resume-test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	userMsg := types.AgentMessage{
		ID:   "user-1",
		Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}},
	}
	if err := s1.Append(types.EntryMessage, MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	assistantMsg := types.AgentMessage{
		ID:   "assistant-1",
		Role: types.RoleAssistant,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hi there!"}},
		API:   "openai-responses",
		Model: "gpt-4o",
	}
	if err := s1.Append(types.EntryMessage, MessageData{Message: assistantMsg}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	filePath := s1.File()
	s1.Close()

	// Reopen session
	s2, err := OpenSession(filePath)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer s2.Close()

	if s2.ID() != "resume-test" {
		t.Errorf("ID mismatch after resume: got %s, want resume-test", s2.ID())
	}

	msgs := s2.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after resume, got %d", len(msgs))
	}
	if msgs[0].ID != "user-1" {
		t.Errorf("first message ID mismatch: got %s, want user-1", msgs[0].ID)
	}
	if msgs[1].ID != "assistant-1" {
		t.Errorf("second message ID mismatch: got %s, want assistant-1", msgs[1].ID)
	}

	// Verify we can append after resume
	userMsg2 := types.AgentMessage{
		ID:   "user-2",
		Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Again"}},
	}
	if err := s2.Append(types.EntryMessage, MessageData{Message: userMsg2}); err != nil {
		t.Fatalf("Append after resume: %v", err)
	}

	msgs = s2.Messages()
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages after append, got %d", len(msgs))
	}
}

func TestSession_ResumeWithModelChange(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s1, err := CreateSession(sessionDir, "/test", "test", "model-test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := s1.Append(types.EntryModelChange, ModelChangeData{ModelID: "claude-sonnet-4"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s1.Append(types.EntryThinkingLevelChange, ThinkingLevelChangeData{ModelID: "claude-sonnet-4", Level: types.ThinkingHigh}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	filePath := s1.File()
	s1.Close()

	s2, err := OpenSession(filePath)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer s2.Close()

	if s2.CurrentModel() != "claude-sonnet-4" {
		t.Errorf("model mismatch after resume: got %s, want claude-sonnet-4", s2.CurrentModel())
	}
	if s2.CurrentThinkingLevel() != types.ThinkingHigh {
		t.Errorf("thinking level mismatch after resume: got %s", s2.CurrentThinkingLevel())
	}
}

func TestSession_UsageTracking(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s, err := CreateSession(sessionDir, "/test", "test", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	// Simulate first turn usage
	usage1 := types.Usage{
		Input: 100, Output: 50, TotalTokens: 150,
		Cost: types.CostDollars{Input: 0.001, Output: 0.0005, Total: 0.0015},
	}
	msg := types.AgentMessage{ID: "msg-1", Role: types.RoleAssistant}
	if err := s.AppendWithUsage(types.EntryMessage, MessageData{Message: msg}, &usage1); err != nil {
		t.Fatalf("AppendWithUsage: %v", err)
	}

	// Simulate second turn usage
	usage2 := types.Usage{
		Input: 200, Output: 100, TotalTokens: 300,
		Cost: types.CostDollars{Input: 0.002, Output: 0.001, Total: 0.003},
	}
	msg2 := types.AgentMessage{ID: "msg-2", Role: types.RoleAssistant}
	if err := s.AppendWithUsage(types.EntryMessage, MessageData{Message: msg2}, &usage2); err != nil {
		t.Fatalf("AppendWithUsage: %v", err)
	}

	total := s.Usage()
	if total.Input != 300 {
		t.Errorf("total input mismatch: got %d, want 300", total.Input)
	}
	if total.Output != 150 {
		t.Errorf("total output mismatch: got %d, want 150", total.Output)
	}
	if total.TotalTokens != 450 {
		t.Errorf("total tokens mismatch: got %d, want 450", total.TotalTokens)
	}
	if math.Abs(total.Cost.Total-0.0045) > 1e-9 {
		t.Errorf("total cost mismatch: got %v, want 0.0045", total.Cost.Total)
	}
}

func TestSession_SaveUsage_Resume(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s1, err := CreateSession(sessionDir, "/test", "test", "usage-test")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	usage := types.Usage{
		Input: 500, Output: 250, TotalTokens: 750,
		Cost: types.CostDollars{Total: 0.0075},
	}

	// Manually set usage (simulating accumulated usage)
	s1.mu.Lock()
	s1.usage = usage
	s1.mu.Unlock()

	// Save usage to disk
	if err := s1.SaveUsage(); err != nil {
		t.Fatalf("SaveUsage: %v", err)
	}

	filePath := s1.File()
	s1.Close()

	// Resume and verify usage is restored
	s2, err := OpenSession(filePath)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer s2.Close()

	resumed := s2.Usage()
	if resumed.Input != 500 {
		t.Errorf("resumed input mismatch: got %d, want 500", resumed.Input)
	}
	if resumed.TotalTokens != 750 {
		t.Errorf("resumed total tokens mismatch: got %d, want 750", resumed.TotalTokens)
	}
	if resumed.Cost.Total != 0.0075 {
		t.Errorf("resumed cost mismatch: got %f, want 0.0075", resumed.Cost.Total)
	}
}

func TestSession_Delete(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s, err := CreateSession(sessionDir, "/test", "test", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	filePath := s.File()
	if err := s.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("session file should be deleted")
	}
}

func TestSession_SetName(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s, err := CreateSession(sessionDir, "/test", "original-name", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	if err := s.SetName("new-name"); err != nil {
		t.Fatalf("SetName: %v", err)
	}

	if s.Name() != "new-name" {
		t.Errorf("name mismatch: got %s, want new-name", s.Name())
	}
}

func TestSession_SetModel(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s, err := CreateSession(sessionDir, "/test", "test", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	if err := s.SetModel("gpt-4o", "openai"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	if s.CurrentModel() != "gpt-4o" {
		t.Errorf("model mismatch: got %s, want gpt-4o", s.CurrentModel())
	}
	if s.CurrentProvider() != "openai" {
		t.Errorf("provider mismatch: got %s, want openai", s.CurrentProvider())
	}
}

func TestSession_SetThinkingLevel(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	s, err := CreateSession(sessionDir, "/test", "test", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	if err := s.SetThinkingLevel("test-model", types.ThinkingHigh); err != nil {
		t.Fatalf("SetThinkingLevel: %v", err)
	}

	if s.CurrentThinkingLevel() != types.ThinkingHigh {
		t.Errorf("thinking level mismatch: got %s", s.CurrentThinkingLevel())
	}
}

func TestSession_FullLifecycle(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	// Phase 1: Create and use session
	s1, err := CreateSession(sessionDir, "/test", "lifecycle-test", "lc-001")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	userMsg := types.AgentMessage{ID: "u1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hello"}}}
	if err := s1.Append(types.EntryMessage, MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	assistantMsg := types.AgentMessage{ID: "a1", Role: types.RoleAssistant,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hi!"}},
		API: "openai-responses", Model: "gpt-4o"}
	usage1 := &types.Usage{
		Input: 10, Output: 5, TotalTokens: 15,
		Cost: types.CostDollars{Total: 0.0001},
	}
	if err := s1.AppendWithUsage(types.EntryMessage, MessageData{Message: assistantMsg}, usage1); err != nil {
		t.Fatalf("AppendWithUsage: %v", err)
	}

	if err := s1.SaveUsage(); err != nil {
		t.Fatalf("SaveUsage: %v", err)
	}

	filePath := s1.File()
	s1.Close()

	// Phase 2: Resume session
	s2, err := OpenSession(filePath)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}

	msgs := s2.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after resume, got %d", len(msgs))
	}

	if s2.Usage().Input != 10 {
		t.Errorf("usage not restored: got input %d, want 10", s2.Usage().Input)
	}

	// Phase 3: Continue session
	userMsg2 := types.AgentMessage{ID: "u2", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Continue"}}}
	if err := s2.Append(types.EntryMessage, MessageData{Message: userMsg2}); err != nil {
		t.Fatalf("Append after resume: %v", err)
	}

	msgs = s2.Messages()
	if len(msgs) != 3 {
		t.Errorf("expected 3 messages after continue, got %d", len(msgs))
	}

	s2.Close()

	// Phase 4: Verify file integrity
	h, entries, err := ReadEntries(filePath)
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if h.ID != "lc-001" {
		t.Errorf("header ID mismatch")
	}
	// header + 2 messages + session_info (save usage) + 1 message = 4 entries
	if len(entries) != 4 {
		t.Errorf("expected 4 entries on disk, got %d", len(entries))
	}
}

func TestSession_OpenNonExistent(t *testing.T) {
	_, err := OpenSession("/nonexistent/session.jsonl")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestSession_ModelRestoreWithProvider(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	// Phase 1: Create session and set model with provider
	s1, err := CreateSession(sessionDir, "/test", "test", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := s1.SetModel("claude-sonnet-4-20250514", "anthropic"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}

	filePath := s1.File()
	s1.Close()

	// Phase 2: Reopen and verify model + provider restored
	s2, err := OpenSession(filePath)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer s2.Close()

	if s2.CurrentModel() != "claude-sonnet-4-20250514" {
		t.Errorf("model not restored: got %q, want claude-sonnet-4-20250514", s2.CurrentModel())
	}
	if s2.CurrentProvider() != "anthropic" {
		t.Errorf("provider not restored: got %q, want anthropic", s2.CurrentProvider())
	}
}

func TestSession_ModelRestoreBackwardCompat(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions")

	// Create session and manually write old-format model change (no provider)
	s1, err := CreateSession(sessionDir, "/test", "test", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Write old-format entry directly (simulating old session files)
	oldData := ModelChangeData{ModelID: "gpt-4o", Provider: ""}
	if err := s1.Append(types.EntryModelChange, oldData); err != nil {
		t.Fatalf("Append old format: %v", err)
	}

	filePath := s1.File()
	s1.Close()

	// Reopen and verify model is restored, provider is empty
	s2, err := OpenSession(filePath)
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	defer s2.Close()

	if s2.CurrentModel() != "gpt-4o" {
		t.Errorf("model not restored: got %q, want gpt-4o", s2.CurrentModel())
	}
	if s2.CurrentProvider() != "" {
		t.Errorf("provider should be empty for old format, got %q", s2.CurrentProvider())
	}
}

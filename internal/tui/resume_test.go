package tui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/sdk"
	"github.com/adam/tau/internal/session"
	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/types"
)

func TestExtractLastUserPrompt(t *testing.T) {
	tests := []struct {
		name     string
		entries  []types.SessionEntry
		wantText string
	}{
		{
			name:     "empty entries",
			entries:  nil,
			wantText: "",
		},
		{
			name: "no user messages",
			entries: []types.SessionEntry{
				{Type: types.EntryMessage, Data: marshalMessage(t, types.AgentMessage{
					Role: types.RoleAssistant,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}},
				})},
			},
			wantText: "",
		},
		{
			name: "single user message",
			entries: []types.SessionEntry{
				{Type: types.EntryMessage, Data: marshalMessage(t, types.AgentMessage{
					Role: types.RoleUser,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "what is go?"}},
				})},
			},
			wantText: "what is go?",
		},
		{
			name: "returns last user message",
			entries: []types.SessionEntry{
				{Type: types.EntryMessage, Data: marshalMessage(t, types.AgentMessage{
					Role: types.RoleUser,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "first prompt"}},
				})},
				{Type: types.EntryMessage, Data: marshalMessage(t, types.AgentMessage{
					Role: types.RoleAssistant,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "answer"}},
				})},
				{Type: types.EntryMessage, Data: marshalMessage(t, types.AgentMessage{
					Role: types.RoleUser,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "follow up question"}},
				})},
			},
			wantText: "follow up question",
		},
		{
			name: "truncates long messages",
			entries: []types.SessionEntry{
				{Type: types.EntryMessage, Data: marshalMessage(t, types.AgentMessage{
					Role: types.RoleUser,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "this is a very long prompt that should be truncated because it exceeds eighty characters in total length"}},
				})},
			},
			wantText: "this is a very long prompt that should be truncated because it exceeds eighty ch…",
		},
		{
			name: "skips non-message entries",
			entries: []types.SessionEntry{
				{Type: types.EntryModelChange, Data: []byte(`{"model_id":"gpt-4"}`)},
				{Type: types.EntryMessage, Data: marshalMessage(t, types.AgentMessage{
					Role: types.RoleUser,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}},
				})},
				{Type: types.EntryCompaction, Data: []byte(`{"summary":"compacted"}`)},
			},
			wantText: "hello",
		},
		{
			name: "extracts text from multi-block message",
			entries: []types.SessionEntry{
				{Type: types.EntryMessage, Data: marshalMessage(t, types.AgentMessage{
					Role: types.RoleUser,
					Content: []types.ContentBlock{
						{Type: types.BlockText, Text: "part one "},
						{Type: types.BlockText, Text: "part two"},
					},
				})},
			},
			wantText: "part one part two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractLastUserPrompt(tt.entries)
			if got != tt.wantText {
				t.Errorf("extractLastUserPrompt() = %q, want %q", got, tt.wantText)
			}
		})
	}
}

func TestBuildBlocksFromSession(t *testing.T) {
	tests := []struct {
		name          string
		messages      []types.AgentMessage
		wantBlocks    int
		wantTurnCount int
		checkBlocks   func(t *testing.T, blocks []messageBlock)
	}{
		{
			name:          "empty messages",
			messages:      nil,
			wantBlocks:    0,
			wantTurnCount: 0,
		},
		{
			name: "single user message",
			messages: []types.AgentMessage{
				{
					Role: types.RoleUser,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}},
				},
			},
			wantBlocks:    1,
			wantTurnCount: 0,
			checkBlocks: func(t *testing.T, blocks []messageBlock) {
				if blocks[0].kind != blockUserMessage {
					t.Errorf("expected user message block, got %d", blocks[0].kind)
				}
				if blocks[0].text != "hello" {
					t.Errorf("expected text 'hello', got %q", blocks[0].text)
				}
			},
		},
		{
			name: "user + assistant turn",
			messages: []types.AgentMessage{
				{
					Role: types.RoleUser,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "what is 2+2?"}},
				},
				{
					Role: types.RoleAssistant,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "4"}},
				},
			},
			wantBlocks:    2,
			wantTurnCount: 1,
			checkBlocks: func(t *testing.T, blocks []messageBlock) {
				if blocks[0].kind != blockUserMessage {
					t.Errorf("block 0: expected user message, got %d", blocks[0].kind)
				}
				if blocks[1].kind != blockAssistantText {
					t.Errorf("block 1: expected assistant text, got %d", blocks[1].kind)
				}
				if !blocks[1].isFinalized {
					t.Error("block 1: expected finalized")
				}
			},
		},
		{
			name: "assistant with thinking",
			messages: []types.AgentMessage{
				{
					Role: types.RoleUser,
					Content: []types.ContentBlock{{Type: types.BlockText, Text: "solve this"}},
				},
				{
					Role: types.RoleAssistant,
					Content: []types.ContentBlock{
						{Type: types.BlockThinking, Text: "let me think..."},
						{Type: types.BlockText, Text: "the answer is 42"},
					},
				},
			},
			wantBlocks:    3,
			wantTurnCount: 1,
			checkBlocks: func(t *testing.T, blocks []messageBlock) {
				if blocks[1].kind != blockThinking {
					t.Errorf("block 1: expected thinking, got %d", blocks[1].kind)
				}
				if blocks[2].kind != blockAssistantText {
					t.Errorf("block 2: expected assistant text, got %d", blocks[2].kind)
				}
			},
		},
		{
			name: "multiple turns",
			messages: []types.AgentMessage{
				{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "first"}}},
				{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "response 1"}}},
				{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "second"}}},
				{Role: types.RoleAssistant, Content: []types.ContentBlock{{Type: types.BlockText, Text: "response 2"}}},
			},
			wantBlocks:    4,
			wantTurnCount: 2,
		},
		{
			name: "consolidates consecutive thinking blocks",
			messages: []types.AgentMessage{
				{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "think about this"}}},
				{Role: types.RoleAssistant, Content: []types.ContentBlock{
					{Type: types.BlockThinking, Text: "let "},
					{Type: types.BlockThinking, Text: "me "},
					{Type: types.BlockThinking, Text: "think..."},
					{Type: types.BlockText, Text: "done"},
				}},
			},
			wantBlocks:    3,
			wantTurnCount: 1,
			checkBlocks: func(t *testing.T, blocks []messageBlock) {
				if blocks[1].kind != blockThinking {
					t.Fatalf("block 1: expected thinking, got %d", blocks[1].kind)
				}
				if blocks[1].text != "let me think..." {
					t.Errorf("block 1: expected 'let me think...', got %q", blocks[1].text)
				}
			},
		},
		{
			name: "consolidates consecutive text blocks",
			messages: []types.AgentMessage{
				{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}}},
				{Role: types.RoleAssistant, Content: []types.ContentBlock{
					{Type: types.BlockText, Text: "part "},
					{Type: types.BlockText, Text: "one "},
					{Type: types.BlockText, Text: "response"},
				}},
			},
			wantBlocks:    2,
			wantTurnCount: 1,
			checkBlocks: func(t *testing.T, blocks []messageBlock) {
				if blocks[1].kind != blockAssistantText {
					t.Fatalf("block 1: expected assistant text, got %d", blocks[1].kind)
				}
				if blocks[1].text != "part one response" {
					t.Errorf("block 1: expected 'part one response', got %q", blocks[1].text)
				}
			},
		},
		{
			name: "thinking then text then thinking stays separate",
			messages: []types.AgentMessage{
				{Role: types.RoleUser, Content: []types.ContentBlock{{Type: types.BlockText, Text: "test"}}},
				{Role: types.RoleAssistant, Content: []types.ContentBlock{
					{Type: types.BlockThinking, Text: "first thought"},
					{Type: types.BlockText, Text: "response"},
					{Type: types.BlockThinking, Text: "afterthought"},
				}},
			},
			wantBlocks:    4,
			wantTurnCount: 1,
			checkBlocks: func(t *testing.T, blocks []messageBlock) {
				if blocks[1].kind != blockThinking || blocks[1].text != "first thought" {
					t.Errorf("block 1: expected 'first thought' thinking, got kind=%d text=%q", blocks[1].kind, blocks[1].text)
				}
				if blocks[2].kind != blockAssistantText || blocks[2].text != "response" {
					t.Errorf("block 2: expected 'response' text, got kind=%d text=%q", blocks[2].kind, blocks[2].text)
				}
				if blocks[3].kind != blockThinking || blocks[3].text != "afterthought" {
					t.Errorf("block 3: expected 'afterthought' thinking, got kind=%d text=%q", blocks[3].kind, blocks[3].text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks, turnCount := buildBlocksFromSession(tt.messages)
			if len(blocks) != tt.wantBlocks {
				t.Errorf("got %d blocks, want %d", len(blocks), tt.wantBlocks)
			}
			if turnCount != tt.wantTurnCount {
				t.Errorf("got turnCount %d, want %d", turnCount, tt.wantTurnCount)
			}
			if tt.checkBlocks != nil {
				tt.checkBlocks(t, blocks)
			}
		})
	}
}

func marshalMessage(t *testing.T, msg types.AgentMessage) []byte {
	t.Helper()
	data := struct {
		Message types.AgentMessage `json:"message"`
	}{Message: msg}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	return b
}

func TestHandleResumeComplete_LoadsSessionHistory(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create a session file with messages using the correct sessions dir
	sessDir, err := config.SessionsDir(tmpDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}

	sess, err := session.CreateSession(sessDir, tmpDir, "test-resume", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	// Add messages
	userMsg := types.AgentMessage{
		ID: "msg-1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello from session"}},
	}
	assistantMsg := types.AgentMessage{
		ID: "msg-2", Role: types.RoleAssistant,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Hi there!"}},
	}
	userMsg2 := types.AgentMessage{
		ID: "msg-3", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "follow up"}},
	}
	assistantMsg2 := types.AgentMessage{
		ID: "msg-4", Role: types.RoleAssistant,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "Follow up response"}},
	}

	for _, msg := range []types.AgentMessage{userMsg, assistantMsg, userMsg2, assistantMsg2} {
		if err := sess.Append(types.EntryMessage, session.MessageData{Message: msg}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	sessionPath := sess.File()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Create SDK session by resuming the file
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resumedSession, err := sdk.CreateSession(ctx, sdk.SessionOptions{
		WorkingDir:  tmpDir,
		SessionPath: sessionPath,
	})
	if err != nil {
		t.Fatalf("CreateSession (resume): %v", err)
	}
	defer resumedSession.Close()

	// Verify messages are available
	msgs := resumedSession.Messages()
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(msgs))
	}

	// Create a TUI model and simulate handleResumeComplete
	m := newTestModel()
	m.session = resumedSession
	m.cwd = tmpDir
	m.width = 80
	m.height = 24

	handleResumeComplete(m)

	// Verify blocks are populated
	if len(m.blocks) != 4 {
		t.Fatalf("expected 4 blocks, got %d", len(m.blocks))
	}
	if m.turnCount != 2 {
		t.Errorf("expected 2 turns, got %d", m.turnCount)
	}

	// Verify first block is user message
	if m.blocks[0].kind != blockUserMessage {
		t.Errorf("expected block 0 to be user message, got %d", m.blocks[0].kind)
	}
	if m.blocks[0].text != "hello from session" {
		t.Errorf("expected block 0 text 'hello from session', got %q", m.blocks[0].text)
	}

	// Verify second block is assistant text
	if m.blocks[1].kind != blockAssistantText {
		t.Errorf("expected block 1 to be assistant text, got %d", m.blocks[1].kind)
	}
	if m.blocks[1].text != "Hi there!" {
		t.Errorf("expected block 1 text 'Hi there!', got %q", m.blocks[1].text)
	}
}

func TestScanSessionsForResume_ShowsLastPrompt(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create a session file with messages using the correct sessions dir
	sessDir, err := config.SessionsDir(tmpDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}

	sess, err := session.CreateSession(sessDir, tmpDir, "prompt-test", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	userMsg := types.AgentMessage{
		ID: "msg-1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "this is a test prompt for session"}},
	}
	assistantMsg := types.AgentMessage{
		ID: "msg-2", Role: types.RoleAssistant,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "response"}},
	}

	if err := sess.Append(types.EntryMessage, session.MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append user: %v", err)
	}
	if err := sess.Append(types.EntryMessage, session.MessageData{Message: assistantMsg}); err != nil {
		t.Fatalf("Append assistant: %v", err)
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Scan sessions
	sessions, err := scanSessionsForResume(tmpDir)
	if err != nil {
		t.Fatalf("scanSessionsForResume: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	// Verify last prompt is extracted
	if sessions[0].lastPrompt == "" {
		t.Fatal("expected lastPrompt to be populated")
	}
	if sessions[0].lastPrompt != "this is a test prompt for session" {
		t.Errorf("expected lastPrompt 'this is a test prompt for session', got %q", sessions[0].lastPrompt)
	}
}

func TestResumeSessionTask_SwapsSession(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create a session file with messages using the correct sessions dir
	sessDir, err := config.SessionsDir(tmpDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}

	sess, err := session.CreateSession(sessDir, tmpDir, "swap-test", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	userMsg := types.AgentMessage{
		ID: "msg-1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}},
	}
	if err := sess.Append(types.EntryMessage, session.MessageData{Message: userMsg}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	sessionPath := sess.File()
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Create a model with an initial session
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initialSession, err := sdk.CreateSession(ctx, sdk.SessionOptions{
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession (initial): %v", err)
	}
	defer initialSession.Close()

	m := newTestModel()
	m.session = initialSession
	m.cwd = tmpDir

	// Run resumeSessionTask
	success, msg, err := resumeSessionTask(m, sessionPath)
	if !success {
		t.Fatalf("resumeSessionTask failed: %s", msg)
	}
	if err != nil {
		t.Fatalf("resumeSessionTask error: %v", err)
	}

	// ResumeSession swaps the internal file but keeps the same SDK session object
	if m.session != initialSession {
		t.Fatal("expected session object to be the same (only internal file swapped)")
	}

	// Verify resumed session has messages
	msgs := m.session.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in resumed session, got %d", len(msgs))
	}
	if msgs[0].Role != types.RoleUser {
		t.Errorf("expected user message, got %s", msgs[0].Role)
	}

	// Verify session is no longer ephemeral
	if m.session.Ephemeral() {
		t.Error("expected session to be non-ephemeral after resume")
	}
}

func TestFullResumeFlow_SessionFileToBlocks(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create a session file with multiple turns using the correct sessions dir
	sessDir, err := config.SessionsDir(tmpDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}

	sess, err := session.CreateSession(sessDir, tmpDir, "full-flow", "")
	if err != nil {
		t.Fatalf("CreateSession file: %v", err)
	}

	turns := []struct {
		user      string
		assistant string
	}{
		{"what is go?", "Go is a programming language"},
		{"show me code", "```go\nfmt.Println(\"hello\")\n```"},
		{"thanks", "You're welcome!"},
	}

	for _, turn := range turns {
		if err := sess.Append(types.EntryMessage, session.MessageData{Message: types.AgentMessage{
			ID: "u-" + turn.user, Role: types.RoleUser,
			Content: []types.ContentBlock{{Type: types.BlockText, Text: turn.user}},
		}}); err != nil {
			t.Fatalf("Append user: %v", err)
		}
		if err := sess.Append(types.EntryMessage, session.MessageData{Message: types.AgentMessage{
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

	// Step 1: Scan sessions and verify last prompt
	sessions, err := scanSessionsForResume(tmpDir)
	if err != nil {
		t.Fatalf("scanSessionsForResume: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].lastPrompt != "thanks" {
		t.Errorf("expected lastPrompt 'thanks', got %q", sessions[0].lastPrompt)
	}

	// Step 2: Resume session via SDK
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resumedSession, err := sdk.CreateSession(ctx, sdk.SessionOptions{
		WorkingDir:  tmpDir,
		SessionPath: sessionPath,
	})
	if err != nil {
		t.Fatalf("CreateSession (resume): %v", err)
	}
	defer resumedSession.Close()

	// Step 3: Verify messages
	msgs := resumedSession.Messages()
	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(msgs))
	}

	// Step 4: Build blocks
	blocks, turnCount := buildBlocksFromSession(msgs)
	if len(blocks) != 6 {
		t.Fatalf("expected 6 blocks, got %d", len(blocks))
	}
	if turnCount != 3 {
		t.Errorf("expected 3 turns, got %d", turnCount)
	}

	// Step 5: Verify block content
	expectedTexts := []string{
		"what is go?", "Go is a programming language",
		"show me code", "```go\nfmt.Println(\"hello\")\n```",
		"thanks", "You're welcome!",
	}
	for i, expected := range expectedTexts {
		if blocks[i].text != expected {
			t.Errorf("block %d: expected %q, got %q", i, expected, blocks[i].text)
		}
	}

	// Step 6: Simulate handleResumeComplete
	m := newTestModel()
	m.session = resumedSession
	m.cwd = tmpDir
	m.width = 80
	m.height = 24

	handleResumeComplete(m)

	if len(m.blocks) != 6 {
		t.Fatalf("expected 6 blocks after handleResumeComplete, got %d", len(m.blocks))
	}
	if m.turnCount != 3 {
		t.Errorf("expected 3 turns after handleResumeComplete, got %d", m.turnCount)
	}
}

func TestScanSessionsForResume_SkipsMalformedFiles(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	sessDir, err := config.SessionsDir(tmpDir)
	if err != nil {
		t.Fatalf("SessionsDir: %v", err)
	}

	// Create a valid session
	sess, err := session.CreateSession(sessDir, tmpDir, "valid", "")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := sess.Append(types.EntryMessage, session.MessageData{Message: types.AgentMessage{
		ID: "1", Role: types.RoleUser,
		Content: []types.ContentBlock{{Type: types.BlockText, Text: "hello"}},
	}}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Create a malformed file
	if err := os.WriteFile(filepath.Join(sessDir, "bad.jsonl"), []byte("not valid json\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a non-jsonl file
	if err := os.WriteFile(filepath.Join(sessDir, "readme.txt"), []byte("readme"), 0644); err != nil {
		t.Fatal(err)
	}

	sessions, err := scanSessionsForResume(tmpDir)
	if err != nil {
		t.Fatalf("scanSessionsForResume: %v", err)
	}

	// Should only find the valid session
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adam/tau/internal/types"
)

func TestLoadHistoryFromSessions(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create a session file
	sessionsDir := filepath.Join(tmpDir, ".tau", "sessions", "-tmp-test-")
	os.MkdirAll(sessionsDir, 0755)

	sessionID := "test1234"
	timestamp := time.Now()
	header := types.SessionHeader{
		Type:      "session",
		Version:   1,
		ID:        sessionID,
		Timestamp: timestamp,
		Cwd:       "/tmp/test",
		Name:      "test-session",
	}

	headerData, _ := json.Marshal(header)
	sessionFile := filepath.Join(sessionsDir, timestamp.Format("20060102-150405")+"-"+sessionID+".jsonl")

	f, _ := os.Create(sessionFile)
	f.Write(headerData)
	f.Write([]byte("\n"))

	// Write some entries
	entries := []types.SessionEntry{
		{
			Type: types.EntryMessage,
			Data: json.RawMessage(`{"message":{"id":"m1","role":"user","content":[{"type":"text","text":"hello world"}],"timestamp":"2026-01-01T00:00:00Z"}}`),
		},
		{
			Type: types.EntryMessage,
			Data: json.RawMessage(`{"message":{"id":"m2","role":"assistant","content":[{"type":"text","text":"hi there"}],"timestamp":"2026-01-01T00:00:01Z"}}`),
		},
		{
			Type: types.EntryMessage,
			Data: json.RawMessage(`{"message":{"id":"m3","role":"user","content":[{"type":"text","text":"second prompt"}],"timestamp":"2026-01-01T00:00:02Z"}}`),
		},
	}

	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(data)
		f.Write([]byte("\n"))
	}
	f.Close()

	// Test loading
	result := loadHistoryFromSessions("/tmp/test")

	if len(result) != 2 {
		t.Fatalf("expected 2 prompts, got %d: %v", len(result), result)
	}

	expected := []string{"hello world", "second prompt"}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("prompt[%d]: expected %q, got %q", i, expected[i], result[i])
		}
	}
}

func TestLoadHistoryFromSessions_NoSessions(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	result := loadHistoryFromSessions("/tmp/nonexistent")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestLoadHistoryFromSessions_EmptySession(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create an empty session file (header only)
	sessionsDir := filepath.Join(tmpDir, ".tau", "sessions", "-tmp-test-")
	os.MkdirAll(sessionsDir, 0755)

	header := types.SessionHeader{
		Type:      "session",
		Version:   1,
		ID:        "test1234",
		Timestamp: time.Now(),
		Cwd:       "/tmp/test",
		Name:      "empty-session",
	}

	headerData, _ := json.Marshal(header)
	sessionFile := filepath.Join(sessionsDir, "20260101-000000-test1234.jsonl")

	f, _ := os.Create(sessionFile)
	f.Write(headerData)
	f.Write([]byte("\n"))
	f.Close()

	result := loadHistoryFromSessions("/tmp/test")
	if len(result) != 0 {
		t.Errorf("expected 0 prompts, got %d", len(result))
	}
}

func TestLoadHistoryFromSessions_PicksMostRecent(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	sessionsDir := filepath.Join(tmpDir, ".tau", "sessions", "-tmp-test-")
	os.MkdirAll(sessionsDir, 0755)

	// Create older session
	oldHeader := types.SessionHeader{
		Type: "session", Version: 1, ID: "old", Timestamp: time.Now().Add(-24 * time.Hour),
		Cwd: "/tmp/test", Name: "old-session",
	}
	oldFile := filepath.Join(sessionsDir, "20260101-000000-old.jsonl")
	f1, _ := os.Create(oldFile)
	d1, _ := json.Marshal(oldHeader)
	f1.Write(d1)
	f1.Write([]byte("\n"))
	d2, _ := json.Marshal(types.SessionEntry{
		Type: types.EntryMessage,
		Data: json.RawMessage(`{"message":{"id":"m1","role":"user","content":[{"type":"text","text":"old prompt"}],"timestamp":"2026-01-01T00:00:00Z"}}`),
	})
	f1.Write(d2)
	f1.Write([]byte("\n"))
	f1.Close()

	// Create newer session
	newHeader := types.SessionHeader{
		Type: "session", Version: 1, ID: "new", Timestamp: time.Now(),
		Cwd: "/tmp/test", Name: "new-session",
	}
	newFile := filepath.Join(sessionsDir, "20260102-000000-new.jsonl")
	f2, _ := os.Create(newFile)
	d3, _ := json.Marshal(newHeader)
	f2.Write(d3)
	f2.Write([]byte("\n"))
	d4, _ := json.Marshal(types.SessionEntry{
		Type: types.EntryMessage,
		Data: json.RawMessage(`{"message":{"id":"m2","role":"user","content":[{"type":"text","text":"new prompt"}],"timestamp":"2026-01-02T00:00:00Z"}}`),
	})
	f2.Write(d4)
	f2.Write([]byte("\n"))
	f2.Close()

	result := loadHistoryFromSessions("/tmp/test")

	// Should pick the newer session (sorted by filename)
	if len(result) != 1 {
		t.Fatalf("expected 1 prompt, got %d: %v", len(result), result)
	}
	// The newer file has a later date prefix, so it should be picked
	if result[0] != "new prompt" {
		t.Errorf("expected 'new prompt', got %q", result[0])
	}
}

func TestPromptHistoryPath_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	path, err := promptHistoryPath("/tmp/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify directory was created
	dir := filepath.Dir(path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("expected directory %s to exist", dir)
	}

	expected := filepath.Join(tmpDir, ".tau", "prompt-history", "-tmp-test-.jsonl")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestAppendPromptHistory_TrimsWhitespace(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	err := appendPromptHistory(tmpDir, "  hello  ")
	if err != nil {
		t.Fatalf("append failed: %v", err)
	}

	loaded := loadPromptHistory(tmpDir)
	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded))
	}
	if loaded[0] != "hello" {
		t.Errorf("expected 'hello', got %q", loaded[0])
	}
}

func TestExtractUserPrompts_SkipsNonTextBlocks(t *testing.T) {
	messages := []types.AgentMessage{
		{
			Role: types.RoleUser,
			Content: []types.ContentBlock{
				{Type: types.BlockText, Text: "text part"},
				{Type: types.BlockToolCall, ToolCall: &types.ToolCallBlock{Name: "bash"}},
			},
		},
	}

	result := extractUserPrompts(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 prompt, got %d", len(result))
	}
	if result[0] != "text part" {
		t.Errorf("expected 'text part', got %q", result[0])
	}
}

func TestMergePromptsIntoHistory_PreservesOrder(t *testing.T) {
	existing := []string{"first", "second", "third"}
	newPrompts := []string{"fourth", "fifth"}

	result := mergePromptsIntoHistory(existing, newPrompts)

	expected := []string{"first", "second", "third", "fourth", "fifth"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d prompts, got %d", len(expected), len(result))
	}
	for i := range result {
		if result[i] != expected[i] {
			t.Errorf("prompt[%d]: expected %q, got %q", i, expected[i], result[i])
		}
	}
}

func TestSaveAndLoadPromptHistory_HandlesInvalidLines(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Write a file with some invalid lines
	path, _ := promptHistoryPath(tmpDir)
	f, _ := os.Create(path)
	f.WriteString(`{"input":"valid1","timestamp":"2026-01-01T00:00:00Z"}` + "\n")
	f.WriteString("this is not json\n")
	f.WriteString(`{"input":"valid2","timestamp":"2026-01-01T00:00:01Z"}` + "\n")
	f.WriteString("\n")
	f.WriteString(`{"input":"valid3","timestamp":"2026-01-01T00:00:02Z"}` + "\n")
	f.Close()

	loaded := loadPromptHistory(tmpDir)
	if len(loaded) != 3 {
		t.Fatalf("expected 3 valid entries, got %d: %v", len(loaded), loaded)
	}

	expected := []string{"valid1", "valid2", "valid3"}
	for i := range loaded {
		if loaded[i] != expected[i] {
			t.Errorf("entry[%d]: expected %q, got %q", i, expected[i], loaded[i])
		}
	}
}

func TestAppendPromptHistory_DedupesConsecutiveOnly(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	appendPromptHistory(tmpDir, "a")
	appendPromptHistory(tmpDir, "b")
	appendPromptHistory(tmpDir, "a") // Should be added (not consecutive duplicate)

	loaded := loadPromptHistory(tmpDir)
	if len(loaded) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(loaded), loaded)
	}

	expected := []string{"a", "b", "a"}
	for i := range loaded {
		if loaded[i] != expected[i] {
			t.Errorf("entry[%d]: expected %q, got %q", i, expected[i], loaded[i])
		}
	}
}

func TestLoadHistoryFromSessions_SkipsMalformedEntries(t *testing.T) {
	tmpDir := t.TempDir()
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	sessionsDir := filepath.Join(tmpDir, ".tau", "sessions", "-tmp-test-")
	os.MkdirAll(sessionsDir, 0755)

	header := types.SessionHeader{
		Type: "session", Version: 1, ID: "test", Timestamp: time.Now(),
		Cwd: "/tmp/test", Name: "test",
	}

	sessionFile := filepath.Join(sessionsDir, "20260101-000000-test.jsonl")
	f, _ := os.Create(sessionFile)
	hd, _ := json.Marshal(header)
	f.Write(hd)
	f.Write([]byte("\n"))

	// Valid entry
	e1, _ := json.Marshal(types.SessionEntry{
		Type: types.EntryMessage,
		Data: json.RawMessage(`{"message":{"id":"m1","role":"user","content":[{"type":"text","text":"valid"}],"timestamp":"2026-01-01T00:00:00Z"}}`),
	})
	f.Write(e1)
	f.Write([]byte("\n"))

	// Malformed entry
	f.Write([]byte(`{"type":"message","data":"not valid json`))
	f.Write([]byte("\n"))

	// Another valid entry
	e2, _ := json.Marshal(types.SessionEntry{
		Type: types.EntryMessage,
		Data: json.RawMessage(`{"message":{"id":"m2","role":"user","content":[{"type":"text","text":"also valid"}],"timestamp":"2026-01-01T00:00:01Z"}}`),
	})
	f.Write(e2)
	f.Write([]byte("\n"))
	f.Close()

	result := loadHistoryFromSessions("/tmp/test")

	// Should only get the valid entries before the malformed one
	// (scanner stops at malformed entry)
	if len(result) < 1 {
		t.Fatalf("expected at least 1 prompt, got %d: %v", len(result), result)
	}
	if result[0] != "valid" {
		t.Errorf("expected first prompt 'valid', got %q", result[0])
	}
}

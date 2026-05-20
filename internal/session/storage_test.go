package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/types"
)

func TestJSONLWriter_RoundTrip(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "test.jsonl")

	w, err := NewJSONLWriter(path)
	if err != nil {
		t.Fatalf("NewJSONLWriter: %v", err)
	}

	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	header := types.SessionHeader{
		Type:      "session",
		Version:   1,
		ID:        "abc12345",
		Timestamp: ts,
		Cwd:       "/home/adam/test",
		Name:      "test-session",
	}

	if err := w.WriteHeader(header); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}

	entry := types.SessionEntry{
		Type:      types.EntryMessage,
		ID:        newID(),
		Timestamp: ts,
	}

	if err := w.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read back
	h, entries, err := ReadEntries(path)
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}

	if h.ID != header.ID {
		t.Errorf("header ID mismatch: got %s, want %s", h.ID, header.ID)
	}
	if h.Version != header.Version {
		t.Errorf("header version mismatch: got %d, want %d", h.Version, header.Version)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Type != types.EntryMessage {
		t.Errorf("entry type mismatch: got %s, want %s", entries[0].Type, types.EntryMessage)
	}
}

func TestJSONLWriter_MultipleEntries(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "test.jsonl")

	w, err := NewJSONLWriter(path)
	if err != nil {
		t.Fatalf("NewJSONLWriter: %v", err)
	}

	header := types.SessionHeader{
		Type:    "session",
		Version: 1,
		ID:      "abc12345",
		Name:    "test",
	}
	if err := w.WriteHeader(header); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}

	ts := time.Now()
	entryTypes := []types.EntryType{
		types.EntryMessage,
		types.EntryModelChange,
		types.EntryThinkingLevelChange,
		types.EntryCompaction,
		types.EntryCustomEntry,
		types.EntryCustomMessage,
		types.EntrySessionInfo,
	}

	for _, et := range entryTypes {
		entry := types.SessionEntry{
			Type:      et,
			ID:        newID(),
			Timestamp: ts,
		}
		if err := w.WriteEntry(entry); err != nil {
			t.Fatalf("WriteEntry(%s): %v", et, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Read back and verify order
	h, entries, err := ReadEntries(path)
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if h.ID != "abc12345" {
		t.Errorf("header ID mismatch")
	}
	if len(entries) != len(entryTypes) {
		t.Fatalf("expected %d entries, got %d", len(entryTypes), len(entries))
	}
	for i, et := range entryTypes {
		if entries[i].Type != et {
			t.Errorf("entry %d type mismatch: got %s, want %s", i, entries[i].Type, et)
		}
	}
}

func TestReadEntries_Corruption_Recovery(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "test.jsonl")

	// Write header + 3 valid entries + partial line
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create: %v", err)
	}

	// Write header
	headerJSON := `{"type":"session","version":1,"id":"abc12345","timestamp":"2026-05-03T12:00:00Z","cwd":"","name":""}`
	f.WriteString(headerJSON + "\n")

	// Write 3 valid entries
	for i := 0; i < 3; i++ {
		entryJSON := `{"type":"message","id":"entry` + string(rune('0'+i)) + `","timestamp":"2026-05-03T12:00:00Z"}`
		f.WriteString(entryJSON + "\n")
	}

	// Write a corrupted partial line (incomplete JSON)
	f.WriteString(`{"type":"message","id":"corrupted","timestamp":"2026-05-03T12:00:00Z"`)
	f.Close()

	// Read entries — should recover 3 valid entries, discard corrupted line
	h, entries, err := ReadEntries(path)
	if err != nil {
		t.Fatalf("ReadEntries should not error on corruption: %v", err)
	}
	if h.ID != "abc12345" {
		t.Errorf("header ID mismatch")
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 valid entries, got %d", len(entries))
	}
}

func TestReadEntries_EmptyFile(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "empty.jsonl")

	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	_, _, err := ReadEntries(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestReadEntries_InvalidHeader(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "invalid.jsonl")

	if err := os.WriteFile(path, []byte("not valid json\n"), 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	_, _, err := ReadEntries(path)
	if err == nil {
		t.Fatal("expected error for invalid header")
	}
}

func TestReadEntries_WrongHeaderType(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "wrong_type.jsonl")

	if err := os.WriteFile(path, []byte(`{"type":"wrong","version":1,"id":"abc"}`+"\n"), 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	_, _, err := ReadEntries(path)
	if err == nil {
		t.Fatal("expected error for wrong header type")
	}
}

func TestReadEntries_UnsupportedVersion(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "old_version.jsonl")

	if err := os.WriteFile(path, []byte(`{"type":"session","version":0,"id":"abc"}`+"\n"), 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	_, _, err := ReadEntries(path)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestReadEntries_FileNotFound(t *testing.T) {
	_, _, err := ReadEntries("/nonexistent/path/file.jsonl")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestCreateSessionFile(t *testing.T) {
	dir := testutil.TempDir(t)
	sessionDir := filepath.Join(dir, "sessions", "test-cwd")

	ts := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	header := types.SessionHeader{
		Type:      "session",
		Version:   1,
		ID:        "abc12345",
		Timestamp: ts,
		Cwd:       "/test/cwd",
		Name:      "test-session",
	}

	fullPath, writer, err := CreateSessionFile(sessionDir, header)
	if err != nil {
		t.Fatalf("CreateSessionFile: %v", err)
	}
	defer writer.Close()

	// Verify file was created
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("session file not created: %v", err)
	}

	// Verify header is readable
	h, entries, err := ReadEntries(fullPath)
	if err != nil {
		t.Fatalf("ReadEntries: %v", err)
	}
	if h.ID != "abc12345" {
		t.Errorf("header ID mismatch: got %s, want %s", h.ID, "abc12345")
	}
	if h.Version != 1 {
		t.Errorf("header version mismatch: got %d, want 1", h.Version)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}

	// Verify directory was created
	if _, err := os.Stat(sessionDir); err != nil {
		t.Fatalf("session directory not created: %v", err)
	}
}

func TestNewJSONLWriter_CreatesFile(t *testing.T) {
	dir := testutil.TempDir(t)
	path := filepath.Join(dir, "new.jsonl")

	w, err := NewJSONLWriter(path)
	if err != nil {
		t.Fatalf("NewJSONLWriter: %v", err)
	}
	defer w.Close()

	// File should exist
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestGenerateID(t *testing.T) {
	id1 := GenerateID()
	id2 := GenerateID()

	if len(id1) != 8 {
		t.Errorf("ID length: got %d, want 8", len(id1))
	}
	if id1 == id2 {
		t.Error("IDs should be unique")
	}
}

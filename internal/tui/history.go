package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/types"
)

const maxPromptHistory = 100

// PromptHistoryEntry represents a single prompt history entry.
type PromptHistoryEntry struct {
	Input     string    `json:"input"`
	Timestamp time.Time `json:"timestamp"`
}

// promptHistoryPath returns the per-project prompt history file path.
func promptHistoryPath(cwd string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}

	encoded := config.EncodeCWD(cwd)
	dir := filepath.Join(home, config.TauDirName, "prompt-history")

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create history dir: %w", err)
	}

	return filepath.Join(dir, encoded+".jsonl"), nil
}

// loadPromptHistory reads the prompt history file and returns inputs newest-first.
// Invalid lines are silently skipped.
func loadPromptHistory(cwd string) []string {
	path, err := promptHistoryPath(cwd)
	if err != nil {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return nil
	}
	defer f.Close()

	var entries []PromptHistoryEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry PromptHistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Entries are stored newest-first, extract inputs
	var inputs []string
	for _, e := range entries {
		if e.Input != "" {
			inputs = append(inputs, e.Input)
		}
	}

	return inputs
}

// savePromptHistory writes all entries to the history file (overwrite).
func savePromptHistory(cwd string, entries []PromptHistoryEntry) error {
	path, err := promptHistoryPath(cwd)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create history file: %w", err)
	}
	defer f.Close()

	buf := bufio.NewWriter(f)
	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if _, err := buf.Write(data); err != nil {
			return err
		}
		if err := buf.WriteByte('\n'); err != nil {
			return err
		}
	}

	return buf.Flush()
}

// appendPromptHistory adds a new prompt to the history file.
// Prepends the entry (newest-first), trims to maxPromptHistory, and saves.
// Consecutive duplicates are not added.
func appendPromptHistory(cwd, input string) error {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil
	}

	// Load existing entries
	path, err := promptHistoryPath(cwd)
	if err != nil {
		return err
	}

	var entries []PromptHistoryEntry
	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var entry PromptHistoryEntry
			if err := json.Unmarshal(line, &entry); err != nil {
				continue
			}
			entries = append(entries, entry)
		}
		f.Close()
	}

	// Don't add consecutive duplicates
	if len(entries) > 0 && entries[0].Input == trimmed {
		return nil
	}

	// Prepend new entry
	entries = append([]PromptHistoryEntry{
		{Input: trimmed, Timestamp: time.Now()},
	}, entries...)

	// Trim to max
	if len(entries) > maxPromptHistory {
		entries = entries[:maxPromptHistory]
	}

	return savePromptHistory(cwd, entries)
}

// extractUserPrompts extracts text content from user messages.
// Returns the prompts in order (oldest-first).
func extractUserPrompts(messages []types.AgentMessage) []string {
	var prompts []string
	for _, msg := range messages {
		if msg.Role != types.RoleUser {
			continue
		}
		var text string
		for _, block := range msg.Content {
			if block.Type == types.BlockText {
				text += block.Text
			}
		}
		if text != "" {
			prompts = append(prompts, text)
		}
	}
	return prompts
}

// mergePromptsIntoHistory merges new prompts into existing history,
// deduplicating against entries already present. Returns newest-first.
func mergePromptsIntoHistory(existing []string, newPrompts []string) []string {
	seen := make(map[string]bool, len(existing))
	for _, p := range existing {
		seen[p] = true
	}

	// Add new prompts that aren't already in history
	for _, p := range newPrompts {
		if !seen[p] {
			existing = append(existing, p)
			seen[p] = true
		}
	}

	return existing
}

// loadHistoryFromSessions scans existing session files in the sessions directory
// and extracts user prompts from the most recent session. Returns prompts newest-first.
func loadHistoryFromSessions(cwd string) []string {
	sessionsDir, err := config.SessionsDir(cwd)
	if err != nil {
		return nil
	}

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	// Find the most recent .jsonl file
	var latestFile string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
		}
	}

	if latestFile == "" {
		return nil
	}

	filePath := filepath.Join(sessionsDir, latestFile)
	header, sessionEntries, err := readSessionEntries(filePath)
	if err != nil {
		return nil
	}

	// Reconstruct messages from entries
	var messages []types.AgentMessage
	for _, entry := range sessionEntries {
		if entry.Type != types.EntryMessage {
			continue
		}
		var data struct {
			Message types.AgentMessage `json:"message"`
		}
		if err := entry.UnmarshalData(&data); err != nil {
			continue
		}
		messages = append(messages, data.Message)
	}

	_ = header
	return extractUserPrompts(messages)
}

// readSessionEntries is a local helper to read entries from a session file.
// Duplicated from resume.go to avoid import cycles.
func readSessionEntries(path string) (*types.SessionHeader, []types.SessionEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	var header *types.SessionHeader
	var entries []types.SessionEntry

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		lineNum++

		if lineNum == 1 {
			var h types.SessionHeader
			if err := json.Unmarshal(line, &h); err != nil {
				return nil, nil, err
			}
			header = &h
			continue
		}

		var entry types.SessionEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			break
		}
		entries = append(entries, entry)
	}

	return header, entries, nil
}

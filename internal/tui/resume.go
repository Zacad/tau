package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/session"
	"github.com/adam/tau/internal/tui/palette"
	"github.com/adam/tau/internal/types"
)

// sessionResumeItem implements palette.PaletteItem for the session resume list.
type sessionResumeItem struct {
	title       string
	description string
	filePath    string
}

func (i sessionResumeItem) Title() string       { return i.title }
func (i sessionResumeItem) Description() string { return i.description }
func (i sessionResumeItem) FilterValue() string { return i.title + " " + i.description }

// scannedSession holds metadata parsed from a session file.
type scannedSession struct {
	name      string
	timestamp time.Time
	cwd       string
	filePath  string
	id        string
	lastPrompt string // first ~80 chars of the last user message
}

// scanSessionsForResume reads all .jsonl files from the session directory and
// returns session metadata sorted by timestamp (newest first).
func scanSessionsForResume(cwd string) ([]scannedSession, error) {
	sessionsDir, err := config.SessionsDir(cwd)
	if err != nil {
		return nil, fmt.Errorf("get sessions dir: %w", err)
	}

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	var sessions []scannedSession
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(sessionsDir, e.Name())
		info, err := readSessionInfo(filePath)
		if err != nil {
			continue // skip malformed files
		}
		sessions = append(sessions, info)
	}

	sortSessionsByTimestamp(sessions)

	return sessions, nil
}

func sortSessionsByTimestamp(sessions []scannedSession) {
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].timestamp.After(sessions[j].timestamp)
	})
}

// readSessionInfo reads the header and extracts the last user message preview.
func readSessionInfo(filePath string) (scannedSession, error) {
	header, entries, err := session.ReadEntries(filePath)
	if err != nil {
		return scannedSession{}, err
	}

	name := header.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	}

	lastPrompt := extractLastUserPrompt(entries)

	return scannedSession{
		name:       name,
		timestamp:  header.Timestamp,
		cwd:        header.Cwd,
		filePath:   filePath,
		id:         header.ID,
		lastPrompt: lastPrompt,
	}, nil
}

// extractLastUserPrompt finds the last user message in session entries and
// returns a truncated preview (~80 chars).
func extractLastUserPrompt(entries []types.SessionEntry) string {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].Type != types.EntryMessage {
			continue
		}
		var data session.MessageData
		if err := entries[i].UnmarshalData(&data); err != nil {
			continue
		}
		if data.Message.Role != types.RoleUser {
			continue
		}
		// Extract text from user message blocks
		var text string
		for _, block := range data.Message.Content {
			if block.Type == types.BlockText {
				text += block.Text
			}
		}
		if text == "" {
			continue
		}
		if len(text) > 80 {
			text = text[:80] + "…"
		}
		return text
	}
	return ""
}

// formatRelativeTime returns a human-readable relative time string.
func formatRelativeTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// cmdResume opens the command palette with a list of past sessions to resume.
func cmdResume(m *Model, _ string) tea.Cmd {
	sessions, err := scanSessionsForResume(m.cwd)
	if err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: "Failed to list sessions: " + err.Error(),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}

	if len(sessions) == 0 {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "No previous sessions found for this directory",
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}

	items := make([]palette.PaletteItem, len(sessions))
	avail := make([]bool, len(sessions))
	for i, s := range sessions {
		relTime := formatRelativeTime(s.timestamp)
		desc := relTime
		if s.lastPrompt != "" {
			desc = s.lastPrompt + "  •  " + relTime
		}
		items[i] = sessionResumeItem{
			title:       s.name,
			description: desc,
			filePath:    s.filePath,
		}
		avail[i] = true
	}

	m.palette.OpenWithItems(items, avail, func(item palette.PaletteItem, index int) tea.Cmd {
		sri, ok := item.(sessionResumeItem)
		if !ok {
			return nil
		}
		m.paletteTaskTitle = "Resuming session"
		m.paletteTaskFunc = func() (bool, string, error) {
			return resumeSessionTask(m, sri.filePath)
		}
		cmd := m.palette.ShowTask("Resuming session", m.paletteTaskFunc)
		m.paletteTaskFunc = nil
		return cmd
	})
	m.paletteActive = true
	m.input.Blur()
	return nil
}

// resumeSessionTask closes the current session and resumes the specified
// session file. Returns (success, message, error).
func resumeSessionTask(m *Model, sessionPath string) (bool, string, error) {
	if err := m.session.ResumeSession(sessionPath); err != nil {
		return false, "Failed to resume session: " + err.Error(), nil
	}

	sessionName := m.session.Name()
	sessionID := m.session.ID()
	idDisplay := sessionID
	if len(idDisplay) > 8 {
		idDisplay = idDisplay[:8]
	}

	return true, fmt.Sprintf("Resumed session: %s (%s)", sessionName, idDisplay), nil
}

// buildBlocksFromSession converts session messages into TUI message blocks.
// Returns the blocks and the turn count.
func buildBlocksFromSession(messages []types.AgentMessage) ([]messageBlock, int) {
	var blocks []messageBlock
	turnCount := 0

	for _, msg := range messages {
		switch msg.Role {
		case types.RoleUser:
			var text string
			for _, block := range msg.Content {
				if block.Type == types.BlockText {
					text += block.Text
				}
			}
			if text != "" {
				blocks = append(blocks, messageBlock{
					kind: blockUserMessage,
					text: text,
				})
			}

	case types.RoleAssistant:
		// Consolidate consecutive blocks of the same type to avoid
		// rendering multiple headers for streaming-chunk fragments.
		for _, block := range msg.Content {
			switch block.Type {
			case types.BlockThinking:
				// Merge into previous thinking block if adjacent
				if len(blocks) > 0 && blocks[len(blocks)-1].kind == blockThinking {
					blocks[len(blocks)-1].text += block.Text
				} else {
					blocks = append(blocks, messageBlock{
						kind: blockThinking,
						text: block.Text,
					})
				}
			case types.BlockText:
				// Merge into previous assistant text block if adjacent
				if len(blocks) > 0 && blocks[len(blocks)-1].kind == blockAssistantText {
					blocks[len(blocks)-1].text += block.Text
					blocks[len(blocks)-1].renderedMarkdown = "" // invalidate cache
				} else {
					blocks = append(blocks, messageBlock{
						kind:           blockAssistantText,
						text:           block.Text,
						isFinalized:    true,
						renderedMarkdown: RenderMarkdown(block.Text, 80),
					})
				}
			case types.BlockToolCall:
				if block.ToolCall != nil {
					argsJSON := ""
					if len(block.ToolCall.Arguments) > 0 {
						b, _ := json.Marshal(block.ToolCall.Arguments)
						argsJSON = string(b)
					}
					blocks = append(blocks, messageBlock{
						kind:     blockToolCall,
						toolName: block.ToolCall.Name,
						toolArgs: argsJSON,
						toolSt:   toolSuccess,
					})
				}
			}
		}
		turnCount++

		case types.RoleToolResult:
			var content string
			for _, block := range msg.Content {
				if block.Type == types.BlockText {
					content += block.Text
				}
			}
			blocks = append(blocks, messageBlock{
				kind:              blockToolResult,
				toolResultName:    msg.ToolCallID,
				toolResultContent: content,
			})
		}
	}

	return blocks, turnCount
}

// handleResumeComplete resets the TUI state after a successful session resume
// and loads the session history into the viewport.
func handleResumeComplete(m *Model) {
	mod := m.session.Model()

	m.modelName = mod.ID
	m.modelProv = mod.Provider
	m.modelReasoning = mod.Reasoning
	if m.modelName == "" {
		m.modelName = "no model"
		m.modelProv = "none"
	}
	m.thinkingLevel = string(m.session.ThinkingLevel())
	m.sessionID = m.session.ID()
	m.sessionName = m.session.Name()

	m.pendingBuilder.Reset()
	m.pendingKind = 0
	m.pendingToolIndex = -1
	m.pendingRendered = ""
	m.pendingRenderedLen = 0
	m.stopSpinner()
	m.invalidateRenderedCache()
	m.lastSetContentPendingLen = 0
	m.lastSetContentPendingRenderedLen = 0
	m.lastSetContentBlocksLen = 0

	// Build blocks from session history
	messages := m.session.Messages()
	blocks, turnCount := buildBlocksFromSession(messages)
	m.blocks = blocks
	m.turnCount = turnCount

	// Seed prompt history from resumed session messages
	sessionPrompts := extractUserPrompts(messages)
	m.promptHistory = mergePromptsIntoHistory(m.promptHistory, sessionPrompts)
	m.promptHistoryIndex = -1

	// Set usage from session
	m.usage = m.session.Usage()

	m.updateViewport()
}

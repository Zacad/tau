package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/list"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/types"
)

// sessionItem implements list.DefaultItem for the session picker.
type sessionItem struct {
	title       string
	description string
	filePath    string
}

func (i sessionItem) Title() string       { return i.title }
func (i sessionItem) Description() string { return i.description }
func (i sessionItem) FilterValue() string { return i.title + " " + i.description }

// sessionPickerModel is the bubbletea model for the session picker.
type sessionPickerModel struct {
	list     list.Model
	selected string
	quitting bool
	width    int
	height   int
}

func (m sessionPickerModel) Init() tea.Cmd {
	return nil
}

func (m sessionPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		padding := 4
		m.list.SetSize(msg.Width-padding, msg.Height-padding)
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			item := m.list.SelectedItem()
			if item != nil {
				if si, ok := item.(sessionItem); ok {
					m.selected = si.filePath
				}
			}
			m.quitting = true
			return m, tea.Quit
		case "esc", "q":
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m sessionPickerModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	v := tea.NewView(m.list.View())
	v.AltScreen = true
	return v
}

// sessionInfo holds metadata parsed from a session file header.
type sessionInfo struct {
	name      string
	timestamp time.Time
	cwd       string
	filePath  string
}

// scanSessions reads all .jsonl files from the session directory and parses
// their headers to build a list of session metadata.
func scanSessions(cwd string) ([]sessionInfo, error) {
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

	var sessions []sessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(sessionsDir, e.Name())
		info, err := readSessionHeader(filePath)
		if err != nil {
			continue // skip malformed files
		}
		info.filePath = filePath
		sessions = append(sessions, info)
	}

	// Sort by timestamp, newest first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].timestamp.After(sessions[j].timestamp)
	})

	return sessions, nil
}

// readSessionHeader reads just the first line (header) of a JSONL session file.
func readSessionHeader(filePath string) (sessionInfo, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return sessionInfo{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return sessionInfo{}, fmt.Errorf("empty session file")
	}

	var header types.SessionHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		return sessionInfo{}, fmt.Errorf("parse header: %w", err)
	}

	name := header.Name
	if name == "" {
		// Derive name from filename
		name = strings.TrimSuffix(filepath.Base(filePath), ".jsonl")
	}

	return sessionInfo{
		name:      name,
		timestamp: header.Timestamp,
		cwd:       header.Cwd,
	}, nil
}

// runSessionPicker shows an interactive session picker and returns the
// selected session file path, or empty string if cancelled.
func runSessionPicker(cwd string) (string, error) {
	sessions, err := scanSessions(cwd)
	if err != nil {
		return "", err
	}

	if len(sessions) == 0 {
		return "", fmt.Errorf("no sessions found for %s", cwd)
	}

	items := make([]list.Item, len(sessions))
	for i, s := range sessions {
		dateStr := s.timestamp.Format("2006-01-02 15:04")
		title := s.name
		description := fmt.Sprintf("%s  •  %s", dateStr, s.cwd)
		items[i] = sessionItem{
			title:       title,
			description: description,
			filePath:    s.filePath,
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(2)

	l := list.New(items, delegate, 80, 20)
	l.Title = "Resume Session"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	m := sessionPickerModel{
		list: l,
	}

	p := tea.NewProgram(&m)
	result, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("session picker: %w", err)
	}

	if picker, ok := result.(*sessionPickerModel); ok {
		return picker.selected, nil
	}
	return "", nil
}

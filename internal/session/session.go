package session

import (
	"fmt"
	"os"
	"sync"

	"github.com/adam/tau/internal/types"
)

// Session manages the lifecycle of a single JSONL session file.
// It provides create, resume, append, and delete operations,
// and maintains in-memory state (messages, usage, model, thinking level).
type Session struct {
	mu            sync.RWMutex
	file          string
	header        types.SessionHeader
	messages      []types.AgentMessage
	usage         types.Usage
	writer        *JSONLWriter
	currentModel  string
	currentProvider string
	thinkingLevel types.ThinkingLevel
	// modelThinkingLevels stores the last thinking level set for each model.
	modelThinkingLevels map[string]types.ThinkingLevel
}

// CreateSession creates a new session in dirPath with the given cwd and name.
// If id is empty, a unique ID is generated.
func CreateSession(dirPath string, cwd string, name string, id string) (*Session, error) {
	if id == "" {
		id = GenerateID()
	}

	ts := now()
	header := types.SessionHeader{
		Type:      "session",
		Version:   1,
		ID:        id,
		Timestamp: ts,
		Cwd:       cwd,
		Name:      name,
	}

	fullPath, writer, err := CreateSessionFile(dirPath, header)
	if err != nil {
		return nil, err
	}

	return &Session{
		file:                fullPath,
		header:              header,
		writer:              writer,
		modelThinkingLevels: make(map[string]types.ThinkingLevel),
	}, nil
}

// OpenSession resumes an existing session from the given file path.
// It reads the JSONL file, rebuilds the message list, and opens the
// file in append mode for continued writing.
func OpenSession(filePath string) (*Session, error) {
	header, entries, err := ReadEntries(filePath)
	if err != nil {
		return nil, fmt.Errorf("read session entries: %w", err)
	}

	s := &Session{
		file:                filePath,
		header:              *header,
		modelThinkingLevels: make(map[string]types.ThinkingLevel),
	}

	// Rebuild state from entries
	for _, entry := range entries {
		switch entry.Type {
		case types.EntryMessage:
			var data MessageData
			if err := entry.UnmarshalData(&data); err != nil {
				continue // skip malformed entries
			}
			s.messages = append(s.messages, data.Message)

		case types.EntryModelChange:
			var data ModelChangeData
			if err := entry.UnmarshalData(&data); err != nil {
				continue
			}
			s.currentModel = data.ModelID
			s.currentProvider = data.Provider

		case types.EntryThinkingLevelChange:
			var data ThinkingLevelChangeData
			if err := entry.UnmarshalData(&data); err != nil {
				continue
			}
			s.thinkingLevel = data.Level
			if data.ModelID != "" {
				s.modelThinkingLevels[data.ModelID] = data.Level
			}

		case types.EntrySessionInfo:
			var data SessionInfoData
			if err := entry.UnmarshalData(&data); err != nil {
				continue
			}
			if data.DisplayName != "" {
				s.header.Name = data.DisplayName
			}
			if data.Usage != nil {
				s.usage = *data.Usage
			}

		case types.EntryCustomEntry, types.EntryCustomMessage, types.EntryCompaction:
			// Stored in file but not reconstructed into in-memory state
			// (compaction summaries are LLM-visible via messages on resume)
		}
	}

	// Open file in append mode for continued writing
	writer, err := NewJSONLWriter(filePath)
	if err != nil {
		return nil, fmt.Errorf("open session for append: %w", err)
	}
	s.writer = writer

	return s, nil
}

// Append appends a new entry to the session file and updates in-memory state.
func (s *Session) Append(entryType types.EntryType, data any) error {
	return s.appendWithUsage(entryType, data, nil)
}

// AppendWithUsage appends a new entry and updates the cumulative usage tracker.
func (s *Session) AppendWithUsage(entryType types.EntryType, data any, usage *types.Usage) error {
	return s.appendWithUsage(entryType, data, usage)
}

func (s *Session) appendWithUsage(entryType types.EntryType, data any, usage *types.Usage) error {
	entryData, err := MarshalEntryData(entryType, data)
	if err != nil {
		return fmt.Errorf("marshal entry data: %w", err)
	}

	entry := types.SessionEntry{
		Type:      entryType,
		ID:        GenerateID(),
		Timestamp: now(),
		Data:      entryData,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Write to file
	if err := s.writer.WriteEntry(entry); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}

	// Update in-memory state
	switch entryType {
	case types.EntryMessage:
		var msgData MessageData
		if err := entry.UnmarshalData(&msgData); err == nil {
			s.messages = append(s.messages, msgData.Message)
		}

	case types.EntryModelChange:
		var d ModelChangeData
		if err := entry.UnmarshalData(&d); err == nil {
			s.currentModel = d.ModelID
			s.currentProvider = d.Provider
		}

	case types.EntryThinkingLevelChange:
		var d ThinkingLevelChangeData
		if err := entry.UnmarshalData(&d); err == nil {
			s.thinkingLevel = d.Level
			if d.ModelID != "" {
				s.modelThinkingLevels[d.ModelID] = d.Level
			}
		}

	case types.EntrySessionInfo:
		var d SessionInfoData
		if err := entry.UnmarshalData(&d); err == nil {
			if d.DisplayName != "" {
				s.header.Name = d.DisplayName
			}
			if d.Usage != nil {
				s.usage = *d.Usage
			}
		}
	}

	// Update cumulative usage
	if usage != nil {
		s.usage = addUsage(s.usage, *usage)
	}

	return nil
}

// Messages returns a copy of the current message list.
func (s *Session) Messages() []types.AgentMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cpy := make([]types.AgentMessage, len(s.messages))
	copy(cpy, s.messages)
	return cpy
}

// Usage returns the cumulative token usage across all turns.
func (s *Session) Usage() types.Usage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usage
}

// Close flushes and closes the session file writer.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writer == nil {
		return nil
	}
	return s.writer.Close()
}

// Delete removes the session file from disk and closes the writer.
func (s *Session) Delete() error {
	s.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	return os.Remove(s.file)
}

// Sync flushes the writer buffer to disk.
func (s *Session) Sync() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.writer == nil {
		return nil
	}
	return s.writer.Sync()
}

// ID returns the session ID.
func (s *Session) ID() string {
	return s.header.ID
}

// Name returns the session name.
func (s *Session) Name() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.header.Name
}

// Cwd returns the session working directory.
func (s *Session) Cwd() string {
	return s.header.Cwd
}

// File returns the full path to the session JSONL file.
func (s *Session) File() string {
	return s.file
}

// CurrentModel returns the model currently active in this session.
func (s *Session) CurrentModel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentModel
}

// CurrentProvider returns the provider currently active in this session.
func (s *Session) CurrentProvider() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentProvider
}

// CurrentThinkingLevel returns the thinking level currently active.
func (s *Session) CurrentThinkingLevel() types.ThinkingLevel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.thinkingLevel
}

// SetName updates the session display name and persists a session_info entry.
func (s *Session) SetName(name string) error {
	data := SessionInfoData{DisplayName: name}
	return s.Append(types.EntrySessionInfo, data)
}

// SetModel updates the current model and persists a model_change entry.
func (s *Session) SetModel(modelID, provider string) error {
	data := ModelChangeData{ModelID: modelID, Provider: provider}
	return s.Append(types.EntryModelChange, data)
}

// SetThinkingLevel updates the thinking level and persists a thinking_level_change entry.
func (s *Session) SetThinkingLevel(modelID string, level types.ThinkingLevel) error {
	data := ThinkingLevelChangeData{ModelID: modelID, Level: level}
	return s.Append(types.EntryThinkingLevelChange, data)
}

// GetThinkingLevelForModel returns the last thinking level set for a specific model.
func (s *Session) GetThinkingLevelForModel(modelID string) types.ThinkingLevel {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.modelThinkingLevels == nil {
		return types.ThinkingMedium
	}
	if level, ok := s.modelThinkingLevels[modelID]; ok {
		return level
	}
	return types.ThinkingMedium
}

// SaveUsage persists the current cumulative usage as a session_info entry.
// Call this periodically (e.g., after each turn) to ensure usage survives restarts.
func (s *Session) SaveUsage() error {
	s.mu.RLock()
	usage := s.usage
	s.mu.RUnlock()

	data := SessionInfoData{Usage: &usage}
	return s.Append(types.EntrySessionInfo, data)
}

// addUsage adds two Usage structs together (cumulative accumulator).
func addUsage(a, b types.Usage) types.Usage {
	return types.Usage{
		Input:       a.Input + b.Input,
		Output:      a.Output + b.Output,
		CacheRead:   a.CacheRead + b.CacheRead,
		CacheWrite:  a.CacheWrite + b.CacheWrite,
		TotalTokens: a.TotalTokens + b.TotalTokens,
		Cost: types.CostDollars{
			Input:      a.Cost.Input + b.Cost.Input,
			Output:     a.Cost.Output + b.Cost.Output,
			CacheRead:  a.Cost.CacheRead + b.Cost.CacheRead,
			CacheWrite: a.Cost.CacheWrite + b.Cost.CacheWrite,
			Total:      a.Cost.Total + b.Cost.Total,
		},
	}
}

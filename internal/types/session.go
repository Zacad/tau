package types

import (
	"encoding/json"
	"time"
)

// EntryType defines the type of a session entry in the JSONL file.
type EntryType string

const (
	EntrySession            EntryType = "session"
	EntryMessage            EntryType = "message"
	EntryModelChange        EntryType = "model_change"
	EntryThinkingLevelChange EntryType = "thinking_level_change"
	EntryCompaction         EntryType = "compaction"
	EntryCustomEntry        EntryType = "custom_entry"
	EntryCustomMessage      EntryType = "custom_message"
	EntrySessionInfo        EntryType = "session_info"
)

// SessionHeader is the first line of a JSONL session file.
// It contains session metadata and is distinct from regular entries.
type SessionHeader struct {
	Type      string    `json:"type"` // always "session"
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Cwd       string    `json:"cwd"`
	Name      string    `json:"name"`
}

// SessionEntry represents a single line in a JSONL session file.
// Data is json.RawMessage to defer unmarshaling until the entry type is known.
type SessionEntry struct {
	Type      EntryType       `json:"type"`
	ID        string          `json:"id"`
	ParentID  string          `json:"parent_id,omitempty"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
}

// UnmarshalData unmarshals the entry's Data payload into the provided value.
func (e *SessionEntry) UnmarshalData(v any) error {
	return json.Unmarshal(e.Data, v)
}

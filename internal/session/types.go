package session

import (
	"encoding/json"
	"fmt"

	"github.com/adam/tau/internal/types"
)

// MessageData wraps an AgentMessage for JSONL persistence.
type MessageData struct {
	Message types.AgentMessage `json:"message"`
}

// ModelChangeData records a model switch event.
type ModelChangeData struct {
	ModelID string `json:"model_id"`
}

// ThinkingLevelChangeData records a thinking level adjustment.
type ThinkingLevelChangeData struct {
	ModelID string              `json:"model_id"`
	Level   types.ThinkingLevel `json:"level"`
}

// CompactionData records a compaction summary.
type CompactionData struct {
	FirstKeptEntryID string `json:"first_kept_entry_id"`
	TokensBefore     int    `json:"tokens_before"`
	Summary          string `json:"summary"`
	Details          string `json:"details,omitempty"`
}

// CustomEntryData stores internal metadata (non-LLM-visible).
type CustomEntryData struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// CustomMessageData stores extension messages (LLM-visible).
type CustomMessageData struct {
	Source  string `json:"source"`
	Content string `json:"content"`
}

// SessionInfoData stores session-level metadata.
type SessionInfoData struct {
	DisplayName string      `json:"display_name"`
	Usage       *types.Usage `json:"usage,omitempty"`
}

// MarshalEntryData converts a typed payload to json.RawMessage for SessionEntry.Data.
func MarshalEntryData(entryType types.EntryType, data any) (json.RawMessage, error) {
	return json.Marshal(data)
}

// UnmarshalEntryData converts SessionEntry.Data back to the appropriate typed struct
// based on the entry type.
func UnmarshalEntryData(entryType types.EntryType, raw json.RawMessage) (any, error) {
	var target any
	switch entryType {
	case types.EntryMessage:
		target = &MessageData{}
	case types.EntryModelChange:
		target = &ModelChangeData{}
	case types.EntryThinkingLevelChange:
		target = &ThinkingLevelChangeData{}
	case types.EntryCompaction:
		target = &CompactionData{}
	case types.EntryCustomEntry:
		target = &CustomEntryData{}
	case types.EntryCustomMessage:
		target = &CustomMessageData{}
	case types.EntrySessionInfo:
		target = &SessionInfoData{}
	default:
		return nil, fmt.Errorf("unknown entry type: %s", entryType)
	}

	if raw == nil {
		return target, nil
	}

	if err := json.Unmarshal(raw, target); err != nil {
		return nil, fmt.Errorf("unmarshal %s entry: %w", entryType, err)
	}
	return target, nil
}

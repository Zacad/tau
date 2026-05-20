package types

import (
	"github.com/invopop/jsonschema"
)

// ThinkingLevel controls the depth of model reasoning/thinking output.
type ThinkingLevel string

const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
)

// AllThinkingLevels returns all standardized thinking levels in order.
func AllThinkingLevels() []ThinkingLevel {
	return []ThinkingLevel{ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh}
}

// ThinkingLevelDescription returns a human-readable description for a thinking level.
func ThinkingLevelDescription(level ThinkingLevel) string {
	switch level {
	case ThinkingOff:
		return "No reasoning"
	case ThinkingMinimal:
		return "Very brief reasoning (~1k tokens)"
	case ThinkingLow:
		return "Light reasoning (~2k tokens)"
	case ThinkingMedium:
		return "Moderate reasoning (~4k tokens)"
	case ThinkingHigh:
		return "Deep reasoning (~8k tokens)"
	case ThinkingXHigh:
		return "Maximum reasoning (~16k tokens)"
	default:
		return ""
	}
}

// StreamEventType defines the type of streaming event emitted during LLM response generation.
type StreamEventType string

const (
	EventStart         StreamEventType = "start"
	EventTextStart     StreamEventType = "text_start"
	EventTextDelta     StreamEventType = "text_delta"
	EventTextEnd       StreamEventType = "text_end"
	EventThinkingStart StreamEventType = "thinking_start"
	EventThinkingDelta StreamEventType = "thinking_delta"
	EventThinkingEnd   StreamEventType = "thinking_end"
	EventToolCallStart StreamEventType = "toolcall_start"
	EventToolCallEnd   StreamEventType = "toolcall_end"
	EventDone          StreamEventType = "done"
	EventError         StreamEventType = "error"
)

// StreamEvent is emitted by a Provider during streaming response generation.
type StreamEvent struct {
	Type    StreamEventType `json:"type"`
	Delta   string          `json:"delta,omitempty"`
	Message *AgentMessage   `json:"message,omitempty"`
	Usage   *Usage          `json:"usage,omitempty"`
	// Error is the error message string. Stored as string (not error interface) for JSON serialization.
	Error string `json:"error,omitempty"`
}

// StreamOptions configures how a provider streams a completion.
type StreamOptions struct {
	ThinkingLevel ThinkingLevel
	MaxTokens     int
	Temperature   float64
	SystemPrompt  string
	Tools         []ToolDefinition
}

// ToolDefinition describes a tool available for LLM tool calling.
// Parameters is generated from a Go struct via github.com/invopop/jsonschema.
type ToolDefinition struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Parameters  *jsonschema.Schema `json:"parameters"`
}

// CostInfo holds pricing information for a model ($ per 1M tokens).
type CostInfo struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
}

// CostDollars holds actual dollar costs for a usage snapshot.
type CostDollars struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read,omitempty"`
	CacheWrite float64 `json:"cache_write,omitempty"`
	Total      float64 `json:"total"`
}

// Usage tracks token consumption for a single LLM response.
type Usage struct {
	Input       int         `json:"input"`
	Output      int         `json:"output"`
	CacheRead   int         `json:"cache_read,omitempty"`
	CacheWrite  int         `json:"cache_write,omitempty"`
	TotalTokens int         `json:"total_tokens"`
	Cost        CostDollars `json:"cost"`
}

// Model describes an LLM model with its capabilities and configuration.
// Defined in types/ (not provider/) to avoid import cycles — provider/ imports this type.
type Model struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Provider         string            `json:"provider"`
	API              string            `json:"api"`
	BaseURL          string            `json:"base_url,omitempty"`
	Reasoning        bool              `json:"reasoning,omitempty"`
	InputTypes       []string          `json:"input_types,omitempty"`
	Cost             CostInfo          `json:"cost,omitempty"`
	ContextWindow    int               `json:"context_window,omitempty"`
	MaxTokens        int               `json:"max_tokens,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	Compat           map[string]any    `json:"compat,omitempty"`
	ThinkingLevelMap map[string]string `json:"thinking_level_map,omitempty"`
}

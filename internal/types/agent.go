package types

// AgentEventType defines the type of agent event emitted during the agent loop.
type AgentEventType string

const (
	AgentEventStart        AgentEventType = "agent_start"
	AgentEventMessageStart AgentEventType = "message_start"
	AgentEventTextDelta    AgentEventType = "text_delta"
	AgentEventThinkingDelta AgentEventType = "thinking_delta"
	AgentEventToolExecStart AgentEventType = "tool_execution_start"
	AgentEventToolExecEnd  AgentEventType = "tool_execution_end"
	AgentEventToolProgress AgentEventType = "tool_progress"
	AgentEventToolResult   AgentEventType = "tool_result"
	AgentEventMessageEnd   AgentEventType = "message_end"
	AgentEventTurnEnd      AgentEventType = "turn_end"
	AgentEventAgentEnd     AgentEventType = "agent_end"
	AgentEventSubAgentStart AgentEventType = "subagent_start"
	AgentEventSubAgentEnd  AgentEventType = "subagent_end"
	AgentEventError        AgentEventType = "error"
)

// AgentEvent is emitted by the agent loop at each state transition.
// It is used by the SDK event subscription system (Task 013).
type AgentEvent struct {
	Type AgentEventType `json:"type"`
	// Data holds event-specific payload (varies by Type).
	Data any `json:"-"`
	// SubAgentID is set when the event originated from a subagent.
	SubAgentID *string `json:"sub_agent_id,omitempty"`
}

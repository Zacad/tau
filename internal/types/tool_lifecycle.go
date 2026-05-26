package types

import "encoding/json"

// ToolLifecyclePhase describes the canonical phase of a model-requested tool call
// at the agent boundary. The phases intentionally separate provider/model
// metadata from local tool execution:
//   - requested: the provider stream announced a tool call, usually before full
//     arguments are available. During the migration this is emitted through
//     AgentEventToolExecStart.
//   - finalized: the tool call metadata is complete enough to execute or display.
//     During the migration this is emitted through AgentEventToolExecEnd because
//     that legacy event name historically meant "tool-call metadata is complete",
//     not "local execution finished".
//   - executing: Tau has started executing the requested tool locally. This phase
//     is emitted through AgentEventToolProgress/typed progress wiring in later
//     subtasks when execution starts are separated from provider call starts.
//   - completed: Tau has finished local execution. The actual tool output is
//     emitted separately as ToolResultEvent so call metadata and result content
//     remain distinct. This phase is emitted before/alongside ToolResultEvent
//     only after downstream consumers support the canonical contract.
type ToolLifecyclePhase string

const (
	ToolLifecycleRequested ToolLifecyclePhase = "requested"
	ToolLifecycleFinalized ToolLifecyclePhase = "finalized"
	ToolLifecycleExecuting ToolLifecyclePhase = "executing"
	ToolLifecycleCompleted ToolLifecyclePhase = "completed"
)

// ToolLifecycleSource records whether Tau received this lifecycle transition
// directly from a provider stream or inferred it from the final assistant
// message / agent state. Providers differ in stream symmetry; preserving this
// marker lets consumers and tests distinguish native events from normalization.
type ToolLifecycleSource string

const (
	ToolLifecycleSourceNative   ToolLifecycleSource = "native"
	ToolLifecycleSourceInferred ToolLifecycleSource = "inferred"
)

// ToolLifecycleEvent is the canonical typed payload for tool-call lifecycle
// events emitted by the agent. It replaces ad hoc map payloads while preserving
// legacy compatibility through LegacyMap during migration.
//
// CallID is the stable correlation key and must be used instead of ToolName for
// matching starts, completions, progress, and results. ToolName is display and
// dispatch metadata only; repeated or concurrent calls can share the same name.
//
// ArgsJSON carries raw, valid, complete JSON arguments when they are available
// and safe for the current consumer path. Do not store partial provider argument
// fragments in ArgsJSON because json.RawMessage must be valid JSON when the
// payload is marshaled; use ArgsSummary for partial/pending display instead.
// ArgsSummary is the sanitized, display-oriented compact summary. ArgsComplete
// indicates whether ArgsJSON/ArgsSummary represent final arguments or a
// partial/pending view.
type ToolLifecycleEvent struct {
	CallID       string              `json:"call_id"`
	ToolName     string              `json:"tool"`
	Phase        ToolLifecyclePhase  `json:"phase"`
	Source       ToolLifecycleSource `json:"source"`
	ArgsJSON     json.RawMessage     `json:"args,omitempty"`
	ArgsSummary  string              `json:"args_summary,omitempty"`
	ArgsComplete bool                `json:"args_complete"`
}

// LegacyMap returns the historical map payload shape used by older SDK/TUI
// consumers. New code should consume ToolLifecycleEvent directly.
func (e ToolLifecycleEvent) LegacyMap() map[string]any {
	m := map[string]any{
		"id":           e.CallID,
		"tool":         e.ToolName,
		"phase":        string(e.Phase),
		"source":       string(e.Source),
		"argsComplete": e.ArgsComplete,
	}
	if len(e.ArgsJSON) > 0 {
		// Historical AgentEventToolExecEnd payloads exposed args as a JSON string.
		// Keep that exact shape for map consumers and add canonical fields below.
		m["args"] = string(e.ArgsJSON)
		m["args_json"] = string(e.ArgsJSON)
	}
	if e.ArgsSummary != "" {
		m["argsSummary"] = e.ArgsSummary
	}
	return m
}

// ToolProgressEvent is the canonical typed payload for in-flight tool progress.
type ToolProgressEvent struct {
	CallID   string `json:"call_id"`
	ToolName string `json:"tool"`
	Message  string `json:"message,omitempty"`
}

// LegacyMap returns the historical map payload shape used by older consumers.
func (e ToolProgressEvent) LegacyMap() map[string]any {
	m := map[string]any{
		"id":   e.CallID,
		"tool": e.ToolName,
	}
	if e.Message != "" {
		m["message"] = e.Message
	}
	return m
}

// ToolResultEvent is the canonical typed payload for produced tool results.
// Result content is intentionally separate from ToolLifecycleEvent so compact
// call metadata and potentially large result output have different policies.
type ToolResultEvent struct {
	CallID   string `json:"call_id"`
	ToolName string `json:"tool"`
	IsError  bool   `json:"is_error"`
	Content  string `json:"content"`
}

// LegacyMap returns the historical map payload shape used by older consumers.
func (e ToolResultEvent) LegacyMap() map[string]any {
	return map[string]any{
		"id":      e.CallID,
		"tool":    e.ToolName,
		"isError": e.IsError,
		"content": e.Content,
	}
}

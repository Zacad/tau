// Package tui provides a bubbletea-based terminal user interface for tau.
package tui

import (
	"github.com/adam/tau/internal/types"
)

// AgentEventMsg wraps a types.AgentEvent for bubbletea's message system.
// It is produced by the bridge Cmd() and dispatched to Model.Update().
type AgentEventMsg struct {
	Event types.AgentEvent
}

// ErrorMsg signals an error during the agent loop.
type ErrorMsg struct {
	Err error
}

// PromptDoneMsg signals that Session.Prompt() returned successfully.
type PromptDoneMsg struct {
	Interrupted bool
}

// ContinueDoneMsg signals that Session.Continue() returned successfully.
type ContinueDoneMsg struct{}

// UserSubmitMsg carries text the user submitted via Enter.
type UserSubmitMsg struct {
	Text string
}

// QuitMsg signals a request to exit the application.
type QuitMsg struct{}

// TuiTickMsg fires periodically during streaming to keep the UI responsive.
type TuiTickMsg struct{}

// AutoScrollMsg fires periodically during mouse selection drag to scroll the viewport.
type AutoScrollMsg struct{}

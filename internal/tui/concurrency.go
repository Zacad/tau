package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/adam/tau/internal/types"
)

// Bridge decouples synchronous SDK event callbacks from the bubbletea event
// loop using a buffered channel with non-blocking push.
//
// This prevents deadlocks: Subscribe() callbacks fire synchronously on the
// agent goroutine (while s.mu is held during Prompt/Continue). The bridge's
// Push() is a non-blocking select, so it never blocks the agent loop. The
// tea.Cmd reads from the channel on the bubbletea goroutine where no SDK
// locks exist.
type Bridge struct {
	ch chan types.AgentEvent
}

// NewBridge creates a bridge with the given buffer size.
func NewBridge(bufSize int) *Bridge {
	return &Bridge{
		ch: make(chan types.AgentEvent, bufSize),
	}
}

// Push enqueues an event with non-blocking semantics.
// If the channel is full, the event is dropped to preserve liveness.
// Safe to call from any goroutine (especially the agent goroutine).
func (b *Bridge) Push(event types.AgentEvent) {
	select {
	case b.ch <- event:
	default:
		// Channel full — drop event (prefer liveness over completeness)
	}
}

// Cmd returns a tea.Cmd that blocks until an event is available on the
// channel, then returns it as an AgentEventMsg.
//
// Each call produces a fresh Cmd; after the Cmd fires, call Cmd() again
// to wait for the next event.
func (b *Bridge) Cmd() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-b.ch
		if !ok {
			// Channel closed — signal no more events
			return nil
		}
		return AgentEventMsg{Event: event}
	}
}

// Close closes the underlying channel. After Close, Push will panic and
// Cmd will return nil. Call once when the session is done.
func (b *Bridge) Close() {
	close(b.ch)
}

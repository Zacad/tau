package agent

import (
	"sync/atomic"

	"github.com/adam/tau/internal/types"
)

type listenerEntry struct {
	id int64
	fn func(types.AgentEvent)
}

var listenerIDSeq int64

// Subscribe registers a listener function that will be called for every
// agent event. Returns an unsubscribe function.
//
// Listeners receive canonical typed tool payloads. Use AgentEvent.LegacyData()
// inside the listener if you need the pre-064 map[string]any tool payload shape.
//
// Listeners are called synchronously on the emitting goroutine.
// Long-running listeners will block the agent loop.
func (a *Agent) Subscribe(listener func(types.AgentEvent)) func() {
	id := atomic.AddInt64(&listenerIDSeq, 1)

	a.mu.Lock()
	a.listeners = append(a.listeners, listenerEntry{id: id, fn: listener})
	a.mu.Unlock()

	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for i, e := range a.listeners {
			if e.id == id {
				a.listeners = append(a.listeners[:i], a.listeners[i+1:]...)
				return
			}
		}
	}
}

// emit sends an event to all registered listeners.
func (a *Agent) emit(event types.AgentEvent) {
	a.mu.RLock()
	// Snapshot under read lock to avoid holding lock during callbacks
	snap := make([]func(types.AgentEvent), len(a.listeners))
	for i, e := range a.listeners {
		snap[i] = e.fn
	}
	a.mu.RUnlock()

	for _, fn := range snap {
		fn(event)
	}
}

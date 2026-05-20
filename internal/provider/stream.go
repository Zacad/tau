package provider

import (
	"context"

	"github.com/adam/tau/internal/types"
)

// streamToChannel safely sends events to a channel and closes it when done.
// It respects context cancellation.
func streamToChannel(ctx context.Context, events []types.StreamEvent) <-chan types.StreamEvent {
	ch := make(chan types.StreamEvent, len(events)+1)
	go func() {
		defer close(ch)
		for _, e := range events {
			select {
			case <-ctx.Done():
				ch <- types.StreamEvent{
					Type:  types.EventError,
					Error: ctx.Err().Error(),
				}
				return
			case ch <- e:
			}
		}
	}()
	return ch
}

// sendEvent sends a single event to a channel if the context is not cancelled.
func sendEvent(ctx context.Context, ch chan<- types.StreamEvent, event types.StreamEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- event:
		return true
	}
}

// closeWithError sends an error event and closes the channel.
func closeWithError(ch chan<- types.StreamEvent, err error) {
	ch <- types.StreamEvent{
		Type:  types.EventError,
		Error: err.Error(),
	}
	close(ch)
}

// collectStream consumes a stream channel and returns all events.
// Useful for testing and for the Complete() method.
func collectStream(ch <-chan types.StreamEvent) []types.StreamEvent {
	var events []types.StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	return events
}

package types

// LegacyData returns an event payload in the historical map shape for tool
// lifecycle events. Non-tool and unknown payloads are returned unchanged.
//
// New consumers should use AgentEvent.Data's typed payloads directly. LegacyData
// exists as a migration shim for older SDK subscribers that asserted
// map[string]any for tool events.
func (e AgentEvent) LegacyData() any {
	switch data := e.Data.(type) {
	case ToolLifecycleEvent:
		return data.LegacyMap()
	case ToolProgressEvent:
		return data.LegacyMap()
	case ToolResultEvent:
		return data.LegacyMap()
	default:
		return e.Data
	}
}

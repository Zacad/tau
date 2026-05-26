package tui

import "github.com/adam/tau/internal/types"

func toolLifecycleDisplayData(payload any) (name, args, callID string) {
	name = "…"
	switch data := payload.(type) {
	case types.ToolLifecycleEvent:
		if data.ToolName != "" {
			name = data.ToolName
		}
		callID = data.CallID
		if data.ArgsSummary != "" {
			args = data.ArgsSummary
		} else if len(data.ArgsJSON) > 0 {
			args = string(data.ArgsJSON)
		}
	case map[string]any:
		if n, ok := data["tool"].(string); ok && n != "" {
			name = n
		}
		if id, ok := data["id"].(string); ok {
			callID = id
		}
		if a, ok := data["args"].(string); ok {
			args = a
		}
		if args == "" {
			if summary, ok := data["argsSummary"].(string); ok {
				args = summary
			}
		}
	case string:
		if data != "" {
			name = data
		}
	}
	return name, args, callID
}

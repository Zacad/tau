package subagent

import (
	"strings"

	"github.com/adam/tau/internal/types"
)

func toolResultText(result *ToolCallResult) string {
	if result == nil || result.Result == nil {
		return ""
	}
	var parts []string
	for _, block := range result.Result.Content {
		if block.Type == types.BlockText && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

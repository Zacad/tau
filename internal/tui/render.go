package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// messageBlock is one unit of renderable content in the viewport.
// Width is passed to render functions, not stored here (changes on resize).
type messageBlock struct {
	kind blockType
	text string // for text, thinking, error, subagent ID

	// renderedMarkdown caches the glamour-rendered output for assistant text blocks.
	// Empty means not yet rendered (pending/streaming) or cache invalidated by resize.
	renderedMarkdown string

	// isFinalized is true for blocks that have been flushed from the pending builder.
	// When true and renderedMarkdown is empty, re-render through glamour (e.g. after resize).
	// When false, always use plain text rendering (streaming/pending blocks).
	isFinalized bool

	// Tool call fields — only populated when kind == blockToolCall.
	toolName string
	toolArgs string
	toolSt   toolStatus
	toolErr  string // truncated error output

	// Tool result fields — only populated when kind == blockToolResult.
	toolResultName    string
	toolResultContent string
	toolResultIsError bool
}

// toolStatus tracks a tool call's execution phase.
type toolStatus int

const (
	toolPending toolStatus = iota
	toolSuccess
	toolError
)

// Truncation limits for tool call display.
const (
	toolArgsMaxLen      = 120
	toolErrMaxLen       = 200
	toolResultMaxLen    = 300
)

// blockType categorizes what kind of content a block holds.
type blockType int

const (
	blockUserMessage blockType = iota
	blockAssistantText
	blockThinking
	blockToolCall
	blockToolResult
	blockTurnSeparator
	blockError
	blockSubAgentStart
	blockSubAgentEnd
	blockQueuedMessage
)

// renderBlocks renders a slice of messageBlocks into a single styled string.
func renderBlocks(blocks []messageBlock, width int) string {
	var buf strings.Builder
	for i := range blocks {
		s := renderBlock(&blocks[i], width)
		if s != "" {
			buf.WriteString(s)
			buf.WriteString("\n")
		}
	}
	return buf.String()
}

// renderBlock renders a single messageBlock into a styled string.
func renderBlock(b *messageBlock, width int) string {
	switch b.kind {
	case blockUserMessage:
		return renderUserMessage(b.text, width)
	case blockAssistantText:
		return renderAssistantText(b, width)
	case blockThinking:
		return renderThinkingBlock(b.text, width)
	case blockToolCall:
		return renderToolCallBlock(*b, width)
	case blockToolResult:
		return renderToolResultBlock(*b, width)
	case blockTurnSeparator:
		return renderTurnSeparator(width)
	case blockError:
		return renderError(b.text, width)
	case blockSubAgentStart:
		return renderSubAgentStart(b.text, width)
	case blockSubAgentEnd:
		return renderSubAgentEnd(b.text, width)
	case blockQueuedMessage:
		return renderQueuedMessage(b.text, width)
	default:
		return fmt.Sprintf("[unknown block: %d]", b.kind)
	}
}

// renderUserMessage renders a user message with distinct styling.
func renderUserMessage(text string, width int) string {
	if text == "" {
		return ""
	}
	prefix := userPrefixStyle.Render("▸ ")
	content := userTextStyle.Render(text)
	return messagePaddingStyle.Width(width).Render(prefix + content)
}

// renderAssistantText renders assistant text with darker background for distinction.
// If renderedMarkdown is populated (finalized block), uses the cached glamour output.
// Otherwise renders as plain text (streaming/pending block).
// When cacheDirty is true, re-renders through glamour with the new width and updates the cache.
func renderAssistantText(b *messageBlock, width int) string {
	if b.text == "" {
		return ""
	}
	// Finalized block with cached glamour output — return as-is
	if b.isFinalized && b.renderedMarkdown != "" {
		return b.renderedMarkdown
	}
	// Finalized block with empty cache (e.g. after resize) — re-render through glamour
	if b.isFinalized {
		rendered := RenderMarkdown(b.text, width)
		if rendered != "" {
			b.renderedMarkdown = rendered
			return rendered
		}
	}
	// Pending/streaming block — plain text
	return assistantBlockStyle.Width(width).Render(b.text)
}

// renderThinkingBlock renders a thinking/reasoning block in dimmed style
// with a header label so it's visually distinct from the assistant response.
func renderThinkingBlock(text string, width int) string {
	if text == "" {
		return ""
	}
	header := thinkingTextStyle.Render("· thinking")
	content := thinkingTextStyle.Render(text)
	return messagePaddingStyle.Width(width).Render(header + "\n" + content)
}

// renderToolCallBlock renders a tool call with name, args, and status.
func renderToolCallBlock(b messageBlock, width int) string {
	name := b.toolName
	if name == "" {
		name = "…"
	}

	var result string
	switch b.toolSt {
	case toolPending:
		prefix := toolCallPendingStyle.Render("⏳")
		nameRendered := toolCallNameStyle.Render(name)
		var argPart string
		if b.toolArgs != "" {
			argPart = toolCallArgsStyle.Render(" " + truncate(b.toolArgs, toolArgsMaxLen))
		}
		result = prefix + " " + nameRendered + argPart

	case toolSuccess:
		prefix := toolCallSuccessStyle.Render("✓")
		nameRendered := toolCallNameStyle.Render(name)
		var parts []string
		parts = append(parts, prefix+" "+nameRendered)
		if b.toolArgs != "" {
			argsRendered := toolCallArgsStyle.Render("  " + formatToolArgs(b.toolName, b.toolArgs))
			parts = append(parts, argsRendered)
		}
		result = strings.Join(parts, "\n")

	case toolError:
		prefix := toolCallErrorStyle.Render("✗")
		nameRendered := toolCallNameStyle.Render(name)
		var parts []string
		parts = append(parts, prefix+" "+nameRendered)
		if b.toolErr != "" {
			errText := toolCallErrStyle.Render("  " + truncate(b.toolErr, toolErrMaxLen))
			parts = append(parts, errText)
		}
		result = strings.Join(parts, "\n")

	default:
		result = toolCallStyle.Render("⚙ " + name)
	}
	return messagePaddingStyle.Width(width).Render(result)
}

// renderToolResultBlock renders a tool result with output preview.
func renderToolResultBlock(b messageBlock, width int) string {
	var result string
	if b.toolResultIsError {
		prefix := toolResultErrStyle.Render("↳")
		nameRendered := toolResultNameStyle.Render(b.toolResultName)
		content := toolCallErrStyle.Render(truncate(b.toolResultContent, toolResultMaxLen))
		result = prefix + " " + nameRendered + "\n  " + content
	} else {
		prefix := toolResultStyle.Render("↳")
		nameRendered := toolResultNameStyle.Render(b.toolResultName)
		content := toolResultContentStyle.Render(truncate(b.toolResultContent, toolResultMaxLen))
		result = prefix + " " + nameRendered + "\n  " + content
	}
	return messagePaddingStyle.Render(result)
}

// renderTurnSeparator renders a subtle horizontal rule.
func renderTurnSeparator(width int) string {
	if width <= 0 {
		return ""
	}
	return turnSeparatorStyle.Width(width).Render(strings.Repeat("─", max(0, width-2)))
}

// renderError renders an error message in red/bold.
func renderError(text string, width int) string {
	if text == "" {
		return ""
	}
	// Strip internal error prefixes to show user-friendly message.
	prefixes := []string{
		"provider stream error: ",
		"agent prompt: ",
		"agent prompt failed: ",
	}
	for {
		stripped := false
		for _, prefix := range prefixes {
			if strings.HasPrefix(text, prefix) {
				text = text[len(prefix):]
				stripped = true
				break
			}
		}
		if !stripped {
			break
		}
	}
	return messagePaddingStyle.Width(width).Render(errorTextStyle.Render("✖ " + text))
}

// renderSubAgentStart renders a subagent starting indicator.
func renderSubAgentStart(id string, width int) string {
	if id == "" {
		return ""
	}
	return messagePaddingStyle.Width(width).Render(subAgentStyle.Render("  ↳ subagent " + id + " starting…"))
}

// renderSubAgentEnd renders a subagent finished indicator.
func renderSubAgentEnd(id string, width int) string {
	if id == "" {
		return ""
	}
	return messagePaddingStyle.Width(width).Render(subAgentStyle.Render("  ↳ subagent " + id + " finished"))
}

// truncate shortens s to maxlen, appending "…" if truncated.
func truncate(s string, maxlen int) string {
	if len(s) <= maxlen {
		return s
	}
	return s[:maxlen] + "…"
}

// formatToolArgs converts raw JSON tool arguments into a human-readable string.
// Known tools get special formatting; unknown tools fall back to clean key: value.
func formatToolArgs(toolName, argsJSON string) string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return truncate(argsJSON, toolArgsMaxLen)
	}

	// Tool-specific formatters
	switch toolName {
	case "websearch":
		return formatWebsearchArgs(raw)
	case "read":
		return formatReadArgs(raw)
	case "write":
		return formatWriteArgs(raw)
	case "edit":
		return formatEditArgs(raw)
	case "bash":
		return formatBashArgs(raw)
	case "grep":
		return formatGrepArgs(raw)
	case "find":
		return formatFindArgs(raw)
	case "ls":
		return formatLsArgs(raw)
	case "webfetch":
		return formatWebfetchArgs(raw)
	}

	// Fallback: clean key: value list
	return formatKeyValueArgs(raw)
}

func formatWebsearchArgs(raw map[string]any) string {
	if query, ok := raw["query"].(string); ok && query != "" {
		return "query: " + truncate(query, toolArgsMaxLen)
	}
	if queries, ok := raw["queries"].([]any); ok && len(queries) > 0 {
		qs := make([]string, 0, len(queries))
		for _, q := range queries {
			if s, ok := q.(string); ok {
				qs = append(qs, truncate(s, 60))
			}
		}
		return strings.Join(qs, ", ")
	}
	return formatKeyValueArgs(raw)
}

func formatReadArgs(raw map[string]any) string {
	if path, ok := raw["path"].(string); ok {
		parts := []string{"path: " + path}
		if offset, ok := raw["offset"]; ok {
			parts = append(parts, fmt.Sprintf("offset: %v", offset))
		}
		if limit, ok := raw["limit"]; ok {
			parts = append(parts, fmt.Sprintf("limit: %v", limit))
		}
		return strings.Join(parts, "  ")
	}
	return formatKeyValueArgs(raw)
}

func formatWriteArgs(raw map[string]any) string {
	if path, ok := raw["path"].(string); ok {
		return "path: " + path
	}
	return formatKeyValueArgs(raw)
}

func formatEditArgs(raw map[string]any) string {
	if path, ok := raw["path"].(string); ok {
		return "path: " + path
	}
	return formatKeyValueArgs(raw)
}

func formatBashArgs(raw map[string]any) string {
	if cmd, ok := raw["command"].(string); ok {
		return truncate(cmd, toolArgsMaxLen)
	}
	return formatKeyValueArgs(raw)
}

func formatGrepArgs(raw map[string]any) string {
	parts := []string{}
	if pattern, ok := raw["pattern"].(string); ok {
		parts = append(parts, "pattern: "+truncate(pattern, 60))
	}
	if path, ok := raw["path"].(string); ok {
		parts = append(parts, "path: "+path)
	}
	if len(parts) > 0 {
		return strings.Join(parts, "  ")
	}
	return formatKeyValueArgs(raw)
}

func formatFindArgs(raw map[string]any) string {
	parts := []string{}
	if name, ok := raw["name"].(string); ok {
		parts = append(parts, "name: "+truncate(name, 60))
	}
	if path, ok := raw["path"].(string); ok {
		parts = append(parts, "path: "+path)
	}
	if len(parts) > 0 {
		return strings.Join(parts, "  ")
	}
	return formatKeyValueArgs(raw)
}

func formatLsArgs(raw map[string]any) string {
	if path, ok := raw["path"].(string); ok {
		return "path: " + path
	}
	return formatKeyValueArgs(raw)
}

func formatWebfetchArgs(raw map[string]any) string {
	if url, ok := raw["url"].(string); ok {
		return truncate(url, toolArgsMaxLen)
	}
	return formatKeyValueArgs(raw)
}

func formatKeyValueArgs(raw map[string]any) string {
	// Sort keys for deterministic output
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[i] > keys[j] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}

	var parts []string
	for _, k := range keys {
		v := raw[k]
		val := fmt.Sprintf("%v", v)
		// Truncate long values
		if len(val) > 80 {
			val = val[:80] + "…"
		}
		parts = append(parts, k+": "+val)
	}
	return strings.Join(parts, "  ")
}

func renderQueuedMessage(text string, width int) string {
	if text == "" {
		return ""
	}
	display := text
	if len(display) > 80 {
		display = display[:80] + "…"
	}
	prefix := queuedMessageStyle.Render("[queued] ")
	content := queuedMessageStyle.Render(display)
	return messagePaddingStyle.Width(width).Render(prefix + content)
}

package tui

import "charm.land/lipgloss/v2"

// Global styles used across the TUI.
var (
	// Header style: bold, with a bottom border.
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true)

	// Footer style: dimmed text, with top border.
	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true)

	// Viewport style: no special styling, delegates to content.
	viewportStyle = lipgloss.NewStyle()

	// User message prefix style.
	userPrefixStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81")).
			Bold(true)

	// User message text style.
	userTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	// Assistant message text style.
	assistantTextStyle = lipgloss.NewStyle()

	// Thinking text style: dimmed, italic.
	thinkingTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242")).
				Italic(true)

	// Tool call style: cyan (legacy, superseded by specific tool styles).
	toolCallStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86"))

	// Tool call pending: spinner icon + cyan name.
	toolCallPendingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("220"))

	// Tool call success: green check.
	toolCallSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("46"))

	// Tool call error: red X.
	toolCallErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	// Tool call name: bold cyan for visibility.
	toolCallNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("86")).
				Bold(true)

	// Tool call args: dimmed.
	toolCallArgsStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242"))

	// Tool call error output: dimmed red.
	toolCallErrStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("160"))

	// Tool result prefix: dimmed.
	toolResultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	// Tool result error prefix: dimmed red.
	toolResultErrStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("160"))

	// Tool result name: dimmed cyan.
	toolResultNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("72"))

	// Tool result content: dimmed.
	toolResultContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("246"))

	// Turn separator style: subtle horizontal rule.
	turnSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("238"))

	// Error text style: red, bold.
	errorTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	// Subagent style: dimmed, italic.
	subAgentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242")).
			Italic(true)

	// Input separator: thin border above the textarea.
	inputSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	// Assistant block: darker background for model answers.
	assistantBlockStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("234")).
				Padding(1, 1)

	// Message padding: one line top/bottom spacing for all message blocks.
	messagePaddingStyle = lipgloss.NewStyle().
				Padding(1, 0)

	// Spinner style: cyan color for visibility.
	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81"))

	// Context usage warning: yellow for >70% context usage.
	contextWarningStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("220"))

	// Context usage error: red for >90% context usage.
	contextErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	// Queued message style: dimmed yellow prefix with truncated content.
	queuedMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242")).
				Italic(true)

	// Multi-step command indicator: cyan arrow.
	multiStepIndicatorStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("81")).
					Bold(true)

	paletteOverlayStyle = lipgloss.NewStyle().
				Padding(0)

	paletteDimStyle = lipgloss.NewStyle().
				Faint(true)

	paletteBoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1, 2)

	paletteCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("81")).
				Bold(true)

	paletteNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15"))

	paletteSelectedNameStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("15")).
					Bold(true)

	paletteDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242"))

	paletteSearchStyle = lipgloss.NewStyle().
				Padding(0, 1)

	paletteDisabledNameStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("238"))

	paletteDisabledDescStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("236"))

	paletteDisabledTagStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("236")).
					Italic(true)

	// Selection highlight style.
	selectionHighlightStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("63")).
					Foreground(lipgloss.Color("15"))
)

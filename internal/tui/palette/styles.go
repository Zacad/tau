package palette

import "charm.land/lipgloss/v2"

var (
	CursorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81")).
			Bold(true)

	NameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15"))

	SelectedNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Bold(true)

	DescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	SearchStyle = lipgloss.NewStyle().
			Padding(0, 1)

	DisabledNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("238"))

	DisabledDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("236"))

	DisabledTagStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("236")).
				Italic(true)

	BoxStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1, 2)

	InputLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Bold(true)

	InputBoxStyle = lipgloss.NewStyle().
			Padding(0, 1)

	InputValueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("81"))

	ConfirmYesStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Bold(true)

	ConfirmNoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	ConfirmYesSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).
				Bold(true).
				Underline(true)

	ConfirmNoSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15")).
				Bold(true).
				Underline(true)

	MessageTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("81")).
				Bold(true)

	MessageBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("15"))

	MessageHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242")).
				Italic(true)

	CategoryHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("81")).
				Bold(true).
				MarginTop(1).
				MarginBottom(1)
)

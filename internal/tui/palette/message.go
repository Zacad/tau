package palette

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type PaletteMessage struct {
	title     string
	message   string
	done      bool
	cancelled bool
	width     int
	height    int
}

func (p *PaletteMessage) Init(title, message string) {
	p.title = title
	p.message = message
	p.done = false
	p.cancelled = false
}

func (p *PaletteMessage) Update(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case tea.KeyPressMsg:
		switch msg.(tea.KeyPressMsg).String() {
		case "enter":
			p.done = true
			return nil
		case "esc":
			p.cancelled = true
			return nil
		}
	}
	return nil
}

func (p *PaletteMessage) View() string {
	boxWidth := min(p.width-8, 90)
	if boxWidth < 20 {
		boxWidth = 20
	}

	var lines []string

	if p.title != "" {
		lines = append(lines, MessageTitleStyle.Render(p.title))
		lines = append(lines, "")
	}

	lines = append(lines, MessageBodyStyle.Render(p.message))
	lines = append(lines, "")
	lines = append(lines, MessageHintStyle.Render("Press Enter to continue, Esc to cancel"))

	content := strings.Join(lines, "\n")
	return BoxStyle.Width(boxWidth).Render(content)
}

func (p *PaletteMessage) Done() bool { return p.done }

func (p *PaletteMessage) Cancelled() bool { return p.cancelled }

func (p *PaletteMessage) Result() string { return p.message }

func (p *PaletteMessage) SetSize(width, height int) {
	p.width = width
	p.height = height
}

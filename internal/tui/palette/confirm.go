package palette

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type PaletteConfirm struct {
	prompt    string
	confirmed bool
	done      bool
	cancelled bool
	result    bool
	width     int
	height    int
}

func (p *PaletteConfirm) Init(prompt string) {
	p.prompt = prompt
	p.done = false
	p.cancelled = false
	p.result = false
	p.confirmed = true
}

func (p *PaletteConfirm) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "y", "Y":
			p.result = true
			p.done = true
			return nil
		case "n", "N":
			p.result = false
			p.done = true
			return nil
		case "enter":
			p.result = p.confirmed
			p.done = true
			return nil
		case "esc":
			p.cancelled = true
			return nil
		}
	}
	return nil
}

func (p *PaletteConfirm) View() string {
	boxWidth := min(p.width-8, 90)
	if boxWidth < 20 {
		boxWidth = 20
	}

	var lines []string
	lines = append(lines, InputLabelStyle.Render(p.prompt))
	lines = append(lines, "")

	yesStyle := ConfirmYesStyle
	noStyle := ConfirmNoStyle
	if p.confirmed {
		yesStyle = ConfirmYesSelectedStyle
		noStyle = ConfirmNoStyle
	} else {
		yesStyle = ConfirmYesStyle
		noStyle = ConfirmNoSelectedStyle
	}

	promptLine := fmt.Sprintf("%s  %s", yesStyle.Render("[Y]es"), noStyle.Render("[N]o"))
	lines = append(lines, promptLine)

	content := strings.Join(lines, "\n")
	return BoxStyle.Width(boxWidth).Render(content)
}

func (p *PaletteConfirm) Done() bool { return p.done }

func (p *PaletteConfirm) Cancelled() bool { return p.cancelled }

func (p *PaletteConfirm) Result() bool { return p.result }

func (p *PaletteConfirm) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *PaletteConfirm) Toggle() {
	p.confirmed = !p.confirmed
}

func (p *PaletteConfirm) IsYes() bool {
	return p.confirmed
}

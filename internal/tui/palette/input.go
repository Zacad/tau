package palette

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"
)

type PaletteInput struct {
	label       string
	placeholder string
	input       textinput.Model
	done        bool
	cancelled   bool
	result      string
	width       int
	height      int
}

func (p *PaletteInput) Init(label, placeholder string) {
	p.label = label
	p.placeholder = placeholder
	p.done = false
	p.cancelled = false
	p.result = ""

	p.input = textinput.New()
	p.input.Placeholder = placeholder
	p.input.CharLimit = 256
	p.input.Focus()
}

func (p *PaletteInput) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			p.result = p.input.Value()
			p.done = true
			return nil
		case "esc":
			p.cancelled = true
			return nil
		}
	}

	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return cmd
}

func (p *PaletteInput) View() string {
	boxWidth := min(p.width-8, 90)
	if boxWidth < 20 {
		boxWidth = 20
	}

	p.input.SetWidth(boxWidth - 8)

	var lines []string

	labelLine := InputLabelStyle.Render(p.label)
	lines = append(lines, labelLine)
	lines = append(lines, "")
	lines = append(lines, InputBoxStyle.Render(p.input.View()))

	content := strings.Join(lines, "\n")
	return BoxStyle.Width(boxWidth).Render(content)
}

func (p *PaletteInput) Done() bool { return p.done }

func (p *PaletteInput) Cancelled() bool { return p.cancelled }

func (p *PaletteInput) Result() string { return p.result }

func (p *PaletteInput) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *PaletteInput) Value() string {
	return p.input.Value()
}

func (p *PaletteInput) SetValue(s string) {
	p.input.SetValue(s)
}

func (p *PaletteInput) Focus() {
	p.input.Focus()
}

func (p *PaletteInput) Blur() {
	p.input.Blur()
}

func renderInputResult(label, value string) string {
	boxWidth := 90
	var lines []string
	lines = append(lines, InputLabelStyle.Render(label))
	lines = append(lines, "")
	lines = append(lines, InputValueStyle.Render(fmt.Sprintf("  %s", value)))
	content := strings.Join(lines, "\n")
	return BoxStyle.Width(boxWidth).Render(content)
}

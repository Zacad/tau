package palette

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

type TaskFunc func() (success bool, message string, err error)

type PaletteTask struct {
	title      string
	task       TaskFunc
	spinner    spinner.Model
	spinnerCmd tea.Cmd
	done       bool
	cancelled  bool
	success    bool
	resultMsg  string
	err        error
	width      int
	height     int
}

func (p *PaletteTask) Init(title string, task TaskFunc) tea.Cmd {
	p.title = title
	p.task = task
	p.done = false
	p.cancelled = false
	p.success = false
	p.resultMsg = ""
	p.err = nil

	p.spinner = spinner.New(spinner.WithSpinner(spinner.Dot))
	var cmd tea.Cmd
	p.spinner, cmd = p.spinner.Update(p.spinner.Tick())
	p.spinnerCmd = cmd

	taskFn := p.task
	return tea.Batch(cmd, func() tea.Msg {
		success, message, err := taskFn()
		return TaskResultMsg{
			Success: success,
			Message: message,
			Err:     err,
		}
	})
}

func (p *PaletteTask) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			p.cancelled = true
			return nil
		case "enter":
			if p.done {
				return nil
			}
		}
	case TaskResultMsg:
		p.success = msg.Success
		p.resultMsg = msg.Message
		p.err = msg.Err
		p.done = true
		return nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		p.spinnerCmd = cmd
		return cmd
	}
	return nil
}

func (p *PaletteTask) View() string {
	boxWidth := min(p.width-8, 90)
	if boxWidth < 20 {
		boxWidth = 20
	}

	var lines []string

	if !p.done {
		lines = append(lines, spinnerStyle.Render(p.spinner.View())+" "+taskTitleStyle.Render(p.title+"..."))
	} else if p.err != nil {
		lines = append(lines, taskErrorStyle.Render("✗ "+p.title+": "+p.err.Error()))
	} else if p.success {
		lines = append(lines, taskSuccessStyle.Render("✓ "+p.resultMsg))
	} else {
		lines = append(lines, taskErrorStyle.Render("✗ "+p.resultMsg))
	}

	content := strings.Join(lines, "\n")
	return BoxStyle.Width(boxWidth).Render(content)
}

func (p *PaletteTask) Done() bool { return p.done }

func (p *PaletteTask) Cancelled() bool { return p.cancelled }

func (p *PaletteTask) Result() (success bool, message string, err error) {
	return p.success, p.resultMsg, p.err
}

func (p *PaletteTask) SetSize(width, height int) {
	p.width = width
	p.height = height
}

type TaskResultMsg struct {
	Success bool
	Message string
	Err     error
}

var (
	spinnerStyle    = InputLabelStyle
	taskTitleStyle  = InputLabelStyle
	taskSuccessStyle = ConfirmYesStyle
	taskErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func renderTaskResult(title string, success bool, message string, err error) string {
	boxWidth := 90
	var lines []string
	if err != nil {
		lines = append(lines, taskErrorStyle.Render("✗ "+title+": "+err.Error()))
	} else if success {
		lines = append(lines, taskSuccessStyle.Render("✓ "+message))
	} else {
		lines = append(lines, taskErrorStyle.Render("✗ "+message))
	}
	content := strings.Join(lines, "\n")
	return BoxStyle.Width(boxWidth).Render(content)
}

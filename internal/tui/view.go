package tui

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("")
	}

	var sections []string
	sections = append(sections, m.renderHeader())
	sections = append(sections, m.renderViewport())
	sections = append(sections, m.renderInput())
	sections = append(sections, m.renderFooter())

	content := strings.Join(sections, "\n")

	if m.paletteActive {
		paletteBox := m.palette.RenderBox(m.width, m.height)
		boxW := lipgloss.Width(paletteBox)
		boxH := lipgloss.Height(paletteBox)
		top := max(0, (m.height-boxH)/2)
		left := max(0, (m.width-boxW)/2)

		dimmedLines := strings.Split(content, "\n")
		boxLines := strings.Split(paletteBox, "\n")

		for i := range dimmedLines {
			dimmedLines[i] = paletteDimStyle.Render(dimmedLines[i])
		}

		var output []string
		for i := 0; i < m.height; i++ {
			if i >= top && i < top+len(boxLines) && i < len(dimmedLines) {
				bgLine := dimmedLines[i]
				fgLine := boxLines[i-top]
				output = append(output, joinOverlayLine(bgLine, fgLine, left, m.width))
			} else if i < len(dimmedLines) {
				output = append(output, dimmedLines[i])
			} else {
				output = append(output, "")
			}
		}

		v := tea.NewView(strings.Join(output, "\n"))
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
		return v
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) renderHeader() string {
	line := "tau │ " + m.modelProv + "/" + m.modelName + " │ " + m.cwd
	return headerStyle.Width(m.width).Render(line)
}

func (m *Model) renderViewport() string {
	return m.viewport.View()
}

func (m *Model) renderFooter() string {
	var pending int
	if m.session != nil {
		pending = m.session.PendingCount()
	}

	state := ""
	switch m.state {
	case stateIdle:
		if m.pendingExit {
			state = "idle (Ctrl+C again to exit)"
		} else if pending > 0 {
			state = fmt.Sprintf("idle (%d queued)", pending)
		} else {
			state = "idle"
		}
	case stateStreaming:
		if pending > 0 {
			if m.spinnerActive {
				state = fmt.Sprintf("%s working (%d queued)", spinnerStyle.Render(m.spinner.View()), pending)
			} else {
				state = fmt.Sprintf("working (%d queued)", pending)
			}
		} else {
			if m.spinnerActive {
				state = spinnerStyle.Render(m.spinner.View()) + " working"
			} else {
				state = "working"
			}
		}
	}

	// Build the info line: model | cwd | turns | tokens | cost | state
	var parts []string
	parts = append(parts, m.modelProv+"/"+m.modelName)

	// Show thinking level for reasoning-capable models
	if m.modelReasoning {
		parts = append(parts, fmt.Sprintf("thinking:%s", m.thinkingLevel))
	}

	parts = append(parts, m.cwd)
	parts = append(parts, fmt.Sprintf("turns:%d", m.turnCount))

	// Context usage display (matches PI footer style)
	if m.contextWindow > 0 && m.contextKnown {
		var ctxStr string
		if m.contextTokens == 0 && m.turnCount == 0 {
			ctxStr = fmt.Sprintf("ctx:0%%/%s", formatTokens(m.contextWindow))
		} else {
			pct := float64(m.contextTokens) / float64(m.contextWindow) * 100
			pctStr := fmt.Sprintf("%.1f%%", pct)
			ctxDisplay := fmt.Sprintf("ctx:%s/%s", pctStr, formatTokens(m.contextWindow))
			if pct > 90 {
				ctxStr = contextErrorStyle.Render(ctxDisplay)
			} else if pct > 70 {
				ctxStr = contextWarningStyle.Render(ctxDisplay)
			} else {
				ctxStr = ctxDisplay
			}
		}
		parts = append(parts, ctxStr)
	}

	if m.usage.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("tokens:%d", m.usage.TotalTokens))
		if m.usage.Cost.Total > 0 {
			parts = append(parts, fmt.Sprintf("$%.2f", m.usage.Cost.Total))
		} else if m.modelProv == "ollama" {
			parts = append(parts, "$0.00 (local)")
		}
	}

	parts = append(parts, state)
	line := strings.Join(parts, " │ ")
	return footerStyle.Width(m.width).Render(line)
}

func (m *Model) renderInput() string {
	input := m.input.View()
	return lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Padding(1, 1).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(m.width).
		Render(input)
}

func joinOverlayLine(bgLine, fgLine string, left, width int) string {
	bgVisible := stripANSI(bgLine)
	fgVisible := stripANSI(fgLine)

	if left >= len(bgVisible) {
		return bgLine
	}

	end := left + len(fgVisible)
	if end > len(bgVisible) {
		end = len(bgVisible)
	}

	leftPart := visibleSubstring(bgLine, 0, left)
	rightPart := visibleSubstring(bgLine, end, len(bgVisible))

	return leftPart + fgLine + rightPart
}

func visibleSubstring(s string, start, end int) string {
	if start >= end {
		return ""
	}

	var result strings.Builder
	visiblePos := 0
	inEscape := false

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		ch := runes[i]
		if ch == '\x1b' {
			inEscape = true
		}

		if inEscape {
			result.WriteRune(ch)
			if ch == 'm' {
				inEscape = false
			}
			continue
		}

		if visiblePos >= start && visiblePos < end {
			result.WriteRune(ch)
		}

		visiblePos++
	}

	return result.String()
}

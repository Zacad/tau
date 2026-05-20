package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"

	"github.com/adam/tau/internal/tui/palette"
)

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		for i := range m.blocks {
			if m.blocks[i].kind == blockAssistantText {
				m.blocks[i].renderedMarkdown = ""
			}
		}
		m.pendingRendered = ""
		m.pendingRenderedLen = 0
		m.invalidateRenderedCache()
		m.resize(msg.Width, msg.Height)
		m.updateViewport()
		m.viewport, _ = m.viewport.Update(msg)
		m.input, _ = m.input.Update(msg)
		if m.paletteActive {
			m.palette.width = msg.Width
			m.palette.height = msg.Height
		}
		return m, nil

	case AgentEventMsg:
		evtCmd := m.processEvent(msg.Event)
		m.updateViewport()
		var cmds []tea.Cmd
		if evtCmd != nil {
			cmds = append(cmds, evtCmd)
		}
		// Start debounce timer for streaming markdown rendering.
		if m.state == stateStreaming && m.pendingKind == blockAssistantText && m.pendingBuilder.Len() > 0 {
			cmds = append(cmds, tea.Every(renderDebounceInterval, func(t time.Time) tea.Msg {
				return TuiTickMsg{}
			}))
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case PromptDoneMsg:
		m.updateViewportWithForce(true)
		cmd = m.handlePromptDone(msg.Interrupted)
		return m, cmd

	case ErrorMsg:
		m.handleError(msg.Err)
		return m, nil

	case spinner.TickMsg:
		if m.spinnerActive {
			var tickCmd tea.Cmd
			m.spinner, tickCmd = m.spinner.Update(msg)
			m.spinnerTick = tickCmd
			return m, tickCmd
		}
		if m.paletteActive && m.palette.IsTaskStep() {
			cmd := m.palette.Update(msg)
			return m, cmd
		}
		if m.paletteActive && m.palette.IsMultiStep() {
			cmd := m.palette.Update(msg)
			return m, cmd
		}
		return m, nil

	case TuiTickMsg:
		// Periodic tick from tea.Every — keeps UI responsive during tool execution
		// and triggers debounced markdown re-rendering during streaming.
		// Debounced markdown re-render for streaming content.
		if m.state == stateStreaming && m.pendingKind == blockAssistantText {
			m.renderPendingMarkdown()
		}
		m.updateViewport()
		return m, nil

	case tea.QuitMsg:
		return m, tea.Quit

	case palette.TaskResultMsg:
		if m.paletteActive && m.palette.IsTaskStep() {
			m.palette.Update(msg)
			if m.palette.TaskDone() {
				if c := m.executePaletteTaskResult(); c != nil {
					return m, c
				}
			}
		}
		if m.paletteActive && m.palette.IsMultiStep() {
			m.palette.Update(msg)
			if m.palette.MultiStepTaskDone() {
				return m, m.handleMultiStepTaskComplete()
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		cmd = m.handleKeyPress(msg)
		// If we handled this key (returned a Cmd), don't delegate to sub-models.
		// This prevents the textarea from also processing Enter.
		if cmd != nil {
			return m, cmd
		}
	}

	// Route messages to palette-based multi-step runner.
	if m.paletteActive && m.palette.IsMultiStep() {
		cmd := m.palette.Update(msg)
		if m.palette.MultiStepTaskDone() {
			return m, m.handleMultiStepTaskComplete()
		}
		if m.palette.MultiStepListDone() || m.palette.MultiStepInputDone() ||
			m.palette.MultiStepConfirmDone() || m.palette.MultiStepMessageDone() {
			if m.palette.MultiStepDone() {
				return m, m.executePaletteMultiStepResult()
			}
			cmd := m.palette.HandleMultiStepDone()
			if m.palette.MultiStepDone() {
				return m, m.executePaletteMultiStepResult()
			}
			return m, cmd
		}
		if m.palette.MultiStepCancelled() {
			m.palette.Close()
			m.openPalette()
			return m, nil
		}
		return m, cmd
	}

	// Delegate unhandled messages to sub-models (normal typing, mouse, etc.)
	// Exit history mode if input value changes while browsing
	wasBrowsing := m.promptHistoryIndex != -1
	oldValue := m.input.Value()

	m.viewport, cmd = m.viewport.Update(msg)
	if cmd != nil {
		return m, cmd
	}

	// Mouse wheel events should only scroll the viewport, not the textarea.
	// Per requirements, prompt input scrolling is via arrow keys only.
	if _, isWheel := msg.(tea.MouseWheelMsg); !isWheel {
		m.input, cmd = m.input.Update(msg)
	}

	// Exit history mode on typing
	if wasBrowsing && m.input.Value() != oldValue {
		m.promptHistoryIndex = -1
	}
	// Open palette when input starts with "/"
	m.handleSlashPrefix()
	// Re-resize if textarea height changed (DynamicHeight)
	if h := m.input.Height(); h != m.lastInputHeight {
		m.lastInputHeight = h
		m.resize(m.width, m.height)
	}
	m.viewport, _ = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) resize(width, height int) {
	headerH := 2
	footerH := 2
	inputAreaH := m.input.Height() + 3 // border top + padding top + input lines + padding bottom
	availH := height - headerH - footerH - inputAreaH
	if availH < 1 {
		availH = 1
	}
	m.viewport.SetWidth(width)
	m.viewport.SetHeight(availH)
	m.input.SetWidth(width)
}

// handleKeyPress handles keys at the model level.
// Returns a Cmd when the key was handled, nil to let sub-models process it.
func (m *Model) handleKeyPress(msg tea.KeyPressMsg) tea.Cmd {
	if m.paletteActive {
		switch msg.String() {
		case "esc":
			if m.palette.IsMultiStep() {
				m.palette.CancelMultiStep()
				m.palette.Close()
				m.openPalette()
				return nil
			}
			m.palette.Close()
			m.paletteActive = false
			m.input.SetValue("")
			m.input.Focus()
			return nil
		case "up":
			if m.palette.IsMultiStep() {
				m.palette.Update(msg)
				return func() tea.Msg { return nil }
			}
			m.palette.Up()
			return nil
		case "down", "tab":
			if m.palette.IsMultiStep() {
				m.palette.Update(msg)
				return func() tea.Msg { return nil }
			}
			m.palette.Down()
			return nil
		case "enter":
			if m.palette.IsMultiStep() {
				return m.handleMultiStepEnter()
			}
			m.palette.Update(msg)
			if m.palette.ListDone() {
				if cmd := m.executePaletteSelection(); cmd != nil {
					return cmd
				}
				if m.paletteActive && m.palette.InputDone() {
					if c := m.executePaletteInputResult(); c != nil {
						return c
					}
					return func() tea.Msg { return nil }
				}
				if m.paletteActive && m.palette.ConfirmDone() {
					if c := m.executePaletteConfirmResult(); c != nil {
						return c
					}
					return func() tea.Msg { return nil }
				}
				if m.paletteActive && m.palette.TaskDone() {
					if c := m.executePaletteTaskResult(); c != nil {
						return c
					}
					return func() tea.Msg { return nil }
				}
				if m.paletteActive && m.palette.MessageDone() {
					if c := m.executePaletteMessageResult(); c != nil {
						return c
					}
					return func() tea.Msg { return nil }
				}
				return func() tea.Msg { return nil }
			}
			if m.palette.InputDone() {
				if c := m.executePaletteInputResult(); c != nil {
					return c
				}
				if m.paletteActive && m.palette.ConfirmDone() {
					if c := m.executePaletteConfirmResult(); c != nil {
						return c
					}
					return func() tea.Msg { return nil }
				}
				if m.paletteActive && m.palette.TaskDone() {
					if c := m.executePaletteTaskResult(); c != nil {
						return c
					}
					return func() tea.Msg { return nil }
				}
				if m.paletteActive && m.palette.MessageDone() {
					if c := m.executePaletteMessageResult(); c != nil {
						return c
					}
					return func() tea.Msg { return nil }
				}
				return func() tea.Msg { return nil }
			}
			if m.palette.ConfirmDone() {
				if c := m.executePaletteConfirmResult(); c != nil {
					return c
				}
				return func() tea.Msg { return nil }
			}
			if m.palette.TaskDone() {
				if c := m.executePaletteTaskResult(); c != nil {
					return c
				}
				return func() tea.Msg { return nil }
			}
			if m.palette.MessageDone() {
				if c := m.executePaletteMessageResult(); c != nil {
					return c
				}
				return func() tea.Msg { return nil }
			}
			return nil
		case "ctrl+p":
			m.palette.Close()
			m.paletteActive = false
			m.input.SetValue("")
			m.input.Focus()
			return nil
		default:
			if m.palette.IsMultiStep() {
				return m.handleMultiStepDefault(msg)
			}
			m.palette.Update(msg)
			if m.palette.TaskDone() {
				if c := m.executePaletteTaskResult(); c != nil {
					return c
				}
				return func() tea.Msg { return nil }
			}
			if m.palette.ConfirmDone() {
				if c := m.executePaletteConfirmResult(); c != nil {
					return c
				}
				return func() tea.Msg { return nil }
			}
			if m.palette.InputDone() {
				if c := m.executePaletteInputResult(); c != nil {
					return c
				}
				return func() tea.Msg { return nil }
			}
			if m.palette.MessageDone() {
				if c := m.executePaletteMessageResult(); c != nil {
					return c
				}
				return func() tea.Msg { return nil }
			}
			return nil
		}
	}

	// Clear pending exit on any key other than Ctrl+C
	if m.pendingExit && msg.String() != "ctrl+c" {
		m.pendingExit = false
	}

	switch msg.String() {
	case "enter":
		val := m.input.Value()
		if val == "" {
			return nil
		}
		m.input.SetValue("")
		m.input.SetCursorColumn(0)
		m.promptHistoryIndex = -1

	// Add to history before submitting
	_ = appendPromptHistory(m.cwd, val)

	if m.state == stateIdle {
		if handled, cmd := m.executeCommand(val); handled {
			if cmd == nil {
				return func() tea.Msg { return nil }
			}
			return cmd
		}
		return m.submitPrompt(val)
	}

	if m.state == stateStreaming {
		if handled, cmd := m.executeCommandStreaming(val); handled {
			if cmd == nil {
				return func() tea.Msg { return nil }
			}
			return cmd
		}
			m.session.EnqueueMessage(val)
			m.blocks = append(m.blocks, messageBlock{
				kind: blockQueuedMessage,
				text: val,
			})
			m.invalidateRenderedCache()
			m.updateViewport()
			// Return a no-op Cmd to prevent the textarea from also processing Enter.
			return func() tea.Msg { return nil }
		}

	case "shift+enter", "ctrl+j":
		m.input.InsertString("\n")
		return nil

	case "ctrl+d":
		if m.state == stateIdle {
			return tea.Quit
		}

	case "ctrl+c":
		// During streaming: abort the current turn
		if m.state == stateStreaming && m.cancelFunc != nil {
			m.cancelFunc()
			return nil
		}
		// When idle: Ctrl+C double-tap to exit
		if m.state == stateIdle {
			if m.pendingExit {
				return tea.Quit
			}
			m.pendingExit = true
			return nil
		}

	case "esc":
		// During streaming: abort the current LLM response
		if m.state == stateStreaming && m.cancelFunc != nil {
			m.cancelFunc()
			return nil
		}
		if m.state == stateIdle {
			m.input.SetValue("")
			return nil
		}

	case "ctrl+p":
		m.openPalette()
		return nil

	case "up":
		if len(m.promptHistory) == 0 {
			return nil
		}
		// Enter or advance history (older entries)
		if m.promptHistoryIndex == -1 {
			m.promptHistoryIndex = 0
		} else if m.promptHistoryIndex < len(m.promptHistory)-1 {
			m.promptHistoryIndex++
		}
		m.input.SetValue(m.promptHistory[m.promptHistoryIndex])
		m.input.SetCursorColumn(len(m.promptHistory[m.promptHistoryIndex]))
		return func() tea.Msg { return nil }

	case "down":
		if m.promptHistoryIndex == -1 {
			return nil
		}
		// Go back through history (newer entries)
		m.promptHistoryIndex--
		if m.promptHistoryIndex == -1 {
			m.input.SetValue("")
			m.input.SetCursorColumn(0)
		} else {
			m.input.SetValue(m.promptHistory[m.promptHistoryIndex])
			m.input.SetCursorColumn(len(m.promptHistory[m.promptHistoryIndex]))
		}
		return func() tea.Msg { return nil }

	case "tab":
		val := m.input.Value()
		if strings.HasPrefix(val, "/skill:") {
			skillPart := strings.TrimPrefix(val, "/skill:")
			completion := completeSkill(skillPart, m.session.Skills())
			if completion != "" {
				m.input.SetValue("/skill:" + completion)
				m.input.SetCursorColumn(len("/skill:" + completion))
			}
		} else if strings.HasPrefix(val, "/") {
			completion := completeCommand(val)
			if completion != "" {
				m.input.SetValue(completion)
				m.input.SetCursorColumn(len(completion))
			}
		}
		return nil
	}
	return nil
}

// executeCommandStreaming handles commands during streaming.
// Non-blocking commands execute now; blocking commands are ignored.
func (m *Model) executeCommandStreaming(input string) (bool, tea.Cmd) {
	name, args, isCmd := ParseCommandInput(input)
	if !isCmd {
		return false, nil
	}

	cmd := m.commandRegistry.Lookup(name)
	if cmd == nil {
		return false, nil
	}
	if !cmd.IsAvailable(m) {
		return false, nil
	}
	h := cmd.Handler()
	if h == nil {
		return true, nil
	}
	return true, h(m, args)
}

// handleSlashPrefix opens the palette when input starts with "/".
func (m *Model) handleSlashPrefix() {
	val := m.input.Value()
	if !strings.HasPrefix(val, "/") {
		return
	}
	if m.paletteActive {
		return
	}
	m.openPalette()
}

// completeCommand returns the best completion for a partial command input.
// Used for tab completion when dropdown is not active.
func completeCommand(input string) string {
	input = strings.TrimSpace(input)
	if input == "" || !strings.HasPrefix(input, "/") {
		return ""
	}

	var matches []string
	for _, cmd := range defaultCommands {
		if strings.HasPrefix(cmd, input) {
			matches = append(matches, cmd)
		}
	}

	if len(matches) == 0 {
		return ""
	}
	if len(matches) == 1 {
		return matches[0]
	}
	return longestCommonPrefix(matches)
}

// defaultCommands is the list of all available slash commands (for tab completion fallback).
var defaultCommands = []string{
	"/quit",
	"/exit",
	"/help",
	"/new",
	"/resume",
	"/name",
	"/session",
	"/model",
	"/compact",
	"/clear",
	"/skills",
	"/skill:",
}

// startMultiStep activates a multi-step command flow via the palette.
func (m *Model) startMultiStep(cmd *Command) tea.Cmd {
	stepsFn := cmd.MultiStep()
	if stepsFn == nil {
		return nil
	}
	steps := stepsFn(m)
	if len(steps) == 0 {
		return nil
	}

	m.multiStepCommandName = cmd.Name()
	m.input.Blur()
	m.paletteActive = true
	return m.palette.ShowSteps(steps, cmd.Name())
}

func (m *Model) openPalette() {
	var cmds []Command
	for i := range m.commandRegistry.commands {
		c := &m.commandRegistry.commands[i]
		if c.Type() == appCommand || c.Type() == appMultiStepCommand {
			cmds = append(cmds, *c)
		}
	}
	m.palette.Open(cmds)
	avail := make(map[string]bool)
	for i := range m.commandRegistry.commands {
		c := &m.commandRegistry.commands[i]
		if c.Type() == appCommand || c.Type() == appMultiStepCommand {
			avail[c.Name()] = c.IsAvailable(m)
		}
	}
	m.palette.SetAvailability(avail)
	m.paletteActive = true
	m.input.Blur()
}

func (m *Model) executePaletteSelection() tea.Cmd {
	if handler := m.palette.SelectionHandler(); handler != nil {
		item, idx := m.palette.ListResult()
		if item != nil {
			cmd := handler(item, idx)
			if m.palette.InputDone() {
				m.palette.Close()
				m.paletteActive = false
				m.input.SetValue("")
				m.input.Focus()
				if c := m.executePaletteInputResult(); c != nil {
					return c
				}
				return func() tea.Msg { return nil }
			}
			if m.palette.IsInputStep() {
				return cmd
			}
			// If handler transitioned to another step (e.g., task), keep palette open.
			if m.palette.IsTaskStep() || m.palette.IsMultiStep() || m.palette.IsMessageStep() || m.palette.IsConfirmStep() {
				return cmd
			}
			m.palette.Close()
			m.paletteActive = false
			m.input.SetValue("")
			m.input.Focus()
			return cmd
		}
		m.palette.Close()
		m.paletteActive = false
		m.input.SetValue("")
		m.input.Focus()
		return func() tea.Msg { return nil }
	}

	cmd := m.palette.Selected()
	if cmd == nil {
		return nil
	}

	if cmd.MultiStep() != nil {
		steps := cmd.MultiStep()(m)
		if len(steps) == 0 {
			m.palette.Close()
			m.paletteActive = false
			m.input.SetValue("")
			m.input.Focus()
			return func() tea.Msg { return nil }
		}
		m.paletteActive = true
		m.multiStepCommandName = cmd.Name()
		return m.palette.ShowSteps(steps, cmd.Name())
	}

	m.palette.Close()
	m.paletteActive = false
	m.input.SetValue("")
	m.input.Focus()

	if h := cmd.Handler(); h != nil {
		if c := h(m, ""); c != nil {
			return c
		}
	}
	return func() tea.Msg { return nil }
}

func (m *Model) executePaletteInputResult() tea.Cmd {
	result := m.palette.InputResult()

	if handler := m.palette.SelectionHandler(); handler != nil {
		if m.paletteConfirmPrompt != "" {
			m.paletteInputResult = result
			m.palette.ShowConfirm(m.paletteConfirmPrompt)
			m.paletteConfirmPrompt = ""
			return nil
		}
		m.palette.Close()
		m.paletteActive = false
		m.input.SetValue("")
		m.input.Focus()
		return nil
	}

	m.palette.Close()
	m.paletteActive = false
	m.input.SetValue("")
	m.input.Focus()

	m.blocks = append(m.blocks, messageBlock{
		kind: blockAssistantText,
		text: "You entered: " + result,
	})
	m.invalidateRenderedCache()
	m.updateViewport()
	return func() tea.Msg { return nil }
}

func (m *Model) executePaletteConfirmResult() tea.Cmd {
	confirmed := m.palette.ConfirmResult()

	if handler := m.palette.SelectionHandler(); handler != nil {
		if confirmed && m.paletteTaskFunc != nil {
			cmd := m.palette.ShowTask(m.paletteTaskTitle, m.paletteTaskFunc)
			m.paletteTaskFunc = nil
			m.paletteTaskTitle = ""
			return cmd
		}
		if !confirmed {
			m.blocks = append(m.blocks, messageBlock{
				kind: blockAssistantText,
				text: "Cancelled",
			})
			m.invalidateRenderedCache()
			m.updateViewport()
		}
		m.palette.Close()
		m.paletteActive = false
		m.input.SetValue("")
		m.input.Focus()
		return nil
	}

	m.palette.Close()
	m.paletteActive = false
	m.input.SetValue("")
	m.input.Focus()

	if confirmed {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "Text saved: " + m.paletteInputResult,
		})
	} else {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "Cancelled",
		})
	}
	m.paletteInputResult = ""
	m.invalidateRenderedCache()
	m.updateViewport()
	return func() tea.Msg { return nil }
}

func (m *Model) executePaletteTaskResult() tea.Cmd {
	success, message, err := m.palette.TaskResult()

	if handler := m.palette.SelectionHandler(); handler != nil {
		if err != nil {
			m.palette.ShowMessage("Error", err.Error())
		} else if success {
			if m.paletteTaskTitle == "Resuming session" {
				handleResumeComplete(m)
				m.palette.Close()
				m.paletteActive = false
				m.input.SetValue("")
				m.input.Focus()
				return nil
			}
			m.palette.ShowMessage("Success", message)
		} else {
			m.palette.ShowMessage("Failed", message)
		}
		return nil
	}

	m.palette.Close()
	m.paletteActive = false
	m.input.SetValue("")
	m.input.Focus()

	if err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: err.Error(),
		})
	} else if success {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: message,
		})
	} else {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: message,
		})
	}
	m.invalidateRenderedCache()
	m.updateViewport()
	return func() tea.Msg { return nil }
}

func (m *Model) executePaletteMessageResult() tea.Cmd {
	result := m.palette.MessageResult()
	hasHandler := m.palette.SelectionHandler() != nil
	m.palette.Close()
	m.paletteActive = false
	m.input.SetValue("")
	m.input.Focus()

	if hasHandler {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: result,
		})
		m.invalidateRenderedCache()
		m.updateViewport()
	}

	return func() tea.Msg { return nil }
}

func (m *Model) executePaletteMultiStepResult() tea.Cmd {
	results := m.palette.MultiStepResults()
	commandName := m.multiStepCommandName
	m.palette.Close()
	m.paletteActive = false
	m.input.SetValue("")
	m.input.Focus()

	return func() tea.Msg {
		switch commandName {
		case "connect":
			handleConnectResult(m, results)
		case "disconnect":
			handleDisconnectResult(m, results)
		}
		return nil
	}
}

func (m *Model) handleMultiStepEnter() tea.Cmd {
	// First, route Enter to the current component
	m.palette.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	// Then check which component completed and advance
	switch {
	case m.palette.MultiStepListDone():
		cmd := m.palette.HandleMultiStepDone()
		if m.palette.MultiStepDone() {
			return m.executePaletteMultiStepResult()
		}
		if cmd != nil {
			return cmd
		}
		return func() tea.Msg { return nil }
	case m.palette.MultiStepInputDone():
		if m.palette.MultiStepInputCancelled() {
			m.palette.CancelMultiStep()
			m.palette.Close()
			m.openPalette()
			return nil
		}
		cmd := m.palette.HandleMultiStepDone()
		if m.palette.MultiStepDone() {
			return m.executePaletteMultiStepResult()
		}
		if cmd != nil {
			return cmd
		}
		return func() tea.Msg { return nil }
	case m.palette.MultiStepConfirmDone():
		if m.palette.MultiStepConfirmCancelled() {
			m.palette.CancelMultiStep()
			m.palette.Close()
			m.openPalette()
			return nil
		}
		cmd := m.palette.HandleMultiStepDone()
		if m.palette.MultiStepDone() {
			return m.executePaletteMultiStepResult()
		}
		if cmd != nil {
			return cmd
		}
		return func() tea.Msg { return nil }
	case m.palette.MultiStepTaskDone():
		return func() tea.Msg { return nil }
	case m.palette.MultiStepMessageDone():
		if m.palette.MultiStepMessageCancelled() {
			m.palette.CancelMultiStep()
			m.palette.Close()
			m.openPalette()
			return nil
		}
		cmd := m.palette.HandleMultiStepDone()
		if m.palette.MultiStepDone() {
			return m.executePaletteMultiStepResult()
		}
		if cmd != nil {
			return cmd
		}
		return func() tea.Msg { return nil }
	default:
		return func() tea.Msg { return nil }
	}
}

func (m *Model) handleMultiStepDefault(msg tea.Msg) tea.Cmd {
	cmd := m.palette.Update(msg)
	if m.palette.MultiStepTaskDone() {
		return m.handleMultiStepTaskComplete()
	}
	if m.palette.MultiStepListDone() || m.palette.MultiStepInputDone() ||
		m.palette.MultiStepConfirmDone() || m.palette.MultiStepMessageDone() {
		if m.palette.MultiStepDone() {
			return m.executePaletteMultiStepResult()
		}
		cmd := m.palette.HandleMultiStepDone()
		if m.palette.MultiStepDone() {
			return m.executePaletteMultiStepResult()
		}
		if cmd != nil {
			return cmd
		}
		return func() tea.Msg { return nil }
	}
	if m.palette.MultiStepCancelled() {
		m.palette.Close()
		m.openPalette()
		return func() tea.Msg { return nil }
	}
	return cmd
}

func (m *Model) handleMultiStepTaskComplete() tea.Cmd {
	cmd := m.palette.HandleMultiStepDone()
	if m.palette.MultiStepDone() {
		return m.executePaletteMultiStepResult()
	}
	return cmd
}

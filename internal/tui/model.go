package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/adam/tau/internal/sdk"
	"github.com/adam/tau/internal/skills"
	"github.com/adam/tau/internal/types"
	"github.com/adam/tau/internal/tui/palette"
	"github.com/charmbracelet/glamour"
)

// bouncingDots is a custom spinner with a dot that moves across positions.
var bouncingDots = spinner.Spinner{
	Frames: []string{"∙∙∙", "●∙∙", "∙●∙", "∙∙●", "∙∙∙"},
	FPS:    time.Second / 8,
}

// modelState represents the current state of the TUI.
type modelState int

const (
	stateIdle    modelState = iota // Waiting for user input
	stateStreaming                 // Agent is responding
)

// Debounce interval for streaming markdown re-rendering.
const renderDebounceInterval = 200 * time.Millisecond

// Throttle interval for viewport updates during streaming (~30fps).
const viewportUpdateInterval = 33 * time.Millisecond

// Model is the bubbletea model for the tau TUI.
type Model struct {
	session *sdk.Session

	viewport viewport.Model
	input    textarea.Model

	width  int
	height int
	state  modelState

	// Cached session info — read once at startup to avoid
	// calling session methods (which acquire s.mu) from View(),
	// which would deadlock with the agent goroutine during Prompt().
	modelName      string
	modelProv      string // provider name (e.g. "ollama", "openai")
	modelReasoning bool   // whether current model supports thinking
	thinkingLevel  string // current thinking level (e.g. "off", "medium")
	cwd            string
	sessionID      string
	sessionName    string

	// program is the running bubbletea program, used to Send messages
	// from the agent goroutine directly to the event loop.
	program *tea.Program

	// Rendering state: finalized blocks + pending streaming builder.
	blocks         []messageBlock
	pendingBuilder *strings.Builder
	pendingKind    blockType

	// pendingToolIndex tracks the index of the in-progress tool call block.
	// Set to -1 when no tool call is pending.
	pendingToolIndex int

	// Debounced streaming markdown render state.
	pendingRendered    string                  // last glamour-rendered output of pending content
	pendingRenderedLen int                     // length of pendingBuilder when last rendered
	lastRenderTime     time.Time               // timestamp of last glamour render
	glamourRenderer    *glamour.TermRenderer   // reused renderer instance

	// Turn tracking.
	turnCount int

	// Cached usage from the last completed turn.
	usage types.Usage

	// Spinner for working indicator in footer.
	spinner       spinner.Model
	spinnerActive bool
	spinnerTick   tea.Cmd // cached tick command to keep spinner alive

	cancelFunc context.CancelFunc

	// pendingExit tracks a pending exit request (Ctrl+C double-tap).
	// When true, the next Ctrl+C while idle will call tea.Quit.
	pendingExit bool

	// Command registry
	commandRegistry *CommandRegistry

	// Command palette
	paletteActive bool
	palette       CommandPalette
	paletteInputResult string
	paletteConfirmPrompt string
	paletteTaskTitle string
	paletteTaskFunc palette.TaskFunc
	paletteMessageTitle string
	paletteMessageBody string

	// Multi-step command state
	multiStepCommandName string

	// Viewport update throttling during streaming.
	lastViewportUpdate time.Time

	// Finalized block render cache.
	renderedCache     string
	renderedCacheValid bool

	// Tracks pending builder length at last SetContent call.
	// Used to skip expensive SetContent() when pending content hasn't changed.
	lastSetContentPendingLen         int
	lastSetContentPendingRenderedLen int
	lastSetContentBlocksLen          int

	// Tracks previous input height to avoid unnecessary resizes.
	lastInputHeight int

	// Prompt history for up/down arrow navigation.
	promptHistory      []string // newest-first
	promptHistoryIndex int      // -1 = not browsing, 0 = most recent, 1 = older...
}

// NewModel creates a new TUI model.
func NewModel(session *sdk.Session) *Model {
	vp := viewport.New()
	vp.SoftWrap = false

	ta := textarea.New()
	ta.Placeholder = "Type a message... (Ctrl+D to quit, /help for commands)"
	ta.Focus()
	ta.ShowLineNumbers = false
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 8

	styles := textarea.DefaultStyles(true)
	bg := lipgloss.Color("235")
	for _, s := range []*textarea.StyleState{&styles.Focused, &styles.Blurred} {
		s.Base = s.Base.Background(bg).Padding(0, 1)
		s.Text = s.Text.Background(bg)
		s.Placeholder = s.Placeholder.Background(bg)
		s.Prompt = s.Prompt.Background(bg)
		s.EndOfBuffer = s.EndOfBuffer.Background(bg)
		s.CursorLine = s.CursorLine.Background(bg)
		s.LineNumber = s.LineNumber.Background(bg)
		s.CursorLineNumber = s.CursorLineNumber.Background(bg)
	}
	ta.SetStyles(styles)

	// Cache session info ONCE — never call session methods from View().
	mod := session.Model()
	modelName := mod.ID
	modelProv := mod.Provider
	modelReasoning := mod.Reasoning
	if modelName == "" {
		modelName = "no model"
		modelProv = "none"
	}
	cwd := session.Cwd()
	sessionID := session.ID()
	sessionName := session.Name()
	thinkingLevel := string(session.ThinkingLevel())

	r, _ := NewRenderer(80)
	m := &Model{
		session:        session,
		viewport:       vp,
		input:          ta,
		state:          stateIdle,
		modelName:      modelName,
		modelProv:      modelProv,
		modelReasoning: modelReasoning,
		thinkingLevel:  thinkingLevel,
		cwd:            cwd,
		sessionID:      sessionID,
		sessionName:    sessionName,
		pendingBuilder: new(strings.Builder),
		pendingToolIndex: -1,
		spinner:        spinner.New(spinner.WithSpinner(bouncingDots)),
		glamourRenderer: r,
		commandRegistry: NewCommandRegistry(),
	}

	_ = m.commandRegistry.LoadCustomCommands(cwd, EmbeddedCommands())

	// Load prompt history from file
	m.promptHistory = loadPromptHistory(cwd)
	m.promptHistoryIndex = -1

	// Seed history from session messages (if resuming) or from existing session files
	if session != nil {
		msgs := session.Messages()
		if len(msgs) > 0 {
			sessionPrompts := extractUserPrompts(msgs)
			m.promptHistory = mergePromptsIntoHistory(m.promptHistory, sessionPrompts)
		} else {
			// Fresh session — load from most recent existing session file
			filePrompts := loadHistoryFromSessions(cwd)
			m.promptHistory = mergePromptsIntoHistory(m.promptHistory, filePrompts)
		}
	}

	return m
}

// SetProgram stores the running bubbletea program for sending messages.
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

func (m *Model) Init() tea.Cmd {
	return nil
}

// --- Turn lifecycle ---

// submitPrompt starts a new agent turn.
func (m *Model) submitPrompt(message string) tea.Cmd {
	m.state = stateStreaming
	m.cancelFunc = nil
	m.resetForTurn()

	// Show user message immediately as a finalized block.
	m.blocks = append(m.blocks, messageBlock{
		kind: blockUserMessage,
		text: message,
	})
	m.updateViewport()

	// Clear input but keep it focused so user can type next message during streaming
	m.input.SetValue("")
	m.input.Focus()

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFunc = cancel

	go func() {
		unsub := m.session.Subscribe(func(e types.AgentEvent) {
			m.program.Send(AgentEventMsg{Event: e})
		})
		defer unsub()

		slog.Debug("tui: calling session.Prompt", "message_len", len(message))
		err := m.session.Prompt(ctx, message)
		slog.Debug("tui: session.Prompt returned", "error", err)
		if err != nil {
			if ctx.Err() == context.Canceled {
				m.program.Send(PromptDoneMsg{Interrupted: true})
				return
			}
			m.program.Send(ErrorMsg{Err: err})
			return
		}
		m.program.Send(PromptDoneMsg{})
	}()

	return m.startSpinner()
}

// resetForTurn clears per-turn state.
// resetForTurn clears per-turn state.
func (m *Model) resetForTurn() {
	m.pendingBuilder.Reset()
	m.pendingKind = 0
	m.pendingToolIndex = -1
	m.pendingRendered = ""
	m.pendingRenderedLen = 0
	m.lastRenderTime = time.Time{}
	m.stopSpinner()
	m.invalidateRenderedCache()
	m.lastSetContentPendingLen = 0
	m.lastSetContentPendingRenderedLen = 0
	m.lastSetContentBlocksLen = 0
}

// captureUsage fetches and caches current session usage.
func (m *Model) captureUsage() {
	if m.session == nil {
		return
	}
	m.usage = m.session.Usage()
}

// updateViewport refreshes the viewport with current rendered content.
func (m *Model) updateViewport() {
	m.updateViewportWithForce(false)
}

// updateViewportWithForce refreshes the viewport, optionally bypassing the throttle.
func (m *Model) updateViewportWithForce(force bool) {
	if !force && time.Since(m.lastViewportUpdate) < viewportUpdateInterval {
		return
	}
	m.lastViewportUpdate = time.Now()

	// Skip expensive SetContent() if nothing has changed since last call.
	pendingLen := m.pendingBuilder.Len()
	pendingRenderedLen := len(m.pendingRendered)
	blocksLen := len(m.blocks)
	if !force && pendingLen == m.lastSetContentPendingLen && pendingRenderedLen == m.lastSetContentPendingRenderedLen && blocksLen == m.lastSetContentBlocksLen {
		return
	}
	m.lastSetContentPendingLen = pendingLen
	m.lastSetContentPendingRenderedLen = pendingRenderedLen
	m.lastSetContentBlocksLen = blocksLen

	var content string
	if m.renderedCacheValid {
		content = m.renderedCache
	} else {
		content = renderBlocks(m.blocks, m.width)
		m.renderedCache = content
		m.renderedCacheValid = true
	}
	if m.pendingBuilder.Len() > 0 {
		if m.pendingKind == blockAssistantText && m.pendingRendered != "" {
			content += renderPendingBlock(m.pendingBuilder.String(), m.pendingKind, m.width, m.pendingRendered)
		} else {
			content += renderPendingBlock(m.pendingBuilder.String(), m.pendingKind, m.width, "")
		}
	}

	wasAtBottom := m.viewport.AtBottom()
	oldYOffset := m.viewport.YOffset()

	m.viewport.SetContent(content)

	if wasAtBottom {
		m.viewport.GotoBottom()
	} else {
		maxOffset := max(0, m.viewport.TotalLineCount()-m.viewport.VisibleLineCount())
		if oldYOffset > maxOffset {
			oldYOffset = maxOffset
		}
		m.viewport.SetYOffset(oldYOffset)
	}
}

// invalidateRenderedCache clears the finalized block render cache.
func (m *Model) invalidateRenderedCache() {
	m.renderedCache = ""
	m.renderedCacheValid = false
}

// refreshRenderedCache re-renders all finalized blocks and stores the result.
func (m *Model) refreshRenderedCache() {
	m.renderedCache = renderBlocks(m.blocks, m.width)
	m.renderedCacheValid = true
}

// renderPendingBlock renders the in-progress streaming block.
// If renderedMarkdown is non-empty, uses it for assistant text blocks.
func renderPendingBlock(text string, kind blockType, width int, renderedMarkdown string) string {
	switch kind {
	case blockAssistantText:
		if renderedMarkdown != "" {
			return renderedMarkdown
		}
		return renderAssistantText(&messageBlock{text: text}, width)
	case blockThinking:
		return renderThinkingBlock(text, width)
	default:
		return text
	}
}

// renderPendingMarkdown re-renders the pending content through glamour.
// Only applies to assistant text blocks. Uses the reused renderer instance.
func (m *Model) renderPendingMarkdown() {
	if m.pendingKind != blockAssistantText || m.pendingBuilder.Len() == 0 {
		return
	}
	if m.glamourRenderer == nil {
		return
	}

	text := m.pendingBuilder.String()
	// Skip if content hasn't changed since last render.
	if len(text) == m.pendingRenderedLen {
		return
	}

	rendered := RenderWithRenderer(m.glamourRenderer, text, text)
	if rendered != "" {
		m.pendingRendered = rendered
		m.pendingRenderedLen = len(text)
		m.lastRenderTime = time.Now()
	}
}
// flushPending moves the pending builder content into a finalized block.
func (m *Model) flushPending() {
	if m.pendingBuilder.Len() > 0 {
		block := messageBlock{
			kind: m.pendingKind,
			text: m.pendingBuilder.String(),
		}
	if m.pendingKind == blockAssistantText {
			block.isFinalized = true
			block.renderedMarkdown = RenderMarkdown(block.text, m.width)
		}
		m.blocks = append(m.blocks, block)
		m.pendingBuilder.Reset()
		m.invalidateRenderedCache()
	}
}

// ensurePending creates a new pending builder if none exists.
func (m *Model) ensurePending(kind blockType) {
	if m.pendingBuilder == nil {
		m.pendingBuilder = new(strings.Builder)
	}
	if m.pendingKind != kind {
		// Flush the old kind and start a new one.
		m.flushPending()
		m.pendingKind = kind
	}
}

// --- Spinner ---

// startSpinner activates the spinner and returns a Tick command.
func (m *Model) startSpinner() tea.Cmd {
	m.spinnerActive = true
	m.spinner = spinner.New(spinner.WithSpinner(bouncingDots))
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(m.spinner.Tick())
	m.spinnerTick = cmd
	return cmd
}

// stopSpinner deactivates the spinner.
func (m *Model) stopSpinner() {
	m.spinnerActive = false
}

// handleSpinnerTick advances the spinner and returns the next Tick command.
func (m *Model) handleSpinnerTick() tea.Cmd {
	if !m.spinnerActive {
		return nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(spinner.TickMsg{})
	return cmd
}

// --- Event processing ---

// processEvent handles an agent event and returns an optional Cmd (spinner tick).

func (m *Model) processEvent(e types.AgentEvent) tea.Cmd {
	var cmd tea.Cmd
	switch e.Type {
	case types.AgentEventMessageStart:
		m.ensurePending(blockAssistantText)

	case types.AgentEventTextDelta:
		m.ensurePending(blockAssistantText)
		if text, ok := e.Data.(string); ok {
			m.pendingBuilder.WriteString(text)
		}

	case types.AgentEventThinkingDelta:
		m.ensurePending(blockThinking)
		if text, ok := e.Data.(string); ok {
			m.pendingBuilder.WriteString(text)
		}

	case types.AgentEventMessageEnd:
		m.flushPending()

	case types.AgentEventTurnEnd:
		m.flushPending()
		m.turnCount++
		// Add turn separator
		m.blocks = append(m.blocks, messageBlock{
			kind: blockTurnSeparator,
			text: "",
		})

	case types.AgentEventAgentEnd:
		m.flushPending()
		m.stopSpinner()

	case types.AgentEventError:
		var errMsg string
		if err, ok := e.Data.(error); ok {
			errMsg = err.Error()
		} else if s, ok := e.Data.(string); ok {
			errMsg = s
		} else {
			errMsg = "unknown error"
		}
		// If a tool call is pending, mark it as failed.
		if m.pendingToolIndex >= 0 && m.pendingToolIndex < len(m.blocks) {
			m.blocks[m.pendingToolIndex].toolSt = toolError
			m.blocks[m.pendingToolIndex].toolErr = errMsg
			m.pendingToolIndex = -1
		}
		m.flushPending()
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: errMsg,
		})

	case types.AgentEventToolExecStart:
		m.flushPending()
		name := "…"
		m.blocks = append(m.blocks, messageBlock{
			kind:     blockToolCall,
			toolName: name,
			toolSt:   toolPending,
		})
		m.pendingToolIndex = len(m.blocks) - 1

	case types.AgentEventToolExecEnd:
		if m.pendingToolIndex >= 0 && m.pendingToolIndex < len(m.blocks) {
			m.blocks[m.pendingToolIndex].toolSt = toolSuccess
			if data, ok := e.Data.(map[string]any); ok {
				if n, ok := data["tool"].(string); ok && n != "" {
					m.blocks[m.pendingToolIndex].toolName = n
				}
				if a, ok := data["args"].(string); ok {
					m.blocks[m.pendingToolIndex].toolArgs = a
				}
			}
		}
		m.pendingToolIndex = -1
		m.invalidateRenderedCache()

	case types.AgentEventToolProgress:
		// Progress events keep TUI responsive during long tool execution.
		// No state change needed — viewport update happens below.

	case types.AgentEventToolResult:
		m.flushPending()
		toolName := ""
		content := ""
		isError := false
		if data, ok := e.Data.(map[string]any); ok {
			if n, ok := data["tool"].(string); ok {
				toolName = n
			}
			if c, ok := data["content"].(string); ok {
				content = c
			}
			if ie, ok := data["isError"].(bool); ok {
				isError = ie
			}
		}
		m.blocks = append(m.blocks, messageBlock{
			kind:              blockToolResult,
			toolResultName:    toolName,
			toolResultContent: content,
			toolResultIsError: isError,
		})
		m.invalidateRenderedCache()

	case types.AgentEventSubAgentStart:
		m.flushPending()
		id := ""
		if s, ok := e.Data.(string); ok {
			id = s
		}
		m.blocks = append(m.blocks, messageBlock{
			kind: blockSubAgentStart,
			text: id,
		})
		m.invalidateRenderedCache()

	case types.AgentEventSubAgentEnd:
		id := ""
		if s, ok := e.Data.(string); ok {
			id = s
		}
		m.blocks = append(m.blocks, messageBlock{
			kind: blockSubAgentEnd,
			text: id,
		})
		m.invalidateRenderedCache()
	}
	return cmd
}

// --- Completion / error handlers ---

func (m *Model) handlePromptDone(interrupted bool) tea.Cmd {
	m.flushPending()
	if interrupted {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: "Interrupted",
		})
		m.invalidateRenderedCache()
	}
	m.updateViewport()
	// Capture usage after prompt completes (s.mu is released by then)
	m.captureUsage()

	if m.session.OverflowCount() > 0 {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Queue overflow: %d message(s) dropped", m.session.OverflowCount()),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		m.session.ResetOverflow()
	}

	m.returnToIdle()

	if next := m.session.DequeueMessage(); next != "" {
		return m.submitPrompt(next)
	}
	return nil
}

func (m *Model) handleError(err error) {
	m.flushPending()
	m.blocks = append(m.blocks, messageBlock{
		kind: blockError,
		text: err.Error(),
	})
	m.invalidateRenderedCache()
	m.updateViewportWithForce(true)
	m.returnToIdle()
}

func (m *Model) returnToIdle() {
	m.state = stateIdle
	m.cancelFunc = nil
	m.pendingExit = false
	m.input.SetValue("")
	m.input.Focus()
}

// --- Slash commands ---

// executeCommand looks up the command in the registry and executes it.
// Returns (handled=true, cmd) if a command was found and available.
// Returns (handled=false, nil) if no matching command or not available.
func (m *Model) executeCommand(input string) (bool, tea.Cmd) {
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

// sessionInfoText builds a human-readable session info string.
// Uses cached fields to avoid acquiring s.mu during streaming.
func (m *Model) sessionInfoText() string {
	var parts []string
	parts = append(parts, "ID: "+m.sessionID)
	parts = append(parts, "Name: "+m.sessionName)
	parts = append(parts, "Model: "+m.modelName)
	parts = append(parts, "Working directory: "+m.cwd)
	parts = append(parts, fmt.Sprintf("Turns: %d", m.turnCount))

	if m.usage.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("Tokens: %d", m.usage.TotalTokens))
		if m.usage.Cost.Total > 0 {
			parts = append(parts, fmt.Sprintf("Cost: $%.4f", m.usage.Cost.Total))
		} else if m.modelProv == "ollama" {
			parts = append(parts, "Cost: $0.00 (local)")
		}
	}

	if m.session.Ephemeral() {
		parts = append(parts, "Mode: ephemeral")
	}

	return strings.Join(parts, "\n")
}

// findSkill looks up a skill by name (case-insensitive).
func (m *Model) findSkill(name string) *skills.Skill {
	for _, sk := range m.session.Skills() {
		if strings.EqualFold(sk.Name, name) {
			return sk
		}
	}
	return nil
}

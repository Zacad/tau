package tui

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/adam/tau/internal/fuzzy"
	"github.com/adam/tau/internal/skills"
	"github.com/adam/tau/internal/subagent"
	"github.com/adam/tau/internal/tui/customcmd"
	"github.com/adam/tau/internal/tui/palette"
	"github.com/adam/tau/internal/types"
)

type commandType int

const (
	appCommand commandType = iota
	appMultiStepCommand
	chatCommand
)

// Command represents a slash command available in the TUI.
type Command struct {
	name         string
	description  string
	typ          commandType
	handler      func(m *Model, args string) tea.Cmd
	available    func(m *Model) bool
	multiStep    func(m *Model) []palette.Step
	chatTemplate string
}

func NewAppCommand(name, desc string, handler func(m *Model, args string) tea.Cmd) Command {
	return Command{
		name:        name,
		description: desc,
		typ:         appCommand,
		handler:     handler,
	}
}

func NewAppMultiStep(name, desc string, steps func(m *Model) []palette.Step) Command {
	return Command{
		name:        name,
		description: desc,
		typ:         appMultiStepCommand,
		multiStep:   steps,
	}
}

func NewChatCommand(name, desc string, template string, available func(m *Model) bool, handler func(m *Model, args string) tea.Cmd) Command {
	return Command{
		name:         name,
		description:  desc,
		typ:          chatCommand,
		chatTemplate: template,
		available:    available,
		handler:      handler,
	}
}

func (c Command) Name() string        { return c.name }
func (c Command) Description() string { return c.description }
func (c Command) Type() commandType   { return c.typ }

func (c Command) IsAvailable(m *Model) bool {
	return c.available == nil || c.available(m)
}

func (c Command) Handler() func(m *Model, args string) tea.Cmd {
	return c.handler
}

func (c Command) MultiStep() func(m *Model) []palette.Step {
	return c.multiStep
}

func (c Command) ChatTemplate() string { return c.chatTemplate }

// CommandRegistry holds all registered commands.
type CommandRegistry struct {
	commands         []Command
	customCommands   []customcmd.CustomCommand
	embeddedCommands []customcmd.CustomCommand
	cwd              string
	builtinCount     int
}

// NewCommandRegistry creates a registry with all built-in commands.
func NewCommandRegistry() *CommandRegistry {
	r := &CommandRegistry{}
	r.registerAll()
	return r
}

func (r *CommandRegistry) registerAll() {
	r.commands = []Command{
		NewAppCommand("quit", "Exit tau", cmdQuit),
		NewAppCommand("exit", "Exit tau", cmdQuit),
		NewAppCommand("help", "Show help text", cmdHelp),
		NewAppCommand("test", "Test command for development verification", cmdTest),
		NewAppCommand("name", "Rename the current session", cmdName),
		NewAppCommand("session", "Show session information", cmdSession),
		NewAppCommand("model", "Change the active model", cmdModel),
		NewAppCommand("thinking", "Set thinking level for current model", cmdThinking),
		NewAppCommand("compact", "Trigger context compaction", cmdCompact),
		NewAppCommand("clear", "Clear the viewport", cmdClear),
		NewAppCommand("skills", "List available skills", cmdSkills),
		NewAppCommand("skill:", "Load a skill's content", cmdSkill),
		NewAppCommand("reload", "Reload custom commands", cmdReload),
		NewAppMultiStep("connect", "Connect to a provider (multi-step)", connectSteps),
		NewAppMultiStep("disconnect", "Disconnect/disable a provider (multi-step)", disconnectSteps),
		NewAppCommand("agents", "List available subagent types and user-defined agents", cmdAgents),
		NewAppCommand("new", "Start a new session", cmdNew),
		NewAppCommand("resume", "Resume a previous session", cmdResume),
	}
	for i := range r.commands {
		switch r.commands[i].name {
		case "name", "model", "compact", "skill:", "reload", "connect", "disconnect", "thinking", "new", "resume":
			r.commands[i].available = availableIdle
		}
	}
	r.builtinCount = len(r.commands)
}

// LoadCustomCommands discovers and registers custom commands from project and global directories.
// Built-in commands always take priority over custom commands with the same name.
// Embedded commands can be passed as defaults (e.g., built-in custom commands shipped with tau).
func (r *CommandRegistry) LoadCustomCommands(cwd string, embedded []customcmd.CustomCommand) error {
	r.cwd = cwd
	r.embeddedCommands = embedded

	customs, err := customcmd.DiscoverCommands(cwd, embedded)
	if err != nil {
		return err
	}
	r.customCommands = customs

	// Truncate to built-ins only, then re-append custom commands
	r.commands = r.commands[:r.builtinCount]

	for _, cc := range customs {
		if r.isBuiltinName(cc.Name) {
			slog.Debug("custom command shadows built-in, skipping", "name", cc.Name, "source", cc.Source)
			continue
		}
		r.commands = append(r.commands, r.customCommandToCommand(cc))
	}

	return nil
}

func (r *CommandRegistry) isBuiltinName(name string) bool {
	for i := 0; i < r.builtinCount; i++ {
		if r.commands[i].Name() == name {
			return true
		}
	}
	return false
}

func (r *CommandRegistry) customCommandToCommand(cc customcmd.CustomCommand) Command {
	desc := cc.Description
	if desc == "" {
		desc = "Custom command"
	}
	desc += " [custom]"

	return NewChatCommand(cc.Name, desc, cc.Template, availableIdle, func(m *Model, args string) tea.Cmd {
		resolver := func(name string) string {
			skill := m.findSkill(name)
			if skill == nil {
				return ""
			}
			return fmt.Sprintf("[Skill: %s]\n%s", skill.Name, skill.Content)
		}
		template := customcmd.ProcessTemplate(cc.Template, args, resolver)
		return m.submitPrompt(template)
	})
}

// Lookup returns a command by name (without /). Returns nil if not found.
func (r *CommandRegistry) Lookup(name string) *Command {
	for i := range r.commands {
		if r.commands[i].Name() == name {
			return &r.commands[i]
		}
	}
	return nil
}

// Filter returns commands matching the query (fuzzy match on Name), sorted by score descending.
// Also returns the matched positions for each command (for highlighting).
func (r *CommandRegistry) Filter(query string) ([]Command, [][]int) {
	type scored struct {
		cmd       Command
		score     int
		positions []int
	}

	var scoredCmds []scored
	for i := range r.commands {
		c := &r.commands[i]
		if score, matched, positions := fuzzy.Match(query, c.Name()); matched {
			scoredCmds = append(scoredCmds, scored{cmd: *c, score: score, positions: positions})
		}
	}

	sort.Slice(scoredCmds, func(i, j int) bool {
		return scoredCmds[i].score > scoredCmds[j].score
	})

	var result []Command
	var resultPositions [][]int
	for _, sc := range scoredCmds {
		result = append(result, sc.cmd)
		resultPositions = append(resultPositions, sc.positions)
	}
	return result, resultPositions
}

// AvailableCommands returns all commands currently available for the given model state.
func (r *CommandRegistry) AvailableCommands(m *Model) []Command {
	var result []Command
	for i := range r.commands {
		c := &r.commands[i]
		if c.IsAvailable(m) {
			result = append(result, *c)
		}
	}
	return result
}

// ParseCommandInput extracts the command name and args from user input.
// Returns ("", "", false) if input is not a slash command.
// Handles special colon-prefixed commands like /skill:name.
func ParseCommandInput(input string) (name string, args string, isCommand bool) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return "", "", false
	}
	rest := strings.TrimPrefix(trimmed, "/")

	// Check for colon-prefixed commands (e.g., skill:name -> skill:)
	if idx := strings.Index(rest, ":"); idx > 0 {
		potentialName := rest[:idx+1] // include the colon
		// Check if this matches a registered colon-command
		for _, cName := range []string{"skill:"} {
			if potentialName == cName {
				return cName, rest[idx+1:], true
			}
		}
	}

	// Normal space-separated command
	if idx := strings.Index(rest, " "); idx >= 0 {
		return rest[:idx], strings.TrimSpace(rest[idx+1:]), true
	}
	return rest, "", true
}

// --- Availability functions ---

func availableIdle(m *Model) bool {
	return m.state == stateIdle
}

func availableIdleNotEphemeral(m *Model) bool {
	return m.state == stateIdle && !m.session.Ephemeral()
}

// --- Command handlers ---

func cmdQuit(m *Model, _ string) tea.Cmd {
	return tea.Quit
}

var helpText = "Commands:\n" +
	"  /quit, /exit    Exit the application\n" +
	"  /help           Show this help message\n" +
	"  /new            Start a new session\n" +
	"  /resume         Resume a previous session\n" +
	"  /name <name>    Rename the current session\n" +
	"  /session        Show session information\n" +
	"  /model          Change the active model\n" +
	"  /thinking       Set thinking level for current model\n" +
	"  /compact        Trigger context compaction\n" +
	"  /clear          Clear the viewport\n" +
	"  /skills         List available skills\n" +
	"  /skill:<name>   Load a skill's content\n" +
	"  /agents         List available subagent types and user-defined agents\n" +
	"  /reload         Reload custom commands\n\n" +
	"Custom commands from .tau/commands/ and ~/.tau/commands/ are loaded automatically.\n" +
	"Define commands as markdown files with YAML frontmatter.\n\n" +
	"Keyboard:\n" +
	"  Enter         Send message\n" +
	"  Shift+Enter   New line\n" +
	"  Ctrl+D        Quit (when input is empty)\n" +
	"  Ctrl+C        Abort current response / Exit (double-tap)\n" +
	"  Ctrl+P        Open command palette\n" +
	"  Esc           Clear input"

func cmdHelp(m *Model, _ string) tea.Cmd {
	m.blocks = append(m.blocks, messageBlock{
		kind: blockAssistantText,
		text: helpText,
	})
	m.invalidateRenderedCache()
	m.updateViewport()
	return nil
}

func cmdTest(m *Model, _ string) tea.Cmd {
	items := []palette.PaletteItem{
		testOptionItem{title: "Enter text", desc: "5-step flow: input → confirm → process → message → done"},
		testOptionItem{title: "Print message", desc: "Print test message to viewport"},
		testOptionItem{title: "Show info", desc: "Show palette component info"},
		testOptionItem{title: "Cancel", desc: "Do nothing"},
	}
	avail := []bool{true, true, true, true}

	m.palette.OpenWithItems(items, avail, func(item palette.PaletteItem, index int) tea.Cmd {
		toi, ok := item.(testOptionItem)
		if !ok {
			return nil
		}
		switch toi.title {
		case "Enter text":
			m.paletteConfirmPrompt = "Save this text?"
			m.paletteTaskTitle = "Processing"
			m.paletteTaskFunc = func() (bool, string, error) {
				time.Sleep(1 * time.Second)
				return true, "Text saved successfully", nil
			}
			m.palette.ShowInput("Enter text", "Type something...")
			return nil
		case "Print message":
			m.blocks = append(m.blocks, messageBlock{
				kind: blockAssistantText,
				text: "Test command executed",
			})
			m.invalidateRenderedCache()
			m.updateViewport()
		case "Show info":
			m.blocks = append(m.blocks, messageBlock{
				kind: blockAssistantText,
				text: "Test info: palette component working",
			})
			m.invalidateRenderedCache()
			m.updateViewport()
		case "Cancel":
		}
		return nil
	})
	m.paletteActive = true
	m.input.Blur()
	return nil
}

type testOptionItem struct {
	title string
	desc  string
}

func (t testOptionItem) Title() string       { return t.title }
func (t testOptionItem) Description() string { return t.desc }
func (t testOptionItem) FilterValue() string { return t.title }

func cmdName(m *Model, args string) tea.Cmd {
	if args == "" {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: "Usage: /name <new-name>",
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}
	if m.session.Ephemeral() {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "Cannot rename in ephemeral mode (no session persistence)",
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}
	if err := m.session.Rename(args); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: "Failed to rename session: " + err.Error(),
		})
	} else {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "Session renamed to: " + args,
		})
	}
	m.invalidateRenderedCache()
	m.updateViewport()
	return nil
}

func cmdSession(m *Model, _ string) tea.Cmd {
	info := m.sessionInfoText()
	m.blocks = append(m.blocks, messageBlock{
		kind: blockAssistantText,
		text: info,
	})
	m.invalidateRenderedCache()
	m.updateViewport()
	return nil
}

type modelPaletteItem struct {
	title       string
	description string
	category    string
	model       types.Model
}

func (i modelPaletteItem) Title() string       { return i.title }
func (i modelPaletteItem) Description() string { return i.description }
func (i modelPaletteItem) FilterValue() string {
	return i.title + " " + i.category + " " + i.description
}
func (i modelPaletteItem) Category() string { return i.category }

func cmdModel(m *Model, _ string) tea.Cmd {
	models := m.session.ListModels()
	if len(models) == 0 {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: noModelGuidanceText(),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}

	items := make([]palette.PaletteItem, len(models))
	avail := make([]bool, len(models))
	for i, mod := range models {
		contextStr := ""
		if mod.ContextWindow > 0 {
			contextStr = formatContextWindow(mod.ContextWindow)
		}
		items[i] = modelPaletteItem{
			title:       mod.ID,
			description: contextStr,
			category:    mod.Provider,
			model:       mod,
		}
		avail[i] = true
	}

	m.palette.OpenWithGroupedItems(items, avail, func(item palette.PaletteItem, index int) tea.Cmd {
		mi, ok := item.(modelPaletteItem)
		if !ok {
			return nil
		}
		canonicalRef := mi.model.Provider + "/" + mi.model.ID
		if err := m.session.SetModel(canonicalRef); err != nil {
			m.blocks = append(m.blocks, messageBlock{
				kind: blockError,
				text: "Failed to switch model: " + err.Error(),
			})
		} else {
			m.modelName = mi.model.ID
			m.modelProv = mi.model.Provider
			m.modelReasoning = mi.model.Reasoning
			m.thinkingLevel = string(m.session.ThinkingLevel())
			m.contextWindow = mi.model.ContextWindow
			m.refreshContext()
			m.blocks = append(m.blocks, messageBlock{
				kind: blockAssistantText,
				text: "Switched to: " + canonicalRef,
			})
		}
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	})
	m.paletteActive = true
	m.input.Blur()
	return nil
}

func noModelGuidanceText() string {
	return "No model connected. Use `/connect` to add a provider, then `/model` to choose one."
}

type thinkingPaletteItem struct {
	title       string
	description string
	level       types.ThinkingLevel
}

func (i thinkingPaletteItem) Title() string       { return i.title }
func (i thinkingPaletteItem) Description() string { return i.description }
func (i thinkingPaletteItem) FilterValue() string { return i.title + " " + i.description }

func cmdThinking(m *Model, _ string) tea.Cmd {
	mod := m.session.Model()
	if !mod.Reasoning {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: fmt.Sprintf("Model %s does not support thinking/reasoning", mod.ID),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}

	levels := mod.GetSupportedThinkingLevels()
	currentLevel := m.session.ThinkingLevel()

	items := make([]palette.PaletteItem, len(levels))
	avail := make([]bool, len(levels))
	for i, level := range levels {
		desc := types.ThinkingLevelDescription(level)
		marker := ""
		if level == currentLevel {
			marker = " (current)"
		}
		items[i] = thinkingPaletteItem{
			title:       string(level) + marker,
			description: desc,
			level:       level,
		}
		avail[i] = true
	}

	m.palette.OpenWithItems(items, avail, func(item palette.PaletteItem, index int) tea.Cmd {
		ti, ok := item.(thinkingPaletteItem)
		if !ok {
			return nil
		}
		if err := m.session.SetThinkingLevel(ti.level); err != nil {
			m.blocks = append(m.blocks, messageBlock{
				kind: blockError,
				text: "Failed to set thinking level: " + err.Error(),
			})
		} else {
			m.thinkingLevel = string(ti.level)
			m.blocks = append(m.blocks, messageBlock{
				kind: blockAssistantText,
				text: fmt.Sprintf("Thinking level set to: %s", ti.level),
			})
		}
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	})
	m.paletteActive = true
	m.input.Blur()
	return nil
}

func cmdCompact(m *Model, _ string) tea.Cmd {
	if m.session.Ephemeral() {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "Cannot compact in ephemeral mode (no session persistence)",
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}
	ctx := context.Background()
	if err := m.session.Compact(ctx); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: "Compaction failed: " + err.Error(),
		})
	} else {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "Session compacted",
		})
	}
	m.invalidateRenderedCache()
	m.updateViewport()
	return nil
}

func cmdClear(m *Model, _ string) tea.Cmd {
	m.blocks = nil
	m.invalidateRenderedCache()
	m.updateViewport()
	return nil
}

func cmdSkills(m *Model, _ string) tea.Cmd {
	skills := m.session.Skills()
	if len(skills) == 0 {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: "No skills discovered",
		})
	} else {
		var lines []string
		for _, sk := range skills {
			lines = append(lines, fmt.Sprintf("%s (%s)\n  %s", sk.Name, sk.Source, sk.Description))
		}
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: strings.Join(lines, "\n\n"),
		})
	}
	m.invalidateRenderedCache()
	m.updateViewport()
	return nil
}

func cmdSkill(m *Model, args string) tea.Cmd {
	if args == "" {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: "Usage: /skill:<name> (use /skills to list available skills)",
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}
	skill := m.findSkill(args)
	if skill == nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: fmt.Sprintf("Skill %q not found (use /skills to list available skills)", args),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}
	content := fmt.Sprintf("[Skill: %s]\n%s", skill.Name, skill.Content)
	if len(content) > 10000 {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: fmt.Sprintf("Warning: skill content exceeds 10k chars (%d chars). Truncating.", len(content)),
		})
		content = content[:10000]
	}
	m.blocks = append(m.blocks, messageBlock{
		kind: blockAssistantText,
		text: fmt.Sprintf("Loading skill: %s", skill.Name),
	})
	m.invalidateRenderedCache()
	m.updateViewport()
	return m.submitPrompt(content)
}

func formatContextWindow(tokens int) string {
	if tokens >= 1_000_000 {
		return fmt.Sprintf("%.0fM ctx", float64(tokens)/1_000_000)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%.0fK ctx", float64(tokens)/1000)
	}
	return fmt.Sprintf("%d ctx", tokens)
}

func cmdReload(m *Model, _ string) tea.Cmd {
	if err := m.commandRegistry.LoadCustomCommands(m.commandRegistry.cwd, m.commandRegistry.embeddedCommands); err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: "Failed to reload custom commands: " + err.Error(),
		})
	} else {
		count := len(m.commandRegistry.customCommands)
		m.blocks = append(m.blocks, messageBlock{
			kind: blockAssistantText,
			text: fmt.Sprintf("Reloaded %d custom command(s)", count),
		})
	}
	m.invalidateRenderedCache()
	m.updateViewport()
	return nil
}

func cmdAgents(m *Model, _ string) tea.Cmd {
	agents := subagent.AllAgents(m.cwd)

	var lines []string

	// Built-in types
	lines = append(lines, "## Built-in Agent Types")
	lines = append(lines, "")
	for _, typ := range subagent.AllTypes() {
		tools := subagent.DefaultToolSet(typ)
		lines = append(lines, fmt.Sprintf("### %s", typ))
		lines = append(lines, fmt.Sprintf("Tools: %s", strings.Join(tools, ", ")))
		lines = append(lines, "")
	}

	// User-defined agents
	userAgents := make(map[string]*subagent.AgentDefinition)
	for name, def := range agents {
		if def.Source != "builtin" {
			userAgents[name] = def
		}
	}

	if len(userAgents) > 0 {
		lines = append(lines, "## User-Defined Agents")
		lines = append(lines, "")

		// Sort by name
		var names []string
		for name := range userAgents {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			def := userAgents[name]
			toolStr := "all parent tools"
			if len(def.Tools) > 0 {
				toolStr = strings.Join(def.Tools, ", ")
			}
			modelStr := ""
			if def.Model != "" {
				modelStr = fmt.Sprintf(" | Model: %s", def.Model)
			}
			lines = append(lines, fmt.Sprintf("### %s (%s)%s", name, def.Source, modelStr))
			lines = append(lines, def.Description)
			lines = append(lines, fmt.Sprintf("Tools: %s", toolStr))
			lines = append(lines, "")
		}
	} else {
		lines = append(lines, "## User-Defined Agents")
		lines = append(lines, "")
		lines = append(lines, "No user-defined agents found.")
		lines = append(lines, "Create agents in `~/.tau/agents/` (user) or `.tau/agents/` (project).")
		lines = append(lines, "Each agent is a Markdown file with YAML frontmatter.")
		lines = append(lines, "")
	}

	m.blocks = append(m.blocks, messageBlock{
		kind: blockAssistantText,
		text: strings.Join(lines, "\n"),
	})
	m.invalidateRenderedCache()
	m.updateViewport()
	return nil
}

func cmdNew(m *Model, _ string) tea.Cmd {
	newID, err := m.session.NewSession()
	if err != nil {
		m.blocks = append(m.blocks, messageBlock{
			kind: blockError,
			text: "Failed to create new session: " + err.Error(),
		})
		m.invalidateRenderedCache()
		m.updateViewport()
		return nil
	}

	m.blocks = nil
	m.pendingBuilder.Reset()
	m.pendingKind = 0
	m.pendingToolIndex = -1
	m.pendingRendered = ""
	m.pendingRenderedLen = 0
	m.turnCount = 0
	m.usage = types.Usage{}
	m.stopSpinner()
	m.invalidateRenderedCache()
	m.lastSetContentPendingLen = 0
	m.lastSetContentPendingRenderedLen = 0
	m.lastSetContentBlocksLen = 0
	m.contextWindow = m.session.Model().ContextWindow
	m.refreshContext()

	m.sessionID = newID
	m.sessionName = m.session.Name()

	m.blocks = append(m.blocks, messageBlock{
		kind: blockAssistantText,
		text: "New session started: " + newID,
	})
	m.updateViewport()
	return nil
}

// --- Skill completion (kept for /skill: tab completion after dropdown) ---

func completeSkill(input string, skillList []*skills.Skill) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	var matches []string
	for _, skill := range skillList {
		if strings.HasPrefix(strings.ToLower(skill.Name), strings.ToLower(input)) {
			matches = append(matches, skill.Name)
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

func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	prefix := strs[0]
	for _, s := range strs[1:] {
		for len(s) < len(prefix) || prefix != s[:len(prefix)] {
			prefix = prefix[:len(prefix)-1]
			if prefix == "" {
				return ""
			}
		}
	}
	return prefix
}

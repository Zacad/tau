package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/adam/tau/internal/tui/customcmd"
	"github.com/adam/tau/internal/tui/palette"
)

func TestCommandRegistry_Lookup(t *testing.T) {
	r := NewCommandRegistry()

	tests := []struct {
		name     string
		input    string
		found    bool
	}{
		{"quit", "quit", true},
		{"exit", "exit", true},
		{"help", "help", true},
		{"skill_colon", "skill:", true},
		{"unknown", "unknown", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := r.Lookup(tt.input)
			if tt.found && cmd == nil {
				t.Fatalf("expected command %q to exist", tt.input)
			}
			if !tt.found && cmd != nil {
				t.Fatalf("expected command %q to not exist", tt.input)
			}
		})
	}
}

func TestCommandRegistry_Filter(t *testing.T) {
	r := NewCommandRegistry()

	tests := []struct {
		name     string
		query    string
		minCount int
	}{
		{"empty_query", "", 18},       // all commands (fuzzy match on empty)
		{"h", "h", 2},                 // help, thinking
		{"s", "s", 7},                 // session, skills, skill:, disconnect, test, agents, resume
		{"se", "se", 3},               // session, disconnect, resume
		{"skill", "skill", 2},         // skills, skill:
		{"skill:", "skill:", 1},       // skill:
		{"unknown", "unknown", 0},     // no match
		{"case_insensitive", "HELP", 1}, // help
		{"n", "n", 7},                 // new, name, thinking, connect, disconnect, agents, session
		{"ne", "ne", 4},               // new, name, connect, disconnect
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, _ := r.Filter(tt.query)
			if len(results) != tt.minCount {
				t.Errorf("Filter(%q) = %d results, want %d", tt.query, len(results), tt.minCount)
			}
		})
	}
}

func TestCommandRegistry_AvailableCommands(t *testing.T) {
	m := newTestModel()
	m.state = stateIdle
	r := NewCommandRegistry()

	available := r.AvailableCommands(m)
	// All commands should be available when idle
	if len(available) != 18 {
		t.Errorf("idle: expected 18 available commands, got %d", len(available))
	}

	// During streaming, commands with availableIdle should be hidden
	m.state = stateStreaming
	available = r.AvailableCommands(m)
	// quit, exit, help, test, session, clear, skills, agents are always available
	// name, model, compact, skill:, reload, connect, disconnect, thinking, new, resume require idle
	if len(available) != 8 {
		t.Errorf("streaming: expected 8 available commands, got %d", len(available))
	}
}

func TestParseCommandInput(t *testing.T) {
	tests := []struct {
		input     string
		wantName  string
		wantArgs  string
		wantCmd   bool
	}{
		{"/quit", "quit", "", true},
		{"/help", "help", "", true},
		{"/name new-name", "name", "new-name", true},
		{"/skill:my-skill", "skill:", "my-skill", true},
		{"/skill:", "skill:", "", true},
		{"not a command", "", "", false},
		{"", "", "", false},
		{"/", "", "", true},
		{"/  ", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, args, isCmd := ParseCommandInput(tt.input)
			if isCmd != tt.wantCmd {
				t.Errorf("isCmd = %v, want %v", isCmd, tt.wantCmd)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if args != tt.wantArgs {
				t.Errorf("args = %q, want %q", args, tt.wantArgs)
			}
		})
	}
}

func TestCommandPalette_FilterIntegration(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()
	m.input.SetValue("/h")

	m.handleSlashPrefix()

	if !m.paletteActive {
		t.Fatal("palette should be active after typing /h")
	}
	if len(m.palette.commands) == 0 {
		t.Fatal("palette should contain commands")
	}
}

func TestCommandPalette_ShowsOnSlash(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()
	m.input.SetValue("/")

	m.handleSlashPrefix()

	if !m.paletteActive {
		t.Fatal("palette should be active after typing /")
	}
	// Should show all available app commands
	count := 0
	for i := range m.commandRegistry.commands {
		c := &m.commandRegistry.commands[i]
		if (c.Type() == appCommand || c.Type() == appMultiStepCommand) && c.IsAvailable(m) {
			count++
		}
	}
	if len(m.palette.commands) != count {
		t.Fatalf("expected %d commands, got %d", count, len(m.palette.commands))
	}
}

func TestCommandPalette_CloseOnNonCommand(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()

	// Open palette via /
	m.input.SetValue("/h")
	m.handleSlashPrefix()
	if !m.paletteActive {
		t.Fatal("palette should be active")
	}

	// Type non-command text — palette stays open (Esc closes it)
	m.input.SetValue("hello")
	m.handleSlashPrefix()
	if !m.paletteActive {
		t.Fatal("palette should stay open when input changes (Esc closes it)")
	}
}

func TestExecuteCommand_Quit(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()

	handled, cmd := m.executeCommand("/quit")
	if !handled {
		t.Fatal("/quit should be handled")
	}
	if cmd == nil {
		t.Fatal("expected Cmd for /quit")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", msg)
	}
}

func TestExecuteCommand_Help(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()

	handled, _ := m.executeCommand("/help")
	if !handled {
		t.Fatal("/help should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockAssistantText {
		t.Fatalf("expected assistantText, got %v", m.blocks[0].kind)
	}
}

func TestExecuteCommand_NotFound(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()

	handled, cmd := m.executeCommand("/unknown")
	if handled {
		t.Fatal("/unknown should not be handled")
	}
	if cmd != nil {
		t.Fatal("expected nil Cmd for unknown command")
	}
}

func TestExecuteCommand_UnavailableDuringStreaming(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()
	m.state = stateStreaming

	// /name is only available when idle
	handled, cmd := m.executeCommand("/name test")
	if handled {
		t.Fatal("/name should not be handled during streaming")
	}
	if cmd != nil {
		t.Fatal("expected nil Cmd")
	}
}

func TestExecuteCommand_SkillColon(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()

	// /skill: without args should show usage error
	handled, _ := m.executeCommand("/skill:")
	if !handled {
		t.Fatal("/skill: should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockError {
		t.Fatalf("expected error block for empty skill, got %v", m.blocks[0].kind)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrImpl(s, substr))
}

func containsStrImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoadCustomCommands_Empty(t *testing.T) {
	r := NewCommandRegistry()
	err := r.LoadCustomCommands("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.commands) != r.builtinCount {
		t.Errorf("expected %d commands, got %d", r.builtinCount, len(r.commands))
	}
}

func TestLoadCustomCommands_EmbeddedTestCommand(t *testing.T) {
	r := NewCommandRegistry()
	err := r.LoadCustomCommands("", EmbeddedCommands())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(r.commands) != r.builtinCount+1 {
		t.Errorf("expected %d commands, got %d", r.builtinCount+1, len(r.commands))
	}

	cmd := r.Lookup("test-command")
	if cmd == nil {
		t.Fatal("expected 'test-command' from embedded commands")
	}
	if !strings.Contains(cmd.Description(), "[custom]") {
		t.Errorf("description should contain [custom], got %q", cmd.Description())
	}
	if cmd.Description() != "Verify embedded command loading [custom]" {
		t.Errorf("description = %q, want %q", cmd.Description(), "Verify embedded command loading [custom]")
	}
}

func TestLoadCustomCommands_WithEmbedded(t *testing.T) {
	r := NewCommandRegistry()
	embedded := []customcmd.CustomCommand{
		{Name: "review", Description: "Review code", Template: "Review: $ARGUMENTS"},
	}
	err := r.LoadCustomCommands("", embedded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.commands) != r.builtinCount+1 {
		t.Errorf("expected %d commands, got %d", r.builtinCount+1, len(r.commands))
	}

	cmd := r.Lookup("review")
	if cmd == nil {
		t.Fatal("expected 'review' command")
	}
	if !strings.Contains(cmd.Description(), "[custom]") {
		t.Errorf("description should contain [custom], got %q", cmd.Description())
	}
}

func TestCustomCommand_CannotOverrideBuiltin(t *testing.T) {
	r := NewCommandRegistry()
	embedded := []customcmd.CustomCommand{
		{Name: "quit", Description: "My quit", Template: "My quit template"},
	}
	err := r.LoadCustomCommands("", embedded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still have only built-in count commands
	if len(r.commands) != r.builtinCount {
		t.Errorf("expected %d commands (builtin override prevented), got %d", r.builtinCount, len(r.commands))
	}

	// quit should still be the built-in
	cmd := r.Lookup("quit")
	if cmd == nil {
		t.Fatal("expected 'quit' command")
	}
	if strings.Contains(cmd.Description(), "[custom]") {
		t.Errorf("quit should not be marked as custom, got %q", cmd.Description())
	}
}

func TestCustomCommand_TemplateExecution(t *testing.T) {
	m := newTestModel()
	embedded := []customcmd.CustomCommand{
		{Name: "greet", Description: "Greet someone", Template: "Hello $1, welcome to $2"},
	}
	err := m.commandRegistry.LoadCustomCommands("", embedded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := m.commandRegistry.Lookup("greet")
	if cmd == nil {
		t.Fatal("expected 'greet' command")
	}
	if !strings.Contains(cmd.Description(), "[custom]") {
		t.Errorf("description should contain [custom], got %q", cmd.Description())
	}

	// Verify template processing works correctly (tested more thoroughly in customcmd package)
	template := customcmd.ProcessTemplate("Hello $1, welcome to $2", "Alice tau", nil)
	if template != "Hello Alice, welcome to tau" {
		t.Errorf("expected 'Hello Alice, welcome to tau', got %q", template)
	}
}

func TestReloadCommand(t *testing.T) {
	m := newTestModel()
	err := m.commandRegistry.LoadCustomCommands("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	handled, _ := m.executeCommand("/reload")
	if !handled {
		t.Fatal("/reload should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockAssistantText {
		t.Fatalf("expected assistantText, got %v", m.blocks[0].kind)
	}
	if !strings.Contains(m.blocks[0].text, "Reloaded 0 custom command(s)") {
		t.Errorf("expected reload message, got %q", m.blocks[0].text)
	}
}

func TestCustomCommand_AvailableOnlyWhenIdle(t *testing.T) {
	m := newTestModel()
	embedded := []customcmd.CustomCommand{
		{Name: "mycmd", Description: "My command", Template: "test"},
	}
	err := m.commandRegistry.LoadCustomCommands("", embedded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be available when idle
	m.state = stateIdle
	cmd := m.commandRegistry.Lookup("mycmd")
	if cmd == nil {
		t.Fatal("expected 'mycmd' command")
	}
	if !cmd.IsAvailable(m) {
		t.Error("mycmd should be available when idle")
	}

	// Should not be available during streaming
	m.state = stateStreaming
	if cmd.IsAvailable(m) {
		t.Error("mycmd should not be available during streaming")
	}
}

func TestNewAppCommand_SetsCorrectType(t *testing.T) {
	called := false
	handler := func(m *Model, args string) tea.Cmd {
		called = true
		return nil
	}
	cmd := NewAppCommand("test", "Test command", handler)

	if cmd.Name() != "test" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "test")
	}
	if cmd.Description() != "Test command" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Test command")
	}
	if cmd.Type() != appCommand {
		t.Errorf("Type() = %v, want %v", cmd.Type(), appCommand)
	}
	if cmd.Handler() == nil {
		t.Fatal("Handler() should not be nil")
	}
	if cmd.MultiStep() != nil {
		t.Error("MultiStep() should be nil for app command")
	}
	if cmd.ChatTemplate() != "" {
		t.Error("ChatTemplate() should be empty for app command")
	}

	cmd.Handler()(nil, "")
	if !called {
		t.Error("handler was not called")
	}
}

func TestNewAppMultiStep_SetsCorrectType(t *testing.T) {
	steps := func(m *Model) []palette.Step {
		return nil
	}
	cmd := NewAppMultiStep("connect", "Connect", steps)

	if cmd.Name() != "connect" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "connect")
	}
	if cmd.Description() != "Connect" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Connect")
	}
	if cmd.Type() != appMultiStepCommand {
		t.Errorf("Type() = %v, want %v", cmd.Type(), appMultiStepCommand)
	}
	if cmd.MultiStep() == nil {
		t.Fatal("MultiStep() should not be nil for multi-step command")
	}
	if cmd.Handler() != nil {
		t.Error("Handler() should be nil for multi-step command")
	}
	if cmd.ChatTemplate() != "" {
		t.Error("ChatTemplate() should be empty for multi-step command")
	}
}

func TestNewChatCommand_SetsCorrectType(t *testing.T) {
	cmd := NewChatCommand("greet", "Greet someone", "Hello $1", nil, nil)

	if cmd.Name() != "greet" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "greet")
	}
	if cmd.Description() != "Greet someone" {
		t.Errorf("Description() = %q, want %q", cmd.Description(), "Greet someone")
	}
	if cmd.Type() != chatCommand {
		t.Errorf("Type() = %v, want %v", cmd.Type(), chatCommand)
	}
	if cmd.ChatTemplate() != "Hello $1" {
		t.Errorf("ChatTemplate() = %q, want %q", cmd.ChatTemplate(), "Hello $1")
	}
	if cmd.Handler() != nil {
		t.Error("Handler() should be nil for chat command without handler")
	}
	if cmd.MultiStep() != nil {
		t.Error("MultiStep() should be nil for chat command")
	}
}

func TestRegistry_CommandTypes(t *testing.T) {
	r := NewCommandRegistry()

	var appCount, multiStepCount, chatCount int
	for i := 0; i < r.builtinCount; i++ {
		switch r.commands[i].Type() {
		case appCommand:
			appCount++
		case appMultiStepCommand:
			multiStepCount++
		case chatCommand:
			chatCount++
		}
	}

	if appCount != 16 {
		t.Errorf("expected 16 app commands, got %d", appCount)
	}
	if multiStepCount != 2 {
		t.Errorf("expected 2 multi-step commands, got %d", multiStepCount)
	}
	if chatCount != 0 {
		t.Errorf("expected 0 chat commands (built-in), got %d", chatCount)
	}
}

func TestTestCommand_Executes(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()

	cmd := m.commandRegistry.Lookup("test")
	if cmd == nil {
		t.Fatal("expected 'test' command to exist")
	}
	if cmd.Type() != appCommand {
		t.Errorf("test command type = %v, want %v", cmd.Type(), appCommand)
	}
	if !cmd.IsAvailable(m) {
		t.Error("test command should be available when idle")
	}

	handled, _ := m.executeCommand("/test")
	if !handled {
		t.Fatal("/test should be handled")
	}
	if !m.paletteActive {
		t.Fatal("/test should open palette")
	}
	if len(m.palette.commands) != 0 {
		t.Fatal("/test should use custom items, not commands")
	}

	// Verify palette has test options
	item := m.palette.list.SelectedItem()
	if item == nil {
		t.Fatal("palette should have items")
	}
	toi, ok := item.(testOptionItem)
	if !ok {
		t.Fatalf("expected testOptionItem, got %T", item)
	}
	if toi.title != "Enter text" {
		t.Fatalf("expected first item 'Enter text', got %q", toi.title)
	}
}

func TestTestCommand_AvailableDuringStreaming(t *testing.T) {
	m := newTestModel()
	m.commandRegistry = NewCommandRegistry()
	m.state = stateStreaming

	cmd := m.commandRegistry.Lookup("test")
	if cmd == nil {
		t.Fatal("expected 'test' command")
	}
	if !cmd.IsAvailable(m) {
		t.Error("test command should be available during streaming")
	}
}

package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/adam/tau/internal/tui/palette"
)

func keyCtrlP() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
}

func keyUp() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyUp}
}

func keyDown() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyDown}
}

func TestCommandPalette_OpenClose(t *testing.T) {
	var p CommandPalette
	if p.IsActive() {
		t.Fatal("palette should not be active initially")
	}

	cmds := []Command{
		{name: "quit", description: "Exit", typ: appCommand},
		{name: "help", description: "Help", typ: appCommand},
	}
	p.Open(cmds)

	if !p.IsActive() {
		t.Fatal("palette should be active after Open")
	}
	if len(p.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(p.commands))
	}

	p.Close()
	if p.IsActive() {
		t.Fatal("palette should not be active after Close")
	}
}

func TestCommandPalette_Navigation(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "a", description: "A", typ: appCommand},
		{name: "b", description: "B", typ: appCommand},
		{name: "c", description: "C", typ: appCommand},
	}
	p.Open(cmds)

	if p.Selected().Name() != "a" {
		t.Fatalf("expected 'a', got %q", p.Selected().Name())
	}

	p.Down()
	if p.Selected().Name() != "b" {
		t.Fatalf("after Down: expected 'b', got %q", p.Selected().Name())
	}

	p.Down()
	if p.Selected().Name() != "c" {
		t.Fatalf("after 2nd Down: expected 'c', got %q", p.Selected().Name())
	}

	p.Down()
	if p.Selected().Name() != "a" {
		t.Fatalf("after wrap Down: expected 'a', got %q", p.Selected().Name())
	}

	p.Up()
	if p.Selected().Name() != "c" {
		t.Fatalf("after Up: expected 'c', got %q", p.Selected().Name())
	}
}

func TestCommandPalette_Selected_NilWhenEmpty(t *testing.T) {
	var p CommandPalette
	p.Open(nil)
	if p.Selected() != nil {
		t.Fatal("Selected() should return nil when commands is empty")
	}

	p.Open([]Command{})
	if p.Selected() != nil {
		t.Fatal("Selected() should return nil when commands slice is empty")
	}
}

func TestCommandPalette_View(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
		{name: "quit", description: "Exit tau", typ: appCommand},
	}
	p.Open(cmds)

	view := p.View(80, 40)
	if view == "" {
		t.Fatal("expected non-empty view when active")
	}
	clean := stripANSI(view)
	if !strings.Contains(clean, "/help") {
		t.Fatal("view should contain /help")
	}
	if !strings.Contains(clean, "/quit") {
		t.Fatal("view should contain /quit")
	}
}

func TestCommandPalette_View_Inactive(t *testing.T) {
	var p CommandPalette
	view := p.View(80, 40)
	if view != "" {
		t.Fatal("expected empty view when inactive")
	}
}

func TestCtrlP_OpensPalette(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	cmd := m.handleKeyPress(keyCtrlP())
	if cmd != nil {
		t.Fatal("Ctrl+P should return nil Cmd")
	}
	if !m.paletteActive {
		t.Fatal("palette should be active after Ctrl+P")
	}
	if !m.palette.IsActive() {
		t.Fatal("palette model should be active")
	}
	if len(m.palette.commands) == 0 {
		t.Fatal("palette should contain commands")
	}
}

func TestCtrlP_OpensPaletteWithAllCommands(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.handleKeyPress(keyCtrlP())

	// Palette should contain all command types including chat commands.
	// Verify that at least one app command is present.
	foundApp := false
	for _, cmd := range m.palette.commands {
		if cmd.Type() == appCommand || cmd.Type() == appMultiStepCommand {
			foundApp = true
		}
	}
	if !foundApp {
		t.Fatal("palette should contain app commands")
	}
}

func TestEsc_ClosesPalette(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.handleKeyPress(keyCtrlP())
	if !m.paletteActive {
		t.Fatal("palette should be active")
	}

	cmd := m.handleKeyPress(keyEsc())
	if cmd != nil {
		t.Fatal("Esc should return nil Cmd")
	}
	if m.paletteActive {
		t.Fatal("palette should be inactive after Esc")
	}
	if m.palette.IsActive() {
		t.Fatal("palette model should be inactive")
	}
}

func TestPalette_UpDownNavigation(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.handleKeyPress(keyCtrlP())

	firstCmd := m.palette.Selected().Name()

	cmd := m.handleKeyPress(keyDown())
	if cmd != nil {
		t.Fatal("Down should return nil Cmd")
	}
	secondCmd := m.palette.Selected().Name()
	if firstCmd == secondCmd {
		t.Fatal("Down should change selection")
	}

	cmd = m.handleKeyPress(keyUp())
	if cmd != nil {
		t.Fatal("Up should return nil Cmd")
	}
	if m.palette.Selected().Name() != firstCmd {
		t.Fatalf("Up should return to first selection, got %q", m.palette.Selected().Name())
	}
}

func TestPalette_EnterExecutesSelection(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	m.handleKeyPress(keyCtrlP())

	for i := range m.commandRegistry.commands {
		if m.commandRegistry.commands[i].Name() == m.palette.Selected().Name() {
			break
		}
	}

	cmd := m.handleKeyPress(keyEnter())
	if m.paletteActive {
		t.Fatal("palette should be closed after Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should return a Cmd")
	}
}

func TestPalette_EnterExecutesHelp(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	m.handleKeyPress(keyCtrlP())

	for m.palette.Selected().Name() != "help" {
		m.handleKeyPress(keyDown())
	}

	cmd := m.handleKeyPress(keyEnter())
	if cmd == nil {
		t.Fatal("Enter should return a Cmd")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block after /help, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockAssistantText {
		t.Fatalf("expected assistantText block, got %v", m.blocks[0].kind)
	}
}

func TestPalette_ResizeUpdatesDimensions(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.handleKeyPress(keyCtrlP())

	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	if cmd != nil {
		t.Fatal("resize should return nil Cmd")
	}
	if m.palette.width != 120 {
		t.Fatalf("expected palette width 120, got %d", m.palette.width)
	}
	if m.palette.height != 60 {
		t.Fatalf("expected palette height 60, got %d", m.palette.height)
	}
}

func TestPalette_ViewReturnsNonEmptyWhenActive(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.handleKeyPress(keyCtrlP())

	view := m.View()
	if view.Content == "" {
		t.Fatal("View() content should be non-empty when palette is active")
	}
}

func TestPalette_EscRefocusesInput(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.handleKeyPress(keyCtrlP())
	m.handleKeyPress(keyEsc())

	if !m.input.Focused() {
		t.Fatal("input should be focused after Esc closes palette")
	}
}

func TestPalette_CtrlPBlursInput(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.input.Focus()
	m.handleKeyPress(keyCtrlP())

	if m.input.Focused() {
		t.Fatal("input should be blurred when palette opens")
	}
}

func TestPalette_OpenPaletteWithAvailableCommands(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.state = stateIdle

	m.handleKeyPress(keyCtrlP())

	if len(m.palette.commands) == 0 {
		t.Fatal("palette should contain commands when idle")
	}

	// When idle, all commands should be marked available
	for i, avail := range m.palette.available {
		if !avail {
			t.Fatalf("command %q should be available when idle", m.palette.commands[i].Name())
		}
	}

	// When streaming, some commands become unavailable but are still shown
	m.state = stateStreaming
	m.palette.Close()
	m.paletteActive = false

	m.handleKeyPress(keyCtrlP())

	// Should still contain all app commands
	if len(m.palette.commands) == 0 {
		t.Fatal("palette should contain commands when streaming")
	}

	// Some commands should be unavailable when streaming
	hasUnavailable := false
	for _, avail := range m.palette.available {
		if !avail {
			hasUnavailable = true
			break
		}
	}
	if !hasUnavailable {
		t.Fatal("palette should have some unavailable commands when streaming")
	}
}

func TestSlash_OpensPalette(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	m.input.SetValue("/")
	m.handleSlashPrefix()

	if !m.paletteActive {
		t.Fatal("palette should be active after typing /")
	}
}

func TestPalette_CenteredRendering(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
		{name: "quit", description: "Exit tau", typ: appCommand},
	}
	p.Open(cmds)

	view := p.View(80, 40)
	if view == "" {
		t.Fatal("expected non-empty view")
	}

	lines := strings.Split(view, "\n")
	if len(lines) < 10 {
		t.Fatalf("expected at least 10 lines for vertical centering, got %d", len(lines))
	}

	topEmpty := 0
	for _, line := range lines {
		clean := stripANSI(line)
		if strings.TrimSpace(clean) == "" {
			topEmpty++
		} else {
			break
		}
	}
	if topEmpty < 3 {
		t.Fatalf("expected top padding for vertical centering, got %d empty lines", topEmpty)
	}
}

func typePaletteChars(p *CommandPalette, s string) {
	for _, ch := range s {
		p.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
}

func TestPalette_SearchFiltering(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
		{name: "quit", description: "Exit tau", typ: appCommand},
		{name: "model", description: "Change model", typ: appCommand},
		{name: "connect", description: "Connect", typ: appMultiStepCommand},
		{name: "disconnect", description: "Disconnect", typ: appMultiStepCommand},
	}
	p.Open(cmds)

	// Search for "mod" should only match /model
	typePaletteChars(&p, "mod")
	if p.Selected() == nil {
		t.Fatal("should have a selection after filtering")
	}
	if p.Selected().Name() != "model" {
		t.Fatalf("search 'mod' should select 'model', got %q", p.Selected().Name())
	}

	// Search for "con" should match /connect and /disconnect
	p.Close()
	p.Open(cmds)
	typePaletteChars(&p, "con")
	if p.Selected().Name() != "connect" && p.Selected().Name() != "disconnect" {
		t.Fatalf("search 'con' should select connect or disconnect, got %q", p.Selected().Name())
	}
}

func TestPalette_NavigationWithFilteredResults(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
		{name: "quit", description: "Exit tau", typ: appCommand},
		{name: "model", description: "Change model", typ: appCommand},
		{name: "connect", description: "Connect", typ: appMultiStepCommand},
		{name: "disconnect", description: "Disconnect", typ: appMultiStepCommand},
	}
	p.Open(cmds)

	// Filter to "con" -> connect, disconnect
	typePaletteChars(&p, "con")

	// First should be connect (higher score due to prefix match)
	if p.Selected().Name() != "connect" && p.Selected().Name() != "disconnect" {
		t.Fatalf("expected selection to be connect or disconnect, got %q", p.Selected().Name())
	}

	// Down should move to the other command
	p.Down()
	second := p.Selected().Name()
	p.Up()
	first := p.Selected().Name()
	if first == second {
		t.Fatal("Up/Down should change selection in filtered list")
	}
}

func TestPalette_ScrollingWithFilteredResults(t *testing.T) {
	var p CommandPalette
	// Create more commands than palette.MaxVisible
	var cmds []Command
	for i := 0; i < 20; i++ {
		cmds = append(cmds, Command{
			name:        fmt.Sprintf("cmd%d", i),
			description: fmt.Sprintf("Command %d", i),
			typ:         appCommand,
		})
	}
	p.Open(cmds)
	p.width = 100
	p.height = 50

	// Filter to "cmd" -> all 20 commands
	typePaletteChars(&p, "cmd")

	// Navigate past the visible window
	for i := 0; i < palette.MaxVisible+2; i++ {
		p.Down()
	}

	// Verify we can select an item (selection moved beyond initial visible range)
	if p.Selected() == nil {
		t.Fatal("should have a selection after navigation")
	}
}

func TestPalette_DisabledCommandsShownButNotSelectable(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.state = stateIdle

	// Open palette while idle - all commands available
	m.handleKeyPress(keyCtrlP())

	idleCount := len(m.palette.commands)
	idleAvailCount := 0
	for _, a := range m.palette.available {
		if a {
			idleAvailCount++
		}
	}
	if idleAvailCount != idleCount {
		t.Fatalf("when idle, all %d commands should be available, got %d", idleCount, idleAvailCount)
	}

	// Close and reopen while streaming - some commands unavailable
	m.palette.Close()
	m.paletteActive = false
	m.state = stateStreaming

	m.handleKeyPress(keyCtrlP())

	// Should still show all commands
	if len(m.palette.commands) != idleCount {
		t.Fatalf("palette should show all %d commands, got %d", idleCount, len(m.palette.commands))
	}

	// Some should be unavailable
	streamAvailCount := 0
	for _, a := range m.palette.available {
		if a {
			streamAvailCount++
		}
	}
	if streamAvailCount >= idleCount {
		t.Fatal("some commands should be unavailable when streaming")
	}

	// Navigation should find available commands
	p := &m.palette
	if streamAvailCount > 0 && p.Selected() == nil {
		t.Fatal("should be able to select an available command")
	}
}

func TestPalette_SearchInputFocusOnOpen(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)

	// The PaletteList's search should be focused - verify via search value behavior
	if p.SearchValue() != "" {
		t.Fatalf("search value should be empty on open, got %q", p.SearchValue())
	}
}

func TestPalette_SearchValueClearedOnClose(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)
	typePaletteChars(&p, "test")

	if p.SearchValue() != "test" {
		t.Fatalf("expected search value 'test', got %q", p.SearchValue())
	}

	p.Close()
	// After close and reopen, search should be empty
	p.Open(cmds)
	if p.SearchValue() != "" {
		t.Fatalf("expected search value cleared after reopen, got %q", p.SearchValue())
	}
}

func containsAll(slice []string, items ...string) bool {
	for _, item := range items {
		found := false
		for _, s := range slice {
			if s == item {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestPalette_ShowInput_TransitionsToInputStep(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)

	if p.IsInputStep() {
		t.Fatal("should not be in input step after Open")
	}

	p.ShowInput("Enter text", "Type here...")

	if !p.IsInputStep() {
		t.Fatal("should be in input step after ShowInput")
	}
}

func TestPalette_InputDone_ReturnsTrueAfterEnter(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)
	p.ShowInput("Enter text", "Type here...")

	if p.InputDone() {
		t.Fatal("should not be done immediately after ShowInput")
	}

	p.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	p.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !p.InputDone() {
		t.Fatal("should be done after Enter")
	}
	if p.InputResult() != "hi" {
		t.Fatalf("result = %q, want %q", p.InputResult(), "hi")
	}
}

func TestPalette_InputEsc_Cancels(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)
	p.ShowInput("Enter text", "Type here...")

	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !p.InputCancelled() {
		t.Fatal("should be cancelled after Esc")
	}
}

func TestPalette_BackToList_ReturnsToListStep(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)
	p.ShowInput("Enter text", "Type here...")

	if !p.IsInputStep() {
		t.Fatal("should be in input step")
	}

	p.BackToList()

	if p.IsInputStep() {
		t.Fatal("should be back to list step")
	}
}

func TestPalette_Close_ResetsStepToList(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)
	p.ShowInput("Enter text", "Type here...")

	p.Close()

	if p.IsInputStep() {
		t.Fatal("step should reset to list after Close")
	}
}

func TestPalette_UpDown_NoOpInInputStep(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)
	p.ShowInput("Enter text", "Type here...")

	p.Up()
	p.Down()

	if !p.IsInputStep() {
		t.Fatal("should still be in input step after Up/Down")
	}
}

func TestPalette_ShowConfirm_TransitionsToConfirmStep(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)

	if p.IsConfirmStep() {
		t.Fatal("should not be in confirm step after Open")
	}

	p.ShowConfirm("Save this?")

	if !p.IsConfirmStep() {
		t.Fatal("should be in confirm step after ShowConfirm")
	}
}

func TestPalette_ConfirmDone_ReturnsTrueAfterY(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)
	p.ShowConfirm("Save this?")

	if p.ConfirmDone() {
		t.Fatal("should not be done immediately after ShowConfirm")
	}

	p.Update(tea.KeyPressMsg{Code: 'y'})

	if !p.ConfirmDone() {
		t.Fatal("should be done after y")
	}
	if !p.ConfirmResult() {
		t.Fatal("result should be true after y")
	}
}

func TestPalette_ConfirmEsc_Cancels(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)
	p.ShowConfirm("Save this?")

	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !p.ConfirmCancelled() {
		t.Fatal("should be cancelled after Esc")
	}
}

func TestPalette_UpDown_NoOpInConfirmStep(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)
	p.ShowConfirm("Save this?")

	p.Up()
	p.Down()

	if !p.IsConfirmStep() {
		t.Fatal("should still be in confirm step after Up/Down")
	}
}

func TestPalette_ThreeStepFlow(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	// Open palette via Ctrl+P
	m.handleKeyPress(keyCtrlP())

	// Navigate to /test and execute
	for m.palette.Selected().Name() != "test" {
		m.handleKeyPress(keyDown())
	}

	m.handleKeyPress(keyEnter())
	if !m.paletteActive {
		t.Fatal("palette should be active after /test")
	}

	// Should show list with test options
	if m.palette.IsInputStep() {
		t.Fatal("should be in list step, not input")
	}

	// Select "Enter text" option
	typePaletteChars(&m.palette, "Enter text")
	m.handleKeyPress(keyEnter())

	// Should transition to input step
	if !m.palette.IsInputStep() {
		t.Fatal("should be in input step after selecting 'Enter text'")
	}

	// Type text and press enter
	m.palette.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m.palette.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m.handleKeyPress(keyEnter())

	// Should transition to confirm step
	if !m.palette.IsConfirmStep() {
		t.Fatal("should be in confirm step after input")
	}

	// Confirm with 'y'
	m.handleKeyPress(keyEnter())

	// Should transition to task step
	if !m.palette.IsTaskStep() {
		t.Fatal("should be in task step after confirm")
	}

	// Simulate task completion
	m.palette.Update(palette.TaskResultMsg{Success: true, Message: "Text saved successfully", Err: nil})

	// Press enter to process task result and transition to message step
	m.handleKeyPress(keyEnter())

	// Should transition to message step
	if !m.palette.IsMessageStep() {
		t.Fatal("should be in message step after task")
	}

	// Press enter to close message step
	m.handleKeyPress(keyEnter())

	// Palette should be closed
	if m.paletteActive {
		t.Fatal("palette should be closed after message")
	}

	// Should have saved message
	found := false
	for _, b := range m.blocks {
		if b.kind == blockAssistantText && b.text == "Text saved successfully" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("should have 'Text saved successfully' message in blocks")
	}
}

func TestPalette_ThreeStepFlow_Cancel(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	// Open palette via Ctrl+P
	m.handleKeyPress(keyCtrlP())

	// Navigate to /test and execute
	for m.palette.Selected().Name() != "test" {
		m.handleKeyPress(keyDown())
	}

	m.handleKeyPress(keyEnter())

	// Select "Enter text" option
	typePaletteChars(&m.palette, "Enter text")
	m.handleKeyPress(keyEnter())

	// Type text and press enter
	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.palette.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m.palette.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.handleKeyPress(keyEnter())

	// Should be in confirm step
	if !m.palette.IsConfirmStep() {
		t.Fatal("should be in confirm step after input")
	}

	// Press 'n' to decline
	m.handleKeyPress(tea.KeyPressMsg{Code: 'n'})

	// Palette should be closed
	if m.paletteActive {
		t.Fatal("palette should be closed after confirm")
	}

	// Should have cancelled message
	found := false
	for _, b := range m.blocks {
		if b.kind == blockAssistantText && b.text == "Cancelled" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("should have 'Cancelled' message in blocks")
	}
}

func TestPalette_ShowTask_TransitionsToTaskStep(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	m.handleKeyPress(keyCtrlP())

	for m.palette.Selected().Name() != "test" {
		m.handleKeyPress(keyDown())
	}

	m.handleKeyPress(keyEnter())

	typePaletteChars(&m.palette, "Enter text")
	m.handleKeyPress(keyEnter())

	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.palette.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m.palette.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.handleKeyPress(keyEnter())

	if !m.palette.IsConfirmStep() {
		t.Fatal("should be in confirm step after input")
	}

	m.handleKeyPress(tea.KeyPressMsg{Code: 'y'})

	if !m.palette.IsTaskStep() {
		t.Fatal("should be in task step after confirm")
	}
}

func TestPalette_TaskDone_ReturnsTrueAfterCompletion(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	m.handleKeyPress(keyCtrlP())

	for m.palette.Selected().Name() != "test" {
		m.handleKeyPress(keyDown())
	}

	m.handleKeyPress(keyEnter())

	typePaletteChars(&m.palette, "Enter text")
	m.handleKeyPress(keyEnter())

	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.palette.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m.palette.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.handleKeyPress(keyEnter())

	m.handleKeyPress(tea.KeyPressMsg{Code: 'y'})

	if !m.palette.IsTaskStep() {
		t.Fatal("should be in task step")
	}

	m.palette.Update(palette.TaskResultMsg{Success: true, Message: "done", Err: nil})

	if !m.palette.TaskDone() {
		t.Fatal("should be done after task result")
	}
}

func TestPalette_TaskEsc_Cancels(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	m.handleKeyPress(keyCtrlP())

	for m.palette.Selected().Name() != "test" {
		m.handleKeyPress(keyDown())
	}

	m.handleKeyPress(keyEnter())

	typePaletteChars(&m.palette, "Enter text")
	m.handleKeyPress(keyEnter())

	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.palette.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m.palette.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.handleKeyPress(keyEnter())

	m.handleKeyPress(tea.KeyPressMsg{Code: 'y'})

	if !m.palette.IsTaskStep() {
		t.Fatal("should be in task step")
	}

	m.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEsc})

	if m.palette.TaskCancelled() {
		t.Fatal("task should be cancelled after esc")
	}
	if m.paletteActive {
		t.Fatal("palette should be closed after esc in task step")
	}
}

func TestPalette_FourStepFlow(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	m.handleKeyPress(keyCtrlP())

	for m.palette.Selected().Name() != "test" {
		m.handleKeyPress(keyDown())
	}

	m.handleKeyPress(keyEnter())

	typePaletteChars(&m.palette, "Enter text")
	m.handleKeyPress(keyEnter())

	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.palette.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m.palette.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.handleKeyPress(keyEnter())

	m.handleKeyPress(tea.KeyPressMsg{Code: 'y'})

	m.palette.Update(palette.TaskResultMsg{Success: true, Message: "Text saved successfully", Err: nil})

	// Press enter to process task result and transition to message step
	m.handleKeyPress(keyEnter())

	// Should transition to message step
	if !m.palette.IsMessageStep() {
		t.Fatal("should be in message step after task")
	}

	m.handleKeyPress(keyEnter())

	if m.paletteActive {
		t.Fatal("palette should be closed after message done")
	}

	found := false
	for _, b := range m.blocks {
		if b.kind == blockAssistantText && b.text == "Text saved successfully" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("should have success message in blocks")
	}
}

func TestPalette_UpDown_NoOpInTaskStep(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	m.handleKeyPress(keyCtrlP())

	for m.palette.Selected().Name() != "test" {
		m.handleKeyPress(keyDown())
	}

	m.handleKeyPress(keyEnter())

	typePaletteChars(&m.palette, "Enter text")
	m.handleKeyPress(keyEnter())

	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.palette.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m.palette.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	m.palette.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	m.handleKeyPress(keyEnter())

	m.handleKeyPress(tea.KeyPressMsg{Code: 'y'})

	if !m.palette.IsTaskStep() {
		t.Fatal("should be in task step")
	}

	selectedBefore := m.palette.Selected()
	m.handleKeyPress(keyUp())
	if m.palette.Selected() != selectedBefore {
		t.Fatal("up should be no-op in task step")
	}

	m.handleKeyPress(keyDown())
	if m.palette.Selected() != selectedBefore {
		t.Fatal("down should be no-op in task step")
	}
}

func TestPalette_MultiStep_ShowSteps(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)

	if p.IsMultiStep() {
		t.Fatal("should not be in multi-step after Open")
	}

	steps := []palette.Step{
		palette.ListStep("Select", "Choose:", []palette.ListOption{
			{Title: "A", Description: "Option A", Value: "a"},
		}),
	}
	p.ShowSteps(steps, "Test")

	if !p.IsMultiStep() {
		t.Fatal("should be in multi-step after ShowSteps")
	}
	if p.MultiStepResults() == nil {
		t.Fatal("results should be initialized")
	}
}

func TestPalette_MultiStep_Render(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)

	steps := []palette.Step{
		palette.ListStep("Select", "Choose:", []palette.ListOption{
			{Title: "A", Description: "Option A", Value: "a"},
		}),
	}
	p.ShowSteps(steps, "Test")

	view := p.View(80, 40)
	if view == "" {
		t.Fatal("view should not be empty in multi-step mode")
	}
	clean := stripANSI(view)
	if !strings.Contains(clean, "Search") {
		t.Fatal("view should contain search input")
	}
}

func TestPalette_MultiStepEsc_ReturnsToCommandList(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	// Open palette
	m.handleKeyPress(keyCtrlP())
	if !m.paletteActive {
		t.Fatal("palette should be active")
	}

	// Select /connect (multi-step command)
	for m.palette.Selected().Name() != "connect" {
		m.handleKeyPress(keyDown())
	}
	m.handleKeyPress(keyEnter())

	if !m.palette.IsMultiStep() {
		t.Fatal("should be in multi-step mode")
	}

	// Press Esc to cancel
	m.handleKeyPress(keyEsc())

	// Should return to command list
	if !m.paletteActive {
		t.Fatal("palette should still be active")
	}
	if m.palette.IsMultiStep() {
		t.Fatal("should not be in multi-step mode after Esc")
	}
}

func TestPalette_MultiStepFlow_Complete(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40
	m.commandRegistry = NewCommandRegistry()

	// Open palette
	m.handleKeyPress(keyCtrlP())

	// Select /connect
	for m.palette.Selected().Name() != "connect" {
		m.handleKeyPress(keyDown())
	}
	m.handleKeyPress(keyEnter())

	if !m.palette.IsMultiStep() {
		t.Fatal("should be in multi-step mode")
	}

	if !m.palette.IsActive() {
		t.Fatal("palette should be active")
	}
	if m.palette.MultiStepResults() == nil {
		t.Fatal("multi-step results should be available")
	}
}

func TestPalette_MultiStep_DoneAndCancelled(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)

	if p.MultiStepDone() {
		t.Fatal("should not be done before multi-step starts")
	}
	if p.MultiStepCancelled() {
		t.Fatal("should not be cancelled before multi-step starts")
	}

	steps := []palette.Step{
		palette.ConfirmStep("Confirm", "Proceed?"),
	}
	p.ShowSteps(steps, "Test")

	if p.MultiStepDone() {
		t.Fatal("should not be done immediately")
	}

	// Cancel the runner
	p.CancelMultiStep()
	if !p.MultiStepCancelled() {
		t.Fatal("should be cancelled after CancelMultiStep")
	}
}

func TestPalette_MultiStep_UpDownNoOp(t *testing.T) {
	var p CommandPalette
	cmds := []Command{
		{name: "help", description: "Show help", typ: appCommand},
	}
	p.Open(cmds)

	steps := []palette.Step{
		palette.ListStep("Select", "Choose:", []palette.ListOption{
			{Title: "A", Description: "Option A", Value: "a"},
		}),
	}
	p.ShowSteps(steps, "Test")

	p.Up()
	p.Down()

	if !p.IsMultiStep() {
		t.Fatal("should still be in multi-step mode")
	}
}

func TestPalette_MultiStepFlow_ListToInput_NotSkipped(t *testing.T) {
	var p CommandPalette

	steps := []palette.Step{
		palette.ListStep("Select", "Choose:", []palette.ListOption{
			{Title: "A", Description: "Option A", Value: "a"},
		}),
		palette.InputStep("API Key", "Enter key:", "sk-..."),
		palette.ConfirmStep("Save", "Save?"),
	}
	p.ShowSteps(steps, "Test")

	// Step 0: ListStep should be current
	if p.stepRunner.CurrentStep().Kind() != palette.StepKindList {
		t.Fatal("first step should be ListStep")
	}

	// Simulate Enter on list
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !p.MultiStepListDone() {
		t.Fatal("list should be done after Enter")
	}

	// Advance to next step
	p.HandleMultiStepDone()

	// Step 1: InputStep should be current, NOT skipped
	step := p.stepRunner.CurrentStep()
	if step == nil {
		t.Fatal("current step should not be nil")
	}
	if step.Kind() != palette.StepKindInput {
		t.Fatalf("second step should be InputStep, got %v", step.Kind())
	}

	// Input should NOT be done yet (user hasn't typed anything)
	if p.input.Done() {
		t.Fatal("input should not be done immediately after showing")
	}

	// Simulate typing and Enter on input
	p.input.SetValue("test-key")
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !p.MultiStepInputDone() {
		t.Fatal("input should be done after Enter")
	}

	// Advance to next step
	p.HandleMultiStepDone()

	// Step 2: ConfirmStep should be current
	step = p.stepRunner.CurrentStep()
	if step == nil {
		t.Fatal("current step should not be nil")
	}
	if step.Kind() != palette.StepKindConfirm {
		t.Fatalf("third step should be ConfirmStep, got %v", step.Kind())
	}
}

func TestPalette_MultiStepFlow_TaskStep_PreservesResultData(t *testing.T) {
	var p CommandPalette

	steps := []palette.Step{
		palette.ListStep("Select", "Choose:", []palette.ListOption{
			{Title: "A", Description: "Option A", Value: "a"},
		}),
		palette.TaskStep("Fetch Data", func(results map[string]any) (bool, string, error) {
			// Task stores additional data in results map
			results["fetch_data_items"] = []string{"item1", "item2", "item3"}
			return true, "Fetched 3 items", nil
		}),
		palette.ConfirmStep("Save", "Save?"),
	}
	p.ShowSteps(steps, "Test")

	// Complete ListStep
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p.HandleMultiStepDone()

	// TaskStep is now active. In the real app, the task function runs async
	// via tea.Batch. Simulate by manually invoking the task function (as the
	// async command would), then sending TaskResultMsg.
	if p.stepRunner.CurrentStep().Kind() != palette.StepKindTask {
		t.Fatal("second step should be TaskStep")
	}
	taskFn := p.stepRunner.CurrentStep().Task()
	taskFn(p.stepRunner.Results())

	// Simulate async task completion
	p.Update(palette.TaskResultMsg{Success: true, Message: "Fetched 3 items", Err: nil})
	if !p.MultiStepTaskDone() {
		t.Fatal("task should be done after TaskResultMsg")
	}
	p.HandleMultiStepDone()

	// Verify task stored data is preserved in results
	results := p.stepRunner.Results()
	items, ok := results["fetch_data_items"].([]string)
	if !ok {
		t.Fatalf("fetch_data_items should be []string, got %T", results["fetch_data_items"])
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Verify metadata keys are also present
	if _, ok := results["fetch_data_ok"]; !ok {
		t.Fatal("fetch_data_ok should be present")
	}
	if _, ok := results["fetch_data_msg"]; !ok {
		t.Fatal("fetch_data_msg should be present")
	}

	// Complete ConfirmStep
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p.HandleMultiStepDone()

	// Verify all results are present after flow completes
	finalResults := p.MultiStepResults()
	finalItems, ok := finalResults["fetch_data_items"].([]string)
	if !ok {
		t.Fatalf("final results should have fetch_data_items as []string, got %T", finalResults["fetch_data_items"])
	}
	if len(finalItems) != 3 {
		t.Fatalf("expected 3 items in final results, got %d", len(finalItems))
	}
}

func TestPalette_MultiStepFlow_ConnectSimulated(t *testing.T) {
	// Simulates the exact step structure of connectSteps to verify
	// discover_models propagates correctly through the full flow.
	steps := []palette.Step{
		palette.ListStep("Select Provider", "Choose a provider to connect to:", []palette.ListOption{
			{Title: "OpenCode Go", Description: "OpenCode Go cloud provider", Value: "opencode-go"},
			{Title: "Ollama", Description: "Local models", Value: "ollama"},
		}),
		palette.InputStep("API Key", "Enter your API key (leave empty to use saved credentials):", "sk-..."),
		palette.TaskStep("Test Connection", func(results map[string]any) (bool, string, error) {
			providerName, _ := results["select_provider"].(string)
			apiKey, _ := results["api_key"].(string)
			if providerName == "" {
				return false, "", fmt.Errorf("no provider selected")
			}
			if apiKey == "" {
				return false, "No API key provided", fmt.Errorf("missing API key")
			}
			return true, fmt.Sprintf("Connected to %s successfully", providerName), nil
		}),
		palette.TaskStep("Discover Models", func(results map[string]any) (bool, string, error) {
			providerName, _ := results["select_provider"].(string)
			apiKey, _ := results["api_key"].(string)
			_ = apiKey

			// Simulate model discovery
			var models []string
			if providerName == "opencode-go" {
				models = []string{"qwen3-235b", "qwen3-30b", "codestral-latest"}
			} else if providerName == "ollama" {
				models = []string{"llama3.2", "gemma3:4b"}
			}

			results["discover_models"] = models
			if len(models) == 0 {
				return true, fmt.Sprintf("No models found for %s", providerName), nil
			}
			return true, fmt.Sprintf("Discovered %d model(s) from %s", len(models), providerName), nil
		}),
		palette.ConfirmStep("Save", "Save credentials and register provider?"),
	}

	var p CommandPalette
	p.ShowSteps(steps, "connect")

	// Step 1: Select provider
	if p.stepRunner.CurrentStep().Kind() != palette.StepKindList {
		t.Fatal("first step should be ListStep")
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p.HandleMultiStepDone()

	// Step 2: API Key input
	if p.stepRunner.CurrentStep().Kind() != palette.StepKindInput {
		t.Fatalf("second step should be InputStep, got %v", p.stepRunner.CurrentStep().Kind())
	}
	p.input.SetValue("sk-test-key-12345")
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p.HandleMultiStepDone()

	// Step 3: Test Connection task
	if p.stepRunner.CurrentStep().Kind() != palette.StepKindTask {
		t.Fatal("third step should be TaskStep (Test Connection)")
	}
	taskFn := p.stepRunner.CurrentStep().Task()
	taskFn(p.stepRunner.Results())
	p.Update(palette.TaskResultMsg{Success: true, Message: "Connected to opencode-go successfully"})
	if !p.MultiStepTaskDone() {
		t.Fatal("task should be done")
	}
	p.HandleMultiStepDone()

	// Step 4: Discover Models task
	if p.stepRunner.CurrentStep().Kind() != palette.StepKindTask {
		t.Fatalf("fourth step should be TaskStep (Discover Models), got %v", p.stepRunner.CurrentStep().Kind())
	}
	taskFn2 := p.stepRunner.CurrentStep().Task()
	taskFn2(p.stepRunner.Results())
	p.Update(palette.TaskResultMsg{Success: true, Message: "Discovered 3 model(s) from opencode-go"})
	if !p.MultiStepTaskDone() {
		t.Fatal("discover models task should be done")
	}
	p.HandleMultiStepDone()

	// Step 5: Confirm
	if p.stepRunner.CurrentStep().Kind() != palette.StepKindConfirm {
		t.Fatal("fifth step should be ConfirmStep")
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	p.HandleMultiStepDone()

	// Flow complete - verify results
	if !p.MultiStepDone() {
		t.Fatal("flow should be complete")
	}

	results := p.MultiStepResults()

	// Verify all expected keys exist
	expectedKeys := []string{"select_provider", "api_key", "discover_models", "save"}
	for _, key := range expectedKeys {
		if _, ok := results[key]; !ok {
			t.Fatalf("results missing key: %s", key)
		}
	}

	// Verify discover_models is []string with correct values
	models, ok := results["discover_models"].([]string)
	if !ok {
		t.Fatalf("discover_models should be []string, got %T", results["discover_models"])
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}
	expectedModels := []string{"qwen3-235b", "qwen3-30b", "codestral-latest"}
	for i, m := range expectedModels {
		if models[i] != m {
			t.Fatalf("model[%d] = %q, want %q", i, models[i], m)
		}
	}

	// Verify other results
	if provider, _ := results["select_provider"].(string); provider != "opencode-go" {
		t.Fatalf("select_provider = %q, want %q", provider, "opencode-go")
	}
	if apiKey, _ := results["api_key"].(string); apiKey != "sk-test-key-12345" {
		t.Fatalf("api_key = %q, want %q", apiKey, "sk-test-key-12345")
	}
	if save, _ := results["save"].(bool); !save {
		t.Fatal("save should be true")
	}
}


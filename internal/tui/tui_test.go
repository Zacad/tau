package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/adam/tau/internal/sdk"
	"github.com/adam/tau/internal/testutil"
	"github.com/adam/tau/internal/types"
)

// keyCtrlC returns a KeyPressMsg representing Ctrl+C.
func keyCtrlC() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
}

// keyCtrlD returns a KeyPressMsg representing Ctrl+D.
func keyCtrlD() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}
}

// keyEnter returns a KeyPressMsg representing Enter.
func keyEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}

// keyEsc returns a KeyPressMsg representing Esc.
func keyEsc() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEsc}
}

// keyA returns a KeyPressMsg representing 'a'.
func keyA() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'a'}
}

// keyShiftEnter returns a KeyPressMsg representing Shift+Enter.
func keyShiftEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
}

// keyCtrlJ returns a KeyPressMsg representing Ctrl+J.
func keyCtrlJ() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl}
}

// TestCtrlC_DoubleTapToExit verifies the Ctrl+C double-tap exit pattern.
func TestCtrlC_DoubleTapToExit(t *testing.T) {
	m := newTestModel()

	// Model starts idle, not pending
	if m.state != stateIdle {
		t.Fatalf("expected stateIdle, got %v", m.state)
	}
	if m.pendingExit {
		t.Fatal("expected pendingExit=false at start")
	}

	// First Ctrl+C: should set pendingExit, return nil Cmd
	cmd := m.handleKeyPress(keyCtrlC())
	if !m.pendingExit {
		t.Fatal("first Ctrl+C should set pendingExit=true")
	}
	if cmd != nil {
		t.Fatal("first Ctrl+C should return nil Cmd (just set pending)")
	}

	// Second Ctrl+C: should return tea.Quit
	cmd = m.handleKeyPress(keyCtrlC())
	if cmd == nil {
		t.Fatal("second Ctrl+C should return a Cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", msg)
	}
}

// TestCtrlC_SingleTapDuringStreaming verifies Ctrl+C during streaming cancels
// without setting pendingExit.
func TestCtrlC_SingleTapDuringStreaming(t *testing.T) {
	m := newTestModel()
	m.state = stateStreaming

	called := false
	m.cancelFunc = func() { called = true }

	cmd := m.handleKeyPress(keyCtrlC())

	if !called {
		t.Fatal("cancelFunc should have been called")
	}
	if m.pendingExit {
		t.Fatal("streaming Ctrl+C should NOT set pendingExit")
	}
	if cmd != nil {
		t.Fatal("streaming Ctrl+C should return nil")
	}
}

// TestCtrlC_PendingClearedByOtherKey verifies that pressing any non-Ctrl+C key
// clears the pending exit state.
func TestCtrlC_PendingClearedByOtherKey(t *testing.T) {
	m := newTestModel()

	// First Ctrl+C sets pending
	m.handleKeyPress(keyCtrlC())
	if !m.pendingExit {
		t.Fatal("expected pendingExit=true after first Ctrl+C")
	}

	// Pressing 'a' should clear pending
	m.handleKeyPress(keyA())
	if m.pendingExit {
		t.Fatal("pendingExit should be cleared after pressing 'a'")
	}

	// Now Ctrl+C should start fresh (set pending again, not exit)
	cmd := m.handleKeyPress(keyCtrlC())
	if cmd != nil {
		t.Fatal("fresh Ctrl+C after clearing should NOT exit")
	}
}

// TestCtrlC_EnterClearsPending verifies that pressing Enter while
// pendingExit clears the pending state.
func TestCtrlC_EnterClearsPending(t *testing.T) {
	m := newTestModel()

	// Set pending
	m.handleKeyPress(keyCtrlC())
	if !m.pendingExit {
		t.Fatal("expected pendingExit=true")
	}

	// Enter with empty input does NOT submit, but still
	// clears pendingExit via the "clear on non-Ctrl+C key" logic.
	m.handleKeyPress(keyEnter())
	if m.pendingExit {
		t.Fatal("pendingExit should be cleared when Enter is pressed")
	}
}

// TestCtrlC_EscClearsPending verifies that pressing Esc while pending clears it.
func TestCtrlC_EscClearsPending(t *testing.T) {
	m := newTestModel()

	m.handleKeyPress(keyCtrlC())
	if !m.pendingExit {
		t.Fatal("expected pendingExit=true")
	}

	m.handleKeyPress(keyEsc())
	if m.pendingExit {
		t.Fatal("pendingExit should be cleared when Esc is pressed")
	}
}

// TestCtrlD_ExitWhenIdle verifies Ctrl+D exits when input is empty and idle.
func TestCtrlD_ExitWhenIdle(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("")

	cmd := m.handleKeyPress(keyCtrlD())
	if cmd == nil {
		t.Fatal("Ctrl+D with empty input should return a Cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", msg)
	}
}

// TestCtrlD_ExitWithNonEmptyInput verifies Ctrl+D quits even when input has text.
func TestCtrlD_ExitWithNonEmptyInput(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("hello")

	cmd := m.handleKeyPress(keyCtrlD())
	if cmd == nil {
		t.Fatal("Ctrl+D with non-empty input should return a command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatal("Ctrl+D with non-empty input should quit")
	}
}

// TestCtrlD_NoExitWhenStreaming verifies Ctrl+D does NOT exit during streaming.
func TestCtrlD_NoExitWhenStreaming(t *testing.T) {
	m := newTestModel()
	m.state = stateStreaming

	// Even with empty input, streaming blocks Ctrl+D
	m.input.SetValue("")
	cmd := m.handleKeyPress(keyCtrlD())
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatal("Ctrl+D during streaming should NOT quit")
		}
	}
}

// TestSlashCommands verifies slash command processing.
func TestSlashCommands(t *testing.T) {
	m := newTestModel()

	tests := []struct {
		input   string
		handled bool
		isQuit  bool
	}{
		{"/quit", true, true},
		{"/exit", true, true},
		{"/help", true, false},
		{"/unknown", false, false},
		{"not a command", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			handled, cmd := m.executeCommand(tt.input)
			if handled != tt.handled {
				t.Errorf("handled=%v, want %v", handled, tt.handled)
			}
			if tt.isQuit {
				if cmd == nil {
					t.Fatal("expected Quit Cmd")
				}
				msg := cmd()
				if _, ok := msg.(tea.QuitMsg); !ok {
					t.Fatalf("expected QuitMsg, got %T", msg)
				}
			}
		})
	}
}

// TestEscClearsInput verifies that Esc clears the input when idle.
func TestExecuteCommand_NewResetsFooterContextUsage(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json with an API key so session model resolution succeeds.
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json", []byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s, err := sdk.CreateSession(context.Background(), sdk.SessionOptions{
		Model:      "openai/gpt-4o",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 120
	m.height = 40
	m.contextWindow = 200000
	m.contextTokens = 54321
	m.contextKnown = true
	m.turnCount = 3
	m.usage = types.Usage{TotalTokens: 999}

	before := stripANSI(m.renderFooter())
	if !strings.Contains(before, "ctx:27.2%/200k") {
		t.Fatalf("expected precondition footer to show previous context usage, got %q", before)
	}

	handled, cmd := m.executeCommand("/new")
	if !handled {
		t.Fatal("/new should be handled")
	}
	if cmd != nil {
		t.Fatal("/new should not return a tea.Cmd")
	}

	if m.contextTokens != 0 {
		t.Fatalf("contextTokens = %d, want 0", m.contextTokens)
	}
	if !m.contextKnown {
		t.Fatal("contextKnown should remain true for an empty session with known context window")
	}
	if m.turnCount != 0 {
		t.Fatalf("turnCount = %d, want 0", m.turnCount)
	}
	if m.usage.TotalTokens != 0 {
		t.Fatalf("usage.TotalTokens = %d, want 0", m.usage.TotalTokens)
	}

	expectedCtx := fmt.Sprintf("ctx:0%%/%s", formatTokens(m.session.Model().ContextWindow))
	after := stripANSI(m.renderFooter())
	if strings.Contains(after, "ctx:27.2%/200k") {
		t.Fatalf("footer should not show stale context usage after /new: %q", after)
	}
	if !strings.Contains(after, expectedCtx) {
		t.Fatalf("footer should show reset context usage after /new, got %q (want substring %q)", after, expectedCtx)
	}
}

func TestEscClearsInput(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("hello")

	cmd := m.handleKeyPress(keyEsc())
	if m.input.Value() != "" {
		t.Fatalf("expected empty input after Esc, got %q", m.input.Value())
	}
	if cmd != nil {
		t.Fatal("Esc should return nil Cmd")
	}
}

// TestEscNoOpWhenStreaming verifies Esc does NOT clear input during streaming.
func TestEscNoOpWhenStreaming(t *testing.T) {
	m := newTestModel()
	m.state = stateStreaming
	m.input.SetValue("hello")

	m.handleKeyPress(keyEsc())
	if m.input.Value() != "hello" {
		t.Fatal("Esc during streaming should NOT clear input")
	}
}

// TestReturnToIdleClearsPending verifies that returnToIdle() resets pendingExit.
func TestReturnToIdleClearsPending(t *testing.T) {
	m := newTestModel()
	m.pendingExit = true

	m.returnToIdle()
	if m.pendingExit {
		t.Fatal("returnToIdle should clear pendingExit")
	}
}

// TestNoPendingExitWhenNotIdle verifies that Ctrl+C during streaming does not
// set pendingExit even if the user then presses Ctrl+C again.
func TestNoPendingExitAfterStreamingAbort(t *testing.T) {
	m := newTestModel()
	m.state = stateStreaming
	cancelled := false
	m.cancelFunc = func() { cancelled = true }

	// Ctrl+C during streaming: cancel only
	m.handleKeyPress(keyCtrlC())
	if !cancelled {
		t.Fatal("cancelFunc should have been called")
	}
	if m.pendingExit {
		t.Fatal("streaming Ctrl+C should NOT set pendingExit")
	}

	// Simulate return to idle
	m.returnToIdle()

	// Now idle, first Ctrl+C sets pending
	m.handleKeyPress(keyCtrlC())
	if !m.pendingExit {
		t.Fatal("idle Ctrl+C should set pendingExit")
	}

	// Second Ctrl+C exits
	cmd := m.handleKeyPress(keyCtrlC())
	if cmd == nil {
		t.Fatal("second Ctrl+C should return tea.Quit")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", msg)
	}
}

// newTestModel creates a minimal Model for testing key handling logic.
func newTestModel() *Model {
	vp := viewport.New()
	ta := textarea.New()
	ta.Focus()
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = 8

	r, _ := NewRenderer(80)

	m := &Model{
		state:           stateIdle,
		viewport:        vp,
		input:           ta,
		modelName:       "test-model",
		cwd:             "/test",
		pendingBuilder:  new(strings.Builder),
		glamourRenderer: r,
		commandRegistry: NewCommandRegistry(),
	}

	_ = m.commandRegistry.LoadCustomCommands("/test", EmbeddedCommands())

	return m
}

// TestShiftEnter_InsertsNewline verifies Shift+Enter inserts a newline.
func TestShiftEnter_InsertsNewline(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("hello")

	cmd := m.handleKeyPress(keyShiftEnter())
	if cmd != nil {
		t.Fatal("Shift+Enter should return nil")
	}
	if m.input.Value() != "hello\n" {
		t.Fatalf("expected 'hello\\n', got %q", m.input.Value())
	}
}

func TestPaletteInputReceivesPasteMsg(t *testing.T) {
	m := newTestModel()
	m.input.Focus()
	m.paletteActive = true
	m.palette.active = true
	m.palette.ShowInput("web-research", "Perform deep web research")

	updated, _ := m.Update(tea.PasteMsg{Content: "tau custom commands"})

	model, ok := updated.(*Model)
	if !ok {
		t.Fatalf("updated model type = %T, want *Model", updated)
	}
	if got := model.palette.input.Value(); got != "tau custom commands" {
		t.Fatalf("palette input value = %q, want pasted content", got)
	}
	if got := model.input.Value(); got != "" {
		t.Fatalf("main input value = %q, want empty", got)
	}
}

// TestShiftEnter_InsertsNewlineDuringStreaming verifies Shift+Enter during streaming inserts newline.
func TestShiftEnter_InsertsNewlineDuringStreaming(t *testing.T) {
	m := newTestModel()
	m.state = stateStreaming
	m.input.SetValue("hello")

	cmd := m.handleKeyPress(keyShiftEnter())
	if cmd != nil {
		t.Fatal("Shift+Enter during streaming should return nil")
	}
	if m.input.Value() != "hello\n" {
		t.Fatalf("expected 'hello\\n', got %q", m.input.Value())
	}
}

// TestCtrlJ_InsertsNewline verifies Ctrl+J inserts a newline.
func TestCtrlJ_InsertsNewline(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("hello")

	cmd := m.handleKeyPress(keyCtrlJ())
	if cmd != nil {
		t.Fatal("Ctrl+J should return nil")
	}
	if m.input.Value() != "hello\n" {
		t.Fatalf("expected 'hello\\n', got %q", m.input.Value())
	}
}

// TestEnter_NoSubmitEmptyIdle verifies Enter with empty input returns nil.
func TestEnter_NoSubmitEmptyIdle(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("")

	cmd := m.handleKeyPress(keyEnter())
	if cmd != nil {
		t.Fatal("Enter with empty input should return nil")
	}
}

// --- Debounced Streaming Markdown Rendering Tests ---

func TestRenderPendingBlock_UsesCachedRendered(t *testing.T) {
	// When renderedMarkdown is provided, it should be used directly.
	cached := "cached rendered output"
	got := renderPendingBlock("raw text", blockAssistantText, 80, cached)
	if got != cached {
		t.Fatalf("expected cached output %q, got %q", cached, got)
	}
}

func TestRenderPendingBlock_FallsBackToPlainText(t *testing.T) {
	// When renderedMarkdown is empty, should fall back to plain text rendering.
	got := renderPendingBlock("raw text", blockAssistantText, 80, "")
	// Should contain the raw text (rendered through assistantBlockStyle)
	if !strings.Contains(got, "raw text") {
		t.Fatalf("expected raw text in output, got %q", got)
	}
}

func TestRenderPendingBlock_ThinkingIgnoresCachedRendered(t *testing.T) {
	// Thinking blocks should ignore cached rendered markdown.
	got := renderPendingBlock("thinking content", blockThinking, 80, "cached")
	if strings.Contains(got, "cached") {
		t.Fatalf("thinking block should not use cached rendered, got %q", got)
	}
	if !strings.Contains(got, "thinking") {
		t.Fatalf("expected thinking content in output, got %q", got)
	}
}

func TestModel_RenderPendingMarkdown_NoOpForThinking(t *testing.T) {
	m := newTestModel()
	m.pendingKind = blockThinking
	m.pendingBuilder.WriteString("thinking...")

	m.renderPendingMarkdown()

	// Should not render for thinking blocks.
	if m.pendingRendered != "" {
		t.Fatal("pendingRendered should be empty for thinking blocks")
	}
}

func TestModel_RenderPendingMarkdown_NoOpForEmptyBuilder(t *testing.T) {
	m := newTestModel()
	m.pendingKind = blockAssistantText
	// Builder is empty.

	m.renderPendingMarkdown()

	if m.pendingRendered != "" {
		t.Fatal("pendingRendered should be empty for empty builder")
	}
}

func TestModel_RenderPendingMarkdown_RendersAssistantText(t *testing.T) {
	m := newTestModel()
	m.pendingKind = blockAssistantText
	m.pendingBuilder.WriteString("**bold** and *italic*")

	m.renderPendingMarkdown()

	if m.pendingRendered == "" {
		t.Fatal("pendingRendered should be populated for assistant text")
	}
	if m.pendingRenderedLen != len("**bold** and *italic*") {
		t.Fatalf("expected pendingRenderedLen %d, got %d", len("**bold** and *italic*"), m.pendingRenderedLen)
	}
	// Rendered output should contain the text content (with ANSI codes)
	clean := stripANSI(m.pendingRendered)
	if !strings.Contains(clean, "bold") {
		t.Fatalf("expected 'bold' in rendered output, got %q", clean)
	}
}

func TestModel_RenderPendingMarkdown_SkipsIfUnchanged(t *testing.T) {
	m := newTestModel()
	m.pendingKind = blockAssistantText
	m.pendingBuilder.WriteString("test content")

	// First render.
	m.renderPendingMarkdown()
	firstRender := m.pendingRendered
	firstLen := m.pendingRenderedLen

	// Second render with same content — should skip.
	m.renderPendingMarkdown()

	if m.pendingRendered != firstRender {
		t.Fatal("pendingRendered should not change if content unchanged")
	}
	if m.pendingRenderedLen != firstLen {
		t.Fatal("pendingRenderedLen should not change if content unchanged")
	}
}

func TestModel_RenderPendingMarkdown_UpdatesOnNewContent(t *testing.T) {
	m := newTestModel()
	m.pendingKind = blockAssistantText
	m.pendingBuilder.WriteString("initial")

	m.renderPendingMarkdown()
	firstRender := m.pendingRendered

	// Append more content.
	m.pendingBuilder.WriteString(" more text")
	m.renderPendingMarkdown()

	if m.pendingRendered == firstRender {
		t.Fatal("pendingRendered should change when content changes")
	}
	if m.pendingRenderedLen != len("initial more text") {
		t.Fatalf("expected pendingRenderedLen %d, got %d", len("initial more text"), m.pendingRenderedLen)
	}
}

func TestModel_ResetForTurn_ClearsRenderCache(t *testing.T) {
	m := newTestModel()
	m.pendingKind = blockAssistantText
	m.pendingBuilder.WriteString("**bold**")
	m.renderPendingMarkdown()

	if m.pendingRendered == "" {
		t.Fatal("pendingRendered should be populated before reset")
	}

	m.resetForTurn()

	if m.pendingRendered != "" {
		t.Fatal("pendingRendered should be cleared after resetForTurn")
	}
	if m.pendingRenderedLen != 0 {
		t.Fatal("pendingRenderedLen should be 0 after resetForTurn")
	}
	if !m.lastRenderTime.IsZero() {
		t.Fatal("lastRenderTime should be zero after resetForTurn")
	}
}

func TestModel_UpdateViewport_UsesCachedRendered(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.pendingKind = blockAssistantText
	m.pendingBuilder.WriteString("**bold text**")

	// Simulate cached rendered output.
	m.pendingRendered = "CACHED_RENDERED_OUTPUT"
	m.pendingRenderedLen = len("**bold text**")

	// Test renderPendingBlock directly with cached output.
	got := renderPendingBlock(m.pendingBuilder.String(), m.pendingKind, m.width, m.pendingRendered)
	if got != "CACHED_RENDERED_OUTPUT" {
		t.Fatalf("expected cached rendered output, got %q", got)
	}
}

func TestModel_UpdateViewport_FallsBackToPlainText(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.pendingKind = blockAssistantText
	m.pendingBuilder.WriteString("plain text content")
	// No cached rendered output.

	// Test renderPendingBlock directly without cached output.
	got := renderPendingBlock(m.pendingBuilder.String(), m.pendingKind, m.width, "")
	if !strings.Contains(got, "plain text content") {
		t.Fatalf("expected plain text in output, got %q", got)
	}
}

func TestNewRenderer_CreatesReusableRenderer(t *testing.T) {
	r, err := NewRenderer(80)
	if err != nil {
		t.Fatalf("failed to create renderer: %v", err)
	}
	if r == nil {
		t.Fatal("renderer should not be nil")
	}

	// Render same content multiple times.
	text := "**bold** and *italic*"
	out1 := RenderWithRenderer(r, text, text)
	out2 := RenderWithRenderer(r, text, text)

	if out1 == "" || out2 == "" {
		t.Fatal("renderer should produce non-empty output")
	}
	if out1 != out2 {
		t.Fatal("renderer should produce consistent output")
	}
}

func TestRenderWithRenderer_EmptyInput(t *testing.T) {
	r, _ := NewRenderer(80)
	got := RenderWithRenderer(r, "", "")
	if got != "" {
		t.Fatalf("expected empty string for empty input, got %q", got)
	}
}

func TestResize_InvalidatesPendingRenderCache(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.pendingKind = blockAssistantText
	m.pendingBuilder.WriteString("**bold**")
	m.pendingRendered = "cached"
	m.pendingRenderedLen = 10

	// Simulate resize.
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	if m.pendingRendered != "" {
		t.Fatal("pendingRendered should be cleared on resize")
	}
	if m.pendingRenderedLen != 0 {
		t.Fatal("pendingRenderedLen should be 0 on resize")
	}
}

// --- Slash Command Tests ---

func TestSlashCommand_Help(t *testing.T) {
	m := newTestModel()
	handled, _ := m.executeCommand("/help")
	if !handled {
		t.Fatal("/help should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockAssistantText {
		t.Fatalf("expected assistantText block, got %v", m.blocks[0].kind)
	}
	if !strings.Contains(m.blocks[0].text, "/model") {
		t.Fatal("help text should mention /model command")
	}
	if !strings.Contains(m.blocks[0].text, "/skill:") {
		t.Fatal("help text should mention /skill: command")
	}
}

func TestSlashCommand_Clear(t *testing.T) {
	m := newTestModel()
	m.blocks = append(m.blocks, messageBlock{kind: blockAssistantText, text: "test"})
	handled, _ := m.executeCommand("/clear")
	if !handled {
		t.Fatal("/clear should be handled")
	}
	if len(m.blocks) != 0 {
		t.Fatalf("expected 0 blocks after /clear, got %d", len(m.blocks))
	}
}

func TestSlashCommand_Name_Empty(t *testing.T) {
	m := newTestModel()
	handled, _ := m.executeCommand("/name")
	if !handled {
		t.Fatal("/name without args should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockError {
		t.Fatalf("expected error block, got %v", m.blocks[0].kind)
	}
	if !strings.Contains(m.blocks[0].text, "Usage") {
		t.Fatal("error should mention usage")
	}
}

func TestSlashCommand_Name_Ephemeral(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	handled, _ := m.executeCommand("/name test-session")
	if !handled {
		t.Fatal("/name should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if !strings.Contains(m.blocks[0].text, "ephemeral") {
		t.Fatal("should mention ephemeral mode")
	}
}

func TestSlashCommand_Session(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.turnCount = 3

	handled, _ := m.executeCommand("/session")
	if !handled {
		t.Fatal("/session should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	text := m.blocks[0].text
	if !strings.Contains(text, "Model:") {
		t.Fatal("session info should contain model")
	}
	if !strings.Contains(text, "Turns: 3") {
		t.Fatal("session info should contain turn count")
	}
}

func TestSlashCommand_Compact_Ephemeral(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	handled, _ := m.executeCommand("/compact")
	if !handled {
		t.Fatal("/compact should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if !strings.Contains(m.blocks[0].text, "ephemeral") {
		t.Fatal("should mention ephemeral mode")
	}
}

func TestSlashCommand_Model(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.width = 80
	m.height = 40

	models := s.ListModels()
	if len(models) == 0 {
		t.Skip("no models available")
	}

	handled, _ := m.executeCommand("/model")
	if !handled {
		t.Fatal("/model should be handled")
	}
	if !m.paletteActive {
		t.Fatal("should activate palette")
	}
	if len(m.palette.list.SelectedItem().(modelPaletteItem).Title()) == 0 {
		t.Fatal("palette should have model items")
	}
}

func TestNewModel_NoActiveModelShowsGuidance(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/config.json", []byte(`{"providers":{"ollama":{"enabled":false}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := sdk.CreateSession(context.Background(), sdk.SessionOptions{WorkingDir: tmpDir, Ephemeral: true})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	m := tuiModelWithSession(s)
	if len(m.blocks) == 0 {
		t.Fatal("expected startup guidance block")
	}
	text := m.blocks[0].text
	if !strings.Contains(text, "/connect") || !strings.Contains(text, "/model") {
		t.Fatalf("guidance should mention /connect and /model, got %q", text)
	}
}

func TestSlashCommand_Model_EscCancels(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.width = 80
	m.height = 40

	models := s.ListModels()
	if len(models) == 0 {
		t.Skip("no models available")
	}

	handled, _ := m.executeCommand("/model")
	if !handled {
		t.Fatal("/model should be handled")
	}
	if !m.paletteActive {
		t.Fatal("should activate palette")
	}

	m.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.paletteActive {
		t.Fatal("palette should be deactivated after Esc")
	}
}

func TestSlashCommand_Model_Selection(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.width = 80
	m.height = 40

	models := s.ListModels()
	if len(models) == 0 {
		t.Skip("no models available")
	}

	handled, _ := m.executeCommand("/model")
	if !handled {
		t.Fatal("/model should be handled")
	}
	if !m.paletteActive {
		t.Fatal("should activate palette")
	}

	// Simulate selecting the first item via Enter
	m.handleKeyPress(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.paletteActive {
		t.Fatal("palette should be deactivated after selection")
	}

	// Verify model was switched
	firstModel := models[0].ID
	found := false
	for _, b := range m.blocks {
		if b.kind == blockAssistantText && strings.Contains(b.text, firstModel) {
			found = true
		}
	}
	if !found {
		t.Fatalf("should show confirmation message with model name %q", firstModel)
	}
}

func TestSlashCommand_Model_NoModelsShowsActionableGuidance(t *testing.T) {
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/config.json", []byte(`{"providers":{"ollama":{"enabled":false}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := sdk.CreateSession(context.Background(), sdk.SessionOptions{WorkingDir: tmpDir, Ephemeral: true})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer s.Close()

	m := tuiModelWithSession(s)
	m.blocks = nil

	handled, _ := m.executeCommand("/model")
	if !handled {
		t.Fatal("/model should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if !strings.Contains(m.blocks[0].text, "/connect") || !strings.Contains(m.blocks[0].text, "/model") {
		t.Fatalf("guidance should mention /connect and /model, got %q", m.blocks[0].text)
	}
}

func TestSlashCommand_Skills_Empty(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	handled, _ := m.executeCommand("/skills")
	if !handled {
		t.Fatal("/skills should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	// Skills may or may not be discovered depending on test environment
	// Just verify it was handled
}

func TestSlashCommand_Skill_NotFound(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	handled, _ := m.executeCommand("/skill:nonexistent-skill-name")
	if !handled {
		t.Fatal("/skill:nonexistent should be handled")
	}
	// Should show error since skill doesn't exist
	found := false
	for _, b := range m.blocks {
		if b.kind == blockError && strings.Contains(b.text, "not found") {
			found = true
		}
	}
	if !found {
		t.Fatal("should show skill not found error")
	}
}

func TestSlashCommand_Skill_EmptyName(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	handled, _ := m.executeCommand("/skill:")
	if !handled {
		t.Fatal("/skill: should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(m.blocks))
	}
	if m.blocks[0].kind != blockError {
		t.Fatalf("expected error block, got %v", m.blocks[0].kind)
	}
}

// --- Helpers ---

// newEphemeralSession creates a real ephemeral SDK session for testing.
func newEphemeralSession(t *testing.T) *sdk.Session {
	t.Helper()
	tmpDir := testutil.TempDir(t)
	testutil.SetHomeEnv(t, tmpDir)

	// Create auth.json with an API key
	tauDir := tmpDir + "/.tau"
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tauDir+"/auth.json",
		[]byte(`{"openai": "sk-test-key"}`), 0600); err != nil {
		t.Fatal(err)
	}

	s, err := sdk.CreateSession(context.Background(), sdk.SessionOptions{
		Model:      "gpt-4o",
		WorkingDir: tmpDir,
		Ephemeral:  true,
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// tuiModelWithSession creates a TUI model with a real SDK session.
func tuiModelWithSession(s *sdk.Session) *Model {
	m := NewModel(s)
	m.width = 80
	m.height = 40
	return m
}

func TestQueue_EnqueueDuringStreaming(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.state = stateStreaming
	m.input.SetValue("queued message")

	cmd := m.handleKeyPress(keyEnter())

	if cmd == nil {
		t.Fatal("Enter during streaming should return a Cmd to prevent textarea processing")
	}
	if s.PendingCount() != 1 {
		t.Fatalf("expected 1 pending message, got %d", s.PendingCount())
	}
	if m.input.Value() != "" {
		t.Fatal("input should be cleared after enqueue")
	}
	hasQueuedBlock := false
	for _, b := range m.blocks {
		if b.kind == blockQueuedMessage && b.text == "queued message" {
			hasQueuedBlock = true
			break
		}
	}
	if !hasQueuedBlock {
		t.Fatal("expected queued message block in viewport")
	}
}

func TestQueue_HandlePromptDone_DrainsQueue(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.state = stateStreaming

	s.EnqueueMessage("first")
	s.EnqueueMessage("second")

	pendingBefore := s.PendingCount()
	next := s.DequeueMessage()

	if next != "first" {
		t.Fatalf("expected 'first', got %q", next)
	}
	if s.PendingCount() != pendingBefore-1 {
		t.Fatalf("expected %d pending after dequeue, got %d", pendingBefore-1, s.PendingCount())
	}
}

func TestQueue_HandlePromptDone_EmptyQueue(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.state = stateStreaming

	cmd := m.handlePromptDone(false)

	if cmd != nil {
		t.Fatal("handlePromptDone should return nil when queue is empty")
	}
	if m.state != stateIdle {
		t.Fatalf("expected stateIdle, got %v", m.state)
	}
}

func TestQueue_FooterShowsPendingCount(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)

	s.EnqueueMessage("msg1")
	s.EnqueueMessage("msg2")
	s.EnqueueMessage("msg3")

	m.state = stateIdle
	view := m.renderFooter()
	if !strings.Contains(view, "3 queued") {
		t.Fatalf("expected footer to show '3 queued', got: %s", view)
	}

	m.state = stateStreaming
	m.spinnerActive = true
	view = m.renderFooter()
	if !strings.Contains(view, "working") || !strings.Contains(view, "3 queued") {
		t.Fatalf("expected footer to show 'working (3 queued)', got: %s", view)
	}
}

func TestQueue_OverflowWarning(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.state = stateStreaming

	for i := 0; i < 12; i++ {
		s.EnqueueMessage(fmt.Sprintf("msg%d", i))
	}

	if s.OverflowCount() != 2 {
		t.Fatalf("expected 2 overflow drops, got %d", s.OverflowCount())
	}

	for s.PendingCount() > 0 {
		s.DequeueMessage()
	}

	m.handlePromptDone(false)

	hasWarning := false
	for _, b := range m.blocks {
		if b.kind == blockError && strings.Contains(b.text, "Queue overflow") {
			hasWarning = true
			break
		}
	}
	if !hasWarning {
		t.Fatal("expected overflow warning block in viewport")
	}
	if s.OverflowCount() != 0 {
		t.Fatal("overflow counter should be reset after warning")
	}
}

func TestQueue_SlashCommand_HelpDuringStreaming(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.state = stateStreaming
	m.input.SetValue("/help")

	handled, _ := m.executeCommandStreaming("/help")

	if !handled {
		t.Fatal("/help should be handled during streaming")
	}
	if s.PendingCount() != 0 {
		t.Fatalf("/help should not be queued, got %d pending", s.PendingCount())
	}
}

func TestQueue_SlashCommand_ClearDuringStreaming(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.state = stateStreaming
	m.blocks = append(m.blocks, messageBlock{kind: blockAssistantText, text: "existing"})

	handled, _ := m.executeCommandStreaming("/clear")

	if !handled {
		t.Fatal("/clear should be handled during streaming")
	}
	if len(m.blocks) != 0 {
		t.Fatal("/clear should clear all blocks")
	}
}

func TestQueue_SlashCommand_QueuesDuringStreaming(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.state = stateStreaming
	m.input.SetValue("/compact")

	cmd := m.handleKeyPress(keyEnter())

	if cmd == nil {
		t.Fatal("Enter during streaming should return a Cmd to prevent textarea processing")
	}
	if s.PendingCount() != 1 {
		t.Fatalf("expected /compact to be queued, got %d pending", s.PendingCount())
	}
}

func TestViewportUpdate_Throttling(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.blocks = append(m.blocks, messageBlock{kind: blockUserMessage, text: "hello"})

	m.updateViewport()
	firstUpdate := m.lastViewportUpdate

	if firstUpdate.IsZero() {
		t.Fatal("lastViewportUpdate should be set after updateViewport")
	}

	m.updateViewport()

	if m.lastViewportUpdate != firstUpdate {
		t.Fatal("second updateViewport within throttle interval should not update timestamp")
	}
}

func TestViewportUpdate_ForceBypassesThrottle(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.blocks = append(m.blocks, messageBlock{kind: blockUserMessage, text: "hello"})

	m.updateViewport()
	firstUpdate := m.lastViewportUpdate

	m.updateViewportWithForce(true)

	if m.lastViewportUpdate == firstUpdate {
		t.Fatal("force update should bypass throttle and update timestamp")
	}
}

func TestRenderedCache_Invalidation(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.blocks = append(m.blocks, messageBlock{kind: blockUserMessage, text: "hello"})
	m.invalidateRenderedCache()

	if m.renderedCacheValid {
		t.Fatal("cache should be invalid after invalidateRenderedCache")
	}

	m.updateViewportWithForce(true)

	if !m.renderedCacheValid {
		t.Fatal("cache should be valid after forced updateViewport")
	}
	if m.renderedCache == "" {
		t.Fatal("cache should contain rendered content")
	}
}

func TestRenderedCache_UsedDuringStreaming(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.blocks = append(m.blocks, messageBlock{kind: blockUserMessage, text: "hello"})
	m.updateViewportWithForce(true)

	cachedContent := m.renderedCache

	m.pendingBuilder.WriteString("streaming response")
	m.pendingKind = blockAssistantText

	m.updateViewport()

	if m.renderedCache != cachedContent {
		t.Fatal("cache should not be modified during streaming update")
	}
	if !m.renderedCacheValid {
		t.Fatal("cache should remain valid during streaming")
	}
}

func TestRenderedCache_InvalidatedOnFlush(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.blocks = append(m.blocks, messageBlock{kind: blockUserMessage, text: "hello"})
	m.updateViewportWithForce(true)

	if !m.renderedCacheValid {
		t.Fatal("cache should be valid")
	}

	m.pendingBuilder.WriteString("response")
	m.pendingKind = blockAssistantText
	m.flushPending()

	if m.renderedCacheValid {
		t.Fatal("cache should be invalidated after flushPending")
	}
}

func TestRenderedCache_InvalidatedOnToolExecEnd(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.blocks = append(m.blocks, messageBlock{kind: blockUserMessage, text: "hello"})
	m.blocks = append(m.blocks, messageBlock{kind: blockToolCall, toolName: "bash", toolSt: toolPending})
	m.updateViewportWithForce(true)
	m.pendingToolIndex = 1

	if !m.renderedCacheValid {
		t.Fatal("cache should be valid")
	}

	m.processEvent(types.AgentEvent{
		Type: types.AgentEventToolExecEnd,
		Data: map[string]any{"tool": "bash"},
	})

	if m.renderedCacheValid {
		t.Fatal("cache should be invalidated after tool exec end")
	}
}

func TestRenderedCache_InvalidatedOnResize(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	m.blocks = append(m.blocks, messageBlock{kind: blockUserMessage, text: "hello"})
	m.updateViewportWithForce(true)

	if !m.renderedCacheValid {
		t.Fatal("cache should be valid")
	}

	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	updated := model.(*Model)

	if updated.renderedCacheValid {
		t.Fatal("cache should be invalidated on resize")
	}
}

func TestRenderedCache_InvalidatedOnQueuedMessage(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.state = stateStreaming
	m.blocks = append(m.blocks, messageBlock{kind: blockUserMessage, text: "hello"})
	m.updateViewportWithForce(true)

	if !m.renderedCacheValid {
		t.Fatal("cache should be valid")
	}

	m.input.SetValue("queued message")
	m.handleKeyPress(keyEnter())

	if m.renderedCacheValid {
		t.Fatal("cache should be invalidated after queuing message")
	}
}

func TestSlashCommand_New_Ephemeral(t *testing.T) {
	s := newEphemeralSession(t)
	m := tuiModelWithSession(s)
	m.blocks = append(m.blocks, messageBlock{kind: blockAssistantText, text: "old content"})
	m.turnCount = 5

	handled, _ := m.executeCommand("/new")
	if !handled {
		t.Fatal("/new should be handled")
	}
	if len(m.blocks) != 1 {
		t.Fatalf("expected 1 block after /new, got %d", len(m.blocks))
	}
	if !strings.Contains(m.blocks[0].text, "New session started") {
		t.Fatalf("expected new session message, got %q", m.blocks[0].text)
	}
	if m.turnCount != 0 {
		t.Errorf("expected turnCount reset to 0, got %d", m.turnCount)
	}
	if m.sessionID != "" {
		t.Errorf("expected empty sessionID in ephemeral mode, got %q", m.sessionID)
	}
}

func TestSlashCommand_New_UnavailableDuringStreaming(t *testing.T) {
	m := newTestModel()
	m.state = stateStreaming

	handled, _ := m.executeCommand("/new")
	if handled {
		t.Fatal("/new should not be handled during streaming")
	}
}

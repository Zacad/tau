package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// --- Focus-gated key routing tests ---

func keyPgUp() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyPgUp}
}

func keyPgDown() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyPgDown}
}

// TestPromptFocused_PageUpDoesNotScrollViewport verifies that when the prompt
// input is focused, PageUp does not scroll the chat viewport.
func TestPromptFocused_PageUpDoesNotScrollViewport(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	// Add enough content to make the viewport scrollable.
	for i := 0; i < 50; i++ {
		m.blocks = append(m.blocks, messageBlock{kind: blockAssistantText, text: fmt.Sprintf("Line %d", i)})
	}
	m.updateViewportWithForce(true)

	// Scroll to bottom.
	m.viewport.GotoBottom()
	offsetBefore := m.viewport.YOffset()

	if offsetBefore == 0 {
		t.Fatal("viewport should be scrolled down with enough content")
	}

	// Ensure input is focused (default).
	if !m.input.Focused() {
		t.Fatal("input should be focused")
	}

	// Send PageUp through the full Update path.
	m.Update(keyPgUp())

	offsetAfter := m.viewport.YOffset()
	if offsetAfter != offsetBefore {
		t.Fatalf("viewport offset should not change when prompt is focused: before=%d, after=%d", offsetBefore, offsetAfter)
	}
}

// TestPromptFocused_PageDownDoesNotScrollViewport verifies that when the prompt
// input is focused, PageDown does not scroll the chat viewport.
func TestPromptFocused_PageDownDoesNotScrollViewport(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	for i := 0; i < 50; i++ {
		m.blocks = append(m.blocks, messageBlock{kind: blockAssistantText, text: fmt.Sprintf("Line %d", i)})
	}
	m.updateViewportWithForce(true)

	// Start at top.
	m.viewport.GotoTop()
	offsetBefore := m.viewport.YOffset()

	if !m.input.Focused() {
		t.Fatal("input should be focused")
	}

	m.Update(keyPgDown())

	offsetAfter := m.viewport.YOffset()
	if offsetAfter != offsetBefore {
		t.Fatalf("viewport offset should not change when prompt is focused: before=%d, after=%d", offsetBefore, offsetAfter)
	}
}

// TestPromptFocused_UpWithEmptyHistoryDoesNotScroll verifies that pressing Up
// when prompt history is empty does not scroll the viewport.
func TestPromptFocused_UpWithEmptyHistoryDoesNotScroll(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	for i := 0; i < 50; i++ {
		m.blocks = append(m.blocks, messageBlock{kind: blockAssistantText, text: fmt.Sprintf("Line %d", i)})
	}
	m.updateViewportWithForce(true)
	m.viewport.GotoBottom()
	offsetBefore := m.viewport.YOffset()

	m.promptHistory = nil
	m.promptHistoryIndex = -1

	m.Update(keyUp())

	offsetAfter := m.viewport.YOffset()
	if offsetAfter != offsetBefore {
		t.Fatalf("viewport offset should not change: before=%d, after=%d", offsetBefore, offsetAfter)
	}
}

// TestPromptFocused_DownWhenNotBrowsingDoesNotScroll verifies that pressing Down
// when not browsing history does not scroll the viewport.
func TestPromptFocused_DownWhenNotBrowsingDoesNotScroll(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	for i := 0; i < 50; i++ {
		m.blocks = append(m.blocks, messageBlock{kind: blockAssistantText, text: fmt.Sprintf("Line %d", i)})
	}
	m.updateViewportWithForce(true)
	m.viewport.GotoBottom()
	offsetBefore := m.viewport.YOffset()

	m.promptHistory = []string{"old prompt"}
	m.promptHistoryIndex = -1 // not browsing

	m.Update(keyDown())

	offsetAfter := m.viewport.YOffset()
	if offsetAfter != offsetBefore {
		t.Fatalf("viewport offset should not change: before=%d, after=%d", offsetBefore, offsetAfter)
	}
}

// TestMouseWheelScrollsViewportWhilePromptFocused verifies that mouse wheel
// events still scroll the viewport even when the prompt input is focused.
func TestMouseWheelScrollsViewportWhilePromptFocused(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 40

	for i := 0; i < 50; i++ {
		m.blocks = append(m.blocks, messageBlock{kind: blockAssistantText, text: fmt.Sprintf("Line %d", i)})
	}
	m.updateViewportWithForce(true)
	m.viewport.GotoTop()
	offsetBefore := m.viewport.YOffset()

	if !m.input.Focused() {
		t.Fatal("input should be focused")
	}

	// Mouse wheel down should scroll viewport.
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})

	offsetAfter := m.viewport.YOffset()
	if offsetAfter <= offsetBefore {
		t.Fatalf("viewport should scroll on mouse wheel: before=%d, after=%d", offsetBefore, offsetAfter)
	}
}

// TestShiftEnterInsertsSingleNewline verifies that Shift+Enter through the full
// Update path inserts exactly one newline (not double-processed).
func TestShiftEnterInsertsSingleNewline(t *testing.T) {
	m := newTestModel()
	m.input.SetValue("hello")

	m.Update(keyShiftEnter())

	if m.input.Value() != "hello\n" {
		t.Fatalf("expected 'hello\\n', got %q", m.input.Value())
	}
}

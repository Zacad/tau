package palette

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPaletteMessage_Init(t *testing.T) {
	var m PaletteMessage
	m.Init("Success", "Operation completed successfully")

	if m.done {
		t.Fatal("should not be done after init")
	}
	if m.cancelled {
		t.Fatal("should not be cancelled after init")
	}
	if m.title != "Success" {
		t.Fatalf("title = %q, want %q", m.title, "Success")
	}
	if m.message != "Operation completed successfully" {
		t.Fatalf("message = %q, want %q", m.message, "Operation completed successfully")
	}
}

func TestPaletteMessage_InitEmptyTitle(t *testing.T) {
	var m PaletteMessage
	m.Init("", "Just a message")

	if m.title != "" {
		t.Fatalf("title = %q, want empty", m.title)
	}
	if m.message != "Just a message" {
		t.Fatalf("message = %q, want %q", m.message, "Just a message")
	}
}

func TestPaletteMessage_Render(t *testing.T) {
	var m PaletteMessage
	m.Init("Info", "This is an informational message")
	m.SetSize(80, 40)

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
	stripped := stripANSI(view)
	if !strings.Contains(stripped, "Info") {
		t.Fatal("view should contain title")
	}
	if !strings.Contains(stripped, "This is an informational message") {
		t.Fatal("view should contain message")
	}
	if !strings.Contains(stripped, "Enter to continue") {
		t.Fatal("view should contain hint")
	}
}

func TestPaletteMessage_RenderNoTitle(t *testing.T) {
	var m PaletteMessage
	m.Init("", "Message without title")
	m.SetSize(80, 40)

	view := m.View()
	stripped := stripANSI(view)
	if !strings.Contains(stripped, "Message without title") {
		t.Fatal("view should contain message")
	}
	if !strings.Contains(stripped, "Enter to continue") {
		t.Fatal("view should contain hint")
	}
}

func TestPaletteMessage_EnterDone(t *testing.T) {
	var m PaletteMessage
	m.Init("Test", "Message")

	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !m.done {
		t.Fatal("should be done after enter")
	}
	if m.cancelled {
		t.Fatal("should not be cancelled after enter")
	}
}

func TestPaletteMessage_EscCancel(t *testing.T) {
	var m PaletteMessage
	m.Init("Test", "Message")

	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !m.cancelled {
		t.Fatal("should be cancelled after esc")
	}
	if m.done {
		t.Fatal("should not be done after cancel")
	}
}

func TestPaletteMessage_Result(t *testing.T) {
	var m PaletteMessage
	m.Init("Test", "Result message")

	result := m.Result()
	if result != "Result message" {
		t.Fatalf("Result() = %q, want %q", result, "Result message")
	}
}

func TestPaletteMessage_SetSize(t *testing.T) {
	var m PaletteMessage
	m.Init("Test", "Message")

	m.SetSize(100, 50)
	if m.width != 100 {
		t.Fatalf("width = %d, want 100", m.width)
	}
	if m.height != 50 {
		t.Fatalf("height = %d, want 50", m.height)
	}
}

func TestPaletteMessage_OtherKeysIgnored(t *testing.T) {
	var m PaletteMessage
	m.Init("Test", "Message")

	m.Update(tea.KeyPressMsg{Code: 'a'})

	if m.done {
		t.Fatal("should not be done after random key")
	}
	if m.cancelled {
		t.Fatal("should not be cancelled after random key")
	}
}

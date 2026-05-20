package palette

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPaletteInput_Init(t *testing.T) {
	var p PaletteInput
	p.Init("Enter name", "Type a name...")

	if p.done {
		t.Fatal("should not be done after init")
	}
	if p.cancelled {
		t.Fatal("should not be cancelled after init")
	}
	if p.result != "" {
		t.Fatalf("expected empty result, got %q", p.result)
	}
	if !p.input.Focused() {
		t.Fatal("input should be focused after init")
	}
	if p.input.Placeholder != "Type a name..." {
		t.Fatalf("placeholder = %q, want %q", p.input.Placeholder, "Type a name...")
	}
}

func TestPaletteInput_Render(t *testing.T) {
	var p PaletteInput
	p.Init("Enter name", "Type a name...")
	p.SetSize(80, 40)

	view := p.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
	if !strings.Contains(stripANSI(view), "Enter name") {
		t.Fatal("view should contain label")
	}
}

func TestPaletteInput_TypeAndEnter(t *testing.T) {
	var p PaletteInput
	p.Init("Enter text", "Type here...")

	typeInput(&p, "hello world")
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !p.done {
		t.Fatal("should be done after enter")
	}
	if p.result != "hello world" {
		t.Fatalf("result = %q, want %q", p.result, "hello world")
	}
}

func TestPaletteInput_EscCancel(t *testing.T) {
	var p PaletteInput
	p.Init("Enter text", "Type here...")

	typeInput(&p, "some text")
	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !p.cancelled {
		t.Fatal("should be cancelled after esc")
	}
	if p.done {
		t.Fatal("should not be done after cancel")
	}
}

func TestPaletteInput_Result(t *testing.T) {
	var p PaletteInput
	p.Init("Enter text", "Type here...")

	typeInput(&p, "test value")
	p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if p.Result() != "test value" {
		t.Fatalf("Result() = %q, want %q", p.Result(), "test value")
	}
}

func TestPaletteInput_SetValue(t *testing.T) {
	var p PaletteInput
	p.Init("Enter text", "Type here...")

	p.SetValue("pre-filled")
	if p.Value() != "pre-filled" {
		t.Fatalf("Value() = %q, want %q", p.Value(), "pre-filled")
	}
}

func TestPaletteInput_FocusBlur(t *testing.T) {
	var p PaletteInput
	p.Init("Enter text", "Type here...")

	if !p.input.Focused() {
		t.Fatal("should be focused after init")
	}

	p.Blur()
	if p.input.Focused() {
		t.Fatal("should not be focused after blur")
	}

	p.Focus()
	if !p.input.Focused() {
		t.Fatal("should be focused after focus")
	}
}

func typeInput(p *PaletteInput, s string) {
	for _, ch := range s {
		p.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
	}
}

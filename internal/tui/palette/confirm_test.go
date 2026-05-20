package palette

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPaletteConfirm_Init(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	if c.done {
		t.Fatal("should not be done after init")
	}
	if c.cancelled {
		t.Fatal("should not be cancelled after init")
	}
	if c.result {
		t.Fatal("result should be false after init")
	}
	if c.prompt != "Save this?" {
		t.Fatalf("prompt = %q, want %q", c.prompt, "Save this?")
	}
}

func TestPaletteConfirm_Render(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")
	c.SetSize(80, 40)

	view := c.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
	if !strings.Contains(stripANSI(view), "Save this?") {
		t.Fatal("view should contain prompt")
	}
	if !strings.Contains(stripANSI(view), "[Y]es") {
		t.Fatal("view should contain [Y]es")
	}
	if !strings.Contains(stripANSI(view), "[N]o") {
		t.Fatal("view should contain [N]o")
	}
}

func TestPaletteConfirm_YesKey(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	c.Update(tea.KeyPressMsg{Code: 'y'})

	if !c.done {
		t.Fatal("should be done after y")
	}
	if !c.result {
		t.Fatal("result should be true after y")
	}
}

func TestPaletteConfirm_UpperYesKey(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	c.Update(tea.KeyPressMsg{Code: 'Y'})

	if !c.done {
		t.Fatal("should be done after Y")
	}
	if !c.result {
		t.Fatal("result should be true after Y")
	}
}

func TestPaletteConfirm_NoKey(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	c.Update(tea.KeyPressMsg{Code: 'n'})

	if !c.done {
		t.Fatal("should be done after n")
	}
	if c.result {
		t.Fatal("result should be false after n")
	}
}

func TestPaletteConfirm_UpperNoKey(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	c.Update(tea.KeyPressMsg{Code: 'N'})

	if !c.done {
		t.Fatal("should be done after N")
	}
	if c.result {
		t.Fatal("result should be false after N")
	}
}

func TestPaletteConfirm_EnterDefault(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	// Default is yes (confirmed=false means yes is selected)
	c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !c.done {
		t.Fatal("should be done after enter")
	}
	if !c.result {
		t.Fatal("result should be true (default yes) after enter")
	}
}

func TestPaletteConfirm_EnterAfterToggle(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	c.Toggle()
	c.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !c.done {
		t.Fatal("should be done after enter")
	}
	if c.result {
		t.Fatal("result should be false (selected no) after enter")
	}
}

func TestPaletteConfirm_EscCancel(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	c.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !c.cancelled {
		t.Fatal("should be cancelled after esc")
	}
	if c.done {
		t.Fatal("should not be done after cancel")
	}
}

func TestPaletteConfirm_Toggle(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	if !c.IsYes() {
		t.Fatal("default should be yes selected")
	}

	c.Toggle()
	if c.IsYes() {
		t.Fatal("should be no selected after toggle")
	}

	c.Toggle()
	if !c.IsYes() {
		t.Fatal("should be yes selected after second toggle")
	}
}

func TestPaletteConfirm_Result(t *testing.T) {
	var c PaletteConfirm
	c.Init("Save this?")

	c.Update(tea.KeyPressMsg{Code: 'y'})
	if c.Result() != true {
		t.Fatalf("Result() = %v, want true", c.Result())
	}

	c2 := PaletteConfirm{}
	c2.Init("Save this?")
	c2.Update(tea.KeyPressMsg{Code: 'n'})
	if c2.Result() != false {
		t.Fatalf("Result() = %v, want false", c2.Result())
	}
}

package palette

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
)

func TestPaletteTask_Init(t *testing.T) {
	var p PaletteTask
	cmd := p.Init("Testing", func() (bool, string, error) {
		return true, "done", nil
	})

	if cmd == nil {
		t.Fatal("expected init cmd")
	}
	if p.done {
		t.Fatal("should not be done after init")
	}
	if p.cancelled {
		t.Fatal("should not be cancelled after init")
	}
	if p.title != "Testing" {
		t.Fatalf("title = %q, want %q", p.title, "Testing")
	}
}

func TestPaletteTask_RenderSpinner(t *testing.T) {
	var p PaletteTask
	p.Init("Testing", func() (bool, string, error) {
		return true, "done", nil
	})
	p.SetSize(80, 40)

	view := p.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
	if !strings.Contains(stripANSI(view), "Testing") {
		t.Fatalf("view should contain title, got: %s", stripANSI(view))
	}
}

func TestPaletteTask_SuccessResult(t *testing.T) {
	var p PaletteTask
	p.Init("Testing", func() (bool, string, error) {
		return true, "operation completed", nil
	})
	p.SetSize(80, 40)

	p.Update(TaskResultMsg{Success: true, Message: "operation completed", Err: nil})

	if !p.done {
		t.Fatal("should be done after result")
	}
	if p.cancelled {
		t.Fatal("should not be cancelled")
	}
	success, message, err := p.Result()
	if !success {
		t.Fatal("result success should be true")
	}
	if message != "operation completed" {
		t.Fatalf("message = %q, want %q", message, "operation completed")
	}
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPaletteTask_ErrorResult(t *testing.T) {
	var p PaletteTask
	expectedErr := errors.New("connection failed")
	p.Init("Testing", func() (bool, string, error) {
		return false, "", expectedErr
	})
	p.SetSize(80, 40)

	p.Update(TaskResultMsg{Success: false, Message: "", Err: expectedErr})

	if !p.done {
		t.Fatal("should be done after result")
	}
	success, message, err := p.Result()
	if success {
		t.Fatal("result success should be false")
	}
	if err != expectedErr {
		t.Fatalf("err = %v, want %v", err, expectedErr)
	}
	_ = message
}

func TestPaletteTask_FailureResult(t *testing.T) {
	var p PaletteTask
	p.Init("Testing", func() (bool, string, error) {
		return false, "validation failed", nil
	})
	p.SetSize(80, 40)

	p.Update(TaskResultMsg{Success: false, Message: "validation failed", Err: nil})

	if !p.done {
		t.Fatal("should be done after result")
	}
	success, message, err := p.Result()
	if success {
		t.Fatal("result success should be false")
	}
	if message != "validation failed" {
		t.Fatalf("message = %q, want %q", message, "validation failed")
	}
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPaletteTask_EscCancel(t *testing.T) {
	var p PaletteTask
	p.Init("Testing", func() (bool, string, error) {
		return true, "done", nil
	})

	p.Update(tea.KeyPressMsg{Code: tea.KeyEsc})

	if !p.cancelled {
		t.Fatal("should be cancelled after esc")
	}
	if p.done {
		t.Fatal("should not be done after cancel")
	}
}

func TestPaletteTask_SpinnerTick(t *testing.T) {
	var p PaletteTask
	p.Init("Testing", func() (bool, string, error) {
		return true, "done", nil
	})
	p.SetSize(80, 40)

	tickMsg := spinner.TickMsg{}
	cmd := p.Update(tickMsg)

	if cmd == nil {
		t.Fatal("spinner tick should return a cmd")
	}
	if p.done {
		t.Fatal("should not be done after spinner tick")
	}
}

func TestPaletteTask_ViewAfterSuccess(t *testing.T) {
	var p PaletteTask
	p.Init("Testing", func() (bool, string, error) {
		return true, "operation completed", nil
	})
	p.SetSize(80, 40)

	p.Update(TaskResultMsg{Success: true, Message: "operation completed", Err: nil})

	view := p.View()
	stripped := stripANSI(view)
	if !strings.Contains(stripped, "operation completed") {
		t.Fatalf("view should contain success message, got: %s", stripped)
	}
}

func TestPaletteTask_ViewAfterError(t *testing.T) {
	var p PaletteTask
	p.Init("Testing", func() (bool, string, error) {
		return false, "", errors.New("timeout")
	})
	p.SetSize(80, 40)

	p.Update(TaskResultMsg{Success: false, Message: "", Err: errors.New("timeout")})

	view := p.View()
	stripped := stripANSI(view)
	if !strings.Contains(stripped, "timeout") {
		t.Fatalf("view should contain error message, got: %s", stripped)
	}
}

func TestPaletteTask_Result(t *testing.T) {
	var p PaletteTask
	p.Init("Testing", func() (bool, string, error) {
		return true, "success message", nil
	})
	p.SetSize(80, 40)

	p.Update(TaskResultMsg{Success: true, Message: "success message", Err: nil})

	success, message, err := p.Result()
	if !success {
		t.Fatal("success should be true")
	}
	if message != "success message" {
		t.Fatalf("message = %q, want %q", message, "success message")
	}
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPaletteTask_SetSize(t *testing.T) {
	var p PaletteTask
	p.Init("Testing", func() (bool, string, error) {
		return true, "done", nil
	})

	p.SetSize(100, 50)
	if p.width != 100 {
		t.Fatalf("width = %d, want 100", p.width)
	}
	if p.height != 50 {
		t.Fatalf("height = %d, want 50", p.height)
	}
}

func TestPaletteTask_RenderTaskResult(t *testing.T) {
	view := renderTaskResult("Test", true, "completed", nil)
	stripped := stripANSI(view)
	if !strings.Contains(stripped, "completed") {
		t.Fatalf("should contain success message, got: %s", stripped)
	}

	view = renderTaskResult("Test", false, "failed", nil)
	stripped = stripANSI(view)
	if !strings.Contains(stripped, "failed") {
		t.Fatalf("should contain failure message, got: %s", stripped)
	}

	view = renderTaskResult("Test", false, "", errors.New("timeout"))
	stripped = stripANSI(view)
	if !strings.Contains(stripped, "timeout") {
		t.Fatalf("should contain error message, got: %s", stripped)
	}
}

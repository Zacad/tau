package tui

import (
	"strings"
	"testing"
)

// TestBase64Encode verifies the base64 encoding used for OSC 52 clipboard.
func TestBase64Encode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"a", "YQ=="},
		{"ab", "YWI="},
		{"abc", "YWJj"},
		{"Hello, World!", "SGVsbG8sIFdvcmxkIQ=="},
		{"test", "dGVzdA=="},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := base64Encode(tt.input)
			if got != tt.expected {
				t.Errorf("base64Encode(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestExtractSelectedText_SingleLine verifies single-line text extraction.
func TestExtractSelectedText_SingleLine(t *testing.T) {
	m := newTestModel()

	// Set rendered cache directly (simulating what updateViewport does)
	m.renderedCache = "Hello World"
	m.renderedCacheValid = true
	m.width = 80

	// Simulate selection of "World"
	m.selectionActive = true
	m.selection.startLine = 0
	m.selection.startCol = 6
	m.selection.endLine = 0
	m.selection.endCol = 11

	got := m.extractSelectedText()
	if got != "World" {
		t.Errorf("extractSelectedText() = %q, want %q", got, "World")
	}
}

// TestExtractSelectedText_MultiLine verifies multi-line text extraction.
func TestExtractSelectedText_MultiLine(t *testing.T) {
	m := newTestModel()

	// Set rendered cache directly
	m.renderedCache = "Line 1\nLine 2\nLine 3"
	m.renderedCacheValid = true
	m.width = 80

	// Simulate selection from "1" (position 5) on line 0 to "Line" (position 4) on line 2
	m.selectionActive = true
	m.selection.startLine = 0
	m.selection.startCol = 5
	m.selection.endLine = 2
	m.selection.endCol = 4

	got := m.extractSelectedText()
	expected := "1\nLine 2\nLine"
	if got != expected {
		t.Errorf("extractSelectedText() = %q, want %q", got, expected)
	}
}

// TestExtractSelectedText_ReversedDirection verifies selection works when dragged upward.
func TestExtractSelectedText_ReversedDirection(t *testing.T) {
	m := newTestModel()

	m.renderedCache = "Hello World"
	m.renderedCacheValid = true
	m.width = 80

	// Selection dragged from right to left (end before start)
	m.selectionActive = true
	m.selection.startLine = 0
	m.selection.startCol = 11
	m.selection.endLine = 0
	m.selection.endCol = 6

	got := m.extractSelectedText()
	if got != "World" {
		t.Errorf("extractSelectedText() reversed = %q, want %q", got, "World")
	}
}

// TestExtractSelectedText_StripsANSI verifies ANSI codes are stripped from clipboard text.
func TestExtractSelectedText_StripsANSI(t *testing.T) {
	m := newTestModel()

	// Simulate rendered content with ANSI codes (like glamour output)
	m.renderedCache = "\x1b[38;5;242mthinking text\x1b[0m\n\x1b[1mbold text\x1b[0m"
	m.renderedCacheValid = true
	m.width = 80

	m.selectionActive = true
	m.selection.startLine = 0
	m.selection.startCol = 0
	m.selection.endLine = 1
	m.selection.endCol = 9

	got := m.extractSelectedText()
	// Should be clean text without ANSI codes
	if got != "thinking text\nbold text" {
		t.Errorf("extractSelectedText() = %q, want %q", got, "thinking text\nbold text")
	}
}

// TestApplySelectionHighlights verifies highlight application.
func TestApplySelectionHighlights(t *testing.T) {
	m := newTestModel()

	m.selectionActive = true
	m.selection.startLine = 2
	m.selection.startCol = 0
	m.selection.endLine = 5
	m.selection.endCol = 100 // Large column to cover entire last line

	content := "line0\nline1\nline2\nline3\nline4\nline5\nline6"
	result := m.applySelectionHighlightsToContent(content)

	// Verify lines 2-5 have highlight codes
	hlStart := "\x1b[97;48;5;63m"
	hlEnd := "\x1b[0m"

	if !strings.Contains(result, hlStart+"line2"+hlEnd) {
		t.Error("line2 should be highlighted")
	}
	if !strings.Contains(result, hlStart+"line3"+hlEnd) {
		t.Error("line3 should be highlighted")
	}
	if !strings.Contains(result, hlStart+"line5"+hlEnd) {
		t.Error("line5 should be highlighted")
	}

	// Verify lines outside selection are not highlighted
	if strings.Contains(result, hlStart+"line0"+hlEnd) {
		t.Error("line0 should NOT be highlighted")
	}
	if strings.Contains(result, hlStart+"line6"+hlEnd) {
		t.Error("line6 should NOT be highlighted")
	}
}

// TestApplySelectionHighlights_ColumnSelection verifies character-level selection.
func TestApplySelectionHighlights_ColumnSelection(t *testing.T) {
	m := newTestModel()

	m.selectionActive = true
	m.selection.startLine = 0
	m.selection.startCol = 6
	m.selection.endLine = 0
	m.selection.endCol = 11

	content := "Hello World"
	result := m.applySelectionHighlightsToContent(content)

	hlStart := "\x1b[97;48;5;63m"
	hlEnd := "\x1b[0m"

	expected := "Hello " + hlStart + "World" + hlEnd
	if result != expected {
		t.Errorf("column selection = %q, want %q", result, expected)
	}
}

// TestApplySelectionHighlights_MultiLineColumn verifies multi-line with column selection.
func TestApplySelectionHighlights_MultiLineColumn(t *testing.T) {
	m := newTestModel()

	m.selectionActive = true
	m.selection.startLine = 0
	m.selection.startCol = 4
	m.selection.endLine = 2
	m.selection.endCol = 4

	content := "Line 1\nLine 2\nLine 3"
	result := m.applySelectionHighlightsToContent(content)

	hlStart := "\x1b[97;48;5;63m"
	hlEnd := "\x1b[0m"

	// First line: "Line" (0-3) unhighlighted, " 1" (4-end) highlighted
	// Middle line: entire line highlighted
	// Last line: "Line" (0-3) highlighted, " 3" (4-end) unhighlighted
	expected := "Line" + hlStart + " 1" + hlEnd + "\n" +
		hlStart + "Line 2" + hlEnd + "\n" +
		hlStart + "Line" + hlEnd + " 3"

	if result != expected {
		t.Errorf("multi-line column selection = %q, want %q", result, expected)
	}
}

// TestApplySelectionHighlights_StripsANSI verifies that ANSI codes are stripped from selected lines.
func TestApplySelectionHighlights_StripsANSI(t *testing.T) {
	m := newTestModel()

	m.selectionActive = true
	m.selection.startLine = 1
	m.selection.startCol = 0
	m.selection.endLine = 1
	m.selection.endCol = 100 // Cover entire line

	// Line 1 has existing ANSI codes (simulating glamour output), line 2 also has them but is NOT selected
	content := "plain\n\x1b[38;5;242mthinking text\x1b[0m\n\x1b[1mbold text\x1b[0m"
	result := m.applySelectionHighlightsToContent(content)

	hlStart := "\x1b[97;48;5;63m"
	hlEnd := "\x1b[0m"

	// Verify the ANSI codes from glamour are stripped from the SELECTED line (index 1)
	// The thinking text line should have clean text with highlight
	if !strings.Contains(result, hlStart+"thinking text"+hlEnd) {
		t.Errorf("selected line should have highlight around clean text, got: %q", result)
	}

	// Verify non-selected line (index 2) keeps its ANSI codes
	if !strings.Contains(result, "\x1b[1mbold text\x1b[0m") {
		t.Error("non-selected line should keep its ANSI codes")
	}
}

// TestViewportTopBottom verifies viewport boundary calculations.
func TestViewportTopBottom(t *testing.T) {
	m := newTestModel()
	m.height = 30
	m.input.SetHeight(3)

	top := m.viewportTop()
	bottom := m.viewportBottom()

	if top != 2 {
		t.Errorf("viewportTop() = %d, want 2", top)
	}

	// bottom = height - footerH(2) - inputAreaH(3+3) = 30 - 2 - 6 = 22
	expectedBottom := 30 - 2 - 6
	if bottom != expectedBottom {
		t.Errorf("viewportBottom() = %d, want %d", bottom, expectedBottom)
	}
}

// TestCopyToClipboard_EmptyString verifies empty string handling.
func TestCopyToClipboard_EmptyString(t *testing.T) {
	err := copyToClipboard("")
	if err != nil {
		t.Errorf("copyToClipboard(\"\") returned error: %v", err)
	}
}

// TestOSC52Format verifies OSC 52 escape sequence format.
func TestOSC52Format(t *testing.T) {
	// OSC 52 format: ESC ] 52 ; c ; <base64> BEL
	// We can't easily test stdout writing, but we can verify the format is correct
	text := "test"
	b64 := base64Encode(text)
	expected := "\x1b]52;c;dGVzdA==\x07"

	// Build the sequence manually
	osc := "\x1b]52;c;" + b64 + "\x07"

	if osc != expected {
		t.Errorf("OSC 52 sequence = %q, want %q", osc, expected)
	}
}

// TestScreenRowToContentLine verifies screen row to content line mapping.
func TestScreenRowToContentLine(t *testing.T) {
	m := newTestModel()

	// Set up viewport with content
	content := strings.Repeat("line\n", 20)
	m.viewport.SetContent(content)
	m.viewport.SetHeight(10)

	// Test mapping at y-offset 0
	m.viewport.GotoTop()
	line0 := m.screenRowToContentLine(0)
	if line0 != 0 {
		t.Errorf("screenRowToContentLine(0) at top = %d, want 0", line0)
	}

	// Scroll down
	m.viewport.ScrollDown(5)
	line5 := m.screenRowToContentLine(0)
	if line5 != 5 {
		t.Errorf("screenRowToContentLine(0) after scroll 5 = %d, want 5", line5)
	}
}

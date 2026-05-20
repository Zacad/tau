package tui

import (
	"github.com/charmbracelet/glamour"
)

// RenderMarkdown renders markdown text to ANSI with a dark theme.
// Returns empty string for empty input. Width controls line wrapping.
func RenderMarkdown(text string, width int) string {
	if text == "" {
		return ""
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return text
	}

	out, err := r.Render(text)
	if err != nil {
		return text
	}

	return WrapURLsWithMarkdown(out, text)
}

// NewRenderer creates a reusable glamour term renderer with dark theme.
// Returns error if renderer creation fails. Width controls line wrapping.
// Reusing the renderer avoids ~830µs creation overhead per call.
func NewRenderer(width int) (*glamour.TermRenderer, error) {
	return glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
}

// RenderWithRenderer renders markdown text using a pre-created glamour renderer.
// Falls back to original text on error. Applies OSC 8 hyperlinks to URLs.
func RenderWithRenderer(r *glamour.TermRenderer, text string, originalMarkdown string) string {
	if text == "" {
		return ""
	}

	out, err := r.Render(text)
	if err != nil {
		return text
	}

	return WrapURLsWithMarkdown(out, originalMarkdown)
}

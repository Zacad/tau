package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// selectionState tracks the current text selection.
type selectionState struct {
	active    bool
	startLine int
	startCol  int
	endLine   int
	endCol    int
}

// copyToClipboard copies text to the system clipboard.
// Tries OSC 52 first (works in most modern terminals), then falls back
// to platform-specific clipboard utilities.
func copyToClipboard(text string) error {
	if text == "" {
		return nil
	}

	// Try OSC 52 first
	if err := copyOSC52(text); err == nil {
		return nil
	}

	// Fallback to platform-specific clipboard
	return copyPlatform(text)
}

// copyOSC52 attempts to copy text using the OSC 52 escape sequence.
// This works in most modern terminal emulators (Kitty, Ghostty, Alacritty,
// WezTerm, iTerm2, tmux, screen).
func copyOSC52(text string) error {
	b64 := base64Encode(text)
	osc := fmt.Sprintf("\x1b]52;c;%s\x07", b64)
	_, err := os.Stdout.Write([]byte(osc))
	return err
}

// base64Encode encodes a string to base64 without external dependencies.
func base64Encode(s string) string {
	const encoding = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var b strings.Builder
	b.Grow((len(s) + 2) / 3 * 4)

	data := []byte(s)
	for i := 0; i < len(data); i += 3 {
		b0 := data[i]
		b1 := byte(0)
		b2 := byte(0)
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}

		b.WriteByte(encoding[b0>>2])
		b.WriteByte(encoding[((b0&0x03)<<4)|(b1>>4)])

		if i+1 < len(data) {
			b.WriteByte(encoding[((b1&0x0f)<<2)|(b2>>6)])
		} else {
			b.WriteByte('=')
		}

		if i+2 < len(data) {
			b.WriteByte(encoding[b2&0x3f])
		} else {
			b.WriteByte('=')
		}
	}

	return b.String()
}

// copyPlatform copies text using platform-specific clipboard utilities.
func copyPlatform(text string) error {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	case "windows":
		cmd := exec.Command("clip")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	default:
		// Linux: try wl-copy (Wayland), then xclip (X11), then xsel
		for _, cmdName := range []string{"wl-copy", "xclip", "xsel"} {
			cmd := exec.Command(cmdName)
			if cmdName == "xclip" {
				cmd.Args = append(cmd.Args, "-selection", "clipboard")
			} else if cmdName == "xsel" {
				cmd.Args = append(cmd.Args, "--clipboard", "--input")
			}
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		return fmt.Errorf("no clipboard utility found (tried wl-copy, xclip, xsel)")
	}
}

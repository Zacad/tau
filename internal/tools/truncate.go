package tools

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultMaxOutputChars is the default maximum output size in characters.
const DefaultMaxOutputChars = 10000

// TruncateResult holds a potentially truncated output and metadata.
type TruncateResult struct {
	Output       string // Truncated output (or full output if under limit)
	Truncated    bool   // Whether the output was truncated
	FullOutput   string // Full output (only set if truncated)
	FullOutputPath string // Path to temp file with full output (only set if truncated)
}

// Truncate truncates text to maxChars. If truncated, the full output is saved
// to a temp file and its path is returned in FullOutputPath.
func Truncate(text string, maxChars int) (*TruncateResult, error) {
	if maxChars <= 0 {
		maxChars = DefaultMaxOutputChars
	}

	if len(text) <= maxChars {
		return &TruncateResult{
			Output: text,
		}, nil
	}

	// Save full output to temp file
	tmpDir := os.TempDir()
	f, err := os.CreateTemp(tmpDir, "tau-truncated-*.txt")
	if err != nil {
		return nil, fmt.Errorf("creating temp file for truncated output: %w", err)
	}
	tmpPath := f.Name()

	// Write full output
	if _, writeErr := f.WriteString(text); writeErr != nil {
		f.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("writing truncated output to temp file: %w", writeErr)
	}
	if closeErr := f.Close(); closeErr != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("closing temp file: %w", closeErr)
	}

	return &TruncateResult{
		Output:       text[:maxChars] + "\n... [output truncated, full output saved to: " + filepath.Base(tmpPath) + "]",
		Truncated:    true,
		FullOutput:   text,
		FullOutputPath: tmpPath,
	}, nil
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/adam/tau/internal/types"
)

// ReadTool reads file contents with optional line range limits.
type ReadTool struct {
	workingDir string
	maxChars   int
}

// NewReadTool creates a new read tool.
func NewReadTool(workingDir string, maxChars int) *ReadTool {
	return &ReadTool{workingDir: workingDir, maxChars: maxChars}
}

func (t *ReadTool) Name() string { return "read" }

func (t *ReadTool) Description() string {
	return "Read the contents of a file. Supports text files and images (jpg, png, gif, webp). " +
		"For text files, output is truncated to a configurable limit. Use offset/limit params for large files."
}

func (t *ReadTool) Parameters() any { return &ReadParams{} }

func (t *ReadTool) ExecutionMode() types.ExecutionMode { return types.ExecutionParallel }

// ReadParams defines the parameters for the read tool.
type ReadParams struct {
	// Path is the file path to read (relative or absolute).
	Path string `json:"path" jsonschema:"required,description=File path to read (relative or absolute)"`
	// Limit is the maximum number of lines to read (for text files).
	Limit IntOrString `json:"limit,omitempty" jsonschema:"description=Maximum number of lines to read"`
	// Offset is the line number to start reading from (1-indexed).
	Offset IntOrString `json:"offset,omitempty" jsonschema:"description=Line number to start reading from (1-indexed)"`
}

// IntOrString is a flexible int type that accepts both JSON numbers and strings.
// This handles LLMs that send integer fields as strings.
type IntOrString int

func (i *IntOrString) UnmarshalJSON(data []byte) error {
	// Try number first
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*i = IntOrString(n)
		return nil
	}
	// Try string
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("cannot unmarshal %s as int or string: %w", string(data), err)
	}
	if s == "" || s == "null" {
		*i = 0
		return nil
	}
	parsed, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("cannot convert %q to int: %w", s, err)
	}
	*i = IntOrString(parsed)
	return nil
}

func (t *ReadTool) FilePaths(params any) []string {
	p := params.(*ReadParams)
	return []string{t.resolvePath(p.Path)}
}

func (t *ReadTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*ReadParams)
	path := t.resolvePath(p.Path)

	content, err := os.ReadFile(path)
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Failed to read file %s: %v", path, err),
			}},
		}, nil
	}

	text := string(content)

	// For large files, apply line-based limiting if offset/limit specified
	if p.Offset > 0 || p.Limit > 0 {
		lines := splitLines(text)
		start := 0
		if p.Offset > 0 {
			start = int(p.Offset) - 1
		}
		if start >= len(lines) {
			text = ""
		} else {
			end := len(lines)
			limit := int(p.Limit)
			if limit > 0 && start+limit < end {
				end = start + limit
			}
			text = joinLines(lines[start:end])
		}
	}

	// Apply character truncation
	result, err := Truncate(text, t.maxChars)
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Truncation error: %v", err),
			}},
		}, nil
	}

	return &types.ToolResult{
		Content: []types.ContentBlock{{
			Type: "text",
			Text: result.Output,
		}},
	}, nil
}

func (t *ReadTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.workingDir, path)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for _, line := range lines {
		result += line
	}
	return result
}

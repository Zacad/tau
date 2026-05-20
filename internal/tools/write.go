package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adam/tau/internal/types"
)

// WriteTool creates or overwrites a file.
type WriteTool struct {
	workingDir string
	maxChars   int
}

// NewWriteTool creates a new write tool.
func NewWriteTool(workingDir string, maxChars int) *WriteTool {
	return &WriteTool{workingDir: workingDir, maxChars: maxChars}
}

func (t *WriteTool) Name() string { return "write" }

func (t *WriteTool) Description() string {
	return "Create or overwrite a file. Automatically creates parent directories if they don't exist."
}

func (t *WriteTool) Parameters() any { return &WriteParams{} }

func (t *WriteTool) ExecutionMode() types.ExecutionMode { return types.ExecutionSequential }

// WriteParams defines the parameters for the write tool.
type WriteParams struct {
	// Path is the file path to write (relative or absolute).
	Path string `json:"path" jsonschema:"required,description=File path to write (relative or absolute)"`
	// Content is the file content to write.
	Content string `json:"content" jsonschema:"required,description=File content to write"`
}

func (t *WriteTool) FilePaths(params any) []string {
	p := params.(*WriteParams)
	return []string{t.resolvePath(p.Path)}
}

func (t *WriteTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*WriteParams)
	path := t.resolvePath(p.Path)

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Failed to create directory for %s: %v", path, err),
			}},
		}, nil
	}

	if err := os.WriteFile(path, []byte(p.Content), 0644); err != nil {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Failed to write file %s: %v", path, err),
			}},
		}, nil
	}

	return &types.ToolResult{
		Content: []types.ContentBlock{{
			Type: "text",
			Text: fmt.Sprintf("Successfully wrote %d bytes to %s", len(p.Content), path),
		}},
	}, nil
}

func (t *WriteTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.workingDir, path)
}

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adam/tau/internal/types"
)

// EditTool performs search-and-replace on files with exact match requirement.
type EditTool struct {
	workingDir string
	maxChars   int
}

// NewEditTool creates a new edit tool.
func NewEditTool(workingDir string, maxChars int) *EditTool {
	return &EditTool{workingDir: workingDir, maxChars: maxChars}
}

func (t *EditTool) Name() string { return "edit" }

func (t *EditTool) Description() string {
	return "Edit a file by replacing exactly one occurrence of oldText with newText. " +
		"Requires an exact, unique match. Returns an error with diagnostics if the match is not found or not unique."
}

func (t *EditTool) Parameters() any { return &EditParams{} }

func (t *EditTool) ExecutionMode() types.ExecutionMode { return types.ExecutionSequential }

// EditParams defines the parameters for the edit tool.
type EditParams struct {
	// Path is the file path to edit (relative or absolute).
	Path string `json:"path" jsonschema:"required,description=File path to edit (relative or absolute)"`
	// OldText is the exact text to find and replace.
	OldText string `json:"oldText" jsonschema:"required,description=Exact text to find and replace"`
	// NewText is the replacement text.
	NewText string `json:"newText" jsonschema:"required,description=Replacement text"`
}

func (t *EditTool) FilePaths(params any) []string {
	p := params.(*EditParams)
	return []string{t.resolvePath(p.Path)}
}

func (t *EditTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*EditParams)
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
	count := strings.Count(text, p.OldText)

	if count == 0 {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("No match found for oldText in %s.\n\n"+
					"Tip: use the read tool to verify the exact content and whitespace.", path),
			}},
		}, nil
	}

	if count > 1 {
		// Provide line-based diagnostics
		lines := strings.Split(text, "\n")
		var matchingLines []int
		for i, line := range lines {
			if strings.Contains(line, p.OldText) {
				matchingLines = append(matchingLines, i+1)
			}
		}
		diag := fmt.Sprintf("Found %d matches for oldText in %s. Match must be unique.\n\n"+
			"Matches near lines: %v\n\n"+
			"Tip: include more surrounding context to make the match unique.",
			count, path, matchingLines)
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{Type: "text", Text: diag}},
		}, nil
	}

	// Single match — perform the replacement
	newText := strings.Replace(text, p.OldText, p.NewText, 1)

	if err := os.WriteFile(path, []byte(newText), 0644); err != nil {
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
			Text: fmt.Sprintf("Successfully replaced text in %s", path),
		}},
	}, nil
}

func (t *EditTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.workingDir, path)
}

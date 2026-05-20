package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adam/tau/internal/types"
)

// FindTool searches for files by name pattern.
type FindTool struct {
	workingDir string
	maxChars   int
}

// NewFindTool creates a new find tool.
func NewFindTool(workingDir string, maxChars int) *FindTool {
	return &FindTool{workingDir: workingDir, maxChars: maxChars}
}

func (t *FindTool) Name() string { return "find" }

func (t *FindTool) Description() string {
	return "Find files by name pattern. Supports glob patterns. Returns matching file paths."
}

func (t *FindTool) Parameters() any { return &FindParams{} }

func (t *FindTool) ExecutionMode() types.ExecutionMode { return types.ExecutionParallel }

// FindParams defines the parameters for the find tool.
type FindParams struct {
	// Pattern is the file name pattern (glob syntax, e.g., '*.go').
	Pattern string `json:"pattern" jsonschema:"required,description=File name pattern (glob syntax, e.g., '*.go')"`
	// Path is the directory to search. Defaults to current directory.
	Path string `json:"path,omitempty" jsonschema:"description=Directory to search (defaults to current directory)"`
}

func (t *FindTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*FindParams)

	searchPath := t.workingDir
	if p.Path != "" {
		searchPath = t.resolvePath(p.Path)
	}

	info, err := os.Stat(searchPath)
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Path not found: %s: %v", searchPath, err),
			}},
		}, nil
	}

	if !info.IsDir() {
		// If it's a file, just check if it matches
		if matched, _ := filepath.Match(p.Pattern, filepath.Base(searchPath)); matched {
			relPath, _ := filepath.Rel(t.workingDir, searchPath)
			return &types.ToolResult{
				Content: []types.ContentBlock{{
					Type: "text",
					Text: relPath,
				}},
			}, nil
		}
		return &types.ToolResult{
			Content: []types.ContentBlock{{
				Type: "text",
				Text: "No matches found.",
			}},
		}, nil
	}

	var results []string
	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base != "." && strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			if base == "node_modules" || base == "vendor" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		if matched, _ := filepath.Match(p.Pattern, filepath.Base(path)); matched {
			relPath, _ := filepath.Rel(t.workingDir, path)
			results = append(results, relPath)
		}
		return nil
	})
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Find error: %v", err),
			}},
		}, nil
	}

	output := strings.Join(results, "\n")
	if output == "" {
		output = "No matches found."
	}

	result, truncErr := Truncate(output, t.maxChars)
	if truncErr != nil {
		return nil, truncErr
	}

	return &types.ToolResult{
		Content: []types.ContentBlock{{
			Type: "text",
			Text: result.Output,
		}},
	}, nil
}

func (t *FindTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.workingDir, path)
}

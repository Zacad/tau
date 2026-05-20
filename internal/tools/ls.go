package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adam/tau/internal/types"
)

// LsTool lists directory contents.
type LsTool struct {
	workingDir string
	maxChars   int
}

// NewLsTool creates a new ls tool.
func NewLsTool(workingDir string, maxChars int) *LsTool {
	return &LsTool{workingDir: workingDir, maxChars: maxChars}
}

func (t *LsTool) Name() string { return "ls" }

func (t *LsTool) Description() string {
	return "List the contents of a directory. Shows file names, sizes, and types (file/directory/symlink)."
}

func (t *LsTool) Parameters() any { return &LsParams{} }

func (t *LsTool) ExecutionMode() types.ExecutionMode { return types.ExecutionParallel }

// LsParams defines the parameters for the ls tool.
type LsParams struct {
	// Path is the directory to list. Defaults to current directory.
	Path string `json:"path,omitempty" jsonschema:"description=Directory to list (defaults to current directory)"`
	// LongFormat enables verbose output with file sizes and types.
	LongFormat bool `json:"longFormat,omitempty" jsonschema:"description=If true, show file sizes and types"`
	// AllFiles includes hidden files (starting with .).
	AllFiles bool `json:"allFiles,omitempty" jsonschema:"description=If true, include hidden files"`
}

func (t *LsTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*LsParams)

	listPath := t.workingDir
	if p.Path != "" {
		listPath = t.resolvePath(p.Path)
	}

	info, err := os.Stat(listPath)
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Path not found: %s: %v", listPath, err),
			}},
		}, nil
	}

	if !info.IsDir() {
		// Single file info
		if p.LongFormat {
			output := formatFileInfo(listPath, info)
			return &types.ToolResult{
				Content: []types.ContentBlock{{Type: "text", Text: output}},
			}, nil
		}
		return &types.ToolResult{
			Content: []types.ContentBlock{{
				Type: "text",
				Text: filepath.Base(listPath),
			}},
		}, nil
	}

	entries, err := os.ReadDir(listPath)
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Failed to read directory %s: %v", listPath, err),
			}},
		}, nil
	}

	// Filter hidden files if not requested
	if !p.AllFiles {
		filtered := entries[:0]
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	// Sort entries (directories first, then alphabetically)
	sort.Slice(entries, func(i, j int) bool {
		iDir := entries[i].IsDir()
		jDir := entries[j].IsDir()
		if iDir != jDir {
			return iDir
		}
		return entries[i].Name() < entries[j].Name()
	})

	var lines []string
	if p.LongFormat {
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			lines = append(lines, formatFileInfo(e.Name(), info))
		}
	} else {
		for _, e := range entries {
			isDir := e.IsDir()
			name := e.Name()
			if isDir {
				name += "/"
			}
			lines = append(lines, name)
		}
	}

	output := strings.Join(lines, "\n")
	if output == "" {
		output = "(empty directory)"
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

func formatFileInfo(name string, info os.FileInfo) string {
	size := info.Size()
	typeStr := "file"
	if info.IsDir() {
		typeStr = "dir"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		typeStr = "symlink"
	}
	return fmt.Sprintf("%-8s %10d  %s", typeStr, size, name)
}

func (t *LsTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.workingDir, path)
}

package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/adam/tau/internal/types"
)

// GrepTool searches file contents for a pattern.
type GrepTool struct {
	workingDir string
	maxChars   int
}

// NewGrepTool creates a new grep tool.
func NewGrepTool(workingDir string, maxChars int) *GrepTool {
	return &GrepTool{workingDir: workingDir, maxChars: maxChars}
}

func (t *GrepTool) Name() string { return "grep" }

func (t *GrepTool) Description() string {
	return "Search for a pattern in file contents. Supports regex. Returns matching lines with file paths and line numbers."
}

func (t *GrepTool) Parameters() any { return &GrepParams{} }

func (t *GrepTool) ExecutionMode() types.ExecutionMode { return types.ExecutionParallel }

// GrepParams defines the parameters for the grep tool.
type GrepParams struct {
	// Pattern is the search pattern (regex supported).
	Pattern string `json:"pattern" jsonschema:"required,description=Search pattern (regex supported)"`
	// Path is the file or directory to search. Defaults to current directory.
	Path string `json:"path,omitempty" jsonschema:"description=File or directory to search (defaults to current directory)"`
	// Glob is a file glob pattern to filter files (e.g., '*.go').
	Glob string `json:"glob,omitempty" jsonschema:"description=File glob pattern to filter files (e.g., '*.go')"`
	// CaseSensitive determines if the search is case-sensitive.
	CaseSensitive bool `json:"caseSensitive,omitempty" jsonschema:"description=If true, search is case-sensitive"`
	// MaxResults limits the number of results returned.
	MaxResults int `json:"maxResults,omitempty" jsonschema:"description=Maximum number of results to return"`
}

func (t *GrepTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*GrepParams)

	searchPath := t.workingDir
	if p.Path != "" {
		searchPath = t.resolvePath(p.Path)
	}

	// Compile regex
	reOpts := ""
	if !p.CaseSensitive {
		reOpts = "(?i)"
	}
	re, err := regexp.Compile(reOpts + p.Pattern)
	if err != nil {
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Invalid regex pattern: %v", err),
			}},
		}, nil
	}

	var results []string
	maxResults := p.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	err = filepath.Walk(searchPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}
		if info.IsDir() {
			// Skip hidden directories and common non-source dirs
			base := filepath.Base(path)
			if base != "." && strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			if base == "node_modules" || base == "vendor" || base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply glob filter
		if p.Glob != "" {
			matched, _ := filepath.Match(p.Glob, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read and search file
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if re.MatchString(line) {
				relPath, _ := filepath.Rel(t.workingDir, path)
				results = append(results, fmt.Sprintf("%s:%d:%s", relPath, lineNum, line))
				if len(results) >= maxResults {
					return fmt.Errorf("max results reached")
				}
			}
		}
		return nil
	})

	if err != nil && err.Error() != "max results reached" && ctx.Err() == nil {
		// Walk error that's not our own cutoff or context
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

func (t *GrepTool) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.workingDir, path)
}

package tools

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/adam/tau/internal/types"
)

// BashTool executes shell commands.
type BashTool struct {
	workingDir string
	maxChars   int
	readOnly   bool
}

// NewBashTool creates a new bash tool.
// If readOnly is true, mutating commands are blocked.
func NewBashTool(workingDir string, maxChars int, readOnly bool) *BashTool {
	return &BashTool{workingDir: workingDir, maxChars: maxChars, readOnly: readOnly}
}

func (t *BashTool) Name() string { return "bash" }

func (t *BashTool) Description() string {
	return "Execute a shell command. Use for running scripts, installing packages, git operations, etc."
}

func (t *BashTool) Parameters() any { return &BashParams{} }

func (t *BashTool) ExecutionMode() types.ExecutionMode { return types.ExecutionExclusive }

// BashParams defines the parameters for the bash tool.
type BashParams struct {
	// Command is the shell command to execute.
	Command string `json:"command" jsonschema:"required,description=Shell command to execute"`
}

func (t *BashTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*BashParams)

	// Read-only mode check
	if t.readOnly {
		if isMutatingCommand(p.Command) {
			return &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf("Command blocked in read-only mode: %s", p.Command),
				}},
			}, nil
		}
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", p.Command)
	cmd.Dir = t.workingDir

	output, err := cmd.CombinedOutput()
	text := string(output)

	// Capture exit code
	var exitCode int
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Context cancellation or other error
			result, truncErr := Truncate(text, t.maxChars)
			if truncErr != nil {
				return nil, truncErr
			}
			bashDetails := types.BashExecution{
				Command:  p.Command,
				Output:   result.Output,
				ExitCode: -1,
			}
			if ctx.Err() != nil {
				bashDetails.Cancelled = true
			}
			return &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{
					Type: "text",
					Text: result.Output + "\n\nError: " + err.Error(),
				}},
				Details: bashDetails,
			}, nil
		}
	}

	result, truncErr := Truncate(text, t.maxChars)
	if truncErr != nil {
		return nil, truncErr
	}

	details := types.BashExecution{
		Command:  p.Command,
		Output:   result.Output,
		ExitCode: exitCode,
	}
	if result.Truncated {
		details.Truncated = true
		details.FullOutputPath = result.FullOutputPath
	}

	return &types.ToolResult{
		Content: []types.ContentBlock{{
			Type: "text",
			Text: result.Output,
		}},
		Details: details,
		IsError: exitCode != 0,
	}, nil
}

// mutatingCommands lists command prefixes that mutate state.
var readOnlyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(rm|mv|cp|mkdir|touch|chmod|chown|ln)\b`),
	regexp.MustCompile(`(?i)\b(apt|yum|brew|pip|npm|go\s+install)\b`),
	regexp.MustCompile(`(?i)\b(git\s+(commit|push|pull|add|checkout|switch))\b`),
	regexp.MustCompile(`(?i)>(>|)\s`),     // redirect operators
	regexp.MustCompile(`(?i)\b(dd|mkfs)\b`),
}

func isMutatingCommand(cmd string) bool {
	// Skip comments and empty commands
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return true
	}

	// Check for shell redirection (>>, >, |)
	if strings.Contains(cmd, ">>") || strings.Contains(cmd, ">") || strings.Contains(cmd, "|") {
		// Pipe is not mutating, but redirects are
		if strings.Contains(cmd, ">") {
			return true
		}
	}

	for _, pat := range readOnlyPatterns {
		if pat.MatchString(cmd) {
			return true
		}
	}
	return false
}

package types

// ToolResult is returned by a tool's Execute method.
// It contains the result content and metadata about the execution.
type ToolResult struct {
	Content   []ContentBlock `json:"content"`
	Details   any            `json:"details,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
	Terminate bool           `json:"terminate,omitempty"`
}

// BashExecution captures semantic details about a bash command execution.
// Used by the bash tool (Task 008) to populate structured tool result metadata.
type BashExecution struct {
	Command            string `json:"command"`
	Output             string `json:"output"`
	ExitCode           int    `json:"exit_code"`
	Cancelled          bool   `json:"cancelled,omitempty"`
	Truncated          bool   `json:"truncated,omitempty"`
	FullOutputPath     string `json:"full_output_path,omitempty"`
	ExcludeFromContext bool   `json:"exclude_from_context,omitempty"`
}

// BeforeToolCallContext is passed to the BeforeToolCall hook.
// It allows inspection and potential blocking of a tool call before execution.
type BeforeToolCallContext struct {
	ToolName  string
	Arguments map[string]any
}

// BeforeToolCallResult is returned by the BeforeToolCall hook.
type BeforeToolCallResult struct {
	Allowed bool
	// OverrideArgs can replace the original arguments if Allowed is true.
	OverrideArgs map[string]any
	// BlockReason explains why the call was blocked.
	BlockReason string
}

// AfterToolCallContext is passed to the AfterToolCall hook.
// It allows inspection and transformation of tool results.
type AfterToolCallContext struct {
	ToolName string
	Arguments map[string]any
	Result   *ToolResult
}

// AfterToolCallResult is returned by the AfterToolCall hook.
type AfterToolCallResult struct {
	Result *ToolResult
}

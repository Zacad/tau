// Package tools implements the Tau tool system: tool definitions, execution
// with parallelism modes, file mutation queue, and built-in tools.
//
// Dependency: only internal/types and stdlib (plus invopop/jsonschema for schemas).
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/adam/tau/internal/types"
	"github.com/invopop/jsonschema"
)

// Tool is the interface all tools must implement.
type Tool interface {
	// Name returns the tool's identifier (e.g. "read", "bash").
	Name() string

	// Description returns a human-readable description for the LLM.
	Description() string

	// Parameters returns a pointer to a struct whose fields define the tool's
	// accepted arguments. Used for JSON Schema generation and argument parsing.
	Parameters() any

	// ExecutionMode declares how the tool may run relative to other tools.
	ExecutionMode() types.ExecutionMode

	// Execute runs the tool with the parsed parameters and returns a result.
	Execute(ctx context.Context, params any) (*types.ToolResult, error)
}

// FileTool is optionally implemented by tools that mutate specific files.
// The Registry uses it to acquire per-file locks for sequential tools.
type FileTool interface {
	Tool
	// FilePaths returns the file paths this call operates on, extracted from
	// the already-parsed params struct.
	FilePaths(params any) []string
}

// ToolCallRequest represents a single tool invocation from the LLM.
type ToolCallRequest struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// parsedCall holds a validated tool call ready for execution.
type parsedCall struct {
	index  int
	tool   Tool
	params any
}

// ToolCallResult holds the outcome of a single tool invocation.
type ToolCallResult struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Result *types.ToolResult `json:"result"`
}

// BeforeToolCallFunc is the hook signature called before a tool executes.
type BeforeToolCallFunc func(types.BeforeToolCallContext) (*types.BeforeToolCallResult, error)

// AfterToolCallFunc is the hook signature called after a tool executes.
type AfterToolCallFunc func(types.AfterToolCallContext) (*types.AfterToolCallResult, error)

// Registry holds registered tools and handles batched execution with
// execution-mode enforcement, allowlisting, and read-only mode.
type Registry struct {
	mu          sync.RWMutex
	tools       map[string]Tool
	allowlist   map[string]bool // nil means all allowed
	readOnly    bool
	queue       *MutationQueue
	beforeCall  BeforeToolCallFunc
	afterCall   AfterToolCallFunc
}

// RegistryOption configures a Registry.
type RegistryOption func(*Registry)

// WithAllowlist restricts the registry to the named tools.
// Tools not in the list return an error on ExecuteBatch.
func WithAllowlist(names []string) RegistryOption {
	return func(r *Registry) {
		r.allowlist = make(map[string]bool, len(names))
		for _, n := range names {
			r.allowlist[n] = true
		}
	}
}

// WithReadOnly enables read-only mode: write, edit, and bash tools
// return an error on ExecuteBatch.
func WithReadOnly(v bool) RegistryOption {
	return func(r *Registry) {
		r.readOnly = v
	}
}

// WithBeforeToolCall sets the pre-execution hook.
func WithBeforeToolCall(fn BeforeToolCallFunc) RegistryOption {
	return func(r *Registry) {
		r.beforeCall = fn
	}
}

// WithAfterToolCall sets the post-execution hook.
func WithAfterToolCall(fn AfterToolCallFunc) RegistryOption {
	return func(r *Registry) {
		r.afterCall = fn
	}
}

// NewRegistry creates a tool registry with the given options.
func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{
		tools: make(map[string]Tool),
		queue: NewMutationQueue(),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register adds a tool to the registry. Panics on duplicate name.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[t.Name()]; exists {
		panic(fmt.Sprintf("tool already registered: %s", t.Name()))
	}
	r.tools[t.Name()] = t
}

// Get returns a tool by name, or nil if not registered.
func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// Names returns a sorted list of registered tool names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Tools returns all registered tool instances.
func (r *Registry) Tools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		all = append(all, t)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name() < all[j].Name()
	})
	return all
}

// ToolDefinitions returns JSON Schema definitions for all registered tools,
// suitable for passing to a provider's tool-calling API.
func (r *Registry) ToolDefinitions() []types.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Sort for deterministic output
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)

	defs := make([]types.ToolDefinition, 0, len(names))
	for _, name := range names {
		t := r.tools[name]
		schema := jsonschema.Reflect(t.Parameters())
		defs = append(defs, types.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  schema,
		})
	}
	return defs
}

// ExecuteBatch executes a batch of tool calls, respecting execution modes.
// Results are returned in the same order as the input calls.
//
// Execution order: exclusive first (bash), then parallel, then sequential.
// Sequential tools are serialized per-file via the mutation queue.
func (r *Registry) ExecuteBatch(ctx context.Context, calls []ToolCallRequest) []*ToolCallResult {
	results := make([]*ToolCallResult, len(calls))

	// Preflight: validate and parse all calls
	var exclusiveCalls []parsedCall
	var parallelCalls []parsedCall
	var sequentialCalls []parsedCall

	for i, call := range calls {
		results[i] = &ToolCallResult{ID: call.ID, Name: call.Name}

		tool := r.Get(call.Name)
		if tool == nil {
			results[i].Result = &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{Type: "text", Text: fmt.Sprintf("Unknown tool: %s", call.Name)}},
			}
			continue
		}

		// Check allowlist
		if !r.isAllowed(call.Name) {
			results[i].Result = &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{Type: "text", Text: fmt.Sprintf("Tool not allowed: %s", call.Name)}},
			}
			continue
		}

		// Check read-only
		if r.readOnly && isMutatingTool(tool) {
			results[i].Result = &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{Type: "text", Text: fmt.Sprintf("Tool blocked in read-only mode: %s", call.Name)}},
			}
			continue
		}

		// Parse arguments into tool's parameter struct
		params := tool.Parameters()
		if len(call.Arguments) > 0 {
			slog.Debug("tool call raw arguments", "tool", call.Name, "raw", string(call.Arguments))
			if err := json.Unmarshal(call.Arguments, params); err != nil {
				results[i].Result = &types.ToolResult{
					IsError: true,
					Content: []types.ContentBlock{{Type: "text", Text: fmt.Sprintf("Invalid arguments for %s: %v", call.Name, err)}},
				}
				continue
			}
		}
		slog.Debug("tool call parsed params", "tool", call.Name, "params", fmt.Sprintf("%+v", params))

		// BeforeToolCall hook
		if r.beforeCall != nil {
			argsMap := make(map[string]any)
			if len(call.Arguments) > 0 {
				_ = json.Unmarshal(call.Arguments, &argsMap)
			}
			hookResult, err := r.beforeCall(types.BeforeToolCallContext{
				ToolName:  call.Name,
				Arguments: argsMap,
			})
			if err != nil {
				results[i].Result = &types.ToolResult{
					IsError: true,
					Content: []types.ContentBlock{{Type: "text", Text: fmt.Sprintf("BeforeToolCall hook error: %v", err)}},
				}
				continue
			}
			if !hookResult.Allowed {
				results[i].Result = &types.ToolResult{
					IsError: true,
					Content: []types.ContentBlock{{Type: "text", Text: fmt.Sprintf("Tool call blocked: %s", hookResult.BlockReason)}},
				}
				continue
			}
			if hookResult.OverrideArgs != nil {
				params = tool.Parameters()
				overrideJSON, _ := json.Marshal(hookResult.OverrideArgs)
				if err := json.Unmarshal(overrideJSON, params); err != nil {
					results[i].Result = &types.ToolResult{
						IsError: true,
						Content: []types.ContentBlock{{Type: "text", Text: fmt.Sprintf("Override args parse error: %v", err)}},
					}
					continue
				}
			}
		}

		pc := parsedCall{index: i, tool: tool, params: params}
		switch tool.ExecutionMode() {
		case types.ExecutionExclusive:
			exclusiveCalls = append(exclusiveCalls, pc)
		case types.ExecutionParallel:
			parallelCalls = append(parallelCalls, pc)
		case types.ExecutionSequential:
			sequentialCalls = append(sequentialCalls, pc)
		}
	}

	// Execute exclusive calls (bash) — one at a time
	for _, pc := range exclusiveCalls {
		r.executeTool(ctx, pc.tool, pc.params, results, pc.index)
	}

	// Execute sequential calls via per-file mutex queue (write/edit before reads)
	for _, pc := range sequentialCalls {
		r.executeSequential(ctx, pc, results)
	}

	// Execute parallel calls concurrently (read/grep/find/ls — after mutations)
	if len(parallelCalls) > 0 {
		var wg sync.WaitGroup
		for _, pc := range parallelCalls {
			wg.Add(1)
			go func(pc parsedCall) {
				defer wg.Done()
				r.executeTool(ctx, pc.tool, pc.params, results, pc.index)
			}(pc)
		}
		wg.Wait()
	}

	return results
}

// executeTool runs a single tool and stores the result, applying AfterToolCall hook.
func (r *Registry) executeTool(ctx context.Context, tool Tool, params any, results []*ToolCallResult, index int) {
	result, err := tool.Execute(ctx, params)
	if err != nil {
		result = &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{Type: "text", Text: fmt.Sprintf("Tool execution error: %v", err)}},
		}
	}

	if r.afterCall != nil {
		argsMap := make(map[string]any)
		if b, e := json.Marshal(params); e == nil {
			_ = json.Unmarshal(b, &argsMap)
		}
		hookResult, hookErr := r.afterCall(types.AfterToolCallContext{
			ToolName:  tool.Name(),
			Arguments: argsMap,
			Result:    result,
		})
		if hookErr == nil && hookResult != nil && hookResult.Result != nil {
			result = hookResult.Result
		}
	}

	results[index].Result = result
}

// executeSequential runs a sequential tool with per-file locking.
func (r *Registry) executeSequential(ctx context.Context, pc parsedCall, results []*ToolCallResult) {
	var filePaths []string
	if ft, ok := pc.tool.(FileTool); ok {
		filePaths = ft.FilePaths(pc.params)
	}

	if len(filePaths) == 0 {
		// No file paths — execute without locking
		r.executeTool(ctx, pc.tool, pc.params, results, pc.index)
		return
	}

	// Acquire locks for all file paths (in sorted order for deadlock avoidance)
	sorted := make([]string, len(filePaths))
	copy(sorted, filePaths)
	sort.Strings(sorted)

	releases := make([]func(), 0, len(sorted))
	for _, fp := range sorted {
		releases = append(releases, r.queue.Acquire(fp))
	}
	defer func() {
		for _, rel := range releases {
			rel()
		}
	}()

	r.executeTool(ctx, pc.tool, pc.params, results, pc.index)
}

func (r *Registry) isAllowed(name string) bool {
	if r.allowlist == nil {
		return true
	}
	return r.allowlist[name]
}

// mutatingTools is the set of tool names that mutate state.
var mutatingTools = map[string]bool{
	"write": true,
	"edit":  true,
	"bash":  true,
}

func isMutatingTool(t Tool) bool {
	return mutatingTools[t.Name()]
}

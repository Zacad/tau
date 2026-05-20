package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/adam/tau/internal/provider"
	"github.com/adam/tau/internal/subagent"
	"github.com/adam/tau/internal/types"
)

// SubAgentTool spawns a sub-agent to execute a task.
type SubAgentTool struct {
	prov            provider.Provider
	model           types.Model
	parentRegistry  *Registry
	parentToolNames []string
	discoveredAgents map[string]*subagent.AgentDefinition
}

// NewSubAgentTool creates a new subagent spawn tool.
// parentRegistry and parentToolNames are used to enforce the tool ceiling
// and provide tool execution to the sub-agent.
// discoveredAgents contains user-defined agents from ~/.tau/agents/ and .tau/agents/.
func NewSubAgentTool(prov provider.Provider, model types.Model, parentRegistry *Registry, parentToolNames []string, discoveredAgents map[string]*subagent.AgentDefinition) *SubAgentTool {
	return &SubAgentTool{
		prov:             prov,
		model:            model,
		parentRegistry:   parentRegistry,
		parentToolNames:  parentToolNames,
		discoveredAgents: discoveredAgents,
	}
}

func (t *SubAgentTool) Name() string { return "subagent" }

func (t *SubAgentTool) Description() string {
	desc := "Launch a sub-agent to handle complex, multi-step tasks autonomously. " +
		"The sub-agent runs with fresh context (no prior conversation history) and returns its output.\n\n" +
		"Built-in agent types (use 'type' parameter):\n" +
		"- general: Versatile default for any task (read, write, edit, bash, grep, find, ls, websearch, webfetch)\n" +
		"- researcher: Research and information gathering (read, grep, find, ls, bash, websearch, webfetch)\n" +
		"- reviewer: Code/content review (read, grep, find, ls)\n" +
		"- implementor: Feature implementation (read, write, edit, bash, grep, find, ls)\n" +
		"- security_reviewer: Security analysis (read, grep, find, bash)\n" +
		"- qa: Testing and quality assurance (read, bash, grep, find, ls, write)"

	if len(t.discoveredAgents) > 0 {
		desc += "\n\nUser-defined agents (use 'agent_name' parameter):\n"
		for name, def := range t.discoveredAgents {
			desc += fmt.Sprintf("- %s: %s\n", name, def.Description)
		}
	}

	desc += "\nIMPORTANT: When the user asks you to use a specific agent (e.g., 'use the greeter agent', 'run the reviewer'), you MUST call this tool with the appropriate 'type' or 'agent_name' parameter.\n\n" +
		"When to use the subagent tool:\n" +
		"- When the user says 'use agent X', 'run X agent', 'spawn X', or similar — call this tool with agent_name='X' or type='X'\n" +
		"- For research tasks that require web searches or reading multiple files\n" +
		"- For code review or analysis across multiple files\n" +
		"- For implementation tasks that are self-contained and well-defined\n" +
		"- For tasks that would consume significant context in the main conversation\n" +
		"- When the user explicitly asks to use a sub-agent\n\n" +
		"When NOT to use the subagent tool:\n" +
		"- If you want to read a specific file, use the read tool instead\n" +
		"- If you are searching for a specific pattern, use grep or find instead\n" +
		"- For simple questions you can answer directly\n" +
		"- For tasks requiring full conversation context\n\n" +
		"Usage notes:\n" +
		"- Provide a detailed, self-contained task description — the sub-agent has no prior context\n" +
		"- Specify exactly what information to return in the final output\n" +
		"- Use 'type' for built-in agents (e.g., type='researcher') or 'agent_name' for user-defined agents (e.g., agent_name='greeter')\n" +
		"- The sub-agent's output should generally be trusted\n" +
		"- Results are returned as structured JSON with subagent_id, type, task, output, and duration"
	return desc
}

func (t *SubAgentTool) Parameters() any { return &SubAgentParams{} }

func (t *SubAgentTool) ExecutionMode() types.ExecutionMode { return types.ExecutionExclusive }

// SubAgentParams defines the parameters for spawning a sub-agent.
type SubAgentParams struct {
	// Task is the task description for the sub-agent (required).
	Task string `json:"task" jsonschema:"required,description=Task description for the sub-agent to execute"`
	// Type is the built-in agent type to use (optional, defaults to "general").
	// Valid types: general, researcher, reviewer, implementor, security_reviewer, qa.
	Type string `json:"type,omitempty" jsonschema:"description=Built-in agent type: general, researcher, reviewer, implementor, security_reviewer, qa. Defaults to general."`
	// AgentName is a user-defined agent name (optional, mutually exclusive with type).
	// Discovered from ~/.tau/agents/ and .tau/agents/ directories.
	AgentName string `json:"agent_name,omitempty" jsonschema:"description=User-defined agent name from ~/.tau/agents/ or .tau/agents/ directories."`
	// Model is the model ID to use (optional, defaults to parent's model).
	Model string `json:"model,omitempty" jsonschema:"description=Model ID to use (defaults to parent model)"`
	// SystemPrompt is an optional system prompt for the sub-agent.
	SystemPrompt string `json:"system_prompt,omitempty" jsonschema:"description=Optional system prompt for the sub-agent"`
	// Timeout is the maximum duration for the sub-agent to run (optional, default 5m).
	// Specify as a Go duration string, e.g. "30s", "2m", "10m".
	Timeout string `json:"timeout,omitempty" jsonschema:"description=Maximum duration for sub-agent (e.g. '30s', '2m', '10m'). Defaults to 5 minutes."`
}

func (t *SubAgentTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*SubAgentParams)

	model := t.model
	if p.Model != "" {
		model = types.Model{
			ID:       p.Model,
			Provider: t.model.Provider,
			API:      t.model.API,
		}
	}

	var timeout time.Duration
	if p.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(p.Timeout)
		if err != nil {
			return &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf("Invalid timeout duration %q: %v", p.Timeout, err),
				}},
			}, nil
		}
	}

	executor := t.buildExecutor()

	var subTools []subagent.Tool
	if t.parentRegistry != nil {
		parentTools := t.parentRegistry.Tools()
		subTools = make([]subagent.Tool, 0, len(parentTools))
		for _, pt := range parentTools {
			if pt.Name() == "subagent" {
				continue
			}
			subTools = append(subTools, pt)
		}
	}

	var sa *subagent.SubAgent
	var agentTypeStr string

	if p.AgentName != "" {
		// User-defined agent
		def, ok := t.discoveredAgents[p.AgentName]
		if !ok {
			return &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf("User-defined agent %q not found. Available agents: %v", p.AgentName, t.availableAgentNames()),
				}},
			}, nil
		}

		// Filter tools to agent's defined set
		var filteredTools []subagent.Tool
		if len(def.Tools) > 0 {
			allowed := make(map[string]bool, len(def.Tools))
			for _, name := range def.Tools {
				allowed[name] = true
			}
			for _, pt := range subTools {
				if allowed[pt.Name()] {
					filteredTools = append(filteredTools, pt)
				}
			}
		} else {
			filteredTools = subTools
		}

		// Override model if specified in definition
		agentModel := model
		if def.Model != "" {
			agentModel = types.Model{
				ID:       def.Model,
				Provider: t.model.Provider,
				API:      t.model.API,
			}
		}

		systemPrompt := def.SystemPrompt
		if p.SystemPrompt != "" {
			systemPrompt = p.SystemPrompt
		}

		sa = subagent.NewSubAgent(t.prov, subagent.SubAgentOpts{
			Task:            p.Task,
			SystemPrompt:    systemPrompt,
			Model:           agentModel,
			Timeout:         timeout,
			Tools:           filteredTools,
			ParentToolNames: t.parentToolNames,
			Executor:        executor,
		})
		agentTypeStr = def.Name
	} else if p.Type != "" {
		// Built-in agent type
		agentType, ok := subagent.ParseType(p.Type)
		if !ok {
			validTypes := subagent.AllTypes()
			return &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf("Unknown agent type %q. Valid types: %v", p.Type, validTypes),
				}},
			}, nil
		}

		var err error
		sa, err = subagent.NewSubAgentByType(agentType, t.prov, subTools, subagent.SubAgentOpts{
			Task:            p.Task,
			SystemPrompt:    p.SystemPrompt,
			Model:           model,
			Timeout:         timeout,
			ParentToolNames: t.parentToolNames,
			Executor:        executor,
		})
		if err != nil {
			return &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf("Failed to create subagent: %v", err),
				}},
			}, nil
		}
		agentTypeStr = string(agentType)
	} else {
		// No type or agent_name — use general defaults
		sa = subagent.NewSubAgent(t.prov, subagent.SubAgentOpts{
			Task:            p.Task,
			SystemPrompt:    p.SystemPrompt,
			Model:           model,
			Timeout:         timeout,
			Tools:           subTools,
			ParentToolNames: t.parentToolNames,
			Executor:        executor,
		})
		agentTypeStr = ""
	}

	slog.Info("subagent: start",
		"id", sa.ID,
		"model", model.ID,
		"provider", model.Provider,
		"task_preview", truncateTask(p.Task, 120),
		"timeout", sa.Timeout,
	)

	result := sa.Run(ctx)

	if !result.Success {
		slog.Warn("subagent: failed",
			"id", sa.ID,
			"timeout", result.Timeout,
			"error", result.Error,
			"duration", result.Duration,
		)
		return &types.ToolResult{
			IsError: true,
			Content: []types.ContentBlock{{
				Type: "text",
				Text: fmt.Sprintf("Sub-agent %s failed: %v", sa.ID, result.Error),
			}},
		}, nil
	}

	slog.Info("subagent: complete",
		"id", sa.ID,
		"duration", result.Duration,
		"output_len", len(result.Output),
		"success", result.Success,
	)

	output, _ := json.Marshal(map[string]any{
		"subagent_id": sa.ID,
		"type":        agentTypeStr,
		"task":        p.Task,
		"output":      result.Output,
		"duration":    result.Duration.String(),
	})

	return &types.ToolResult{
		Content: []types.ContentBlock{{
			Type: "text",
			Text: string(output),
		}},
	}, nil
}

// buildExecutor creates a subagent.Executor that delegates to the parent registry.
func (t *SubAgentTool) buildExecutor() subagent.Executor {
	if t.parentRegistry == nil {
		return nil
	}
	return func(ctx context.Context, calls []subagent.ToolCallRequest) []*subagent.ToolCallResult {
		toolCalls := make([]ToolCallRequest, len(calls))
		for i, c := range calls {
			toolCalls[i] = ToolCallRequest{
				ID:        c.ID,
				Name:      c.Name,
				Arguments: c.Arguments,
			}
		}

		results := t.parentRegistry.ExecuteBatch(ctx, toolCalls)

		subResults := make([]*subagent.ToolCallResult, len(results))
		for i, r := range results {
			subResults[i] = &subagent.ToolCallResult{
				ID:     r.ID,
				Name:   r.Name,
				Result: r.Result,
			}
		}
		return subResults
	}
}

// availableAgentNames returns a sorted list of all available agent names
// (built-in types + user-defined agents).
func (t *SubAgentTool) availableAgentNames() []string {
	names := make([]string, 0, len(t.discoveredAgents)+len(subagent.AllTypes()))
	for _, typ := range subagent.AllTypes() {
		names = append(names, string(typ))
	}
	for name := range t.discoveredAgents {
		names = append(names, name)
	}
	// Sort for consistent output
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[i] > names[j] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

func truncateTask(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

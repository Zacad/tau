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

// minSubagentTimeout is the minimum timeout enforced for subagents.
// Prevents the LLM from setting unrealistically short timeouts that would fail
// on tasks requiring multiple model/tool iterations.
const minSubagentTimeout = 5 * time.Minute

// subagentDefaultCandidate defines a single fallback model candidate.
// The system walks through this list in order and uses the first candidate
// whose provider is registered and model is available.
type subagentDefaultCandidate struct {
	// ModelID is the bare model identifier (e.g. "gemma4", "gpt-5.4-mini").
	ModelID string
	// Provider is the expected provider name (must match registry).
	Provider string
}

// subagentDefaultModels defines the ordered fallback list for subagent execution.
// Priority: local (free, no auth) → cloud (cost, auth required).
// Model IDs use substring matching via ResolveModelWithFallback, so bare IDs
// like "gemma4" will match "gemma4:26b", "gemma4:4b", etc. in the catalog.
// When no model is specified (via frontmatter or prompt), the system walks
// through this list and uses the first available candidate.
var subagentDefaultModels = []subagentDefaultCandidate{
	// Ollama — local, no auth, no cost
	{ModelID: "gemma4", Provider: "ollama"},
	// Anthropic
	{ModelID: "claude-haiku-4-5", Provider: "anthropic"},
	// OpenAI
	{ModelID: "gpt-5.4-mini", Provider: "openai"},
	// Qwen 3.6 via multiple providers (in priority order)
	{ModelID: "qwen3.6-plus", Provider: "opencode-zen"},
	{ModelID: "qwen3.6-plus", Provider: "opencode-go"},
	{ModelID: "qwen3.6-plus", Provider: "openrouter"},
}

// SubAgentTool spawns a sub-agent to execute a task.
type SubAgentTool struct {
	prov             provider.Provider
	model            types.Model
	provReg          *provider.Registry
	parentRegistry   *Registry
	parentToolNames  []string
	discoveredAgents map[string]*subagent.AgentDefinition
	defaultTimeout   time.Duration
}

// NewSubAgentTool creates a new subagent spawn tool.
// parentRegistry and parentToolNames are used to enforce the tool ceiling
// and provide tool execution to the sub-agent.
// discoveredAgents contains user-defined agents from ~/.tau/agents/ and .tau/agents/.
// provReg is the provider registry used for model resolution — if nil, model
// resolution falls back to inheriting the parent's provider (legacy behavior).
// defaultTimeout is the default timeout from config, used when the LLM doesn't specify one.
func NewSubAgentTool(prov provider.Provider, model types.Model, provReg *provider.Registry, parentRegistry *Registry, parentToolNames []string, discoveredAgents map[string]*subagent.AgentDefinition, defaultTimeout time.Duration) *SubAgentTool {
	return &SubAgentTool{
		prov:             prov,
		model:            model,
		provReg:          provReg,
		parentRegistry:   parentRegistry,
		parentToolNames:  parentToolNames,
		discoveredAgents: discoveredAgents,
		defaultTimeout:   defaultTimeout,
	}
}

func (t *SubAgentTool) Name() string { return "subagent" }

// UpdateParentModel updates the parent model and provider that subagents
// inherit when no explicit model is specified. This is called by
// Session.SetModel so that future subagent spawns use the new parent model.
func (t *SubAgentTool) UpdateParentModel(prov provider.Provider, model types.Model) {
	t.prov = prov
	t.model = model
}

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
	// Specify as a Go duration string, e.g. "5m", "10m", "20m".
	Timeout string `json:"timeout,omitempty" jsonschema:"description=Maximum duration for sub-agent (e.g. '5m', '10m', '20m'). Omit unless the user explicitly requests a timeout. Defaults to 5 minutes."`
}

func (t *SubAgentTool) Execute(ctx context.Context, params any) (*types.ToolResult, error) {
	p := params.(*SubAgentParams)

	// Model resolution is handled via resolveSubAgentModel() for each agent branch.
	// Priority: 1) frontmatter model  2) prompt model  3) parent model  4) defaults

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
	if timeout <= 0 {
		if t.defaultTimeout > 0 {
			timeout = t.defaultTimeout
		} else {
			timeout = subagent.DefaultTimeout
		}
	}
	if timeout < minSubagentTimeout {
		slog.Debug("subagent: enforcing minimum timeout",
			"requested", timeout, "minimum", minSubagentTimeout)
		timeout = minSubagentTimeout
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
	var subProv provider.Provider
	var subModel types.Model
	// explicitModelRequested tracks whether the user asked for a specific model
	// that couldn't be resolved. In that case, we should try defaults before
	// falling back to the parent model (which may use the wrong provider).
	explicitModelRequested := p.Model != ""

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

		// Resolve model: frontmatter model takes priority over prompt model
		var err error
		subModel, subProv, err = t.resolveSubAgentModel(def.Model, p.Model)
		if err != nil {
			return &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf("Failed to resolve subagent model: %v", err),
				}},
			}, nil
		}

		systemPrompt := def.SystemPrompt
		if p.SystemPrompt != "" {
			systemPrompt = p.SystemPrompt
		}

		sa = subagent.NewSubAgent(subProv, subagent.SubAgentOpts{
			Task:            p.Task,
			SystemPrompt:    systemPrompt,
			Model:           subModel,
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

		// Resolve model: prompt model takes priority, then parent model, then defaults
		var err error
		subModel, subProv, err = t.resolveSubAgentModel("", p.Model)
		if err != nil {
			return &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf("Failed to resolve subagent model: %v", err),
				}},
			}, nil
		}

		sa, err = subagent.NewSubAgentByType(agentType, subProv, subTools, subagent.SubAgentOpts{
			Task:            p.Task,
			SystemPrompt:    p.SystemPrompt,
			Model:           subModel,
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
		// No type or agent_name — resolve model: prompt → parent → defaults
		var err error
		subModel, subProv, err = t.resolveSubAgentModel("", p.Model)
		if err != nil {
			return &types.ToolResult{
				IsError: true,
				Content: []types.ContentBlock{{
					Type: "text",
					Text: fmt.Sprintf("Failed to resolve subagent model: %v", err),
				}},
			}, nil
		}

		sa = subagent.NewSubAgent(subProv, subagent.SubAgentOpts{
			Task:            p.Task,
			SystemPrompt:    p.SystemPrompt,
			Model:           subModel,
			Timeout:         timeout,
			Tools:           subTools,
			ParentToolNames: t.parentToolNames,
			Executor:        executor,
		})
		agentTypeStr = ""
	}

	// Step 4 fallback: try subagent default models list when:
	// a) the resolved model's provider is not registered/unavailable, OR
	// b) the user explicitly requested a model but resolution fell through to parent
	//    (meaning the requested model couldn't be resolved — try defaults before
	//    using parent, since parent may use a completely different provider).
	needsDefaultFallback := false
	if t.provReg != nil {
		_, providerOK := t.provReg.Get(subModel.Provider)
		if !providerOK {
			needsDefaultFallback = true
		} else if explicitModelRequested && subModel.ID == t.model.ID {
			// User asked for a specific model but we fell back to parent — try defaults first
			needsDefaultFallback = true
			slog.Debug("subagent: requested model couldn't be resolved, trying defaults before parent",
				"requested", p.Model, "fell_to_parent", subModel.ID)
		}
	} else {
		// No registry — parent provider is the only option
		needsDefaultFallback = false
	}

	if needsDefaultFallback {
		if fallbackModel, fallbackProv, ok := t.trySubagentDefaults(); ok {
			subModel = fallbackModel
			subProv = fallbackProv
			// Re-create the subagent with the fallback model
			sa = subagent.NewSubAgent(subProv, subagent.SubAgentOpts{
				Task:            p.Task,
				SystemPrompt:    t.buildSystemPromptForSubagent(p),
				Model:           subModel,
				Timeout:         timeout,
				Tools:           subTools,
				ParentToolNames: t.parentToolNames,
				Executor:        executor,
			})
		}
	}

	slog.Info("subagent: start",
		"id", sa.ID,
		"model", subModel.ID,
		"provider", subModel.Provider,
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
		"model":       subModel.ID,
		"provider":    subModel.Provider,
		"timeout":     timeout.String(),
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

// buildSystemPromptForSubagent constructs the system prompt for a subagent
// when re-creating it with a fallback model. Mirrors the prompt-building
// logic in the Execute method.
func (t *SubAgentTool) buildSystemPromptForSubagent(p *SubAgentParams) string {
	return p.SystemPrompt
}

// resolveSubAgentModel resolves a subagent model using the priority chain:
// 1. Use model defined in agent frontmatter (agentModel) — resolved via registry
// 2. Use model specified in prompt (promptModel) — resolved via registry
// 3. Use parent agent's model (parentModel)
// 4. Fallback to subagent default models — first available from any registered provider
//
// Returns both the resolved Model and the corresponding Provider instance.
// If provReg is nil, falls back to legacy behavior (inherit parent provider) for
// custom models — this preserves backward compatibility for unit tests.
func (t *SubAgentTool) resolveSubAgentModel(agentModel, promptModel string) (types.Model, provider.Provider, error) {
	// Helper: try to resolve a model pattern via the registry
	tryResolve := func(pattern string) (types.Model, provider.Provider, bool) {
		if pattern == "" || t.provReg == nil {
			return types.Model{}, nil, false
		}
		model, err := t.provReg.ResolveModelWithFallback(pattern)
		if err != nil {
			slog.Debug("subagent: model resolution failed", "pattern", pattern, "error", err)
			return types.Model{}, nil, false
		}
		prov, ok := t.provReg.Get(model.Provider)
		if !ok {
			slog.Debug("subagent: provider not registered for resolved model",
				"model", model.ID, "provider", model.Provider)
			return types.Model{}, nil, false
		}
		return model, prov, true
	}

	// Helper: legacy fallback — create model with inherited parent provider.
	// Only used when provReg is nil (legacy test mode).
	legacyResolve := func(modelID string) (types.Model, provider.Provider) {
		return types.Model{
			ID:       modelID,
			Provider: t.model.Provider,
			API:      t.model.API,
		}, t.prov
	}

	// Step 1: Use model from agent frontmatter
	if agentModel != "" {
		if model, prov, ok := tryResolve(agentModel); ok {
			slog.Debug("subagent: using agent frontmatter model", "model", model.ID, "provider", model.Provider)
			return model, prov, nil
		}
		// Registry unavailable (provReg is nil) — legacy fallback for tests
		if t.provReg == nil {
			model, prov := legacyResolve(agentModel)
			slog.Debug("subagent: using frontmatter model (legacy fallback)", "model", model.ID)
			return model, prov, nil
		}
		// Registry exists but model resolution failed — return empty so caller
		// can try trySubagentDefaults with other providers.
		slog.Debug("subagent: frontmatter model resolution failed, falling through to parent/default",
			"model", agentModel)
	}

	// Step 2: Use model specified in prompt
	if promptModel != "" {
		if model, prov, ok := tryResolve(promptModel); ok {
			slog.Debug("subagent: using prompt model", "model", model.ID, "provider", model.Provider)
			return model, prov, nil
		}
		// Registry unavailable (provReg is nil) — legacy fallback for tests
		if t.provReg == nil {
			model, prov := legacyResolve(promptModel)
			slog.Debug("subagent: using prompt model (legacy fallback)", "model", model.ID)
			return model, prov, nil
		}
		// Registry exists but model resolution failed — return empty so caller
		// can try trySubagentDefaults with other providers.
		slog.Debug("subagent: prompt model resolution failed, falling through to parent/default",
			"model", promptModel)
	}

	// Step 3: Use parent agent's model
	slog.Debug("subagent: using parent model", "model", t.model.ID, "provider", t.model.Provider)
	return t.model, t.prov, nil

	// Step 4 (fallback) is handled by the caller — only reached if parent model is also unavailable.
}

// trySubagentDefaults walks through the subagent default candidates list and
// returns the first candidate whose provider is registered and model exists.
func (t *SubAgentTool) trySubagentDefaults() (types.Model, provider.Provider, bool) {
	if t.provReg == nil {
		return types.Model{}, nil, false
	}
	for _, c := range subagentDefaultModels {
		// Check provider is registered
		prov, ok := t.provReg.Get(c.Provider)
		if !ok {
			slog.Debug("subagent: skipping default candidate, provider not registered",
				"model", c.ModelID, "provider", c.Provider)
			continue
		}
		// Try to resolve the model within this provider
		model, err := t.provReg.ResolveModelWithFallback(c.ModelID)
		if err != nil {
			slog.Debug("subagent: skipping default candidate, model not found",
				"model", c.ModelID, "provider", c.Provider, "error", err)
			continue
		}
		// Verify the resolved model actually belongs to the expected provider
		if model.Provider != c.Provider {
			slog.Debug("subagent: skipping default candidate, resolved to different provider",
				"requested", c.ModelID, "expected_provider", c.Provider,
				"resolved_provider", model.Provider)
			continue
		}
		slog.Info("subagent: falling back to default model", "model", model.ID, "provider", model.Provider)
		return model, prov, true
	}
	return types.Model{}, nil, false
}

func truncateTask(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

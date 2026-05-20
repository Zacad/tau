---
name: subagent-builder
description: |
  Create and manage subagent definitions for Tau. Use this skill when you need to
  define new subagent types, customize agent behavior, or build reusable delegation
  workflows. Covers subagent lifecycle, context modes (fresh/fork), tool scoping,
  and result handling patterns.
scripts: []
references: []
assets: []
---

# Subagent Builder

Define and configure subagents that the Tau orchestrator can spawn during agent loops.
Subagents enable delegation of specialized work (research, review, implementation) to
isolated contexts with controlled capabilities.

## When to Use

- Define a new subagent type for a specific role or workflow
- Customize tool sets for existing subagent types
- Configure context inheritance (fresh vs fork) for delegation patterns
- Build reusable agent definitions for project-specific workflows
- Set up parallel review or research pipelines

## Subagent Types

Tau supports 5 built-in subagent types:

| Type | Purpose | Default Tools |
|------|---------|---------------|
| Researcher | Information gathering, codebase exploration | read, grep, find, ls, bash (read-only) |
| Reviewer | Code/content review | read, grep, find, ls |
| Implementor | Feature implementation | read, write, edit, bash, grep, find, ls |
| SecurityReviewer | Security analysis | read, grep, find, bash (static analysis) |
| QA | Testing and quality assurance | read, bash, grep, find, ls, write |

## Context Modes

| Mode | Behavior | Use Case |
|------|----------|----------|
| `fresh` | Empty message history, inherits system prompt and tools | Research, isolated review, independent tasks |
| `fork` | Cloned message slice from parent (shallow copy) | Implementation, advisory review with full context |

### Choosing Context Mode

- Use **fork** when the subagent needs to understand the conversation history and accumulated context
- Use **fresh** when the subagent should reason independently or when history would be distracting

## Subagent Structure

A subagent definition consists of:

```go
type SubAgent struct {
    ID           string            // Unique identifier
    Type         string            // "researcher" | "reviewer" | "implementor" | ...
    Task         string            // What the subagent should do
    ContextMode  string            // "fresh" | "fork"
    Tools        []tools.Tool      // Scoped tool set
    Model        provider.Model    // Inherited or overridden
    SystemPrompt string            // Role-specific instructions
    Events       chan AgentEvent   // Optional streaming visibility
    Result       chan SubAgentResult
    cancel       context.CancelFunc
}
```

## Result Handling

```go
type SubAgentResult struct {
    Success   bool
    Output    string
    Artifacts []string      // File paths created/modified
    Error     error
    Duration  time.Duration
    Usage     types.Usage
}
```

### LLM Visibility

- **Default**: Result is LLM-visible — parent agent can act on results
- **Opt-out**: Use `custom_entry` for non-LLM-visible results (logging/metadata only)

## Execution Model

- Subagents run **synchronously** — parent waits with configurable timeout (default 5 minutes)
- **Timeout**: Cancelled via `context.CancelFunc` if exceeded
- **Error isolation**: Failures returned as `SubAgentResult{Success: false}` — parent continues

## Best Practices

### Task Design

1. **Be specific**: "Review auth.go for SQL injection vulnerabilities" not "review the code"
2. **Set boundaries**: Define what NOT to do (e.g., "do not edit files")
3. **Include success criteria**: What does "done" look like?
4. **Provide context paths**: Point to relevant files or sections

### Tool Scoping

- **Principle of least privilege**: Only give tools the subagent needs
- **Researchers** should not have write access
- **Reviewers** should not have bash access for mutations
- **Implementors** need full tool access

### Parallel Patterns

For independent subagent tasks, spawn multiple subagents concurrently:

```
Researcher: "Find all auth endpoints"        (fresh context)
Researcher: "Check dependency vulnerabilities" (fresh context)
```

Both run in parallel — results are combined when both complete.

### Workflow Patterns

**Research → Plan → Implement**:
1. Spawn researcher (fresh context) to gather information
2. Analyze research results
3. Spawn implementor (fork context) to execute

**Review Cycle**:
1. Spawn reviewer (fresh context) for objective analysis
2. Review findings
3. Spawn implementor (fork context) to fix issues

## Creating a New Subagent Type

1. Define the subagent struct with appropriate defaults
2. Set the default tool set based on the role
3. Write a system prompt that defines the role clearly
4. Choose the default context mode (fresh for independent, fork for context-aware)
5. Document the subagent's purpose, inputs, and expected outputs

## Security Considerations

- Subagents inherit the parent's security context
- Tool allowlists apply per-subagent
- Read-only mode disables mutation tools for all subagents
- Subagent filesystem access is scoped to the parent's working directory

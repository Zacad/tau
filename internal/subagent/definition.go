package subagent

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentDefinition represents a user-defined agent parsed from a Markdown file
// with YAML frontmatter.
type AgentDefinition struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Tools        []string `json:"tools,omitempty"`
	Model        string   `json:"model,omitempty"`
	SystemPrompt string   `json:"system_prompt"`
	Content      string   `json:"-"` // Markdown content after frontmatter
	Source       string   `json:"source"`
	Path         string   `json:"path"`
}

// Validate checks that the AgentDefinition has required fields.
func (d *AgentDefinition) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("agent definition missing required field: name")
	}
	if d.Description == "" {
		return fmt.Errorf("agent definition missing required field: description")
	}
	if d.SystemPrompt == "" {
		return fmt.Errorf("agent definition missing required field: system_prompt")
	}
	return nil
}

// agentFrontmatter represents the YAML frontmatter in an agent Markdown file.
type agentFrontmatter struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Tools        []string `yaml:"tools,omitempty"`
	Model        string   `yaml:"model,omitempty"`
	SystemPrompt string   `yaml:"system_prompt"`
}

// ParseAgentMD parses an agent Markdown file from the given data.
// The source parameter indicates where the file came from (e.g., "user", "project").
// The path parameter is the full file path for error reporting.
func ParseAgentMD(data []byte, source, path string) (*AgentDefinition, error) {
	fm, content, err := parseAgentFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	def := &AgentDefinition{
		Name:         fm.Name,
		Description:  fm.Description,
		Tools:        fm.Tools,
		Model:        fm.Model,
		SystemPrompt: fm.SystemPrompt,
		Content:      content,
		Source:       source,
		Path:         path,
	}

	if err := def.Validate(); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return def, nil
}

// parseAgentFrontmatter extracts YAML frontmatter and remaining content from data.
// Frontmatter is delimited by --- markers at the start of the file.
func parseAgentFrontmatter(data []byte) (*agentFrontmatter, string, error) {
	trimmed := bytes.TrimSpace(data)
	if !bytes.HasPrefix(trimmed, []byte("---")) {
		return nil, "", fmt.Errorf("agent file must start with YAML frontmatter (---)")
	}

	// Find the closing --- marker after the opening
	rest := trimmed[3:] // skip opening ---
	idx := bytes.Index(rest, []byte("---"))
	if idx < 0 {
		return nil, "", fmt.Errorf("agent file frontmatter not closed (missing ---)")
	}

	yamlBytes := rest[:idx]
	contentBytes := rest[idx+3:]

	// Trim leading whitespace from content (the newline after closing ---)
	content := strings.TrimLeft(string(contentBytes), "\n\r")

	var fm agentFrontmatter
	if err := yaml.Unmarshal(yamlBytes, &fm); err != nil {
		return nil, "", fmt.Errorf("parsing frontmatter YAML: %w", err)
	}

	return &fm, content, nil
}

// DiscoverAgents finds all user-defined agents across two discovery tiers:
// user (~/.tau/agents/) and project (.tau/agents/ walking up from cwd).
//
// When the same agent name exists in multiple tiers, project overrides user.
// Built-in agents are NOT included — caller should merge with built-in agents.
//
// Invalid agents are silently skipped with a warning logged.
func DiscoverAgents(cwd string) map[string]*AgentDefinition {
	agents := make(map[string]*AgentDefinition)

	// Tier 1: User (lower priority, loaded first)
	loadUserAgents(agents)

	// Tier 2: Project (higher priority, loaded last so it overrides)
	loadProjectAgents(cwd, agents)

	return agents
}

// loadUserAgents loads agents from ~/.tau/agents/.
func loadUserAgents(agents map[string]*AgentDefinition) {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("unable to determine home directory, skipping user agents", "error", err)
		return
	}

	userDir := filepath.Join(home, ".tau", "agents")
	loadAgentDirectory(userDir, "user", agents)
}

// loadProjectAgents walks up from cwd looking for .tau/agents/ directories.
// Directories are loaded from root to cwd so that closer agents override
// parent ones (project agents closest to cwd have highest priority).
func loadProjectAgents(cwd string, agents map[string]*AgentDefinition) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		slog.Warn("unable to resolve cwd for project agent discovery", "error", err)
		return
	}

	// Collect all candidate directories from cwd to root
	var dirs []string
	for {
		dirs = append(dirs, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Load from root to cwd (reverse order) so closest wins
	for i := len(dirs) - 1; i >= 0; i-- {
		agentsDir := filepath.Join(dirs[i], ".tau", "agents")
		loadAgentDirectory(agentsDir, "project", agents)
	}
}

// loadAgentDirectory scans a directory for agent Markdown files (*.md).
func loadAgentDirectory(dir, source string, agents map[string]*AgentDefinition) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // directory doesn't exist or isn't readable — silently skip
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}

		fullPath := filepath.Join(dir, name)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			slog.Warn("failed to read agent file", "path", fullPath, "error", err)
			continue
		}

		def, err := ParseAgentMD(data, source, fullPath)
		if err != nil {
			slog.Warn("skipping invalid agent file", "path", fullPath, "error", err)
			continue
		}

		// Validate tool names against known tool names
		if err := validateAgentTools(def.Tools); err != nil {
			slog.Warn("skipping agent with invalid tools", "agent", def.Name, "path", fullPath, "error", err)
			continue
		}

		agents[def.Name] = def
	}
}

// validateAgentTools checks that all tool names in the agent definition are known.
func validateAgentTools(tools []string) error {
	knownTools := map[string]bool{
		"read":      true,
		"write":     true,
		"edit":      true,
		"bash":      true,
		"grep":      true,
		"find":      true,
		"ls":        true,
		"websearch": true,
		"webfetch":  true,
		"subagent":  true,
	}

	for _, tool := range tools {
		if !knownTools[tool] {
			return fmt.Errorf("unknown tool %q", tool)
		}
	}

	return nil
}

// AllAgents returns all available agents: built-in types merged with
// discovered user/project agents. Precedence: project > user > built-in.
func AllAgents(cwd string) map[string]*AgentDefinition {
	result := make(map[string]*AgentDefinition)

	// Start with built-in agents
	for _, t := range AllTypes() {
		tools := DefaultToolSet(t)
		if tools == nil {
			tools = []string{}
		}
		result[string(t)] = &AgentDefinition{
			Name:         string(t),
			Description:  "Built-in agent type: " + string(t),
			Tools:        tools,
			SystemPrompt: DefaultSystemPrompt(t),
			Source:       "builtin",
		}
	}

	// Overlay discovered agents (project > user)
	discovered := DiscoverAgents(cwd)
	for name, def := range discovered {
		result[name] = def
	}

	return result
}

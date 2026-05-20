package subagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAgentMD_ValidFile(t *testing.T) {
	data := []byte(`---
name: test-agent
description: A test agent for verification
tools: [read, grep]
model: gpt-4
system_prompt: You are a test agent.
---

This is the markdown content after the frontmatter.
`)

	def, err := ParseAgentMD(data, "user", "/test/test-agent.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if def.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", def.Name)
	}
	if def.Description != "A test agent for verification" {
		t.Errorf("expected description, got %q", def.Description)
	}
	if len(def.Tools) != 2 || def.Tools[0] != "read" || def.Tools[1] != "grep" {
		t.Errorf("expected tools [read, grep], got %v", def.Tools)
	}
	if def.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", def.Model)
	}
	if def.SystemPrompt != "You are a test agent." {
		t.Errorf("expected system prompt, got %q", def.SystemPrompt)
	}
	if def.Content != "This is the markdown content after the frontmatter." {
		t.Errorf("expected content, got %q", def.Content)
	}
	if def.Source != "user" {
		t.Errorf("expected source 'user', got %q", def.Source)
	}
	if def.Path != "/test/test-agent.md" {
		t.Errorf("expected path, got %q", def.Path)
	}
}

func TestParseAgentMD_MinimalFile(t *testing.T) {
	data := []byte(`---
name: minimal
description: Minimal agent
system_prompt: Hello
---
`)

	def, err := ParseAgentMD(data, "project", "/test/minimal.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if def.Name != "minimal" {
		t.Errorf("expected name 'minimal', got %q", def.Name)
	}
	if len(def.Tools) != 0 {
		t.Errorf("expected empty tools, got %v", def.Tools)
	}
	if def.Model != "" {
		t.Errorf("expected empty model, got %q", def.Model)
	}
}

func TestParseAgentMD_NoFrontmatter(t *testing.T) {
	data := []byte(`This is just markdown content without frontmatter.`)

	_, err := ParseAgentMD(data, "user", "/test/no-fm.md")
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseAgentMD_UnclosedFrontmatter(t *testing.T) {
	data := []byte(`---
name: test
description: Test
system_prompt: Hello`)

	_, err := ParseAgentMD(data, "user", "/test/unclosed.md")
	if err == nil {
		t.Fatal("expected error for unclosed frontmatter")
	}
}

func TestParseAgentMD_InvalidYAML(t *testing.T) {
	data := []byte(`---
name: test
description: [invalid yaml
system_prompt: Hello
---
`)

	_, err := ParseAgentMD(data, "user", "/test/invalid-yaml.md")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestParseAgentMD_MissingName(t *testing.T) {
	data := []byte(`---
description: No name here
system_prompt: Hello
---
`)

	_, err := ParseAgentMD(data, "user", "/test/no-name.md")
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseAgentMD_MissingDescription(t *testing.T) {
	data := []byte(`---
name: no-desc
system_prompt: Hello
---
`)

	_, err := ParseAgentMD(data, "user", "/test/no-desc.md")
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}

func TestParseAgentMD_MissingSystemPrompt(t *testing.T) {
	data := []byte(`---
name: no-prompt
description: No system prompt
---
`)

	_, err := ParseAgentMD(data, "user", "/test/no-prompt.md")
	if err == nil {
		t.Fatal("expected error for missing system_prompt")
	}
}

func TestValidateAgentTools_ValidTools(t *testing.T) {
	tools := []string{"read", "write", "bash", "grep", "find", "ls", "edit", "websearch", "webfetch", "subagent"}
	err := validateAgentTools(tools)
	if err != nil {
		t.Errorf("unexpected error for valid tools: %v", err)
	}
}

func TestValidateAgentTools_UnknownTool(t *testing.T) {
	tools := []string{"read", "unknown_tool"}
	err := validateAgentTools(tools)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestValidateAgentTools_EmptyTools(t *testing.T) {
	err := validateAgentTools(nil)
	if err != nil {
		t.Errorf("unexpected error for empty tools: %v", err)
	}

	err = validateAgentTools([]string{})
	if err != nil {
		t.Errorf("unexpected error for empty slice: %v", err)
	}
}

func TestLoadAgentDirectory_NonExistent(t *testing.T) {
	agents := make(map[string]*AgentDefinition)
	loadAgentDirectory("/nonexistent/path/that/does/not/exist", "user", agents)
	if len(agents) != 0 {
		t.Errorf("expected no agents from non-existent directory, got %d", len(agents))
	}
}

func TestLoadAgentDirectory_WithValidAgent(t *testing.T) {
	dir := t.TempDir()
	agentFile := filepath.Join(dir, "my-agent.md")
	content := `---
name: my-agent
description: My test agent
tools: [read, grep]
system_prompt: You are my agent.
---

Some content.
`
	if err := os.WriteFile(agentFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	agents := make(map[string]*AgentDefinition)
	loadAgentDirectory(dir, "user", agents)

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	def, ok := agents["my-agent"]
	if !ok {
		t.Fatal("expected 'my-agent' to be loaded")
	}
	if def.Source != "user" {
		t.Errorf("expected source 'user', got %q", def.Source)
	}
}

func TestLoadAgentDirectory_SkipsNonMarkdown(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "agent.txt"), []byte(`---
name: txt-agent
description: Should be skipped
system_prompt: Hello
---
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "agent.md"), []byte(`---
name: md-agent
description: Should be loaded
system_prompt: Hello
---
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	agents := make(map[string]*AgentDefinition)
	loadAgentDirectory(dir, "user", agents)

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if _, ok := agents["md-agent"]; !ok {
		t.Fatal("expected 'md-agent' to be loaded")
	}
}

func TestLoadAgentDirectory_SkipsInvalidAgent(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "bad.md"), []byte(`---
name: bad
description: [unclosed
system_prompt: Hello
---
`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	agents := make(map[string]*AgentDefinition)
	loadAgentDirectory(dir, "user", agents)

	if len(agents) != 0 {
		t.Errorf("expected 0 agents (invalid YAML should be skipped), got %d", len(agents))
	}
}

func TestDiscoverAgents_UserDirectory(t *testing.T) {
	home := t.TempDir()
	agentsDir := filepath.Join(home, ".tau", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(agentsDir, "user-agent.md"), []byte(`---
name: user-agent
description: User-level agent
system_prompt: User prompt
---
`), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	agents := DiscoverAgents(home)

	if _, ok := agents["user-agent"]; !ok {
		t.Fatal("expected 'user-agent' to be discovered")
	}
}

func TestDiscoverAgents_ProjectOverridesUser(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "project")
	userAgentsDir := filepath.Join(home, ".tau", "agents")
	projectAgentsDir := filepath.Join(projectDir, ".tau", "agents")

	if err := os.MkdirAll(userAgentsDir, 0755); err != nil {
		t.Fatalf("failed to create user agents dir: %v", err)
	}
	if err := os.MkdirAll(projectAgentsDir, 0755); err != nil {
		t.Fatalf("failed to create project agents dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(userAgentsDir, "shared.md"), []byte(`---
name: shared
description: User version
system_prompt: User system prompt
---
`), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(projectAgentsDir, "shared.md"), []byte(`---
name: shared
description: Project version
system_prompt: Project system prompt
---
`), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	agents := DiscoverAgents(projectDir)

	def, ok := agents["shared"]
	if !ok {
		t.Fatal("expected 'shared' agent to exist")
	}
	if def.Description != "Project version" {
		t.Errorf("expected project version to override user, got description: %q", def.Description)
	}
	if def.Source != "project" {
		t.Errorf("expected source 'project', got %q", def.Source)
	}
}

func TestDiscoverAgents_EmptyDirectories(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	agents := DiscoverAgents(projectDir)

	if len(agents) != 0 {
		t.Errorf("expected 0 agents from empty dirs, got %d", len(agents))
	}
}

func TestDiscoverAgents_ProjectWalkUp(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "project")
	subDir := filepath.Join(projectDir, "src", "pkg")
	agentsDir := filepath.Join(projectDir, ".tau", "agents")

	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(agentsDir, "walker.md"), []byte(`---
name: walker
description: Found by walking up
system_prompt: Walker prompt
---
`), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	agents := DiscoverAgents(subDir)

	if _, ok := agents["walker"]; !ok {
		t.Fatal("expected 'walker' agent to be found by walking up from subdir")
	}
}

func TestAgentDefinition_Validate(t *testing.T) {
	tests := []struct {
		name    string
		def     AgentDefinition
		wantErr bool
	}{
		{
			name:    "valid definition",
			def:     AgentDefinition{Name: "a", Description: "b", SystemPrompt: "c"},
			wantErr: false,
		},
		{
			name:    "missing name",
			def:     AgentDefinition{Description: "b", SystemPrompt: "c"},
			wantErr: true,
		},
		{
			name:    "missing description",
			def:     AgentDefinition{Name: "a", SystemPrompt: "c"},
			wantErr: true,
		},
		{
			name:    "missing system_prompt",
			def:     AgentDefinition{Name: "a", Description: "b"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAllAgents_IncludesBuiltins(t *testing.T) {
	home := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	agents := AllAgents(home)

	// All 6 built-in types should be present
	for _, typ := range AllTypes() {
		if _, ok := agents[string(typ)]; !ok {
			t.Errorf("expected built-in agent %q to be present", typ)
		}
	}
}

func TestAllAgents_UserOverridesBuiltin(t *testing.T) {
	// Use separate temp dirs so user agents dir is NOT in project walk-up path
	home := t.TempDir()
	projectDir := t.TempDir()
	agentsDir := filepath.Join(home, ".tau", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatalf("failed to create agents dir: %v", err)
	}

	// Create user agent with same name as built-in
	if err := os.WriteFile(filepath.Join(agentsDir, "researcher.md"), []byte(`---
name: researcher
description: Custom researcher
system_prompt: Custom researcher prompt
---
`), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	agents := AllAgents(projectDir)

	def, ok := agents["researcher"]
	if !ok {
		t.Fatal("expected 'researcher' agent to exist")
	}
	if def.Source != "user" {
		t.Errorf("expected user agent to override builtin, got source: %q", def.Source)
	}
	if def.Description != "Custom researcher" {
		t.Errorf("expected custom description, got: %q", def.Description)
	}
}

func TestAllAgents_ProjectOverridesUser(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "project")
	userAgentsDir := filepath.Join(home, ".tau", "agents")
	projectAgentsDir := filepath.Join(projectDir, ".tau", "agents")

	if err := os.MkdirAll(userAgentsDir, 0755); err != nil {
		t.Fatalf("failed to create user agents dir: %v", err)
	}
	if err := os.MkdirAll(projectAgentsDir, 0755); err != nil {
		t.Fatalf("failed to create project agents dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(userAgentsDir, "custom.md"), []byte(`---
name: custom
description: User version
system_prompt: User prompt
---
`), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(projectAgentsDir, "custom.md"), []byte(`---
name: custom
description: Project version
system_prompt: Project prompt
---
`), 0644); err != nil {
		t.Fatalf("failed to write agent file: %v", err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	agents := AllAgents(projectDir)

	def, ok := agents["custom"]
	if !ok {
		t.Fatal("expected 'custom' agent to exist")
	}
	if def.Source != "project" {
		t.Errorf("expected project to override user, got source: %q", def.Source)
	}
	if def.Description != "Project version" {
		t.Errorf("expected project description, got: %q", def.Description)
	}
}

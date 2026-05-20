package customcmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFrontmatter_Valid(t *testing.T) {
	content := `---
name: test
description: Run tests
model: openai/gpt-4o
agent: build
---
Run the tests with $ARGUMENTS`

	fm, template, err := parseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "test" {
		t.Errorf("name = %q, want %q", fm.Name, "test")
	}
	if fm.Description != "Run tests" {
		t.Errorf("description = %q, want %q", fm.Description, "Run tests")
	}
	if fm.Model != "openai/gpt-4o" {
		t.Errorf("model = %q, want %q", fm.Model, "openai/gpt-4o")
	}
	if fm.Agent != "build" {
		t.Errorf("agent = %q, want %q", fm.Agent, "build")
	}
	if template != "Run the tests with $ARGUMENTS" {
		t.Errorf("template = %q, want %q", template, "Run the tests with $ARGUMENTS")
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "Just plain text, no frontmatter"

	fm, template, err := parseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "" {
		t.Errorf("name should be empty, got %q", fm.Name)
	}
	if template != "Just plain text, no frontmatter" {
		t.Errorf("template = %q, want %q", template, "Just plain text, no frontmatter")
	}
}

func TestParseFrontmatter_MissingClosingDelimiters(t *testing.T) {
	content := "---\nname: test\ndescription: Run tests\nRun the tests"

	fm, template, err := parseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "" {
		t.Errorf("name should be empty without closing ---, got %q", fm.Name)
	}
	if template != content {
		t.Errorf("template should be entire content, got %q", template)
	}
}

func TestParseFrontmatter_InvalidYAML(t *testing.T) {
	content := "---\nname: [invalid\n---\nTemplate here"

	_, _, err := parseFrontmatter(content)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestParseFrontmatter_EmptyYAML(t *testing.T) {
	content := "---\n---\nTemplate content"

	fm, template, err := parseFrontmatter(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm.Name != "" {
		t.Errorf("name should be empty, got %q", fm.Name)
	}
	if template != "Template content" {
		t.Errorf("template = %q, want %q", template, "Template content")
	}
}

func TestParseFile_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	err := os.WriteFile(path, []byte(`---
name: mytest
description: My test command
---
Run tests: $ARGUMENTS`), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd, err := parseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "mytest" {
		t.Errorf("name = %q, want %q", cmd.Name, "mytest")
	}
	if cmd.Description != "My test command" {
		t.Errorf("description = %q, want %q", cmd.Description, "My test command")
	}
	if cmd.Template != "Run tests: $ARGUMENTS" {
		t.Errorf("template = %q, want %q", cmd.Template, "Run tests: $ARGUMENTS")
	}
	if cmd.Source != path {
		t.Errorf("source = %q, want %q", cmd.Source, path)
	}
}

func TestParseFile_NameFallbackToFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mycommand.md")
	err := os.WriteFile(path, []byte(`---
description: A command without name
---
Do something`), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd, err := parseFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.Name != "mycommand" {
		t.Errorf("name = %q, want %q", cmd.Name, "mycommand")
	}
}

func TestParseFile_NonExistent(t *testing.T) {
	_, err := parseFile("/nonexistent/path/file.md")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestParseDirectory(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, "test.md"), []byte(`---
name: test
description: Run tests
---
Run tests`), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err = os.WriteFile(filepath.Join(dir, "deploy.md"), []byte(`---
description: Deploy app
---
Deploy to production`), 0644)
	if err != nil {
		t.Fatalf("failed to write deploy file: %v", err)
	}

	// Malformed file
	err = os.WriteFile(filepath.Join(dir, "bad.md"), []byte("---\ninvalid: [\n---\nTemplate"), 0644)
	if err != nil {
		t.Fatalf("failed to write bad file: %v", err)
	}

	// Non-markdown file (should be ignored)
	err = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a command"), 0644)
	if err != nil {
		t.Fatalf("failed to write txt file: %v", err)
	}

	cmds, err := parseDirectory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}

	names := make(map[string]bool)
	for _, cmd := range cmds {
		names[cmd.Name] = true
	}
	if !names["test"] {
		t.Error("expected 'test' command")
	}
	if !names["deploy"] {
		t.Error("expected 'deploy' command")
	}
}

func TestParseDirectory_NonExistent(t *testing.T) {
	_, err := parseDirectory("/nonexistent/dir")
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
}

func TestDiscoverCommands_EmbeddedOnly(t *testing.T) {
	embedded := []CustomCommand{
		{Name: "embedded1", Description: "Embedded cmd 1", Template: "template1"},
		{Name: "embedded2", Description: "Embedded cmd 2", Template: "template2"},
	}

	cmds, err := DiscoverCommands("", embedded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}
}

func TestDiscoverCommands_ProjectOverridesGlobal(t *testing.T) {
	// Create project commands dir
	projectDir := t.TempDir()
	commandsDir := filepath.Join(projectDir, ".tau", "commands")
	err := os.MkdirAll(commandsDir, 0755)
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(commandsDir, "test.md"), []byte(`---
name: test
description: Project test
---
Project template`), 0644)
	if err != nil {
		t.Fatalf("failed to write project file: %v", err)
	}

	// Create global commands dir
	globalDir := filepath.Join(t.TempDir(), ".tau", "commands")
	err = os.MkdirAll(globalDir, 0755)
	if err != nil {
		t.Fatalf("failed to create global dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(globalDir, "test.md"), []byte(`---
name: test
description: Global test
---
Global template`), 0644)
	if err != nil {
		t.Fatalf("failed to write global file: %v", err)
	}

	err = os.WriteFile(filepath.Join(globalDir, "deploy.md"), []byte(`---
name: deploy
description: Global deploy
---
Deploy template`), 0644)
	if err != nil {
		t.Fatalf("failed to write deploy file: %v", err)
	}

	// Use a wrapper to override home dir
	t.Setenv("HOME", filepath.Dir(filepath.Dir(globalDir)))

	cmds, err := DiscoverCommands(projectDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}

	// Project "test" should override global "test"
	found := false
	for _, cmd := range cmds {
		if cmd.Name == "test" {
			if cmd.Description == "Project test" {
				found = true
			} else {
				t.Errorf("project 'test' should have description 'Project test', got %q", cmd.Description)
			}
		}
	}
	if !found {
		t.Error("expected project 'test' to override global 'test'")
	}

	// Global "deploy" should be present
	foundDeploy := false
	for _, cmd := range cmds {
		if cmd.Name == "deploy" && cmd.Description == "Global deploy" {
			foundDeploy = true
		}
	}
	if !foundDeploy {
		t.Error("expected global 'deploy' to be present")
	}
}

func TestDiscoverCommands_EmbeddedOverriddenByProject(t *testing.T) {
	projectDir := t.TempDir()
	commandsDir := filepath.Join(projectDir, ".tau", "commands")
	err := os.MkdirAll(commandsDir, 0755)
	if err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(commandsDir, "test.md"), []byte(`---
name: test
description: Project test
---
Project template`), 0644)
	if err != nil {
		t.Fatalf("failed to write project file: %v", err)
	}

	embedded := []CustomCommand{
		{Name: "test", Description: "Embedded test", Template: "Embedded template"},
		{Name: "review", Description: "Embedded review", Template: "Review template"},
	}

	cmds, err := DiscoverCommands(projectDir, embedded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(cmds))
	}

	// Project "test" should override embedded "test"
	found := false
	for _, cmd := range cmds {
		if cmd.Name == "test" && cmd.Description == "Project test" {
			found = true
		}
	}
	if !found {
		t.Error("expected project 'test' to override embedded 'test'")
	}
}

func TestDiscoverCommands_NoDirectories(t *testing.T) {
	cmds, err := DiscoverCommands("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmds) != 0 {
		t.Fatalf("expected 0 commands, got %d", len(cmds))
	}
}

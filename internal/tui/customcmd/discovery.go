package customcmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CustomCommand represents a user-defined slash command loaded from a markdown file.
type CustomCommand struct {
	Name        string
	Description string
	Template    string
	Model       string
	Agent       string
	Source      string
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Model       string `yaml:"model"`
	Agent       string `yaml:"agent"`
}

// DiscoverCommands scans for custom command files in project and global directories,
// merging them with embedded commands. Priority order: project > global > embedded
// (higher priority overrides same-name commands from lower priority sources).
func DiscoverCommands(cwd string, embedded []CustomCommand) ([]CustomCommand, error) {
	// Collect directories in priority order (lowest first): global, then project
	var dirs []string

	// Global directory (lower priority)
	home, err := os.UserHomeDir()
	if err == nil {
		globalDir := filepath.Join(home, ".tau", "commands")
		if _, err := os.Stat(globalDir); err == nil {
			dirs = append(dirs, globalDir)
		}
	}

	// Project-level directory (higher priority, parsed last to override)
	if cwd != "" {
		projectDir := filepath.Join(cwd, ".tau", "commands")
		if _, err := os.Stat(projectDir); err == nil {
			dirs = append(dirs, projectDir)
		}
	}

	// Collect all commands in priority order (lowest first): embedded, global, project
	var all []CustomCommand
	all = append(all, embedded...)

	for _, dir := range dirs {
		cmds, err := parseDirectory(dir)
		if err != nil {
			slog.Warn("failed to read custom commands directory", "dir", dir, "error", err)
			continue
		}
		all = append(all, cmds...)
	}

	// Deduplicate: keep last occurrence (highest priority wins)
	seen := make(map[string]int)
	for i, cmd := range all {
		seen[cmd.Name] = i
	}

	result := make([]CustomCommand, 0, len(seen))
	for i, cmd := range all {
		if seen[cmd.Name] == i {
			result = append(result, cmd)
		}
	}

	return result, nil
}

func parseDirectory(dir string) ([]CustomCommand, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var cmds []CustomCommand
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		cmd, err := parseFile(path)
		if err != nil {
			slog.Warn("skipping malformed custom command file", "path", path, "error", err)
			continue
		}
		if cmd.Name == "" {
			continue
		}
		cmds = append(cmds, cmd)
	}

	return cmds, nil
}

func parseFile(path string) (CustomCommand, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CustomCommand{}, err
	}

	fm, template, err := parseFrontmatter(string(data))
	if err != nil {
		return CustomCommand{}, err
	}

	name := fm.Name
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), ".md")
	}

	return CustomCommand{
		Name:        name,
		Description: fm.Description,
		Template:    template,
		Model:       fm.Model,
		Agent:       fm.Agent,
		Source:      path,
	}, nil
}

func parseFrontmatter(content string) (frontmatter, string, error) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return frontmatter{}, trimmed, nil
	}

	endIdx := strings.Index(trimmed[3:], "---")
	if endIdx < 0 {
		return frontmatter{}, trimmed, nil
	}

	yamlBlock := trimmed[3 : 3+endIdx]
	template := strings.TrimSpace(trimmed[3+endIdx+3:])

	var fm frontmatter
	if err := yaml.Unmarshal([]byte(yamlBlock), &fm); err != nil {
		return frontmatter{}, "", err
	}

	return fm, template, nil
}

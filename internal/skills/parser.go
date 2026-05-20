package skills

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// skillFrontmatter represents the YAML frontmatter in a SKILL.md file.
type skillFrontmatter struct {
	Name                   string   `yaml:"name"`
	Description            string   `yaml:"description"`
	DisableModelInvocation bool     `yaml:"disable_model_invocation"`
	Scripts                []string `yaml:"scripts,omitempty"`
	References             []string `yaml:"references,omitempty"`
	Assets                 []string `yaml:"assets,omitempty"`
}

// ParseSkillMD parses a SKILL.md file from the given reader.
// The dirName parameter is the name of the directory containing the SKILL.md
// file, which must match the skill name in the frontmatter.
func ParseSkillMD(r io.Reader, dirName string) (*Skill, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading SKILL.md: %w", err)
	}

	fm, content, err := parseFrontmatter(data)
	if err != nil {
		return nil, err
	}

	skill := &Skill{
		Name:                   fm.Name,
		Description:            fm.Description,
		DisableModelInvocation: fm.DisableModelInvocation,
		Scripts:                fm.Scripts,
		References:             fm.References,
		Assets:                 fm.Assets,
		Content:                content,
	}

	if err := skill.Validate(); err != nil {
		return nil, fmt.Errorf("validating skill %q: %w", dirName, err)
	}

	if skill.Name != dirName {
		return nil, fmt.Errorf("skill name %q does not match directory name %q",
			skill.Name, dirName)
	}

	return skill, nil
}

// parseFrontmatter extracts YAML frontmatter and remaining content from data.
// Frontmatter is delimited by --- markers at the start of the file.
func parseFrontmatter(data []byte) (*skillFrontmatter, string, error) {
	trimmed := bytes.TrimSpace(data)
	if !bytes.HasPrefix(trimmed, []byte("---")) {
		return nil, "", fmt.Errorf("SKILL.md must start with YAML frontmatter (---)")
	}

	// Find the closing --- marker after the opening
	rest := trimmed[3:] // skip opening ---
	idx := bytes.Index(rest, []byte("---"))
	if idx < 0 {
		return nil, "", fmt.Errorf("SKILL.md frontmatter not closed (missing ---)")
	}

	yamlBytes := rest[:idx]
	contentBytes := rest[idx+3:]

	// Trim leading whitespace from content (the newline after closing ---)
	content := strings.TrimLeft(string(contentBytes), "\n\r")

	var fm skillFrontmatter
	if err := yaml.Unmarshal(yamlBytes, &fm); err != nil {
		return nil, "", fmt.Errorf("parsing frontmatter YAML: %w", err)
	}

	if fm.Name == "" {
		return nil, "", fmt.Errorf("frontmatter missing required field: name")
	}
	if fm.Description == "" {
		return nil, "", fmt.Errorf("frontmatter missing required field: description")
	}

	return &fm, content, nil
}

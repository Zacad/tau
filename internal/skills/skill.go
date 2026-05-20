// Package skills implements skill discovery, parsing, and progressive disclosure
// for the Tau agentic coding tool.
//
// Skills define capabilities that the agent can discover and use. The system
// supports 3-tier discovery (built-in, global, project), SKILL.md parsing with
// YAML frontmatter, and progressive disclosure for system prompts.
//
// Dependencies: only stdlib (plus gopkg.in/yaml.v3 for frontmatter parsing).
package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
)

// Source constants for skill origin.
const (
	SourceBuiltin = "builtin"
	SourceGlobal  = "global"
	SourceProject = "project"
)

// Maximum lengths per the Agent Skills standard.
const (
	MaxNameLength        = 64
	MaxDescriptionLength = 1024
)

// Sentinel errors for skill validation.
var (
	errEmptyName          = errors.New("skill name cannot be empty")
	errNameTooLong        = fmt.Errorf("skill name exceeds %d characters", MaxNameLength)
	errInvalidNameChars   = errors.New("skill name must contain only lowercase letters, digits, and hyphens, and must start with a letter or digit")
	errEmptyDescription   = errors.New("skill description is required")
	errDescriptionTooLong = fmt.Errorf("skill description exceeds %d characters", MaxDescriptionLength)
)

// Valid skill name: lowercase letters, digits, hyphens; must start with letter or digit.
var validSkillNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Skill represents a discovered skill with its metadata and content.
type Skill struct {
	Name                   string   // Matches directory name; lowercase, hyphens, 0-9, max 64 chars
	Description            string   // Max 1024 chars; required
	DisableModelInvocation bool     // If true, excluded from progressive disclosure
	Content                string   // Full SKILL.md markdown body (after frontmatter)
	Scripts                []string // Available script paths (relative to skill root)
	References             []string // Reference file paths (relative to skill root)
	Assets                 []string // Asset file paths (relative to skill root)
	Source                 string   // "builtin" | "global" | "project"

	// Internal fields — not part of the public API.
	dir  string // filesystem root directory (empty for builtin)
	fsys fs.FS  // filesystem for reading skill files (os.DirFS or embed.FS)
}

// ValidateSkillName checks that a skill name conforms to the Agent Skills standard.
// Rules: non-empty, lowercase letters/digits/hyphens only, starts with letter or digit,
// maximum 64 characters.
func ValidateSkillName(name string) error {
	if name == "" {
		return errEmptyName
	}
	if len(name) > MaxNameLength {
		return errNameTooLong
	}
	if !validSkillNameRe.MatchString(name) {
		return errInvalidNameChars
	}
	return nil
}

// ValidateDescription checks that a skill description is non-empty and within
// the maximum length limit.
func ValidateDescription(desc string) error {
	if desc == "" {
		return errEmptyDescription
	}
	if len(desc) > MaxDescriptionLength {
		return errDescriptionTooLong
	}
	return nil
}

// Validate checks that the Skill has a valid name and description.
func (s *Skill) Validate() error {
	if err := ValidateSkillName(s.Name); err != nil {
		return err
	}
	if err := ValidateDescription(s.Description); err != nil {
		return err
	}
	return nil
}

// IsValidSource returns true if the skill's Source field is one of the
// recognized source constants.
func (s *Skill) IsValidSource() bool {
	switch s.Source {
	case SourceBuiltin, SourceGlobal, SourceProject:
		return true
	default:
		return false
	}
}

// ResolvePath resolves a relative path against the skill's source directory.
// Returns empty string for built-in skills (use ReadFile instead).
func (s *Skill) ResolvePath(rel string) string {
	if s.dir == "" {
		return ""
	}
	// Use the filesystem's PathJoin-equivalent; fs.ReadFile handles
	// path cleaning internally, but for display purposes we return
	// the joined OS path.
	return s.dir + "/" + rel
}

// ReadFile reads a file from the skill's filesystem (embedded or OS).
// The path is relative to the skill root.
func (s *Skill) ReadFile(rel string) ([]byte, error) {
	if s.fsys == nil {
		return nil, fmt.Errorf("skill %q has no filesystem", s.Name)
	}
	return fs.ReadFile(s.fsys, rel)
}

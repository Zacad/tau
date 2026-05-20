package skills

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// DiscoverSkills finds all skills across the three discovery tiers:
// built-in (embedded), global (~/.tau/skills/, ~/.agents/skills/),
// and project (.agents/skills/ walking up from cwd).
//
// When the same skill name exists in multiple tiers, project overrides global,
// and global overrides built-in.
//
// Invalid skills are silently skipped with a warning logged.
func DiscoverSkills(cwd string) []*Skill {
	return DiscoverSkillsWithFS(cwd, BuiltinFS())
}

// DiscoverSkillsWithFS is like DiscoverSkills but accepts an explicit
// embedded filesystem for built-in skills (useful for testing).
func DiscoverSkillsWithFS(cwd string, builtinFS fs.FS) []*Skill {
	skills := make(map[string]*Skill) // name → skill (later tiers override)

	// Tier 1: Built-in (lowest priority, first loaded)
	loadBuiltinSkills(builtinFS, skills)

	// Tier 2: Global
	loadGlobalSkills(skills)

	// Tier 3: Project (highest priority, loaded last so it overrides)
	loadProjectSkills(cwd, skills)

	// Convert map to slice
	result := make([]*Skill, 0, len(skills))
	for _, s := range skills {
		result = append(result, s)
	}
	return result
}

// loadBuiltinSkills loads skills from the embedded filesystem.
func loadBuiltinSkills(builtinFS fs.FS, skills map[string]*Skill) {
	entries, err := fs.ReadDir(builtinFS, "builtin")
	if err != nil {
		slog.Warn("failed to read builtin skills directory", "error", err)
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillDir := "builtin/" + entry.Name()
		skillPath := skillDir + "/SKILL.md"

		if _, err := fs.Stat(builtinFS, skillPath); err != nil {
			continue // no SKILL.md, not a skill
		}

		f, err := builtinFS.Open(skillPath)
		if err != nil {
			slog.Warn("failed to open builtin SKILL.md", "skill", entry.Name(), "error", err)
			continue
		}

		skill, err := ParseSkillMD(f, entry.Name())
		f.Close()
		if err != nil {
			slog.Warn("skipping invalid builtin skill", "skill", entry.Name(), "error", err)
			continue
		}

		skill.Source = SourceBuiltin
		skill.fsys = builtinFS
		skill.dir = "" // builtin has no filesystem directory
		skills[skill.Name] = skill
	}
}

// loadGlobalSkills loads skills from global directories.
func loadGlobalSkills(skills map[string]*Skill) {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("unable to determine home directory, skipping global skills", "error", err)
		return
	}

	globalDirs := []string{
		filepath.Join(home, ".tau", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}

	for _, dir := range globalDirs {
		loadSkillDirectory(dir, SourceGlobal, skills)
	}
}

// loadProjectSkills walks up from cwd looking for .agents/skills/ directories.
func loadProjectSkills(cwd string, skills map[string]*Skill) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		slog.Warn("unable to resolve cwd for project skill discovery", "error", err)
		return
	}

	for {
		skillsDir := filepath.Join(dir, ".agents", "skills")
		loadSkillDirectory(skillsDir, SourceProject, skills)

		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached filesystem root
		}
		dir = parent
	}
}

// loadSkillDirectory scans a directory for skills (subdirectories with SKILL.md).
// It follows symlinks, skips node_modules, and respects .gitignore/.ignore patterns.
func loadSkillDirectory(dir, source string, skills map[string]*Skill) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // directory doesn't exist or isn't readable — silently skip
	}

	ignorePatterns := loadIgnorePatterns(dir)

	for _, entry := range entries {
		name := entry.Name()

		// Skip node_modules
		if name == "node_modules" {
			continue
		}

		// Check .gitignore/.ignore patterns
		if matchesIgnorePattern(name, ignorePatterns) {
			continue
		}

		// Resolve symlinks — entry.IsDir() is false for symlinks.
		fullPath := filepath.Join(dir, name)
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			continue
		}

		skillPath := filepath.Join(fullPath, "SKILL.md")

		// Check SKILL.md exists and is a regular file
		skillInfo, err := os.Stat(skillPath)
		if err != nil || skillInfo.IsDir() {
			continue // no SKILL.md
		}

		f, err := os.Open(skillPath)
		if err != nil {
			slog.Warn("failed to open SKILL.md", "skill", name, "path", skillPath, "error", err)
			continue
		}

		skill, err := ParseSkillMD(f, name)
		f.Close()
		if err != nil {
			slog.Warn("skipping invalid skill", "skill", name, "path", skillPath, "error", err)
			continue
		}

		skill.Source = source
		skill.dir = filepath.Join(dir, name)
		skill.fsys = os.DirFS(skill.dir)

		// Load reference .md files from skill root (direct children only)
		skill.References = append(skill.References, loadReferenceFiles(skill.dir, skill.References)...)

		skills[skill.Name] = skill
	}
}

// loadReferenceFiles finds .md files that are direct children of the skill directory,
// excluding SKILL.md itself.
func loadReferenceFiles(dir string, existing []string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return existing
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "SKILL.md" {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		existing = append(existing, name)
	}
	return existing
}

// ignorePattern represents a single line from a .gitignore or .ignore file.
type ignorePattern struct {
	pattern string
	negate  bool // starts with !
}

// loadIgnorePatterns reads .gitignore and .ignore from the given directory.
func loadIgnorePatterns(dir string) []ignorePattern {
	var patterns []ignorePattern

	for _, filename := range []string{".gitignore", ".ignore"} {
		data, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			continue
		}

		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			negate := false
			if strings.HasPrefix(line, "!") {
				negate = true
				line = strings.TrimPrefix(line, "!")
			}

			patterns = append(patterns, ignorePattern{pattern: line, negate: negate})
		}
	}

	return patterns
}

// matchesIgnorePattern checks if a name matches any of the ignore patterns.
// Supports simple glob patterns with * and ?.
func matchesIgnorePattern(name string, patterns []ignorePattern) bool {
	for _, p := range patterns {
		matched, err := filepath.Match(p.pattern, name)
		if err != nil {
			continue // invalid pattern, skip
		}
		if matched {
			return !p.negate
		}
	}
	return false
}

package skills

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/adam/tau/internal/testutil"
)

func TestDiscoverSkills_EmptyDirs(t *testing.T) {
	root := testutil.TempDir(t)
	skills := DiscoverSkillsWithFS(root, emptyEmbedFS{})
	if len(skills) != 0 {
		t.Errorf("expected 0 skills from empty dirs, got %d", len(skills))
	}
}

func TestDiscoverSkills_BuiltinOnly(t *testing.T) {
	root := testutil.TempDir(t)
	skills := DiscoverSkillsWithFS(root, testBuiltinFS)
	if len(skills) < 2 {
		t.Errorf("expected at least 2 builtin skills, got %d", len(skills))
	}
	names := skillNames(skills)
	if !contains(names, "skill-builder") {
		t.Errorf("missing builtin skill 'skill-builder', found: %v", names)
	}
	if !contains(names, "subagent-builder") {
		t.Errorf("missing builtin skill 'subagent-builder', found: %v", names)
	}
}

func TestDiscoverSkills_ProjectOverridesGlobal(t *testing.T) {
	// Create a temp dir that serves as both cwd and home
	root := testutil.TempDir(t)
	testutil.SetHomeEnv(t, root)

	// Global skill
	globalDir := filepath.Join(root, ".tau", "skills", "my-skill")
	mkdir(t, globalDir)
	writeFile(t, filepath.Join(globalDir, "SKILL.md"),
		"---\nname: my-skill\ndescription: Global version\n---\n\nGlobal content.\n")

	// Project skill via .tau/skills/ (overrides global)
	cwd := filepath.Join(root, "project")
	projectDir := filepath.Join(cwd, ".tau", "skills", "my-skill")
	mkdir(t, projectDir)
	writeFile(t, filepath.Join(projectDir, "SKILL.md"),
		"---\nname: my-skill\ndescription: Project version\n---\n\nProject content.\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Description != "Project version" {
		t.Errorf("expected project override, got description: %q", skills[0].Description)
	}
	if skills[0].Source != SourceProject {
		t.Errorf("expected source=project, got %q", skills[0].Source)
	}
}

func TestDiscoverSkills_NodeModulesSkipped(t *testing.T) {
	cwd := testutil.TempDir(t)

	skillsDir := filepath.Join(cwd, ".agents", "skills")

	// Skill inside node_modules — should be skipped
	mkdir(t, filepath.Join(skillsDir, "node_modules", "evil-skill"))
	writeFile(t, filepath.Join(skillsDir, "node_modules", "evil-skill", "SKILL.md"),
		"---\nname: evil-skill\ndescription: Should not load\n---\n")

	// Good skill
	mkdir(t, filepath.Join(skillsDir, "good-skill"))
	writeFile(t, filepath.Join(skillsDir, "good-skill", "SKILL.md"),
		"---\nname: good-skill\ndescription: Should load\n---\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (node_modules skipped), got %d", len(skills))
	}
	if skills[0].Name != "good-skill" {
		t.Errorf("expected 'good-skill', got %q", skills[0].Name)
	}
}

func TestDiscoverSkills_GitignorePatterns(t *testing.T) {
	cwd := testutil.TempDir(t)
	skillsDir := filepath.Join(cwd, ".agents", "skills")
	mkdir(t, skillsDir)

	writeFile(t, filepath.Join(skillsDir, ".gitignore"), "ignored-skill\n")

	mkdir(t, filepath.Join(skillsDir, "good-skill"))
	writeFile(t, filepath.Join(skillsDir, "good-skill", "SKILL.md"),
		"---\nname: good-skill\ndescription: Should load\n---\n")

	mkdir(t, filepath.Join(skillsDir, "ignored-skill"))
	writeFile(t, filepath.Join(skillsDir, "ignored-skill", "SKILL.md"),
		"---\nname: ignored-skill\ndescription: Should be skipped\n---\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (.gitignore respected), got %d", len(skills))
	}
	if skills[0].Name != "good-skill" {
		t.Errorf("expected 'good-skill', got %q", skills[0].Name)
	}
}

func TestDiscoverSkills_GitignoreGlobPattern(t *testing.T) {
	cwd := testutil.TempDir(t)
	skillsDir := filepath.Join(cwd, ".agents", "skills")
	mkdir(t, skillsDir)

	writeFile(t, filepath.Join(skillsDir, ".gitignore"), "test-*\n")

	mkdir(t, filepath.Join(skillsDir, "prod-skill"))
	writeFile(t, filepath.Join(skillsDir, "prod-skill", "SKILL.md"),
		"---\nname: prod-skill\ndescription: Should load\n---\n")

	mkdir(t, filepath.Join(skillsDir, "test-skill-a"))
	writeFile(t, filepath.Join(skillsDir, "test-skill-a", "SKILL.md"),
		"---\nname: test-skill-a\ndescription: Should be skipped\n---\n")

	mkdir(t, filepath.Join(skillsDir, "test-skill-b"))
	writeFile(t, filepath.Join(skillsDir, "test-skill-b", "SKILL.md"),
		"---\nname: test-skill-b\ndescription: Should be skipped\n---\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (glob pattern respected), got %d", len(skills))
	}
	if skills[0].Name != "prod-skill" {
		t.Errorf("expected 'prod-skill', got %q", skills[0].Name)
	}
}

func TestDiscoverSkills_NoRecursion(t *testing.T) {
	cwd := testutil.TempDir(t)
	skillsDir := filepath.Join(cwd, ".agents", "skills")

	mkdir(t, filepath.Join(skillsDir, "parent"))
	writeFile(t, filepath.Join(skillsDir, "parent", "SKILL.md"),
		"---\nname: parent\ndescription: Parent skill\n---\n")

	mkdir(t, filepath.Join(skillsDir, "parent", "child"))
	writeFile(t, filepath.Join(skillsDir, "parent", "child", "SKILL.md"),
		"---\nname: child\ndescription: Should not be found\n---\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (no recursion), got %d", len(skills))
	}
	if skills[0].Name != "parent" {
		t.Errorf("expected 'parent', got %q", skills[0].Name)
	}
}

func TestDiscoverSkills_SymlinkFollowed(t *testing.T) {
	cwd := testutil.TempDir(t)

	// Actual skill in a separate location
	actualDir := filepath.Join(cwd, "actual-skills", "linked-skill")
	mkdir(t, actualDir)
	writeFile(t, filepath.Join(actualDir, "SKILL.md"),
		"---\nname: linked-skill\ndescription: Symlinked skill\n---\n\nContent.\n")

	// Symlink in skills directory
	skillsDir := filepath.Join(cwd, ".agents", "skills")
	mkdir(t, skillsDir)
	if err := os.Symlink(actualDir, filepath.Join(skillsDir, "linked-skill")); err != nil {
		t.Fatal(err)
	}

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (symlink followed), got %d", len(skills))
	}
	if skills[0].Name != "linked-skill" {
		t.Errorf("expected 'linked-skill', got %q", skills[0].Name)
	}
}

func TestDiscoverSkills_InvalidSkillSkipped(t *testing.T) {
	cwd := testutil.TempDir(t)
	skillsDir := filepath.Join(cwd, ".agents", "skills")

	// Valid skill
	mkdir(t, filepath.Join(skillsDir, "good"))
	writeFile(t, filepath.Join(skillsDir, "good", "SKILL.md"),
		"---\nname: good\ndescription: Valid skill\n---\n")

	// Invalid name (uppercase)
	mkdir(t, filepath.Join(skillsDir, "bad-name"))
	writeFile(t, filepath.Join(skillsDir, "bad-name", "SKILL.md"),
		"---\nname: BAD_NAME\ndescription: Invalid name\n---\n")

	// Missing description
	mkdir(t, filepath.Join(skillsDir, "no-desc"))
	writeFile(t, filepath.Join(skillsDir, "no-desc", "SKILL.md"),
		"---\nname: no-desc\n---\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 valid skill (invalid skipped), got %d", len(skills))
	}
	if skills[0].Name != "good" {
		t.Errorf("expected 'good', got %q", skills[0].Name)
	}
}

func TestDiscoverSkills_GlobalDirs(t *testing.T) {
	root := testutil.TempDir(t)
	testutil.SetHomeEnv(t, root)

	// .tau/skills
	tauDir := filepath.Join(root, ".tau", "skills", "tau-skill")
	mkdir(t, tauDir)
	writeFile(t, filepath.Join(tauDir, "SKILL.md"),
		"---\nname: tau-skill\ndescription: From .tau\n---\n")

	// .agents/skills (global)
	agentsDir := filepath.Join(root, ".agents", "skills", "agents-skill")
	mkdir(t, agentsDir)
	writeFile(t, filepath.Join(agentsDir, "SKILL.md"),
		"---\nname: agents-skill\ndescription: From .agents\n---\n")

	cwd := filepath.Join(root, "project")
	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	names := skillNames(skills)
	if !contains(names, "tau-skill") {
		t.Errorf("missing global skill 'tau-skill' from .tau/skills/, found: %v", names)
	}
	if !contains(names, "agents-skill") {
		t.Errorf("missing global skill 'agents-skill' from .agents/skills/, found: %v", names)
	}
}

func TestDiscoverSkills_LoadBuiltinContent(t *testing.T) {
	root := testutil.TempDir(t)
	skills := DiscoverSkillsWithFS(root, testBuiltinFS)

	for _, s := range skills {
		if s.Content == "" {
			t.Errorf("builtin skill %q has empty content", s.Name)
		}
		if s.Description == "" {
			t.Errorf("builtin skill %q has empty description", s.Name)
		}
	}
}

func TestDiscoverSkills_ProjectWalksUpParents(t *testing.T) {
	root := testutil.TempDir(t)

	mkdir(t, filepath.Join(root, "project", ".agents", "skills", "parent-skill"))
	writeFile(t, filepath.Join(root, "project", ".agents", "skills", "parent-skill", "SKILL.md"),
		"---\nname: parent-skill\ndescription: From parent directory\n---\n")

	cwd := filepath.Join(root, "project", "subdir")
	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	names := skillNames(skills)
	if !contains(names, "parent-skill") {
		t.Errorf("expected to find parent-skill when cwd is subdir, found: %v", names)
	}
}

func TestDiscoverSkills_ProjectWalksUpParentsTauSkills(t *testing.T) {
	root := testutil.TempDir(t)

	mkdir(t, filepath.Join(root, "project", ".tau", "skills", "tau-parent-skill"))
	writeFile(t, filepath.Join(root, "project", ".tau", "skills", "tau-parent-skill", "SKILL.md"),
		"---\nname: tau-parent-skill\ndescription: From .tau/skills in parent\n---\n")

	cwd := filepath.Join(root, "project", "subdir")
	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	names := skillNames(skills)
	if !contains(names, "tau-parent-skill") {
		t.Errorf("expected to find tau-parent-skill when cwd is subdir, found: %v", names)
	}
}

func TestDiscoverSkills_ProjectTauSkillsDir(t *testing.T) {
	root := testutil.TempDir(t)

	mkdir(t, filepath.Join(root, "project", ".tau", "skills", "tau-project-skill"))
	writeFile(t, filepath.Join(root, "project", ".tau", "skills", "tau-project-skill", "SKILL.md"),
		"---\nname: tau-project-skill\ndescription: From .tau/skills in project\n---\n")

	cwd := filepath.Join(root, "project")
	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	names := skillNames(skills)
	if !contains(names, "tau-project-skill") {
		t.Errorf("expected to find tau-project-skill from .tau/skills/, found: %v", names)
	}
}

func TestDiscoverSkills_ProjectTauOverridesGlobal(t *testing.T) {
	root := testutil.TempDir(t)
	testutil.SetHomeEnv(t, root)

	// Global skill
	globalDir := filepath.Join(root, ".tau", "skills", "my-skill")
	mkdir(t, globalDir)
	writeFile(t, filepath.Join(globalDir, "SKILL.md"),
		"---\nname: my-skill\ndescription: Global version\n---\n\nGlobal content.\n")

	// Project skill in .tau/skills/ (overrides global)
	cwd := filepath.Join(root, "project")
	projectDir := filepath.Join(cwd, ".tau", "skills", "my-skill")
	mkdir(t, projectDir)
	writeFile(t, filepath.Join(projectDir, "SKILL.md"),
		"---\nname: my-skill\ndescription: Project version\n---\n\nProject content.\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Description != "Project version" {
		t.Errorf("expected project override, got description: %q", skills[0].Description)
	}
	if skills[0].Source != SourceProject {
		t.Errorf("expected source=project, got %q", skills[0].Source)
	}
}

func TestDiscoverSkills_SameNameGlobalVsProject(t *testing.T) {
	root := testutil.TempDir(t)
	testutil.SetHomeEnv(t, root)

	// Global version
	mkdir(t, filepath.Join(root, ".tau", "skills", "shared"))
	writeFile(t, filepath.Join(root, ".tau", "skills", "shared", "SKILL.md"),
		"---\nname: shared\ndescription: Global version\n---\n")

	// Project version
	cwd := filepath.Join(root, "project")
	mkdir(t, filepath.Join(cwd, ".agents", "skills", "shared"))
	writeFile(t, filepath.Join(cwd, ".agents", "skills", "shared", "SKILL.md"),
		"---\nname: shared\ndescription: Project version\n---\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill (project overrides global), got %d", len(skills))
	}
	if skills[0].Source != SourceProject {
		t.Errorf("expected source=project, got %q", skills[0].Source)
	}
	if skills[0].Description != "Project version" {
		t.Errorf("expected project description, got %q", skills[0].Description)
	}
}

func TestDiscoverSkills_IgnoreFile(t *testing.T) {
	cwd := testutil.TempDir(t)
	skillsDir := filepath.Join(cwd, ".agents", "skills")
	mkdir(t, skillsDir)

	// .ignore file (not .gitignore)
	writeFile(t, filepath.Join(skillsDir, ".ignore"), "secret-*\n")

	mkdir(t, filepath.Join(skillsDir, "public-skill"))
	writeFile(t, filepath.Join(skillsDir, "public-skill", "SKILL.md"),
		"---\nname: public-skill\ndescription: Should load\n---\n")

	mkdir(t, filepath.Join(skillsDir, "secret-skill"))
	writeFile(t, filepath.Join(skillsDir, "secret-skill", "SKILL.md"),
		"---\nname: secret-skill\ndescription: Should be skipped\n---\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	names := skillNames(skills)
	if !contains(names, "public-skill") {
		t.Errorf("missing 'public-skill', found: %v", names)
	}
	if contains(names, "secret-skill") {
		t.Errorf("'secret-skill' should be ignored by .ignore file, found: %v", names)
	}
}

func TestDiscoverSkills_ReferenceFiles(t *testing.T) {
	cwd := testutil.TempDir(t)
	skillsDir := filepath.Join(cwd, ".agents", "skills")
	mkdir(t, skillsDir)

	mkdir(t, filepath.Join(skillsDir, "ref-skill"))
	writeFile(t, filepath.Join(skillsDir, "ref-skill", "SKILL.md"),
		"---\nname: ref-skill\ndescription: Skill with references\n---\n")
	writeFile(t, filepath.Join(skillsDir, "ref-skill", "guide.md"),
		"# Guide\n\nThis is a reference file.\n")
	writeFile(t, filepath.Join(skillsDir, "ref-skill", "examples.md"),
		"# Examples\n\nSome examples.\n")

	skills := DiscoverSkillsWithFS(cwd, emptyEmbedFS{})
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	refs := skills[0].References
	if !contains(refs, "guide.md") || !contains(refs, "examples.md") {
		t.Errorf("expected references [guide.md, examples.md], got %v", refs)
	}
}

// emptyEmbedFS is an fs.FS that contains no skills, for testing non-builtin scenarios.
type emptyEmbedFS struct{}

func (emptyEmbedFS) Open(_ string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

func (emptyEmbedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return nil, nil
}

// Test helpers.

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// testBuiltinFS wraps the real embedded filesystem for testing.
var testBuiltinFS = builtinFS

func skillNames(skills []*Skill) []string {
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	return names
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

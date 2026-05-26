package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adam/tau/internal/config"
	"github.com/adam/tau/internal/testutil"
)

func TestEncodeCWD_NormalPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/", "root"},
		{"/home/adam/Projects/tau", "-home-adam-Projects-tau-"},
		{"/a", "-a-"},
		{"/a/b/c", "-a-b-c-"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := config.EncodeCWD(tt.path)
			if got != tt.want {
				t.Errorf("EncodeCWD(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestEncodeCWD_SpecialCharacters(t *testing.T) {
	got := config.EncodeCWD("/home/user/my project")
	want := "-home-user-my project-"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEncodeCWD_EmptyString(t *testing.T) {
	got := config.EncodeCWD("")
	want := "--"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestContextFileSearchList(t *testing.T) {
	// Use a temp directory to control the search path
	dir := testutil.TempDir(t)
	home := filepath.Dir(dir)

	// Create AGENTS.md in the temp dir
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("test"), 0644); err != nil {
		t.Fatalf("writing AGENTS.md: %v", err)
	}

	// Set HOME so ContextFileSearchList uses our temp home
	testutil.SetHomeEnv(t, home)
	// Create global AGENTS.md in temp home
	globalDir := filepath.Join(home, ".tau")
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	paths, err := config.ContextFileSearchList(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First two should be AGENTS.md and CLAUDE.md in dir
	if len(paths) < 2 {
		t.Fatalf("expected at least 2 paths, got %d", len(paths))
	}
	if paths[0] != filepath.Join(dir, "AGENTS.md") {
		t.Errorf("paths[0]: got %q, want %q", paths[0], filepath.Join(dir, "AGENTS.md"))
	}
	if paths[1] != filepath.Join(dir, "CLAUDE.md") {
		t.Errorf("paths[1]: got %q, want %q", paths[1], filepath.Join(dir, "CLAUDE.md"))
	}

	// Last path should be global AGENTS.md
	lastPath := paths[len(paths)-1]
	expectedGlobal := filepath.Join(home, ".tau", "AGENTS.md")
	if lastPath != expectedGlobal {
		t.Errorf("last path: got %q, want %q", lastPath, expectedGlobal)
	}
}

func TestContextFileSearchList_RootPath(t *testing.T) {
	paths, err := config.ContextFileSearchList("/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have at least AGENTS.md and CLAUDE.md for root, plus global
	if len(paths) < 3 {
		t.Errorf("expected at least 3 paths for root, got %d", len(paths))
	}
}

func TestContextFileSearchList_NestedDir(t *testing.T) {
	// Create a nested directory structure
	root := testutil.TempDir(t)
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	paths, err := config.ContextFileSearchList(nested)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should include AGENTS.md/CLAUDE.md for each level (c, b, a, root) plus global
	// 4 levels * 2 files + 1 global = 9
	expectedMin := 9
	if len(paths) < expectedMin {
		t.Errorf("expected at least %d paths, got %d: %v", expectedMin, len(paths), paths)
	}
}

func TestComputePaths(t *testing.T) {
	cwd := "/home/adam/Projects/tau"

	home := testutil.TempDir(t)
	testutil.SetHomeEnv(t, home)

	paths, err := config.ComputePaths(cwd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if paths.EncodedCwd != "-home-adam-Projects-tau-" {
		t.Errorf("EncodedCwd: got %q, want %q", paths.EncodedCwd, "-home-adam-Projects-tau-")
	}

	expectedSessionsDir := filepath.Join(home, ".tau", "sessions", "-home-adam-Projects-tau-")
	if paths.SessionsDir != expectedSessionsDir {
		t.Errorf("SessionsDir: got %q, want %q", paths.SessionsDir, expectedSessionsDir)
	}
}

func TestComputeSkillsDirs_GlobalOnly(t *testing.T) {
	home := testutil.TempDir(t)
	testutil.SetHomeEnv(t, home)

	// No project skills dirs, only global ones
	dir := testutil.TempDir(t)
	paths, err := config.ComputePaths(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have at least the two global skill dirs
	if len(paths.SkillsDirs) < 2 {
		t.Errorf("expected at least 2 global skills dirs, got %d", len(paths.SkillsDirs))
	}
}

func TestComputeSkillsDirs_ProjectAndGlobal(t *testing.T) {
	home := testutil.TempDir(t)
	testutil.SetHomeEnv(t, home)

	// Create a project-level skills directory
	root := testutil.TempDir(t)
	skillsDir := filepath.Join(root, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	paths, err := config.ComputePaths(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First should be project skills dir
	if paths.SkillsDirs[0] != skillsDir {
		t.Errorf("SkillsDirs[0]: got %q, want %q", paths.SkillsDirs[0], skillsDir)
	}
}

func TestLatestSessionFile_ReturnsMostRecent(t *testing.T) {
	dir := testutil.TempDir(t)

	// Create two session files with different modification times
	file1 := filepath.Join(dir, "20260101T120000_abc.jsonl")
	file2 := filepath.Join(dir, "20260101T130000_def.jsonl")
	if err := os.WriteFile(file1, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	// Make file2 more recently modified than file1
	oldTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)
	if err := os.Chtimes(file1, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file2, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	got, err := config.LatestSessionFile(dir)
	if err != nil {
		t.Fatalf("LatestSessionFile: %v", err)
	}
	if got != file2 {
		t.Errorf("got %q, want %q", got, file2)
	}
}

func TestLatestSessionFile_SameMtime_UsesFilename(t *testing.T) {
	dir := testutil.TempDir(t)

	file1 := filepath.Join(dir, "20260101T120000_abc.jsonl")
	file2 := filepath.Join(dir, "20260101T130000_def.jsonl")
	if err := os.WriteFile(file1, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set identical modification times
	sameTime := time.Date(2026, 1, 1, 12, 30, 0, 0, time.UTC)
	if err := os.Chtimes(file1, sameTime, sameTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(file2, sameTime, sameTime); err != nil {
		t.Fatal(err)
	}

	got, err := config.LatestSessionFile(dir)
	if err != nil {
		t.Fatalf("LatestSessionFile: %v", err)
	}
	// When mtime is equal, filename is the tie-breaker (lexicographic)
	if got != file2 {
		t.Errorf("got %q, want %q", got, file2)
	}
}

func TestLatestSessionFile_OlderFileModifiedLater(t *testing.T) {
	dir := testutil.TempDir(t)

	// file1 has an older creation timestamp but will be modified later
	file1 := filepath.Join(dir, "20260101T120000_abc.jsonl")
	file2 := filepath.Join(dir, "20260101T130000_def.jsonl")
	if err := os.WriteFile(file1, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2, []byte("b"), 0644); err != nil {
		t.Fatal(err)
	}

	// file1 created first with old mtime, file2 created with middle mtime
	midTime := time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 1, 1, 14, 0, 0, 0, time.UTC)
	if err := os.Chtimes(file2, midTime, midTime); err != nil {
		t.Fatal(err)
	}
	// Make file1 newest by setting its mtime after file2
	if err := os.Chtimes(file1, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	got, err := config.LatestSessionFile(dir)
	if err != nil {
		t.Fatalf("LatestSessionFile: %v", err)
	}
	if got != file1 {
		t.Errorf("got %q, want %q (older creation but newer mtime)", got, file1)
	}
}

func TestLatestSessionFile_NoFiles(t *testing.T) {
	dir := testutil.TempDir(t)

	got, err := config.LatestSessionFile(dir)
	if err != nil {
		t.Fatalf("LatestSessionFile: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

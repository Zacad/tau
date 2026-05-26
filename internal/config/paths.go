package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TauPaths holds computed directory paths for Tau.
type TauPaths struct {
	// HomeDir is the user's home directory.
	HomeDir string

	// TauDir is ~/.tau
	TauDir string

	// SkillsDirs contains all skill directories (global + project).
	SkillsDirs []string

	// SessionsDir is ~/.tau/sessions/<encoded-cwd>
	SessionsDir string

	// EncodedCwd is the working directory encoded for filesystem use.
	EncodedCwd string
}

// ComputePaths resolves all Tau directory paths based on the given
// working directory. If cwd is empty, uses os.Getwd().
func ComputePaths(cwd string) (*TauPaths, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	tauDir := filepath.Join(home, TauDirName)
	encodedCwd := EncodeCWD(cwd)
	sessionsDir := filepath.Join(tauDir, SessionsDirName, encodedCwd)

	skillsDirs := computeSkillsDirs(cwd, home)

	return &TauPaths{
		HomeDir:     home,
		TauDir:   tauDir,
		SkillsDirs:  skillsDirs,
		SessionsDir: sessionsDir,
		EncodedCwd:  encodedCwd,
	}, nil
}

// EncodeCWD encodes a directory path for use as a filesystem directory name.
// Replaces "/" with "-", wrapping with leading/trailing "-".
// Special case: root "/" is encoded as "root".
func EncodeCWD(path string) string {
	if path == "/" {
		return "root"
	}
	// Strip leading "/"
	stripped := strings.TrimPrefix(path, "/")
	// Replace remaining "/" with "-"
	encoded := strings.ReplaceAll(stripped, "/", "-")
	// Wrap with "-"
	return "-" + encoded + "-"
}

// ContextFileSearchList returns the ordered list of file paths to check
// for AGENTS.md and CLAUDE.md, walking from cwd up to the filesystem root.
// It does NOT read file contents — the caller should use os.ReadFile on each path.
// If cwd is empty, uses os.Getwd().
// Returns paths in order from cwd upward (closest first).
func ContextFileSearchList(cwd string) ([]string, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	// Clean the path to resolve any trailing slashes
	cwd = filepath.Clean(cwd)

	var paths []string
	dir := cwd

	for {
		paths = append(paths,
			filepath.Join(dir, "AGENTS.md"),
			filepath.Join(dir, "CLAUDE.md"),
		)

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	// Add global AGENTS.md
	home, err := os.UserHomeDir()
	if err == nil {
		globalPath := filepath.Join(home, TauDirName, "AGENTS.md")
		paths = append(paths, globalPath)
	}

	return paths, nil
}

// computeSkillsDirs returns the list of skill directories in priority order:
// 1. Project-level .agents/skills/ (walk up from cwd)
// 2. Global ~/.tau/skills/
// 3. Global ~/.agents/skills/
func computeSkillsDirs(cwd, home string) []string {
	var dirs []string

	// Walk up from cwd looking for .agents/skills/
	dir := filepath.Clean(cwd)
	for {
		candidate := filepath.Join(dir, AgentsDirName, SkillsDirName)
		if _, err := os.Stat(candidate); err == nil {
			dirs = append(dirs, candidate)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Global skill directories
	dirs = append(dirs, filepath.Join(home, TauDirName, SkillsDirName))
	dirs = append(dirs, filepath.Join(home, AgentsDirName, SkillsDirName))

	return dirs
}

// SessionsDir returns the session directory path for the given cwd.
// Creates the directory if it doesn't exist.
func SessionsDir(cwd string) (string, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	encoded := EncodeCWD(cwd)
	dir := filepath.Join(home, TauDirName, SessionsDirName, encoded)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	return dir, nil
}

// LatestSessionFile finds the most recent session file in the given directory
// by file modification time, using filename as a deterministic tie-breaker.
// Returns the full path, or empty string if no sessions exist.
func LatestSessionFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	// Collect .jsonl files with their modification times
	type fileWithTime struct {
		name string
		mod  time.Time
	}
	var files []fileWithTime
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			info, err := e.Info()
			if err != nil {
				continue // skip files we can't stat
			}
			files = append(files, fileWithTime{name: e.Name(), mod: info.ModTime()})
		}
	}

	if len(files) == 0 {
		return "", nil
	}

	// Sort by modification time (newest last), filename as tie-breaker
	sort.Slice(files, func(i, j int) bool {
		if files[i].mod.Equal(files[j].mod) {
			return files[i].name < files[j].name
		}
		return files[i].mod.Before(files[j].mod)
	})

	return filepath.Join(dir, files[len(files)-1].name), nil
}

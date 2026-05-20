package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TempDir creates a temporary directory managed by t.Cleanup.
// Returns the directory path.
func TempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

// TempFile creates a temporary file with the given content in the test's
// temp directory. Returns the file path.
func TempFile(t *testing.T, name string, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp file %s: %v", path, err)
	}
	return path
}

// TempDirTree creates a nested directory structure with files.
// Each entry in files is a path (relative to the temp dir) with content.
// Directories are created as needed. Returns the root temp dir path.
func TempDirTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("creating directory for %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("writing temp file %s: %v", path, err)
		}
	}
	return root
}

// SetupTauHome creates a temporary ~/.tau-like directory structure
// for testing config loading without touching the real home directory.
// Returns the temp home directory path.
func SetupTauHome(t *testing.T, configJSON, authJSON string) string {
	t.Helper()
	home := t.TempDir()
	tauDir := filepath.Join(home, ".tau")
	if err := os.MkdirAll(tauDir, 0755); err != nil {
		t.Fatalf("creating tau dir: %v", err)
	}
	if configJSON != "" {
		if err := os.WriteFile(filepath.Join(tauDir, "config.json"), []byte(configJSON), 0644); err != nil {
			t.Fatalf("writing config.json: %v", err)
		}
	}
	if authJSON != "" {
		if err := os.WriteFile(filepath.Join(tauDir, "auth.json"), []byte(authJSON), 0600); err != nil {
			t.Fatalf("writing auth.json: %v", err)
		}
	}
	return home
}

// SetHomeEnv sets HOME to dir and restores it after the test.
func SetHomeEnv(t *testing.T, dir string) {
	t.Helper()
	orig, had := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", dir); err != nil {
		t.Fatalf("setting HOME: %v", err)
	}
	t.Cleanup(func() {
		if had {
			os.Setenv("HOME", orig)
		} else {
			os.Unsetenv("HOME")
		}
	})
}

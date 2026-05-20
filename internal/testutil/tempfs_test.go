package testutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adam/tau/internal/testutil"
)

func TestTempDir(t *testing.T) {
	dir := testutil.TempDir(t)
	if dir == "" {
		t.Fatal("TempDir returned empty path")
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat temp dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestTempFile(t *testing.T) {
	path := testutil.TempFile(t, "test.txt", "hello world")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temp file: %v", err)
	}
	if string(content) != "hello world" {
		t.Errorf("content: got %q, want %q", string(content), "hello world")
	}
}

func TestTempDirTree(t *testing.T) {
	files := map[string]string{
		"file1.txt":           "content1",
		"sub/file2.txt":       "content2",
		"sub/deep/file3.txt":  "content3",
	}

	root := testutil.TempDirTree(t, files)

	for name, want := range files {
		path := filepath.Join(root, name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if string(content) != want {
			t.Errorf("%s: got %q, want %q", name, string(content), want)
		}
	}
}

func TestSetupTauHome(t *testing.T) {
	home := testutil.SetupTauHome(t, `{"default_model": "test"}`, `{"openai": "sk-key"}`)

	configPath := filepath.Join(home, ".tau", "config.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("config.json not found: %v", err)
	}

	authPath := filepath.Join(home, ".tau", "auth.json")
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatalf("auth.json not found: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("auth.json permissions: got %o, want 0600", perm)
	}
}

func TestSetHomeEnv(t *testing.T) {
	origHome, hadOrig := os.LookupEnv("HOME")
	tempDir := testutil.TempDir(t)

	testutil.SetHomeEnv(t, tempDir)

	currentHome := os.Getenv("HOME")
	if currentHome != tempDir {
		t.Errorf("HOME: got %q, want %q", currentHome, tempDir)
	}

	// Cleanup happens after test, verify it was set correctly
	_ = origHome
	_ = hadOrig
}

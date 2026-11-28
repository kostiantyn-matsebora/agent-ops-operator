package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// mustRunGit runs a git command in dir, failing the test on error. Test-only
// plumbing: every "clone against a real repository" test in this package
// needs one of these, local and network-free (file:// URLs throughout).
func mustRunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// newOriginRepo creates a local git repository with the given files already
// committed, standing in for the "repository at a URL" every clone test
// needs. No bare repo is required — cloning from an ordinary working
// directory over file:// works exactly the same for this component's
// purposes.
func newOriginRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	mustRunGit(t, dir, "init", "-q", "-b", "main")
	mustRunGit(t, dir, "config", "user.email", "workspace-sync-test@example.com")
	mustRunGit(t, dir, "config", "user.name", "workspace-sync-test")
	for name, content := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	mustRunGit(t, dir, "add", ".")
	mustRunGit(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

// fileURL turns a local path into the file:// URL git's clone accepts.
func fileURL(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs %s: %v", path, err)
	}
	return "file://" + abs
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

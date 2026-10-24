package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitAt runs a real git command against dir, for setting up a "remote" repo
// to sync from. It is test scaffolding, not the code under test.
func gitAt(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(gitBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// Sync with no URL is a no-op -- a repo-less profile carries its role inline.
func TestSyncWithNoURLIsANoOp(t *testing.T) {
	r := Repo{Workspace: t.TempDir()}
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("no-op sync must not error: %v", err)
	}
}

// Sync clones on first use (no .git yet) and thereafter fetch+resets, against
// a REAL local repository -- no mocking of git itself.
func TestSyncClonesThenFetchesAndResetsAgainstARealRepo(t *testing.T) {
	remote := t.TempDir()
	gitAt(t, remote, "init", "-b", "main")
	os.WriteFile(filepath.Join(remote, "f.txt"), []byte("v1"), 0o644)
	gitAt(t, remote, "add", "f.txt")
	gitAt(t, remote, "commit", "-m", "v1")

	ws := filepath.Join(t.TempDir(), "workspace")
	r := Repo{URL: remote, Ref: "main", Workspace: ws}
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("first sync (clone): %v", err)
	}
	data, err := os.ReadFile(filepath.Join(ws, "f.txt"))
	if err != nil || string(data) != "v1" {
		t.Fatalf("clone did not check out the file: %v %q", err, data)
	}

	// A second sync, with the remote having moved on, must fetch+reset the
	// existing checkout rather than re-clone -- the workspace already has a
	// .git directory this time.
	os.WriteFile(filepath.Join(remote, "f.txt"), []byte("v2"), 0o644)
	gitAt(t, remote, "commit", "-am", "v2")
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("second sync (fetch+reset): %v", err)
	}
	data, err = os.ReadFile(filepath.Join(ws, "f.txt"))
	if err != nil || string(data) != "v2" {
		t.Fatalf("fetch+reset did not pick up the new commit: %v %q", err, data)
	}
}

// A local change in the workspace must not survive a re-sync: --hard is what
// makes this a RESET, not a merge.
func TestSyncHardResetsLocalChanges(t *testing.T) {
	remote := t.TempDir()
	gitAt(t, remote, "init", "-b", "main")
	os.WriteFile(filepath.Join(remote, "f.txt"), []byte("v1"), 0o644)
	gitAt(t, remote, "add", "f.txt")
	gitAt(t, remote, "commit", "-m", "v1")

	ws := filepath.Join(t.TempDir(), "workspace")
	r := Repo{URL: remote, Ref: "main", Workspace: ws}
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("clone: %v", err)
	}
	os.WriteFile(filepath.Join(ws, "f.txt"), []byte("locally modified"), 0o644)
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "f.txt"))
	if string(data) != "v1" {
		t.Errorf("a local edit must be discarded on re-sync, got %q", data)
	}
}

// clearDir empties a directory's CONTENTS without removing the directory
// itself -- the workspace is a mount point.
func TestClearDirRemovesContentsButNotTheDirectory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a"), []byte("x"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub", "nested"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "nested", "b"), []byte("x"), 0o644)
	if err := clearDir(dir); err != nil {
		t.Fatalf("clearDir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("directory itself must survive: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("want an empty directory, got %v", entries)
	}
}

func TestClearDirOnAMissingDirectoryIsAnError(t *testing.T) {
	if err := clearDir(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("want an error reading a missing directory")
	}
}

// Sync clears a half-populated workspace before cloning into it -- the case
// where a previous attempt left files behind but no .git.
func TestSyncClearsAPartialWorkspaceBeforeCloning(t *testing.T) {
	remote := t.TempDir()
	gitAt(t, remote, "init", "-b", "main")
	os.WriteFile(filepath.Join(remote, "f.txt"), []byte("v1"), 0o644)
	gitAt(t, remote, "add", "f.txt")
	gitAt(t, remote, "commit", "-m", "v1")

	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "leftover.txt"), []byte("stale"), 0o644)
	r := Repo{URL: remote, Ref: "main", Workspace: ws}
	if err := r.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws, "leftover.txt")); !os.IsNotExist(err) {
		t.Error("a stale file from a previous failed attempt must be cleared before cloning")
	}
	if data, _ := os.ReadFile(filepath.Join(ws, "f.txt")); string(data) != "v1" {
		t.Error("the clone itself must still succeed")
	}
}

// cloneURL embeds the token as an x-access-token basic-auth prefix ONLY for
// https auth with a token and an https:// URL; every other combination is
// passed through unchanged.
func TestCloneURLEmbedsTheTokenOnlyForHTTPSAuth(t *testing.T) {
	r := Repo{URL: "https://example.test/org/repo.git", AuthType: "https", Token: "sekret"}
	got := r.cloneURL()
	want := "https://x-access-token:sekret@example.test/org/repo.git"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	// no token: URL unchanged
	r2 := Repo{URL: "https://example.test/org/repo.git", AuthType: "https"}
	if got := r2.cloneURL(); got != r2.URL {
		t.Errorf("no token must leave the URL unchanged, got %q", got)
	}
	// ssh auth: URL unchanged regardless of token
	r3 := Repo{URL: "git@example.test:org/repo.git", AuthType: "ssh", Token: "unused"}
	if got := r3.cloneURL(); got != r3.URL {
		t.Errorf("ssh auth must leave the URL unchanged, got %q", got)
	}
}

// env only sets GIT_SSH_COMMAND for ssh auth with a key configured.
func TestEnvSetsGitSSHCommandOnlyForSSHAuth(t *testing.T) {
	r := Repo{AuthType: "ssh", SSHKey: "/data/context/.ssh/id"}
	env := r.env()
	found := false
	for _, kv := range env {
		if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
			found = true
			if !strings.Contains(kv, r.SSHKey) {
				t.Errorf("GIT_SSH_COMMAND must name the key: %s", kv)
			}
		}
	}
	if !found {
		t.Error("ssh auth with a key must set GIT_SSH_COMMAND")
	}

	r2 := Repo{AuthType: "https", Token: "x"}
	for _, kv := range r2.env() {
		if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
			t.Errorf("https auth must not set GIT_SSH_COMMAND: %s", kv)
		}
	}
}

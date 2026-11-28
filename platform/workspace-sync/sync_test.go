package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Layout: zero, one, many (task 3.2) ---------------------------------

func TestSyncAllZeroReposLeavesWorkspaceEmpty(t *testing.T) {
	workspace := t.TempDir()
	if err := syncAll(context.Background(), workspace, nil, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Fatalf("reading workspace: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected an empty workspace, got %v", entries)
	}
}

func TestSyncAllOneRepoClonesIntoWorkspaceRoot(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"README.md": "hello"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mustReadFile(t, filepath.Join(workspace, "README.md")); got != "hello" {
		t.Fatalf("got %q", got)
	}
	if !fileExists(markerPath(workspace, "app")) {
		t.Fatal("expected a completion marker for app")
	}
}

func TestSyncAllManyReposCloneIntoSubdirectories(t *testing.T) {
	app := newOriginRepo(t, map[string]string{"app.txt": "app"})
	infra := newOriginRepo(t, map[string]string{"infra.txt": "infra"})
	workspace := t.TempDir()
	repos := []RepoEntry{
		{Name: "app", URL: fileURL(t, app), Ref: "main"},
		{Name: "infra", URL: fileURL(t, infra), Ref: "main"},
	}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mustReadFile(t, filepath.Join(workspace, "app", "app.txt")); got != "app" {
		t.Fatalf("got %q", got)
	}
	if got := mustReadFile(t, filepath.Join(workspace, "infra", "infra.txt")); got != "infra" {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(filepath.Join(workspace, "README.md")); !os.IsNotExist(err) {
		t.Fatal("workspace root must hold no file of its own in the many-repo layout")
	}
}

// --- keepGit (task 3.4 / spec "keepGit controls whether history stays") --

func TestSyncAllKeepGitFalseRemovesDotGit(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"f.txt": "x"})
	workspace := t.TempDir()
	no := false
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main", KeepGit: &no}}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git")); !os.IsNotExist(err) {
		t.Fatal("expected .git to be removed when keepGit is false")
	}
}

func TestSyncAllKeepGitTrueKeepsDotGitAndExcludesAgentops(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"f.txt": "x"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err != nil {
		t.Fatalf(".git should survive keepGit:true: %v", err)
	}
	exclude := mustReadFile(t, filepath.Join(workspace, ".git", "info", "exclude"))
	if !strings.Contains(exclude, ".agentops/") {
		t.Fatalf("expected .agentops/ in .git/info/exclude, got %q", exclude)
	}
}

// --- Re-run rule (task 3.4 / design D6) ----------------------------------

func TestSyncAllLeavesACompletedRepoUntouched(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"f.txt": "original"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Uncommitted edit, standing in for the agent's own in-flight work.
	editPath := filepath.Join(workspace, "f.txt")
	if err := os.WriteFile(editPath, []byte("edited by the agent"), 0o644); err != nil {
		t.Fatalf("writing edit: %v", err)
	}
	// Change what origin would serve on a re-clone, so a reset/refetch
	// would be detectable: the marker must make this run a no-op instead.
	mustRunGit(t, origin, "commit", "--allow-empty", "-q", "-m", "a later upstream commit")

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if got := mustReadFile(t, editPath); got != "edited by the agent" {
		t.Fatalf("uncommitted edit did not survive a re-run: got %q", got)
	}
}

func TestSyncAllReclonesAPartialRepoWithNoMarker(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"f.txt": "x"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}

	// Simulate a clone killed mid-way: files present, no completion marker.
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "partial.tmp"), []byte("half-written"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "partial.tmp")); !os.IsNotExist(err) {
		t.Fatal("the partial clone's leftovers should have been removed")
	}
	if got := mustReadFile(t, filepath.Join(workspace, "f.txt")); got != "x" {
		t.Fatalf("expected a fresh clone, got f.txt=%q", got)
	}
	if !fileExists(markerPath(workspace, "app")) {
		t.Fatal("expected a completion marker after the reclone")
	}
}

func TestSyncAllNeighbourNotDisturbedByAPartialSibling(t *testing.T) {
	app := newOriginRepo(t, map[string]string{"app.txt": "app"})
	infra := newOriginRepo(t, map[string]string{"infra.txt": "infra-original"})
	workspace := t.TempDir()
	repos := []RepoEntry{
		{Name: "app", URL: fileURL(t, app), Ref: "main"},
		{Name: "infra", URL: fileURL(t, infra), Ref: "main"},
	}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// infra loses its marker and gains a partial leftover, as a clone cut
	// short mid-way would; app keeps both its marker and an uncommitted edit.
	if err := os.Remove(markerPath(workspace, "infra")); err != nil {
		t.Fatalf("removing infra marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "infra", "partial.tmp"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	appEdit := filepath.Join(workspace, "app", "app.txt")
	if err := os.WriteFile(appEdit, []byte("edited"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if got := mustReadFile(t, appEdit); got != "edited" {
		t.Fatalf("app should have been left untouched, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(workspace, "infra", "partial.tmp")); !os.IsNotExist(err) {
		t.Fatal("infra's partial leftovers should have been removed")
	}
	if got := mustReadFile(t, filepath.Join(workspace, "infra", "infra.txt")); got != "infra-original" {
		t.Fatalf("infra should have been recloned, got %q", got)
	}
}

func TestSyncAllSingleRepoRetrySparesAgentopsDir(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"f.txt": "x"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Force the "no marker, partial clone" path on a retry, in the
	// single-repo layout where the repository IS the workspace root.
	if err := os.Remove(markerPath(workspace, "app")); err != nil {
		t.Fatalf("removing marker: %v", err)
	}
	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !fileExists(markerPath(workspace, "app")) {
		t.Fatal(".agentops/sync/app.done should exist after the reclone")
	}
}

func TestSyncAllStaleMarkerWithNoDirectoryReclones(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"f.txt": "x"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Multi-repo stand-in: use a second workspace layout so removing the
	// repo directory does not also remove the workspace-rooted marker dir.
	many := []RepoEntry{repos[0], {Name: "infra", URL: fileURL(t, origin), Ref: "main"}}
	workspace2 := t.TempDir()
	if err := syncAll(context.Background(), workspace2, many, nil, nil); err != nil {
		t.Fatalf("many-repo run: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(workspace2, "infra")); err != nil {
		t.Fatalf("removing infra dir: %v", err)
	}
	if !fileExists(markerPath(workspace2, "infra")) {
		t.Fatal("expected the marker to still exist before the re-run")
	}
	if err := syncAll(context.Background(), workspace2, many, nil, nil); err != nil {
		t.Fatalf("re-run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace2, "infra", "f.txt")); err != nil {
		t.Fatalf("expected infra to be recloned: %v", err)
	}
}

// --- Agent file verification (task 3.5) ----------------------------------

func TestSyncAllAgentFileMissingFails(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"f.txt": "x"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}
	agentFile := &AgentFileRef{Path: "agents/missing.md"}

	err := syncAll(context.Background(), workspace, repos, agentFile, nil)
	if err == nil {
		t.Fatal("expected an error for a missing agent file")
	}
	if !strings.Contains(err.Error(), "agents/missing.md") {
		t.Fatalf("expected the error to name the path, got %v", err)
	}
}

func TestSyncAllAgentFileDirectoryFails(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"agents/placeholder": "x"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}
	agentFile := &AgentFileRef{Path: "agents"}

	if err := syncAll(context.Background(), workspace, repos, agentFile, nil); err == nil {
		t.Fatal("expected an error: agents/ is a directory, not a file")
	}
}

func TestSyncAllAgentFileSymlinkEscapeFails(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"f.txt": "x"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}

	if err := syncAll(context.Background(), workspace, repos, nil, nil); err != nil {
		t.Fatalf("clone: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	link := filepath.Join(workspace, "escape.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	agentFile := &AgentFileRef{Path: "escape.md"}
	// SyncRepo is already satisfied (marker exists from the first call), so
	// this call exercises the agent-file check alone.
	err := syncAll(context.Background(), workspace, repos, agentFile, nil)
	if err == nil {
		t.Fatal("expected an error: escape.md resolves outside its repository")
	}
}

func TestSyncAllAgentFileValidPasses(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"agents/main.md": "hi"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main"}}
	agentFile := &AgentFileRef{Path: "agents/main.md"}

	if err := syncAll(context.Background(), workspace, repos, agentFile, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

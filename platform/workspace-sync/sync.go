package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// agentopsDirName is the one directory this component ever writes at the
// workspace root that is NOT a cloned repository. It must survive every
// re-run's cleanup, including the single-repo case where the "repository"
// and the workspace root are the same directory.
const agentopsDirName = ".agentops"

// syncMarkerDir is always rooted at the workspace, never inside a
// per-repository directory — spec.md fixes the marker path at
// /workspace/.agentops/sync/<name>.done regardless of the 1-repo-is-root or
// N-repos-in-subdirectories layout.
func syncMarkerDir(workspaceRoot string) string {
	return filepath.Join(workspaceRoot, agentopsDirName, "sync")
}

func markerPath(workspaceRoot, name string) string {
	return filepath.Join(syncMarkerDir(workspaceRoot), name+".done")
}

// Syncer clones every repository in a manifest into one workspace, per
// design D3/D5/D6.
type Syncer struct {
	WorkspaceRoot string
	Repos         []RepoEntry
	Creds         map[int]string
}

// SyncRepo brings one repository to the state design D6 requires: a marker
// present means leave it alone, whatever keepGit says; otherwise clear
// whatever is there and clone fresh, writing the marker only after the
// clone — and, when keepGit is false, the .git removal — fully succeeds.
func (s *Syncer) SyncRepo(ctx context.Context, idx int, repo RepoEntry) error {
	dest := repoDirFor(s.WorkspaceRoot, s.Repos, idx)
	marker := markerPath(s.WorkspaceRoot, repo.Name)

	dirPresent, err := dirHasEntries(dest)
	if err != nil {
		return fmt.Errorf("checking %s: %w", dest, err)
	}
	markerPresent := fileExists(marker)

	if markerPresent && dirPresent {
		// The one firm invariant (design D6): a completed clone is NEVER
		// reset, cleaned or fetched into, whatever ref now says upstream.
		return nil
	}
	if markerPresent && !dirPresent {
		// The repository vanished out from under a surviving marker —
		// treat it as never cloned rather than trust a marker the directory
		// itself no longer backs.
		if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing stale marker: %w", err)
		}
	}
	if dirPresent {
		if err := clearRepoDir(dest, dest == s.WorkspaceRoot); err != nil {
			return fmt.Errorf("clearing partial clone: %w", err)
		}
	}

	if err := s.cloneRepo(ctx, idx, repo, dest); err != nil {
		return err
	}
	if !repo.keepGit() {
		if err := os.RemoveAll(filepath.Join(dest, ".git")); err != nil {
			return fmt.Errorf("removing .git: %w", err)
		}
	} else if dest == s.WorkspaceRoot {
		// Only the single-repository layout puts .agentops INSIDE a
		// repository's own working tree (design D6) — every other layout
		// already keeps it as a sibling of every clone, outside any of
		// them.
		if err := excludeAgentopsDir(dest); err != nil {
			return fmt.Errorf("excluding %s: %w", agentopsDirName, err)
		}
	}
	if err := s.writeMarker(repo.Name); err != nil {
		return fmt.Errorf("writing completion marker: %w", err)
	}
	return nil
}

// cloneRepo performs the clone itself. The credential, when there is one,
// reaches the git process through its environment alone (design D5) — it is
// never interpolated into args, a URL or a file this clone leaves behind.
//
// It clones into a SCRATCH directory first, under the workspace's own
// .agentops/tmp (same filesystem as dest, so the move below is a rename,
// never a copy), then moves every entry into dest. Cloning directly into
// dest would refuse to run in the single-repository layout the moment
// .agentops/ exists there (git's own clone requires an empty destination,
// and "empty" does not overlook a hidden directory) — exactly the case a
// retry after a partial clone leaves behind.
func (s *Syncer) cloneRepo(ctx context.Context, idx int, repo RepoEntry, dest string) error {
	cred, err := gitEnvFor(idx, repo, s.Creds)
	if err != nil {
		return fmt.Errorf("credentials: %w", err)
	}
	defer cred.cleanup()

	tmpRoot := filepath.Join(s.WorkspaceRoot, agentopsDirName, "tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return fmt.Errorf("creating scratch directory: %w", err)
	}
	tmp, err := os.MkdirTemp(tmpRoot, repo.Name+"-")
	if err != nil {
		return fmt.Errorf("creating scratch directory: %w", err)
	}
	defer os.RemoveAll(tmp)

	args := []string{"clone"}
	if !repo.keepGit() {
		args = append(args, "--depth", "1")
	}
	if repo.Ref != "" {
		args = append(args, "--branch", repo.Ref)
	}
	args = append(args, repo.URL, tmp)

	out, err := runGit(ctx, cred.env, args...)
	if err != nil {
		return fmt.Errorf("git clone: %s: %w", strings.TrimSpace(out), err)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	return moveTreeInto(tmp, dest)
}

// moveTreeInto renames every entry of src into dest. A rename, not a copy —
// both sides live under the same workspace filesystem by construction
// (cloneRepo's tmp is always under WorkspaceRoot), so this never falls back
// to a cross-device copy.
func moveTreeInto(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.Rename(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name())); err != nil {
			return fmt.Errorf("moving %s into place: %w", e.Name(), err)
		}
	}
	return nil
}

// runGit runs one git command with its own environment, returning combined
// output for the caller to fold into an error (git's own practice of
// writing progress to stderr even on success means output alone is not a
// success/failure signal).
func runGit(ctx context.Context, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = env
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// dirHasEntries reports whether dest exists and holds at least one entry.
// An absent directory and an empty one are the same starting point: clone.
func dirHasEntries(dest string) (bool, error) {
	entries, err := os.ReadDir(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return len(entries) > 0, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// clearRepoDir empties dest for a fresh clone. When dest is the workspace
// root itself (the single-repository layout), the marker directory this
// component owns is spared — removal-on-retry must never destroy the very
// bookkeeping deciding what to retry.
func clearRepoDir(dest string, sparesAgentopsDir bool) error {
	entries, err := os.ReadDir(dest)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if sparesAgentopsDir && e.Name() == agentopsDirName {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dest, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// excludeAgentopsDir keeps .agentops/ from ever showing as untracked inside
// a kept .git — required only where .agentops sits inside the repository's
// own working tree, the single-repository layout.
func excludeAgentopsDir(dest string) error {
	excludePath := filepath.Join(dest, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	existing, _ := os.ReadFile(excludePath)
	if strings.Contains(string(existing), agentopsDirName+"/") {
		return nil
	}
	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(agentopsDirName + "/\n")
	return err
}

// writeMarker records that repository name's clone completed, LAST — the
// one fact design D6 hinges every future run's behaviour on.
func (s *Syncer) writeMarker(name string) error {
	dir := syncMarkerDir(s.WorkspaceRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := time.Now().UTC().Format(time.RFC3339) + "\n"
	return os.WriteFile(markerPath(s.WorkspaceRoot, name), []byte(content), 0o644)
}

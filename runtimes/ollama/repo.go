package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo checks the profile's repository out at the workspace, ported from the
// reference runtime's syncRepo.
type Repo struct {
	URL, Ref, AuthType, SSHKey, Token string
	Workspace                         string
}

func (r Repo) env() []string {
	env := os.Environ()
	if r.AuthType == "ssh" && r.SSHKey != "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -i "+r.SSHKey+" -o UserKnownHostsFile=/tmp/known_hosts -o StrictHostKeyChecking=accept-new")
	}
	return env
}

func (r Repo) cloneURL() string {
	if r.AuthType == "https" && r.Token != "" && strings.HasPrefix(r.URL, "https://") {
		return "https://x-access-token:" + r.Token + "@" + strings.TrimPrefix(r.URL, "https://")
	}
	return r.URL
}

func (r Repo) git(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.Workspace
	cmd.Env = r.env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		s := string(out)
		if len(s) > 400 {
			s = s[len(s)-400:]
		}
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(s))
	}
	return nil
}

// clearDir removes a directory's CONTENTS. The workspace is a mount point, so
// the directory itself is never removed.
func clearDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Sync clones on first use and fetch+resets afterwards. No URL means no
// checkout — a repo-less profile carries its role inline.
func (r Repo) Sync(ctx context.Context) error {
	if r.URL == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(r.Workspace, ".git")); err != nil {
		if err := os.MkdirAll(r.Workspace, 0o755); err != nil {
			return err
		}
		if err := clearDir(r.Workspace); err != nil {
			return err
		}
		return r.git(ctx, "clone", "--depth", "1", "-b", r.Ref, r.cloneURL(), ".")
	}
	if err := r.git(ctx, "fetch", "--depth", "1", "origin", r.Ref); err != nil {
		return err
	}
	return r.git(ctx, "reset", "--hard", "origin/"+r.Ref)
}

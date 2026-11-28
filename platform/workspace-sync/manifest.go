package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Environment variable names making up this component's whole input — see
// specs/workspace-sync/spec.md. The credential itself never travels inside
// the manifest; design D5 keys it by position instead (repoCredEnv).
const (
	envWorkspaceRepos     = "WORKSPACE_REPOS"
	envWorkspaceAgentFile = "WORKSPACE_AGENT_FILE"
	envWorkspaceRoot      = "WORKSPACE"
	defaultWorkspaceRoot  = "/workspace"
)

// repoCredEnv names the credential env var for the repository at zero-based
// position idx in the full list — "the credential is named by position
// because a repository name may hold characters an environment variable name
// cannot" (spec.md).
func repoCredEnv(idx int) string {
	return fmt.Sprintf("REPO_CRED_%d", idx)
}

// RepoAuth names how a clone must authenticate. It never carries the
// credential value itself — that arrives separately, keyed by position.
type RepoAuth struct {
	Type string `json:"type"`
}

// RepoEntry is one manifest entry, matching the repositories[] shape added
// to AgentProfile/AgentCapability. KeepGit is a pointer so an absent field
// (default true, per the proposal) is distinguishable from an explicit
// false.
type RepoEntry struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Ref     string   `json:"ref"`
	Auth    RepoAuth `json:"auth"`
	KeepGit *bool    `json:"keepGit"`
}

// keepGit resolves the default: absent means true.
func (r RepoEntry) keepGit() bool {
	if r.KeepGit == nil {
		return true
	}
	return *r.KeepGit
}

// AgentFileRef names the agent definition's location. Set only when the
// profile/capability declares one.
type AgentFileRef struct {
	Repo string `json:"repo"`
	Path string `json:"path"`
}

// parseRepos reads WORKSPACE_REPOS and validates every entry. This is
// external input — the manager's own compiled JSON, but still client data as
// far as this process is concerned — so a malformed entry fails this
// container with a specific reason rather than cloning into a traversal path
// or silently colliding two repositories on one directory name.
func parseRepos(raw string) ([]RepoEntry, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var repos []RepoEntry
	if err := json.Unmarshal([]byte(raw), &repos); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", envWorkspaceRepos, err)
	}
	seen := make(map[string]bool, len(repos))
	for i, r := range repos {
		if err := validateRepoName(r.Name); err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", envWorkspaceRepos, i, err)
		}
		if seen[r.Name] {
			return nil, fmt.Errorf("%s[%d]: duplicate repository name %q", envWorkspaceRepos, i, r.Name)
		}
		seen[r.Name] = true
		if strings.TrimSpace(r.URL) == "" {
			return nil, fmt.Errorf("%s[%d] %q: url is required", envWorkspaceRepos, i, r.Name)
		}
		switch r.Auth.Type {
		case "", "https", "ssh":
		default:
			return nil, fmt.Errorf("%s[%d] %q: unknown auth.type %q", envWorkspaceRepos, i, r.Name, r.Auth.Type)
		}
	}
	return repos, nil
}

// validateRepoName guards the one thing the CRD's own CEL cannot see from
// here: a name this process is about to turn into a directory and a marker
// file name. Safe-by-construction on the CRD side does not excuse this
// process from checking its own input again.
func validateRepoName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if name == "." || name == ".." || strings.HasPrefix(name, ".") || strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("name %q is not a safe directory name", name)
	}
	return nil
}

// parseAgentFile reads WORKSPACE_AGENT_FILE, present only when the
// profile/capability declares an agent file.
func parseAgentFile(raw string) (*AgentFileRef, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var ref AgentFileRef
	if err := json.Unmarshal([]byte(raw), &ref); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", envWorkspaceAgentFile, err)
	}
	if strings.TrimSpace(ref.Path) == "" {
		return nil, fmt.Errorf("%s: path is required", envWorkspaceAgentFile)
	}
	if strings.HasPrefix(ref.Path, "/") || strings.Contains(ref.Path, "..") {
		return nil, fmt.Errorf("%s: path %q must be relative with no \"..\"", envWorkspaceAgentFile, ref.Path)
	}
	return &ref, nil
}

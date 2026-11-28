package main

import (
	"fmt"
	"path/filepath"
)

// repoDirFor implements design D3's layout rule, the one thing this
// component owes every runtime: a single repository IS the workspace root
// (so an existing single-repo setup, CLAUDE.md at the root included, keeps
// working), two or more each land in their own named subdirectory.
func repoDirFor(workspaceRoot string, repos []RepoEntry, idx int) string {
	if len(repos) == 1 {
		return workspaceRoot
	}
	return filepath.Join(workspaceRoot, repos[idx].Name)
}

// resolveRepoDir finds the directory agentFile.repo names. With exactly one
// repository the name is optional (the proposal: "repo is optional with one
// repo"), so any value — including empty — resolves to the workspace root.
// With two or more it must name an entry in the list; a name that matches
// none fails here as a last-resort check, the cross-object validation the
// Ready condition on Pipeline/Coordinator already owns notwithstanding.
func resolveRepoDir(workspaceRoot string, repos []RepoEntry, name string) (string, error) {
	if len(repos) == 1 {
		return workspaceRoot, nil
	}
	for i, r := range repos {
		if r.Name == name {
			return repoDirFor(workspaceRoot, repos, i), nil
		}
	}
	return "", fmt.Errorf("agentFile.repo %q names no repository in the manifest", name)
}

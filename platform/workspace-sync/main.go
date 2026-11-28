// Command workspace-sync is the init container that populates a runtime
// pod's /workspace before the agent container starts (specs/workspace-sync).
//
// It is the only holder of a git credential in the pod: the manager compiles
// each repository's auth.secretRef to a REPO_CRED_<n> env var on THIS
// container alone (design D5), and nothing here ever writes that value into
// a file under /workspace, a remote URL, or anything git itself persists.
//
//  1. read WORKSPACE_REPOS (and WORKSPACE_AGENT_FILE, when set)
//  2. bring every repository to the layout design D3 fixes by count
//  3. verify the agent file, when one is named, after every clone
//  4. exit 0, or exit non-zero with a termination message naming the
//     repository and the cause — never a credential (design D9)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(0)

	creds := collectCredentials()
	red := newRedactor(creds)

	workspaceRoot := os.Getenv(envWorkspaceRoot)
	if workspaceRoot == "" {
		workspaceRoot = defaultWorkspaceRoot
	}

	repos, err := parseRepos(os.Getenv(envWorkspaceRepos))
	if err != nil {
		fail(red, fmt.Errorf("manifest: %w", err))
	}
	agentFile, err := parseAgentFile(os.Getenv(envWorkspaceAgentFile))
	if err != nil {
		fail(red, fmt.Errorf("manifest: %w", err))
	}

	if err := syncAll(context.Background(), workspaceRoot, repos, agentFile, creds); err != nil {
		fail(red, err)
	}

	if len(repos) == 0 {
		// design D3: zero repositories is a no-op, and the manager does not
		// even add this container in that case — this path exists for
		// anything that invokes the image directly anyway.
		log.Printf("workspace-sync: no repositories declared, leaving %s untouched", workspaceRoot)
		return
	}
	log.Printf("workspace-sync: %d repositor%s ready at %s", len(repos), plural(len(repos)), workspaceRoot)
}

// syncAll brings every repository in repos to the state design D6 requires
// and, when agentFile names one, verifies it afterward. Split out of main
// so the whole orchestration is one function a test can call without a
// process exit.
func syncAll(ctx context.Context, workspaceRoot string, repos []RepoEntry, agentFile *AgentFileRef, creds map[int]string) error {
	if len(repos) == 0 {
		return nil
	}
	s := &Syncer{WorkspaceRoot: workspaceRoot, Repos: repos, Creds: creds}
	for i, repo := range repos {
		if err := s.SyncRepo(ctx, i, repo); err != nil {
			return &repoError{repo: repo.Name, cause: err}
		}
	}
	if agentFile == nil {
		return nil
	}
	repoDir, err := resolveRepoDir(workspaceRoot, repos, agentFile.Repo)
	if err != nil {
		return fmt.Errorf("agent file: %w", err)
	}
	if err := verifyAgentFile(repoDir, agentFile.Path); err != nil {
		return &agentFileError{repo: agentFile.Repo, path: agentFile.Path, cause: err}
	}
	return nil
}

// repoError names the repository a clone failure happened on — the
// termination message's required shape (spec.md: "naming the repository
// and the cause").
type repoError struct {
	repo  string
	cause error
}

func (e *repoError) Error() string { return fmt.Sprintf("repository %q: %v", e.repo, e.cause) }
func (e *repoError) Unwrap() error { return e.cause }

// agentFileError names both the repository and the path the verification in
// agentfile.go failed for.
type agentFileError struct {
	repo, path string
	cause      error
}

func (e *agentFileError) Error() string {
	repo := e.repo
	if repo == "" {
		repo = "(the workspace root)"
	}
	return fmt.Sprintf("agent file %q in repository %s: %v", e.path, repo, e.cause)
}
func (e *agentFileError) Unwrap() error { return e.cause }

// fail is the one exit path for every failure this container can have:
// clone, credential or agent-file. It redacts, logs, writes the termination
// message, and exits non-zero — never a credential in any of the three
// (design D5/D9, the "A message names no secret" scenario).
func fail(red *redactor, err error) {
	msg := red.redact(err.Error())
	log.Print("workspace-sync: " + msg)
	writeTerminationMessage(msg)
	os.Exit(1)
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

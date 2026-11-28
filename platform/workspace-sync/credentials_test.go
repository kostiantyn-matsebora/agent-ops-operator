package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// grepTreeFor reports whether needle appears in any file under root.
// file:// clones are local, so this walks the whole tree directly rather
// than shelling out to grep.
func grepTreeFor(t *testing.T, root, needle string) bool {
	t.Helper()
	found := false
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.Contains(string(b), needle) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return found
}

func TestHTTPSCredentialNeverReachesTheWorkspace(t *testing.T) {
	const token = "s3cr3t-token-https"
	origin := newOriginRepo(t, map[string]string{"f.txt": "x"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main", Auth: RepoAuth{Type: "https"}}}
	creds := map[int]string{0: token}

	if err := syncAll(context.Background(), workspace, repos, nil, creds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if grepTreeFor(t, workspace, token) {
		t.Fatal("the https token must never appear under the workspace")
	}
	cfg := mustReadFile(t, filepath.Join(workspace, ".git", "config"))
	if strings.Contains(cfg, token) {
		t.Fatal("the https token must never appear in .git/config")
	}
	url := strings.TrimSpace(mustRunGit(t, workspace, "remote", "get-url", "origin"))
	if url != fileURL(t, origin) {
		t.Fatalf("origin should hold the declared URL, got %q", url)
	}
}

func TestSSHCredentialNeverReachesTheWorkspace(t *testing.T) {
	const key = "-----BEGIN OPENSSH PRIVATE KEY-----\nfake-ssh-key-material\n-----END OPENSSH PRIVATE KEY-----"
	origin := newOriginRepo(t, map[string]string{"f.txt": "x"})
	workspace := t.TempDir()
	repos := []RepoEntry{{Name: "app", URL: fileURL(t, origin), Ref: "main", Auth: RepoAuth{Type: "ssh"}}}
	creds := map[int]string{0: key}

	if err := syncAll(context.Background(), workspace, repos, nil, creds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if grepTreeFor(t, workspace, "fake-ssh-key-material") {
		t.Fatal("the ssh key must never appear under the workspace")
	}
	cfg := mustReadFile(t, filepath.Join(workspace, ".git", "config"))
	if strings.Contains(cfg, "fake-ssh-key-material") {
		t.Fatal("the ssh key must never appear in .git/config")
	}
}

func TestTwoRepositoriesTwoKeysStayApart(t *testing.T) {
	appOrigin := newOriginRepo(t, map[string]string{"app.txt": "app"})
	infraOrigin := newOriginRepo(t, map[string]string{"infra.txt": "infra"})
	workspace := t.TempDir()
	repos := []RepoEntry{
		{Name: "app", URL: fileURL(t, appOrigin), Ref: "main", Auth: RepoAuth{Type: "ssh"}},
		{Name: "infra", URL: fileURL(t, infraOrigin), Ref: "main", Auth: RepoAuth{Type: "ssh"}},
	}
	creds := map[int]string{0: "app-key-material", 1: "infra-key-material"}

	if err := syncAll(context.Background(), workspace, repos, nil, creds); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if grepTreeFor(t, workspace, "app-key-material") || grepTreeFor(t, workspace, "infra-key-material") {
		t.Fatal("no key should reach either cloned repository's tree")
	}
}

func TestGitEnvForCredentialFilesLiveOutsideAnyGivenDirectory(t *testing.T) {
	workspace := t.TempDir()
	repo := RepoEntry{Name: "app", Auth: RepoAuth{Type: "ssh"}}
	cred, err := gitEnvFor(0, repo, map[int]string{0: "key-material"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cred.cleanup()

	var sshCmd string
	for _, kv := range cred.env {
		if strings.HasPrefix(kv, "GIT_SSH_COMMAND=") {
			sshCmd = kv
		}
	}
	if sshCmd == "" {
		t.Fatal("expected a GIT_SSH_COMMAND entry")
	}
	if strings.Contains(sshCmd, workspace) {
		t.Fatalf("the ssh key file must not be placed under a workspace path, got %q", sshCmd)
	}
	if strings.Contains(sshCmd, "key-material") {
		t.Fatal("the key material must never appear on the command line, only in the file it points to")
	}
}

func TestGitEnvForMissingCredentialFailsFast(t *testing.T) {
	if _, err := gitEnvFor(0, RepoEntry{Name: "app", Auth: RepoAuth{Type: "https"}}, map[int]string{}); err == nil {
		t.Fatal("expected an error when auth.type is declared but no credential is supplied")
	}
}

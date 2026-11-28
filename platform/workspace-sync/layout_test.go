package main

import "testing"

func TestRepoDirForOneRepoIsWorkspaceRoot(t *testing.T) {
	repos := []RepoEntry{{Name: "app"}}
	if got := repoDirFor("/workspace", repos, 0); got != "/workspace" {
		t.Errorf("got %q, want /workspace", got)
	}
}

func TestRepoDirForManyUsesSubdirectories(t *testing.T) {
	repos := []RepoEntry{{Name: "app"}, {Name: "infra"}}
	if got := repoDirFor("/workspace", repos, 0); got != "/workspace/app" {
		t.Errorf("got %q, want /workspace/app", got)
	}
	if got := repoDirFor("/workspace", repos, 1); got != "/workspace/infra" {
		t.Errorf("got %q, want /workspace/infra", got)
	}
}

func TestResolveRepoDirOneRepoIgnoresName(t *testing.T) {
	repos := []RepoEntry{{Name: "app"}}
	got, err := resolveRepoDir("/workspace", repos, "")
	if err != nil || got != "/workspace" {
		t.Fatalf("got (%q, %v), want (/workspace, nil)", got, err)
	}
	got, err = resolveRepoDir("/workspace", repos, "anything")
	if err != nil || got != "/workspace" {
		t.Fatalf("got (%q, %v), want (/workspace, nil)", got, err)
	}
}

func TestResolveRepoDirManyMatchesByName(t *testing.T) {
	repos := []RepoEntry{{Name: "app"}, {Name: "infra"}}
	got, err := resolveRepoDir("/workspace", repos, "infra")
	if err != nil || got != "/workspace/infra" {
		t.Fatalf("got (%q, %v), want (/workspace/infra, nil)", got, err)
	}
}

func TestResolveRepoDirManyRejectsUnknownName(t *testing.T) {
	repos := []RepoEntry{{Name: "app"}, {Name: "infra"}}
	if _, err := resolveRepoDir("/workspace", repos, "ghost"); err == nil {
		t.Fatal("expected an error for a dangling agentFile.repo")
	}
}

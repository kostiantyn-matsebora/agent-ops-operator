package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Repo.git was already the file's least-covered method before this session
// -- it needs a real git binary, which the test environment genuinely has,
// so it is exercised directly rather than left untested because it shells
// out. Covers gitBin's own call site (go:S4036's fix) both ways: success
// and the command's own reported failure.

func TestRepoGitRunsWithTheResolvedBinary(t *testing.T) {
	dir := t.TempDir()
	r := Repo{Workspace: dir}
	if err := r.git(context.Background(), "init"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf("git init did not create .git: %v", err)
	}
}

func TestRepoGitReturnsTheCommandsOwnError(t *testing.T) {
	dir := t.TempDir()
	r := Repo{Workspace: dir}
	if err := r.git(context.Background(), "not-a-real-git-subcommand"); err == nil {
		t.Fatal("want an error for an unrecognised git subcommand")
	}
}

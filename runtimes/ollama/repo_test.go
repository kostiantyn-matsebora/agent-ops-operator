package main

import (
	"os"
	"path/filepath"
	"testing"
)

// resolveBin exists so git is spawned by an explicit, PATH-resolved path
// rather than by bare name (go:S4036).
func TestResolveBinFindsAnExecutableOnPATH(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "agentops-fake-bin")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if got := resolveBin("agentops-fake-bin"); got != bin {
		t.Fatalf("got %q, want %q", got, bin)
	}
}

func TestResolveBinFallsBackToTheBareNameWhenNothingMatches(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if got := resolveBin("agentops-nowhere"); got != "agentops-nowhere" {
		t.Fatalf("got %q, want the bare name unchanged", got)
	}
}

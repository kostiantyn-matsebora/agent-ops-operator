package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// env, envInt and logf were entirely untested (0%) -- main() wires the whole
// process from them, but they are pure enough to test directly.

func TestEnvFallsBackWhenUnset(t *testing.T) {
	os.Unsetenv("AGENTOPS_TEST_ENV_UNSET")
	if got := env("AGENTOPS_TEST_ENV_UNSET", "fallback"); got != "fallback" {
		t.Errorf("got %q, want the default", got)
	}
	t.Setenv("AGENTOPS_TEST_ENV_UNSET", "set")
	if got := env("AGENTOPS_TEST_ENV_UNSET", "fallback"); got != "set" {
		t.Errorf("got %q, want the set value", got)
	}
}

func TestEnvIntParsesFallsBackAndRejectsNonPositive(t *testing.T) {
	name := "AGENTOPS_TEST_ENV_INT"
	os.Unsetenv(name)
	if got := envInt(name, 7); got != 7 {
		t.Errorf("unset: got %d, want the default", got)
	}
	t.Setenv(name, "not-a-number")
	if got := envInt(name, 7); got != 7 {
		t.Errorf("unparseable: got %d, want the default", got)
	}
	t.Setenv(name, "0")
	if got := envInt(name, 7); got != 7 {
		t.Errorf("zero must not override: got %d, want the default", got)
	}
	t.Setenv(name, "-3")
	if got := envInt(name, 7); got != 7 {
		t.Errorf("negative must not override: got %d, want the default", got)
	}
	t.Setenv(name, "42")
	if got := envInt(name, 7); got != 42 {
		t.Errorf("a valid positive value must win: got %d", got)
	}
}

// multi-repo-workspace moved cloning into the `workspace-sync` init
// container: runUnit must run the already-populated workspace as is,
// invoking no git command of its own, and must leave what workspace-sync
// put there untouched. A fake `git` placed first on PATH that fails loudly
// if ever invoked proves the absence, rather than merely asserting the
// removed Repo/Sync symbols are gone.
func TestRunUnitNeverInvokesGit(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "git-invoked")
	script := "#!/bin/sh\necho \"git invoked: $@\" >> " + marker + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	agent, ws := newAgent(t, &scripted{replies: []Message{text("the answer")}})
	sentinel := filepath.Join(ws, "already-here.txt")
	if err := os.WriteFile(sentinel, []byte("workspace-sync put this here"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := runUnit(context.Background(), Builtins{Workspace: ws}, &MCPClient{}, agent, WorkUnit{PromptText: "q"}, t.Logf)
	if res.Status != "succeeded" {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the runtime invoked git on a run — it must clone nothing, workspace-sync already populated the workspace")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "workspace-sync put this here" {
		t.Errorf("the pre-populated workspace must survive untouched, got %q, err=%v", got, err)
	}
}

func TestLogfWritesALineToStdout(t *testing.T) {
	// logf hardcodes os.Stdout, so redirect the process's own stdout fd for the
	// call -- the only way to observe it without changing the function.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	logf("hello %s", "world")
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	if got := buf.String(); !strings.HasSuffix(got, "hello world\n") {
		t.Errorf("got %q", got)
	}
}

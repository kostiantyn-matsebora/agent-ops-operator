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

// A failed sync must fail the RUN with a reason instead of letting the loop
// carry on against a workspace that was never actually refreshed (O9) --
// `git clone` against a source path that does not exist fails immediately,
// with no network involved.
func TestRunUnitFailsTheRunWhenSyncFails(t *testing.T) {
	repo := Repo{URL: filepath.Join(t.TempDir(), "does-not-exist-agentops-repo"), Ref: "main", Workspace: t.TempDir()}
	agent, _ := newAgent(t, &scripted{})
	var logged []string
	logf := func(f string, a ...any) { logged = append(logged, f) }
	res := runUnit(context.Background(), repo, Builtins{}, &MCPClient{}, agent, WorkUnit{PromptText: "q"}, logf)
	if res.Status != "failed" || !strings.Contains(res.Result, "workspace sync failed") {
		t.Errorf("%+v", res)
	}
	if res.ExitCode != nil {
		t.Errorf("no process ran -- exitCode must be omitted, got %v", *res.ExitCode)
	}
	found := false
	for _, l := range logged {
		if strings.Contains(l, "workspace sync failed") {
			found = true
		}
	}
	if !found {
		t.Errorf("the sync failure must be logged, got %v", logged)
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

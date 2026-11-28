package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMainHelperProcess is never a real test: it only runs main() when
// re-invoked as a subprocess by runMainHelper, the standard way to exercise
// a function that calls os.Exit without taking the whole test binary down
// with it.
func TestMainHelperProcess(t *testing.T) {
	if os.Getenv("WSYNC_MAIN_HELPER") != "1" {
		t.Skip("helper process, invoked only by runMainHelper")
	}
	main()
}

// runMainHelper runs this package's main() in a subprocess under the given
// environment overrides, returning its exit code and combined output.
func runMainHelper(t *testing.T, overrides map[string]string) (exitCode int, output string) {
	t.Helper()
	env := filterEnv(os.Environ(), overrides)
	env = append(env, "WSYNC_MAIN_HELPER=1")
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainHelperProcess$")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(out)
	}
	t.Fatalf("running helper process: %v", err)
	return -1, string(out)
}

func filterEnv(env []string, overrides map[string]string) []string {
	var out []string
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if _, skip := overrides[k]; skip {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func TestRepoErrorUnwrapsToItsCause(t *testing.T) {
	cause := errors.New("boom")
	e := &repoError{repo: "app", cause: cause}
	if e.Unwrap() != cause {
		t.Fatal("Unwrap should return the wrapped cause")
	}
}

func TestAgentFileErrorUnwrapsToItsCause(t *testing.T) {
	cause := errors.New("boom")
	e := &agentFileError{repo: "app", path: "agents/main.md", cause: cause}
	if e.Unwrap() != cause {
		t.Fatal("Unwrap should return the wrapped cause")
	}
	noRepo := &agentFileError{path: "agents/main.md", cause: cause}
	if !strings.Contains(noRepo.Error(), "workspace root") {
		t.Fatalf("expected the single-repository fallback name, got %q", noRepo.Error())
	}
}

func TestPlural(t *testing.T) {
	if plural(1) != "y" {
		t.Fatal(`plural(1) should be "y"`)
	}
	if plural(2) != "ies" {
		t.Fatal(`plural(2) should be "ies"`)
	}
}

func TestMainExitsZeroWithNoRepositories(t *testing.T) {
	workspace := t.TempDir()
	code, out := runMainHelper(t, map[string]string{envWorkspaceRoot: workspace})
	if code != 0 {
		t.Fatalf("exit code %d, output: %s", code, out)
	}
}

func TestMainExitsNonZeroAndNamesTheCauseOnABadManifest(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "term-log")
	code, out := runMainHelper(t, map[string]string{
		envWorkspaceRoot:      t.TempDir(),
		envWorkspaceRepos:     "not json",
		terminationLogPathEnv: logPath,
	})
	if code == 0 {
		t.Fatalf("expected a non-zero exit, output: %s", out)
	}
	msg := mustReadFile(t, logPath)
	if !strings.Contains(msg, "manifest") {
		t.Fatalf("expected the termination message to name the cause, got %q", msg)
	}
}

func TestMainEndToEndClonesARealRepository(t *testing.T) {
	origin := newOriginRepo(t, map[string]string{"README.md": "hello from main"})
	workspace := t.TempDir()
	code, out := runMainHelper(t, map[string]string{
		envWorkspaceRoot:  workspace,
		envWorkspaceRepos: `[{"name":"app","url":"` + fileURL(t, origin) + `","ref":"main"}]`,
	})
	if code != 0 {
		t.Fatalf("exit code %d, output: %s", code, out)
	}
	if got := mustReadFile(t, filepath.Join(workspace, "README.md")); got != "hello from main" {
		t.Fatalf("got %q", got)
	}
}

func TestMainEndToEndFailureNamesRepositoryAndOmitsToken(t *testing.T) {
	const token = "end-to-end-planted-token"
	logPath := filepath.Join(t.TempDir(), "term-log")
	workspace := t.TempDir()
	badURL := fileURL(t, filepath.Join(t.TempDir(), "does-not-exist"))
	code, out := runMainHelper(t, map[string]string{
		envWorkspaceRoot:      workspace,
		envWorkspaceRepos:     `[{"name":"app","url":"` + badURL + `","ref":"main","auth":{"type":"https"}}]`,
		"REPO_CRED_0":         token,
		terminationLogPathEnv: logPath,
	})
	if code == 0 {
		t.Fatalf("expected a non-zero exit, output: %s", out)
	}
	if strings.Contains(out, token) {
		t.Fatalf("the token leaked into process output: %q", out)
	}
	msg := mustReadFile(t, logPath)
	if strings.Contains(msg, token) {
		t.Fatalf("the token leaked into the termination message: %q", msg)
	}
	if !strings.Contains(msg, "app") {
		t.Fatalf("expected the termination message to name the repository, got %q", msg)
	}
}

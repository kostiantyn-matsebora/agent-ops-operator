package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- the four env parsers: pure functions, real os.Setenv/Unsetenv --------

func TestEnv(t *testing.T) {
	t.Setenv("HK_ENV_TEST", "  value  ")
	if got := env("HK_ENV_TEST", "fallback"); got != "value" {
		t.Errorf("env with surrounding whitespace = %q, want it trimmed", got)
	}
	os.Unsetenv("HK_ENV_TEST")
	if got := env("HK_ENV_TEST", "fallback"); got != "fallback" {
		t.Errorf("env unset = %q, want the default", got)
	}
	t.Setenv("HK_ENV_TEST", "   ")
	if got := env("HK_ENV_TEST", "fallback"); got != "fallback" {
		t.Errorf("env of pure whitespace = %q, want the default", got)
	}
}

func TestEnvBool(t *testing.T) {
	cases := map[string]bool{"true": true, "1": true, "yes": true, "on": true,
		"false": false, "0": false, "no": false, "off": false, "TRUE": true, " On ": true}
	for v, want := range cases {
		t.Setenv("HK_BOOL_TEST", v)
		if got := envBool("HK_BOOL_TEST", !want); got != want {
			t.Errorf("envBool(%q) = %v, want %v", v, got, want)
		}
	}
	t.Setenv("HK_BOOL_TEST", "sideways")
	if got := envBool("HK_BOOL_TEST", true); got != true {
		t.Errorf("an unrecognised value must fall back to the default, got %v", got)
	}
}

func TestEnvInt(t *testing.T) {
	t.Setenv("HK_INT_TEST", "42")
	if got := envInt("HK_INT_TEST", 7); got != 42 {
		t.Errorf("envInt = %d, want 42", got)
	}
	t.Setenv("HK_INT_TEST", "not-a-number")
	if got := envInt("HK_INT_TEST", 7); got != 7 {
		t.Errorf("an invalid value must fall back to the default, got %d", got)
	}
	// The parser accepts only v >= 0 — a negative bound is nonsensical for a
	// per-run deletion cap, so it must be rejected the same as garbage.
	t.Setenv("HK_INT_TEST", "-3")
	if got := envInt("HK_INT_TEST", 7); got != 7 {
		t.Errorf("a negative value must fall back to the default, got %d", got)
	}
	t.Setenv("HK_INT_TEST", "0")
	if got := envInt("HK_INT_TEST", 7); got != 0 {
		t.Errorf("zero is a valid bound (unbounded), got %d", got)
	}
}

func TestEnvDuration(t *testing.T) {
	t.Setenv("HK_DUR_TEST", "48h")
	if got := envDuration("HK_DUR_TEST", time.Minute); got != 48*time.Hour {
		t.Errorf("envDuration = %v, want 48h", got)
	}
	t.Setenv("HK_DUR_TEST", "nonsense")
	if got := envDuration("HK_DUR_TEST", time.Minute); got != time.Minute {
		t.Errorf("an invalid duration must fall back to the default, got %v", got)
	}
	// A zero or negative grace period would reclaim in-flight transcripts
	// immediately, so the parser rejects both, same as a garbage string.
	t.Setenv("HK_DUR_TEST", "0s")
	if got := envDuration("HK_DUR_TEST", time.Minute); got != time.Minute {
		t.Errorf("a non-positive duration must fall back to the default, got %v", got)
	}
	t.Setenv("HK_DUR_TEST", "-5m")
	if got := envDuration("HK_DUR_TEST", time.Minute); got != time.Minute {
		t.Errorf("a negative duration must fall back to the default, got %v", got)
	}
}

// --- main(): real subprocesses, per design.md's "no mocking" rule ---------
//
// main() calls log.Fatal/os.Exit on its failure paths, which would kill the
// test process itself if called in-process. The standard reentrant pattern
// re-invokes the compiled test binary as a real child process: it is real
// main(), a real process boundary and a real exit code, never a stand-in.

const reentryEnv = "HOUSEKEEPING_TEST_REENTER_MAIN"

func TestMain(m *testing.M) {
	if os.Getenv(reentryEnv) == "1" {
		if dir := os.Getenv("HOUSEKEEPING_TEST_SADIR"); dir != "" {
			saDir = dir
		}
		main()
		os.Exit(0) // main() returned normally: no failure was recorded
	}
	os.Exit(m.Run())
}

// runMainSubprocess re-executes this test binary with reentryEnv set, so the
// TestMain hook above calls the real main() instead of running the suite.
func runMainSubprocess(t *testing.T, env map[string]string, extraSADir string) (out string, exitCode int) {
	t.Helper()

	// Build a de-duplicated environment (last write wins) rather than merely
	// appending: os.Environ() may already carry a var we mean to override
	// (or unset, via ""), and a duplicate KEY entry must not leave the
	// original value in effect.
	merged := map[string]string{}
	for _, kv := range os.Environ() {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			merged[kv[:i]] = kv[i+1:]
		}
	}
	merged[reentryEnv] = "1"
	if extraSADir != "" {
		merged["HOUSEKEEPING_TEST_SADIR"] = extraSADir
	}
	for k, v := range env {
		merged[k] = v
	}
	cmd := exec.Command(os.Args[0])
	for k, v := range merged {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	output, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("running subprocess: %v", err)
		}
	}
	return string(output), code
}

// A missing POD_NAMESPACE must be a hard, immediate failure: the job has no
// idea what to scan.
func TestMainExitsWithoutPodNamespace(t *testing.T) {
	out, code := runMainSubprocess(t, map[string]string{"POD_NAMESPACE": ""}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1: %s", code, out)
	}
	if !strings.Contains(out, "POD_NAMESPACE is required") {
		t.Fatalf("output = %q, want the missing-namespace message", out)
	}
}

// Outside a cluster (no KUBERNETES_SERVICE_HOST/PORT) construction of the
// in-cluster client fails, and main must report that and exit non-zero
// rather than silently doing nothing.
func TestMainExitsWhenNotInCluster(t *testing.T) {
	out, code := runMainSubprocess(t, map[string]string{
		"POD_NAMESPACE":           "agent-ops",
		"KUBERNETES_SERVICE_HOST": "",
		"KUBERNETES_SERVICE_PORT": "",
	}, "")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1: %s", code, out)
	}
	if !strings.Contains(out, "in-cluster client") {
		t.Fatalf("output = %q, want the in-cluster-client failure message", out)
	}
}

// With a real (if not reachable) in-cluster identity and no roots
// configured, both jobs are skipped and the run must report success — the
// "nothing to do" happy path a chart with neither PVC bound would take.
func TestMainSucceedsWithNoRootsConfigured(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), selfSignedCAPEM(t), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := runMainSubprocess(t, map[string]string{
		"POD_NAMESPACE":           "agent-ops",
		"KUBERNETES_SERVICE_HOST": "10.11.12.13",
		"KUBERNETES_SERVICE_PORT": "6443",
		"WORKSPACE_ROOT":          "",
		"SESSIONS_ROOT":           "",
	}, dir)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0: %s", code, out)
	}
	if !strings.Contains(out, "no root configured, skipping") {
		t.Fatalf("output = %q, want both jobs reported as skipped", out)
	}
	if !strings.Contains(out, "housekeeping complete") {
		t.Fatalf("output = %q, want the success line", out)
	}
}

// A root that is configured but simply does not exist yet (a claim that has
// never been written to) is a no-op, not an error, and each job still runs
// and reports a scan of zero.
func TestMainRunsJobsAgainstUnmountedRoots(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), selfSignedCAPEM(t), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "not-mounted-yet")
	out, code := runMainSubprocess(t, map[string]string{
		"POD_NAMESPACE":           "agent-ops",
		"KUBERNETES_SERVICE_HOST": "10.11.12.13",
		"KUBERNETES_SERVICE_PORT": "6443",
		"WORKSPACE_ROOT":          missing,
		"SESSIONS_ROOT":           missing,
	}, dir)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0: %s", code, out)
	}
	if !strings.Contains(out, "workspaces: scanned 0") || !strings.Contains(out, "sessions: scanned 0") {
		t.Fatalf("output = %q, want both jobs to report a clean, empty run", out)
	}
}

// When a job's scan itself fails (root exists but is not a directory —
// os.ReadDir refuses it with something other than IsNotExist), main must
// log the failure, keep running the other job, and still exit non-zero:
// one half failing must not stop the other, but must not look like success.
func TestMainExitsNonZeroWhenAJobFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), selfSignedCAPEM(t), 0o644); err != nil {
		t.Fatal(err)
	}
	notADir := filepath.Join(t.TempDir(), "regular-file")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := runMainSubprocess(t, map[string]string{
		"POD_NAMESPACE":           "agent-ops",
		"KUBERNETES_SERVICE_HOST": "10.11.12.13",
		"KUBERNETES_SERVICE_PORT": "6443",
		"WORKSPACE_ROOT":          notADir,
		"SESSIONS_ROOT":           "",
	}, dir)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1: %s", code, out)
	}
	if !strings.Contains(out, "workspaces:") {
		t.Fatalf("output = %q, want the workspaces job's error reported", out)
	}
	if strings.Contains(out, "housekeeping complete") {
		t.Fatalf("output = %q, must not claim success after a job failed", out)
	}
}

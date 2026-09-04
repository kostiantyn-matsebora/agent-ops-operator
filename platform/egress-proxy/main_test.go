package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// main() was entirely untested — it calls os.Exit on every non-trivial path,
// which would otherwise kill the test binary itself. The standard fix (used
// by the stdlib's own os/exec tests) is to re-exec the test binary as a
// subprocess with a marker env var set, so os.Exit terminates the
// subprocess, and read its exit code and output from the parent test.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("EGRESS_PROXY_WANT_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	os.Args = append([]string{"egress-proxy"}, args...)
	main()
	os.Exit(0) // reached only if main() took a path with no error
}

func runMain(t *testing.T, env []string, args ...string) (output string, exitCode int) {
	t.Helper()
	cs := append([]string{"-test.run=TestHelperProcess", "--"}, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = append(append([]string{}, os.Environ()...), "EGRESS_PROXY_WANT_HELPER=1")
	cmd.Env = append(cmd.Env, env...)
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if err == nil {
		return out.String(), 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return out.String(), exitErr.ExitCode()
	}
	t.Fatalf("running helper subprocess: %v", err)
	return "", -1
}

// Task 8.1 — with no subcommand at all, main must print usage and exit 2.
func TestMainRequiresACommand(t *testing.T) {
	out, code := runMain(t, nil)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2: %s", code, out)
	}
	if !strings.Contains(out, "usage:") {
		t.Fatalf("missing usage message: %s", out)
	}
}

func TestMainRejectsAnUnknownCommand(t *testing.T) {
	out, code := runMain(t, nil, "bogus")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1: %s", code, out)
	}
	if !strings.Contains(out, `unknown command "bogus"`) {
		t.Fatalf("missing the unknown-command message: %s", out)
	}
}

// The "install-redirect" dispatch branch, and main's error-propagation from
// it, via a flag runInstallRedirect itself rejects.
func TestMainPropagatesInstallRedirectFailure(t *testing.T) {
	out, code := runMain(t, nil, "install-redirect", "--exclude-ports=notaport")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1: %s", code, out)
	}
	if !strings.Contains(out, "egress-proxy:") {
		t.Fatalf("main must prefix the propagated error: %s", out)
	}
}

// The "proxy" dispatch branch and its error propagation, via a LISTEN_PORT
// runProxy rejects before ever calling serve — so this cannot hang.
func TestMainPropagatesProxyFailure(t *testing.T) {
	out, code := runMain(t, []string{"LISTEN_PORT=not-a-number"}, "proxy")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1: %s", code, out)
	}
	if !strings.Contains(out, "LISTEN_PORT") {
		t.Fatalf("the propagated error must name what failed: %s", out)
	}
}

// The one exit-free path: install-redirect succeeding. Real stub binaries as
// in redirect_cmd_test.go, so this is a genuine subprocess run of main()
// end to end, not just of runInstallRedirect in isolation.
func TestMainInstallRedirectSucceeds(t *testing.T) {
	dir := t.TempDir()
	logPath := dir + "/invocations.log"
	writeStub(t, dir, "iptables", logPath)
	writeStub(t, dir, "ip6tables", logPath)

	out, code := runMain(t, []string{"PATH=" + dir}, "install-redirect")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0: %s", code, out)
	}
	if _, err := os.ReadFile(logPath); err != nil {
		t.Fatalf("main never ran the real install: %v", err)
	}
}

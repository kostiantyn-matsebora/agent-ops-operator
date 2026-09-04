package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStub drops an executable shell script named `name` into dir, so that
// hasBinary/exec.LookPath find it on PATH exactly as they would find a real
// iptables. It logs its own invocation (args) to logPath, which is how the
// test observes what `run` actually executed — a real subprocess, not a
// recorded call.
func writeStub(t *testing.T, dir, name, logPath string) {
	t.Helper()
	script := "#!/bin/sh\necho \"$0 $*\" >> " + logPath + "\nexit 0\n"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatalf("writing stub %s: %v", name, err)
	}
}

// Task 8.1 — runInstallRedirect, installFamily and `run` were entirely
// unexercised: every existing redirect test drives installRedirect with an
// injected runner, never the real one, and this container ships neither
// iptables nor ip6tables so hasBinary always failed the "installed" branch.
// Stubbing both binaries on PATH lets `run` genuinely fork/exec a real
// process — no root or NET_ADMIN needed, since nothing here touches netfilter
// — and pushes execution through installFamily for both address families.
func TestRunInstallRedirectExecutesRealSubprocessesForBothFamilies(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "invocations.log")
	writeStub(t, dir, "iptables", logPath)
	writeStub(t, dir, "ip6tables", logPath)
	t.Setenv("PATH", dir)

	err := runInstallRedirect([]string{
		"--proxy-port=15001", "--proxy-uid=1337", "--exclude-ports=53,9090",
	})
	if err != nil {
		t.Fatalf("runInstallRedirect: %v", err)
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("no stub was ever invoked: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(log)), "\n")
	// redirectRules emits 4 fixed rules, one per excluded port (2 here), and
	// the final catch-all REDIRECT = 7 rules, and both families must have run
	// every one of them.
	const wantPerFamily = 7
	var v4, v6 int
	for _, l := range lines {
		switch {
		case strings.Contains(l, "/iptables "):
			v4++
		case strings.Contains(l, "/ip6tables "):
			v6++
		}
	}
	if v4 != wantPerFamily {
		t.Fatalf("iptables ran %d times, want %d: %s", v4, wantPerFamily, log)
	}
	if v6 != wantPerFamily {
		t.Fatalf("ip6tables ran %d times, want %d — IPv6 must not be skipped when the binary exists: %s", v6, wantPerFamily, log)
	}
	if !strings.Contains(string(log), fmt.Sprintf("--to-ports %d", 15001)) {
		t.Fatalf("the real rule content never reached the subprocess: %s", log)
	}
}

// The other half of the same fallback: a cluster with no IPv6 stack at all
// must install v4 and move on rather than failing, but that path was never
// reached either — the container had NEITHER binary, so installRedirect only
// ever took the "no iptables" fatal branch (fam.bin == "iptables"). Isolating
// PATH to a directory holding only the v4 binary reaches the actual
// `continue` for the missing v6 case.
func TestRunInstallRedirectToleratesAMissingIPv6Binary(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "invocations.log")
	writeStub(t, dir, "iptables", logPath)
	t.Setenv("PATH", dir)

	if err := runInstallRedirect([]string{"--proxy-port=15001", "--proxy-uid=1337"}); err != nil {
		t.Fatalf("a missing ip6tables must not fail the install: %v", err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("iptables was never invoked: %v", err)
	}
	if strings.Contains(string(log), "ip6tables") {
		t.Fatalf("ip6tables does not exist on PATH; nothing should have run it: %s", log)
	}
}

// A malformed --exclude-ports must fail BEFORE any subprocess runs — this is
// runInstallRedirect's own flag-to-parsePorts wiring, distinct from
// parsePorts' own unit test.
func TestRunInstallRedirectRejectsAMalformedExcludeList(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "invocations.log")
	writeStub(t, dir, "iptables", logPath)
	t.Setenv("PATH", dir)

	if err := runInstallRedirect([]string{"--exclude-ports=notaport"}); err == nil {
		t.Fatal("a malformed exclude list must fail runInstallRedirect")
	}
	if _, err := os.ReadFile(logPath); err == nil {
		t.Fatal("no rule should ever have been installed for a rejected flag set")
	}
}

// An unknown flag must fail flag parsing itself, the other error return
// runInstallRedirect owns.
func TestRunInstallRedirectRejectsAnUnknownFlag(t *testing.T) {
	if err := runInstallRedirect([]string{"--not-a-real-flag"}); err == nil {
		t.Fatal("an unrecognised flag must be refused")
	}
}

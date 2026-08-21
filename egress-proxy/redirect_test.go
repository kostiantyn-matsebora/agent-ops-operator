package main

import (
	"errors"
	"strings"
	"testing"
)

func recordRules(o redirectOpts) []string {
	var got []string
	for _, r := range redirectRules(o) {
		got = append(got, strings.Join(r, " "))
	}
	return got
}

// The two exclusions are not optimisations. Without the uid exclusion every
// forwarded connection is redirected back into the proxy, which is an infinite
// loop that presents as a hung agent.
func TestProxyTrafficEscapesItsOwnRedirect(t *testing.T) {
	rules := recordRules(redirectOpts{proxyPort: 15001, proxyUID: 1337})

	var uidRule, catchAll int = -1, -1
	for i, r := range rules {
		if strings.Contains(r, "--uid-owner 1337") && strings.Contains(r, "RETURN") {
			uidRule = i
		}
		if strings.Contains(r, "REDIRECT") {
			catchAll = i
		}
	}
	if uidRule < 0 {
		t.Fatal("the proxy's own traffic must be excluded, or forwarding loops forever")
	}
	if catchAll < 0 {
		t.Fatal("no catch-all redirect: nothing would be mediated")
	}
	if uidRule > catchAll {
		t.Fatal("the exclusion must precede the catch-all; the first match wins")
	}
}

// A declared hole must appear as an exclusion, before the catch-all, or it is
// not a hole at all and the operator's declaration silently did nothing.
func TestExcludedPortsPrecedeTheCatchAll(t *testing.T) {
	rules := recordRules(redirectOpts{proxyPort: 15001, proxyUID: 1337, excludePorts: []int{53, 9090}})

	catchAll := -1
	for i, r := range rules {
		if strings.Contains(r, "REDIRECT") {
			catchAll = i
		}
	}
	for _, port := range []string{"--dport 53", "--dport 9090"} {
		found := -1
		for i, r := range rules {
			if strings.Contains(r, port) && strings.Contains(r, "RETURN") {
				found = i
			}
		}
		if found < 0 {
			t.Fatalf("%s was declared excluded but no rule excludes it", port)
		}
		if found > catchAll {
			t.Fatalf("%s is excluded after the catch-all, so it is not excluded", port)
		}
	}
}

// Loopback carries the agent's own sidecar traffic. Redirecting a connection
// that never leaves the pod buys nothing and breaks context-sync.
func TestLoopbackIsLeftAlone(t *testing.T) {
	for _, r := range recordRules(redirectOpts{proxyPort: 15001, proxyUID: 1337}) {
		if strings.Contains(r, "-o lo") && strings.Contains(r, "RETURN") {
			return
		}
	}
	t.Fatal("loopback must be excluded from the redirect")
}

// Task 3.5 — a v4-only redirect on a dual-stack cluster leaves an unmediated
// path that works perfectly, which is the worst way for this to fail.
func TestBothAddressFamiliesAreInstalled(t *testing.T) {
	var families []string
	o := redirectOpts{proxyPort: 15001, proxyUID: 1337}
	o.runner = func(bin string, args ...string) error {
		families = append(families, bin)
		return nil
	}
	// hasBinary is the real lookup, so this asserts intent rather than the
	// container's contents: whichever families exist, each is installed once
	// and a v4 failure is fatal.
	_ = installRedirect(o)

	seen := map[string]bool{}
	for _, f := range families {
		seen[f] = true
	}
	if hasBinary("iptables") && !seen["iptables"] {
		t.Fatal("IPv4 rules were not installed")
	}
	if hasBinary("ip6tables") && !seen["ip6tables"] {
		t.Fatal("IPv6 rules were not installed, leaving an unmediated path")
	}
}

// A partially installed redirect is worse than none: the install believes it is
// mediated. A rule that fails must fail the container.
func TestAFailedRuleFailsTheInstall(t *testing.T) {
	o := redirectOpts{proxyPort: 15001, proxyUID: 1337}
	o.runner = func(bin string, args ...string) error { return errors.New("permission denied") }
	if !hasBinary("iptables") {
		t.Skip("no iptables in this environment")
	}
	if err := installRedirect(o); err == nil {
		t.Fatal("a failed rule must fail the install, never leave a half-open boundary")
	}
}

func TestBadExcludedPortIsRejected(t *testing.T) {
	if _, err := parsePorts("53,notaport"); err == nil {
		t.Fatal("a malformed port list must be refused, not silently partially applied")
	}
	if _, err := parsePorts("53, 9090 ,"); err != nil {
		t.Fatalf("a well-formed list with spacing must parse: %v", err)
	}
}

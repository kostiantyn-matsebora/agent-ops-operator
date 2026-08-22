package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// The redirect is what makes this a boundary rather than a setting.
//
// Everything the agent could edit — its mcp.json, its proxy environment
// variables — is configuration it can also ignore. A rule in the pod's network
// namespace holds for destinations the agent CHOOSES, which is the whole
// requirement.
//
// Two properties the rules must have, and both have bitten service meshes:
//
//   - The proxy's own forwarded traffic must escape the redirect, or every
//     connection loops back into the proxy forever. That exclusion is by UID,
//     which is why the agent must be unable to become that uid.
//   - BOTH address families must be covered. A v4-only redirect on a
//     dual-stack cluster leaves the agent an unmediated path to any service
//     with a v6 address, and it fails silently — the traffic simply works.

// redirectOpts is the agreement between this command and the proxy it serves.
type redirectOpts struct {
	proxyPort    int
	proxyUID     int
	excludePorts []int
	// runner executes a rule command. Injected so tests assert the RULES
	// rather than needing a privileged kernel.
	runner func(bin string, args ...string) error
}

func runInstallRedirect(argv []string) error {
	fs := flag.NewFlagSet("install-redirect", flag.ContinueOnError)
	port := fs.Int("proxy-port", 15001, "port the proxy listens on")
	uid := fs.Int("proxy-uid", 1337, "uid the proxy runs as, excluded from the redirect")
	exclude := fs.String("exclude-ports", "", "comma-separated destination ports left unredirected")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	ports, err := parsePorts(*exclude)
	if err != nil {
		return err
	}
	opts := redirectOpts{proxyPort: *port, proxyUID: *uid, excludePorts: ports, runner: run}
	return installRedirect(opts)
}

// installRedirect writes the rules for both address families.
//
// IPv6 failing is NOT tolerated when ip6tables exists: a partially installed
// redirect is worse than none, because the install believes it is mediated. It
// IS tolerated when the binary is absent, which is a cluster with no IPv6 at
// all rather than one where we failed to cover it.
func installRedirect(o redirectOpts) error {
	for _, fam := range []struct {
		bin  string
		name string
	}{{"iptables", "IPv4"}, {"ip6tables", "IPv6"}} {
		if !hasBinary(fam.bin) {
			if fam.bin == "iptables" {
				return fmt.Errorf("no iptables binary: the redirect cannot be installed")
			}
			continue
		}
		if err := installFamily(o, fam.bin); err != nil {
			return fmt.Errorf("%s redirect: %w", fam.name, err)
		}
	}
	return nil
}

// installFamily writes one family's rules, in the order they must be evaluated.
func installFamily(o redirectOpts, bin string) error {
	rules := redirectRules(o)
	for _, r := range rules {
		if err := o.runner(bin, r...); err != nil {
			return fmt.Errorf("%s %s: %w", bin, strings.Join(r, " "), err)
		}
	}
	return nil
}

// redirectRules is the rule set, built as data so a test can read it.
//
// Ordering is load-bearing: the exclusions must precede the catch-all, because
// the first matching rule wins and the catch-all matches everything.
func redirectRules(o redirectOpts) [][]string {
	const chain = "AGENTOPS_EGRESS"
	rules := [][]string{
		{"-t", "nat", "-N", chain},
		{"-t", "nat", "-A", "OUTPUT", "-p", "tcp", "-j", chain},

		// The proxy's own traffic. Without this every forwarded connection is
		// redirected back into the proxy, which is an infinite loop that looks
		// like a hung agent.
		{"-t", "nat", "-A", chain, "-m", "owner", "--uid-owner",
			strconv.Itoa(o.proxyUID), "-j", "RETURN"},

		// Loopback. The agent reaches its context-sync sidecar this way, and
		// redirecting a connection that never leaves the pod buys nothing.
		{"-t", "nat", "-A", chain, "-o", "lo", "-j", "RETURN"},
	}
	for _, p := range o.excludePorts {
		// Declared holes. Each one is a destination the agent reaches
		// unmediated, which is why they are explicit rather than defaulted.
		rules = append(rules, []string{"-t", "nat", "-A", chain, "-p", "tcp",
			"--dport", strconv.Itoa(p), "-j", "RETURN"})
	}
	rules = append(rules, []string{"-t", "nat", "-A", chain, "-p", "tcp",
		"-j", "REDIRECT", "--to-ports", strconv.Itoa(o.proxyPort)})
	return rules
}

func parsePorts(s string) ([]int, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []int
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		p, err := strconv.Atoi(f)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("bad excluded port %q", f)
		}
		out = append(out, p)
	}
	return out, nil
}

func hasBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func run(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

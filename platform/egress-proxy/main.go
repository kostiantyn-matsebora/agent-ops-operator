// Command egress-proxy enforces a conversation's bound tool access from inside
// the runtime pod, on traffic the agent cannot route around.
//
// Two subcommands, one binary:
//
//	install-redirect  writes the redirect rules, as a privileged init container
//	proxy             serves the redirected connections
//
// One binary because the two must agree exactly on the port and the uid, and a
// second image is a second place for that agreement to rot.
//
// WHY IT EXISTS. The tool allowlist reaches the agent as `--allowedTools` on a
// CLI running beside it, so it configures a COOPERATING agent. An agent with a
// shell can open a socket to a bound MCP server and call anything that server
// registers. This process is the wall that binds an agent that does not
// cooperate. See docs/adr/0001-bound-component-reach.md.
//
// WHAT IT IS NOT. It terminates no TLS, inspects no non-MCP byte, and holds no
// Kubernetes credential. Traffic that is not headed for a bound MCP endpoint is
// copied through untouched.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: egress-proxy (install-redirect|proxy) [flags]")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "install-redirect":
		err = runInstallRedirect(os.Args[2:])
	case "proxy":
		err = runProxy(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "egress-proxy:", err)
		os.Exit(1)
	}
}

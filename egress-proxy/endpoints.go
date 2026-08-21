package main

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// WHAT A REDIRECTED CONNECTION ACTUALLY CARRIES.
//
// The pod builder names endpoints the way a human reads them —
// `kubernetes=agentops-mcp-k8s.agent-ops.svc:8080`. What arrives here is the
// destination the KERNEL recorded before the redirect, which for a Service is
// its ClusterIP: `10.43.224.73:8080`.
//
// Comparing those two strings never matches, and the failure is silent in the
// worst possible way: an unmatched destination falls through to opaque
// forwarding, so every tool call is carried unexamined while the pod, the
// condition and the logs all report mediation as active. That is precisely
// what shipped the first time, and only a live call caught it.
//
// So endpoints are RESOLVED, indexed by address, and re-resolved when a
// destination on a known port fails to match.

type entry struct {
	key  string // the MCP server key, e.g. "kubernetes"
	host string
	port string
}

type endpoints struct {
	mu sync.RWMutex
	// spec is what was configured, kept so it can be resolved again.
	spec    []entry
	control entry
	// byAddr maps a resolved "ip:port" to its server key.
	byAddr map[string]string
	// controlAddrs are the resolved addresses of the work contract.
	controlAddrs map[string]bool
	// ports every configured endpoint listens on, so a miss on a KNOWN port
	// can force a re-resolve rather than silently becoming pass-through.
	ports    map[string]bool
	resolved time.Time
}

// parseEndpointSpec reads `key=host:port` entries, comma separated. A bare
// `host:port` is accepted with an empty key, which means tool names cannot be
// qualified — so it is logged rather than quietly enforced against.
func parseEndpointSpec(s string) []entry {
	var out []entry
	for _, raw := range strings.Split(s, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		key := ""
		addr := raw
		if i := strings.Index(raw, "="); i >= 0 {
			key, addr = raw[:i], raw[i+1:]
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			log.Printf("ignoring malformed MCP endpoint %q: %v", raw, err)
			continue
		}
		if key == "" {
			log.Printf("MCP endpoint %q has no server key; tool names cannot be qualified", raw)
		}
		out = append(out, entry{key: key, host: host, port: port})
	}
	return out
}

func newEndpoints(mcpSpec, controlURL string) *endpoints {
	e := &endpoints{spec: parseEndpointSpec(mcpSpec)}
	if host, port, err := net.SplitHostPort(hostPortOf(controlURL)); err == nil {
		e.control = entry{host: host, port: port}
	}
	e.resolve()
	return e
}

// resolve rebuilds the address index. A name that does not resolve is kept in
// the spec and retried — a Service that is briefly unresolvable must not
// downgrade enforcement to pass-through for the life of the pod.
func (e *endpoints) resolve() {
	byAddr := map[string]string{}
	ports := map[string]bool{}
	for _, en := range e.spec {
		ports[en.port] = true
		for _, addr := range addrsFor(en.host, en.port) {
			byAddr[addr] = en.key
		}
	}
	control := map[string]bool{}
	if e.control.host != "" {
		ports[e.control.port] = true
		for _, addr := range addrsFor(e.control.host, e.control.port) {
			control[addr] = true
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.byAddr, e.controlAddrs, e.ports, e.resolved = byAddr, control, ports, time.Now()
	log.Printf("resolved %d MCP address(es) and %d control address(es)", len(byAddr), len(control))
}

// addrsFor returns every address a name answers with, plus the literal form so
// a configuration given as an IP works without a lookup.
func addrsFor(host, port string) []string {
	out := []string{net.JoinHostPort(host, port)}
	ips, err := net.LookupHost(host)
	if err != nil {
		log.Printf("cannot resolve %q yet: %v", host, err)
		return out
	}
	for _, ip := range ips {
		out = append(out, net.JoinHostPort(ip, port))
	}
	return out
}

// classifyBy decides how one connection is handled, from the destination the
// kernel recorded AND the Host the request names.
//
// THE HOST HEADER IS THE RELIABLE HALF, and on some clusters the only one.
// Where kube-proxy DNATs a ClusterIP, the recorded destination is that
// ClusterIP and resolving the Service name matches it. Where the CNI does
// socket-level load balancing instead — Cilium's kube-proxy replacement, for
// one — the destination is rewritten inside connect(), BEFORE netfilter sees
// it, so what is recorded is a backend POD IP that no Service name resolves to.
//
// A live call is what found this: the address index matched nothing, every tool
// call fell through to opaque forwarding, and the pod, the condition and the
// logs all still reported mediation as active.
//
// The Host survives both, because it is what the client wrote.
func (e *endpoints) classifyBy(dst, host string) (serverKey string, isMCP, isControl bool) {
	if host != "" {
		if k, mcp, ctl := e.lookupHost(host); mcp || ctl {
			return k, mcp, ctl
		}
	}
	return e.classify(dst)
}

// lookupHost matches the hostname a request names against the configured
// endpoints, ignoring any port and any trailing cluster-domain suffix — a
// client may write `svc`, `svc.cluster.local` or the bare Service name for the
// same server.
func (e *endpoints) lookupHost(host string) (string, bool, bool) {
	h := hostOnly(host)
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, en := range e.spec {
		if sameHost(h, en.host) {
			return en.key, true, false
		}
	}
	if e.control.host != "" && sameHost(h, e.control.host) {
		return "", false, true
	}
	return "", false, false
}

// sameHost compares two names by their leading labels, so `x.ns.svc`,
// `x.ns.svc.cluster.local` and `x` all name one server.
func sameHost(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSuffix(a, ".")), strings.ToLower(strings.TrimSuffix(b, "."))
	if a == b {
		return true
	}
	return strings.HasPrefix(a, b+".") || strings.HasPrefix(b, a+".")
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// classify decides how one connection is handled from the address alone.
//
// A miss on a port some endpoint listens on triggers ONE re-resolve before
// falling through. Silently carrying a tool call because a ClusterIP changed is
// the failure this whole file exists to prevent.
func (e *endpoints) classify(dst string) (serverKey string, isMCP, isControl bool) {
	if k, mcp, ctl := e.lookup(dst); mcp || ctl {
		return k, mcp, ctl
	}
	_, port, err := net.SplitHostPort(dst)
	if err != nil {
		return "", false, false
	}
	e.mu.RLock()
	known := e.ports[port]
	stale := time.Since(e.resolved) > 5*time.Second
	e.mu.RUnlock()
	if !known || !stale {
		return "", false, false
	}
	log.Printf("unmatched destination %s on a known endpoint port; re-resolving", dst)
	e.resolve()
	return e.lookup(dst)
}

func (e *endpoints) lookup(dst string) (string, bool, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if k, ok := e.byAddr[dst]; ok {
		return k, true, false
	}
	if e.controlAddrs[dst] {
		return "", false, true
	}
	return "", false, false
}

// refreshLoop keeps the index current for the life of the pod.
func (e *endpoints) refreshLoop(every time.Duration, stop <-chan struct{}) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			e.resolve()
		case <-stop:
			return
		}
	}
}

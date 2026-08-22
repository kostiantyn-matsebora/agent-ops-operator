package runtimepod

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// Egress mediation interposes a proxy the agent's traffic cannot route around,
// so the tool access its wiring granted is enforced somewhere the agent does
// not control. See docs/adr/0001-bound-component-reach.md for why enforcement
// sits in the pod rather than at the server or on the network.
//
// The mechanism is the one service meshes use, for the reason they use it: a
// redirect installed before the agent starts holds for destinations the AGENT
// chooses, not only for destinations we configured. Anything the agent can
// edit — its mcp.json, its environment — is configuration, not a boundary.

const (
	// egressProxyUID is what the proxy container runs as, and it MUST differ
	// from the agent's uid: the redirect excludes this uid so the proxy's own
	// forwarded traffic escapes it, and that exclusion is the only thing
	// separating the two. An agent running as this uid would be unmediated.
	egressProxyUID int64 = 1337

	// egressProxyContainer and egressInitContainer are stable names. The
	// reaper, the conditions and any operator reading a pod all key on them.
	egressProxyContainer = "egress-proxy"
	egressInitContainer  = "egress-init"
)

// mediating reports whether this pod redirects the agent's egress. Both halves
// are required: a runtime that asks for it, and an image to serve it. A runtime
// declaring mediation on an install that ships no proxy image gets today's pod
// rather than a half-applied boundary.
func mediating(resolved Resolved) bool {
	return resolved.EgressMediation != nil && resolved.Config.EgressProxyImage != ""
}

// egressInitImage is the privileged image that installs the redirect, falling
// back to the proxy's own when an install ships one image for both.
func egressInitImage(cfg Config) string {
	if cfg.EgressInitImage != "" {
		return cfg.EgressInitImage
	}
	return cfg.EgressProxyImage
}

// egressInitContainerSpec builds the container that installs the redirect.
//
// It is an ORDINARY init container, not a native sidecar: it runs to completion
// before anything else starts, which is what makes the fail-closed property
// free. From the moment the agent can run, its traffic is redirected — and
// until the proxy is serving, the kernel refuses those connections. Nothing has
// to detect the not-ready case because there is no window in which traffic
// escapes.
//
// It holds the only privilege in this pod, and it is gone before the agent
// exists.
func egressInitContainerSpec(cfg Config, med *agentopsv1alpha1.EgressMediation) corev1.Container {
	yes := true
	no := false
	return corev1.Container{
		Name:  egressInitContainer,
		Image: egressInitImage(cfg),
		Args: []string{
			"install-redirect",
			"--proxy-port", strconv.Itoa(int(med.MediationPort())),
			"--proxy-uid", strconv.FormatInt(egressProxyUID, 10),
			"--exclude-ports", excludePortList(med.ExcludePorts),
		},
		SecurityContext: &corev1.SecurityContext{
			// Root and NET_ADMIN are what writing the rules costs. Both are
			// confined to this container, which has exited before the agent
			// container starts.
			RunAsUser:                ptr(int64(0)),
			RunAsNonRoot:             &no,
			AllowPrivilegeEscalation: &yes,
			Capabilities: &corev1.Capabilities{
				Add:  []corev1.Capability{"NET_ADMIN", "NET_RAW"},
				Drop: []corev1.Capability{"ALL"},
			},
		},
	}
}

// egressProxyContainerSpec builds the enforcing proxy.
//
// A NATIVE sidecar for the same two reasons context-sync is one: it starts
// before the agent, and it terminates when the agent exits so the pod can still
// reach Succeeded.
//
// It receives NO Kubernetes credential and NO allowlist. The access decision
// arrives on the work unit it is already forwarding — see the module's README
// and design decision D3 — which is what keeps enforcement and configuration
// from ever drifting apart.
func egressProxyContainerSpec(cfg Config, med *agentopsv1alpha1.EgressMediation,
	endpoints []string, controlURL string) corev1.Container {

	always := corev1.ContainerRestartPolicyAlways
	no := false
	yes := true
	c := corev1.Container{
		Name:          egressProxyContainer,
		Image:         cfg.EgressProxyImage,
		RestartPolicy: &always,
		Args:          []string{"proxy"},
		Env: []corev1.EnvVar{
			{Name: "LISTEN_PORT", Value: strconv.Itoa(int(med.MediationPort()))},
			// The endpoints its wiring bound. Everything else is forwarded as
			// opaque bytes, so this list is what separates "enforce" from
			// "carry".
			{Name: "MCP_ENDPOINTS", Value: strings.Join(endpoints, ",")},
			// Where the work contract really lives, so the proxy can forward
			// what it reads the access decision from.
			{Name: "CONTROL_URL", Value: controlURL},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsUser:                ptr(egressProxyUID),
			RunAsGroup:               ptr(egressProxyUID),
			RunAsNonRoot:             &yes,
			AllowPrivilegeEscalation: &no,
			Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
		},
	}
	if med.Resources != nil {
		c.Resources = *med.Resources
	}
	return c
}

// hardenAgentContainer pins the properties interception depends on.
//
// Interception tells the agent from the proxy by UID. That distinction is only
// worth something while the agent cannot become another uid, so this is not
// hardening in the general sense — it is the mechanism. Without it an agent
// with a shell runs as the proxy's uid and its traffic is excluded from the
// redirect by the very rule meant to contain it.
//
// The pod already runs non-root at a fixed uid. What was incidental becomes
// load-bearing here, which is why it is stated on the container and pinned by
// test rather than left to the pod default.
func hardenAgentContainer(c *corev1.Container) {
	no := false
	yes := true
	c.SecurityContext = &corev1.SecurityContext{
		RunAsUser:                ptr(runtimeUID),
		RunAsGroup:               ptr(runtimeUID),
		RunAsNonRoot:             &yes,
		AllowPrivilegeEscalation: &no,
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
	}
}

// excludePortList renders the unredirected ports for the init container.
//
// Every port here is reachable by the agent UNMEDIATED. It is a hole in the
// boundary by construction, which is why it is passed explicitly rather than
// defaulted to something convenient.
func excludePortList(ports []int32) string {
	if len(ports) == 0 {
		return ""
	}
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, strconv.Itoa(int(p)))
	}
	return strings.Join(out, ",")
}

// mcpEndpoints lists `<serverKey>=<host:port>` for every MCP server the
// conversation's wiring bound, which is what the proxy enforces on.
//
// The KEY travels with the address because enforcement happens in the WIRING's
// vocabulary: a server calls its tool `pods_exec`, a toolset calls it
// `mcp__kubernetes__pods_exec`, and only the key bridges the two. Without it
// the proxy would have to take toolset patterns apart to guess, and patterns
// are opaque by contract.
func mcpEndpoints(servers map[string]string) []string {
	out := make([]string, 0, len(servers))
	for key, url := range servers {
		if hp := hostPort(url); hp != "" {
			out = append(out, key+"="+hp)
		}
	}
	sortStrings(out)
	return out
}

// hostPort reduces an endpoint URL to the host:port a redirected connection
// arrives with, defaulting the port from the scheme.
func hostPort(raw string) string {
	rest := raw
	scheme := ""
	if i := strings.Index(rest, "://"); i >= 0 {
		scheme, rest = rest[:i], rest[i+3:]
	}
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	if rest == "" {
		return ""
	}
	if strings.Contains(rest, ":") {
		return rest
	}
	switch scheme {
	case "https":
		return rest + ":443"
	case "http", "":
		return rest + ":80"
	}
	return fmt.Sprintf("%s:80", rest)
}

func ptr[T any](v T) *T { return &v }

// sortStrings keeps the rendered pod stable across reconciles: a map iteration
// order leaking into a pod spec is a permanent diff and a permanent rewrite.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

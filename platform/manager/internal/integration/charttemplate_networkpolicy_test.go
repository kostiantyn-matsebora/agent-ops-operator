package integration

import (
	"strings"
	"testing"
)

// Network restriction is off unless asked for. On a cluster that DOES enforce,
// a policy missing one flow breaks it, and that is not a surprise to hand an
// install by default.
func TestNetworkPolicyIsOptIn(t *testing.T) {
	if out := helmTemplate(t); strings.Contains(out, "kind: NetworkPolicy") {
		t.Fatal("no policy may render by default")
	}
	on := helmTemplate(t, "--set", "global.agentops.networkPolicy.enabled=true")
	if !strings.Contains(on, "kind: NetworkPolicy") {
		t.Fatal("enabling it must render policies")
	}
}

// Task 5.4 — every wired flow is pinned, so tightening the policy later cannot
// silently sever one. Each entry is a flow that, if dropped, breaks a lane an
// adopter is using right now.
func TestEveryWiredFlowIsAllowed(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.networkPolicy.enabled=true",
		"--set", "kubernetes.enabled=true")

	policies := splitPolicies(out)

	manager, ok := policies["agentops-manager"]
	if !ok {
		t.Fatal("the manager has no policy, so its unauthenticated work contract stays open")
	}
	for _, caller := range []string{
		"agentops-runtime",        // /work, /work/done — the whole dispatch lane
		"agentops-adapter",        // channel ops and inbound
		"agentops-signal-adapter", // /signal/inbound — no signal reaches the manager without it
	} {
		if !strings.Contains(manager, caller) {
			t.Errorf("the manager policy does not admit %q; that lane breaks when policy is enforced", caller)
		}
	}

	// The adapters' policy is the BUNDLE's, because only it knows their ports.
	tg := helmTemplate(t,
		"--set", "global.agentops.networkPolicy.enabled=true",
		"--set", "telegram.enabled=true")
	tgPolicies := splitPolicies(tg)
	for _, name := range []string{"agentops-telegram-adapters", "agentops-telegram-signal-adapter"} {
		p, ok := tgPolicies[name]
		if !ok {
			t.Errorf("%s policy missing; the router's push lane is unprotected", name)
			continue
		}
		if !strings.Contains(p, "agentops-gateway-telegram") {
			t.Errorf("%s must admit the router, or chat stops when policy is enforced", name)
		}
	}

	mcp, ok := policies["agentops-mcp-k8s"]
	if !ok {
		t.Fatal("the MCP server has no policy, which is the exposure this change exists to close")
	}
	if !strings.Contains(mcp, "agentops-runtime") {
		t.Error("runtime pods must still reach the MCP server, or agents lose every tool")
	}
}

// Runtime pods serve nobody. An empty ingress list is the whole policy, and it
// is what stops a compromised pod being reachable from elsewhere.
func TestRuntimePodsAcceptNothing(t *testing.T) {
	out := helmTemplate(t, "--set", "global.agentops.networkPolicy.enabled=true")
	rt, ok := splitPolicies(out)["agentops-runtime"]
	if !ok {
		t.Fatal("no runtime policy")
	}
	if !strings.Contains(rt, "ingress: []") {
		t.Fatalf("runtime pods must accept nothing:\n%s", rt)
	}
}

// A collector in another namespace is invisible to a policy that does not name
// it. Enabling restriction without naming one silences monitoring, so the value
// exists and NOTES.txt says so.
func TestMetricsCallerCanBeNamed(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.networkPolicy.enabled=true",
		"--set-json", `global.agentops.networkPolicy.metricsFrom=[{"namespaceSelector":{"matchLabels":{"kubernetes.io/metadata.name":"metrics-ns"}}}]`)

	mgr := splitPolicies(out)["agentops-manager"]
	if !strings.Contains(mgr, "metrics-ns") {
		t.Fatalf("a named metrics caller must appear in the policy:\n%s", mgr)
	}
	if !strings.Contains(mgr, "9090") {
		t.Fatal("the metrics port must be opened to it")
	}
}

// An ingress controller lives outside the namespace, so the console needs its
// own key. Losing the console is the most visible way to get this wrong.
func TestConsoleCallerCanBeNamed(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.networkPolicy.enabled=true",
		"--set-json", `global.agentops.networkPolicy.consoleFrom=[{"namespaceSelector":{"matchLabels":{"kubernetes.io/metadata.name":"ingress-ns"}}}]`)

	if c, ok := splitPolicies(out)["agentops-console"]; !ok || !strings.Contains(c, "ingress-ns") {
		t.Fatalf("the console policy must admit the named ingress controller:\n%s", c)
	}
}

// The bundles own their MCP server workloads, and a subchart reads no parent
// scope but global. This pins that the one switch reaches them.
func TestBundleMCPServersAreCoveredByTheSameSwitch(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.networkPolicy.enabled=true",
		"--set", "home-assistant.enabled=true",
		"--set", "home-assistant.homeAssistant.endpoint=http://ha:8123",
		"--set", "home-assistant.homeAssistant.credentials.operatorToken=t",
		"--set", "home-assistant.adminMcpServer.enabled=true",
		"--set", "home-assistant.adminMcp.enabled=true")

	ha, ok := splitPolicies(out)["agentops-mcp-ha"]
	if !ok {
		t.Fatal("the HA MCP server holds an operator token and must be covered")
	}
	if !strings.Contains(ha, "agentops-runtime") {
		t.Error("runtime pods must still reach it")
	}
}

// splitPolicies indexes rendered NetworkPolicy documents by name.
func splitPolicies(out string) map[string]string {
	found := map[string]string{}
	for _, doc := range strings.Split(out, "\n---") {
		if !strings.Contains(doc, "kind: NetworkPolicy") {
			continue
		}
		for _, line := range strings.Split(doc, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name: ") {
				found[strings.TrimPrefix(line, "name: ")] = doc
				break
			}
		}
	}
	return found
}

// Mediation is ON by default now — the wall that constrains an uncooperative
// agent must not be something an operator has to discover — and a runtime can
// DECLINE it, getting back exactly the pod it had.
//
// `false` is a zero value, which is precisely what a naive merge would drop, so
// the opt-out is the half worth pinning.
func TestEgressMediationIsOptOut(t *testing.T) {
	on := helmTemplate(t, "--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x")
	if !strings.Contains(on, "egressMediation:") {
		t.Fatal("mediation must be declared on the default runtime")
	}

	off := helmTemplate(t,
		"--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x",
		"--set", "claude.egressMediation.enabled=false")
	if strings.Contains(off, "egressMediation:") && !strings.Contains(off, "description:") {
		t.Fatal("a runtime declining mediation must render no stanza")
	}
	if strings.Contains(off, "EGRESS_PROXY_IMAGE") {
		t.Fatal("the manager must not be told a proxy image when nothing asked for mediation")
	}
}

// An EMPTY stanza is null to the API server, and null means ABSENT — so a
// values switch that rendered nothing would report itself enabled while
// changing nothing at all. This is the one failure mode of this feature that is
// completely silent, which is why it is pinned.
func TestEnabledMediationRendersANonEmptyStanza(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x")

	rt := runtimeDoc(t, out)
	if !strings.Contains(rt, "egressMediation:") {
		t.Fatal("the runtime must carry the stanza when mediation is enabled")
	}
	if !strings.Contains(rt, "port: 15001") {
		t.Fatalf("the stanza must carry a field, or it is null and means OFF:\n%s", rt)
	}
	if !strings.Contains(out, "EGRESS_PROXY_IMAGE") {
		t.Fatal("the manager needs the proxy image, or it builds today's pod regardless")
	}
}

// A declared hole reaches the CR. An excluded port that vanished in rendering
// would be a boundary the operator believes is narrower than it is — here, one
// they believe is WIDER, which breaks their traffic instead.
func TestExcludedPortsReachTheRuntimeCR(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.runtimeDefaults.egressMediation.excludePorts={53}",
		"--set", "global.agentops.runtimeDefaults.credentialsSecret.token=x")

	if rt := runtimeDoc(t, out); !strings.Contains(rt, "- 53") {
		t.Fatalf("the excluded port must reach the CR:\n%s", rt)
	}
}

// THE POLICIES MUST FOLLOW THE VALUES, NOT THE AUTHOR'S CLUSTER.
//
// Every port and every name a policy matches on is configurable, and a policy
// that hardcoded one would not fail the render — it would select nothing, or
// open nothing, on an install that moved it. Silent either way: an unprotected
// component, or a severed lane.
//
// So this renders with NOTHING left at its default and asserts each policy
// followed. Two defaults agreeing is what hid the console bug: the label was
// read from the wrong key and matched anyway.
func TestPoliciesFollowConfiguredPortsAndNames(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.networkPolicy.enabled=true",
		"--set", "console.name=my-console",
		"--set", "console.port=9443",
		"--set", "telegram.enabled=true",
		"--set", "telegram.channelAdapter.name=tg-chan",
		"--set", "telegram.channelAdapter.port=9001",
		"--set", "telegram.signalAdapter.name=tg-sig",
		"--set", "telegram.signalAdapter.port=9002",
		"--set", "kubernetes.enabled=true",
		"--set", "kubernetes.mcpServers.port=9300")

	policies := splitPolicies(out)

	console, ok := policies["agentops-console"]
	if !ok {
		t.Fatal("no console policy")
	}
	// The pod label carries the ADAPTER name. Reading the Channel name instead
	// selects nothing and leaves the console open while looking correct.
	if !strings.Contains(console, "agentops.dev/adapter: my-console") {
		t.Errorf("console policy does not select the configured adapter name:\n%s", console)
	}
	if !strings.Contains(console, "port: 9443") {
		t.Errorf("console policy does not use the configured port:\n%s", console)
	}

	for name, want := range map[string]string{
		"agentops-telegram-adapters":       "9001",
		"agentops-telegram-signal-adapter": "9002",
		"agentops-mcp-k8s":                 "9300",
	} {
		p, ok := policies[name]
		if !ok {
			t.Errorf("%s policy missing", name)
			continue
		}
		if !strings.Contains(p, "port: "+want) {
			t.Errorf("%s does not use its configured port %s:\n%s", name, want, p)
		}
	}
	if p := policies["agentops-telegram-adapters"]; !strings.Contains(p, "tg-chan") {
		t.Errorf("the channel adapter policy does not follow its configured name:\n%s", p)
	}
	if p := policies["agentops-telegram-signal-adapter"]; !strings.Contains(p, "tg-sig") {
		t.Errorf("the signal adapter policy does not follow its configured name:\n%s", p)
	}
}

// An adapter's port is declared by the bundle that ships it, so the bundle
// renders its own policy. The parent must not ship a generic adapter policy
// with a guessed port.
func TestParentShipsNoGuessedAdapterPolicy(t *testing.T) {
	out := helmTemplate(t, "--set", "global.agentops.networkPolicy.enabled=true")
	if _, ok := splitPolicies(out)["agentops-adapters"]; ok {
		t.Fatal("the parent cannot know an adapter's port; that policy belongs to the bundle that declares it")
	}
}

// The metrics MCP server is the third one, and was the last unprotected. It
// authenticates nobody, so without a policy any pod can query the whole metrics
// backend through it.
func TestPrometheusMCPServerIsRestricted(t *testing.T) {
	out := helmTemplate(t,
		"--set", "global.agentops.networkPolicy.enabled=true",
		"--set", "prometheus.enabled=true",
		"--set", "prometheus.mcp.enabled=true",
		"--set", "prometheus.mcpServers.enabled=true",
		"--set", "prometheus.mcpServers.backend=http://vm:8428")

	p, ok := splitPolicies(out)["agentops-mcp-prometheus"]
	if !ok {
		t.Fatal("the metrics MCP server has no policy")
	}
	if !strings.Contains(p, "agentops-runtime") {
		t.Error("runtime pods must still reach it, or agents lose their metrics tools")
	}
}

// AN EMPTY SENDER LIST LEAVES THE WEBHOOK ADAPTER OPEN, ON PURPOSE.
//
// A policy selecting the adapter and naming nobody denies the alert lane —
// silently, and discovered during an incident. Under-restricting is the
// recoverable mistake, so the policy renders only once a sender is named.
func TestWebhookAdapterIsOnlyRestrictedOnceTheSenderIsNamed(t *testing.T) {
	base := []string{
		"--set", "global.agentops.networkPolicy.enabled=true",
		"--set", "prometheus.enabled=true",
	}
	if _, ok := splitPolicies(helmTemplate(t, base...))["agentops-signal-alertmanager"]; ok {
		t.Fatal("with no sender named, the adapter must be left reachable rather than cut off")
	}

	named := helmTemplate(t, append(base, "--set-json",
		`prometheus.alertmanager.webhookFrom=[{"namespaceSelector":{"matchLabels":{"kubernetes.io/metadata.name":"monitoring"}}}]`)...)
	p, ok := splitPolicies(named)["agentops-signal-alertmanager"]
	if !ok {
		t.Fatal("naming the sender must restrict the adapter to it")
	}
	if !strings.Contains(p, "monitoring") {
		t.Errorf("the named sender must appear in the policy:\n%s", p)
	}
}

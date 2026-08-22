package main

import "testing"

// The bug a live call caught: the builder passes `key=host:port`, and treating
// the whole string as an address means nothing ever matches — so every tool
// call is carried unexamined while everything reports mediation as active.
func TestEndpointSpecSeparatesKeyFromAddress(t *testing.T) {
	got := parseEndpointSpec("kubernetes=agentops-mcp-k8s.agent-ops.svc:8080,homeassistant=ha.svc:8086")
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d: %+v", len(got), got)
	}
	if got[0].key != "kubernetes" || got[0].host != "agentops-mcp-k8s.agent-ops.svc" || got[0].port != "8080" {
		t.Fatalf("first entry parsed wrong: %+v", got[0])
	}
	if got[1].key != "homeassistant" || got[1].port != "8086" {
		t.Fatalf("second entry parsed wrong: %+v", got[1])
	}
}

func TestMalformedEndpointsAreDroppedNotGuessed(t *testing.T) {
	if got := parseEndpointSpec("kubernetes=no-port-here,,   "); len(got) != 0 {
		t.Fatalf("a malformed endpoint must be dropped, got %+v", got)
	}
}

// The second half of the same bug: a redirected connection arrives with the
// ClusterIP the kernel recorded, never the DNS name. An index built only from
// names matches nothing.
func TestAnIPDestinationMatchesANamedEndpoint(t *testing.T) {
	e := &endpoints{spec: []entry{{key: "kubernetes", host: "127.0.0.1", port: "8080"}}}
	e.resolve()

	key, isMCP, isControl := e.classify("127.0.0.1:8080")
	if !isMCP || isControl {
		t.Fatalf("the resolved address must classify as MCP, got mcp=%v control=%v", isMCP, isControl)
	}
	if key != "kubernetes" {
		t.Fatalf("the server key must survive resolution, got %q", key)
	}
}

// The control stream has to be recognised too, or the policy is never learned
// and mediation fails closed on everything.
func TestControlAddressIsRecognised(t *testing.T) {
	e := newEndpoints("", "http://127.0.0.1:8080")
	_, isMCP, isControl := e.classify("127.0.0.1:8080")
	if isMCP || !isControl {
		t.Fatalf("want control, got mcp=%v control=%v", isMCP, isControl)
	}
}

// Anything else is carried, which is the common case and must stay so.
func TestUnknownDestinationsArePassedThrough(t *testing.T) {
	e := &endpoints{spec: []entry{{key: "kubernetes", host: "127.0.0.1", port: "8080"}}}
	e.resolve()
	if _, isMCP, isControl := e.classify("140.82.121.4:443"); isMCP || isControl {
		t.Fatal("unrelated traffic must not be classified as MCP or control")
	}
}

// THE FAILURE A LIVE CALL FOUND, and the reason the Host header is consulted.
//
// Where the CNI does socket-level load balancing (Cilium's kube-proxy
// replacement), the destination is rewritten inside connect() before netfilter
// sees it — so SO_ORIGINAL_DST yields a BACKEND POD IP that no Service name
// resolves to. Address matching alone then matches nothing, every tool call is
// carried unexamined, and the pod, the condition and the logs all still report
// mediation as active. That is the worst shape a security control can fail in.
func TestBackendPodIPIsStillAttributedByHost(t *testing.T) {
	e := &endpoints{spec: []entry{
		{key: "kubernetes", host: "agentops-mcp-k8s.agent-ops.svc", port: "8080"},
	}}
	e.resolve()

	// What the kernel recorded: a pod IP, nothing like the ClusterIP.
	const podIP = "10.42.4.217:8080"
	if _, isMCP, _ := e.classify(podIP); isMCP {
		t.Fatal("precondition: the address alone must not match, or this test proves nothing")
	}

	key, isMCP, _ := e.classifyBy(podIP, "agentops-mcp-k8s.agent-ops.svc:8080")
	if !isMCP {
		t.Fatal("the Host must attribute the connection when the address cannot")
	}
	if key != "kubernetes" {
		t.Fatalf("server key = %q", key)
	}
}

// A client may write the short name, the svc form or the full cluster domain
// for one server. All three must attribute to it.
func TestHostFormsAllNameTheSameServer(t *testing.T) {
	e := &endpoints{spec: []entry{{key: "kubernetes", host: "agentops-mcp-k8s.agent-ops.svc", port: "8080"}}}
	e.resolve()

	for _, host := range []string{
		"agentops-mcp-k8s.agent-ops.svc:8080",
		"agentops-mcp-k8s.agent-ops.svc.cluster.local:8080",
		"agentops-mcp-k8s.agent-ops.svc.",
	} {
		if _, isMCP, _ := e.classifyBy("10.42.4.217:8080", host); !isMCP {
			t.Errorf("host %q did not attribute to its server", host)
		}
	}
}

// The work contract needs the same treatment, or the policy is never learned
// and mediation refuses everything.
func TestControlIsAttributedByHostToo(t *testing.T) {
	e := newEndpoints("", "http://agentops-manager.agent-ops.svc.cluster.local:8080")
	_, isMCP, isControl := e.classifyBy("10.42.0.190:8080", "agentops-manager.agent-ops.svc:8080")
	if isMCP || !isControl {
		t.Fatalf("want control, got mcp=%v control=%v", isMCP, isControl)
	}
}

// An unrelated host on a shared port must NOT be attributed to an MCP server
// just because the port matches.
func TestAnUnrelatedHostOnAKnownPortIsNotClaimed(t *testing.T) {
	e := &endpoints{spec: []entry{{key: "kubernetes", host: "agentops-mcp-k8s.agent-ops.svc", port: "8080"}}}
	e.resolve()
	if _, isMCP, isControl := e.classifyBy("10.42.9.9:8080", "some-other-service.other-ns.svc:8080"); isMCP || isControl {
		t.Fatal("a different host on the same port must be carried, not enforced against")
	}
}

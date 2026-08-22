package runtimepod

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/mcpcompile"
)

func medCfg() Config {
	return Config{
		Image: "runtime:1", ServiceAccount: "sa", ControlURL: "http://manager:8080",
		HomePVC: "agentops-home", EgressProxyImage: "egress-proxy:1",
	}
}

func initContainer(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == name {
			return &pod.Spec.InitContainers[i]
		}
	}
	return nil
}

func buildMediated(t *testing.T, cfg Config, med *agentopsv1alpha1.EgressMediation,
	endpoints map[string]string) *corev1.Pod {
	t.Helper()
	return Build(conversation("c1"), &agentopsv1alpha1.AgentProfile{},
		mcpcompile.Result{Endpoints: endpoints}, "mcp-cm",
		Resolved{Config: cfg, EgressMediation: med})
}

// Task 1.3 — the absent case is not "roughly the same pod", it is the same pod.
// An install that never asked for mediation must not discover it through an
// upgrade, so this pins the whole shape rather than one field.
func TestNoMediationBuildsTodaysPod(t *testing.T) {
	pod := build("c1", medCfg()) // Resolved.EgressMediation nil

	if len(pod.Spec.InitContainers) != 0 {
		t.Fatalf("no containers may be added without the stanza; got %d", len(pod.Spec.InitContainers))
	}
	if len(pod.Spec.Containers) != 1 || pod.Spec.Containers[0].Name != "worker" {
		t.Fatal("the pod must hold the worker alone")
	}
	if pod.Spec.Containers[0].SecurityContext != nil {
		t.Fatal("the worker's security context must stay at the pod default when nothing asked for mediation")
	}
	if got := envOf(&pod.Spec.Containers[0], "CONTROL_URL"); got != "http://manager:8080" {
		t.Fatalf("CONTROL_URL = %q, want the manager directly", got)
	}
}

// A runtime asking for mediation on an install that ships no proxy image gets
// today's pod, not a half-applied boundary — the redirect without a proxy is a
// pod that cannot reach anything.
func TestMediationWithoutAnImageIsNotHalfApplied(t *testing.T) {
	cfg := medCfg()
	cfg.EgressProxyImage = ""
	pod := buildMediated(t, cfg, &agentopsv1alpha1.EgressMediation{}, nil)

	if len(pod.Spec.InitContainers) != 0 {
		t.Fatal("without an image there must be neither redirect nor proxy")
	}
}

// Task 3.1 — the privilege interception costs is confined to a container that
// has exited before the agent exists.
func TestInterceptionPrivilegeIsConfinedToStartup(t *testing.T) {
	pod := buildMediated(t, medCfg(), &agentopsv1alpha1.EgressMediation{}, nil)

	init := initContainer(pod, egressInitContainer)
	if init == nil {
		t.Fatal("no redirect installer in the pod")
	}
	if init.RestartPolicy != nil {
		t.Fatal("the installer must run to completion, not stay alive as a sidecar")
	}
	var addsNetAdmin bool
	for _, c := range init.SecurityContext.Capabilities.Add {
		if c == "NET_ADMIN" {
			addsNetAdmin = true
		}
	}
	if !addsNetAdmin {
		t.Fatal("the installer needs NET_ADMIN to write the rules")
	}

	worker := container(pod, "worker")
	if worker.SecurityContext == nil || worker.SecurityContext.Capabilities == nil {
		t.Fatal("the agent container must state its capabilities under mediation")
	}
	for _, c := range worker.SecurityContext.Capabilities.Add {
		t.Fatalf("the agent must hold no added capability; got %q", c)
	}
	proxy := initContainer(pod, egressProxyContainer)
	if proxy == nil || proxy.SecurityContext.Capabilities == nil {
		t.Fatal("the proxy must state its capabilities")
	}
	for _, c := range proxy.SecurityContext.Capabilities.Add {
		t.Fatalf("the proxy needs no capability; got %q", c)
	}
}

// Task 3.2 — the ordering IS the fail-closed property. The redirect must be
// installed by a container that completes before anything long-running starts.
func TestRedirectIsInstalledBeforeAnythingRuns(t *testing.T) {
	pod := buildMediated(t, medCfg(), &agentopsv1alpha1.EgressMediation{}, nil)

	if len(pod.Spec.InitContainers) < 2 {
		t.Fatalf("want installer and proxy; got %d init containers", len(pod.Spec.InitContainers))
	}
	if pod.Spec.InitContainers[0].Name != egressInitContainer {
		t.Fatalf("the redirect must be installed first; first container is %q",
			pod.Spec.InitContainers[0].Name)
	}
	proxy := initContainer(pod, egressProxyContainer)
	if proxy.RestartPolicy == nil || *proxy.RestartPolicy != corev1.ContainerRestartPolicyAlways {
		t.Fatal("the proxy must be a native sidecar: started before the agent, ended with it")
	}
}

// Task 3.3 — soundness rests on the agent being unable to become the proxy's
// identity. This is the mechanism, not general hardening, so it is pinned.
func TestAgentCannotAssumeTheProxyIdentity(t *testing.T) {
	pod := buildMediated(t, medCfg(), &agentopsv1alpha1.EgressMediation{}, nil)

	worker := container(pod, "worker")
	sc := worker.SecurityContext
	if sc == nil || sc.RunAsUser == nil || *sc.RunAsUser != runtimeUID {
		t.Fatal("the agent must run as its own fixed uid")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Fatal("privilege escalation must be denied, or the uid distinction buys nothing")
	}
	if sc.Capabilities == nil || len(sc.Capabilities.Drop) == 0 || sc.Capabilities.Drop[0] != "ALL" {
		t.Fatal("the agent must drop all capabilities, including any permitting a uid change")
	}

	proxy := initContainer(pod, egressProxyContainer)
	if proxy.SecurityContext.RunAsUser == nil || *proxy.SecurityContext.RunAsUser != egressProxyUID {
		t.Fatal("the proxy must run as its own uid")
	}
	if *proxy.SecurityContext.RunAsUser == runtimeUID {
		t.Fatal("the two identities must differ; the redirect excludes one of them")
	}
}

// Task 3.4 — the proxy is told which destinations it enforces on. Everything
// else it carries, so this list is the difference between the two.
func TestBoundEndpointsReachTheProxy(t *testing.T) {
	pod := buildMediated(t, medCfg(), &agentopsv1alpha1.EgressMediation{}, map[string]string{
		"kubernetes":    "http://agentops-mcp-k8s.agent-ops.svc:8080/mcp",
		"homeassistant": "http://agentops-mcp-ha.agent-ops.svc:8086/mcp",
	})

	proxy := initContainer(pod, egressProxyContainer)
	got := envOf(proxy, "MCP_ENDPOINTS")
	for _, want := range []string{
		"agentops-mcp-k8s.agent-ops.svc:8080",
		"agentops-mcp-ha.agent-ops.svc:8086",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("MCP_ENDPOINTS = %q, missing %q", got, want)
		}
	}
	if envOf(proxy, "CONTROL_URL") != "http://manager:8080" {
		t.Fatal("the proxy must know the manager's URL to recognise the work contract")
	}
}

// A rendered pod that differs between reconciles is a permanent rewrite. Map
// iteration order is the usual cause, so the endpoint list is sorted.
func TestEndpointListIsStable(t *testing.T) {
	endpoints := map[string]string{"b": "http://b:1", "a": "http://a:2", "c": "http://c:3"}
	first := envOf(initContainer(buildMediated(t, medCfg(), &agentopsv1alpha1.EgressMediation{}, endpoints), egressProxyContainer), "MCP_ENDPOINTS")
	for i := 0; i < 20; i++ {
		again := envOf(initContainer(buildMediated(t, medCfg(), &agentopsv1alpha1.EgressMediation{}, endpoints), egressProxyContainer), "MCP_ENDPOINTS")
		if again != first {
			t.Fatalf("endpoint order is unstable: %q then %q", first, again)
		}
	}
}

// The port default has to hold for a stanza built in Go, not only for one that
// passed through the API server's defaulting.
func TestPortDefaultsWithoutTheAPIServer(t *testing.T) {
	pod := buildMediated(t, medCfg(), &agentopsv1alpha1.EgressMediation{}, nil)
	proxy := initContainer(pod, egressProxyContainer)
	if got := envOf(proxy, "LISTEN_PORT"); got != "15001" {
		t.Fatalf("LISTEN_PORT = %q, want the documented default", got)
	}

	pod = buildMediated(t, medCfg(), &agentopsv1alpha1.EgressMediation{Port: 15999}, nil)
	if got := envOf(initContainer(pod, egressProxyContainer), "LISTEN_PORT"); got != "15999" {
		t.Fatalf("LISTEN_PORT = %q, want the override", got)
	}
}

// Excluded ports are a hole in the boundary by construction, so they must reach
// the installer verbatim rather than being quietly dropped.
func TestExcludedPortsReachTheInstaller(t *testing.T) {
	pod := buildMediated(t, medCfg(),
		&agentopsv1alpha1.EgressMediation{ExcludePorts: []int32{53, 9090}}, nil)

	args := strings.Join(initContainer(pod, egressInitContainer).Args, " ")
	if !strings.Contains(args, "53,9090") {
		t.Fatalf("installer args = %q, want the excluded ports", args)
	}
}

// Mediation and context-sync are independent opt-ins that must compose: the
// sidecar keeps its ordering guarantees and the redirect still precedes it.
func TestMediationComposesWithContextSync(t *testing.T) {
	cfg := medCfg()
	cfg.ContextSyncImage = "context-sync:1"
	pod := Build(conversation("c1"), &agentopsv1alpha1.AgentProfile{}, mcpcompile.Result{}, "mcp-cm",
		Resolved{Config: cfg, ContextSync: syncSpec(), EgressMediation: &agentopsv1alpha1.EgressMediation{}})

	if pod.Spec.InitContainers[0].Name != egressInitContainer {
		t.Fatal("the redirect must still be installed before the sidecar starts")
	}
	if initContainer(pod, "context-sync") == nil {
		t.Fatal("context-sync must survive mediation being enabled")
	}
	if initContainer(pod, egressProxyContainer) == nil {
		t.Fatal("the proxy must survive context-sync being enabled")
	}
}

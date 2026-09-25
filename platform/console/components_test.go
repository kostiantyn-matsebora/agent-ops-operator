package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// The Components and Infrastructure facts are read from what the cluster
// holds — labels, specs, pods — and nothing is added that no object says.

func labelled(o *Object, labels map[string]string) *Object {
	o.Metadata.Labels = labels
	return o
}

func findComponent(topo Topology, id string) *Component {
	for i := range topo.Components {
		if topo.Components[i].ID == id {
			return &topo.Components[i]
		}
	}
	return nil
}

func findPod(topo Topology, name string) *Pod {
	for i := range topo.Pods {
		if topo.Pods[i].Name == name {
			return &topo.Pods[i]
		}
	}
	return nil
}

func hasComponentEdge(topo Topology, from, to, kind string) bool {
	for _, e := range topo.ComponentEdges {
		if e.From == from && e.To == to && e.Kind == kind {
			return true
		}
	}
	return false
}

func runtimePod(name, conv, image, node string) *Object {
	return labelled(obj("pods", name, "1",
		`{"nodeName":"`+node+`","containers":[{"name":"context-sync","image":"example/context-sync:1"},`+
			`{"name":"worker","image":"`+image+`"},{"name":"egress-proxy","image":"example/egress-proxy:1"}]}`,
		`{"phase":"Running","containerStatuses":[{"name":"worker","ready":true,"restartCount":2}]}`),
		map[string]string{"app.kubernetes.io/name": "agentops-runtime", "agentops.dev/conversation": conv})
}

func TestBundleIsReadFromTheHelmLabel(t *testing.T) {
	topo := BuildTopology(staticCache(
		labelled(obj("signalsources", "cluster-events", "1", `{"adapter":"k8s-events"}`, ""),
			map[string]string{"helm.sh/chart": "kubernetes-0.3.2"}),
		labelled(obj("mcpconfigs", "ha-api", "1", `{}`, ""),
			map[string]string{"helm.sh/chart": "home-assistant-1.0.0-rc.1"}),
		obj("agentruntimes", "default", "1", `{"image":"x:1"}`, ""),
	))
	if n := findNode(topo, "signalsources", "cluster-events"); n == nil || n.Bundle != "kubernetes" {
		t.Fatalf("the bundle is the chart name without its version: %+v", n)
	}
	if n := findNode(topo, "mcpconfigs", "ha-api"); n == nil || n.Bundle != "home-assistant" {
		t.Fatalf("a hyphenated chart and a prerelease version: %+v", n)
	}
	if n := findNode(topo, "agentruntimes", "default"); n.Bundle != "" {
		t.Fatalf("shared substrate carries no bundle: %+v", n)
	}
}

func TestPodClusterNodeIsReadFromItsSpec(t *testing.T) {
	topo := BuildTopology(staticCache(
		runtimePod("agentops-conv-a", "a", "example/agentops-runtime-claude:1", "node-7"),
		obj("pods", "pending-pod", "1", `{"containers":[{"name":"c","image":"i"}]}`, `{"phase":"Pending"}`),
	))
	pod := findPod(topo, "agentops-conv-a")
	if pod == nil || pod.ClusterNode != "node-7" || pod.Phase != "Running" || pod.Health != HealthOK {
		t.Fatalf("a pod's node, phase and health: %+v", pod)
	}
	if pod.Conversation != "a" || len(pod.Containers) != 3 || pod.Containers[1].Role != RoleRuntimeImage ||
		pod.Containers[1].Restarts != 2 || pod.Containers[0].Role != RoleContextSync {
		t.Fatalf("a runtime pod opens into its containers: %+v", pod)
	}
	if p := findPod(topo, "pending-pod"); p == nil || p.ClusterNode != "" {
		t.Fatalf("an unscheduled pod has no node, not a guessed one: %+v", p)
	}
}

func TestDeclaredExternalsBecomeComponentsJoinedToTheirAdapter(t *testing.T) {
	topo := BuildTopology(staticCache(
		obj("signaladapters", "alertmanager", "1",
			`{"image":"x:1","externals":[{"name":"Alertmanager","kind":"sender"},{"name":"Kubernetes API","kind":"kubernetes"}]}`,
			cond("Ready", "True", "")),
		obj("signaladapters", "k8s-events", "1",
			`{"image":"x:1","externals":[{"name":"Kubernetes API","kind":"kubernetes"}]}`, cond("Ready", "True", "")),
		obj("signaladapters", "cron", "1", `{"image":"x:1"}`, cond("Ready", "True", "")),
	))
	if n := findNode(topo, "signaladapters", "alertmanager"); n == nil || len(n.Externals) != 2 || n.Externals[0].Name != "Alertmanager" {
		t.Fatalf("the adapter node carries its declaration verbatim: %+v", n)
	}
	ext := findComponent(topo, "external/Kubernetes API")
	if ext == nil || ext.ExternalKind != "kubernetes" {
		t.Fatalf("one external per declared name, however many declare it: %+v", ext)
	}
	if !hasComponentEdge(topo, "external/Alertmanager", "signal-adapter/alertmanager", "sends") {
		t.Fatalf("a sender pushes to the adapter: %+v", topo.ComponentEdges)
	}
	if !hasComponentEdge(topo, "signal-adapter/k8s-events", "external/Kubernetes API", "calls") ||
		!hasComponentEdge(topo, "signal-adapter/alertmanager", "external/Kubernetes API", "calls") {
		t.Fatalf("the adapter calls what it faces: %+v", topo.ComponentEdges)
	}
	for _, e := range topo.ComponentEdges {
		if e.From == "signal-adapter/cron" || e.To == "signal-adapter/cron" {
			t.Fatalf("an adapter declaring nothing faces nothing: %+v", e)
		}
	}
	if n := findNode(topo, "signaladapters", "cron"); len(n.Externals) != 0 {
		t.Fatalf("no declaration, no externals: %+v", n)
	}
}

func TestRuntimeImageInSeveralPodsIsOneComponentWithACount(t *testing.T) {
	image := "ghcr.io/example/agentops-runtime-claude:0.9.0"
	topo := BuildTopology(staticCache(
		obj("agentruntimes", "claude", "1", `{"image":"`+image+`"}`, ""),
		obj("agentruntimes", "default", "1", `{"image":"`+image+`"}`, ""),
		runtimePod("agentops-conv-a", "a", image, "node-1"),
		runtimePod("agentops-conv-b", "b", image, "node-2"),
		runtimePod("agentops-conv-c", "c", image, "node-2"),
	))
	var images []Component
	for _, c := range topo.Components {
		if c.Role == RoleRuntimeImage {
			images = append(images, c)
		}
	}
	if len(images) != 1 {
		t.Fatalf("one image is one component: %+v", images)
	}
	img := images[0]
	if img.ID != "runtime-image/"+image || img.Count != 3 || len(img.Implements) != 2 {
		t.Fatalf("three pods, two runtimes, one node: %+v", img)
	}
	if img.Harness != "Claude Code" || img.Vendor != "Anthropic" {
		t.Fatalf("an image this repository builds names its harness and vendor: %+v", img)
	}
	if cs := findComponent(topo, "context-sync/context-sync"); cs == nil || cs.Count != 3 {
		t.Fatalf("each pod's sidecar is counted on its component: %+v", cs)
	}
	if p := findPod(topo, "agentops-conv-b"); p.Component != img.ID {
		t.Fatalf("a runtime pod runs its worker's image: %+v", p)
	}
}

func TestRuntimeNodesCarryImageAndVendorFacts(t *testing.T) {
	topo := BuildTopology(staticCache(
		obj("agentruntimes", "ollama", "1", `{"image":"ghcr.io/example/agentops-runtime-ollama@sha256:abc"}`, ""),
		obj("agentruntimes", "custom", "1", `{"image":"registry.example.org/team/my-runtime:2"}`, ""),
	))
	if n := findNode(topo, "agentruntimes", "ollama"); n.Vendor != "Ollama" || n.Image == "" {
		t.Fatalf("a digest-pinned image is still recognised: %+v", n)
	}
	if n := findNode(topo, "agentruntimes", "custom"); n.Vendor != "" || n.Harness != "" || n.Image != "registry.example.org/team/my-runtime:2" {
		t.Fatalf("a derived image is shown, never guessed at: %+v", n)
	}
}

func TestComponentsAreDerivedFromTheDeployments(t *testing.T) {
	topo := BuildTopology(staticCache(
		obj("deployments", "agentops-manager", "1",
			`{"template":{"spec":{"containers":[{"name":"manager","image":"example/manager:1",`+
				`"env":[{"name":"CONTEXT_SYNC_IMAGE","value":"example/context-sync:1"},{"name":"EGRESS_PROXY_IMAGE","value":"example/egress-proxy:1"}]}]}}}`, ""),
		obj("pods", "agentops-manager-5d9-abc", "1", `{"nodeName":"node-1","containers":[{"name":"manager","image":"example/manager:1"}]}`, `{"phase":"Running"}`),
		obj("channeladapters", "telegram", "1", `{"image":"example/channel-telegram:1"}`, cond("Ready", "False", "ImagePull")),
		obj("deployments", "agentops-adapter-telegram", "1", `{"template":{"spec":{"containers":[{"name":"a","image":"example/channel-telegram:1"}]}}}`, ""),
		obj("channeladapters", "console", "1", `{"image":"example/console:1"}`, cond("Ready", "True", "")),
		obj("signaladapters", "console", "1", `{"servedBy":{"kind":"ChannelAdapter","name":"console"}}`, ""),
		obj("deployments", "agentops-gateway-telegram", "1", `{"template":{"spec":{"containers":[{"name":"g","image":"example/gateway-telegram:1"}]}}}`, ""),
		obj("mcpconfigs", "k8s-api", "1", `{"servers":{"kubernetes":{"type":"http","url":"http://agentops-mcp-k8s.agent-ops.svc:8080/mcp"}}}`, ""),
		obj("deployments", "agentops-mcp-k8s", "1", `{"template":{"spec":{"containers":[{"name":"m","image":"example/mcp:1"}]}}}`, ""),
		obj("deployments", "agentops-mcp-k8s-admin", "1", `{"template":{"spec":{"containers":[{"name":"m","image":"example/mcp:1"}]}}}`, ""),
		obj("pods", "agentops-mcp-k8s-admin-7f-xyz", "1", `{"containers":[{"name":"m","image":"example/mcp:1"}]}`, `{"phase":"Running"}`),
		obj("cronjobs", "agentops-housekeeping", "1",
			`{"jobTemplate":{"spec":{"template":{"spec":{"containers":[{"name":"h","image":"example/housekeeping:1"}]}}}}}`, ""),
		obj("agentprofiles", "ops", "1", `{"repository":{"url":"https://git.example.org/ops.git"}}`, ""),
	))
	manager := findComponent(topo, "manager/manager")
	if manager == nil || manager.Workload != "deployments/agentops-manager" || manager.Count != 1 {
		t.Fatalf("the manager is a component with its pod counted: %+v", manager)
	}
	for id, image := range map[string]string{
		"context-sync/context-sync": "example/context-sync:1",
		"egress-proxy/egress-proxy": "example/egress-proxy:1",
	} {
		if c := findComponent(topo, id); c == nil || c.Image != image || c.Count != 0 {
			t.Fatalf("a sidecar is a component before any pod runs it: %s %+v", id, c)
		}
	}
	tg := findComponent(topo, "channel-adapter/telegram")
	if tg == nil || tg.Workload != "deployments/agentops-adapter-telegram" || tg.Health != HealthBad || tg.Reason != "ImagePull" {
		t.Fatalf("an adapter's health is its CR's own condition: %+v", tg)
	}
	if c := findComponent(topo, "signal-adapter/console"); c == nil || c.ServedBy != "channel-adapter/console" || c.Workload != "" {
		t.Fatalf("an externally served adapter runs in its server's process: %+v", c)
	}
	if c := findComponent(topo, "gateway/telegram"); c == nil || c.Image != "example/gateway-telegram:1" {
		t.Fatalf("the gateway: %+v", c)
	}
	if c := findComponent(topo, "mcp-server/kubernetes"); c == nil || c.Workload != "deployments/agentops-mcp-k8s" ||
		len(c.Implements) != 1 || c.Implements[0] != "mcpconfigs/k8s-api" {
		t.Fatalf("an MCP server is named by its key and runs as the deployment its URL names: %+v", c)
	}
	if c := findComponent(topo, "workload/agentops-mcp-k8s-admin"); c == nil || c.Count != 1 {
		t.Fatalf("an unrecognised deployment is drawn, and its pod is not credited to a shorter name: %+v", c)
	}
	if c := findComponent(topo, "housekeeping/housekeeping"); c == nil || c.Workload != "cronjobs/agentops-housekeeping" || c.Image == "" {
		t.Fatalf("housekeeping is its CronJob: %+v", c)
	}
	if c := findComponent(topo, "repository/https://git.example.org/ops.git"); c == nil || c.Implements[0] != "agentprofiles/ops" {
		t.Fatalf("a profile's repository: %+v", c)
	}
	if p := findPod(topo, "agentops-manager-5d9-abc"); p.Component != "manager/manager" || p.ClusterNode != "node-1" {
		t.Fatalf("a pod joins the component it runs: %+v", p)
	}
}

func TestRuntimeEdgeRunsFromThePipelineAndConversationsAreDrawn(t *testing.T) {
	topo := BuildTopology(staticCache(
		obj("agentprofiles", "ops", "1", `{"runtimeRef":{"name":"legacy"}}`, ""),
		obj("agentruntimes", "claude", "1", `{"image":"x:1"}`, ""),
		obj("agentruntimes", "legacy", "1", `{"image":"x:1"}`, ""),
		obj("pipelines", "p1", "1", `{"profileRef":{"name":"ops"},"runtimeRef":{"name":"claude"}}`, ""),
		obj("pipelines", "p2", "1", `{"profileRef":{"name":"ops"}}`, ""),
		obj("conversations", "c1", "1", `{"profileRef":{"name":"ops"},"pipelineRef":{"name":"p1"}}`,
			`{"phase":"Working","runtimePod":"agentops-conv-c1"}`),
	))
	if hasEdge(topo, "pipelines/p1", "agentruntimes/claude") == nil {
		t.Fatal("the Pipeline's own runtime")
	}
	if hasEdge(topo, "pipelines/p2", "agentruntimes/legacy") == nil {
		t.Fatal("the profile's deprecated runtime, drawn from the Pipeline")
	}
	for _, e := range topo.Edges {
		if e.From == "agentprofiles/ops" {
			t.Fatalf("no edge runs from a profile to a runtime: %+v", e)
		}
	}
	conv := findNode(topo, "conversations", "c1")
	if conv == nil || conv.Phase != "Working" || conv.RuntimePod != "pods/agentops-conv-c1" {
		t.Fatalf("a conversation is a Model node: %+v", conv)
	}
	if e := hasEdge(topo, "pipelines/p1", "conversations/c1"); e == nil || e.Kind != "opened" {
		t.Fatalf("the pipeline that opened it: %+v", e)
	}
}

func TestHopsAreServedAndAModelIsKnownByItsCalls(t *testing.T) {
	image := "example/agentops-runtime-claude:1"
	api, _, _, _ := apiWithOptions(t, "tok", true,
		obj("agentruntimes", "default", "1", `{"image":"`+image+`"}`, ""))
	api.activity.add(ActivityEvent{
		Cursor: pad(1), TS: time.Now().UTC(), Kind: "model.call", Status: "ok",
		From: &NodeRef{"runtime-image", image}, To: &NodeRef{"model", "model-a"},
		RunID: "r1", Data: map[string]string{"tokensIn": "1200"},
	})
	rec := authed(t, api.Handler(http.NotFoundHandler()), "GET", "/api/topology?windowSeconds=60", "")
	var out struct {
		Topology Topology `json:"topology"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if m := findComponent(out.Topology, "model/model-a"); m == nil || m.Role != RoleModel {
		t.Fatalf("a model named by a hop is a component: %+v", out.Topology.Components)
	}
	if len(out.Topology.Hops) != 1 || out.Topology.Hops[0].To.Name != "model-a" {
		t.Fatalf("the windowed hops are served in the activity vocabulary: %+v", out.Topology.Hops)
	}
	if c := findComponent(out.Topology, "runtime-image/"+image); c == nil {
		t.Fatal("a hop's endpoint is a component id as it stands")
	}

	rec = authed(t, api.Handler(http.NotFoundHandler()), "GET", "/api/activity", "")
	var feed struct {
		Events []ActivityEvent `json:"events"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatal(err)
	}
	if len(feed.Events) != 1 || feed.Events[0].Data["tokensIn"] != "1200" {
		t.Fatalf("a hop's data reaches the browser: %s", rec.Body.String())
	}
}

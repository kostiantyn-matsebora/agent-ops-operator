// Wiring-level tool access: Pipeline toolsets/mcpConfigs bindings, their
// materialization onto conversations, per-conversation MCP compilation, and
// dispatch-time allowlist resolution. The binding-less paths are pinned
// byte-identical here — this change must be invisible to anything that does
// not opt in.
package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/runtimepod"
)

func mkToolset(t *testing.T, name string, tools ...string) {
	t.Helper()
	ts := &agentopsv1alpha1.MCPToolset{}
	ts.Name, ts.Namespace = name, ns
	ts.Spec.Tools = tools
	if err := k8sClient.Create(context.Background(), ts); err != nil {
		t.Fatal(err)
	}
}

func mkMCPConfig(t *testing.T, name, serverKey, url string) {
	t.Helper()
	mc := &agentopsv1alpha1.MCPConfig{}
	mc.Name, mc.Namespace = name, ns
	mc.Spec.Servers = map[string]agentopsv1alpha1.MCPServer{serverKey: {Type: "sse", URL: url}}
	if err := k8sClient.Create(context.Background(), mc); err != nil {
		t.Fatal(err)
	}
}

func bind(refs ...string) *agentopsv1alpha1.ToolingBinding {
	b := &agentopsv1alpha1.ToolingBinding{}
	for _, r := range refs {
		b.Refs = append(b.Refs, agentopsv1alpha1.ObjectRef{Name: r})
	}
	return b
}

// mkRawMCPConfig creates the escape-hatch form: a hand-written mcp.json.
func mkRawMCPConfig(t *testing.T, name, configMap string) {
	t.Helper()
	mc := &agentopsv1alpha1.MCPConfig{}
	mc.Name, mc.Namespace = name, ns
	mc.Spec.ConfigMapRef = &agentopsv1alpha1.ObjectRef{Name: configMap}
	if err := k8sClient.Create(context.Background(), mc); err != nil {
		t.Fatal(err)
	}
}

// mkCapabilityPipeline declares a profile's BASELINE: no sources, no channels.
func mkCapabilityPipeline(t *testing.T, name, profile string, toolsets, mcpConfigs *agentopsv1alpha1.ToolingBinding) {
	t.Helper()
	mkToolPipeline(t, name, nil, nil, profile, toolsets, mcpConfigs)
	reconcilePipeline(t, name)
}

// mkToolPipeline creates a Pipeline carrying tooling bindings.
func mkToolPipeline(t *testing.T, name string, sources, channels []string, profile string,
	toolsets, mcpConfigs *agentopsv1alpha1.ToolingBinding) {
	t.Helper()
	mkPipeline(t, name, sources, channels, profile)
	var p agentopsv1alpha1.Pipeline
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &p); err != nil {
		t.Fatal(err)
	}
	p.Spec.Toolsets, p.Spec.MCPConfigs = toolsets, mcpConfigs
	if err := k8sClient.Update(context.Background(), &p); err != nil {
		t.Fatal(err)
	}
}

// mkIdentityProfile creates a profile carrying identity only — no tools, no
// MCP. Every profile is one of these now; capabilities come from the wiring.
func mkIdentityProfile(t *testing.T, name string) {
	t.Helper()
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = name, ns
	p.Spec.Repository = agentopsv1alpha1.RepositorySpec{URL: "https://example.com/repo.git", Ref: "main"}
	p.Spec.Agent = "tester"
	p.Spec.MaxTurns = 5
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

// clearRuntimePods frees the shared MaxRuntimes pool. envtest runs no kubelet,
// so every pod an earlier test created is still Pending and still counts as
// live — without this, pod creation here silently returns "pool full".
func clearRuntimePods(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	var pods corev1.PodList
	if err := k8sClient.List(ctx, &pods, client.InNamespace(ns),
		client.MatchingLabels{runtimepod.LabelApp: runtimepod.LabelAppValue}); err != nil {
		t.Fatal(err)
	}
	for i := range pods.Items {
		_ = k8sClient.Delete(ctx, &pods.Items[i], client.GracePeriodSeconds(0))
	}
}

// mkConv creates a channel-less conversation with one task input, cleaned up
// (pod included) so the shared MaxRuntimes pool stays predictable.
func mkConv(t *testing.T, name, profile string, toolsets, mcpConfigs *agentopsv1alpha1.ToolingBinding) *agentopsv1alpha1.Conversation {
	t.Helper()
	clearRuntimePods(t)
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	conv.Spec.Toolsets, conv.Spec.MCPConfigs = toolsets, mcpConfigs
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "do things"}}
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, name) })
	return conv
}

func reconcileErr(name string) error {
	_, err := reconciler().Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}})
	return err
}

// mountedMCPConfigMap returns the ConfigMap the runtime pod mounts at
// /etc/agentops.
func mountedMCPConfigMap(t *testing.T, convName string) string {
	t.Helper()
	var pod corev1.Pod
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: runtimepod.PodName(convName)}, &pod); err != nil {
		t.Fatalf("runtime pod for %s: %v", convName, err)
	}
	for _, v := range pod.Spec.Volumes {
		if v.Name == "mcp" && v.ConfigMap != nil {
			return v.ConfigMap.Name
		}
	}
	t.Fatalf("no mcp ConfigMap volume on pod for %s: %+v", convName, pod.Spec.Volumes)
	return ""
}

func mcpServerKeys(t *testing.T, cmName string) map[string]bool {
	t.Helper()
	var cm corev1.ConfigMap
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: cmName}, &cm); err != nil {
		t.Fatalf("ConfigMap %s: %v", cmName, err)
	}
	var doc struct {
		McpServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(cm.Data["mcp.json"]), &doc); err != nil {
		t.Fatalf("mcp.json in %s: %v", cmName, err)
	}
	keys := map[string]bool{}
	for k := range doc.McpServers {
		keys[k] = true
	}
	return keys
}

// dispatchedAllowedTools runs one /work dispatch and returns its allowlist.
func dispatchedAllowedTools(t *testing.T, convName string) string {
	t.Helper()
	rec := adapterReq(apiServer(), "GET", "/work?convo="+convName+"&wait=0", nil, "")
	if rec.Code != 200 {
		t.Fatalf("dispatch %s: %d %s", convName, rec.Code, rec.Body.String())
	}
	var unit struct {
		AllowedTools string `json:"allowedTools"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)
	return unit.AllowedTools
}

// ---- 2. Pipeline validation + materialization -------------------------------

func TestPipelineToolingRefsValidation(t *testing.T) {
	mkIdentityProfile(t, "prof-toolref")

	// dangling tooling refs are as fatal to Ready as a dangling channel
	mkToolPipeline(t, "toolref-pipe", nil, nil, "prof-toolref",
		bind("ts-late"), bind("cfg-late"))
	p := reconcilePipeline(t, "toolref-pipe")
	ready := apimeta.FindStatusCondition(p.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" || ready.Reason != "MissingReferences" {
		t.Fatalf("dangling tooling refs must break Ready: %+v", p.Status.Conditions)
	}
	if !strings.Contains(ready.Message, "mcptoolset/ts-late") || !strings.Contains(ready.Message, "mcpconfig/cfg-late") {
		t.Fatalf("Ready must name both missing refs: %q", ready.Message)
	}

	// creating them flips Ready back — the same heal path the other refs have
	mkToolset(t, "ts-late", "mcp__victorialogs__*")
	mkMCPConfig(t, "cfg-late", "victorialogs", "http://vl/sse")
	if p := reconcilePipeline(t, "toolref-pipe"); !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("Ready expected once tooling refs resolve: %+v", p.Status.Conditions)
	}
}

func TestPipelineBindingsMaterializeOnConversations(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-mat")
	mkToolset(t, "mat-ts", "mcp__victorialogs__*")
	mkMCPConfig(t, "mat-cfg", "victorialogs", "http://vl/sse")
	mkSignalSource(t, "mat-src", "mat-sig", "")
	mkToolPipeline(t, "mat-pipe", []string{"mat-src"}, nil, "prof-mat",
		bind("mat-ts"), bind("mat-cfg"))
	if p := reconcilePipeline(t, "mat-pipe"); !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("pipeline not Ready: %+v", p.Status.Conditions)
	}

	srv := apiServer()
	rec := postSignal(t, srv.Handler(), testMasterToken, "mat-src", []map[string]any{{
		"fingerprint": "mat-1", "labels": map[string]string{"alertname": "MatAlert"}, "payload": "boom",
	}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":1`) {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	var conv *agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].Spec.ProfileRef.Name == "prof-mat" {
			conv = &list.Items[i]
		}
	}
	if conv == nil {
		t.Fatal("signal conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	if conv.Spec.Toolsets == nil || len(conv.Spec.Toolsets.Refs) != 1 || conv.Spec.Toolsets.Refs[0].Name != "mat-ts" {
		t.Fatalf("toolsets binding not materialized: %+v", conv.Spec.Toolsets)
	}
	if conv.Spec.MCPConfigs == nil || len(conv.Spec.MCPConfigs.Refs) != 1 || conv.Spec.MCPConfigs.Refs[0].Name != "mat-cfg" {
		t.Fatalf("mcpConfigs binding not materialized: %+v", conv.Spec.MCPConfigs)
	}

	// POST /task addresses the SAME Pipeline and gets its whole wiring —
	// profile, channels, and capabilities from one lookup.
	rec = adapterReq(srv, "POST", "/task", map[string]any{"pipeline": "mat-pipe", "task": "addressed"}, "")
	if rec.Code != 202 {
		t.Fatalf("task: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Conversation string `json:"conversation"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	t.Cleanup(func() { cleanupConversation(t, created.Conversation) })
	var taskConv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: created.Conversation}, &taskConv); err != nil {
		t.Fatal(err)
	}
	if taskConv.Spec.ProfileRef.Name != "prof-mat" {
		t.Fatalf("profile must come from the addressed pipeline: %+v", taskConv.Spec.ProfileRef)
	}
	if taskConv.Spec.Toolsets == nil || taskConv.Spec.Toolsets.Refs[0].Name != "mat-ts" {
		t.Fatalf("addressed pipeline's toolsets must carry: %+v", taskConv.Spec.Toolsets)
	}

	// addressing nothing, or something unknown, is refused rather than
	// producing a conversation nobody wired
	if rec := adapterReq(srv, "POST", "/task", map[string]any{"task": "no pipeline"}, ""); rec.Code != 400 {
		t.Fatalf("a task naming no pipeline must be refused: %d %s", rec.Code, rec.Body.String())
	}
	if rec := adapterReq(srv, "POST", "/task", map[string]any{"pipeline": "nope", "task": "x"}, ""); rec.Code != 404 {
		t.Fatalf("an unknown pipeline must 404: %d", rec.Code)
	}
}

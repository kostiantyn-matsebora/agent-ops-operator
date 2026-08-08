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
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/mcpcompile"
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

func bind(mode string, refs ...string) *agentopsv1alpha1.ToolingBinding {
	b := &agentopsv1alpha1.ToolingBinding{Mode: mode}
	for _, r := range refs {
		b.Refs = append(b.Refs, agentopsv1alpha1.ObjectRef{Name: r})
	}
	return b
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

// mkProfileWithMCP creates a profile carrying its own MCP and allowlist — the
// "before" side of every merge/overwrite assertion.
func mkProfileWithMCP(t *testing.T, name, allowedTools string, mcp *agentopsv1alpha1.MCPSpec) {
	t.Helper()
	p := &agentopsv1alpha1.AgentProfile{}
	p.Name, p.Namespace = name, ns
	p.Spec.Repository = agentopsv1alpha1.RepositorySpec{URL: "https://example.com/repo.git", Ref: "main"}
	p.Spec.Agent = "tester"
	p.Spec.AllowedTools = allowedTools
	p.Spec.MaxTurns = 5
	p.Spec.MCP = mcp
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
	mkProfile(t, "prof-toolref")

	// dangling tooling refs are as fatal to Ready as a dangling channel
	mkToolPipeline(t, "toolref-pipe", nil, nil, "prof-toolref",
		bind(agentopsv1alpha1.ToolingMerge, "ts-late"), bind(agentopsv1alpha1.ToolingMerge, "cfg-late"))
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
	mkProfile(t, "prof-mat")
	mkToolset(t, "mat-ts", "mcp__victorialogs__*")
	mkMCPConfig(t, "mat-cfg", "victorialogs", "http://vl/sse")
	mkSignalSource(t, "mat-src", "mat-sig", "")
	mkToolPipeline(t, "mat-pipe", []string{"mat-src"}, nil, "prof-mat",
		bind(agentopsv1alpha1.ToolingMerge, "mat-ts"), bind(agentopsv1alpha1.ToolingOverwrite, "mat-cfg"))
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

	if conv.Spec.Toolsets == nil || conv.Spec.Toolsets.Mode != agentopsv1alpha1.ToolingMerge ||
		len(conv.Spec.Toolsets.Refs) != 1 || conv.Spec.Toolsets.Refs[0].Name != "mat-ts" {
		t.Fatalf("toolsets binding not materialized: %+v", conv.Spec.Toolsets)
	}
	if conv.Spec.MCPConfigs == nil || conv.Spec.MCPConfigs.Mode != agentopsv1alpha1.ToolingOverwrite ||
		len(conv.Spec.MCPConfigs.Refs) != 1 || conv.Spec.MCPConfigs.Refs[0].Name != "mat-cfg" {
		t.Fatalf("mcpConfigs binding not materialized (modes are independent): %+v", conv.Spec.MCPConfigs)
	}

	// POST /task has no originating pipeline — it keeps pure profile behavior
	rec = adapterReq(srv, "POST", "/task", map[string]any{"profile": "prof-mat", "task": "no wiring here"}, "")
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
	if taskConv.Spec.Toolsets != nil || taskConv.Spec.MCPConfigs != nil {
		t.Fatalf("task-API conversations must carry no bindings: %+v %+v", taskConv.Spec.Toolsets, taskConv.Spec.MCPConfigs)
	}
}

// ---- 3. Resolution: compile + dispatch --------------------------------------

// The whole point of the binding being optional: a conversation without one
// renders the same ConfigMap, under the same name and owner, as before.
func TestBindingLessConversationIsUnchanged(t *testing.T) {
	ctx := context.Background()
	mcp := &agentopsv1alpha1.MCPSpec{Servers: map[string]agentopsv1alpha1.MCPServer{
		"profilesrv": {Type: "sse", URL: "http://profile/sse"},
	}}
	mkProfileWithMCP(t, "prof-plain", "Read,Bash", mcp)
	mkConv(t, "plain-conv", "prof-plain", nil, nil)
	reconcile(t, "plain-conv")

	want, err := mcpcompile.Compile(mcp, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmName := controller.MCPConfigMapName("prof-plain")
	if got := mountedMCPConfigMap(t, "plain-conv"); got != cmName {
		t.Fatalf("binding-less pod must mount the profile ConfigMap, got %q", got)
	}
	var cm corev1.ConfigMap
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: cmName}, &cm); err != nil {
		t.Fatal(err)
	}
	if cm.Data["mcp.json"] != want.JSON {
		t.Fatalf("profile ConfigMap content changed:\n%s\n---\n%s", cm.Data["mcp.json"], want.JSON)
	}
	if len(cm.OwnerReferences) != 1 || cm.OwnerReferences[0].Kind != "AgentProfile" {
		t.Fatalf("profile ConfigMap must stay profile-owned: %+v", cm.OwnerReferences)
	}
	if got := dispatchedAllowedTools(t, "plain-conv"); got != "Read,Bash" {
		t.Fatalf("binding-less allowedTools must be the profile's verbatim: %q", got)
	}
}

// Two pipelines binding different configs to ONE profile must not fight over a
// shared ConfigMap — each conversation compiles its own, owned by itself.
func TestBoundConversationsGetOwnConfigMaps(t *testing.T) {
	ctx := context.Background()
	mcp := &agentopsv1alpha1.MCPSpec{Servers: map[string]agentopsv1alpha1.MCPServer{
		"profilesrv": {Type: "sse", URL: "http://profile/sse"},
	}}
	mkProfileWithMCP(t, "prof-bound", "Read", mcp)
	mkMCPConfig(t, "bound-logs", "victorialogs", "http://vl/sse")
	mkMCPConfig(t, "bound-metrics", "victoriametrics", "http://vm/sse")

	mkConv(t, "bound-merge", "prof-bound", nil, bind(agentopsv1alpha1.ToolingMerge, "bound-logs"))
	reconcile(t, "bound-merge")
	mergeCM := mountedMCPConfigMap(t, "bound-merge")
	if mergeCM != "agentops-mcp-conv-bound-merge" {
		t.Fatalf("bound conversation must mount its own ConfigMap, got %q", mergeCM)
	}
	keys := mcpServerKeys(t, mergeCM)
	if !keys["profilesrv"] || !keys["victorialogs"] || len(keys) != 2 {
		t.Fatalf("merge must combine profile and bound servers: %v", keys)
	}
	var cm corev1.ConfigMap
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: mergeCM}, &cm); err != nil {
		t.Fatal(err)
	}
	if len(cm.OwnerReferences) != 1 || cm.OwnerReferences[0].Kind != "Conversation" {
		t.Fatalf("per-conversation ConfigMap must be conversation-owned for GC: %+v", cm.OwnerReferences)
	}

	mkConv(t, "bound-over", "prof-bound", nil, bind(agentopsv1alpha1.ToolingOverwrite, "bound-metrics"))
	reconcile(t, "bound-over")
	overCM := mountedMCPConfigMap(t, "bound-over")
	if overCM == mergeCM {
		t.Fatal("two bound conversations must not share a ConfigMap")
	}
	keys = mcpServerKeys(t, overCM)
	if !keys["victoriametrics"] || len(keys) != 1 {
		t.Fatalf("overwrite must drop the profile's servers: %v", keys)
	}

	// the shared profile ConfigMap is never touched by bound conversations
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: controller.MCPConfigMapName("prof-bound")}, &cm); err == nil {
		t.Fatalf("bound conversations must not write the profile ConfigMap: %v", cm.Data)
	}
}

func TestDispatchAllowlistFollowsToolsets(t *testing.T) {
	mkProfileWithMCP(t, "prof-allow", "Read,Bash", nil)
	mkToolset(t, "allow-ts", "Bash", "mcp__victorialogs__*")

	mkConv(t, "allow-merge", "prof-allow", bind(agentopsv1alpha1.ToolingMerge, "allow-ts"), nil)
	if got := dispatchedAllowedTools(t, "allow-merge"); got != "Read,Bash,mcp__victorialogs__*" {
		t.Fatalf("merge allowlist: %q", got)
	}

	mkConv(t, "allow-over", "prof-allow", bind(agentopsv1alpha1.ToolingOverwrite, "allow-ts"), nil)
	if got := dispatchedAllowedTools(t, "allow-over"); got != "Bash,mcp__victorialogs__*" {
		t.Fatalf("overwrite allowlist: %q", got)
	}
}

// Toolset CONTENT is read at dispatch, not snapshotted: editing a toolset
// reaches conversations already running under it, with no pod restart.
func TestToolsetEditsReachRunningConversations(t *testing.T) {
	ctx := context.Background()
	mkProfileWithMCP(t, "prof-edit", "Read", nil)
	mkToolset(t, "edit-ts", "Grep")
	conv := mkConv(t, "edit-conv", "prof-edit", bind(agentopsv1alpha1.ToolingMerge, "edit-ts"), nil)

	if got := dispatchedAllowedTools(t, conv.Name); got != "Read,Grep" {
		t.Fatalf("initial allowlist: %q", got)
	}
	var ts agentopsv1alpha1.MCPToolset
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "edit-ts"}, &ts); err != nil {
		t.Fatal(err)
	}
	ts.Spec.Tools = []string{"Grep", "mcp__victorialogs__*"}
	if err := k8sClient.Update(ctx, &ts); err != nil {
		t.Fatal(err)
	}
	// clear the inflight run and queue a second unit
	var fresh agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: conv.Name}, &fresh); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(fresh.DeepCopy())
	fresh.Status.Inflight = nil
	if err := k8sClient.Status().Patch(ctx, &fresh, patch); err != nil {
		t.Fatal(err)
	}
	specPatch := client.MergeFrom(fresh.DeepCopy())
	fresh.Spec.Inputs = append(fresh.Spec.Inputs,
		agentopsv1alpha1.InputItem{ID: "i2", Type: agentopsv1alpha1.InputTask, Payload: "again"})
	if err := k8sClient.Patch(ctx, &fresh, specPatch); err != nil {
		t.Fatal(err)
	}
	if got := dispatchedAllowedTools(t, conv.Name); got != "Read,Grep,mcp__victorialogs__*" {
		t.Fatalf("toolset edit must reach the next work unit: %q", got)
	}
}

// A binding that no longer resolves must be loud. Silently falling back to the
// profile's tools would hand the agent a working prompt and quietly missing
// tools — the failure mode this whole path exists to avoid.
func TestMissingToolsetRefFailsVisibly(t *testing.T) {
	ctx := context.Background()
	mkProfileWithMCP(t, "prof-gone", "Read", nil)
	mkConv(t, "gone-conv", "prof-gone", bind(agentopsv1alpha1.ToolingMerge, "never-existed"), nil)

	rec := adapterReq(apiServer(), "GET", "/work?convo=gone-conv&wait=0", nil, "")
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "never-existed") {
		t.Fatalf("missing toolset must fail the dispatch naming the ref: %d %s", rec.Code, rec.Body.String())
	}
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "gone-conv"}, &conv); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(conv.Status.Conditions, controller.ConditionToolingResolved)
	if cond == nil || cond.Status != "False" || !strings.Contains(cond.Message, "never-existed") {
		t.Fatalf("failure must be visible on the conversation: %+v", conv.Status.Conditions)
	}
}

// A raw-form profile MCP is opaque, so merging bound configs onto it is refused
// rather than half-applied — and the refusal names the incompatibility.
func TestRawFormProfileRefusesMergeVisibly(t *testing.T) {
	ctx := context.Background()
	mkProfileWithMCP(t, "prof-raw", "Read",
		&agentopsv1alpha1.MCPSpec{ConfigMapRef: &agentopsv1alpha1.ObjectRef{Name: "hand-written-mcp"}})
	mkMCPConfig(t, "raw-logs", "victorialogs", "http://vl/sse")
	mkConv(t, "raw-conv", "prof-raw", nil, bind(agentopsv1alpha1.ToolingMerge, "raw-logs"))

	if err := reconcileErr("raw-conv"); err == nil || !strings.Contains(err.Error(), "hand-written-mcp") {
		t.Fatalf("merge onto a raw profile MCP must fail naming it: %v", err)
	}
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "raw-conv"}, &conv); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(conv.Status.Conditions, controller.ConditionToolingResolved)
	if cond == nil || cond.Status != "False" || cond.Reason != "IncompatibleMCPForm" {
		t.Fatalf("incompatibility must surface on the conversation: %+v", conv.Status.Conditions)
	}
	// no pod with half-applied tooling
	var pod corev1.Pod
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: runtimepod.PodName("raw-conv")}, &pod); err == nil {
		t.Fatal("no runtime pod may be created while the MCP merge is unresolved")
	}

	// overwrite ignores the profile's MCP entirely, so it works over raw forms
	mkConv(t, "raw-over", "prof-raw", nil, bind(agentopsv1alpha1.ToolingOverwrite, "raw-logs"))
	if err := reconcileErr("raw-over"); err != nil {
		t.Fatalf("overwrite over a raw-form profile must work: %v", err)
	}
	keys := mcpServerKeys(t, mountedMCPConfigMap(t, "raw-over"))
	if !keys["victorialogs"] || len(keys) != 1 {
		t.Fatalf("overwrite result: %v", keys)
	}
}

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

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
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

// bindTools is bind for the toolsets stanza, which additionally carries the
// mode composing its tools with the agent definition's.
func bindTools(refs ...string) *agentopsv1alpha1.ToolsetBinding {
	b := &agentopsv1alpha1.ToolsetBinding{}
	for _, r := range refs {
		b.Refs = append(b.Refs, agentopsv1alpha1.ObjectRef{Name: r})
	}
	return b
}

// bindToolsMode is bindTools with an explicit composition mode.
func bindToolsMode(mode string, refs ...string) *agentopsv1alpha1.ToolsetBinding {
	b := bindTools(refs...)
	b.Mode = mode
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
func mkCapabilityPipeline(t *testing.T, name, profile string, toolsets *agentopsv1alpha1.ToolsetBinding, mcpConfigs *agentopsv1alpha1.ToolingBinding) {
	t.Helper()
	mkToolPipeline(t, name, nil, nil, profile, toolsets, mcpConfigs)
	reconcilePipeline(t, name)
}

// mkToolPipeline creates a Pipeline carrying tooling bindings.
func mkToolPipeline(t *testing.T, name string, sources, channels []string, profile string,
	toolsets *agentopsv1alpha1.ToolsetBinding, mcpConfigs *agentopsv1alpha1.ToolingBinding) {
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
func mkConv(t *testing.T, name, profile string, toolsets *agentopsv1alpha1.ToolsetBinding, mcpConfigs *agentopsv1alpha1.ToolingBinding) *agentopsv1alpha1.Conversation {
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
		bindTools("ts-late"), bind("cfg-late"))
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
		bindTools("mat-ts"), bind("mat-cfg"))
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

	// A posted task reaches the SAME Pipeline and gets its whole wiring —
	// profile, channels, and capabilities. It names the SOURCE, never the
	// pipeline: which agent answers is declared by the claim, not chosen here.
	rec = postSignal(t, srv.Handler(), testMasterToken, "mat-src", []map[string]any{{
		"fingerprint": "mat-task-1", "kind": "task", "payload": "addressed",
	}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":1`) {
		t.Fatalf("task signal: %d %s", rec.Code, rec.Body.String())
	}
	var taskConv *agentopsv1alpha1.Conversation
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	for i := range list.Items {
		c := &list.Items[i]
		if c.Spec.ProfileRef.Name == "prof-mat" && strings.HasPrefix(c.Name, "task-") {
			taskConv = c
		}
	}
	if taskConv == nil {
		t.Fatal("task conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, taskConv.Name) })
	if taskConv.Spec.Toolsets == nil || taskConv.Spec.Toolsets.Refs[0].Name != "mat-ts" {
		t.Fatalf("the claiming pipeline's toolsets must carry: %+v", taskConv.Spec.Toolsets)
	}
	if taskConv.Spec.MCPConfigs == nil || taskConv.Spec.MCPConfigs.Refs[0].Name != "mat-cfg" {
		t.Fatalf("the claiming pipeline's mcpConfigs must carry: %+v", taskConv.Spec.MCPConfigs)
	}

	// a malformed signal, or one naming a source nobody serves, is refused
	// rather than producing a conversation nobody wired
	if rec := postSignal(t, srv.Handler(), testMasterToken, "mat-src", []map[string]any{{
		"kind": "task", "payload": "no fingerprint",
	}}); rec.Code != 400 {
		t.Fatalf("a signal with no fingerprint must be refused: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postSignal(t, srv.Handler(), testMasterToken, "nope", []map[string]any{{
		"fingerprint": "mat-task-x", "kind": "task", "payload": "x",
	}}); rec.Code != 404 {
		t.Fatalf("an unknown source must 404: %d", rec.Code)
	}
}

// ---- 5. Toolsets composition mode -------------------------------------------

// dispatchedTooling runs one /work dispatch and returns the fields the runtime
// composes with: the wiring's tools, the mode, and the agent whose definition
// supplies the other half.
func dispatchedTooling(t *testing.T, convName string) (tools, mode, agent string) {
	t.Helper()
	rec := adapterReq(apiServer(), "GET", "/work?convo="+convName+"&wait=0", nil, "")
	if rec.Code != 200 {
		t.Fatalf("dispatch %s: %d %s", convName, rec.Code, rec.Body.String())
	}
	var unit struct {
		AllowedTools string `json:"allowedTools"`
		ToolsMode    string `json:"toolsMode"`
		Agent        string `json:"agent"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &unit)
	return unit.AllowedTools, unit.ToolsMode, unit.Agent
}

// The mode is wiring, so it must survive the whole path: Pipeline -> the
// conversation it originates -> the work unit the runtime composes from.
func TestToolsModeRoundTripsFromPipelineToWorkUnit(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-mode")
	mkToolset(t, "mode-ts", "Bash")
	mkSignalSource(t, "mode-src", "mode-sig", "")
	mkToolPipeline(t, "mode-pipe", []string{"mode-src"}, nil, "prof-mode",
		bindToolsMode(agentopsv1alpha1.ToolsModeOverwrite, "mode-ts"), nil)
	if p := reconcilePipeline(t, "mode-pipe"); !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("pipeline not Ready: %+v", p.Status.Conditions)
	}

	rec := postSignal(t, apiServer().Handler(), testMasterToken, "mode-src", []map[string]any{{
		"fingerprint": "mode-1", "labels": map[string]string{"alertname": "ModeAlert"}, "payload": "boom",
	}})
	if rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	var conv *agentopsv1alpha1.Conversation
	for i := range list.Items {
		if list.Items[i].Spec.ProfileRef.Name == "prof-mode" {
			conv = &list.Items[i]
		}
	}
	if conv == nil {
		t.Fatal("signal conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })

	if conv.Spec.Toolsets == nil || conv.Spec.Toolsets.Mode != agentopsv1alpha1.ToolsModeOverwrite {
		t.Fatalf("mode not materialized onto the conversation: %+v", conv.Spec.Toolsets)
	}
	tools, mode, agent := dispatchedTooling(t, conv.Name)
	if tools != "Bash" {
		t.Fatalf("wiring contribution: want Bash, got %q", tools)
	}
	if mode != agentopsv1alpha1.ToolsModeOverwrite {
		t.Fatalf("work unit mode: want overwrite, got %q", mode)
	}
	// Without the agent name the runtime cannot find the definition whose
	// tools the mode composes against.
	if agent != "tester" {
		t.Fatalf("work unit agent: want tester (the profile's), got %q", agent)
	}
}

// A binding stored without a mode — an older object, or a Pipeline applied
// before the field existed — must dispatch as merge. Reading it as overwrite
// would silently strip whatever the agent declared for itself.
func TestAbsentToolsModeDispatchesAsMerge(t *testing.T) {
	mkIdentityProfile(t, "prof-nomode")
	mkToolset(t, "nomode-ts", "Grep")
	mkConv(t, "nomode-conv", "prof-nomode", bindTools("nomode-ts"), nil)

	_, mode, _ := dispatchedTooling(t, "nomode-conv")
	if mode != agentopsv1alpha1.ToolsModeMerge {
		t.Fatalf("absent mode must dispatch as merge, got %q", mode)
	}
}

// A conversation with no toolsets binding at all still states a mode: the
// runtime always composes, and "no wiring tools, merge" is what lets an agent
// definition's own declaration stand on its own.
func TestNoBindingStillCarriesMergeMode(t *testing.T) {
	mkIdentityProfile(t, "prof-nobind")
	mkConv(t, "nobind-conv", "prof-nobind", nil, nil)

	tools, mode, _ := dispatchedTooling(t, "nobind-conv")
	if tools != "" {
		t.Fatalf("unbound conversation must contribute no tools, got %q", tools)
	}
	if mode != agentopsv1alpha1.ToolsModeMerge {
		t.Fatalf("want merge, got %q", mode)
	}
}

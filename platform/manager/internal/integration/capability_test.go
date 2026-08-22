// Capability resolution after capabilities left the AgentProfile: what an agent
// may do comes only from the Pipeline that originated its conversation. There
// is no profile default and no fallback — a Pipeline that declares nothing
// grants nothing, which is a configuration, not a defect.
package integration

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// A conversation whose Pipeline declared a binding carries exactly that.
func TestBoundConversationOwnsItsConfigMap(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-cm")
	mkMCPConfig(t, "cm-logs", "victorialogs", "http://vl/sse")
	mkConv(t, "cm-conv", "prof-cm", nil, bind("cm-logs"))
	reconcile(t, "cm-conv")

	name := mountedMCPConfigMap(t, "cm-conv")
	if name != "agentops-mcp-conv-cm-conv" {
		t.Fatalf("conversation-owned ConfigMap expected, got %q", name)
	}
	if keys := mcpServerKeys(t, name); !keys["victorialogs"] || len(keys) != 1 {
		t.Fatalf("bound configs are the whole MCP: %v", keys)
	}
	var cm corev1.ConfigMap
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &cm); err != nil {
		t.Fatal(err)
	}
	if len(cm.OwnerReferences) != 1 || cm.OwnerReferences[0].Kind != "Conversation" {
		t.Fatalf("must be conversation-owned for GC: %+v", cm.OwnerReferences)
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "agentops-mcp-prof-cm"}, &cm); err == nil {
		t.Fatal("no profile-keyed ConfigMap may exist — profiles declare no MCP")
	}
}

// A hand-written mcp.json is opaque, so binding it alongside another config
// refuses loudly rather than mounting one side and dropping the other.
func TestRawConfigNotExclusiveFailsVisibly(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-raw")
	mkRawMCPConfig(t, "raw-hand", "hand-written-mcp")
	mkMCPConfig(t, "raw-other", "victorialogs", "http://vl/sse")
	mkConv(t, "raw-conv", "prof-raw", nil, bind("raw-hand", "raw-other"))

	err := reconcileErr("raw-conv")
	if err == nil || !strings.Contains(err.Error(), "raw-hand") {
		t.Fatalf("combining a raw config must fail naming it: %v", err)
	}
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "raw-conv"}, &conv); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(conv.Status.Conditions, controller.ConditionToolingResolved)
	if cond == nil || cond.Status != "False" || cond.Reason != "RawConfigNotExclusive" {
		t.Fatalf("must surface on the conversation: %+v", conv.Status.Conditions)
	}

	// bound alone, the escape hatch still works
	mkConv(t, "raw-alone", "prof-raw", nil, bind("raw-hand"))
	if err := reconcileErr("raw-alone"); err != nil {
		t.Fatalf("a raw config bound alone must work: %v", err)
	}
}

// Dispatch reads the bound toolsets, so editing one reaches running
// conversations without a pod restart.
func TestToolsetEditsReachRunningConversations(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-edit")
	mkToolset(t, "edit-ts", "Grep")
	mkConv(t, "edit-conv", "prof-edit", bindTools("edit-ts"), nil)

	if got := dispatchedAllowedTools(t, "edit-conv"); got != "Grep" {
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
	clearInflight(t, "edit-conv")
	if got := dispatchedAllowedTools(t, "edit-conv"); got != "Grep,mcp__victorialogs__*" {
		t.Fatalf("toolset edit must reach the next work unit: %q", got)
	}
}

// A binding that stops resolving fails the dispatch visibly rather than
// quietly handing the agent fewer tools than its wiring promised.
func TestMissingToolsetRefFailsVisibly(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-gone")
	mkConv(t, "gone-conv", "prof-gone", bindTools("never-existed"), nil)

	rec := adapterReq(apiServer(), "GET", "/work?convo=gone-conv&wait=0", nil, "")
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "never-existed") {
		t.Fatalf("missing toolset must fail the dispatch naming the ref: %d %s", rec.Code, rec.Body.String())
	}
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "gone-conv"}, &conv); err != nil {
		t.Fatal(err)
	}
	cond := apimeta.FindStatusCondition(conv.Status.Conditions, controller.ConditionToolingResolved)
	if cond == nil || cond.Status != "False" {
		t.Fatalf("failure must be visible on the conversation: %+v", conv.Status.Conditions)
	}
}

// The regression this change fixes: the chart's own routing Pipelines declared
// no capabilities, so every signal-driven conversation — an alert arriving and
// an agent investigating it, the product's headline flow — dispatched an empty
// allowlist. Nothing asserted the signal path's tools end to end, which is why
// it shipped.
//
// This builds the bundle's shape as the chart emits it: a source, a Pipeline
// claiming it that declares its toolsets, and a signal arriving.
func TestSignalDrivenConversationIsEquipped(t *testing.T) {
	ctx := context.Background()
	mkIdentityProfile(t, "prof-sigdriven")
	mkToolset(t, "sigdriven-observe", "Read", "Grep")
	mkToolset(t, "sigdriven-shell", "Bash")
	mkSignalSource(t, "sigdriven-src", "sigdriven-sig", "")
	mkToolPipeline(t, "sigdriven-pipe", []string{"sigdriven-src"}, nil, "prof-sigdriven",
		bindTools("sigdriven-observe", "sigdriven-shell"), nil)
	if p := reconcilePipeline(t, "sigdriven-pipe"); !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("pipeline not Ready: %+v", p.Status.Conditions)
	}

	rec := postSignal(t, apiServer().Handler(), testMasterToken, "sigdriven-src", []map[string]any{{
		"fingerprint": "sig-1", "labels": map[string]string{"alertname": "BackOff"}, "payload": "boom",
	}})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":1`) {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	var list agentopsv1alpha1.ConversationList
	_ = k8sClient.List(ctx, &list, client.InNamespace(ns))
	var name string
	for i := range list.Items {
		if list.Items[i].Spec.ProfileRef.Name == "prof-sigdriven" {
			name = list.Items[i].Name
		}
	}
	if name == "" {
		t.Fatal("signal conversation not created")
	}
	t.Cleanup(func() { cleanupConversation(t, name) })

	got := dispatchedAllowedTools(t, name)
	if got == "" {
		t.Fatal("a signal-driven conversation dispatched an EMPTY allowlist — the agent can do nothing")
	}
	if got != "Read,Grep,Bash" {
		t.Fatalf("allowlist must be the Pipeline's declared toolsets: %q", got)
	}
}

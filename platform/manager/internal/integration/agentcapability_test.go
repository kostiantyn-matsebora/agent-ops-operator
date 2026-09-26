// AgentCapability: the six capability fields a Pipeline can either inline
// (today's shape, unchanged) or name through capabilityRef instead. Phase 1 of
// coordinated-agents (design D-A): pure addition, so every existing Pipeline
// fixture in this suite must keep behaving exactly as it did before this file
// existed.
package integration

import (
	"context"
	"strings"
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// mkCapability creates an AgentCapability naming a profile.
func mkCapability(t *testing.T, name, profile string) *agentopsv1alpha1.AgentCapability {
	t.Helper()
	c := &agentopsv1alpha1.AgentCapability{}
	c.Name, c.Namespace = name, ns
	if profile != "" {
		c.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: profile}
	}
	if err := k8sClient.Create(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c
}

func reconcileCapability(t *testing.T, name string) *agentopsv1alpha1.AgentCapability {
	t.Helper()
	rc := &controller.AgentCapabilityReconciler{Client: k8sClient}
	if _, err := rc.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
		t.Fatal(err)
	}
	var c agentopsv1alpha1.AgentCapability
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

// 1.4/1.7 — an AgentCapability validates the SAME refs a Pipeline's inline
// capability does, through the shared validator: a dangling profile and a
// dangling runtime are both named, and every ref resolving is Ready.
func TestAgentCapabilityValidatesItsOwnRefs(t *testing.T) {
	mkProfile(t, "cap-prof-ok")
	mkRuntime(t, "cap-rt-ok", "example/agent:cap", "")

	mkCapability(t, "cap-ok", "cap-prof-ok")
	if c := reconcileCapability(t, "cap-ok"); !apimeta.IsStatusConditionTrue(c.Status.Conditions, "Ready") {
		t.Fatalf("every ref resolves: %+v", c.Status.Conditions)
	}

	mkCapability(t, "cap-dangling-profile", "no-such-profile")
	c := reconcileCapability(t, "cap-dangling-profile")
	ready := apimeta.FindStatusCondition(c.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" || !strings.Contains(ready.Message, "agentprofile/no-such-profile") {
		t.Fatalf("dangling profile not surfaced: %+v", c.Status.Conditions)
	}

	withRuntime := &agentopsv1alpha1.AgentCapability{}
	withRuntime.Name, withRuntime.Namespace = "cap-dangling-runtime", ns
	withRuntime.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "cap-prof-ok"}
	withRuntime.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "no-such-runtime"}
	if err := k8sClient.Create(context.Background(), withRuntime); err != nil {
		t.Fatal(err)
	}
	c = reconcileCapability(t, "cap-dangling-runtime")
	ready = apimeta.FindStatusCondition(c.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" || !strings.Contains(ready.Message, "agentruntime/no-such-runtime") {
		t.Fatalf("dangling runtime not surfaced: %+v", c.Status.Conditions)
	}

	// carries no wiring of its own — an AgentCapability that nothing
	// references is INERT, not invalid.
	mkCapability(t, "cap-unreferenced", "cap-prof-ok")
	if c := reconcileCapability(t, "cap-unreferenced"); !apimeta.IsStatusConditionTrue(c.Status.Conditions, "Ready") {
		t.Fatalf("an unreferenced but internally-valid capability is still Ready: %+v", c.Status.Conditions)
	}
}

// 1.5 — a Pipeline naming NEITHER capabilityRef nor profileRef fails Ready,
// never admission: CEL cannot express "one of" across an embedded struct.
func TestPipelineNeitherCapabilityRefNorProfileRefFailsReady(t *testing.T) {
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "pipe-no-capability", ns
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatalf("admission must accept this — Ready is where it fails: %v", err)
	}
	got := reconcilePipeline(t, "pipe-no-capability")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" || !strings.Contains(ready.Message, "profileRef or capabilityRef") {
		t.Fatalf("missing capability not surfaced: %+v", got.Status.Conditions)
	}
}

// 1.5 — a Pipeline naming a capabilityRef that does not exist fails Ready
// naming it, the same shape a dangling profileRef/runtimeRef already take.
func TestPipelineDanglingCapabilityRefFailsReady(t *testing.T) {
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "pipe-dangling-capability", ns
	p.Spec.AgentRef = &agentopsv1alpha1.ObjectRef{Name: "no-such-capability"}
	if err := k8sClient.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := reconcilePipeline(t, "pipe-dangling-capability")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" || !strings.Contains(ready.Message, "agentcapability/no-such-capability") {
		t.Fatalf("dangling capabilityRef not surfaced: %+v", got.Status.Conditions)
	}
}

// 1.2 — the API server refuses capabilityRef alongside an inline field, rather
// than a reconciler reporting it after the object is stored: the two are two
// answers to "where does this Pipeline's capability live".
func TestPipelineCapabilityRefAndInlineFieldsAreMutuallyExclusive(t *testing.T) {
	mkProfile(t, "cap-xor-prof")
	mkCapability(t, "cap-xor-cap", "cap-xor-prof")

	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = "pipe-cap-xor", ns
	p.Spec.AgentRef = &agentopsv1alpha1.ObjectRef{Name: "cap-xor-cap"}
	p.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "cap-xor-prof"}

	err := k8sClient.Create(context.Background(), p)
	if err == nil {
		_ = k8sClient.Delete(context.Background(), p)
		t.Fatal("the API server accepted capabilityRef AND an inline field on one Pipeline")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("refused, but not by the CEL rule: %v", err)
	}
}

// 1.3/1.7 — THE REQUIREMENT PHASE 1 EXISTS FOR: a Pipeline inlining a
// capability and one referencing the SAME fields through an AgentCapability
// resolve to byte-identical conversations. dispatch.ResolveCapability is what
// makes the two indistinguishable to every consumer downstream of it.
func TestInlineAndReferencedPipelinesResolveIdenticalCapabilities(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "cap-parity-prof")
	mkRuntime(t, "cap-parity-rt", "example/agent:parity", "")
	mkToolset(t, "cap-parity-ts", "Read", "Grep")
	mkMCPConfig(t, "cap-parity-mc", "parity-server", "http://parity/sse")

	capability := mkCapability(t, "cap-parity", "cap-parity-prof")
	capability.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "cap-parity-rt"}
	capability.Spec.ServiceAccountName = "cap-parity-sa"
	capability.Spec.Toolsets = bindTools("cap-parity-ts")
	capability.Spec.MCPConfigs = bind("cap-parity-mc")
	if err := k8sClient.Update(ctx, capability); err != nil {
		t.Fatal(err)
	}
	if c := reconcileCapability(t, "cap-parity"); !apimeta.IsStatusConditionTrue(c.Status.Conditions, "Ready") {
		t.Fatalf("capability must be Ready: %+v", c.Status.Conditions)
	}

	mkSignalSource(t, "cap-parity-src-inline", "cap-parity-sig", "")
	mkSignalSource(t, "cap-parity-src-ref", "cap-parity-sig", "")

	inline := &agentopsv1alpha1.Pipeline{}
	inline.Name, inline.Namespace = "cap-parity-pipe-inline", ns
	inline.Spec.SignalSourceRefs = []agentopsv1alpha1.ObjectRef{{Name: "cap-parity-src-inline"}}
	inline.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "cap-parity-prof"}
	inline.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "cap-parity-rt"}
	inline.Spec.ServiceAccountName = "cap-parity-sa"
	inline.Spec.Toolsets = bindTools("cap-parity-ts")
	inline.Spec.MCPConfigs = bind("cap-parity-mc")
	if err := k8sClient.Create(ctx, inline); err != nil {
		t.Fatal(err)
	}

	referenced := &agentopsv1alpha1.Pipeline{}
	referenced.Name, referenced.Namespace = "cap-parity-pipe-ref", ns
	referenced.Spec.SignalSourceRefs = []agentopsv1alpha1.ObjectRef{{Name: "cap-parity-src-ref"}}
	referenced.Spec.AgentRef = &agentopsv1alpha1.ObjectRef{Name: "cap-parity"}
	if err := k8sClient.Create(ctx, referenced); err != nil {
		t.Fatal(err)
	}

	if p := reconcilePipeline(t, "cap-parity-pipe-inline"); !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("inline pipeline must be Ready: %+v", p.Status.Conditions)
	}
	if p := reconcilePipeline(t, "cap-parity-pipe-ref"); !apimeta.IsStatusConditionTrue(p.Status.Conditions, "Ready") {
		t.Fatalf("capabilityRef pipeline must be Ready: %+v", p.Status.Conditions)
	}

	h := apiServer().Handler()
	postAndFind := func(source, fingerprint string) *agentopsv1alpha1.Conversation {
		rec := postSignal(t, h, testMasterToken, source, []map[string]any{{
			"fingerprint": fingerprint, "labels": map[string]string{"alertname": "ParityAlert"}, "payload": "boom",
		}})
		if rec.Code != 200 {
			t.Fatalf("signal to %s: %d %s", source, rec.Code, rec.Body.String())
		}
		var conv *agentopsv1alpha1.Conversation
		var list agentopsv1alpha1.ConversationList
		if err := k8sClient.List(ctx, &list); err != nil {
			t.Fatal(err)
		}
		for i := range list.Items {
			if list.Items[i].Spec.Signal != nil && list.Items[i].Spec.Signal.SourceRef != nil &&
				list.Items[i].Spec.Signal.SourceRef.Name == source {
				conv = &list.Items[i]
			}
		}
		if conv == nil {
			t.Fatalf("no conversation created from %s", source)
		}
		t.Cleanup(func() { cleanupConversation(t, conv.Name) })
		return conv
	}

	fromInline := postAndFind("cap-parity-src-inline", "cap-parity-inline-1")
	fromRef := postAndFind("cap-parity-src-ref", "cap-parity-ref-1")

	if fromInline.Spec.ProfileRef.Name != fromRef.Spec.ProfileRef.Name {
		t.Fatalf("profile diverged: inline=%q ref=%q", fromInline.Spec.ProfileRef.Name, fromRef.Spec.ProfileRef.Name)
	}
	if fromInline.Spec.ServiceAccountName != fromRef.Spec.ServiceAccountName {
		t.Fatalf("service account diverged: inline=%q ref=%q", fromInline.Spec.ServiceAccountName, fromRef.Spec.ServiceAccountName)
	}
	if fromInline.Spec.RuntimeRef == nil || fromRef.Spec.RuntimeRef == nil ||
		fromInline.Spec.RuntimeRef.Name != fromRef.Spec.RuntimeRef.Name {
		t.Fatalf("runtime diverged: inline=%+v ref=%+v", fromInline.Spec.RuntimeRef, fromRef.Spec.RuntimeRef)
	}
	if fromInline.Spec.Toolsets == nil || fromRef.Spec.Toolsets == nil ||
		len(fromInline.Spec.Toolsets.Refs) != 1 || len(fromRef.Spec.Toolsets.Refs) != 1 ||
		fromInline.Spec.Toolsets.Refs[0].Name != fromRef.Spec.Toolsets.Refs[0].Name {
		t.Fatalf("toolsets diverged: inline=%+v ref=%+v", fromInline.Spec.Toolsets, fromRef.Spec.Toolsets)
	}
	if fromInline.Spec.MCPConfigs == nil || fromRef.Spec.MCPConfigs == nil ||
		len(fromInline.Spec.MCPConfigs.Refs) != 1 || len(fromRef.Spec.MCPConfigs.Refs) != 1 ||
		fromInline.Spec.MCPConfigs.Refs[0].Name != fromRef.Spec.MCPConfigs.Refs[0].Name {
		t.Fatalf("mcpConfigs diverged: inline=%+v ref=%+v", fromInline.Spec.MCPConfigs, fromRef.Spec.MCPConfigs)
	}
}

// Coordinator: the CRD schema and Ready validation. Phase 2 of
// coordinated-agents (design D-B), the schema-and-reconciler slice only — a
// Coordinator nobody applies or that nothing yet reads changes nothing:
// routing, `invoke` and the rest of D-B onward land in a later commit on this
// branch.
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

func reconcileCoordinator(t *testing.T, name string) *agentopsv1alpha1.Coordinator {
	t.Helper()
	rc := &controller.CoordinatorReconciler{Client: k8sClient}
	if _, err := rc.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
		t.Fatal(err)
	}
	var co agentopsv1alpha1.Coordinator
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &co); err != nil {
		t.Fatal(err)
	}
	return &co
}

// 2.1/2.4 — a Coordinator with a valid inline capability and no agents[] is
// Ready: an entry-less Coordinator is inert, not invalid, exactly as an
// unreferenced AgentCapability is.
func TestCoordinatorWithInlineCapabilityIsReady(t *testing.T) {
	mkProfile(t, "co-prof-ok")
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-ok", ns
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-prof-ok"}
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	if got := reconcileCoordinator(t, "co-ok"); !apimeta.IsStatusConditionTrue(got.Status.Conditions, "Ready") {
		t.Fatalf("valid inline capability, no agents: %+v", got.Status.Conditions)
	}
}

// 2.1/2.4 — a Coordinator naming its OWN capability through capabilityRef
// (never inlining it) is Ready once the referenced AgentCapability itself is.
func TestCoordinatorWithCapabilityRefIsReady(t *testing.T) {
	mkProfile(t, "co-capref-prof")
	mkCapability(t, "co-capref-cap", "co-capref-prof")
	if c := reconcileCapability(t, "co-capref-cap"); !apimeta.IsStatusConditionTrue(c.Status.Conditions, "Ready") {
		t.Fatalf("capability must be Ready: %+v", c.Status.Conditions)
	}

	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-capref-ok", ns
	co.Spec.AgentRef = &agentopsv1alpha1.ObjectRef{Name: "co-capref-cap"}
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	if got := reconcileCoordinator(t, "co-capref-ok"); !apimeta.IsStatusConditionTrue(got.Status.Conditions, "Ready") {
		t.Fatalf("valid capabilityRef, no agents: %+v", got.Status.Conditions)
	}
}

// 2.1/2.4 — a Coordinator's own capabilityRef naming a capability that does
// not exist fails Ready naming it, distinctly from the not-ready case below.
func TestCoordinatorCapabilityRefDanglingFailsReady(t *testing.T) {
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-capref-dangling", ns
	co.Spec.AgentRef = &agentopsv1alpha1.ObjectRef{Name: "no-such-capability"}
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	got := reconcileCoordinator(t, "co-capref-dangling")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" ||
		!strings.Contains(ready.Message, "agentcapability/no-such-capability") ||
		strings.Contains(ready.Message, "not ready") {
		t.Fatalf("dangling capabilityRef not surfaced: %+v", got.Status.Conditions)
	}
}

// 2.1/2.4 — a Coordinator's own capabilityRef naming a capability that
// exists but is not itself Ready fails Ready too, naming the capability as
// not ready rather than missing.
func TestCoordinatorCapabilityRefNotReadyFailsReady(t *testing.T) {
	mkCapability(t, "co-capref-not-ready-cap", "no-such-profile")

	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-capref-not-ready", ns
	co.Spec.AgentRef = &agentopsv1alpha1.ObjectRef{Name: "co-capref-not-ready-cap"}
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	got := reconcileCoordinator(t, "co-capref-not-ready")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" ||
		!strings.Contains(ready.Message, "agentcapability/co-capref-not-ready-cap not ready") {
		t.Fatalf("not-ready capabilityRef not surfaced: %+v", got.Status.Conditions)
	}
}

// 2.1/2.4 — neither capabilityRef nor profileRef fails Ready, never
// admission, the same shape a Pipeline already takes.
func TestCoordinatorNeitherCapabilityRefNorProfileRefFailsReady(t *testing.T) {
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-no-capability", ns
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatalf("admission must accept this — Ready is where it fails: %v", err)
	}
	got := reconcileCoordinator(t, "co-no-capability")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" || !strings.Contains(ready.Message, "profileRef or capabilityRef") {
		t.Fatalf("missing capability not surfaced: %+v", got.Status.Conditions)
	}
}

// 1.2-style CEL, restated on Coordinator — capabilityRef and an inline field
// are mutually exclusive at admission.
func TestCoordinatorCapabilityRefAndInlineFieldsAreMutuallyExclusive(t *testing.T) {
	mkProfile(t, "co-xor-prof")
	mkCapability(t, "co-xor-cap", "co-xor-prof")

	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-cap-xor", ns
	co.Spec.AgentRef = &agentopsv1alpha1.ObjectRef{Name: "co-xor-cap"}
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-xor-prof"}

	err := k8sClient.Create(context.Background(), co)
	if err == nil {
		_ = k8sClient.Delete(context.Background(), co)
		t.Fatal("the API server accepted capabilityRef AND an inline field on one Coordinator")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("refused, but not by the CEL rule: %v", err)
	}
}

// 2.4 — every agents[] entry's ref must resolve, and the message names the
// failing entry, not just the dangling ref.
func TestCoordinatorDanglingAgentEntryFailsReadyNamingTheEntry(t *testing.T) {
	mkProfile(t, "co-agents-prof")
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-dangling-agent", ns
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-agents-prof"}
	co.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{
		Name:          "reviewer",
		Description:   "reviews the diff",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "no-such-capability"},
	}}
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	got := reconcileCoordinator(t, "co-dangling-agent")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" ||
		!strings.Contains(ready.Message, "agents[reviewer]: agentcapability/no-such-capability") {
		t.Fatalf("dangling agent entry not surfaced by name: %+v", got.Status.Conditions)
	}
}

// 2.4 — an agents[] entry naming a capability that exists but is not itself
// Ready also fails, distinctly from a dangling ref.
func TestCoordinatorAgentEntryCapabilityNotReadyFailsReady(t *testing.T) {
	mkCapability(t, "co-agents-not-ready-cap", "no-such-profile")
	mkProfile(t, "co-agents-prof2")
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-agent-not-ready", ns
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-agents-prof2"}
	co.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{
		Name:          "reviewer",
		Description:   "reviews the diff",
		CapabilityRef: &agentopsv1alpha1.ObjectRef{Name: "co-agents-not-ready-cap"},
	}}
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	got := reconcileCoordinator(t, "co-agent-not-ready")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" ||
		!strings.Contains(ready.Message, "agents[reviewer]: agentcapability/co-agents-not-ready-cap not ready") {
		t.Fatalf("not-ready agent capability not surfaced: %+v", got.Status.Conditions)
	}
}

// 2.4 — nesting: an agents[] entry naming another, Ready Coordinator resolves
// and the outer Coordinator is Ready.
func TestCoordinatorNestingResolvesThroughAReadyCoordinator(t *testing.T) {
	mkProfile(t, "co-nest-prof-inner")
	mkProfile(t, "co-nest-prof-outer")

	inner := &agentopsv1alpha1.Coordinator{}
	inner.Name, inner.Namespace = "co-nest-inner", ns
	inner.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-nest-prof-inner"}
	if err := k8sClient.Create(context.Background(), inner); err != nil {
		t.Fatal(err)
	}
	if got := reconcileCoordinator(t, "co-nest-inner"); !apimeta.IsStatusConditionTrue(got.Status.Conditions, "Ready") {
		t.Fatalf("inner coordinator must be Ready: %+v", got.Status.Conditions)
	}

	outer := &agentopsv1alpha1.Coordinator{}
	outer.Name, outer.Namespace = "co-nest-outer", ns
	outer.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-nest-prof-outer"}
	outer.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{
		Name:           "sub",
		Description:    "delegates to the inner coordinator",
		CoordinatorRef: &agentopsv1alpha1.ObjectRef{Name: "co-nest-inner"},
	}}
	if err := k8sClient.Create(context.Background(), outer); err != nil {
		t.Fatal(err)
	}
	if got := reconcileCoordinator(t, "co-nest-outer"); !apimeta.IsStatusConditionTrue(got.Status.Conditions, "Ready") {
		t.Fatalf("outer coordinator nesting a Ready coordinator must be Ready: %+v", got.Status.Conditions)
	}
}

// 2.4/D-B — a Coordinator naming ITSELF as a coordinatorRef entry is the
// simplest static cycle: refused by name rather than recursed into again.
func TestCoordinatorSelfCycleFailsReadyNamingTheCycle(t *testing.T) {
	mkProfile(t, "co-cycle-prof")
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-self-cycle", ns
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-cycle-prof"}
	co.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{
		Name:           "loopback",
		Description:    "names itself",
		CoordinatorRef: &agentopsv1alpha1.ObjectRef{Name: "co-self-cycle"},
	}}
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	got := reconcileCoordinator(t, "co-self-cycle")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" ||
		!strings.Contains(ready.Message, "coordinator/co-self-cycle cycle") {
		t.Fatalf("self-cycle not surfaced: %+v", got.Status.Conditions)
	}
}

// 2.4/D-B — a two-node cycle (A -> B -> A) fails Ready on BOTH ends, each
// reporting the other as not ready through the cycle it detects one hop in.
func TestCoordinatorTwoNodeCycleFailsReadyOnBothEnds(t *testing.T) {
	mkProfile(t, "co-cycle2-prof-a")
	mkProfile(t, "co-cycle2-prof-b")

	a := &agentopsv1alpha1.Coordinator{}
	a.Name, a.Namespace = "co-cycle2-a", ns
	a.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-cycle2-prof-a"}
	a.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{
		Name: "b", Description: "invokes b", CoordinatorRef: &agentopsv1alpha1.ObjectRef{Name: "co-cycle2-b"},
	}}
	if err := k8sClient.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}

	b := &agentopsv1alpha1.Coordinator{}
	b.Name, b.Namespace = "co-cycle2-b", ns
	b.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-cycle2-prof-b"}
	b.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{
		Name: "a", Description: "invokes a", CoordinatorRef: &agentopsv1alpha1.ObjectRef{Name: "co-cycle2-a"},
	}}
	if err := k8sClient.Create(context.Background(), b); err != nil {
		t.Fatal(err)
	}

	gotA := reconcileCoordinator(t, "co-cycle2-a")
	if apimeta.IsStatusConditionTrue(gotA.Status.Conditions, "Ready") {
		t.Fatalf("a two-node cycle must not be Ready on either end: a=%+v", gotA.Status.Conditions)
	}
	gotB := reconcileCoordinator(t, "co-cycle2-b")
	if apimeta.IsStatusConditionTrue(gotB.Status.Conditions, "Ready") {
		t.Fatalf("a two-node cycle must not be Ready on either end: b=%+v", gotB.Status.Conditions)
	}
}

// 2.4 — a name held by both a Coordinator and a Pipeline is reported on the
// Coordinator's own Ready (design D-B): addressing cannot tell the two apart.
func TestCoordinatorNameCollisionWithPipelineFailsReady(t *testing.T) {
	mkProfile(t, "co-collide-prof")
	mkPipeline(t, "co-collide", nil, nil, "co-collide-prof")

	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-collide", ns
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-collide-prof"}
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	got := reconcileCoordinator(t, "co-collide")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" ||
		!strings.Contains(ready.Message, "pipeline/co-collide holds the same name") {
		t.Fatalf("name collision with a pipeline not surfaced: %+v", got.Status.Conditions)
	}
}

// 2.1 — signalSourceRefs and channelRefs resolve exactly as a Pipeline's own,
// the same missing-reference shape.
func TestCoordinatorDanglingSourceAndChannelRefsFailReady(t *testing.T) {
	mkProfile(t, "co-refs-prof")
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-dangling-refs", ns
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-refs-prof"}
	co.Spec.SignalSourceRefs = []agentopsv1alpha1.ObjectRef{{Name: "no-such-source"}}
	co.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "no-such-channel"}}
	if err := k8sClient.Create(context.Background(), co); err != nil {
		t.Fatal(err)
	}
	got := reconcileCoordinator(t, "co-dangling-refs")
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != "False" ||
		!strings.Contains(ready.Message, "signalsource/no-such-source") ||
		!strings.Contains(ready.Message, "channel/no-such-channel") {
		t.Fatalf("dangling source/channel refs not surfaced: %+v", got.Status.Conditions)
	}
}

// 2.1 — an agents[] entry naming neither ref is refused at ADMISSION by CEL,
// never left for Ready to catch, exactly like the top-level capability xor.
func TestCoordinatorAgentEntryNeitherRefIsRefusedAtAdmission(t *testing.T) {
	mkProfile(t, "co-neither-prof")
	co := &agentopsv1alpha1.Coordinator{}
	co.Name, co.Namespace = "co-agent-neither-ref", ns
	co.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "co-neither-prof"}
	co.Spec.Agents = []agentopsv1alpha1.CoordinatorAgentEntry{{
		Name: "neither", Description: "names nothing",
	}}
	err := k8sClient.Create(context.Background(), co)
	if err == nil {
		_ = k8sClient.Delete(context.Background(), co)
		t.Fatal("the API server accepted an agents[] entry naming neither capabilityRef nor coordinatorRef")
	}
	if !strings.Contains(err.Error(), "exactly one of capabilityRef or coordinatorRef") {
		t.Fatalf("refused, but not by the CEL rule: %v", err)
	}
}

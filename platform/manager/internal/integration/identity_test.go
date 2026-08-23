// Execution identity is WIRING: the Pipeline selects what runs a conversation
// and under whose account, the Conversation freezes that at creation, and
// nothing reads the Pipeline again.
//
// Two properties carry the whole change, and they pull in opposite directions
// on purpose:
//
//	the REF is frozen     — re-wiring a Pipeline must not move a running
//	                        conversation onto different cluster power
//	the CONTENT is not    — correcting the AgentRuntime must still heal it
//
// A test that only pinned the first would pass on an implementation that
// snapshotted the whole runtime and stranded every conversation on a bad image.
package integration

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// mkRuntime creates an AgentRuntime carrying an image and, optionally, its own
// service account — the rung a Pipeline's account overrides.
func mkRuntime(t *testing.T, name, image, sa string) *agentopsv1alpha1.AgentRuntime {
	t.Helper()
	rt := &agentopsv1alpha1.AgentRuntime{}
	rt.Name, rt.Namespace = name, ns
	rt.Spec.Image = image
	rt.Spec.ServiceAccountName = sa
	if err := k8sClient.Create(context.Background(), rt); err != nil {
		t.Fatal(err)
	}
	return rt
}

// mkWiredPipeline is mkPipeline plus the execution wiring this file is about.
func mkWiredPipeline(t *testing.T, name string, sources []string, profile, runtimeRef, sa string) {
	t.Helper()
	mkPipeline(t, name, sources, nil, profile)
	var p agentopsv1alpha1.Pipeline
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &p); err != nil {
		t.Fatal(err)
	}
	if runtimeRef != "" {
		p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: runtimeRef}
	}
	p.Spec.ServiceAccountName = sa
	if err := k8sClient.Update(context.Background(), &p); err != nil {
		t.Fatal(err)
	}
}

// podFor reconciles a conversation and returns the runtime pod it built.
//
// It registers the CLEANUP too, because these tests are the only ones in the
// suite that build several pods each: the namespace is shared and the
// admission cap is counted from the live pod list, so a pod left behind here
// starves an unrelated capacity test with no hint of where the slot went.
func podFor(t *testing.T, conv string) *corev1.Pod {
	t.Helper()
	t.Cleanup(func() { cleanupConversation(t, conv) })
	reconcile(t, conv)
	var pod corev1.Pod
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: runtimepod.PodName(conv)}, &pod); err != nil {
		t.Fatalf("runtime pod for %s: %v", conv, err)
	}
	return &pod
}

// onlyConv returns the single conversation a test's own profile originated,
// failing loudly on any other count — every assertion below is about ONE.
func onlyConv(t *testing.T, profile string) *agentopsv1alpha1.Conversation {
	t.Helper()
	convs := convsForProfile(t, profile)
	if len(convs) != 1 {
		t.Fatalf("expected exactly one conversation for %s, got %d", profile, len(convs))
	}
	return &convs[0]
}

// 8.1 — the case the change exists for. Two routes, ONE runtime image, two
// identities. Before this, the second trust level needed a cloned AgentRuntime
// whose only difference was the account it named.
func TestTwoPipelinesOneRuntimeTwoIdentities(t *testing.T) {
	mkProfile(t, "id-prof-twoid")
	mkRuntime(t, "id-rt-shared", "example/agent:1.0", "agentops-runtime")
	mkSignalSource(t, "id-src-observe", "id-sig-observe", "")
	mkSignalSource(t, "id-src-actor", "id-sig-actor", "")
	mkWiredPipeline(t, "id-observe-pipe", []string{"id-src-observe"}, "id-prof-twoid", "id-rt-shared", "agentops-runtime-observer")
	mkWiredPipeline(t, "id-act-pipe", []string{"id-src-actor"}, "id-prof-twoid", "id-rt-shared", "agentops-runtime-actor")
	reconcilePipeline(t, "id-observe-pipe")
	reconcilePipeline(t, "id-act-pipe")
	srv := apiServer()

	if rec := taskSignal(t, srv, "id-src-observe", "id-observe-1", "look", nil); rec.Code != 200 {
		t.Fatalf("observe signal: %d %s", rec.Code, rec.Body.String())
	}
	if rec := taskSignal(t, srv, "id-src-actor", "id-act-1", "fix", nil); rec.Code != 200 {
		t.Fatalf("act signal: %d %s", rec.Code, rec.Body.String())
	}

	byAccount := map[string]string{}
	for _, c := range convsForProfile(t, "id-prof-twoid") {
		pod := podFor(t, c.Name)
		if got := pod.Spec.Containers[0].Image; got != "example/agent:1.0" {
			t.Fatalf("%s: image = %q — both routes run ONE image, the account is the difference", c.Name, got)
		}
		byAccount[pod.Spec.ServiceAccountName] = c.Name
	}
	for _, want := range []string{"agentops-runtime-observer", "agentops-runtime-actor"} {
		if byAccount[want] == "" {
			t.Fatalf("no pod ran as %q — got %v", want, byAccount)
		}
	}

	// ...and no second AgentRuntime was needed to express it.
	var runtimes agentopsv1alpha1.AgentRuntimeList
	if err := k8sClient.List(context.Background(), &runtimes); err != nil {
		t.Fatal(err)
	}
	for i := range runtimes.Items {
		if runtimes.Items[i].Spec.Image == "example/agent:1.0" && runtimes.Items[i].Name != "id-rt-shared" {
			t.Fatalf("a clone of rt-shared exists (%s) — carrying an identity is what that clone was for",
				runtimes.Items[i].Name)
		}
	}
}

// 3.4 — THE PRIVILEGE CASE, and the reason the snapshot exists. Editing a
// Pipeline's account must not change what an EXISTING conversation's next pod
// runs as: that would be a privilege change applied to work already in
// progress, decided by an edit made after the work started.
func TestEditingAPipelineDoesNotMoveARunningConversationsIdentity(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "id-prof-frozen")
	mkRuntime(t, "id-rt-frozen", "example/agent:frozen", "")
	mkSignalSource(t, "id-src-frozen", "id-sig-frozen", "")
	mkWiredPipeline(t, "id-frozen-pipe", []string{"id-src-frozen"}, "id-prof-frozen", "id-rt-frozen", "sa-at-creation")
	reconcilePipeline(t, "id-frozen-pipe")
	srv := apiServer()

	if rec := taskSignal(t, srv, "id-src-frozen", "frozen-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := onlyConv(t, "id-prof-frozen")
	if conv.Spec.ServiceAccountName != "sa-at-creation" {
		t.Fatalf("the conversation must snapshot the account: got %q", conv.Spec.ServiceAccountName)
	}

	// Re-wire the Pipeline to a MORE powerful account, as an operator would.
	var p agentopsv1alpha1.Pipeline
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "id-frozen-pipe"}, &p); err != nil {
		t.Fatal(err)
	}
	p.Spec.ServiceAccountName = "sa-escalated"
	p.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "id-rt-elsewhere"}
	if err := k8sClient.Update(ctx, &p); err != nil {
		t.Fatal(err)
	}

	pod := podFor(t, conv.Name)
	if pod.Spec.ServiceAccountName != "sa-at-creation" {
		t.Fatalf("the next pod ran as %q — a Pipeline edit re-wired a conversation already running, "+
			"which is a privilege change applied to work in progress", pod.Spec.ServiceAccountName)
	}
	// The runtime ref is frozen by the same rule — and note `rt-elsewhere` does
	// not exist, so resolving through the Pipeline would not merely be wrong,
	// it would fail the pod build.
	if got := pod.Spec.Containers[0].Image; got != "example/agent:frozen" {
		t.Fatalf("image = %q — the runtime ref is frozen at creation too", got)
	}
}

// 3.5 — the other half, and the one a too-eager snapshot breaks. The REF is
// frozen; the AgentRuntime's CONTENT is not. Correcting a bad image must reach
// conversations that are already running, exactly as correcting a toolset does.
func TestCorrectingTheRuntimeReachesExistingConversations(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "id-prof-heal")
	mkRuntime(t, "id-rt-heal", "example/agent:typo", "")
	mkSignalSource(t, "id-src-heal", "id-sig-heal", "")
	mkWiredPipeline(t, "id-heal-pipe", []string{"id-src-heal"}, "id-prof-heal", "id-rt-heal", "")
	reconcilePipeline(t, "id-heal-pipe")
	srv := apiServer()

	if rec := taskSignal(t, srv, "id-src-heal", "heal-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := onlyConv(t, "id-prof-heal")
	if got := podFor(t, conv.Name).Spec.Containers[0].Image; got != "example/agent:typo" {
		t.Fatalf("image = %q, want the typo it was created with", got)
	}

	var rt agentopsv1alpha1.AgentRuntime
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "id-rt-heal"}, &rt); err != nil {
		t.Fatal(err)
	}
	rt.Spec.Image = "example/agent:fixed"
	rt.Spec.ServiceAccountName = "sa-fixed"
	if err := k8sClient.Update(ctx, &rt); err != nil {
		t.Fatal(err)
	}
	// The pod is rebuilt from the corrected CR — delete the old one first, since
	// the reconciler leaves a healthy pod alone.
	var old corev1.Pod
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: runtimepod.PodName(conv.Name)}, &old); err == nil {
		if err := k8sClient.Delete(ctx, &old); err != nil {
			t.Fatal(err)
		}
	}
	pod := podFor(t, conv.Name)
	if got := pod.Spec.Containers[0].Image; got != "example/agent:fixed" {
		t.Fatalf("image = %q — the REF is frozen, the CONTENT is not, and a fixed runtime must heal "+
			"conversations already running", got)
	}
	// The account is the runtime's CONTENT too, whenever the Pipeline named
	// none. Freezing it at creation would strand every conversation on a name
	// the operator has already corrected.
	if pod.Spec.ServiceAccountName != "sa-fixed" {
		t.Fatalf("service account = %q — with no account on the Pipeline, the runtime's own is content "+
			"and is re-read", pod.Spec.ServiceAccountName)
	}
}

// 8.2 — a Pipeline naming NEITHER field behaves exactly as one that could not
// name them. This is the no-op half of the migration plan, and the property an
// existing install upgrades on.
func TestNamingNeitherFieldChangesNothing(t *testing.T) {
	mkProfile(t, "id-prof-noop")
	mkSignalSource(t, "id-src-noop", "id-sig-noop", "")
	mkPipeline(t, "id-noop-pipe", []string{"id-src-noop"}, nil, "id-prof-noop")
	reconcilePipeline(t, "id-noop-pipe")
	srv := apiServer()

	if rec := taskSignal(t, srv, "id-src-noop", "noop-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := onlyConv(t, "id-prof-noop")
	if conv.Spec.ServiceAccountName != "" {
		t.Fatalf("no account was named anywhere, so none is snapshotted: got %q "+
			"— an empty snapshot is what lets the runtime's own account stay content",
			conv.Spec.ServiceAccountName)
	}
	pod := podFor(t, conv.Name)
	// The suite's bootstrap config names no account, and neither does anything
	// this test created — so the pod spec carries none and the API server
	// defaults it, exactly as it did before these fields existed. `default` is
	// KUBERNETES' default here, not the chart's.
	if pod.Spec.ServiceAccountName != "default" {
		t.Fatalf("service account = %q — with nothing named anywhere the pod must be built with none, "+
			"which is what the API server then defaults", pod.Spec.ServiceAccountName)
	}
}

// 7.2 — THE DUAL READ, pinned working. A profile applied before the upgrade
// keeps dispatching to the runtime it named, for one release.
//
// Its counterpart is TestDeprecatedProfileRuntimeRefIsGone below: when the
// field goes, this test is deleted and that one starts passing. The removal is
// visible from both sides on purpose.
func TestDeprecatedProfileRuntimeRefStillDispatches(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "id-prof-deprecated")
	mkRuntime(t, "id-rt-deprecated", "example/agent:deprecated", "sa-deprecated")
	var prof agentopsv1alpha1.AgentProfile
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "id-prof-deprecated"}, &prof); err != nil {
		t.Fatal(err)
	}
	prof.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "id-rt-deprecated"}
	if err := k8sClient.Update(ctx, &prof); err != nil {
		t.Fatal(err)
	}
	mkSignalSource(t, "id-src-deprecated", "id-sig-deprecated", "")
	// The Pipeline names NO runtime — which is the shape of an install that has
	// not migrated yet.
	mkPipeline(t, "id-deprecated-pipe", []string{"id-src-deprecated"}, nil, "id-prof-deprecated")
	reconcilePipeline(t, "id-deprecated-pipe")
	srv := apiServer()

	if rec := taskSignal(t, srv, "id-src-deprecated", "dep-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := onlyConv(t, "id-prof-deprecated")
	// Resolved at creation, so a later profile edit does not move it either.
	if conv.Spec.RuntimeRef == nil || conv.Spec.RuntimeRef.Name != "id-rt-deprecated" {
		t.Fatalf("the deprecated ref must resolve INTO the snapshot: %+v", conv.Spec.RuntimeRef)
	}
	pod := podFor(t, conv.Name)
	if got := pod.Spec.Containers[0].Image; got != "example/agent:deprecated" {
		t.Fatalf("image = %q — a profile applied before the upgrade must keep working for one release", got)
	}
	if pod.Spec.ServiceAccountName != "sa-deprecated" {
		t.Fatalf("service account = %q, want the deprecated runtime's own", pod.Spec.ServiceAccountName)
	}
}

// ...and the Pipeline WINS over it, so adopting the new model needs no profile
// edit. An install can move route by route while the old field is still set.
func TestThePipelineWinsOverTheDeprecatedProfileRef(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "id-prof-bothrefs")
	mkRuntime(t, "id-rt-old", "example/agent:old", "")
	mkRuntime(t, "id-rt-new", "example/agent:new", "")
	var prof agentopsv1alpha1.AgentProfile
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "id-prof-bothrefs"}, &prof); err != nil {
		t.Fatal(err)
	}
	prof.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "id-rt-old"}
	if err := k8sClient.Update(ctx, &prof); err != nil {
		t.Fatal(err)
	}
	mkSignalSource(t, "id-src-bothrefs", "id-sig-bothrefs", "")
	mkWiredPipeline(t, "id-bothrefs-pipe", []string{"id-src-bothrefs"}, "id-prof-bothrefs", "id-rt-new", "")
	reconcilePipeline(t, "id-bothrefs-pipe")
	srv := apiServer()

	if rec := taskSignal(t, srv, "id-src-bothrefs", "both-1", "work", nil); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	conv := onlyConv(t, "id-prof-bothrefs")
	if got := podFor(t, conv.Name).Spec.Containers[0].Image; got != "example/agent:new" {
		t.Fatalf("image = %q — the Pipeline sits ABOVE the deprecated profile ref, so adopting the "+
			"new model needs no profile edit", got)
	}
}

// THE REMOVAL MUST BE VISIBLE. This test is SKIPPED while the deprecated field
// exists and asserts it is gone. When AgentProfileSpec.RuntimeRef is deleted:
//
//  1. this file stops compiling at the line below — that is the point
//  2. remove the skip and the reference, delete
//     TestDeprecatedProfileRuntimeRefStillDispatches, and delete
//     runtimepod.deprecatedProfileRuntime and the profile rung in SnapshotFor
//
// A "remove it later" note in a proposal is what this replaces: a compile error
// in a named test is the only reminder that cannot be scrolled past.
func TestDeprecatedProfileRuntimeRefIsGone(t *testing.T) {
	t.Skip("AgentProfile.spec.runtimeRef is dual-read for ONE release — delete this skip with the field")

	var prof agentopsv1alpha1.AgentProfile
	if prof.Spec.RuntimeRef != nil {
		t.Fatal("unreachable while skipped; the reference above is the tripwire")
	}
}

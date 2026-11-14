// Namespace split (split-conversation-namespace): placement when the
// control and conversations namespaces are GENUINELY distinct real
// namespaces in this envtest cluster — decision 1's placement rule and
// decision 2's one-code-path promise, pinned here at the envtest level.
// Every other test in this package builds a ConversationReconciler with
// Namespaces.Control == Namespaces.Conversations (the suite-wide `ns`); this
// file is the one place that is deliberately not true.
package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

var splitNSCounter int

// createNamespace creates a fresh, real namespace in the envtest cluster —
// a second one beside the suite-wide `ns`, standing in for a release's
// conversations namespace. Deleted on cleanup; envtest runs no namespace
// controller, so the delete itself is best-effort (the API server accepts
// it and nothing later depends on it actually finalizing).
func createNamespace(t *testing.T) string {
	t.Helper()
	splitNSCounter++
	name := fmt.Sprintf("conv-split-%d-%d", os.Getpid(), splitNSCounter)
	obj := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := k8sClient.Create(context.Background(), obj); err != nil {
		t.Fatalf("create namespace %q: %v", name, err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
	return name
}

// splitStores opens a FRESH kube backend over the given pair. The
// suite-wide `stores` var is always opened equal (ns, ns) — a split test
// needs its own, independently scoped, over the same underlying
// WithWatch client production itself opens one from.
func splitStores(t *testing.T, pair namespace.Namespaces) state.Stores {
	t.Helper()
	backend, err := kube.OpenStores(storeClient, pair)
	if err != nil {
		t.Fatalf("OpenStores(%+v): %v", pair, err)
	}
	return backend.Stores
}

// splitReconciler is reconcilerWithCap's own split-aware sibling: a store
// and a Namespaces pair that is not forced equal.
func splitReconciler(t *testing.T, pair namespace.Namespaces) (*controller.ConversationReconciler, state.Stores) {
	t.Helper()
	st := splitStores(t, pair)
	rc := &controller.ConversationReconciler{
		Client:        k8sClient,
		Conversations: st.Conversations, Runs: st.Runs, Blobs: st.Blobs,
		Namespaces:             pair,
		Scheme:                 scheme,
		MaxActiveConversations: 100,
		Runtime: runtimepod.Config{
			Image: "busybox:stub", ServiceAccount: "default",
			ControlURL: "http://manager:8080", IdleTTLMinutes: 1,
		},
	}
	return rc, st
}

func reconcileSplit(t *testing.T, rc *controller.ConversationReconciler, convNS, name string) {
	t.Helper()
	if _, err := rc.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: convNS, Name: name}}); err != nil {
		t.Fatal(err)
	}
}

func podExistsIn(t *testing.T, nsName, conv string) bool {
	t.Helper()
	var pod corev1.Pod
	err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: nsName, Name: runtimepod.PodName(conv)}, &pod)
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	return err == nil
}

func configMapExistsIn(t *testing.T, nsName, conv string) bool {
	t.Helper()
	var cm corev1.ConfigMap
	err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: nsName, Name: controller.MCPConfigMapName(conv)}, &cm)
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	return err == nil
}

// TestSplitNamespacePlacesWorkloadInConversationsWiringInControl pins
// split-conversation-namespace decision 1: a conversation's runtime pod and
// its MCP ConfigMap land in the conversations namespace, it runs as an
// account of that namespace, and every piece of wiring it resolves —
// profile, runtime and MCP config — is read from the control namespace.
// Reconcile only SUCCEEDS here because each wiring Get names the control
// namespace: every one of these fixtures exists ONLY there, never in the
// conversations namespace, so a reconciler reading the wrong plane would
// fail this test with NotFound rather than silently passing.
func TestSplitNamespacePlacesWorkloadInConversationsWiringInControl(t *testing.T) {
	ctx := context.Background()
	convNS := createNamespace(t)
	pair := namespace.Namespaces{Control: ns, Conversations: convNS}

	mkProfile(t, "split-wire-profile") // control namespace (ns)
	mkMCPConfig(t, "split-wire-mcp", "k8s", "http://mcp.example/sse")

	rt := &agentopsv1alpha1.AgentRuntime{}
	rt.Name, rt.Namespace = "split-wire-rt", ns
	rt.Spec.Image = "example/split-wire-runtime:1.0"
	if err := k8sClient.Create(ctx, rt); err != nil {
		t.Fatal(err)
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "split-wire-profile"}, &profile); err != nil {
		t.Fatal(err)
	}
	profile.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "split-wire-rt"}
	if err := k8sClient.Update(ctx, &profile); err != nil {
		t.Fatal(err)
	}

	rc, _ := splitReconciler(t, pair)

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "split-wire", convNS
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "split-wire-profile"}
	conv.Spec.MCPConfigs = bind("split-wire-mcp")
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "do things"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}

	reconcileSplit(t, rc, convNS, "split-wire")

	if !podExistsIn(t, convNS, "split-wire") {
		t.Fatal("the runtime pod must be in the conversations namespace")
	}
	if podExistsIn(t, ns, "split-wire") {
		t.Fatal("the runtime pod must NOT be in the control namespace")
	}
	if !configMapExistsIn(t, convNS, "split-wire") {
		t.Fatal("the MCP ConfigMap must be in the conversations namespace")
	}
	if configMapExistsIn(t, ns, "split-wire") {
		t.Fatal("the MCP ConfigMap must NOT be in the control namespace")
	}

	var pod corev1.Pod
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: convNS, Name: runtimepod.PodName("split-wire")}, &pod); err != nil {
		t.Fatal(err)
	}
	if pod.Namespace != convNS {
		t.Fatalf("pod namespace = %q, want %q", pod.Namespace, convNS)
	}
	if pod.Spec.ServiceAccountName != "default" {
		t.Fatalf("pod runs as %q, want the account named in its OWN namespace", pod.Spec.ServiceAccountName)
	}
	// decision 1/3: the kube backend's synthetic ownerRef stays valid because
	// owner and dependent share a namespace — never a cross-namespace ref.
	if len(pod.OwnerReferences) != 1 || pod.OwnerReferences[0].Kind != "Conversation" {
		t.Fatalf("ownerRef: %+v", pod.OwnerReferences)
	}
	// the runtime wiring was actually read (not merely defaulted): the image
	// is the one split-wire-rt names, not the reconciler's own busybox default.
	if got := pod.Spec.Containers[0].Image; got != "example/split-wire-runtime:1.0" {
		t.Fatalf("runtime image = %q, want the AgentRuntime read from the control namespace", got)
	}
}

// TestSplitNamespacePipelineClaimRendersInConversations pins the claims row
// of decision 1's placement table: a Pipeline is control-plane wiring, read
// from its own (control) namespace, but the claim it causes to exist is a
// workload sharing a runtime pod's claims, and renders in Conversations.
func TestSplitNamespacePipelineClaimRendersInConversations(t *testing.T) {
	ctx := context.Background()
	convNS := createNamespace(t)

	mkProfile(t, "split-claim-profile")
	mkPipeline(t, "split-claim-pipe", nil, nil, "split-claim-profile")
	bindPersistence(t, "split-claim-pipe", &agentopsv1alpha1.PersistenceBinding{VolumeName: "pv-split-claim"})

	rc := &controller.PipelineReconciler{Client: k8sClient, ConversationsNamespace: convNS}
	if _, err := rc.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "split-claim-pipe"}}); err != nil {
		t.Fatal(err)
	}

	claimName := runtimepod.PipelineClaimName("split-claim-pipe", runtimepod.VolumeContext)
	var claim corev1.PersistentVolumeClaim
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: convNS, Name: claimName}, &claim); err != nil {
		t.Fatalf("claim must render in the conversations namespace: %v", err)
	}
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: claimName}, &corev1.PersistentVolumeClaim{}); err == nil {
		t.Fatal("claim must NOT render in the control (Pipeline's own) namespace")
	} else if !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
}

// TestSplitNamespaceCloseReleasesPodAndConfigMapInConversations pins
// decision 1/3: closing a conversation releases its pod and MCP ConfigMap
// explicitly, in the conversations namespace, under the kube backend.
func TestSplitNamespaceCloseReleasesPodAndConfigMapInConversations(t *testing.T) {
	ctx := context.Background()
	convNS := createNamespace(t)
	pair := namespace.Namespaces{Control: ns, Conversations: convNS}

	mkProfile(t, "split-close-profile")
	mkMCPConfig(t, "split-close-mcp", "k8s", "http://mcp.example/sse")
	rc, _ := splitReconciler(t, pair)

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "split-close", convNS
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "split-close-profile"}
	conv.Spec.MCPConfigs = bind("split-close-mcp")
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	reconcileSplit(t, rc, convNS, "split-close")
	if !podExistsIn(t, convNS, "split-close") || !configMapExistsIn(t, convNS, "split-close") {
		t.Fatal("setup: expected a provisioned pod and ConfigMap before closing")
	}

	var got agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: convNS, Name: "split-close"}, &got); err != nil {
		t.Fatal(err)
	}
	now := metav1.Now()
	got.Status.Phase, got.Status.ClosedAt = agentopsv1alpha1.ConversationClosed, &now
	if err := k8sClient.Status().Update(ctx, &got); err != nil {
		t.Fatal(err)
	}
	reconcileSplit(t, rc, convNS, "split-close")

	if podExistsIn(t, convNS, "split-close") {
		t.Error("a closed conversation must have no runtime pod in the conversations namespace")
	}
	if configMapExistsIn(t, convNS, "split-close") {
		t.Error("a closed conversation must have no MCP ConfigMap in the conversations namespace")
	}
	if getSplitConv(t, convNS, "split-close").Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Error("the closed object must survive its release")
	}
}

func getSplitConv(t *testing.T, convNS, name string) *agentopsv1alpha1.Conversation {
	t.Helper()
	var got agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: convNS, Name: name}, &got); err != nil {
		t.Fatal(err)
	}
	return &got
}

// TestOrphanSweeperDeletesAnUnrecordedPodInTheConversationsNamespace pins
// decision 1/3's last clause: the orphan sweeper lists the conversations
// namespace, and a workload with an identical label in the CONTROL
// namespace — which the sweeper never lists — is untouched.
func TestOrphanSweeperDeletesAnUnrecordedPodInTheConversationsNamespace(t *testing.T) {
	ctx := context.Background()
	convNS := createNamespace(t)
	pair := namespace.Namespaces{Control: ns, Conversations: convNS}
	st := splitStores(t, pair)

	mkOrphanPod := func(nsName, conv string) *corev1.Pod {
		p := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: runtimepod.PodName(conv), Namespace: nsName,
				Labels: map[string]string{runtimepod.LabelApp: runtimepod.LabelAppValue, runtimepod.LabelConversation: conv},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "runtime", Image: "busybox:stub"}}},
		}
		if err := k8sClient.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// An orphan: a runtime pod in the CONVERSATIONS namespace with no
	// conversation record behind it.
	orphan := mkOrphanPod(convNS, "split-orphan")

	// A same-labelled pod in the CONTROL namespace, standing for a workload
	// the sweeper must never reach: it is scoped to Conversations alone.
	control := mkOrphanPod(ns, "split-control-orphan")
	t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), control, client.GracePeriodSeconds(0)) })

	sweeper := &controller.OrphanSweeper{Client: k8sClient, Conversations: st.Conversations, Namespace: convNS}
	// envtest's API server sets CreationTimestamp itself (it cannot be
	// backdated on Create as the fake-client unit test does), so a FUTURE
	// `now` stands in for "old enough to judge" instead — Sweep's own
	// signature exists for exactly this.
	future := time.Now().Add(10 * time.Minute)
	n, err := sweeper.Sweep(ctx, future)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted %d objects, want 1 (the orphan in the conversations namespace)", n)
	}
	if podExistsIn(t, convNS, "split-orphan") {
		t.Error("the orphaned pod in the conversations namespace must be gone")
	}
	if !podExistsIn(t, ns, "split-control-orphan") {
		t.Error("a same-named orphan in the CONTROL namespace must be untouched: the sweeper is scoped to Conversations alone")
	}
	_ = orphan
}

// TestEqualNamespacesReproduceTodaysPlacement pins split-conversation-
// namespace's one-code-path promise (design decision 2) at the envtest
// level: a pair built the way cmd/manager/main.go builds one when
// CONVERSATION_NAMESPACE is unset — namespace.Resolve(control, "") —
// collapses to one namespace, and a conversation's reconcile places every
// object there, exactly as it did before this change. No branch anywhere
// in ConversationReconciler reads Namespaces.Distinct() to decide behavior
// — this is the fact that makes that true for THIS reconcile, not an
// assumption about it.
func TestEqualNamespacesReproduceTodaysPlacement(t *testing.T) {
	ctx := context.Background()
	pair := namespace.Resolve(ns, "")
	if pair.Distinct() {
		t.Fatalf("an unset CONVERSATION_NAMESPACE must resolve to one namespace, got %+v", pair)
	}
	if pair.Control != ns || pair.Conversations != ns {
		t.Fatalf("Resolve(%q, \"\") = %+v, want both fields set to %q", ns, pair, ns)
	}

	mkProfile(t, "equal-ns-profile")
	rc, _ := splitReconciler(t, pair)

	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "equal-ns-conv", ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "equal-ns-profile"}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "x"}}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	reconcileSplit(t, rc, ns, "equal-ns-conv")

	if !podExistsIn(t, ns, "equal-ns-conv") {
		t.Fatal("the pod must land in the one namespace both fields name")
	}
}

// The externally-served SignalAdapter: two adapter identities, ONE pod.
//
// A chat transport is inherently a surface AND an originator. Before this mode
// existed, declaring both identities meant two Deployments, one of which was an
// idle pod whose only job was to make a source Served — the exact shape this
// repo already paid for with telegram-router. These tests pin that the second
// pod does not come back.
package integration

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
)

// mkServedSignalAdapter creates a SignalAdapter with no image, served by a
// ChannelAdapter's workload.
func mkServedSignalAdapter(t *testing.T, name, servingAdapter string) {
	t.Helper()
	a := &agentopsv1alpha1.SignalAdapter{}
	a.Name, a.Namespace = name, ns
	a.Spec.ServedBy = &agentopsv1alpha1.AdapterRef{Kind: "ChannelAdapter", Name: servingAdapter}
	if err := k8sClient.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
}

func signalAdapterOf(t *testing.T, name string) *agentopsv1alpha1.SignalAdapter {
	t.Helper()
	var a agentopsv1alpha1.SignalAdapter
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &a); err != nil {
		t.Fatal(err)
	}
	return &a
}

func deploymentMissing(t *testing.T, name string) bool {
	t.Helper()
	var d appsv1.Deployment
	err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &d)
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	return apierrors.IsNotFound(err)
}

// 2.1/2.2: exactly-one-of is enforced by the API, not by the reconciler — a CR
// that declares both, or neither, never reaches a controller at all.
func TestSignalAdapterRequiresExactlyOneOfImageOrServedBy(t *testing.T) {
	ctx := context.Background()

	both := &agentopsv1alpha1.SignalAdapter{}
	both.Name, both.Namespace = "sb-both", ns
	both.Spec.Image = "example/adapter:1"
	both.Spec.ServedBy = &agentopsv1alpha1.AdapterRef{Kind: "ChannelAdapter", Name: "console"}
	if err := k8sClient.Create(ctx, both); err == nil {
		t.Fatal("an adapter declaring both an image and servedBy must be refused")
	}

	neither := &agentopsv1alpha1.SignalAdapter{}
	neither.Name, neither.Namespace = "sb-neither", ns
	if err := k8sClient.Create(ctx, neither); err == nil {
		t.Fatal("an adapter declaring neither an image nor servedBy must be refused")
	}
}

// 2.2/2.5: no workload of any kind, and Ready=True with reason ServedBy — an
// adapter with nothing to become available must not read as a fault.
func TestExternallyServedAdapterCreatesNoWorkload(t *testing.T) {
	ctx := context.Background()
	mkChannelAdapter(t, "sb-host")
	mkServedSignalAdapter(t, "sb-served", "sb-host")
	reconcileSignalAdapter(t, "sb-served")

	name := controller.SignalAdapterDeploymentName("sb-served")
	if !deploymentMissing(t, name) {
		t.Fatal("an externally-served adapter must own no Deployment")
	}
	var svc corev1.Service
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &svc); !apierrors.IsNotFound(err) {
		t.Fatalf("an externally-served adapter must own no Service: %v", err)
	}
	var sa corev1.ServiceAccount
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &sa); !apierrors.IsNotFound(err) {
		t.Fatalf("an externally-served adapter must own no ServiceAccount: %v", err)
	}

	a := signalAdapterOf(t, "sb-served")
	ready := apimeta.FindStatusCondition(a.Status.Conditions, controller.ConditionReady)
	if ready == nil || ready.Status != metav1.ConditionTrue || ready.Reason != controller.ReasonServedBy {
		t.Fatalf("want Ready=True/ServedBy, got %+v", ready)
	}
}

// 2.4: a source served by an externally-served adapter resolves Served=True
// exactly as one served by a workload-owning adapter. The serving relationship
// is what matters; where the process lives is not the source's business.
func TestSourceOfAnExternallyServedAdapterResolvesServed(t *testing.T) {
	ctx := context.Background()
	mkChannelAdapter(t, "sb-host-served")
	mkServedSignalAdapter(t, "sb-adapter-served", "sb-host-served")
	mkSignalSource(t, "sb-src", "sb-adapter-served", "")
	reconcileSignalAdapter(t, "sb-adapter-served")

	rc := &controller.SignalSourceReconciler{Client: k8sClient}
	if _, err := rc.Reconcile(ctx,
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "sb-src"}}); err != nil {
		t.Fatal(err)
	}
	var src agentopsv1alpha1.SignalSource
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "sb-src"}, &src); err != nil {
		t.Fatal(err)
	}
	if !apimeta.IsStatusConditionTrue(src.Status.Conditions, controller.ConditionServed) {
		t.Fatalf("Served must resolve for an externally-served adapter: %+v", src.Status.Conditions)
	}
}

// 2.3/2.5: both tokens land on ONE pod, they are not equal, and each is a
// stranger to the other surface — the identities share a process, never a scope.
func TestServingPodCarriesBothIdentities(t *testing.T) {
	mkChannelAdapter(t, "sb-two-id")
	mkServedSignalAdapter(t, "sb-two-id-signal", "sb-two-id")
	reconcileAdapter(t, "sb-two-id")

	var deploy appsv1.Deployment
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: controller.AdapterDeploymentName("sb-two-id")}, &deploy); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	wantChannel := chat.DeriveAdapterToken(testMasterToken, "sb-two-id")
	wantSignal := chat.DeriveSignalAdapterToken(testMasterToken, "sb-two-id-signal")
	if env["ADAPTER_TOKEN"] != wantChannel {
		t.Fatalf("channel token missing: %+v", env)
	}
	if env["SIGNAL_ADAPTER_TOKEN"] != wantSignal {
		t.Fatalf("signal token missing: %+v", env)
	}
	if env["SIGNAL_ADAPTER_NAME"] != "sb-two-id-signal" {
		t.Fatalf("the pod must know which signal identity it holds: %+v", env)
	}
	if env["ADAPTER_TOKEN"] == env["SIGNAL_ADAPTER_TOKEN"] {
		t.Fatal("the two identities must not share a token")
	}

	// ONE Deployment despite two identities — the whole point of the mode.
	if !deploymentMissing(t, controller.SignalAdapterDeploymentName("sb-two-id-signal")) {
		t.Fatal("two identities must not produce two Deployments")
	}

	// each token is refused by the other surface
	srv, _ := apiServerWithActivity()
	if rec := adapterReq(srv, "GET", "/signal/sources?adapter=sb-two-id-signal", nil, wantChannel); rec.Code != 401 {
		t.Fatalf("a channel token must not open the signal surface: %d", rec.Code)
	}
	if rec := adapterReq(srv, "GET", "/channel/channels?adapter=sb-two-id", nil, wantSignal); rec.Code != 401 {
		t.Fatalf("a signal token must not open the channel surface: %d", rec.Code)
	}
	// and each opens its own
	if rec := adapterReq(srv, "GET", "/signal/sources?adapter=sb-two-id-signal", nil, wantSignal); rec.Code != 200 {
		t.Fatalf("the signal identity must serve its own surface: %d %s", rec.Code, rec.Body.String())
	}
	if rec := adapterReq(srv, "GET", "/channel/channels?adapter=sb-two-id", nil, wantChannel); rec.Code != 200 {
		t.Fatalf("the channel identity must serve its own surface: %d %s", rec.Code, rec.Body.String())
	}
}

// Clearing the link takes the capability away. A token left behind in a pod
// nobody re-rendered would be a grant with no declaration.
func TestClearingServedByRemovesTheSignalToken(t *testing.T) {
	ctx := context.Background()
	mkChannelAdapter(t, "sb-clear")
	mkServedSignalAdapter(t, "sb-clear-signal", "sb-clear")
	reconcileAdapter(t, "sb-clear")

	a := signalAdapterOf(t, "sb-clear-signal")
	a.Spec.ServedBy = nil
	a.Spec.Image = "example/signal-adapter:1"
	if err := k8sClient.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	reconcileAdapter(t, "sb-clear")

	var deploy appsv1.Deployment
	if err := k8sClient.Get(ctx,
		types.NamespacedName{Namespace: ns, Name: controller.AdapterDeploymentName("sb-clear")}, &deploy); err != nil {
		t.Fatal(err)
	}
	for _, e := range deploy.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "SIGNAL_ADAPTER_TOKEN" {
			t.Fatal("clearing servedBy must remove the injected signal token")
		}
	}

	// and the mode is reversible: with an image again, the workload comes back
	reconcileSignalAdapter(t, "sb-clear-signal")
	if deploymentMissing(t, controller.SignalAdapterDeploymentName("sb-clear-signal")) {
		t.Fatal("supplying an image again must recreate the workload")
	}
}

// Switching INTO the mode must take the old workload away: ownerRef GC will not,
// because the owner still exists, and a leftover Deployment is the second pod
// this mode exists to prevent.
func TestSwitchingToServedByRemovesTheOldWorkload(t *testing.T) {
	ctx := context.Background()
	mkChannelAdapter(t, "sb-switch-host")
	mkSignalAdapter(t, "sb-switch")
	reconcileSignalAdapter(t, "sb-switch")
	if deploymentMissing(t, controller.SignalAdapterDeploymentName("sb-switch")) {
		t.Fatal("setup: the image mode should have produced a workload")
	}

	a := signalAdapterOf(t, "sb-switch")
	a.Spec.Image = ""
	a.Spec.ServedBy = &agentopsv1alpha1.AdapterRef{Kind: "ChannelAdapter", Name: "sb-switch-host"}
	if err := k8sClient.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	reconcileSignalAdapter(t, "sb-switch")
	if !deploymentMissing(t, controller.SignalAdapterDeploymentName("sb-switch")) {
		t.Fatal("switching to servedBy must remove the workload it used to own")
	}

	// ...but NOT the ServiceAccount. The manager holds no `delete` verb on
	// serviceaccounts by design, so attempting one wedges the reconciler in a
	// permanent forbidden loop — which is exactly how this was found on a live
	// cluster. A leftover SA carries zero RBAC and is GC'd with the adapter CR.
	var sa corev1.ServiceAccount
	if err := k8sClient.Get(ctx,
		types.NamespacedName{Namespace: ns, Name: controller.SignalAdapterDeploymentName("sb-switch")},
		&sa); err != nil {
		t.Fatalf("the ServiceAccount must be left alone (the manager cannot delete one): %v", err)
	}
}

// The manager's own RBAC must keep granting no serviceaccount delete — the
// reconciler above depends on not needing it, and widening the grant to tidy an
// inert object would be the wrong trade.
func TestManagerCannotDeleteServiceAccounts(t *testing.T) {
	out := helmTemplate(t)
	var role string
	for _, doc := range splitDocs(out) {
		if strings.Contains(doc, "\nkind: Role\n") && strings.Contains(doc, "name: agentops-manager") {
			role = stripComments(doc)
		}
	}
	if role == "" {
		t.Fatal("manager Role not rendered")
	}
	lines := strings.Split(role, "\n")
	for i, line := range lines {
		if !strings.Contains(line, `resources: ["serviceaccounts"]`) {
			continue
		}
		if i+1 < len(lines) && strings.Contains(lines[i+1], "delete") {
			t.Fatalf("the manager must not be granted serviceaccount delete:\n%s", lines[i+1])
		}
		return
	}
	t.Fatalf("no serviceaccounts rule found in the manager Role:\n%s", role)
}

// A dangling servedBy is DIAGNOSABLE: without this the adapter would sit Ready
// while nothing holds its token, and its sources would look Served while
// nothing served them.
func TestDanglingServedByIsDiagnosable(t *testing.T) {
	mkServedSignalAdapter(t, "sb-dangling", "no-such-adapter")
	reconcileSignalAdapter(t, "sb-dangling")

	a := signalAdapterOf(t, "sb-dangling")
	ready := apimeta.FindStatusCondition(a.Status.Conditions, controller.ConditionReady)
	if ready == nil || ready.Status != metav1.ConditionFalse {
		t.Fatalf("want Ready=False for a dangling servedBy, got %+v", ready)
	}
	if !strings.Contains(ready.Message, "no-such-adapter") {
		t.Fatalf("the reason must NAME the missing adapter: %q", ready.Message)
	}
}

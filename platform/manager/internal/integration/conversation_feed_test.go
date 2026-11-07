package integration

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// A real manager running the conversation controller, with no test calling
// Reconcile: the store's feed is the only thing that can hand it work. It pins
// the SetupWithManager wiring of design D-6 — a raw source for the
// conversations, a pod watch standing in for Owns — against a real API server.
func TestConversationControllerIsDrivenByTheStoreFeed(t *testing.T) {
	mkProfile(t, "profile-feed")
	mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
		Scheme:                 scheme,
		Cache:                  cache.Options{DefaultNamespaces: map[string]cache.Config{ns: {}}},
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Controller:             ctrlconfig.Controller{SkipNameValidation: boolPtr(true)},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := reconcilerWithCap(nil, 100)
	if err := r.SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = mgr.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
		}
	})

	conv := mkConv(t, "feed-driven", "profile-feed", nil, nil)

	key := types.NamespacedName{Namespace: ns, Name: conv.Name}
	var got agentopsv1alpha1.Conversation
	waitFor(t, "the finalizer and the runtime pod", func() bool {
		if k8sClient.Get(ctx, key, &got) != nil || len(got.Finalizers) == 0 {
			return false
		}
		var pod corev1.Pod
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: runtimepod.PodName(conv.Name)}, &pod)
		return err == nil && metaControlledBy(&pod, conv.Name)
	})

	// A change to the pod reaches its conversation again through the owner map:
	// deleting it makes the controller notice the missing worker and recreate it.
	pod := &corev1.Pod{}
	pod.Name, pod.Namespace = runtimepod.PodName(conv.Name), ns
	if err := k8sClient.Delete(ctx, pod, client.GracePeriodSeconds(0)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the pod recreated after its deletion", func() bool {
		var p corev1.Pod
		return k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: runtimepod.PodName(conv.Name)}, &p) == nil
	})
}

func metaControlledBy(pod *corev1.Pod, convName string) bool {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "Conversation" && ref.Name == convName && ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}

// waitFor polls a condition against the API server within a bound. The
// controller works on its own goroutine, so there is no event to block on.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

package controller

import (
	"context"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

// recordingQueue keeps every request in the order it was added. The real queue
// collapses a key already waiting, which would hide the sequence this test is
// about.
type recordingQueue struct {
	workqueue.TypedRateLimitingInterface[reconcile.Request]
	mu  sync.Mutex
	got []reconcile.Request
}

func (q *recordingQueue) Add(r reconcile.Request) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.got = append(q.got, r)
}

func (q *recordingQueue) names() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []string
	for _, r := range q.got {
		if n := len(out); n == 0 || out[n-1] != r.Name { // the two-write create repeats a key
			out = append(out, r.Name)
		}
	}
	return out
}

func feedReconciler(t *testing.T) (*ConversationReconciler, state.Store) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).Build()
	store := kube.NewStore(c, "ns-a")
	return &ConversationReconciler{Client: c, Store: store, Namespace: "ns-a"}, store
}

func waitForNames(t *testing.T, q *recordingQueue, want ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got := q.names()
		if len(got) >= len(want) {
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("feed order = %v, want %v", got, want)
				}
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("feed delivered %v, want %v", q.names(), want)
}

// The event source the reconciler used to get from the informer: a record that
// exists when the controller starts is reconciled, and each later create,
// update and delete reaches the queue, in the order the store made them. This is
// the lifecycle sequence the change promises is unchanged by the move to the
// store's feed.
func TestFeedDeliversTheLifecycleInOrder(t *testing.T) {
	r, store := feedReconciler(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, err := store.Create(ctx, "early", state.ConversationRecord{Name: "early", ProfileRef: "p"}); err != nil {
		t.Fatal(err)
	}
	q := &recordingQueue{}
	if err := r.startFeed(ctx, q); err != nil {
		t.Fatal(err)
	}
	waitForNames(t, q, "early") // the initial list

	if _, err := store.Create(ctx, "a", state.ConversationRecord{Name: "a", ProfileRef: "p"}); err != nil {
		t.Fatal(err)
	}
	waitForNames(t, q, "early", "a")

	if _, err := state.Mutate(ctx, store, "a", func(c *state.ConversationRecord) error {
		c.Phase = state.PhaseQueued
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, "b", state.ConversationRecord{Name: "b", ProfileRef: "p"}); err != nil {
		t.Fatal(err)
	}
	waitForNames(t, q, "early", "a", "b")

	if err := store.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	waitForNames(t, q, "early", "a", "b", "a")

	for _, req := range q.got {
		if req.Namespace != "ns-a" {
			t.Fatalf("a request must carry the store's namespace, got %+v", req)
		}
	}
}

// The feed ends with its context, and the goroutine behind it with the feed.
func TestFeedStopsWithItsContext(t *testing.T) {
	r, _ := feedReconciler(t)
	ctx, cancel := context.WithCancel(context.Background())
	q := &recordingQueue{}
	done := make(chan struct{})
	go func() {
		r.pumpFeed(ctx, q)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pumpFeed outlived its context")
	}
}

func TestMapPodToOwnerReadsTheControllerReference(t *testing.T) {
	r := &ConversationReconciler{Namespace: "ns-a"}
	owned := r.ownerOf(&state.ConversationRecord{Name: "conv-1", ID: "uid-1"})
	pod := ownedPod(t, r, owned)
	got := r.mapPodToOwner(context.Background(), pod)
	if len(got) != 1 || got[0].Name != "conv-1" || got[0].Namespace != "ns-a" {
		t.Fatalf("got %+v, want the owning conversation", got)
	}
	if r.mapPodToOwner(context.Background(), bareObject()) != nil {
		t.Fatal("a pod with no controller reference maps to nothing")
	}
}

func ownedPod(t *testing.T, r *ConversationReconciler, owner *metav1.PartialObjectMetadata) *corev1.Pod {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns-a"}}
	if err := controllerutil.SetControllerReference(owner, pod, scheme); err != nil {
		t.Fatalf("SetControllerReference with the record-built owner: %v", err)
	}
	ref := metav1.GetControllerOf(pod)
	if ref == nil || ref.UID != "uid-1" || ref.Kind != "Conversation" {
		t.Fatalf("owner reference = %+v, want kind Conversation carrying the record's id", ref)
	}
	return pod
}

func bareObject() *corev1.Pod { return &corev1.Pod{} }

package controller

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/kube"
)

// The Signal reconciler's event source is the SignalStore's feed, the way the
// Conversation reconciler's is the Store's: a signal that exists when the
// controller starts is reconciled, and each later create, update and delete
// reaches the queue in the order the store made them.
func TestSignalFeedDeliversTheLifecycleInOrder(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&agentopsv1alpha1.Signal{}).Build()
	store := kube.NewSignalStore(c, "ns-a")
	r := &SignalReconciler{Signals: store, Namespace: "ns-a"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mk := func(name string) state.Revision {
		rev, err := store.Create(ctx, state.Key(name), state.SignalRecord{SourceRef: "src", InitiationID: name})
		if err != nil {
			t.Fatal(err)
		}
		return rev
	}
	mk("early")
	q := &recordingQueue{}
	if err := r.startFeed(ctx, q); err != nil {
		t.Fatal(err)
	}
	waitForNames(t, q, "early") // the initial list

	rev := mk("a")
	waitForNames(t, q, "early", "a")
	if _, err := store.Update(ctx, "a", rev, state.SignalRecord{Phase: state.SignalPending, Attempts: 1}); err != nil {
		t.Fatal(err)
	}
	mk("b")
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

// signalFeedStub is a SignalStore whose Watch is scripted.
type signalFeedStub struct {
	state.SignalStore
	mu     sync.Mutex
	steps  []func() (<-chan state.SignalEvent, error)
	resume []state.Revision
}

func (f *signalFeedStub) Watch(_ context.Context, resume state.Revision) (<-chan state.SignalEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resume = append(f.resume, resume)
	if len(f.steps) == 0 {
		return nil, errors.New("no more scripted watches")
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	return step()
}

func (f *signalFeedStub) List(context.Context, state.SignalIndex, state.SignalListKey) ([]state.SignalRecord, error) {
	return nil, nil
}

func (f *signalFeedStub) seen() []state.Revision {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]state.Revision(nil), f.resume...)
}

func signalEventsThenClose(evs ...state.SignalEvent) func() (<-chan state.SignalEvent, error) {
	return func() (<-chan state.SignalEvent, error) {
		ch := make(chan state.SignalEvent, len(evs))
		for _, ev := range evs {
			ch <- ev
		}
		close(ch)
		return ch, nil
	}
}

func waitForSignalResumes(t *testing.T, f *signalFeedStub, n int) []state.Revision {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if got := f.seen(); len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("watch opened %v times, want %d", f.seen(), n)
	return nil
}

// The signal feed resumes from the last revision it saw, drops a resume point
// the store rejects and relists, and treats a bookmark as a resume point only.
func TestSignalFeedResumesClearsARejectedPointAndFollowsBookmarks(t *testing.T) {
	stub := &signalFeedStub{steps: []func() (<-chan state.SignalEvent, error){
		signalEventsThenClose(state.SignalEvent{Type: state.EventPut, Key: "a", Resume: "5"},
			state.SignalEvent{Type: state.EventBookmark, Resume: "9"}),
		func() (<-chan state.SignalEvent, error) { return nil, errors.New("too old resource version") },
		signalEventsThenClose(),
	}}
	r := &SignalReconciler{Signals: stub, Namespace: "ns-a"}
	q := &recordingQueue{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.startFeed(ctx, q); err != nil {
		t.Fatal(err)
	}
	got := waitForSignalResumes(t, stub, 3)
	if got[0] != "" || got[1] != "9" || got[2] != "" {
		t.Fatalf("resume points = %q, want [\"\" \"9\" \"\"]", got[:3])
	}
	if names := q.names(); len(names) != 1 || names[0] != "a" {
		t.Fatalf("only the put enqueues, got %v", names)
	}
}

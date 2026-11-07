// Task 2.6: expiry — invisible before gone, eventually swept, observable on
// Watch. Only an Expiring kind has it, and in the inventory that is the signal:
// a conversation's record carries no expiry, so these scenarios run over the
// Signal layout.
package kube

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func openSignals(t testing.TB, c client.WithWatch, ns string, interval time.Duration) *Expiring[state.SignalRecord] {
	t.Helper()
	s, err := OpenExpiring(c, ns, SignalLayout(), WithSweepInterval(interval))
	if err != nil {
		t.Fatalf("OpenExpiring: %v", err)
	}
	return s
}

func expiredSignal(name string, at time.Time) state.SignalRecord {
	return state.SignalRecord{Name: name, SourceRef: "src", Adapter: "cron", InitiationID: name, Phase: state.SignalRouted, ExpiresAt: &at}
}

// TestExpiredRecordInvisibleBeforeSweep is the spec's own "An expired record is
// invisible before it is gone" scenario: Get and every List omit it the instant
// its expiry has passed, with no sweep having run at all (the store never calls
// Run).
func TestExpiredRecordInvisibleBeforeSweep(t *testing.T) {
	ns := newNamespace(t)
	s := openSignals(t, wwClient, ns, DefaultSweepInterval)
	ctx := context.Background()

	rec := expiredSignal("sig-expired", time.Now().Add(-time.Hour))
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, _, err := s.Get(ctx, state.Key(rec.Name)); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("Get on an expired record: got %v, want ErrNotFound", err)
	}

	for _, probe := range []struct {
		idx   state.Index[state.SignalRecord]
		value string
	}{
		{state.SignalAll, ""},
		{state.SignalBySource, "src"},
	} {
		got, err := s.List(ctx, probe.idx, probe.value)
		if err != nil {
			t.Fatalf("List(%s): %v", probe.idx.Name(), err)
		}
		for _, r := range got {
			if r.Name == rec.Name {
				t.Fatalf("List(%s) returned an expired record: %+v", probe.idx.Name(), r)
			}
		}
	}
}

// TestExpiredRecordEventuallySweptAndObservedOnWatch runs Run with a short
// sweep interval and asserts the expired record is (a) actually deleted, not
// merely filtered, and (b) its deletion is observable on Watch, like any other
// delete (spec: "Expiry is observable on the feed").
func TestExpiredRecordEventuallySweptAndObservedOnWatch(t *testing.T) {
	ns := newNamespace(t)
	s := openSignals(t, wwClient, ns, 50*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := expiredSignal("sig-expired-swept", time.Now().Add(-time.Hour))
	rev, err := s.Create(ctx, state.Key(rec.Name), rec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	events, err := s.Watch(ctx, rev)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	go s.Run(ctx)

	ev := recvEvent(t, events)
	if ev.Type != state.EventDelete || ev.Key != state.Key(rec.Name) {
		t.Fatalf("expiry sweep's delete was not observed on Watch: %+v", ev)
	}

	// Confirm the object is genuinely gone from the apiserver, not merely
	// filtered by Get/List's own expiry check: bypass the store and ask the raw
	// client directly.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var sig agentopsv1alpha1.Signal
		err := wwClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: rec.Name}, &sig)
		if apierrors.IsNotFound(err) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expired record was never actually deleted by the sweep (last error: %v)", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The sweep lists the namespace only when something can have expired: a tick
// with nothing armed makes no list call.
func TestSweepSkipsTheNamespaceListUntilAnExpiryIsArmed(t *testing.T) {
	ns := newNamespace(t)
	var lists atomic.Int32
	c := interceptor.NewClient(wwClient, interceptor.Funcs{
		List: func(ctx context.Context, cl client.WithWatch, l client.ObjectList, opts ...client.ListOption) error {
			if _, ok := l.(*agentopsv1alpha1.SignalList); ok {
				lists.Add(1)
			}
			return cl.List(ctx, l, opts...)
		},
	})
	s := openSignals(t, c, ns, 20*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	time.Sleep(300 * time.Millisecond)
	if n := lists.Load(); n != 1 {
		t.Fatalf("idle sweep listed %d times, want exactly the start-up one", n)
	}

	// A future expiry arms the sweep without handing it a record to delete
	// between the create and its status write.
	rec := expiredSignal("sig-arm", time.Now().Add(time.Hour))
	if _, err := s.Create(ctx, state.Key(rec.Name), rec); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for lists.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("a write carrying an expiry did not arm the sweep")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

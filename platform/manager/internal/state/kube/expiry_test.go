// The generic expiry scenarios (invisible before removed, delivered as a
// delete on the feed once swept) now live in internal/state/conformance,
// driven by signalFixture and run through TestStores (stores_test.go). What
// is left here is specific to this backend's own sweep implementation: it
// must not cost the apiserver a list when nothing is armed to expire.
package kube

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
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

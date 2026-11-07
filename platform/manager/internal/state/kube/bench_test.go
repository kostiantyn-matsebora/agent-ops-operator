package kube

import (
	"context"
	"fmt"
	"testing"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state/conformance"
)

// The kube backend's read cost against envtest's real API server: every number
// is one round trip to a LOCAL apiserver backed by a local etcd, with no
// kubelet, no network hop and no other client. They bound the work the
// backend adds per call; they are not cluster latencies (design D-4).

func benchConversation(b *testing.B) (*Store, state.Key) {
	b.Helper()
	_, store := newTestNamespace(b)
	ctx := context.Background()
	key := state.Key("bench-conv")
	if _, err := store.Create(ctx, key, state.ConversationRecord{Name: "bench-conv", ProfileRef: "p"}); err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 5; i++ { // a conversation with some history
		if err := store.AppendRun(ctx, key, state.Run{RunID: fmt.Sprintf("run-%d", i), Status: "Succeeded"}); err != nil {
			b.Fatal(err)
		}
	}
	return store, key
}

// BenchmarkKubeReconcileReadsTwoReads is what Reconcile paid before the
// combined read: Get, then ListRuns.
func BenchmarkKubeReconcileReadsTwoReads(b *testing.B) {
	store, key := benchConversation(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := store.Get(ctx, key); err != nil {
			b.Fatal(err)
		}
		if _, err := store.ListRuns(ctx, key); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkKubeReconcileReadsCombined is the same two facts from GetWithRuns.
func BenchmarkKubeReconcileReadsCombined(b *testing.B) {
	store, key := benchConversation(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, err := store.GetWithRuns(ctx, key); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkKubeSignalStore is conformance.BenchSignals over Signal objects.
func BenchmarkKubeSignalStore(b *testing.B) {
	conformance.BenchSignals(b, func() state.SignalStore {
		ns, _ := newTestNamespace(b)
		return NewSignalStore(wwClient, ns)
	})
}

package kube

import (
	"context"
	"sync/atomic"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// countingClient counts the apiserver reads a store makes.
type countingClient struct {
	client.WithWatch
	gets atomic.Int32
}

func (c *countingClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	c.gets.Add(1)
	return c.WithWatch.Get(ctx, key, obj, opts...)
}

// GetWithRuns is the reconciler's per-pass read: the record and its runs cost
// one apiserver read, where Get then ListRuns cost two.
func TestGetWithRunsIsOneApiserverRead(t *testing.T) {
	ctx := context.Background()
	ns, _ := newTestNamespace(t)
	cc := &countingClient{WithWatch: wwClient}
	store := NewStore(cc, ns)
	if _, err := store.Create(ctx, "c1", state.ConversationRecord{Name: "c1", ProfileRef: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendRun(ctx, "c1", state.Run{RunID: "r1"}); err != nil {
		t.Fatal(err)
	}
	cc.gets.Store(0)
	rec, _, runs, err := store.GetWithRuns(ctx, "c1")
	if err != nil || rec.Name != "c1" || len(runs) != 1 {
		t.Fatalf("GetWithRuns = %+v, %v, %v", rec, runs, err)
	}
	if n := cc.gets.Load(); n != 1 {
		t.Fatalf("GetWithRuns made %d apiserver reads, want 1", n)
	}
}

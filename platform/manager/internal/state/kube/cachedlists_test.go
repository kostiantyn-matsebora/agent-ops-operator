package kube

import (
	"context"
	"sync/atomic"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// countingReader is a client.Reader that answers every List with a fixed
// result and counts the calls, standing in for the informer cache.
type countingReader struct {
	client.Reader
	lists atomic.Int32
}

func (c *countingReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	c.lists.Add(1)
	return c.Reader.List(ctx, list, opts...)
}

// With a cache the store hands List to it and never lists the apiserver
// itself, while Get still reads the apiserver: a lookup may be served from
// memory, a precondition may not. Before this option a close that walked the
// child lookup twice listed 552 conversations (6 MB) from the apiserver each
// time and a console batch passed the gateway's 60 seconds.
func TestCachedListsServeListFromTheReaderAndNothingElse(t *testing.T) {
	_, s := newTestNamespace(t)
	ctx := context.Background()
	createForList(t, ctx, s, "conv-a", "id-1", "pipe-1", "Working")

	cache := &countingReader{Reader: s.client}
	s.WithCachedLists(cache)

	got, err := s.List(ctx, state.ByAll, state.ListKey{})
	if err != nil || len(got) != 1 {
		t.Fatalf("List through the reader = %v, %v; want the one record", got, err)
	}
	if n := cache.lists.Load(); n != 1 {
		t.Fatalf("List called the reader %d times, want 1", n)
	}

	before := cache.lists.Load()
	if _, _, err := s.Get(ctx, state.Key("conv-a")); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cache.lists.Load() != before {
		t.Fatal("Get went through the cached reader; it carries the compare-and-swap and must not")
	}
}

// Without the option the store lists through its own client, as before.
func TestListWithoutACacheUsesTheClient(t *testing.T) {
	_, s := newTestNamespace(t)
	if s.listReader() != client.Reader(s.client) {
		t.Fatal("a store given no cache must list through its client")
	}
}

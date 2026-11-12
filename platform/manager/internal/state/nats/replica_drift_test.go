package nats

import (
	"context"
	"sort"
	"sync"
	"testing"
)

// A bucket's replica count is set when it is created and never changed by the
// manager afterwards (state-store-nats, "The state server has no single point
// of failure by default"). Re-applying a changed NATS_REPLICAS to a live
// bucket would re-replicate it under load, so the backend keeps what exists
// and reports the difference instead.

type driftLog struct {
	mu   sync.Mutex
	seen []string
	have map[string]int
	want map[string]int
}

func (d *driftLog) hook(bucket string, have, want int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.have == nil {
		d.have, d.want = map[string]int{}, map[string]int{}
	}
	d.seen = append(d.seen, bucket)
	d.have[bucket], d.want[bucket] = have, want
}

func replicasOf(t *testing.T, conn *Conn, stream string) int {
	t.Helper()
	s, err := conn.js.Stream(context.Background(), stream)
	if err != nil {
		t.Fatalf("stream %s: %v", stream, err)
	}
	return s.CachedInfo().Config.Replicas
}

func TestAChangedReplicaCountLeavesExistingBucketsUnchangedAndIsReported(t *testing.T) {
	scope := newScope(t)
	first := openBackend(t, sharedTestConn(t), scope)
	first.Close()

	changed, err := Connect(Config{URL: sharedTestConn(t).cfg.URL, Replicas: 3})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(changed.Close)

	var d driftLog
	b, err := OpenStores(context.Background(), changed, scope, WithReplicaDriftHook(d.hook))
	if err != nil {
		t.Fatalf("a changed replica count must not fail the open: %v", err)
	}
	t.Cleanup(b.Close)

	want := []string{
		bucketName(scope, bucketAdapterState), bucketName(scope, bucketConversations),
		bucketName(scope, bucketCooldowns), bucketName(scope, bucketReceipts),
		bucketName(scope, bucketRuns), bucketName(scope, bucketSignals),
		bucketName(scope, objectStoreBlobs),
	}
	sort.Strings(want)
	sort.Strings(d.seen)
	if len(d.seen) != len(want) {
		t.Fatalf("drift reported for %v, want every bucket %v", d.seen, want)
	}
	for i := range want {
		if d.seen[i] != want[i] {
			t.Fatalf("drift reported for %v, want %v", d.seen, want)
		}
	}
	for _, name := range want {
		if d.have[name] != 1 || d.want[name] != 3 {
			t.Errorf("%s: reported have=%d want=%d, want have=1 want=3", name, d.have[name], d.want[name])
		}
	}
	if got := replicasOf(t, changed, kvStreamName(bucketName(scope, bucketConversations))); got != 1 {
		t.Errorf("the conversations bucket now has %d replicas, want it left at 1", got)
	}
}

func TestAnUnchangedReplicaCountReportsNothing(t *testing.T) {
	scope := newScope(t)
	openBackend(t, sharedTestConn(t), scope).Close()

	var d driftLog
	b, err := OpenStores(context.Background(), sharedTestConn(t), scope, WithReplicaDriftHook(d.hook))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	if len(d.seen) != 0 {
		t.Fatalf("drift reported for %v with the count unchanged", d.seen)
	}
}

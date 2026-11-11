package nats

import (
	"context"
	"testing"
)

// A KV Get is answered by JetStream's direct get whenever the stream allows it,
// and a direct get may be served by ANY replica, one that has not yet applied
// the write the reader was just told about. The reconcilers read the record a
// change feed names, and treat not-found as deleted and say nothing, so a
// follower that lagged turned a created record into a missing one and a
// deletion mark into an old record: the change was seen and never acted on.
// Measured on a three-server install: a Signal stored, its feed event
// delivered, and no pass ever made over it. Reads go to the stream leader
// instead, which is linearizable.

func allowDirect(t *testing.T, conn *Conn, stream string) bool {
	t.Helper()
	s, err := conn.js.Stream(context.Background(), stream)
	if err != nil {
		t.Fatalf("stream %s: %v", stream, err)
	}
	return s.CachedInfo().Config.AllowDirect
}

func TestNoBucketAllowsAReadFromAReplicaThatMayLag(t *testing.T) {
	conn := sharedTestConn(t)
	scope := newScope(t)
	check := func(label string) {
		t.Helper()
		var streams []string
		for _, base := range []string{bucketConversations, bucketRuns, bucketSignals, bucketCooldowns, bucketReceipts, bucketAdapterState} {
			streams = append(streams, kvStreamName(bucketName(scope, base)))
		}
		streams = append(streams, objectStreamName(bucketName(scope, objectStoreBlobs)))
		for _, name := range streams {
			if allowDirect(t, conn, name) {
				t.Errorf("%s: %s allows direct reads, which any replica may answer", label, name)
			}
		}
	}

	b := openBackend(t, conn, scope)
	check("after the first open")
	b.Close()

	openBackend(t, conn, scope)
	check("after a second open")
}

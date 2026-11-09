package nats

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A failed open must not leave the mirrors of the buckets it did create
// running, or a caller that retries piles up a set per attempt.
func TestAFailedOpenLeavesNothingRunning(t *testing.T) {
	conn := sharedTestConn(t)
	scope := newScope(t)
	blockStream(t, conn, kvStreamName(bucketName(scope, bucketAdapterState))) // the last bucket to be created

	// The first attempt starts the client's own machinery, once. Measure from
	// after it, so what is counted is what each further attempt leaves behind.
	if _, err := OpenStores(context.Background(), conn, scope); err == nil {
		t.Fatal("OpenStores succeeded although a bucket cannot be created")
	}
	time.Sleep(300 * time.Millisecond)
	runtime.GC()
	before := runtime.NumGoroutine()
	for i := 0; i < 6; i++ {
		if _, err := OpenStores(context.Background(), conn, scope); err == nil {
			t.Fatal("OpenStores succeeded although a bucket cannot be created")
		}
	}
	eventually(t, "the goroutines of the failed attempts ended", func() bool {
		runtime.GC()
		return runtime.NumGoroutine() <= before+4
	})
}

func TestOpenStoresWhenReadyWaitsOutAServerThatCannotYetPlaceABucket(t *testing.T) {
	conn := sharedTestConn(t)
	scope := newScope(t)
	name := kvStreamName(bucketName(scope, bucketSignals))
	blockStream(t, conn, name)
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = conn.js.DeleteStream(context.Background(), name) // quorum, in effect, arrives
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	start := time.Now()
	b, err := OpenStoresWhenReady(ctx, conn, scope, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("OpenStoresWhenReady: %v", err)
	}
	t.Cleanup(b.Close)
	if time.Since(start) < 250*time.Millisecond {
		t.Fatal("it opened before the blocker was lifted")
	}
	if _, err := b.Stores.Signals.Create(ctx, "after-the-wait", signalFixtureForRetry()); err != nil {
		t.Fatalf("the opened backend does not work: %v", err)
	}
}

func TestOpenStoresWhenReadyGivesUpWithTheCallerAndNamesTheLastRefusal(t *testing.T) {
	conn := sharedTestConn(t)
	scope := newScope(t)
	blockStream(t, conn, kvStreamName(bucketName(scope, bucketCooldowns)))
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	_, err := OpenStoresWhenReady(ctx, conn, scope, 0) // zero takes the default interval
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), bucketName(scope, bucketCooldowns)) {
		t.Fatalf("OpenStoresWhenReady = %v, want the deadline naming the bucket that refused", err)
	}
}

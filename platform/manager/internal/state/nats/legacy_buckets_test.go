package nats

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// A bucket this backend no longer opens (an earlier command bus kept its tasks,
// ids and dead letters in three) is removed when the stores open, so nothing
// the code has stopped owning stays on the server forever. Anything else on
// the server is left alone.
func TestOpeningTheStoresRetiresTheBucketsNoCodeOwnsAnyMore(t *testing.T) {
	conn := sharedTestConn(t)
	scope := newScope(t)
	ctx := context.Background()

	for _, base := range legacyBuckets {
		kv, err := conn.js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: bucketName(scope, base)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := kv.Put(ctx, "old", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	// Someone else's bucket, and a current one, in the same scope.
	other, err := conn.js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: bucketName(scope, "somethingelse")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.js.DeleteKeyValue(context.Background(), bucketName(scope, "somethingelse")) })

	b := openBackend(t, conn, scope)

	for _, base := range legacyBuckets {
		if _, err := conn.js.KeyValue(ctx, bucketName(scope, base)); !errors.Is(err, jetstream.ErrBucketNotFound) {
			t.Errorf("legacy bucket %q still exists after open: %v", base, err)
		}
	}
	if _, err := conn.js.KeyValue(ctx, other.Bucket()); err != nil {
		t.Errorf("a bucket the code never owned was removed: %v", err)
	}
	if len(b.Retired) != len(legacyBuckets) {
		t.Errorf("Retired = %v, want every legacy bucket reported so the manager can log it", b.Retired)
	}
	if _, err := conn.js.KeyValue(ctx, bucketName(scope, bucketRuns)); err != nil {
		t.Errorf("a current bucket is missing: %v", err)
	}
}

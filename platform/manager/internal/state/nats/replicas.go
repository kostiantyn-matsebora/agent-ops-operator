package nats

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// A bucket's replica count is set when the bucket is created and never changed
// by the manager afterwards (spec state-store-nats, "The state server has no
// single point of failure by default"). Bootstrap still goes through
// CreateOrUpdate, which validates what exists, but hands it the bucket's OWN
// count when there is one, so a changed NATS_REPLICAS can never re-replicate a
// live bucket under load.

// objectStreamName is the stream NATS gives an object store ("OBJ_<bucket>").
func objectStreamName(bucket string) string { return "OBJ_" + bucket }

// keepExistingReplicas returns the replica count bootstrap must apply to
// bucket: the live stream's count when the bucket exists, want when it does
// not. A difference is logged and handed to the caller's hook, never applied.
// A stream that cannot be read counts as absent, so a create decides.
func (b *Backend) keepExistingReplicas(ctx context.Context, bucket, stream string, want int) int {
	s, err := b.conn.js.Stream(ctx, stream)
	if err != nil {
		return want
	}
	have := s.CachedInfo().Config.Replicas
	if have == want {
		return want
	}
	log.Log.WithName("nats-state").Info(
		"replica count differs from the existing bucket, leaving the bucket unchanged: rebuild the state to change it",
		"bucket", bucket, "have", have, "want", want)
	if b.onDrift != nil {
		b.onDrift(bucket, have, want)
	}
	return have
}

// leaderReadsOnly turns off a stream's direct gets. The client library creates
// every KV bucket and object store with them on, and a direct get may be served
// by any replica, including one that has not applied the write the reader was
// just told about. The reconcilers read the record a change names and treat a
// missing one as deleted, so a lagging follower made them skip a change for
// good. Without direct gets a read goes to the stream leader, which is current.
func leaderReadsOnly(ctx context.Context, js jetstream.JetStream, stream string) error {
	s, err := js.Stream(ctx, stream)
	if err != nil {
		return err
	}
	cfg := s.CachedInfo().Config
	if !cfg.AllowDirect {
		return nil
	}
	cfg.AllowDirect = false
	_, err = js.UpdateStream(ctx, cfg)
	return err
}

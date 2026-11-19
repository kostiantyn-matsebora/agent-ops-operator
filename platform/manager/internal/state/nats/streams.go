package nats

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

// objectStreamName is the stream NATS gives an object store ("OBJ_<bucket>").
func objectStreamName(bucket string) string { return "OBJ_" + bucket }

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

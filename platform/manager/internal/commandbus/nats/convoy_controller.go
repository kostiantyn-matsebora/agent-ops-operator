package nats

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// A convoy queue is the temporary queue of one key on one queue: the standard
// sequential convoy, built from plain JetStream. Messages of the key are
// published to its subject in the queue's stream, and one durable pull
// consumer with MaxAckPending = 1 delivers them one at a time, in order, to
// whichever replica pulls. Different keys have different convoy queues and run
// in parallel.

// Convoy is one registry entry: a convoy queue and everything needed to attach
// to it. It is created once and never rewritten.
type Convoy struct {
	Queue   commandbus.Queue `json:"queue"`
	Key     commandbus.Key   `json:"key"`
	Subject string           `json:"subject"`
	// Consumer is the durable consumer name every replica attaches to.
	Consumer  string    `json:"consumer"`
	CreatedAt time.Time `json:"createdAt"`
	// Revision is the registry revision this value was read at. A delete
	// compares against it, so an entry recreated in between is never removed.
	Revision uint64 `json:"-"`
}

// ConvoyEvent is a change of the registry.
type ConvoyEvent struct {
	Convoy  Convoy
	Deleted bool
}

// convoyController owns the lifecycle of the convoy queues: the registry in
// NATS, kept in one KV bucket, one entry per convoy queue. Ensure is safe on
// any replica: it is one atomic create, so concurrent callers agree on one
// entry. Removal is the sweep's, which runs on the leader only.
type convoyController struct {
	kv   jetstream.KeyValue
	subj subjects
}

func newConvoyController(kv jetstream.KeyValue, subj subjects) *convoyController {
	return &convoyController{kv: kv, subj: subj}
}

func (c *convoyController) subjectFor(q commandbus.Queue, k commandbus.Key) string {
	return c.subj.prefix + ".q." + escapeToken(string(q)) + "." + escapeToken(string(k))
}

func registryKey(q commandbus.Queue, k commandbus.Key) string {
	return escapeToken(string(q)) + "." + escapeToken(string(k))
}

// consumerName is a short, valid JetStream consumer name for a registry key:
// escaped keys may be long.
func consumerName(regKey string) string {
	sum := sha256.Sum256([]byte(regKey))
	return "cq-" + hex.EncodeToString(sum[:12])
}

// Ensure returns the convoy queue of (queue, key), creating its registry entry
// if it has none. Idempotent, atomic, and callable from any replica.
func (c *convoyController) Ensure(ctx context.Context, q commandbus.Queue, k commandbus.Key, now time.Time) (Convoy, error) {
	rk := registryKey(q, k)
	cv := Convoy{Queue: q, Key: k, Subject: c.subjectFor(q, k), Consumer: consumerName(rk), CreatedAt: now.UTC()}
	body, err := json.Marshal(cv)
	if err != nil {
		return Convoy{}, err
	}
	rev, err := c.kv.Create(ctx, rk, body)
	if err == nil {
		cv.Revision = rev
		return cv, nil
	}
	if !errors.Is(err, jetstream.ErrKeyExists) {
		return Convoy{}, fmt.Errorf("commandbus/nats: ensure convoy queue: %w", err)
	}
	return c.get(ctx, rk)
}

func (c *convoyController) get(ctx context.Context, rk string) (Convoy, error) {
	e, err := c.kv.Get(ctx, rk)
	if err != nil {
		return Convoy{}, fmt.Errorf("commandbus/nats: read convoy queue: %w", err)
	}
	return decodeConvoy(e)
}

func decodeConvoy(e jetstream.KeyValueEntry) (Convoy, error) {
	var cv Convoy
	if err := json.Unmarshal(e.Value(), &cv); err != nil {
		return Convoy{}, fmt.Errorf("commandbus/nats: decode convoy queue %q: %w", e.Key(), err)
	}
	cv.Revision = e.Revision()
	return cv, nil
}

// List returns every convoy queue.
func (c *convoyController) List(ctx context.Context) ([]Convoy, error) {
	keys, err := c.kv.Keys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("commandbus/nats: list convoy queues: %w", err)
	}
	out := make([]Convoy, 0, len(keys))
	for _, k := range keys {
		cv, err := c.get(ctx, k)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue // removed between the listing and the read
			}
			return nil, err
		}
		out = append(out, cv)
	}
	return out, nil
}

// Watch delivers every existing convoy queue, then every later change, until
// ctx ends. The channel is closed then.
func (c *convoyController) Watch(ctx context.Context) (<-chan ConvoyEvent, error) {
	w, err := c.kv.WatchAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: watch convoy queues: %w", err)
	}
	out := make(chan ConvoyEvent)
	go func() {
		defer close(out)
		defer w.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-w.Updates():
				if !ok {
					return
				}
				if e == nil {
					continue // the end of the initial values
				}
				var ev ConvoyEvent
				if e.Operation() == jetstream.KeyValueDelete || e.Operation() == jetstream.KeyValuePurge {
					ev = ConvoyEvent{Deleted: true, Convoy: Convoy{Revision: e.Revision()}}
					ev.Convoy.Subject = e.Key() // the registry key names it when no value is left
				} else {
					cv, err := decodeConvoy(e)
					if err != nil {
						continue
					}
					ev = ConvoyEvent{Convoy: cv}
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// Remove deletes a convoy queue's registry entry only if it is still at the
// revision the caller read, so an entry recreated meanwhile survives.
func (c *convoyController) Remove(ctx context.Context, cv Convoy) error {
	err := c.kv.Delete(ctx, registryKey(cv.Queue, cv.Key), jetstream.LastRevision(cv.Revision))
	if err != nil {
		return fmt.Errorf("commandbus/nats: remove convoy queue: %w", err)
	}
	return nil
}

package nats

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// DeadLetters implements commandbus.Bus, reading the "dead" bucket
// directly (package doc): a listing call, not Claim's hot reconcile path,
// so a live scan costs nothing the contract minds, and it is the ONLY
// source that needs to be correct regardless of whether the ledger
// retention sweep (sweep.go) has already purged the matching "tasks"
// record — archiveDead (task.go) writes here the instant a task reaches
// Dead, never waiting for that sweep.
func (b *Bus) DeadLetters(ctx context.Context, queue commandbus.Queue) ([]commandbus.Task, error) {
	lister, err := b.dead.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("commandbus/nats: dead letters: list keys: %w", err)
	}
	var out []commandbus.Task
	for uid := range lister.Keys() {
		entry, err := b.dead.Get(ctx, uid)
		if err != nil {
			continue // raced the retention sweep's own purge; not this listing's job to report
		}
		t, err := decodeTask(entry.Value())
		if err != nil {
			continue
		}
		if queue != "" && t.Queue != queue {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DeadAt.Equal(out[j].DeadAt) {
			return out[i].Order < out[j].Order
		}
		return out[i].DeadAt.Before(out[j].DeadAt)
	})
	return out, nil
}

package signalroute

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// defaultCooldownHours is a machine source's window when its spec sets none.
const defaultCooldownHours = 6

// cooldownReadTimeout bounds the one store read that rebuilds a window. The
// per-source lock is held across it, so an unreachable backend must fail the
// pass (which repeats) rather than hold every signal of the source behind it.
var cooldownReadTimeout = 10 * time.Second

// spendCooldown consults the source's cooldown for sig's initiationId and,
// when the id is fresh, spends it and records the window through the store.
// Consultation and spend are atomic per source (the book's lock), since the
// consumers of the pool share the map. The returned rollback undoes a spend
// whose Signal could not then be recorded. An error means the window could not
// be recovered: the pass is repeated rather than admitted against an empty
// window, which would re-open conversations for ids still inside theirs.
func spendCooldown(ctx context.Context, deps Deps, source *agentopsv1alpha1.SignalSource, sig *state.SignalRecord) (spent bool, rollback func(), err error) {
	if deps.Cooldowns == nil {
		return true, nil, nil
	}
	unlock := deps.Cooldowns.Lock(source.Name)
	defer unlock()
	var recoverErr error
	cd := deps.Cooldowns.For(source.Name, func() *ingest.Cooldown {
		var cd *ingest.Cooldown
		cd, recoverErr = recoverCooldown(ctx, deps, source, sig.Kind)
		return cd
	})
	if cd == nil {
		return false, nil, fmt.Errorf("recover the cooldown window of source %q: %w", source.Name, recoverErr)
	}
	id := sig.InitiationID
	if len(cd.Fresh([]string{id})) == 0 {
		return false, nil, nil
	}
	recordCooldown(ctx, deps, source.Name, cd)
	return true, func() {
		defer deps.Cooldowns.Lock(source.Name)()
		cd.Release(id)
		recordCooldown(ctx, deps, source.Name, cd)
	}, nil
}

// recoverCooldown builds a source's cooldown window, RECOVERED from the
// state store on first use in this process. The in-memory map is the hot path;
// the store is the record, so a restart or a leader change mid-incident does
// not re-open conversations for signals still inside their window. It returns
// nil and the cause when the store cannot be read, which leaves the book unbuilt
// so the next pass asks again.
func recoverCooldown(ctx context.Context, deps Deps, source *agentopsv1alpha1.SignalSource, kind string) (*ingest.Cooldown, error) {
	hours := source.Spec.CooldownHours
	if hours <= 0 {
		hours = defaultCooldownHours
		if kind == kindChat {
			// Chat defaults cooldown OFF. Id dedup exists so a flapping alert
			// opens one investigation; a person asking the same thing twice
			// means it twice, and swallowing the second ask would be a bug
			// wearing dedup's clothes.
			hours = 0
		}
	}
	if hours == 0 {
		// A disabled window has nothing to recover, and reading one would let a
		// store error fail a pass that could not fail before.
		return ingest.NewCooldownFrom(0, nil), nil
	}
	readCtx, cancel := context.WithTimeout(ctx, cooldownReadTimeout)
	defer cancel()
	window, err := GetCooldownWindow(readCtx, deps.CooldownWindows, source.Name)
	if err != nil {
		return nil, err
	}
	recorded := make([]ingest.Entry, 0, len(window))
	for _, e := range window {
		recorded = append(recorded, ingest.Entry{InitiationID: e.InitiationID, At: e.At})
	}
	return ingest.NewCooldownFrom(time.Duration(hours)*time.Hour, recorded), nil
}

// recordCooldown writes the cooldown window back through the store. Entries
// past the window are pruned and the list is bounded by the write. A failed
// write is logged, never fatal: the cooldown window still holds in memory, and failing
// the pass over it would trade a duplicate investigation for a stuck signal.
func recordCooldown(ctx context.Context, deps Deps, sourceName string, cd *ingest.Cooldown) {
	entries := cd.Entries()
	if entries == nil {
		return // window disabled — nothing to recover
	}
	out := make([]state.CooldownEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, state.CooldownEntry{InitiationID: e.InitiationID, At: e.At})
	}
	if err := PutCooldownWindow(ctx, deps.CooldownWindows, sourceName, out); err != nil {
		log.FromContext(ctx).Info("recording cooldown on the signal source failed; "+
			"the cooldown window holds in memory but will not survive a restart",
			"source", sourceName, "error", err.Error())
	}
}

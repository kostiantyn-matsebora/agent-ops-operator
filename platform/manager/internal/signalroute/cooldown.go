package signalroute

import (
	"context"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// defaultCooldownHours is a machine source's window when its spec sets none.
const defaultCooldownHours = 6

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

// recoverCooldown builds a source's suppression window, RECOVERED from the
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
	window, err := deps.Signals.GetCooldown(ctx, source.Name)
	if err != nil {
		return nil, err
	}
	recorded := make([]ingest.Entry, 0, len(window))
	for _, e := range window {
		recorded = append(recorded, ingest.Entry{InitiationID: e.InitiationID, At: e.At})
	}
	return ingest.NewCooldownFrom(time.Duration(hours)*time.Hour, recorded), nil
}

// recordCooldown writes the suppression window back through the store. Entries
// past the window are pruned and the list is bounded by the write. A failed
// write is logged, never fatal: suppression still holds in memory, and failing
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
	if err := deps.Signals.PutCooldown(ctx, sourceName, out); err != nil {
		log.FromContext(ctx).Info("recording cooldown on the signal source failed; "+
			"suppression holds in memory but will not survive a restart",
			"source", sourceName, "error", err.Error())
	}
}

// RecordReceived moves the source's status.receivedTotal and lastReceived for
// one routed signal. The counter is a read-modify-write shared by every
// consumer of the pool, so it writes under the resourceVersion it read and
// retries on conflict.
func RecordReceived(ctx context.Context, deps Deps, sourceName string) error {
	for attempt := 0; attempt < 5; attempt++ {
		var src agentopsv1alpha1.SignalSource
		if err := deps.Reader.Get(ctx, types.NamespacedName{Namespace: deps.Namespace, Name: sourceName}, &src); err != nil {
			return err
		}
		patch := client.MergeFromWithOptions(src.DeepCopy(), client.MergeFromWithOptimisticLock{})
		now := metav1.Now()
		src.Status.LastReceived = &now
		src.Status.ReceivedTotal++
		err := deps.Client.Status().Patch(ctx, &src, patch)
		if err == nil {
			return nil
		}
		if !apierrors.IsConflict(err) {
			return err
		}
	}
	return fmt.Errorf("conflict recording receipt on source %s", sourceName)
}

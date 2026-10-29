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
)

// defaultCooldownHours is a machine source's window when its spec sets none.
const defaultCooldownHours = 6

// spendCooldown consults the source's cooldown for sig's initiationId and,
// when the id is fresh, spends it and records the window on the source.
// Consultation and spend are atomic per source (the book's lock), since the
// consumers of the pool share the map. The returned rollback undoes a spend
// whose Signal could not then be recorded.
func spendCooldown(ctx context.Context, deps Deps, source *agentopsv1alpha1.SignalSource, sig *agentopsv1alpha1.Signal) (spent bool, rollback func()) {
	if deps.Cooldowns == nil {
		return true, nil
	}
	unlock := deps.Cooldowns.Lock(source.Name)
	defer unlock()
	cd := deps.Cooldowns.For(source.Name, func() *ingest.Cooldown { return recoverCooldown(source, sig.Spec.Kind) })
	id := sig.Spec.InitiationID
	if len(cd.Fresh([]string{id})) == 0 {
		return false, nil
	}
	recordCooldown(ctx, deps, source.Name, cd)
	return true, func() {
		defer deps.Cooldowns.Lock(source.Name)()
		cd.Release(id)
		recordCooldown(ctx, deps, source.Name, cd)
	}
}

// recoverCooldown builds a source's suppression window, RECOVERED from the
// source on first use in this process. The in-memory map is the hot path; the
// CR is the record, so a restart or a leader change mid-incident does not
// re-open conversations for signals still inside their window.
func recoverCooldown(source *agentopsv1alpha1.SignalSource, kind string) *ingest.Cooldown {
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
	recorded := make([]ingest.Entry, 0, len(source.Status.Cooldown))
	for _, e := range source.Status.Cooldown {
		recorded = append(recorded, ingest.Entry{InitiationID: e.InitiationID, At: e.At.Time})
	}
	return ingest.NewCooldownFrom(time.Duration(hours)*time.Hour, recorded)
}

// recordCooldown writes the suppression window back to the source. Entries
// past the window are pruned and the list is bounded by the write. A failed
// write is logged, never fatal: suppression still holds in memory, and failing
// the pass over it would trade a duplicate investigation for a stuck signal.
func recordCooldown(ctx context.Context, deps Deps, sourceName string, cd *ingest.Cooldown) {
	entries := cd.Entries()
	if entries == nil {
		return // window disabled — nothing to recover
	}
	out := make([]agentopsv1alpha1.CooldownEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, agentopsv1alpha1.CooldownEntry{InitiationID: e.InitiationID, At: metav1.NewTime(e.At)})
	}
	var fresh agentopsv1alpha1.SignalSource
	if err := deps.Reader.Get(ctx, types.NamespacedName{Namespace: deps.Namespace, Name: sourceName}, &fresh); err != nil {
		log.FromContext(ctx).Info("recording cooldown: reading the signal source failed",
			"source", sourceName, "error", err.Error())
		return
	}
	patch := client.MergeFrom(fresh.DeepCopy())
	fresh.Status.Cooldown = out
	if err := deps.Client.Status().Patch(ctx, &fresh, patch); err != nil {
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

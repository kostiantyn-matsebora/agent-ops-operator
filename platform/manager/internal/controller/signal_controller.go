package controller

import (
	"context"
	"fmt"
	"sort"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/signalroute"
)

// DefaultSignalRetryDeadline bounds how long a claimant may sit Routing,
// measured from status.firstAttemptAt (SIGNAL_RETRY_DEADLINE, chart
// signalRetryDeadline) — "A partially routed signal exits": past this, every
// remaining Routing claimant is flipped to Failed so PartiallyRouted (and a
// Pending signal whose only claimants never got past Routing) never sits
// non-terminal forever.
const DefaultSignalRetryDeadline = time.Hour

// DefaultMaxSignalsPerSource caps simultaneously NON-TERMINAL Signals one
// source may hold (MAX_SIGNALS_PER_SOURCE, chart maxSignalsPerSource) before
// the oldest is evicted. Zero on the reconciler struct means "use this
// default", never "uncapped" — the cap is a safety rail on etcd, not an
// opt-in feature.
const DefaultMaxSignalsPerSource = 200

// DefaultSignalTTLDropped is how long a Dropped Signal survives before GC
// (SIGNAL_TTL_DROPPED, chart signalTtlDropped) — short, since a pre-fan-out
// drop carries no routing record worth keeping long.
const DefaultSignalTTLDropped = 4 * time.Hour

// DefaultSignalTTLRouted is how long a Routed OR Failed Signal survives
// before GC (SIGNAL_TTL_ROUTED, chart signalTtlRouted). Failed shares this
// TTL rather than carrying its own (design D-6): "nobody got it" is exactly
// the record an operator investigates, the same reasoning that sizes
// Routed's own, longer, audit-trail window.
const DefaultSignalTTLRouted = 72 * time.Hour

// DefaultSignalRetryInterval is how soon a non-terminal Signal with a
// claimant still Routing — and not yet past its retry deadline — is
// reconciled again. Matches the order of magnitude ConversationReconciler's
// own "worth checking again soon" requeues use (enterPending's 30s), rather
// than inventing a different cadence for a sibling kind of retry.
const DefaultSignalRetryInterval = 30 * time.Second

// SignalReconciler owns every routing attempt on a Signal AFTER its first —
// ingest's own inline call makes that one (design D-2/D-3) — plus the three
// bounds that make a Signal a bounded, terminating record rather than a
// queue entry nothing ever closes: the retry deadline, the per-source live
// cap, and TTL garbage collection once a Signal goes terminal.
//
// It is the reconciler-side half of signalroute.RouteClaimants's own
// contract: "the caller is expected to call this again later ... the
// reconciler's retry". Crash/leader-change recovery needs no code here
// beyond an ordinary watch: the Pending/PartiallyRouted Signals are exactly
// the objects a new leader's initial reconcile already revisits
// (controller-runtime re-enqueues every watched object once its cache
// syncs), the mechanism design D-3 describes as "free".
type SignalReconciler struct {
	client.Client
	// Activity records the loud Failed surfacing and the overflow-eviction
	// drop. Nil is usable and ignores every emission — activity.Log.Emit's
	// own contract, shared by every other reconciler's Activity field.
	Activity *activity.Log
	// Runtime and MaxQueued are threaded straight into signalroute.Deps —
	// the SAME bootstrap config and backlog bound dispatch/ingest already
	// resolve against, so a retry here can never disagree with the inline
	// first attempt about what a claimant's conversation looks like.
	Runtime   runtimepod.Config
	MaxQueued int

	// RetryDeadline overrides DefaultSignalRetryDeadline. Zero means the
	// default, never "off" — SIGNAL_RETRY_DEADLINE has no "off" spelling.
	RetryDeadline time.Duration
	// MaxPerSource overrides DefaultMaxSignalsPerSource. Zero means the
	// default.
	MaxPerSource int
	// TTLDropped/TTLRouted override DefaultSignalTTLDropped/
	// DefaultSignalTTLRouted. Zero means the matching default.
	TTLDropped time.Duration
	TTLRouted  time.Duration
}

func (r *SignalReconciler) retryDeadline() time.Duration {
	if r.RetryDeadline > 0 {
		return r.RetryDeadline
	}
	return DefaultSignalRetryDeadline
}

func (r *SignalReconciler) maxPerSource() int {
	if r.MaxPerSource > 0 {
		return r.MaxPerSource
	}
	return DefaultMaxSignalsPerSource
}

func (r *SignalReconciler) ttlDropped() time.Duration {
	if r.TTLDropped > 0 {
		return r.TTLDropped
	}
	return DefaultSignalTTLDropped
}

func (r *SignalReconciler) ttlRouted() time.Duration {
	if r.TTLRouted > 0 {
		return r.TTLRouted
	}
	return DefaultSignalTTLRouted
}

// ttlFor is the GC window for a TERMINAL phase. Routed and Failed share one
// window (design D-6); Dropped carries its own, shorter one.
func (r *SignalReconciler) ttlFor(phase agentopsv1alpha1.SignalPhase) time.Duration {
	if phase == agentopsv1alpha1.SignalDropped {
		return r.ttlDropped()
	}
	return r.ttlRouted()
}

// terminalSignalPhase mirrors internal/httpapi's own isTerminalSignalPhase.
// Duplicated rather than shared: internal/httpapi already imports
// internal/controller (CredentialEnvPrefix), so the dependency cannot run
// the other way — the same layering signalroute's own package comment and
// SignalClaimant's doc explain for their own small duplications.
func terminalSignalPhase(p agentopsv1alpha1.SignalPhase) bool {
	switch p {
	case agentopsv1alpha1.SignalRouted, agentopsv1alpha1.SignalDropped, agentopsv1alpha1.SignalFailed:
		return true
	default:
		return false
	}
}

// Reconcile advances one Signal: retry, bound, or garbage-collect it.
func (r *SignalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sig agentopsv1alpha1.Signal
	if err := r.Get(ctx, req.NamespacedName, &sig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if terminalSignalPhase(sig.Status.Phase) {
		return r.reconcileTerminal(ctx, &sig)
	}
	return r.reconcileNonTerminal(ctx, &sig)
}

// reconcileTerminal is TTL GC: delete a terminal Signal once it has outlived
// its window, or requeue for exactly the moment it will.
func (r *SignalReconciler) reconcileTerminal(ctx context.Context, sig *agentopsv1alpha1.Signal) (ctrl.Result, error) {
	ttl := r.ttlFor(sig.Status.Phase)
	elapsed := time.Since(sig.CreationTimestamp.Time)
	if elapsed >= ttl {
		log.FromContext(ctx).Info("deleting a terminal signal past its TTL",
			"signal", sig.Name, "source", sig.Spec.SourceRef.Name, "phase", sig.Status.Phase,
			"age", elapsed.Round(time.Minute), "ttl", ttl)
		return ctrl.Result{}, client.IgnoreNotFound(r.Delete(ctx, sig))
	}
	return ctrl.Result{RequeueAfter: jitter(ttl - elapsed)}, nil
}

// reconcileNonTerminal is the retry path: the per-source cap first (it can
// terminate `sig` itself before any routing attempt is worth making), then
// the retry deadline, then an ordinary signalroute.RouteClaimants retry.
func (r *SignalReconciler) reconcileNonTerminal(ctx context.Context, sig *agentopsv1alpha1.Signal) (ctrl.Result, error) {
	evictedSelf, err := r.enforceSourceCap(ctx, sig)
	if err != nil {
		return ctrl.Result{}, err
	}
	if evictedSelf {
		return r.reconcileTerminal(ctx, sig)
	}

	// The retry deadline is measured FROM status.firstAttemptAt, so a Signal
	// that has never had one yet (nil) has not started that clock — it is
	// waiting for its FIRST attempt, not a retry, and the ordinary
	// RouteClaimants call below both makes that attempt and sets the stamp.
	// Treating nil as "already expired" would fail a Signal before it was
	// ever given a chance to route.
	if sig.Status.FirstAttemptAt != nil {
		if elapsed := time.Since(sig.Status.FirstAttemptAt.Time); elapsed >= r.retryDeadline() {
			if err := r.exhaustRetries(ctx, sig, elapsed); err != nil {
				return ctrl.Result{}, err
			}
			return r.reconcileTerminal(ctx, sig)
		}
	}

	if err := signalroute.RouteClaimants(ctx, r.deps(sig.Namespace), sig); err != nil {
		return ctrl.Result{}, err
	}
	// Reached only when `sig` entered this reconcile non-terminal, so a
	// Failed phase here is a FRESH transition this pass (every claimant
	// resolved — typically all Skipped — with none Routed). Retry-deadline
	// exhaustion and overflow eviction have their own call sites below;
	// this is the third, rarer way RouteClaimants alone can produce it.
	if sig.Status.Phase == agentopsv1alpha1.SignalFailed {
		r.emitSignalFailed(ctx, sig, activity.CodeRetryExhausted, "every claimant resolved without routing")
	}
	if terminalSignalPhase(sig.Status.Phase) {
		return r.reconcileTerminal(ctx, sig)
	}
	return ctrl.Result{RequeueAfter: DefaultSignalRetryInterval}, nil
}

// exhaustRetries flips every still-Routing claimant to Failed once elapsed
// has passed the retry deadline, recomputes phase via RollupPhase, and
// surfaces a resulting Failed as loudly as the unwired drop.
func (r *SignalReconciler) exhaustRetries(ctx context.Context, sig *agentopsv1alpha1.Signal, elapsed time.Duration) error {
	patch := client.MergeFrom(sig.DeepCopy())
	reason := fmt.Sprintf("retry deadline exceeded after %s (limit %s)", elapsed.Round(time.Second), r.retryDeadline())
	exhausted := false
	for i := range sig.Status.Claimants {
		c := &sig.Status.Claimants[i]
		if c.State != agentopsv1alpha1.ClaimantRouting {
			continue
		}
		c.State = agentopsv1alpha1.ClaimantFailed
		c.Reason = reason
		exhausted = true
	}
	if !exhausted {
		// Nothing left Routing — an ordinary retry on an earlier pass
		// already resolved every claimant, and this reconcile is stale.
		return nil
	}
	sig.Status.Phase = agentopsv1alpha1.RollupPhase(sig.Status.Claimants)
	if err := r.Status().Patch(ctx, sig, patch); err != nil {
		return err
	}
	log.FromContext(ctx).Info("signal retry deadline exceeded; exhausted claimant(s) marked Failed",
		"signal", sig.Name, "source", sig.Spec.SourceRef.Name, "phase", sig.Status.Phase,
		"elapsed", elapsed.Round(time.Second), "deadline", r.retryDeadline())
	if sig.Status.Phase == agentopsv1alpha1.SignalFailed {
		r.emitSignalFailed(ctx, sig, activity.CodeRetryExhausted, "retry deadline exceeded with no claimant routed")
	}
	return nil
}

// enforceSourceCap re-checks MAX_SIGNALS_PER_SOURCE on every non-terminal
// Signal's own reconcile — ingest's create (section 2) never checks it, so
// this reconciler is the one place it is enforced at all. Evicts the SINGLE
// oldest live Signal on this source when over cap; a burst that overshoots
// by more than one converges over the next few reconciles rather than in
// one pass, which costs nothing a Signal's own short retry requeue does not
// already pay for.
//
// Reports whether `sig` ITSELF was the one evicted, updating its in-memory
// copy in place so the caller can route straight to the terminal path
// without a second Get.
func (r *SignalReconciler) enforceSourceCap(ctx context.Context, sig *agentopsv1alpha1.Signal) (bool, error) {
	limit := r.maxPerSource()
	var list agentopsv1alpha1.SignalList
	if err := r.List(ctx, &list, client.InNamespace(sig.Namespace)); err != nil {
		return false, err
	}
	var live []*agentopsv1alpha1.Signal
	for i := range list.Items {
		item := &list.Items[i]
		if item.Spec.SourceRef.Name == sig.Spec.SourceRef.Name && !terminalSignalPhase(item.Status.Phase) {
			live = append(live, item)
		}
	}
	if len(live) <= limit {
		return false, nil
	}
	sort.Slice(live, func(i, j int) bool {
		return live[i].CreationTimestamp.Before(&live[j].CreationTimestamp)
	})
	victim := live[0]
	if err := r.evictOverflow(ctx, victim, len(live), limit); err != nil {
		return false, err
	}
	if victim.Name == sig.Name {
		*sig = *victim
		return true, nil
	}
	return false, nil
}

// evictOverflow terminates the oldest live Signal on a source past
// MAX_SIGNALS_PER_SOURCE: pre-fan-out (no claimant snapshot yet) it goes
// Dropped/overflow, exactly like every other pre-fan-out drop (design
// D-5a); post-fan-out every still-Routing claimant goes Failed/overflow and
// the phase rolls up — Dropped stays strictly pre-fan-out either way.
func (r *SignalReconciler) evictOverflow(ctx context.Context, victim *agentopsv1alpha1.Signal, live, limit int) error {
	patch := client.MergeFrom(victim.DeepCopy())
	if len(victim.Status.Claimants) == 0 {
		victim.Status.Phase = agentopsv1alpha1.SignalDropped
		victim.Status.Reason = agentopsv1alpha1.SignalDropOverflow
	} else {
		for i := range victim.Status.Claimants {
			c := &victim.Status.Claimants[i]
			if c.State == agentopsv1alpha1.ClaimantRouting {
				c.State = agentopsv1alpha1.ClaimantFailed
				c.Reason = "evicted: per-source live Signal cap exceeded"
			}
		}
		victim.Status.Phase = agentopsv1alpha1.RollupPhase(victim.Status.Claimants)
	}
	if err := r.Status().Patch(ctx, victim, patch); err != nil {
		return err
	}
	// COUNTED AND LOGGED (design D-6): the live/limit pair is what lets an
	// operator tell a one-off overshoot from a source that is permanently
	// over its cap.
	log.FromContext(ctx).Info("evicted the oldest live signal on an over-cap source",
		"signal", victim.Name, "source", victim.Spec.SourceRef.Name,
		"live", live, "limit", limit, "phase", victim.Status.Phase)
	switch victim.Status.Phase {
	case agentopsv1alpha1.SignalDropped:
		r.Activity.Emit(activity.Event{
			Kind:   activity.KindSignalDropped,
			From:   activity.Node(activity.NodeSignalSource, victim.Spec.SourceRef.Name),
			Status: activity.StatusError,
			Code:   activity.CodeOverflow,
			Detail: fmt.Sprintf("evicted: per-source live Signal cap exceeded (%s)", victim.Spec.InitiationID),
		})
	case agentopsv1alpha1.SignalFailed:
		r.emitSignalFailed(ctx, victim, activity.CodeOverflow,
			"evicted: per-source live Signal cap exceeded, no claimant had routed")
	}
	return nil
}

// emitSignalFailed surfaces a Signal that just rolled up to Failed as
// loudly as the unwired drop (signal-persistence spec, "the phase is a
// roll-up ... the Failed loud-surfacing half") — an activity event AND a
// log line, never a silent status change.
func (r *SignalReconciler) emitSignalFailed(ctx context.Context, sig *agentopsv1alpha1.Signal, code, reason string) {
	log.FromContext(ctx).Info("signal failed: no claimant routed it",
		"signal", sig.Name, "source", sig.Spec.SourceRef.Name,
		"initiationId", sig.Spec.InitiationID, "reason", reason)
	r.Activity.Emit(activity.Event{
		Kind:   activity.KindSignalFailed,
		From:   activity.Node(activity.NodeSignalSource, sig.Spec.SourceRef.Name),
		Status: activity.StatusError,
		Code:   code,
		Detail: fmt.Sprintf("%s (%s)", reason, sig.Spec.InitiationID),
	})
}

// deps builds signalroute.Deps from this reconciler's own fields — the same
// shape ingest's Server builds from its own (internal/httpapi's
// signalRouteDeps), so the two callers of RouteClaimants can never disagree
// about what a routing attempt may read or write.
func (r *SignalReconciler) deps(namespace string) signalroute.Deps {
	return signalroute.Deps{
		Client: r.Client, Reader: r.Client, Namespace: namespace,
		Activity: r.Activity, Runtime: r.Runtime, MaxQueued: r.MaxQueued,
	}
}

// SetupWithManager wires the controller to Signal events alone. No extra
// watch is needed for a re-wire (a Pipeline/Coordinator claim changing who
// serves a source): RouteClaimants itself re-resolves the live claimant set
// on every call and Skips one no longer served, and this reconciler's own
// short retry requeue (DefaultSignalRetryInterval) already revisits a
// non-terminal Signal often enough to notice.
func (r *SignalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.Signal{}).
		Complete(r)
}

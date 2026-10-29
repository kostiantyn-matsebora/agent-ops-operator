package controller

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/signalroute"
)

// LeaderLeaseName is the controller-runtime leader-election Lease this manager
// holds (main.go's LeaderElectionID). Its holderIdentity is the string a
// terminal Signal records as status.consumedBy.
const LeaderLeaseName = "agentops-manager.agentops.dev"

// The defaults of the six signal bounds (chart keys signalTtlRouted,
// signalTtlDropped, signalTtlFailed, signalRetryDeadline, maxSignalsPerSource,
// signalConsumerConcurrency). Zero on the reconciler means "use this", never
// "off": every bound is a safety rail on etcd or on the leader.
const (
	DefaultSignalTTLRouted           = 8 * time.Hour
	DefaultSignalTTLDropped          = 4 * time.Hour
	DefaultSignalTTLFailed           = 72 * time.Hour // the record an operator investigates
	DefaultSignalRetryDeadline       = time.Hour
	DefaultMaxSignalsPerSource       = 200
	DefaultSignalConsumerConcurrency = 4
)

// DefaultSignalRetryInterval is how soon a non-terminal Signal with a claimant
// still Routing is processed again. Matches the order of magnitude the
// ConversationReconciler's own "worth checking again soon" requeues use.
const DefaultSignalRetryInterval = 30 * time.Second

// defaultOrderRetry is how soon a Signal that must wait its turn (an earlier
// message of its conversation is still in flight) looks again.
const defaultOrderRetry = time.Second

// SignalReconciler is the Signal consumer pool and housekeeping, in one
// controller so that both go through ONE work queue (design D-3, D-11): the
// queue hands a Signal to one worker at a time, so a retry deadline or an
// eviction never collides with a consumer mid-pass.
//
// It runs on the LEADER only — controller-runtime's own leader gating, the
// same that holds back every other reconciler on a standby. The cooldown map,
// the channel-operation queue and the activity log it works from are
// per-process state only the leader fills.
//
// Every pass is idempotent (equality reuse + the input named for the Signal),
// so a pass interrupted by a crash or a leader change is simply repeated; a
// new leader needs no recovery step beyond the initial reconcile of every
// non-terminal Signal controller-runtime performs when its cache syncs.
type SignalReconciler struct {
	client.Client
	// Reader is the API server read path. A Signal is read from it, never
	// from the cache, so a requeued pass cannot act on a status it already
	// advanced. Nil falls back to Client (tests).
	Reader client.Reader
	// Activity records the loud Failed/Dropped surfacing. Nil is inert.
	Activity *activity.Log
	// Ops carries the chat notices, fire-and-forget. Nil skips notices.
	Ops *chat.OpQueue
	// Cooldowns is the leader's per-source cooldown map.
	Cooldowns *ingest.CooldownBook
	// Runtime and MaxQueued are the SAME bootstrap config and backlog bound
	// the conversation reconciler resolves against.
	Runtime   runtimepod.Config
	MaxQueued int
	// Identity is the fallback for status.consumedBy when the leader Lease
	// cannot be read: this replica's own name.
	Identity string

	// Concurrency is SIGNAL_CONSUMER_CONCURRENCY. Zero is the default.
	Concurrency int
	// RetryDeadline, MaxPerSource and the three TTLs override the matching
	// Default*. Zero means the default.
	RetryDeadline time.Duration
	MaxPerSource  int
	TTLRouted     time.Duration
	TTLDropped    time.Duration
	TTLFailed     time.Duration
	// RetryInterval and OrderRetry override the requeue delays (tests).
	RetryInterval time.Duration
	OrderRetry    time.Duration

	// Now is the clock; nil is time.Now.
	Now func() time.Time

	mu   sync.Mutex
	held map[string]struct{} // (source, initiationId) keys a pass is working on
	kick chan event.GenericEvent
}

func (r *SignalReconciler) reader() client.Reader {
	if r.Reader != nil {
		return r.Reader
	}
	return r.Client
}

func (r *SignalReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func orDuration(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

func (r *SignalReconciler) retryDeadline() time.Duration {
	return orDuration(r.RetryDeadline, DefaultSignalRetryDeadline)
}

func (r *SignalReconciler) retryInterval() time.Duration {
	return orDuration(r.RetryInterval, DefaultSignalRetryInterval)
}

func (r *SignalReconciler) orderRetry() time.Duration {
	return orDuration(r.OrderRetry, defaultOrderRetry)
}

func (r *SignalReconciler) maxPerSource() int {
	if r.MaxPerSource > 0 {
		return r.MaxPerSource
	}
	return DefaultMaxSignalsPerSource
}

func (r *SignalReconciler) concurrency() int {
	if r.Concurrency > 0 {
		return r.Concurrency
	}
	return DefaultSignalConsumerConcurrency
}

// ttlFor is the deletion window of a TERMINAL phase, measured from
// status.completedAt. Each phase has its own.
func (r *SignalReconciler) ttlFor(phase agentopsv1alpha1.SignalPhase) time.Duration {
	switch phase {
	case agentopsv1alpha1.SignalDropped:
		return orDuration(r.TTLDropped, DefaultSignalTTLDropped)
	case agentopsv1alpha1.SignalFailed:
		return orDuration(r.TTLFailed, DefaultSignalTTLFailed)
	default:
		return orDuration(r.TTLRouted, DefaultSignalTTLRouted)
	}
}

// terminalSignalPhase reports whether a phase is final.
func terminalSignalPhase(p agentopsv1alpha1.SignalPhase) bool {
	switch p {
	case agentopsv1alpha1.SignalRouted, agentopsv1alpha1.SignalDropped, agentopsv1alpha1.SignalFailed:
		return true
	default:
		return false
	}
}

func (r *SignalReconciler) deps(namespace string) signalroute.Deps {
	return signalroute.Deps{
		Client: r.Client, Reader: r.reader(), Namespace: namespace,
		Activity: r.Activity, Ops: r.Ops, Cooldowns: r.Cooldowns,
		Runtime: r.Runtime, MaxQueued: r.MaxQueued,
	}
}

// Reconcile is one pass over one Signal: housekeeping first (TTL delete, cap,
// retry deadline), then — in arrival order within its conversation — the
// consumer.
func (r *SignalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var sig agentopsv1alpha1.Signal
	if err := r.reader().Get(ctx, req.NamespacedName, &sig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if terminalSignalPhase(sig.Status.Phase) {
		return r.reconcileTerminal(ctx, &sig)
	}
	return r.reconcileLive(ctx, &sig)
}

// reconcileTerminal is TTL housekeeping: delete a terminal Signal once its
// phase's TTL has passed, measured from status.completedAt — never from
// creationTimestamp, which a long-retried signal left behind long ago — or
// requeue for exactly the moment it will.
func (r *SignalReconciler) reconcileTerminal(ctx context.Context, sig *agentopsv1alpha1.Signal) (ctrl.Result, error) {
	if sig.Status.CompletedAt == nil {
		// A terminal Signal this version did not write. Start its clock now
		// rather than delete on a creation time it was never measured from.
		before := sig.DeepCopy()
		now := metav1.NewTime(r.now())
		sig.Status.CompletedAt = &now
		if err := r.writeStatus(ctx, sig, before); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}
	ttl := r.ttlFor(sig.Status.Phase)
	age := r.now().Sub(sig.Status.CompletedAt.Time)
	if age < ttl {
		return ctrl.Result{RequeueAfter: jitter(ttl - age)}, nil
	}
	log.FromContext(ctx).Info("deleting a terminal signal past its TTL",
		"signal", sig.Name, "source", sig.Spec.SourceRef.Name, "phase", sig.Status.Phase,
		"sinceCompleted", age.Round(time.Minute), "ttl", ttl)
	// Conditional on the version this pass read: a write that landed since is
	// a Signal that is not finished after all.
	err := r.Delete(ctx, sig, client.Preconditions{UID: &sig.UID, ResourceVersion: &sig.ResourceVersion})
	if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, err
}

// reconcileLive handles a non-terminal Signal.
func (r *SignalReconciler) reconcileLive(ctx context.Context, sig *agentopsv1alpha1.Signal) (ctrl.Result, error) {
	var src agentopsv1alpha1.SignalSource
	if err := r.reader().Get(ctx, types.NamespacedName{Namespace: sig.Namespace, Name: sig.Spec.SourceRef.Name}, &src); err != nil {
		// A source deleted under its Signals takes them through the ownerRef;
		// nothing here can route without it.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if evicted, err := r.enforceSourceCap(ctx, sig); err != nil || evicted {
		return ctrl.Result{}, err
	}
	if first := sig.Status.FirstAttemptAt; first != nil {
		if elapsed := r.now().Sub(first.Time); elapsed >= r.retryDeadline() {
			return ctrl.Result{}, r.exhaust(ctx, sig, elapsed)
		}
	}

	// Messages of one conversation are processed one at a time, in arrival
	// order; different conversations run in parallel (design D-3).
	key := sig.Spec.SourceRef.Name + "\x00" + sig.Spec.InitiationID
	if !r.acquire(key) {
		return ctrl.Result{RequeueAfter: r.orderRetry()}, nil
	}
	defer r.release(key)
	head, err := r.isHead(ctx, sig)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !head {
		return ctrl.Result{RequeueAfter: r.orderRetry()}, nil
	}
	return r.consume(ctx, sig, &src)
}

// acquire takes the in-process lock of one (source, initiationId) key. The
// work queue already guarantees one worker per Signal; this is the per-KEY
// exclusion on top, so a requeued Signal takes the same lock as a first pass.
func (r *SignalReconciler) acquire(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.held == nil {
		r.held = map[string]struct{}{}
	}
	if _, busy := r.held[key]; busy {
		return false
	}
	r.held[key] = struct{}{}
	return true
}

func (r *SignalReconciler) release(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.held, key)
}

// signalBefore orders by arrival: creationTimestamp, then name.
func signalBefore(a, b *agentopsv1alpha1.Signal) bool {
	if !a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.CreationTimestamp.Before(&b.CreationTimestamp)
	}
	return a.Name < b.Name
}

// isHead reports whether sig is the earliest non-terminal Signal of its
// (source, initiationId): an earlier one still in flight goes first, because
// both feed one conversation and appending out of order — or each failing to
// find the conversation the other is creating — is the failure this prevents.
func (r *SignalReconciler) isHead(ctx context.Context, sig *agentopsv1alpha1.Signal) (bool, error) {
	// Read from the API server, not the cache: an earlier message the cache has
	// not seen yet would otherwise be overtaken. The namespace holds at most
	// the per-source cap of live Signals per source, so the list stays small.
	var list agentopsv1alpha1.SignalList
	if err := r.reader().List(ctx, &list, client.InNamespace(sig.Namespace)); err != nil {
		return false, fmt.Errorf("list signals of %s: %w", sig.Spec.InitiationID, err)
	}
	for i := range list.Items {
		other := &list.Items[i]
		if other.Name == sig.Name || terminalSignalPhase(other.Status.Phase) ||
			other.Spec.SourceRef.Name != sig.Spec.SourceRef.Name || other.Spec.InitiationID != sig.Spec.InitiationID {
			continue
		}
		if signalBefore(other, sig) {
			return false, nil
		}
	}
	return true, nil
}

// consume runs one consumer pass: the admission decision (when the claimant
// snapshot does not exist yet), then the routing of every claimant still
// Routing. Each step is recorded in its own status write, and the snapshot is
// written BEFORE anything is created: a repeated pass then skips admission, so
// the cooldown it spent cannot suppress the repeat.
func (r *SignalReconciler) consume(ctx context.Context, sig *agentopsv1alpha1.Signal, src *agentopsv1alpha1.SignalSource) (ctrl.Result, error) {
	deps := r.deps(sig.Namespace)
	before := sig.DeepCopy()
	r.stampAttempt(sig)

	if len(sig.Status.Claimants) == 0 {
		res, err := signalroute.Admit(ctx, deps, sig, src)
		if err != nil {
			// Persist the attempt even so: attempts and firstAttemptAt must
			// advance, or a poison Signal would never reach its retry deadline.
			_ = r.writeStatus(ctx, sig, before)
			rollback(res)
			return ctrl.Result{}, err
		}
		r.finishIfTerminal(ctx, sig)
		if err := r.writeStatus(ctx, sig, before); err != nil {
			rollback(res)
			return ctrl.Result{}, err
		}
		if res.Dropped {
			return ctrl.Result{}, nil
		}
		before = sig.DeepCopy()
	}

	res, procErr := signalroute.Route(ctx, deps, sig)
	r.finishIfTerminal(ctx, sig)
	if err := r.writeStatus(ctx, sig, before); err != nil {
		return ctrl.Result{}, err
	}
	if procErr != nil {
		return ctrl.Result{}, procErr
	}
	if res.FirstRouted {
		if err := signalroute.RecordReceived(ctx, deps, src.Name); err != nil {
			log.FromContext(ctx).Info("recording receipt on the signal source failed",
				"source", src.Name, "error", err.Error())
		}
	}
	if sig.Status.Phase == agentopsv1alpha1.SignalFailed {
		signalroute.Loud(ctx, r.Activity, sig, activity.KindSignalFailed, activity.CodeSkipped,
			"every claimant resolved without routing the signal")
	}
	if res.Wait && !terminalSignalPhase(sig.Status.Phase) {
		return ctrl.Result{RequeueAfter: r.retryInterval()}, nil
	}
	return ctrl.Result{}, nil
}

func rollback(res signalroute.Result) {
	if res.Rollback != nil {
		res.Rollback()
	}
}

// stampAttempt counts the pass and stamps the first one — the origin the retry
// deadline measures from.
func (r *SignalReconciler) stampAttempt(sig *agentopsv1alpha1.Signal) {
	sig.Status.Attempts++
	if sig.Status.FirstAttemptAt == nil {
		now := metav1.NewTime(r.now())
		sig.Status.FirstAttemptAt = &now
	}
}

// finishIfTerminal stamps the write that makes a phase terminal: completedAt
// (never moved by a later write), and the leader that processed it.
func (r *SignalReconciler) finishIfTerminal(ctx context.Context, sig *agentopsv1alpha1.Signal) {
	if !terminalSignalPhase(sig.Status.Phase) || sig.Status.CompletedAt != nil {
		return
	}
	now := metav1.NewTime(r.now())
	sig.Status.CompletedAt = &now
	sig.Status.ConsumedAt = &now
	sig.Status.ConsumedBy = r.consumer(ctx, sig.Namespace)
}

// consumer is the leader's identity: the leader Lease's holderIdentity, which
// carries controller-runtime's own uniqueness suffix, falling back to this
// replica's name when the Lease cannot be read.
func (r *SignalReconciler) consumer(ctx context.Context, namespace string) string {
	var lease coordinationv1.Lease
	if err := r.reader().Get(ctx, types.NamespacedName{Namespace: namespace, Name: LeaderLeaseName}, &lease); err == nil &&
		lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != "" {
		return *lease.Spec.HolderIdentity
	}
	return r.Identity
}

// writeStatus persists sig's status against the version it was read at, so a
// concurrent writer fails this pass (and it is repeated) instead of being
// overwritten.
func (r *SignalReconciler) writeStatus(ctx context.Context, sig, before *agentopsv1alpha1.Signal) error {
	patch := client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})
	return r.Status().Patch(ctx, sig, patch)
}

// exhaust applies the retry deadline: every non-terminal claimant becomes
// Failed with the exhaustion as its reason — or, before fan-out, the Signal is
// Dropped with reason exhausted. A PartiallyRouted signal always exits.
func (r *SignalReconciler) exhaust(ctx context.Context, sig *agentopsv1alpha1.Signal, elapsed time.Duration) error {
	reason := fmt.Sprintf("retry deadline exceeded after %s (limit %s)", elapsed.Round(time.Second), r.retryDeadline())
	before := sig.DeepCopy()
	if len(sig.Status.Claimants) == 0 {
		sig.Status.Phase = agentopsv1alpha1.SignalDropped
		sig.Status.Reason = agentopsv1alpha1.SignalDropExhausted
	} else {
		failRouting(sig, reason)
	}
	r.finishIfTerminal(ctx, sig)
	if err := r.writeStatus(ctx, sig, before); err != nil {
		return err
	}
	r.surface(ctx, sig, activity.CodeRetryExhausted, activity.CodeExhausted, reason)
	return nil
}

// failRouting marks every claimant still Routing Failed and rolls the phase up.
func failRouting(sig *agentopsv1alpha1.Signal, reason string) {
	for i := range sig.Status.Claimants {
		c := &sig.Status.Claimants[i]
		if c.State == agentopsv1alpha1.ClaimantRouting {
			c.State = agentopsv1alpha1.ClaimantFailed
			c.Reason = reason
		}
	}
	sig.Status.Phase = agentopsv1alpha1.RollupPhase(sig.Status.Claimants)
}

// surface makes a housekeeping end as loud as the unwired drop: a Dropped
// signal carries dropCode, a Failed one failCode; a Routed roll-up (some
// claimants routed, the rest failed) is logged only, as the Signal is not lost.
func (r *SignalReconciler) surface(ctx context.Context, sig *agentopsv1alpha1.Signal, failCode, dropCode, detail string) {
	switch sig.Status.Phase {
	case agentopsv1alpha1.SignalDropped:
		signalroute.Loud(ctx, r.Activity, sig, activity.KindSignalDropped, dropCode, detail)
	case agentopsv1alpha1.SignalFailed:
		signalroute.Loud(ctx, r.Activity, sig, activity.KindSignalFailed, failCode, detail)
	default:
		log.FromContext(ctx).Info("signal housekeeping failed its unrouted claimants",
			"signal", sig.Name, "source", sig.Spec.SourceRef.Name, "phase", sig.Status.Phase, "detail", detail)
	}
}

// enforceSourceCap holds the per-source cap on non-terminal Signals by
// evicting the OLDEST first. The eviction happens in the victim's OWN pass —
// the work queue is what keeps it from racing a consumer — so a pass that finds
// its source over the cap evicts itself when it is among the oldest, and
// kicks the others' passes otherwise. Reports whether sig itself was evicted.
func (r *SignalReconciler) enforceSourceCap(ctx context.Context, sig *agentopsv1alpha1.Signal) (bool, error) {
	var list agentopsv1alpha1.SignalList
	if err := r.List(ctx, &list, client.InNamespace(sig.Namespace)); err != nil {
		return false, err
	}
	live := []*agentopsv1alpha1.Signal{sig}
	for i := range list.Items {
		item := &list.Items[i]
		if item.Name != sig.Name && item.Spec.SourceRef.Name == sig.Spec.SourceRef.Name && !terminalSignalPhase(item.Status.Phase) {
			live = append(live, item)
		}
	}
	limit := r.maxPerSource()
	over := len(live) - limit
	if over <= 0 {
		return false, nil
	}
	sort.Slice(live, func(i, j int) bool { return signalBefore(live[i], live[j]) })
	evictedSelf := false
	for _, victim := range live[:over] {
		if victim.Name != sig.Name {
			r.kickSignal(victim)
			continue
		}
		if err := r.evict(ctx, sig, len(live), limit); err != nil {
			return false, err
		}
		evictedSelf = true
	}
	return evictedSelf, nil
}

// kickSignal enqueues another Signal's pass on the same work queue.
func (r *SignalReconciler) kickSignal(victim *agentopsv1alpha1.Signal) {
	if r.kick == nil {
		return
	}
	select {
	case r.kick <- event.GenericEvent{Object: victim.DeepCopy()}:
	default: // full: the victim's own requeue reaches it
	}
}

// evict terminates sig to hold the per-source cap: before fan-out it is
// Dropped (reason overflow), after it every non-terminal claimant is Failed
// (reason overflow) and the phase rolls up — Dropped stays strictly
// pre-fan-out. Counted and logged either way.
func (r *SignalReconciler) evict(ctx context.Context, sig *agentopsv1alpha1.Signal, live, limit int) error {
	before := sig.DeepCopy()
	const reason = "evicted: per-source signal cap exceeded (overflow)"
	if len(sig.Status.Claimants) == 0 {
		sig.Status.Phase = agentopsv1alpha1.SignalDropped
		sig.Status.Reason = agentopsv1alpha1.SignalDropOverflow
	} else {
		failRouting(sig, reason)
	}
	r.finishIfTerminal(ctx, sig)
	if err := r.writeStatus(ctx, sig, before); err != nil {
		return err
	}
	log.FromContext(ctx).Info("evicted the oldest non-terminal signal of an over-cap source",
		"signal", sig.Name, "source", sig.Spec.SourceRef.Name, "live", live, "limit", limit, "phase", sig.Status.Phase)
	r.surface(ctx, sig, activity.CodeOverflow, activity.CodeOverflow, reason)
	return nil
}

// SetupWithManager wires the consumer pool: SIGNAL_CONSUMER_CONCURRENCY workers
// over one work queue keyed by Signal, plus the channel housekeeping uses to
// enqueue another Signal's pass. The controller runs on the leader only.
func (r *SignalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.kick = make(chan event.GenericEvent, 64)
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.Signal{}).
		WatchesRawSource(source.Channel(r.kick, &handler.EnqueueRequestForObject{})).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.concurrency()}).
		Complete(r)
}

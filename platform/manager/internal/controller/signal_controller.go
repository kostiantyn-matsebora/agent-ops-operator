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
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
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
// signal the feed's relist enqueues when it starts.
type SignalReconciler struct {
	client.Client
	// Reader is the API server read path for the objects that stay Kubernetes
	// objects under any backend (the SignalSource, the Lease, claimants). Nil
	// falls back to Client (tests).
	Reader client.Reader
	// Activity records the loud Failed/Dropped surfacing. Nil is inert.
	Activity *activity.Log
	// Ops carries the chat notices, fire-and-forget. Nil skips notices.
	Ops *chat.OpQueue
	// Conversations, Runs and Blobs are the only way a signal's route reaches
	// a Conversation, its run history or a queued input's payload.
	Conversations state.Collection[state.ConversationRecord]
	Runs          state.Appended[state.ConversationRecord, state.Run]
	Blobs         state.Blobs
	// Signals is the only way this controller reaches a Signal. It is read
	// straight from its source of truth, never a cache, so a requeued pass
	// cannot act on a status it already advanced.
	Signals state.Expiring[state.SignalRecord]
	// CooldownWindows and Receipts hold one source's cooldown window and
	// receipt counters respectively, read and written through
	// internal/signalroute's own helpers over Mutate.
	CooldownWindows state.Collection[state.CooldownWindow]
	Receipts        state.Collection[state.Received]
	// Namespace names the install's namespace, for the objects read beside the
	// signals and for the requests the feed enqueues.
	Namespace string
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

// ttlFor is the expiry window of a TERMINAL phase, measured from
// status.completedAt. Each phase has its own.
func (r *SignalReconciler) ttlFor(phase string) time.Duration {
	switch phase {
	case state.SignalDropped:
		return orDuration(r.TTLDropped, DefaultSignalTTLDropped)
	case state.SignalFailed:
		return orDuration(r.TTLFailed, DefaultSignalTTLFailed)
	default:
		return orDuration(r.TTLRouted, DefaultSignalTTLRouted)
	}
}

func (r *SignalReconciler) deps() signalroute.Deps {
	return signalroute.Deps{
		Client: r.Client, Reader: r.reader(), Namespace: r.Namespace,
		Conversations: r.Conversations, Runs: r.Runs, Blobs: r.Blobs,
		CooldownWindows: r.CooldownWindows, Receipts: r.Receipts,
		Activity: r.Activity, Ops: r.Ops, Cooldowns: r.Cooldowns,
		Runtime: r.Runtime, MaxQueued: r.MaxQueued,
	}
}

// signalPass is the record one Reconcile holds and the revision it was read
// at. Every write of the pass goes through write, which carries the revision it
// returns into the next one: a concurrent writer fails the pass with
// ErrConflict, and the work queue repeats it. There is no get-modify-update
// loop here on purpose — re-running a routing decision that already created a
// conversation is the failure the held revision prevents.
type signalPass struct {
	key state.Key
	rec state.SignalRecord
	rev state.Revision
}

// write persists the pass's record against the revision it holds.
func (r *SignalReconciler) write(ctx context.Context, p *signalPass) error {
	rev, err := r.Signals.Update(ctx, p.key, p.rev, p.rec)
	if err != nil {
		return err
	}
	p.rev = rev
	return nil
}

// Reconcile is one pass over one Signal: housekeeping first (expiry, cap,
// retry deadline), then — in arrival order within its conversation — the
// consumer.
func (r *SignalReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	key := state.Key(req.Name)
	rec, rev, err := r.Signals.Get(ctx, key)
	if err != nil {
		return ctrl.Result{}, state.IgnoreNotFound(err)
	}
	p := &signalPass{key: key, rec: rec, rev: rev}
	if p.rec.Terminal() {
		return r.reconcileTerminal(ctx, p)
	}
	return r.reconcileLive(ctx, p)
}

// reconcileTerminal is the expiry housekeeping: a terminal Signal carries the
// instant it removes itself, written in the update that made it terminal. One
// found without — written by an earlier build, or by a crash between the status
// write and the expiry write — is stamped here, measured from status.completedAt
// and never from creation, which a long-retried signal left behind long ago.
// Nothing deletes it by hand: the store's own sweep does, once the instant
// passes.
func (r *SignalReconciler) reconcileTerminal(ctx context.Context, p *signalPass) (ctrl.Result, error) {
	stamped := false
	if p.rec.CompletedAt == nil {
		// A terminal Signal this version did not write. Its completion time was
		// never recorded, so take the best the record itself knows: the first
		// attempt, then the creation. Stamping "now" would restart the expiry of
		// a signal that finished days ago.
		at := p.rec.CreatedAt
		if p.rec.FirstAttemptAt != nil {
			at = *p.rec.FirstAttemptAt
		}
		p.rec.CompletedAt, stamped = &at, true
	}
	if p.rec.ExpiresAt == nil {
		expires := p.rec.CompletedAt.Add(r.ttlFor(p.rec.Phase))
		p.rec.ExpiresAt, stamped = &expires, true
	}
	if !stamped {
		return ctrl.Result{}, nil
	}
	return ctrl.Result{}, state.IgnoreNotFound(r.write(ctx, p))
}

// reconcileLive handles a non-terminal Signal.
func (r *SignalReconciler) reconcileLive(ctx context.Context, p *signalPass) (ctrl.Result, error) {
	var src agentopsv1alpha1.SignalSource
	if err := r.reader().Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: p.rec.SourceRef}, &src); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, r.releaseOrphans(ctx, p.rec.SourceRef)
		}
		return ctrl.Result{}, err
	}

	if evicted, err := r.enforceSourceCap(ctx, p); err != nil || evicted {
		return ctrl.Result{}, err
	}
	if first := p.rec.FirstAttemptAt; first != nil {
		if elapsed := r.now().Sub(*first); elapsed >= r.retryDeadline() {
			return ctrl.Result{}, r.exhaust(ctx, p, elapsed)
		}
	}

	// Messages of one conversation are processed one at a time, in arrival
	// order; different conversations run in parallel (design D-3).
	key := p.rec.SourceRef + "\x00" + p.rec.InitiationID
	if !r.acquire(key) {
		return ctrl.Result{RequeueAfter: r.orderRetry()}, nil
	}
	defer r.release(key)
	head, err := r.isHead(ctx, &p.rec)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !head {
		return ctrl.Result{RequeueAfter: r.orderRetry()}, nil
	}
	return r.consume(ctx, p, &src)
}

// releaseOrphans removes every Signal of a source that no longer exists. The
// manager does this itself rather than rely on an owner reference being
// honoured: the store may not be the Kubernetes API, and under envtest no
// garbage collector runs. The pass knows the identity its Signal recorded, so a
// successor that took the name meanwhile keeps its own signals and window.
func (r *SignalReconciler) releaseOrphans(ctx context.Context, source string) error {
	log.FromContext(ctx).Info("releasing the signals of a source that no longer exists", "source", source)
	releaser := &SourceReleaser{Signals: r.Signals, Cooldowns: r.CooldownWindows, Live: liveSignalSource(r.reader(), r.Namespace)}
	return releaser.Release(ctx, source)
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

// isHead reports whether sig is the earliest non-terminal Signal of its
// (source, initiationId): an earlier one still in flight goes first, because
// both feed one conversation and appending out of order — or each failing to
// find the conversation the other is creating — is the failure this prevents.
func (r *SignalReconciler) isHead(ctx context.Context, sig *state.SignalRecord) (bool, error) {
	// The store reads its source of truth, not a cache: an earlier message a
	// cache has not seen yet would otherwise be overtaken. The index returns at
	// most the per-source cap of live Signals, so the list stays small.
	live, err := r.Signals.List(ctx, state.SignalNonTerminalByInitiation,
		state.IndexValue(sig.SourceRef, sig.InitiationID))
	if err != nil {
		return false, fmt.Errorf("list signals of %s: %w", sig.InitiationID, err)
	}
	for i := range live {
		if live[i].Name != sig.Name && state.SignalArrivesBefore(&live[i], sig) {
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
func (r *SignalReconciler) consume(ctx context.Context, p *signalPass, src *agentopsv1alpha1.SignalSource) (ctrl.Result, error) {
	deps := r.deps()
	sig := &p.rec
	r.stampAttempt(sig)

	if len(sig.Claimants) == 0 {
		res, err := signalroute.Admit(ctx, deps, sig, src)
		if err != nil {
			// Persist the attempt even so: attempts and firstAttemptAt must
			// advance, or a poison Signal would never reach its retry deadline.
			_ = r.write(ctx, p)
			rollback(res)
			return ctrl.Result{}, err
		}
		r.finishIfTerminal(ctx, sig)
		if err := r.write(ctx, p); err != nil {
			rollback(res)
			return ctrl.Result{}, err
		}
		if res.Dropped {
			return ctrl.Result{}, nil
		}
	}

	res, procErr := signalroute.Route(ctx, deps, sig)
	r.finishIfTerminal(ctx, sig)
	if err := r.write(ctx, p); err != nil {
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
	if sig.Phase == state.SignalFailed {
		signalroute.Loud(ctx, r.Activity, sig, activity.KindSignalFailed, activity.CodeSkipped,
			"every claimant resolved without routing the signal")
	}
	if res.Wait && !sig.Terminal() {
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
func (r *SignalReconciler) stampAttempt(sig *state.SignalRecord) {
	sig.Attempts++
	if sig.FirstAttemptAt == nil {
		now := r.now().UTC().Truncate(time.Second)
		sig.FirstAttemptAt = &now
	}
}

// finishIfTerminal stamps the write that makes a phase terminal: completedAt
// (never moved by a later write), the leader that processed it, and the instant
// the record expires — completedAt plus its phase's TTL, so the same
// compare-and-swap that ends the signal also starts its clock and nothing
// deletes it by hand. Instants are held at the second a stored status keeps, so
// a stamp read back equals the one computed here.
func (r *SignalReconciler) finishIfTerminal(ctx context.Context, sig *state.SignalRecord) {
	if !sig.Terminal() || sig.CompletedAt != nil {
		return
	}
	now := r.now().UTC().Truncate(time.Second)
	sig.CompletedAt = &now
	sig.ConsumedAt = &now
	sig.ConsumedBy = r.consumer(ctx)
	expires := now.Add(r.ttlFor(sig.Phase))
	sig.ExpiresAt = &expires
}

// consumer is the leader's identity: the leader Lease's holderIdentity, which
// carries controller-runtime's own uniqueness suffix, falling back to this
// replica's name when the Lease cannot be read.
func (r *SignalReconciler) consumer(ctx context.Context) string {
	var lease coordinationv1.Lease
	if err := r.reader().Get(ctx, types.NamespacedName{Namespace: r.Namespace, Name: LeaderLeaseName}, &lease); err == nil &&
		lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != "" {
		return *lease.Spec.HolderIdentity
	}
	return r.Identity
}

// exhaust applies the retry deadline: every non-terminal claimant becomes
// Failed with the exhaustion as its reason — or, before fan-out, the Signal is
// Dropped with reason exhausted. A PartiallyRouted signal always exits.
func (r *SignalReconciler) exhaust(ctx context.Context, p *signalPass, elapsed time.Duration) error {
	sig := &p.rec
	reason := fmt.Sprintf("retry deadline exceeded after %s (limit %s)", elapsed.Round(time.Second), r.retryDeadline())
	if len(sig.Claimants) == 0 {
		sig.Phase = state.SignalDropped
		sig.Reason = string(agentopsv1alpha1.SignalDropExhausted)
	} else {
		failRouting(sig, reason)
	}
	r.finishIfTerminal(ctx, sig)
	if err := r.write(ctx, p); err != nil {
		return err
	}
	r.surface(ctx, sig, activity.CodeRetryExhausted, activity.CodeExhausted, reason)
	return nil
}

// failRouting marks every claimant still Routing Failed and rolls the phase up.
func failRouting(sig *state.SignalRecord, reason string) {
	for i := range sig.Claimants {
		c := &sig.Claimants[i]
		if c.State == state.ClaimantRouting {
			c.State = state.ClaimantFailed
			c.Reason = reason
		}
	}
	sig.Phase = state.SignalRollupPhase(sig.Claimants)
}

// surface makes a housekeeping end as loud as the unwired drop: a Dropped
// signal carries dropCode, a Failed one failCode; a Routed roll-up (some
// claimants routed, the rest failed) is logged only, as the Signal is not lost.
func (r *SignalReconciler) surface(ctx context.Context, sig *state.SignalRecord, failCode, dropCode, detail string) {
	switch sig.Phase {
	case state.SignalDropped:
		signalroute.Loud(ctx, r.Activity, sig, activity.KindSignalDropped, dropCode, detail)
	case state.SignalFailed:
		signalroute.Loud(ctx, r.Activity, sig, activity.KindSignalFailed, failCode, detail)
	default:
		log.FromContext(ctx).Info("signal housekeeping failed its unrouted claimants",
			"signal", sig.Name, "source", sig.SourceRef, "phase", sig.Phase, "detail", detail)
	}
}

// enforceSourceCap holds the per-source cap on non-terminal Signals by
// evicting the OLDEST first. The eviction happens in the victim's OWN pass —
// the work queue is what keeps it from racing a consumer — so a pass that finds
// its source over the cap evicts itself when it is among the oldest, and
// kicks the others' passes otherwise. Reports whether sig itself was evicted.
func (r *SignalReconciler) enforceSourceCap(ctx context.Context, p *signalPass) (bool, error) {
	sig := &p.rec
	others, err := r.Signals.List(ctx, state.SignalNonTerminalBySource, sig.SourceRef)
	if err != nil {
		return false, err
	}
	live := []*state.SignalRecord{sig}
	for i := range others {
		if others[i].Name != sig.Name {
			live = append(live, &others[i])
		}
	}
	limit := r.maxPerSource()
	over := len(live) - limit
	if over <= 0 {
		return false, nil
	}
	sort.Slice(live, func(i, j int) bool { return state.SignalArrivesBefore(live[i], live[j]) })
	evictedSelf := false
	for _, victim := range live[:over] {
		if victim.Name != sig.Name {
			r.kickSignal(victim)
			continue
		}
		if err := r.evict(ctx, p, len(live), limit); err != nil {
			return false, err
		}
		evictedSelf = true
	}
	return evictedSelf, nil
}

// kickSignal enqueues another Signal's pass on the same work queue.
func (r *SignalReconciler) kickSignal(victim *state.SignalRecord) {
	if r.kick == nil {
		return
	}
	obj := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: victim.Name, Namespace: r.Namespace}}
	select {
	case r.kick <- event.GenericEvent{Object: obj}:
	default: // full: the victim's own requeue reaches it
	}
}

// evict terminates sig to hold the per-source cap: before fan-out it is
// Dropped (reason overflow), after it every non-terminal claimant is Failed
// (reason overflow) and the phase rolls up — Dropped stays strictly
// pre-fan-out. Counted and logged either way.
func (r *SignalReconciler) evict(ctx context.Context, p *signalPass, live, limit int) error {
	sig := &p.rec
	const reason = "evicted: per-source signal cap exceeded (overflow)"
	if len(sig.Claimants) == 0 {
		sig.Phase = state.SignalDropped
		sig.Reason = string(agentopsv1alpha1.SignalDropOverflow)
	} else {
		failRouting(sig, reason)
	}
	r.finishIfTerminal(ctx, sig)
	if err := r.write(ctx, p); err != nil {
		return err
	}
	log.FromContext(ctx).Info("evicted the oldest non-terminal signal of an over-cap source",
		"signal", sig.Name, "source", sig.SourceRef, "live", live, "limit", limit, "phase", sig.Phase)
	r.surface(ctx, sig, activity.CodeOverflow, activity.CodeOverflow, reason)
	return nil
}

// SetupWithManager wires the consumer pool: SIGNAL_CONSUMER_CONCURRENCY workers
// over one work queue keyed by Signal, fed by the Signals collection's feed (design
// D-14) rather than an informer on the CRD, plus the channel housekeeping uses
// to enqueue another Signal's pass. The controller runs on the leader only.
func (r *SignalReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.kick = make(chan event.GenericEvent, 64)
	return ctrl.NewControllerManagedBy(mgr).
		Named("signal").
		WatchesRawSource(source.Func(r.startFeed)).
		WatchesRawSource(source.Channel(r.kick, &handler.EnqueueRequestForObject{})).
		WithOptions(controller.Options{MaxConcurrentReconciles: r.concurrency()}).
		Complete(r)
}

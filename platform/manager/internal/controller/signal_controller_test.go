package controller

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
)

// sigCtrlNS is the namespace every fixture in this file shares.
const sigCtrlNS = "agent-ops"

// fakeSignalClient mirrors internal/httpapi's own sigServer: a fake client
// with the Signal status subresource and both informer field indexes
// RouteClaimants itself reads (IndexConversationID for the reuse lookup)
// registered, so the reconciler sees exactly the client shape production's
// cache-backed one provides.
func fakeSignalClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&agentopsv1alpha1.Signal{}, &agentopsv1alpha1.Conversation{}).
		WithIndex(&agentopsv1alpha1.Conversation{}, agentopsv1alpha1.IndexConversationID, ConversationIDIndexer).
		WithIndex(&agentopsv1alpha1.Signal{}, agentopsv1alpha1.IndexSignalInitiationID, SignalInitiationIDIndexer).
		WithObjects(objs...).Build()
}

// sigCtrlSource is a minimal SignalSource fixture.
func sigCtrlSource(name, adapter string) *agentopsv1alpha1.SignalSource {
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = name, sigCtrlNS
	src.Spec.Adapter = adapter
	return src
}

// sigCtrlPipeline is a Ready Pipeline claiming the given source, its
// capability inlined — mirrors internal/httpapi's own sigTestPipeline:
// ResolveCapability/SnapshotFor both degrade gracefully to the bootstrap
// default when neither an AgentProfile nor an AgentRuntime resolves, so no
// such object is needed here either.
func sigCtrlPipeline(name, source string) *agentopsv1alpha1.Pipeline {
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = name, sigCtrlNS
	p.Spec.SignalSourceRefs = []agentopsv1alpha1.ObjectRef{{Name: source}}
	p.Spec.AgentCapabilitySpec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "prof-" + name}
	p.Status.Conditions = []metav1.Condition{{
		Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "ready",
		LastTransitionTime: metav1.Now(),
	}}
	return p
}

// sigCtrlClaimant builds one SignalClaimant entry.
func sigCtrlClaimant(pipeline string, state agentopsv1alpha1.ClaimantState) agentopsv1alpha1.SignalClaimant {
	return agentopsv1alpha1.SignalClaimant{
		Ref:   agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKindPipeline, Name: pipeline},
		State: state,
	}
}

// sigCtrlSignal builds a Signal fixture, owned by source — the ownerRef
// every Signal carries from creation (section 2), set here with the exact
// helper ingest itself uses.
func sigCtrlSignal(t *testing.T, name string, source *agentopsv1alpha1.SignalSource, initiationID string,
	phase agentopsv1alpha1.SignalPhase, claimants []agentopsv1alpha1.SignalClaimant, firstAttempt *metav1.Time, created time.Time,
) *agentopsv1alpha1.Signal {
	t.Helper()
	sig := &agentopsv1alpha1.Signal{}
	sig.Name, sig.Namespace = name, sigCtrlNS
	sig.CreationTimestamp = metav1.NewTime(created)
	sig.Spec = agentopsv1alpha1.SignalSpec{
		SourceRef:    agentopsv1alpha1.ObjectRef{Name: source.Name},
		Adapter:      source.Spec.Adapter,
		InitiationID: initiationID,
		Payload:      "payload-" + initiationID,
		Occurrences:  1,
	}
	sig.Status.Phase = phase
	sig.Status.Claimants = claimants
	sig.Status.FirstAttemptAt = firstAttempt
	scheme := runtime.NewScheme()
	if err := agentopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := controllerutil.SetControllerReference(source, sig, scheme); err != nil {
		t.Fatal(err)
	}
	return sig
}

func sigReconcile(t *testing.T, r *SignalReconciler, name string) (ctrl.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: sigCtrlNS, Name: name},
	})
}

func getSignal(t *testing.T, c client.Client, name string) *agentopsv1alpha1.Signal {
	t.Helper()
	var sig agentopsv1alpha1.Signal
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigCtrlNS, Name: name}, &sig); err != nil {
		t.Fatal(err)
	}
	return &sig
}

// --- "A partially routed signal exits" (retry deadline) ---------------------

// One claimant already Routed, the other past the retry deadline still
// Routing: the exhausted claimant is marked Failed with the exhaustion
// recorded as its reason, and the signal rolls up to Routed — NOT Failed,
// since at least one claimant did route it (design D-5a).
func TestSignalReconcileExhaustedClaimantRollsUpToRoutedAlongsideAnAlreadyRoutedOne(t *testing.T) {
	source := sigCtrlSource("deadline-src", "deadline-t")
	past := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	sig := sigCtrlSignal(t, "deadline-sig", source, "init-1", agentopsv1alpha1.SignalPartiallyRouted,
		[]agentopsv1alpha1.SignalClaimant{
			sigCtrlClaimant("routed-pipe", agentopsv1alpha1.ClaimantRouted),
			sigCtrlClaimant("stuck-pipe", agentopsv1alpha1.ClaimantRouting),
		}, &past, past.Time)
	c := fakeSignalClient(t, source, sig)
	r := &SignalReconciler{Client: c, Activity: activity.New(64)}

	if _, err := sigReconcile(t, r, sig.Name); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := getSignal(t, c, sig.Name)
	if got.Status.Phase != agentopsv1alpha1.SignalRouted {
		t.Fatalf("phase = %s, want Routed", got.Status.Phase)
	}
	if got.Status.Claimants[1].State != agentopsv1alpha1.ClaimantFailed || got.Status.Claimants[1].Reason == "" {
		t.Fatalf("exhausted claimant must be Failed with a reason, got %+v", got.Status.Claimants[1])
	}
	if got.Status.Claimants[0].State != agentopsv1alpha1.ClaimantRouted {
		t.Fatalf("the already-routed claimant must be untouched, got %+v", got.Status.Claimants[0])
	}
	events, _ := r.Activity.Since("", 0)
	for _, e := range events {
		if e.Kind == activity.KindSignalFailed {
			t.Fatalf("a signal that still rolled up to Routed must never emit KindSignalFailed, got %+v", e)
		}
	}
}

// Every claimant past the retry deadline, none ever Routed: the signal
// rolls up to Failed, and Failed is surfaced as loudly as the unwired drop —
// an activity event AND a log line.
func TestSignalReconcileAllClaimantsExhaustedRollsUpToFailedWithLoudSurfacing(t *testing.T) {
	source := sigCtrlSource("deadline-fail-src", "deadline-fail-t")
	past := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	sig := sigCtrlSignal(t, "deadline-fail-sig", source, "init-2", agentopsv1alpha1.SignalPending,
		[]agentopsv1alpha1.SignalClaimant{sigCtrlClaimant("stuck-pipe", agentopsv1alpha1.ClaimantRouting)},
		&past, past.Time)
	c := fakeSignalClient(t, source, sig)
	acts := activity.New(64)
	r := &SignalReconciler{Client: c, Activity: acts}

	if _, err := sigReconcile(t, r, sig.Name); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := getSignal(t, c, sig.Name)
	if got.Status.Phase != agentopsv1alpha1.SignalFailed {
		t.Fatalf("phase = %s, want Failed", got.Status.Phase)
	}
	if got.Status.Claimants[0].State != agentopsv1alpha1.ClaimantFailed || got.Status.Claimants[0].Reason == "" {
		t.Fatalf("claimant must be Failed with a reason, got %+v", got.Status.Claimants[0])
	}
	events, _ := acts.Since("", 0)
	found := false
	for _, e := range events {
		if e.Kind == activity.KindSignalFailed && e.Code == activity.CodeRetryExhausted {
			found = true
		}
	}
	if !found {
		t.Fatal("a signal rolling up to Failed must emit KindSignalFailed loudly")
	}
}

// status.firstAttemptAt nil means the retry deadline's clock has not started
// — a signal still waiting for its very FIRST attempt is never treated as
// already expired, however old the object itself is. It must instead be
// given that first attempt (through the ordinary RouteClaimants call),
// which sets the stamp.
func TestSignalReconcileNilFirstAttemptAtNeverExpiresBeforeTheFirstAttempt(t *testing.T) {
	source := sigCtrlSource("nilstart-src", "nilstart-t")
	pipeline := sigCtrlPipeline("nilstart-pipe", source.Name)
	old := time.Now().Add(-48 * time.Hour) // far older than any retry deadline
	sig := sigCtrlSignal(t, "nilstart-sig", source, "init-3", agentopsv1alpha1.SignalPending, nil, nil, old)
	c := fakeSignalClient(t, source, pipeline, sig)
	r := &SignalReconciler{Client: c, Activity: activity.New(64), MaxQueued: 50}

	if _, err := sigReconcile(t, r, sig.Name); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := getSignal(t, c, sig.Name)
	if got.Status.FirstAttemptAt == nil {
		t.Fatal("the first ordinary attempt must stamp firstAttemptAt")
	}
	if len(got.Status.Claimants) != 1 || got.Status.Claimants[0].State != agentopsv1alpha1.ClaimantRouted {
		t.Fatalf("the claimant must be routed on its first attempt, got %+v", got.Status.Claimants)
	}
}

// --- "Live signals are bounded" (per-source cap + TTL GC) -------------------

// Over MAX_SIGNALS_PER_SOURCE, pre-fan-out (no claimant snapshot yet): the
// OLDEST non-terminal Signal on the source is evicted straight to Dropped,
// reason overflow — never Failed, since it never reached a claimant.
func TestSignalReconcileOverflowEvictsOldestPreFanoutAsDropped(t *testing.T) {
	source := sigCtrlSource("overflow-src", "overflow-t")
	now := time.Now()
	older := sigCtrlSignal(t, "overflow-old", source, "init-old", agentopsv1alpha1.SignalPending, nil, nil, now.Add(-time.Hour))
	newer := sigCtrlSignal(t, "overflow-new", source, "init-new", agentopsv1alpha1.SignalPending, nil, nil, now)
	c := fakeSignalClient(t, source, older, newer)
	r := &SignalReconciler{Client: c, Activity: activity.New(64), MaxPerSource: 1}

	// Reconciling the NEWER one still evicts the OLDER one — the cap looks
	// at every live Signal on the source, not only the one in hand.
	if _, err := sigReconcile(t, r, newer.Name); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	evicted := getSignal(t, c, older.Name)
	if evicted.Status.Phase != agentopsv1alpha1.SignalDropped || evicted.Status.Reason != agentopsv1alpha1.SignalDropOverflow {
		t.Fatalf("oldest signal must be Dropped/overflow, got phase=%s reason=%s", evicted.Status.Phase, evicted.Status.Reason)
	}
	events, _ := r.Activity.Since("", 0)
	found := false
	for _, e := range events {
		if e.Kind == activity.KindSignalDropped && e.Code == activity.CodeOverflow {
			found = true
		}
	}
	if !found {
		t.Fatal("a pre-fan-out overflow eviction must be counted and logged as a signal.dropped event")
	}
}

// Over MAX_SIGNALS_PER_SOURCE, post-fan-out (already has a claimant
// snapshot): every still-Routing claimant on the oldest live Signal goes
// Failed/overflow, the phase rolls up — never Dropped once fanned out
// (design D-5a).
func TestSignalReconcileOverflowEvictsOldestPostFanoutAsFailedNeverDropped(t *testing.T) {
	source := sigCtrlSource("overflow2-src", "overflow2-t")
	now := time.Now()
	older := sigCtrlSignal(t, "overflow2-old", source, "init-old2", agentopsv1alpha1.SignalPending,
		[]agentopsv1alpha1.SignalClaimant{sigCtrlClaimant("some-pipe", agentopsv1alpha1.ClaimantRouting)},
		&metav1.Time{Time: now.Add(-time.Hour)}, now.Add(-time.Hour))
	newer := sigCtrlSignal(t, "overflow2-new", source, "init-new2", agentopsv1alpha1.SignalPending, nil, nil, now)
	c := fakeSignalClient(t, source, older, newer)
	r := &SignalReconciler{Client: c, Activity: activity.New(64), MaxPerSource: 1}

	if _, err := sigReconcile(t, r, newer.Name); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	evicted := getSignal(t, c, older.Name)
	if evicted.Status.Phase == agentopsv1alpha1.SignalDropped {
		t.Fatalf("a fanned-out signal must never be evicted to Dropped, got %+v", evicted.Status)
	}
	if evicted.Status.Phase != agentopsv1alpha1.SignalFailed {
		t.Fatalf("phase = %s, want Failed (the only claimant was Routing)", evicted.Status.Phase)
	}
	if evicted.Status.Claimants[0].State != agentopsv1alpha1.ClaimantFailed || evicted.Status.Claimants[0].Reason == "" {
		t.Fatalf("claimant must be Failed with a reason, got %+v", evicted.Status.Claimants[0])
	}
	events, _ := r.Activity.Since("", 0)
	found := false
	for _, e := range events {
		if e.Kind == activity.KindSignalFailed && e.Code == activity.CodeOverflow {
			found = true
		}
	}
	if !found {
		t.Fatal("a post-fan-out overflow eviction rolling up to Failed must emit KindSignalFailed loudly")
	}
}

// A Dropped signal survives SIGNAL_TTL_DROPPED, then is deleted; it is kept
// (requeued) while still inside the window.
func TestSignalReconcileTerminalDroppedTTL(t *testing.T) {
	source := sigCtrlSource("ttl-drop-src", "ttl-drop-t")

	within := sigCtrlSignal(t, "ttl-drop-within", source, "init-ttl-1", agentopsv1alpha1.SignalDropped, nil, nil, time.Now().Add(-30*time.Minute))
	within.Status.Reason = agentopsv1alpha1.SignalDropUnclaimed
	past := sigCtrlSignal(t, "ttl-drop-past", source, "init-ttl-2", agentopsv1alpha1.SignalDropped, nil, nil, time.Now().Add(-2*time.Hour))
	past.Status.Reason = agentopsv1alpha1.SignalDropUnclaimed

	c := fakeSignalClient(t, source, within, past)
	r := &SignalReconciler{Client: c, Activity: activity.New(64), TTLDropped: time.Hour}

	if res, err := sigReconcile(t, r, within.Name); err != nil {
		t.Fatalf("Reconcile(within): %v", err)
	} else if res.RequeueAfter <= 0 {
		t.Fatal("a Dropped signal still inside its TTL must be requeued, not deleted")
	}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigCtrlNS, Name: within.Name}, &agentopsv1alpha1.Signal{}); err != nil {
		t.Fatalf("within-TTL signal must survive: %v", err)
	}

	if _, err := sigReconcile(t, r, past.Name); err != nil {
		t.Fatalf("Reconcile(past): %v", err)
	}
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigCtrlNS, Name: past.Name}, &agentopsv1alpha1.Signal{}); err == nil {
		t.Fatal("a Dropped signal past its TTL must be deleted")
	}
}

// Routed and Failed share ONE TTL (SIGNAL_TTL_ROUTED) — Failed carries no
// TTL of its own.
func TestSignalReconcileRoutedAndFailedShareTheSameTTL(t *testing.T) {
	source := sigCtrlSource("ttl-share-src", "ttl-share-t")
	routed := sigCtrlSignal(t, "ttl-share-routed", source, "init-r", agentopsv1alpha1.SignalRouted,
		[]agentopsv1alpha1.SignalClaimant{sigCtrlClaimant("p", agentopsv1alpha1.ClaimantRouted)},
		&metav1.Time{Time: time.Now()}, time.Now().Add(-2*time.Hour))
	failed := sigCtrlSignal(t, "ttl-share-failed", source, "init-f", agentopsv1alpha1.SignalFailed,
		[]agentopsv1alpha1.SignalClaimant{sigCtrlClaimant("p", agentopsv1alpha1.ClaimantFailed)},
		&metav1.Time{Time: time.Now()}, time.Now().Add(-2*time.Hour))
	c := fakeSignalClient(t, source, routed, failed)
	r := &SignalReconciler{Client: c, Activity: activity.New(64), TTLRouted: time.Hour}

	for _, name := range []string{routed.Name, failed.Name} {
		if _, err := sigReconcile(t, r, name); err != nil {
			t.Fatalf("Reconcile(%s): %v", name, err)
		}
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: sigCtrlNS, Name: name}, &agentopsv1alpha1.Signal{}); err == nil {
			t.Fatalf("%s past the shared Routed/Failed TTL must be deleted", name)
		}
	}
}

// --- ownerRef GC (plain Kubernetes consequence, confirmed not re-built) -----

// A Signal's ownerRef names its SignalSource as the controller — the plain
// Kubernetes ownerRef Kubernetes itself cascades on delete (section 2 sets
// it at creation; this reconciler builds no deletion-on-source-delete logic
// of its own).
func TestSignalOwnerReferenceNamesItsSignalSource(t *testing.T) {
	source := sigCtrlSource("owner-src", "owner-t")
	sig := sigCtrlSignal(t, "owner-sig", source, "init-owner", agentopsv1alpha1.SignalPending, nil, nil, time.Now())

	if len(sig.OwnerReferences) != 1 || sig.OwnerReferences[0].Kind != "SignalSource" || sig.OwnerReferences[0].Name != source.Name {
		t.Fatalf("ownerRef missing or wrong: %+v", sig.OwnerReferences)
	}
	if sig.OwnerReferences[0].Controller == nil || !*sig.OwnerReferences[0].Controller {
		t.Fatalf("ownerRef must be a CONTROLLER reference for GC to cascade: %+v", sig.OwnerReferences[0])
	}
}

// --- leader resume: ordinary Reconcile, no bespoke startup pass ------------

// A fresh SignalReconciler instance — standing in for a newly-elected leader
// that never saw this Signal's earlier attempts — routes it through the
// SAME ordinary Reconcile call. A claimant already Routed is never
// re-routed (no second conversation, no replayed work); the one still
// Routing is attempted normally.
func TestSignalReconcileLeaderResumeRoutesWithoutReplayingRoutedWork(t *testing.T) {
	source := sigCtrlSource("resume-src", "resume-t")
	routedPipe := sigCtrlPipeline("resume-routed-pipe", source.Name)
	pendingPipe := sigCtrlPipeline("resume-pending-pipe", source.Name)

	existingConv := &agentopsv1alpha1.Conversation{}
	existingConv.Name, existingConv.Namespace = "resume-conv", sigCtrlNS
	existingConv.Spec.PipelineRef = &agentopsv1alpha1.ObjectRef{Name: routedPipe.Name}
	existingConv.Spec.ConversationID = "init-resume"

	attempt := metav1.Now()
	sig := sigCtrlSignal(t, "resume-sig", source, "init-resume", agentopsv1alpha1.SignalPartiallyRouted,
		[]agentopsv1alpha1.SignalClaimant{
			{Ref: agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKindPipeline, Name: routedPipe.Name},
				State: agentopsv1alpha1.ClaimantRouted, ConversationRef: &agentopsv1alpha1.ObjectRef{Name: existingConv.Name}},
			sigCtrlClaimant(pendingPipe.Name, agentopsv1alpha1.ClaimantRouting),
		}, &attempt, attempt.Time)

	c := fakeSignalClient(t, source, routedPipe, pendingPipe, existingConv, sig)
	// A FRESH reconciler, built exactly as a newly-elected leader's manager
	// would build one — nothing about it remembers an earlier attempt.
	freshLeader := &SignalReconciler{Client: c, Activity: activity.New(64), MaxQueued: 50}

	if _, err := sigReconcile(t, freshLeader, sig.Name); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got := getSignal(t, c, sig.Name)
	if got.Status.Phase != agentopsv1alpha1.SignalRouted {
		t.Fatalf("phase = %s, want Routed", got.Status.Phase)
	}
	// The already-Routed claimant must still point at the SAME conversation
	// — never a second one.
	if got.Status.Claimants[0].ConversationRef == nil || got.Status.Claimants[0].ConversationRef.Name != existingConv.Name {
		t.Fatalf("already-routed claimant must not be replayed, got %+v", got.Status.Claimants[0])
	}
	if got.Status.Claimants[1].State != agentopsv1alpha1.ClaimantRouted {
		t.Fatalf("the still-Routing claimant must be routed on resume, got %+v", got.Status.Claimants[1])
	}
	var convs agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &convs, client.InNamespace(sigCtrlNS)); err != nil {
		t.Fatal(err)
	}
	if len(convs.Items) != 2 {
		t.Fatalf("resume must create exactly ONE new conversation (for the pending claimant), got %d total", len(convs.Items))
	}
}

// --- defaults ----------------------------------------------------------------

func TestSignalReconcilerDefaultsApplyWhenFieldsAreZero(t *testing.T) {
	r := &SignalReconciler{}
	if got := r.retryDeadline(); got != DefaultSignalRetryDeadline {
		t.Fatalf("retryDeadline() = %s, want default %s", got, DefaultSignalRetryDeadline)
	}
	if got := r.maxPerSource(); got != DefaultMaxSignalsPerSource {
		t.Fatalf("maxPerSource() = %d, want default %d", got, DefaultMaxSignalsPerSource)
	}
	if got := r.ttlDropped(); got != DefaultSignalTTLDropped {
		t.Fatalf("ttlDropped() = %s, want default %s", got, DefaultSignalTTLDropped)
	}
	if got := r.ttlRouted(); got != DefaultSignalTTLRouted {
		t.Fatalf("ttlRouted() = %s, want default %s", got, DefaultSignalTTLRouted)
	}
}

// Reconcile on a Signal already gone (deleted between the event and the
// reconcile) returns no error — the same client.IgnoreNotFound shape every
// reconciler in this package takes.
func TestSignalReconcileMissingObjectReturnsNilError(t *testing.T) {
	c := fakeSignalClient(t)
	r := &SignalReconciler{Client: c}
	if _, err := sigReconcile(t, r, "missing"); err != nil {
		t.Fatalf("Reconcile(missing) = %v, want nil", err)
	}
}

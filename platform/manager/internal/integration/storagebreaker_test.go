// The breaker's PROVISIONING edge.
//
// It already treated "many runs cannot reach their context" as an outage. It
// could not see the most total form of that outage — no pod starts at all, so
// no run exists to file a report — which is exactly what happened on
// 2026-08-20. These tests pin the second edge, and pin equally hard that
// failures which are NOT about storage may not open it.
package integration

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/storagebreaker"
)

// TestUnattachableVolumesOpenTheBreaker: three conversations whose pods cannot
// attach a volume is an install-wide storage incident, not three coincidences.
func TestUnattachableVolumesOpenTheBreaker(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Nanosecond
	r.StorageBreaker = storagebreaker.New()

	mkProfile(t, "p-breaker")
	for _, name := range []string{"br-1", "br-2", "br-3"} {
		mkChanConv(t, name, "p-breaker")
		reconcileWith(t, r, name)
		markPodUnschedulableOnVolume(t, name)
		reconcileWith(t, r, name) // reap: one storage-attributable report each
	}

	open, _ := r.StorageBreaker.Open()
	if !open {
		t.Fatal("three unattachable volumes inside the window is an outage, not three losses")
	}
}

// TestNonStorageFailuresNeverOpenTheBreaker is the guard that keeps the
// mechanism honest. An unpullable image is a real failure and a real reason to
// reap the pod, but holding every conversation in the install for it would be
// an outage manufactured out of an unrelated fault.
func TestNonStorageFailuresNeverOpenTheBreaker(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Nanosecond
	r.StorageBreaker = storagebreaker.New()

	mkProfile(t, "p-image")
	for _, name := range []string{"img-1", "img-2", "img-3", "img-4"} {
		mkChanConv(t, name, "p-image")
		reconcileWith(t, r, name)
		markPodImagePullFailure(t, name)
		reconcileWith(t, r, name)
	}

	if open, _ := r.StorageBreaker.Open(); open {
		t.Fatal("image pull failures must never be read as a storage outage")
	}
}

// TestOpenBreakerHoldsWorkAndSaysWhy: while storage is out, work is HELD, not
// failed — and the conversation says storage rather than looking like a queue.
func TestOpenBreakerHoldsWorkAndSaysWhy(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Hour
	b := storagebreaker.New()
	for i := 0; i < storagebreaker.Threshold; i++ {
		b.Report()
	}
	if open, _ := b.Open(); !open {
		t.Fatal("precondition: breaker should be open")
	}
	// Consume the free first probe so this conversation meets a closed door.
	b.ProbeDue(time.Now())
	r.StorageBreaker = b

	mkProfile(t, "p-held")
	mkChanConv(t, "held-1", "p-held")
	reconcileWith(t, r, "held-1")

	if podExists(t, "held-1") {
		t.Fatal("no pod may be provisioned while storage is being treated as an outage")
	}
	if got := phaseOf(t, "held-1"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("phase = %q, want Pending", got)
	}
	cond := conditionOf(t, "held-1", controller.ConditionRuntimeStarted)
	if cond == nil || cond.Reason != controller.ReasonStorageUnavailable {
		t.Fatalf("held conversation must say STORAGE, not look like a queue; got %+v", cond)
	}

	// Held, never failed: the input is still there to be dispatched when
	// storage returns. Failing it would spend the message on an outage.
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "held-1"}, &conv); err != nil {
		t.Fatal(err)
	}
	if len(conv.Spec.Inputs) == 0 {
		t.Fatal("the input must be held for later dispatch, not consumed by the outage")
	}
}

// TestOneCanaryProbesRecovery: the provisioning edge cannot close its own
// breaker, so exactly one attempt is let through per interval to re-test.
func TestOneCanaryProbesRecovery(t *testing.T) {
	b := storagebreaker.New()
	for i := 0; i < storagebreaker.Threshold; i++ {
		b.Report()
	}
	now := time.Now()
	if !b.ProbeDue(now) {
		t.Fatal("the first attempt after opening should be let through")
	}
	if b.ProbeDue(now.Add(time.Second)) {
		t.Fatal("a second attempt inside the interval must be refused: one canary, not a herd")
	}
	if !b.ProbeDue(now.Add(storagebreaker.ProbeInterval + time.Second)) {
		t.Fatal("a probe must be due again after the interval, or recovery is never noticed")
	}
	b.Continued()
	if open, _ := b.Open(); open {
		t.Fatal("a successful continuation closes the breaker")
	}
}

// TestBlockedStartEmitsNoSignal is the invariant guard: agent-ops' own health
// is STATUS, never SIGNAL. Routing a failed runtime pod back through ingest
// would wake an agent, whose conversation would create another runtime pod
// under a new name, forever — and nothing downstream catches it, because every
// fingerprint is fresh.
func TestBlockedStartEmitsNoSignal(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Nanosecond
	r.StorageBreaker = storagebreaker.New()

	before := countConversations(t, "p-nosignal")

	mkProfile(t, "p-nosignal")
	mkChanConv(t, "nosig-1", "p-nosignal")
	reconcileWith(t, r, "nosig-1")
	markPodUnschedulableOnVolume(t, "nosig-1")
	reconcileWith(t, r, "nosig-1")

	if after := countConversations(t, "p-nosignal"); after != before+1 {
		t.Fatalf("a failed runtime start created %d extra conversations; it must create NONE — "+
			"that is the signal loop the self-exclusion rules exist to stop", after-before-1)
	}
}

// FAULT INJECTION, end to end: make the volume unattachable, confirm the
// breaker opens, work is HELD rather than failed, nothing is provisioned, and
// recovery drains in FIFO order.
//
// This is the 2026-08-20 outage replayed against the fix.
func TestStorageOutageHoldsThenDrainsInOrder(t *testing.T) {
	r := reconcilerWithCap(nil, 100)
	r.RuntimeStartDeadline = time.Nanosecond
	b := storagebreaker.New()
	r.StorageBreaker = b

	mkProfile(t, "p-e2e")

	// Three conversations whose pods cannot attach: an install-wide incident.
	victims := []string{"e2e-1", "e2e-2", "e2e-3"}
	for _, n := range victims {
		mkChanConv(t, n, "p-e2e")
		reconcileWith(t, r, n)
		markPodUnschedulableOnVolume(t, n)
		reconcileWith(t, r, n)
	}
	if open, _ := b.Open(); !open {
		t.Fatal("three unattachable volumes must be read as an outage")
	}

	// A conversation arriving DURING the outage is held, not failed, and is
	// given no pod at all.
	b.ProbeDue(time.Now()) // consume the free canary
	mkChanConv(t, "e2e-late", "p-e2e")
	reconcileWith(t, r, "e2e-late")
	if podExists(t, "e2e-late") {
		t.Fatal("nothing may be provisioned while storage is treated as an outage")
	}
	if got := phaseOf(t, "e2e-late"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("phase = %q, want Pending", got)
	}
	var late agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "e2e-late"}, &late); err != nil {
		t.Fatal(err)
	}
	if len(late.Spec.Inputs) == 0 {
		t.Fatal("the input must be HELD for later dispatch, never consumed by the outage")
	}

	// Storage returns.
	b.Continued()
	if open, _ := b.Open(); open {
		t.Fatal("a successful continuation must close the breaker")
	}

	// And work flows again.
	r2 := reconcilerWithCap(nil, 100)
	r2.RuntimeStartDeadline = time.Hour
	r2.StorageBreaker = b
	reconcileWith(t, r2, "e2e-late")
	if !podExists(t, "e2e-late") {
		t.Fatal("held work must be provisioned once storage is reachable again")
	}
}

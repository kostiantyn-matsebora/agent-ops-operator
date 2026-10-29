package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/ingest"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// The consumer pool is leader-gated by controller-runtime itself. These tests
// run real managers with leader election over the real API server, wiring the
// SignalReconciler as main.go does, so what they pin is the gating a standby
// actually gets — not a stand-in that reconciles when told.

type replica struct {
	mgr  ctrl.Manager
	stop context.CancelFunc
	done chan struct{}
}

// startReplica starts one manager replica competing for the Lease in nsName.
func startReplica(t *testing.T, nsName string, mod func(*controller.SignalReconciler)) *replica {
	t.Helper()
	mgr, err := ctrl.NewManager(testEnv.Config, ctrl.Options{
		Scheme:                        scheme,
		Cache:                         cache.Options{DefaultNamespaces: map[string]cache.Config{nsName: {}}},
		Metrics:                       metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress:        "0",
		LeaderElection:                true,
		LeaderElectionID:              controller.LeaderLeaseName,
		LeaderElectionNamespace:       nsName,
		LeaderElectionReleaseOnCancel: true,
		LeaseDuration:                 durPtr(2 * time.Second),
		RenewDeadline:                 durPtr(1 * time.Second),
		RetryPeriod:                   durPtr(200 * time.Millisecond),
		Controller:                    ctrlconfig.Controller{SkipNameValidation: boolPtr(true)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.SetupConversationIndexes(mgr.GetFieldIndexer()); err != nil {
		t.Fatal(err)
	}
	r := &controller.SignalReconciler{
		Client: mgr.GetClient(), Reader: mgr.GetAPIReader(),
		Cooldowns: ingest.NewCooldownBook(),
		Runtime:   runtimepod.Config{ContextPVC: "test-context"},
		MaxQueued: 50, Identity: "integration-replica",
		OrderRetry: time.Millisecond, RetryInterval: 100 * time.Millisecond,
	}
	if mod != nil {
		mod(r)
	}
	if err := r.SetupWithManager(mgr); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	rep := &replica{mgr: mgr, stop: cancel, done: make(chan struct{})}
	go func() {
		defer close(rep.done)
		_ = mgr.Start(ctx)
	}()
	t.Cleanup(rep.shutdown)
	return rep
}

func (r *replica) shutdown() {
	r.stop()
	select {
	case <-r.done:
	case <-time.After(15 * time.Second):
	}
}

func (r *replica) leads() bool {
	select {
	case <-r.mgr.Elected():
		return true
	default:
		return false
	}
}

func durPtr(d time.Duration) *time.Duration { return &d }
func boolPtr(b bool) *bool                  { return &b }

func mkNamespace(t *testing.T, name string) {
	t.Helper()
	n := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := k8sClient.Create(context.Background(), n); err != nil {
		t.Fatal(err)
	}
}

func mkSourceIn(t *testing.T, nsName, name string) *agentopsv1alpha1.SignalSource {
	t.Helper()
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = name, nsName
	src.Spec.Adapter = "adapter-" + name
	if err := k8sClient.Create(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	return src
}

func mkReadyPipelineIn(t *testing.T, nsName, name string, sources ...string) {
	t.Helper()
	ctx := context.Background()
	p := &agentopsv1alpha1.Pipeline{}
	p.Name, p.Namespace = name, nsName
	for _, s := range sources {
		p.Spec.SignalSourceRefs = append(p.Spec.SignalSourceRefs, agentopsv1alpha1.ObjectRef{Name: s})
	}
	p.Spec.ProfileRef = &agentopsv1alpha1.ObjectRef{Name: "prof-" + name}
	if err := k8sClient.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	p.Status.Conditions = []metav1.Condition{{
		Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", Message: "ready", LastTransitionTime: metav1.Now(),
	}}
	if err := k8sClient.Status().Update(ctx, p); err != nil {
		t.Fatal(err)
	}
}

func mkSignalIn(t *testing.T, nsName string, src *agentopsv1alpha1.SignalSource, name, initiationID string) *agentopsv1alpha1.Signal {
	t.Helper()
	sig := &agentopsv1alpha1.Signal{}
	sig.Name, sig.Namespace = name, nsName
	sig.Spec = agentopsv1alpha1.SignalSpec{
		SourceRef: agentopsv1alpha1.ObjectRef{Name: src.Name}, Adapter: src.Spec.Adapter,
		InitiationID: initiationID, Payload: "payload of " + name,
	}
	if err := ctrl.SetControllerReference(src, sig, scheme); err != nil {
		t.Fatal(err)
	}
	if err := k8sClient.Create(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
	return sig
}

func getSignalIn(t *testing.T, nsName, name string) *agentopsv1alpha1.Signal {
	t.Helper()
	var sig agentopsv1alpha1.Signal
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: nsName, Name: name}, &sig); err != nil {
		t.Fatal(err)
	}
	return &sig
}

// eventually polls cond against the API server up to a deadline; the failure
// message names what never held.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("never held: %s", what)
}

func conversationsIn(t *testing.T, nsName string) []agentopsv1alpha1.Conversation {
	t.Helper()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(context.Background(), &list, client.InNamespace(nsName)); err != nil {
		t.Fatal(err)
	}
	return list.Items
}

func inputsIn(t *testing.T, nsName string) int {
	t.Helper()
	var list agentopsv1alpha1.ConversationInputList
	if err := k8sClient.List(context.Background(), &list, client.InNamespace(nsName)); err != nil {
		t.Fatal(err)
	}
	return len(list.Items)
}

func signalGone(nsName, name string) bool {
	var sig agentopsv1alpha1.Signal
	err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: nsName, Name: name}, &sig)
	return apierrors.IsNotFound(err)
}

func terminate(t *testing.T, nsName, name string, phase agentopsv1alpha1.SignalPhase, completed time.Time) {
	t.Helper()
	sig := getSignalIn(t, nsName, name)
	c := metav1.NewTime(completed)
	sig.Status.Phase, sig.Status.CompletedAt = phase, &c
	if err := k8sClient.Status().Update(context.Background(), sig); err != nil {
		t.Fatal(err)
	}
}

// holdLeaseElsewhere seeds the leader Lease as held, fresh, by another replica.
func holdLeaseElsewhere(t *testing.T, nsName string) {
	t.Helper()
	holder, dur, now := "other-replica_1", int32(3600), metav1.NewMicroTime(time.Now())
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: controller.LeaderLeaseName, Namespace: nsName},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: &holder, LeaseDurationSeconds: &dur, AcquireTime: &now, RenewTime: &now},
	}
	if err := k8sClient.Create(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
}

// expireLease makes the seeded holder look dead, so a standby may take over.
func expireLease(t *testing.T, nsName string) {
	t.Helper()
	var lease coordinationv1.Lease
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: nsName, Name: controller.LeaderLeaseName}, &lease); err != nil {
		t.Fatal(err)
	}
	old, dur := metav1.NewMicroTime(time.Now().Add(-time.Hour)), int32(1)
	lease.Spec.RenewTime, lease.Spec.LeaseDurationSeconds = &old, &dur
	if err := k8sClient.Update(context.Background(), &lease); err != nil {
		t.Fatal(err)
	}
}

// A standby replica runs no consumer, retry deadline, cap or TTL pass; the same
// replica does every one the moment it holds the Lease. The second half is the
// control: without it, "nothing happened" could just be a broken consumer.
func TestSignalHousekeepingAndConsumerRunOnTheLeaderOnly(t *testing.T) {
	const n = "sigleader-gate"
	mkNamespace(t, n)
	holdLeaseElsewhere(t, n)

	main := mkSourceIn(t, n, "main")
	late := mkSourceIn(t, n, "late")
	capped := mkSourceIn(t, n, "capped")
	mkReadyPipelineIn(t, n, "ops", "main")

	mkSignalIn(t, n, main, "live", "live-1")
	mkSignalIn(t, n, late, "stale", "stale-1")
	st := getSignalIn(t, n, "stale")
	first := metav1.NewTime(time.Now().Add(-2 * time.Hour))
	st.Status.FirstAttemptAt = &first
	if err := k8sClient.Status().Update(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	mkSignalIn(t, n, capped, "cap-old", "cap-1")
	time.Sleep(1100 * time.Millisecond) // creationTimestamp has one-second resolution
	mkSignalIn(t, n, capped, "cap-new", "cap-2")
	mkSignalIn(t, n, main, "expired", "expired-1")
	terminate(t, n, "expired", agentopsv1alpha1.SignalRouted, time.Now().Add(-9*time.Hour))

	standby := startReplica(t, n, func(r *controller.SignalReconciler) { r.MaxPerSource = 1 })

	select {
	case <-standby.mgr.Elected():
		t.Fatal("the standby won the election against a live Lease")
	case <-time.After(2500 * time.Millisecond):
	}
	for _, name := range []string{"live", "stale", "cap-old", "cap-new"} {
		got := getSignalIn(t, n, name)
		if got.Status.Phase != "" && got.Status.Phase != agentopsv1alpha1.SignalPending {
			t.Errorf("a standby consumed %s: %+v", name, got.Status)
		}
		if got.Status.Attempts != 0 && name != "stale" {
			t.Errorf("a standby attempted %s", name)
		}
	}
	if signalGone(n, "expired") {
		t.Error("a standby ran a TTL delete")
	}
	if len(conversationsIn(t, n)) != 0 {
		t.Error("a standby opened a conversation")
	}

	expireLease(t, n)
	eventually(t, "the standby took the Lease over", standby.leads)
	eventually(t, "the new leader routed the live signal", func() bool {
		return getSignalIn(t, n, "live").Status.Phase == agentopsv1alpha1.SignalRouted
	})
	eventually(t, "the new leader enforced the retry deadline", func() bool {
		s := getSignalIn(t, n, "stale").Status
		return s.Phase == agentopsv1alpha1.SignalDropped && s.Reason == agentopsv1alpha1.SignalDropExhausted
	})
	eventually(t, "the new leader evicted the oldest over the cap", func() bool {
		s := getSignalIn(t, n, "cap-old").Status
		return s.Phase == agentopsv1alpha1.SignalDropped && s.Reason == agentopsv1alpha1.SignalDropOverflow
	})
	eventually(t, "the new leader deleted the expired terminal signal", func() bool { return signalGone(n, "expired") })
}

// A PartiallyRouted signal is resumed by the next leader from its recorded
// claimants. The status the old leader lost (one claimant's Routed write)
// must not make the new leader open that claimant's conversation a second time.
func TestAPartiallyRoutedSignalResumedAfterALeaderChangeOpensNoSecondConversation(t *testing.T) {
	const n = "sigleader-resume"
	mkNamespace(t, n)
	src := mkSourceIn(t, n, "alerts")
	mkReadyPipelineIn(t, n, "first", "alerts")
	mkReadyPipelineIn(t, n, "second", "alerts")
	mkSignalIn(t, n, src, "sig", "incident-1")

	old := startReplica(t, n, nil)
	eventually(t, "the first leader routed both claimants", func() bool {
		return getSignalIn(t, n, "sig").Status.Phase == agentopsv1alpha1.SignalRouted
	})
	convs, inputs := len(conversationsIn(t, n)), inputsIn(t, n)
	if convs == 0 {
		t.Fatal("setup: nothing was routed")
	}
	old.shutdown()

	// The leader died between creating the second claimant's conversation and
	// recording that it had.
	sig := getSignalIn(t, n, "sig")
	if len(sig.Status.Claimants) != 2 {
		t.Fatalf("setup: want 2 claimants, got %+v", sig.Status.Claimants)
	}
	sig.Status.Phase = agentopsv1alpha1.SignalPartiallyRouted
	sig.Status.CompletedAt, sig.Status.ConsumedBy, sig.Status.ConsumedAt = nil, "", nil
	sig.Status.Claimants[1].State, sig.Status.Claimants[1].ConversationRef = agentopsv1alpha1.ClaimantRouting, nil
	if err := k8sClient.Status().Update(context.Background(), sig); err != nil {
		t.Fatal(err)
	}

	next := startReplica(t, n, nil)
	eventually(t, "the new leader took over", next.leads)
	eventually(t, "the new leader resumed the signal to Routed", func() bool {
		return getSignalIn(t, n, "sig").Status.Phase == agentopsv1alpha1.SignalRouted
	})

	got := getSignalIn(t, n, "sig")
	for i, c := range got.Status.Claimants {
		if c.State != agentopsv1alpha1.ClaimantRouted || c.ConversationRef == nil {
			t.Errorf("claimant %d not resolved onto a conversation: %+v", i, c)
		}
	}
	if after := len(conversationsIn(t, n)); after != convs {
		t.Errorf("conversations after the resume = %d, want the %d the first leader opened", after, convs)
	}
	if after := inputsIn(t, n); after != inputs {
		t.Errorf("inputs after the resume = %d, want the %d the first leader appended", after, inputs)
	}
}

// Each terminal phase has its own TTL, measured from completedAt: Dropped 4h,
// Routed 8h, Failed 72h. A signal one hour inside its window stays; one hour
// outside goes. The ordering is the point — the same age deletes a Dropped
// one and keeps a Routed one, deletes a Routed one and keeps a Failed one.
func TestTerminalSignalsExpireOnTheirOwnPhasesTTLInOrder(t *testing.T) {
	const n = "sigleader-ttl"
	mkNamespace(t, n)
	src := mkSourceIn(t, n, "alerts")
	cases := []struct {
		name  string
		phase agentopsv1alpha1.SignalPhase
		age   time.Duration
		gone  bool
	}{
		{"dropped-inside", agentopsv1alpha1.SignalDropped, 3 * time.Hour, false},
		{"dropped-outside", agentopsv1alpha1.SignalDropped, 5 * time.Hour, true},
		{"routed-inside", agentopsv1alpha1.SignalRouted, 7 * time.Hour, false},
		{"routed-outside", agentopsv1alpha1.SignalRouted, 9 * time.Hour, true},
		{"failed-inside", agentopsv1alpha1.SignalFailed, 71 * time.Hour, false},
		{"failed-outside", agentopsv1alpha1.SignalFailed, 73 * time.Hour, true},
		{"routed-at-failed-window", agentopsv1alpha1.SignalRouted, 10 * time.Hour, true},
		{"failed-at-routed-window", agentopsv1alpha1.SignalFailed, 10 * time.Hour, false},
	}
	for i, c := range cases {
		mkSignalIn(t, n, src, c.name, fmt.Sprintf("ttl-%d", i))
		terminate(t, n, c.name, c.phase, time.Now().Add(-c.age))
	}

	startReplica(t, n, nil)
	for _, c := range cases {
		if c.gone {
			c := c
			eventually(t, c.name+" deleted", func() bool { return signalGone(n, c.name) })
		}
	}
	for _, c := range cases {
		if !c.gone && signalGone(n, c.name) {
			t.Errorf("%s (%s, %s old) was deleted inside its TTL", c.name, c.phase, c.age)
		}
	}
}

// The cascade from a deleted SignalSource is the API server's garbage
// collector, which envtest does not run, so what can be pinned here is the
// reference it follows: a Signal the publisher stores names its source by
// kind, name and UID, as controller owner.
func TestAPublishedSignalIsOwnedByItsSignalSource(t *testing.T) {
	mkSignalSource(t, "own-src", "own-t", "")
	h := apiServer().Handler()
	body, _ := json.Marshal(map[string]any{"source": "own-src", "signals": []map[string]any{{"initiationId": "own-1", "payload": "x"}}})
	req := httptest.NewRequest("POST", "/signal/inbound", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testMasterToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body.String())
	}
	sig := onlySignalFor(t, "own-1")
	var src agentopsv1alpha1.SignalSource
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: "own-src"}, &src); err != nil {
		t.Fatal(err)
	}
	if len(sig.OwnerReferences) != 1 {
		t.Fatalf("want exactly one ownerReference, got %+v", sig.OwnerReferences)
	}
	o := sig.OwnerReferences[0]
	if o.Kind != "SignalSource" || o.Name != "own-src" || o.UID != src.UID || o.APIVersion != agentopsv1alpha1.GroupVersion.String() {
		t.Errorf("ownerReference does not identify the source: %+v", o)
	}
	if o.Controller == nil || !*o.Controller {
		t.Errorf("the source must be the controller owner: %+v", o)
	}
}

// A Signal whose source was deleted (garbage collection not yet caught up) is
// left alone by the leader rather than routed or errored.
func TestALeaderIgnoresALiveSignalWhoseSourceIsGone(t *testing.T) {
	const n = "sigleader-orphan"
	mkNamespace(t, n)
	src := mkSourceIn(t, n, "alerts")
	mkReadyPipelineIn(t, n, "ops", "alerts")
	mkSignalIn(t, n, src, "orphan", "orphan-1")
	if err := k8sClient.Delete(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	rep := startReplica(t, n, nil)
	eventually(t, "leading", rep.leads)
	time.Sleep(time.Second)
	if got := getSignalIn(t, n, "orphan").Status; got.Phase == agentopsv1alpha1.SignalRouted || len(conversationsIn(t, n)) != 0 {
		t.Errorf("an orphaned signal was routed: %+v", got)
	}
}

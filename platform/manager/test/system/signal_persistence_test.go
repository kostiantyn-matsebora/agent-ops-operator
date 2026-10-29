//go:build system

package system

// Section 6 (signal-persistence) — the publish/consume broker end to end
// against a REAL /signal/inbound HTTP endpoint (publish only: store and ack),
// the leader's REAL consumer pool, a REAL owned Signal CR and REAL
// Conversation CRs, plus the one event only a live cluster adds: a genuine
// manager restart handing a Signal the consumer left non-terminal to a fresh
// leader, with neither claimant's conversation duplicated. The claimant
// state machine and roll-up MATH (signalroute, agentopsv1alpha1.RollupPhase)
// is pinned at envtest (internal/controller/signal_controller_test.go) —
// nothing here re-derives that logic. What only a real cluster adds is a
// real OwnerReference set by the real API server, the real leader Lease and a
// real rolled Deployment recovering from nothing but the Signal CRs
// themselves.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// setupDedicatedSignalSource wires one fresh SignalSource to one fresh
// Pipeline, isolated from the shared wiring and from every other test's own
// source — correlation_test.go's setupDedicatedCorrelationSource, under
// this file's own name so a signal-persistence lane never shares a
// cooldown cache or a Signal/Conversation count with any other test.
func setupDedicatedSignalSource(t *testing.T, ctx context.Context, e *Env, name string) *agentopsv1alpha1.SignalSource {
	t.Helper()
	src := source(name, "e2e", nil)
	mustCreate(t, e.K, src)
	p := pipeline(name, ProfileStub, []string{name}, []string{ChannelConsole})
	mustCreate(t, e.K, p)
	if err := waitPipelineReady(ctx, e.K, name, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	return src
}

// signalForInitiationID finds the Signal CR a source persisted for id — the
// system pack's own equivalent of e.ConversationFor, read off the live
// cluster's Signal list rather than the Conversation one.
func signalForInitiationID(ctx context.Context, e *Env, sourceName, id string) (*agentopsv1alpha1.Signal, error) {
	items, err := e.K.Signals(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		s := &items[i]
		if s.Spec.SourceRef.Name == sourceName && s.Spec.InitiationID == id {
			return s, nil
		}
	}
	return nil, nil
}

// waitSignalPhase polls until the Signal for (sourceName, id) reaches one of
// the wanted phases, returning it.
func waitSignalPhase(t *testing.T, ctx context.Context, e *Env, sourceName, id string, timeout time.Duration, want ...agentopsv1alpha1.SignalPhase) *agentopsv1alpha1.Signal {
	t.Helper()
	var found *agentopsv1alpha1.Signal
	waitFor(t, fmt.Sprintf("the signal for %s/%s to reach phase %v", sourceName, id, want), timeout, func() (bool, error) {
		sig, err := signalForInitiationID(ctx, e, sourceName, id)
		if err != nil || sig == nil {
			return false, err
		}
		found = sig
		for _, w := range want {
			if sig.Status.Phase == w {
				return true, nil
			}
		}
		return false, nil
	})
	return found
}

// assertClaimant asserts one named claimant's own state on sig.
func assertClaimant(t *testing.T, sig *agentopsv1alpha1.Signal, name string, want agentopsv1alpha1.ClaimantState) {
	t.Helper()
	for _, c := range sig.Status.Claimants {
		if c.Ref.Name == name {
			if c.State != want {
				t.Fatalf("claimant %s on signal %s: got state %s, want %s (%+v)", name, sig.Name, c.State, want, c)
			}
			return
		}
	}
	t.Fatalf("claimant %s not found on signal %s: %+v", name, sig.Name, sig.Status.Claimants)
}

// assertExactlyOneConversationPerClaimant asserts that exactly one
// conversation in the namespace carries conversationId==id and names each
// pipeline as its own PipelineRef — the idempotency claim a restart
// mid-routing must hold: equality-reuse (already open before the restart)
// and a genuinely fresh create (after resume) must never both leave a
// conversation behind for the SAME claimant.
func assertExactlyOneConversationPerClaimant(t *testing.T, ctx context.Context, e *Env, id string, pipelineNames ...string) {
	t.Helper()
	items, err := e.K.Conversations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for i := range items {
		c := &items[i]
		if c.Spec.ConversationID != id || c.Spec.PipelineRef == nil {
			continue
		}
		counts[c.Spec.PipelineRef.Name]++
	}
	for _, name := range pipelineNames {
		if counts[name] != 1 {
			t.Fatalf("pipeline %s must own exactly one conversation for %s, got %d", name, id, counts[name])
		}
	}
}

// 6.1.a The happy-path lane: a posted signal persists as an owned Signal CR
// before the manager answers, is routed by the leader's consumer, and reaches
// Routed naming the real conversation it opened and the replica that did it.
func TestSignalPersistenceReachesRoutedPhase(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	const name = "e2e-sigpersist-happy"
	src := setupDedicatedSignalSource(t, ctx, e, name)

	stamp := fmt.Sprint(time.Now().UnixNano())
	id := "e2e-sigpersist-happy-" + stamp
	e.PostSignal(t, name, map[string]any{"initiationId": id, "kind": "task", "payload": "first", "title": id})

	sig := waitSignalPhase(t, ctx, e, name, id, 2*time.Minute, agentopsv1alpha1.SignalRouted)

	owned := false
	for _, ref := range sig.OwnerReferences {
		if ref.Kind == "SignalSource" && ref.Name == src.Name {
			owned = true
		}
	}
	if !owned {
		t.Fatalf("the Signal must be owned by its SignalSource, got owners %+v", sig.OwnerReferences)
	}
	if sig.Status.CompletedAt == nil || sig.Status.ConsumedBy == "" || sig.Status.ConsumedAt == nil {
		t.Fatalf("a terminal Signal records when and by whom it was consumed: %+v", sig.Status)
	}
	if sig.Spec.InitiationID != id {
		t.Fatalf("the Signal must carry the posted initiationId, got %q want %q", sig.Spec.InitiationID, id)
	}
	if len(sig.Status.Claimants) != 1 || sig.Status.Claimants[0].State != agentopsv1alpha1.ClaimantRouted {
		t.Fatalf("exactly one Routed claimant is expected, got %+v", sig.Status.Claimants)
	}
	convRef := sig.Status.Claimants[0].ConversationRef
	if convRef == nil || convRef.Name == "" {
		t.Fatalf("a Routed claimant must name the conversation it opened: %+v", sig.Status.Claimants[0])
	}
	conv, err := e.K.Conversation(ctx, convRef.Name)
	if err != nil {
		t.Fatalf("the conversation the claimant named must exist: %v", err)
	}
	if conv.Spec.ConversationID != id {
		t.Fatalf("the routed conversation must carry the posted initiationId as its conversationId, got %q want %q", conv.Spec.ConversationID, id)
	}
	t.Cleanup(func() {
		_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: conv.Name}})
	})
	// The owning SignalSource's own cleanup (setupDedicatedSignalSource's
	// mustCreate) deletes it on teardown, which cascades the owned Signal
	// away too (design D-6's ownerRef) — nothing here deletes sig directly.
}

// 6.1.b Manager-restart resume: a Signal left PartiallyRouted — one
// claimant Routed by equality-reuse, the other still Routing because the
// pending-conversation backlog was (deliberately, temporarily) forced full
// — survives a real manager restart and finishes routing without ever
// duplicating either claimant's conversation. FULL TIER ONLY: this is
// TWO real rolled Deployments, heavier than this pack's smoke budget
// affords for one assertion — the same call correlation_test.go's own
// TestCorrelationRepeatOutsideCooldownRecursOnOpenConversation already
// makes for a single restart.
//
// WHY THIS DRIVES PartiallyRouted THE WAY IT DOES: the consumer attempts
// every Routing claimant in ONE pass, so there is no window to observe two
// claimants of the SAME pass finishing at different times. The only
// per-claimant branch is the pending-backlog check ahead of a CREATE
// (design D-8): a claimant that already has an open, reusable conversation
// for this initiationId (equality-reuse) skips that check entirely and
// always succeeds; a claimant with none must CREATE, which the check
// refuses — left Routing, not Failed — whenever the backlog is full.
// Setting MAX_QUEUED_CONVERSATIONS=0 makes that check UNCONDITIONALLY true
// (a non-negative count is always >= 0), so this needs no fragile
// saturation of dozens of real pending conversations racing this pack's
// other lanes for the same global cap: one claimant with a pre-existing
// open conversation routes by reuse regardless of backlog, the other —
// which has none — is GUARANTEED to stay Routing, deterministically,
// whatever any other lane's own backlog happens to be at the time.
func TestSignalPersistenceResumesPartiallyRoutedAfterManagerRestart(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	const p1 = "e2e-sigpersist-p1"
	const p2 = "e2e-sigpersist-p2"
	const soleSource = "e2e-sigpersist-sole"     // claimed by p1 alone
	const sharedSource = "e2e-sigpersist-shared" // claimed by p1 AND p2
	id := "e2e-sigpersist-partial-" + stamp

	// Deletes every conversation this test opens, under either pipeline,
	// however far the test got — registered before any risky call, exactly
	// as replicas_test.go's scaleManagerReplicas explains: a t.Fatal on a
	// forced rollout must still leave nothing behind for a later lane's own
	// backlog count.
	t.Cleanup(func() {
		items, err := e.K.Conversations(context.Background())
		if err != nil {
			return
		}
		for i := range items {
			if items[i].Spec.ConversationID == id {
				_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: items[i].Name}})
			}
		}
	})

	mustCreate(t, e.K, source(soleSource, "e2e", nil))
	mustCreate(t, e.K, source(sharedSource, "e2e", nil))
	mustCreate(t, e.K, pipeline(p1, ProfileStub, []string{soleSource, sharedSource}, []string{ChannelConsole}))
	mustCreate(t, e.K, pipeline(p2, ProfileStub, []string{sharedSource}, []string{ChannelConsole}))
	if err := waitPipelineReady(ctx, e.K, p1, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := waitPipelineReady(ctx, e.K, p2, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	// P1 opens a conversation for id via the SOLE source first, ordinary
	// equality routing — nothing about this first post is capacity-sensitive,
	// so it runs before the backlog is touched at all.
	e.PostSignal(t, soleSource, map[string]any{"initiationId": id, "kind": "task", "payload": "p1 only", "title": id})
	e.ConversationFor(t, id, 2*time.Minute)

	// Force every FRESH create to stay Routing — see the doc comment above.
	restoreQueueCap := setManagerQueueCap(t, ctx, e, "0")

	// The SAME initiationId, now on the SHARED source: a fresh Signal CR
	// (keyed on sharedSource, not soleSource), fanning out to BOTH p1 and
	// p2. P1 reuses its own already-open conversation (skips the backlog
	// check, routes at once); p2 has none and must create, which the
	// forced-full backlog refuses.
	e.PostSignal(t, sharedSource, map[string]any{"initiationId": id, "kind": "task", "payload": "shared", "title": id})

	sig := waitSignalPhase(t, ctx, e, sharedSource, id, 2*time.Minute, agentopsv1alpha1.SignalPartiallyRouted)
	assertClaimant(t, sig, p1, agentopsv1alpha1.ClaimantRouted)
	assertClaimant(t, sig, p2, agentopsv1alpha1.ClaimantRouting)

	// Restore the backlog bound and roll the manager — the real-cluster
	// event this test exists for: a fresh leader must resume p2's routing
	// from the Signal CR alone (design D-3's "crash recovery is free"),
	// with no in-memory record of the first attempt surviving the restart.
	restoreQueueCap()

	sig = waitSignalPhase(t, ctx, e, sharedSource, id, 3*time.Minute, agentopsv1alpha1.SignalRouted)
	assertClaimant(t, sig, p1, agentopsv1alpha1.ClaimantRouted)
	assertClaimant(t, sig, p2, agentopsv1alpha1.ClaimantRouted)

	// No duplicate conversation for EITHER claimant — idempotency across the
	// restart, not merely across one process's own retries.
	assertExactlyOneConversationPerClaimant(t, ctx, e, id, p1, p2)
}

// setManagerQueueCap overrides MAX_QUEUED_CONVERSATIONS on the manager
// Deployment and rolls it, reusing replicas_test.go's own
// forceManagerRollout for the scheduling-safe mechanics — no new rollout
// mechanism, exactly as correlation_test.go's restartManager and
// replicas_test.go's scaleManagerReplicas already share it for their own
// Deployment edits. Returns a restore function the caller invokes to put
// the original value back and roll again — also registered as a
// t.Cleanup safety net BEFORE the first rollout, so a t.Fatal partway
// through this test still leaves the chart's own default for whatever
// lane runs after it, the same ordering scaleManagerReplicas's own comment
// explains.
func setManagerQueueCap(t *testing.T, ctx context.Context, e *Env, value string) (restore func()) {
	t.Helper()
	var dep appsv1.Deployment
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-manager"}, &dep); err != nil {
		t.Fatal(err)
	}
	original := managerEnvValue(&dep, "MAX_QUEUED_CONVERSATIONS")
	restored := false
	restore = func() {
		if restored {
			return
		}
		restored = true
		patchManagerEnv(t, context.Background(), e, "MAX_QUEUED_CONVERSATIONS", original)
	}
	t.Cleanup(restore)
	patchManagerEnv(t, ctx, e, "MAX_QUEUED_CONVERSATIONS", value)
	return restore
}

// managerEnvValue reads one env var's current value off the manager
// container, "" if unset — the value setManagerQueueCap restores on
// cleanup.
func managerEnvValue(dep *appsv1.Deployment, name string) string {
	for ci := range dep.Spec.Template.Spec.Containers {
		c := &dep.Spec.Template.Spec.Containers[ci]
		if c.Name != "manager" {
			continue
		}
		for _, v := range c.Env {
			if v.Name == name {
				return v.Value
			}
		}
	}
	return ""
}

// patchManagerEnv sets one env var on the manager container to value and
// rolls the Deployment — restartManager's own annotation-bump-then-
// forceManagerRollout shape (correlation_test.go), with the env override
// folded into the SAME patch call so forceManagerRollout's own re-Get
// preserves both edits, exactly as it already preserves restartManager's
// annotation bump alone.
func patchManagerEnv(t *testing.T, ctx context.Context, e *Env, name, value string) {
	t.Helper()
	var dep appsv1.Deployment
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-manager"}, &dep); err != nil {
		t.Fatal(err)
	}
	n := int(derefOr(dep.Spec.Replicas, 1))
	patch := client.MergeFrom(dep.DeepCopy())
	for ci := range dep.Spec.Template.Spec.Containers {
		c := &dep.Spec.Template.Spec.Containers[ci]
		if c.Name != "manager" {
			continue
		}
		set := false
		for ei := range c.Env {
			if c.Env[ei].Name == name {
				c.Env[ei].Value = value
				set = true
				break
			}
		}
		if !set {
			c.Env = append(c.Env, corev1.EnvVar{Name: name, Value: value})
		}
	}
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations["agentops.dev/system-test-restarted-at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := e.K.Patch(ctx, &dep, patch); err != nil {
		t.Fatal(err)
	}
	forceManagerRollout(t, ctx, e, n)
}

// 6.1.c A signalId makes publication idempotent against the real API server:
// the same (source, signalId) posted twice stores ONE Signal, and the second
// answer still counts it queued.
func TestSignalPersistenceSignalIDDeduplicates(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	const name = "e2e-sigpersist-dedup"
	setupDedicatedSignalSource(t, ctx, e, name)

	stamp := fmt.Sprint(time.Now().UnixNano())
	id := "e2e-sigpersist-dedup-" + stamp
	sigID := "delivery-" + stamp
	for i := 0; i < 2; i++ {
		res := e.PostSignal(t, name, map[string]any{
			"initiationId": id, "signalId": sigID, "kind": "task", "payload": "once", "title": id,
		})
		if res["queued"] != float64(1) {
			t.Fatalf("post %d: a resend is still acknowledged as queued, got %v", i, res)
		}
	}
	waitSignalPhase(t, ctx, e, name, id, 2*time.Minute, agentopsv1alpha1.SignalRouted)
	items, err := e.K.Signals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for i := range items {
		if items[i].Spec.SourceRef.Name == name && items[i].Spec.SignalID == sigID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("one signalId must store exactly one Signal, got %d", n)
	}
	assertExactlyOneConversationPerClaimant(t, ctx, e, id, name)
}

// 8.1 (addressed) The console path: a person types `/<pipeline> <task>` in
// the console, which originates a chat Signal on the console source. The
// consumer takes the NAMED pipeline as the one claimant — no claim, so the
// pipeline's own source is NOT the console's — and the Signal reaches Routed
// naming the conversation it opened.
func TestSignalPersistenceAddressedConsoleCommandRoutesNamedPipeline(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	const name = "e2e-sigpersist-addr"
	setupDedicatedSignalSource(t, ctx, e, name)

	start := time.Now().Add(-5 * time.Second)
	t.Cleanup(func() { cleanupConsoleSignals(e, name) })

	stamp := fmt.Sprint(time.Now().UnixNano())
	var code int
	var out string
	waitFor(t, "the console to accept an addressed start", time.Minute, func() (bool, error) {
		code, out = e.ConsoleStart(t, "/"+name+" echo addr-"+stamp)
		return code/100 == 2, nil
	})
	if code/100 != 2 {
		t.Fatalf("addressed start: %d %s", code, out)
	}

	var sig *agentopsv1alpha1.Signal
	waitFor(t, "a console Signal addressed to "+name+" to reach Routed", 2*time.Minute, func() (bool, error) {
		var done bool
		var err error
		sig, done, err = addressedConsoleSignalRouted(ctx, e, name, start, sig)
		return done, err
	})
	assertClaimant(t, sig, name, agentopsv1alpha1.ClaimantRouted)
	c := claimantNamed(sig, name)
	if c.ConversationRef == nil || c.ConversationRef.Name == "" {
		t.Fatalf("the addressed claimant must name the conversation it opened: %+v", c)
	}
	conv, err := e.K.Conversation(ctx, c.ConversationRef.Name)
	if err != nil {
		t.Fatal(err)
	}
	if conv.Spec.PipelineRef == nil || conv.Spec.PipelineRef.Name != name {
		t.Fatalf("the addressed conversation must name pipeline %s, got %+v", name, conv.Spec.PipelineRef)
	}
}

// cleanupConsoleSignals deletes the console Signals claimed by name, and the
// conversations they opened.
func cleanupConsoleSignals(e *Env, name string) {
	items, err := e.K.Signals(context.Background())
	if err != nil {
		return
	}
	for i := range items {
		s := &items[i]
		c := claimantNamed(s, name)
		if s.Spec.SourceRef.Name != SourceConsole || c == nil {
			continue
		}
		if c.ConversationRef != nil {
			_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: c.ConversationRef.Name}})
		}
		_ = e.K.Delete(context.Background(), s)
	}
}

// addressedConsoleSignalRouted finds the console Signal addressed to name
// created after start and reports whether it reached Routed. With none found
// it returns prev unchanged.
func addressedConsoleSignalRouted(ctx context.Context, e *Env, name string, start time.Time, prev *agentopsv1alpha1.Signal) (*agentopsv1alpha1.Signal, bool, error) {
	items, err := e.K.Signals(ctx)
	if err != nil {
		return prev, false, err
	}
	for i := range items {
		s := &items[i]
		if s.Spec.SourceRef.Name == SourceConsole && s.CreationTimestamp.Time.After(start) && claimantNamed(s, name) != nil {
			return s, s.Status.Phase == agentopsv1alpha1.SignalRouted, nil
		}
	}
	return prev, false, nil
}

// claimantNamed returns sig's claimant for name, nil if absent.
func claimantNamed(sig *agentopsv1alpha1.Signal, name string) *agentopsv1alpha1.SignalClaimant {
	for i := range sig.Status.Claimants {
		if sig.Status.Claimants[i].Ref.Name == name {
			return &sig.Status.Claimants[i]
		}
	}
	return nil
}

// currentLeaderPod names the manager pod holding the leader Lease — the
// holder identity is "<pod>_<uuid>" (main.go's LeaderElectionID).
func currentLeaderPod(t *testing.T, ctx context.Context, e *Env) string {
	t.Helper()
	lease, err := e.K.Clientset.CoordinationV1().Leases(Namespace).Get(ctx, "agentops-manager.agentops.dev", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" {
		t.Fatalf("the leader Lease has no holder: %+v", lease.Spec)
	}
	return strings.SplitN(*lease.Spec.HolderIdentity, "_", 2)[0]
}

// 8.2 Leader loss mid-routing: a Signal left PartiallyRouted survives the
// LEADER POD being removed, and the next leader finishes it from the Signal
// CR alone, opening exactly one conversation per claimant. FULL TIER ONLY.
// The part-routed state is driven exactly as the restart lane above drives it
// (MAX_QUEUED_CONVERSATIONS=0 holds the claimant with no open conversation
// Routing). The leader is then force-deleted and the bound restored; the
// Signal must end Routed, consumed by a pod other than the one removed.
func TestSignalPersistenceLeaderRemovalFinishesPartiallyRouted(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	const p1 = "e2e-sigpersist-l1"
	const p2 = "e2e-sigpersist-l2"
	const soleSource = "e2e-sigpersist-lsole"
	const sharedSource = "e2e-sigpersist-lshared"
	id := "e2e-sigpersist-leader-" + stamp

	t.Cleanup(func() {
		items, err := e.K.Conversations(context.Background())
		if err != nil {
			return
		}
		for i := range items {
			if items[i].Spec.ConversationID == id {
				_ = e.K.Delete(context.Background(), &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: items[i].Name}})
			}
		}
	})

	mustCreate(t, e.K, source(soleSource, "e2e", nil))
	mustCreate(t, e.K, source(sharedSource, "e2e", nil))
	mustCreate(t, e.K, pipeline(p1, ProfileStub, []string{soleSource, sharedSource}, []string{ChannelConsole}))
	mustCreate(t, e.K, pipeline(p2, ProfileStub, []string{sharedSource}, []string{ChannelConsole}))
	for _, p := range []string{p1, p2} {
		if err := waitPipelineReady(ctx, e.K, p, 2*time.Minute); err != nil {
			t.Fatal(err)
		}
	}

	e.PostSignal(t, soleSource, map[string]any{"initiationId": id, "kind": "task", "payload": "p1 only", "title": id})
	e.ConversationFor(t, id, 2*time.Minute)

	restoreQueueCap := setManagerQueueCap(t, ctx, e, "0")
	e.PostSignal(t, sharedSource, map[string]any{"initiationId": id, "kind": "task", "payload": "shared", "title": id})
	sig := waitSignalPhase(t, ctx, e, sharedSource, id, 2*time.Minute, agentopsv1alpha1.SignalPartiallyRouted)
	assertClaimant(t, sig, p2, agentopsv1alpha1.ClaimantRouting)

	// Remove the leader while the Signal is part-routed.
	leader := currentLeaderPod(t, ctx, e)
	if err := e.K.Delete(ctx, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: leader}}, client.GracePeriodSeconds(0)); err != nil {
		t.Fatal(err)
	}
	restoreQueueCap()

	sig = waitSignalPhase(t, ctx, e, sharedSource, id, 3*time.Minute, agentopsv1alpha1.SignalRouted)
	assertClaimant(t, sig, p1, agentopsv1alpha1.ClaimantRouted)
	assertClaimant(t, sig, p2, agentopsv1alpha1.ClaimantRouted)
	if sig.Status.ConsumedBy == leader {
		t.Fatalf("the Signal must be finished by a successor, but ConsumedBy is the removed leader %s", leader)
	}
	assertExactlyOneConversationPerClaimant(t, ctx, e, id, p1, p2)
}

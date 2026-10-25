//go:build system

package system

// Section 6 (signal-persistence) — the persist-then-ack lane end to end
// against a REAL /signal/inbound HTTP endpoint, a REAL owned Signal CR and
// REAL Conversation CRs, plus the one event only a live cluster adds: a
// genuine manager restart resuming a Signal the inline attempt left
// non-terminal, with neither claimant's conversation duplicated. The pure
// claimant-state-machine and roll-up MATH (signalroute.RouteClaimants,
// agentopsv1alpha1.RollupPhase) is pinned at envtest
// (internal/controller/signal_controller_test.go) — nothing here re-derives
// that logic. What only a real cluster adds is a real OwnerReference set by
// the real API server, a real informer field index serving the
// absorb-by-initiationId read, and a real rolled Deployment recovering from
// nothing but the Signal CRs themselves (design D-3's "crash recovery is
// therefore free").

import (
	"context"
	"fmt"
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
// before the manager ever answers, routes inline on the first attempt, and
// reaches Routed naming the real conversation it opened.
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
// WHY THIS DRIVES PartiallyRouted THE WAY IT DOES: signalroute.RouteClaimants
// attempts every Routing claimant inside ONE Status().Patch call, so there
// is no window an HTTP caller can observe between two claimants of the SAME
// inline attempt finishing at different times — by the time
// /signal/inbound answers, the Signal's status already reflects every
// claimant's end state for that one call. The only per-claimant branch
// RouteClaimants makes is the pending-backlog check ahead of a CREATE
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

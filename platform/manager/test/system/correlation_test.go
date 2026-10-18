//go:build system

package system

// Section 9 — the sender-minted correlation id, end to end through the
// REAL /signal/inbound HTTP endpoint, a REAL Pipeline claim and REAL
// Conversation CRs. The pure id-equality routing MATH is already pinned at
// envtest (platform/manager/internal/integration/correlation_test.go's four
// scenarios: open-match recurrence, fresh-id creation, distinct-ids-never-
// merge, closed-conversation exclusion) — nothing here re-derives that
// logic. What only a live cluster adds is the PLUMBING envtest's in-process
// handler and fake networking cannot stand in for: a real webhook POST
// reaching a real claimed source, a real informer field index serving the
// routing read, and a real pod restart's recovery of the durable cooldown
// record (envtest fakes that recovery with a second in-process
// *httpapi.Server; here it is an actual rolled Deployment).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// conversationsFromSource lists every conversation this source originated —
// the system pack's own equivalent of the envtest suite's
// convsFromPipeline, read off the live cluster instead of a fake client.
func conversationsFromSource(ctx context.Context, e *Env, sourceName string) ([]agentopsv1alpha1.Conversation, error) {
	items, err := e.K.Conversations(ctx)
	if err != nil {
		return nil, err
	}
	var out []agentopsv1alpha1.Conversation
	for _, c := range items {
		if c.Spec.Signal != nil && c.Spec.Signal.SourceRef != nil && c.Spec.Signal.SourceRef.Name == sourceName {
			out = append(out, c)
		}
	}
	return out, nil
}

// setupDedicatedCorrelationSource wires one fresh SignalSource to one fresh
// Pipeline, isolated from the shared wiring and from every other test's
// source — a shared source's cooldown cache or open conversation would
// otherwise leak between runs of this lane and any other.
func setupDedicatedCorrelationSource(t *testing.T, ctx context.Context, e *Env, name string) *agentopsv1alpha1.SignalSource {
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

// 9.1.a Two signals with distinct initiationIds, posted to one real source
// through one real claiming Pipeline, open two separate conversations —
// pure equality routing, proven over the actual HTTP endpoint rather than
// envtest's in-process handler.
func TestCorrelationDistinctIDsOpenSeparateConversations(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	setupDedicatedCorrelationSource(t, ctx, e, "e2e-corr-distinct")

	stamp := fmt.Sprint(time.Now().UnixNano())
	idA := "e2e-corr-distinct-a-" + stamp
	idB := "e2e-corr-distinct-b-" + stamp
	e.PostSignal(t, "e2e-corr-distinct", map[string]any{"initiationId": idA, "kind": "task", "payload": "first", "title": idA})
	e.PostSignal(t, "e2e-corr-distinct", map[string]any{"initiationId": idB, "kind": "task", "payload": "second", "title": idB})

	convA := e.ConversationFor(t, idA, 2*time.Minute)
	convB := e.ConversationFor(t, idB, 2*time.Minute)
	if convA.Name == convB.Name {
		t.Fatalf("two distinct initiationIds must never share one conversation, both resolved to %s", convA.Name)
	}
	if convA.Spec.ConversationID != idA {
		t.Fatalf("a fresh id must be adopted verbatim as conversationId, got %q want %q", convA.Spec.ConversationID, idA)
	}
	if convB.Spec.ConversationID != idB {
		t.Fatalf("a fresh id must be adopted verbatim as conversationId, got %q want %q", convB.Spec.ConversationID, idB)
	}
	all, err := conversationsFromSource(ctx, e, "e2e-corr-distinct")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("exactly two conversations must exist for this source, got %d", len(all))
	}
}

// 9.1.b A signal with NO initiationId is rejected 4xx at the real endpoint,
// naming the field — a loud contract violation rather than a synthesized
// id or a silently merged conversation. The check runs before the source is
// even resolved, so an ordinary already-wired source is enough; nothing
// about this rejection is source-specific.
func TestCorrelationMissingInitiationIDIsRejected(t *testing.T) {
	e := requireEnv(t)
	body, _ := json.Marshal(map[string]any{
		"source":  SourceTasks,
		"signals": []any{map[string]any{"kind": "task", "payload": "no id given", "title": "e2e-corr-no-id"}},
	})
	code, out := e.do(t, "POST", e.Manager.URL()+"/signal/inbound", body, bearerPrefix+e.Values.AdapterToken)
	if code < 400 || code >= 500 {
		t.Fatalf("a signal with no initiationId must be rejected 4xx, got %d: %s", code, out)
	}
	if !strings.Contains(out, "initiationId") {
		t.Fatalf("the rejection must name the missing field, got: %s", out)
	}
}

// 9.1.c A thread binding carries the conversationId end to end: a signal
// opens a conversation bound to two real channel threads (console and the
// Telegram lane, through the fake Bot API), and the SAME conversation
// object — identified by the conversationId it adopted from the posted
// initiationId — is the one that owns both bindings. The wire-level
// conversationId field on each /channel/ops operation is pinned at the
// dispatch/chat unit tier (task 3.5); what only a live cluster adds is that
// a real signal POST really drives a real channel adapter into really
// creating a real thread for that same object.
func TestCorrelationIDReachesThreadBinding(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	stamp := fmt.Sprint(time.Now().UnixNano())
	id := "e2e-corr-bind-" + stamp
	e.PostTask(t, SourceFanout, id, "echo correlation-bind "+stamp)

	conv := e.ConversationFor(t, id, 2*time.Minute)
	if conv.Spec.ConversationID != id {
		t.Fatalf("the opened conversation must adopt the posted initiationId verbatim, got %q want %q", conv.Spec.ConversationID, id)
	}
	waitFor(t, "two thread bindings on the conversation carrying this id", 2*time.Minute, func() (bool, error) {
		c, err := e.K.Conversation(ctx, conv.Name)
		if err != nil {
			return false, err
		}
		return c.Spec.ConversationID == id && len(c.Status.Threads) == 2, nil
	})
	waitFor(t, "a forum topic opened for this signal-originated conversation", 2*time.Minute, func() (bool, error) {
		return len(e.BotCalls(t, "createForumTopic")) > 0, nil
	})
}

// 9.1.d A repeat of the SAME initiationId, posted OUTSIDE the source's
// cooldown window, recurs into the still-open conversation rather than
// opening a second one. The manager's cooldown map lives in memory for the
// whole life of the process (internal/httpapi/signals.go's
// Server.cooldowns) — nothing short of a real restart ever re-reads the
// durable record on the SignalSource, so "outside the window" is proven
// here by backdating that record and then actually rolling the manager
// Deployment, the one real-cluster event envtest's own correlation_test.go
// stands in for with a second in-process *httpapi.Server. FULL TIER: a
// manager restart is heavier than this pack's smoke budget affords for one
// assertion, and SYSTEM_TIER=full is what 9.2 runs.
func TestCorrelationRepeatOutsideCooldownRecursOnOpenConversation(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	const name = "e2e-corr-cooldown"
	setupDedicatedCorrelationSource(t, ctx, e, name)

	stamp := fmt.Sprint(time.Now().UnixNano())
	id := "e2e-corr-cooldown-" + stamp
	e.PostSignal(t, name, map[string]any{"initiationId": id, "kind": "task", "payload": "first", "title": id})
	conv := e.ConversationFor(t, id, 2*time.Minute)
	conv = e.WaitRun(t, conv.Name, 1, 5*time.Minute)
	if conv.Status.Runs[0].Status != "succeeded" {
		t.Fatalf("the first run must succeed before the cooldown half of this test means anything: %+v", conv.Status.Runs[0])
	}

	backdateSourceCooldown(t, ctx, e, name, id)
	restartManager(t, ctx, e)

	e.PostSignal(t, name, map[string]any{"initiationId": id, "kind": "task", "payload": "second", "title": id})
	e.WaitRun(t, conv.Name, 2, 5*time.Minute)

	all, err := conversationsFromSource(ctx, e, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("a repeat outside the cooldown window must recur into the open conversation, not open a second one, got %d conversations", len(all))
	}
}

// backdateSourceCooldown rewrites a SignalSource's own recorded cooldown
// entry for one initiationId to look EXPIRED — mirrored off
// internal/integration/suite_test.go's backdateCooldown, which fakes the
// SAME recovery-from-status path with a swapped-out in-process Server
// rather than a real pod boundary.
func backdateSourceCooldown(t *testing.T, ctx context.Context, e *Env, sourceName, initiationID string) {
	t.Helper()
	var src agentopsv1alpha1.SignalSource
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: e.K.Namespace, Name: sourceName}, &src); err != nil {
		t.Fatal(err)
	}
	hours := src.Spec.CooldownHours
	if hours <= 0 {
		hours = 6
	}
	patch := client.MergeFrom(src.DeepCopy())
	found := false
	for i := range src.Status.Cooldown {
		if src.Status.Cooldown[i].InitiationID == initiationID {
			src.Status.Cooldown[i].At = metav1.NewTime(time.Now().Add(-time.Duration(hours+1) * time.Hour))
			found = true
		}
	}
	if !found {
		t.Fatalf("the source must already have recorded a cooldown entry for %s before it can be backdated", initiationID)
	}
	if err := e.K.Status().Patch(ctx, &src, patch); err != nil {
		t.Fatal(err)
	}
}

// restartManager rolls the manager Deployment and waits for the rollout to
// finish — the only way its in-memory cooldown map is ever cleared, exactly
// as a real upgrade or node drain would.
//
// A plain `kubectl rollout restart` deadlocks here: the chart's own default
// is replicas: 2 with HARD pod anti-affinity (replicas_test.go), which this
// pack's single-node cluster can only ever schedule one of — the shared
// install already runs with one pod Pending for every other lane. A rollout
// restart's new ReplicaSet wants two MORE pods on the one node already
// holding both of the old ReplicaSet's, and nothing unblocks that on its
// own (measured live: the rollout sat at "1 out of 2 new replicas updated"
// and every later lane needing the manager then failed too). This reuses
// replicas_test.go's own forceManagerRollout, which clears the stale
// affinity and force-deletes the pods blocking the new template — the
// exact mechanics already proven for scaleManagerReplicas, at the SAME
// replica count so nothing here changes how many pods the rest of the pack
// sees. The harness's port-forwards already survive a backing pod's
// replacement (cluster.go's Forward runs a supervisor goroutine for exactly
// this), so nothing here needs to re-dial.
//
// forceManagerRollout only produces a NEW ReplicaSet when the pod template
// actually changes — true the FIRST time anything clears the affinity
// field, but a no-op once it is already nil, which replicas_test.go itself
// documents as permanent for the rest of a run ("once cleared, left
// cleared"). On SYSTEM_REUSE=1 (this pack's own documented local-dev
// workflow) or any run ordering where a replicas lane already executed,
// that leaves nothing for forceManagerRollout to roll: no new pods, no
// cleared cooldown map, and this test's whole premise unexercised. Bumping
// a template annotation first forces a genuine diff every time, independent
// of affinity state, before forceManagerRollout's scheduling-safe mechanics
// take over.
func restartManager(t *testing.T, ctx context.Context, e *Env) {
	t.Helper()
	var dep appsv1.Deployment
	if err := e.K.Get(ctx, types.NamespacedName{Namespace: Namespace, Name: "agentops-manager"}, &dep); err != nil {
		t.Fatal(err)
	}
	n := int(derefOr(dep.Spec.Replicas, 1))

	patch := client.MergeFrom(dep.DeepCopy())
	if dep.Spec.Template.Annotations == nil {
		dep.Spec.Template.Annotations = map[string]string{}
	}
	dep.Spec.Template.Annotations["agentops.dev/system-test-restarted-at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := e.K.Patch(ctx, &dep, patch); err != nil {
		t.Fatal(err)
	}

	forceManagerRollout(t, ctx, e, n)
}

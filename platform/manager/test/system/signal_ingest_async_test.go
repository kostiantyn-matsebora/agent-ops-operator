//go:build system

package system

// async-signal-ingest tasks.md 4.1-4.2 — the real-cluster lanes for
// /signal/inbound's accept-then-process split (proposal.md/design.md): the
// real HTTP endpoint, the real "in.signal" command-bus consumer
// (HandleIngestSignalTask, internal/httpapi/ingesttask.go) and a real
// Signal CR, observed through the SAME task-stream/`/state/tasks` surface
// section 9's own end-to-end flows (async_e2e_test.go) and section 11's own
// command-bus lanes (async_system_test.go) already use — every helper this
// file needs (openTaskStream, awaitTaskUIDSucceeded, waitTaskState,
// waitTaskSettled, deadLetters, streamFrames, postInboundDuringRestart,
// restartManagerSchedulable, acceptedWire, taskStateWire) is reused from
// there, and setupDedicatedSignalSource/waitSignalPhase from
// signal_persistence_test.go, never redefined.
//
// Task 4.1's own ask ("run against THIS working copy's chart and images,
// not the default branch's") is, as async_system_test.go's own doc comment
// already records, a PACK-WIDE guarantee TestMain gives every lane in this
// package — nothing below re-asserts it.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// firstTaskID extracts the one taskId a POST /signal/inbound accept response
// (design D-1/D-3) names, failing the test when it is not exactly one.
func firstTaskID(t *testing.T, res map[string]any) string {
	t.Helper()
	ids, _ := res["taskIds"].([]any)
	if len(ids) != 1 {
		t.Fatalf("want exactly one taskId, got %v", res["taskIds"])
	}
	uid, _ := ids[0].(string)
	if uid == "" {
		t.Fatalf("taskIds[0] is not a string: %v", res["taskIds"])
	}
	return uid
}

// ---- 4.1.a -------------------------------------------------------------

// 4.1.a The happy path: a signal posted through the real /signal/inbound
// endpoint is accepted (202, queued: 1), its own ingest-signal task appears
// on the real task stream and reaches succeeded, and the Signal that task
// created is there to find once it has.
//
// signal-persistence's own happy-path lane
// (TestSignalPersistenceReachesRoutedPhase, signal_persistence_test.go)
// already proves the Signal's eventual Routed phase and ownership — it
// never opens the task stream, so it cannot show that accept actually
// produced a durable, visible task. That is what this lane adds.
func TestSignalIngestTaskReachesSucceededAndSignalAppears(t *testing.T) {
	e := requireEnv(t)
	ctx := context.Background()
	const name = "e2e-sigingest-happy"
	src := setupDedicatedSignalSource(t, ctx, e, name)
	auth := bearerPrefix + e.Values.AdapterToken

	// Opened before the post, as every lane watching the task stream does
	// (async_system_test.go's 11.6): a resumed stream starts from "now", so a
	// task enqueued before it opened would never be seen.
	resp := openTaskStream(t, ctx, e, auth)
	defer resp.Body.Close()
	frames := streamFrames(resp)

	stamp := fmt.Sprint(time.Now().UnixNano())
	id := "e2e-sigingest-happy-" + stamp
	res := e.PostSignal(t, name, map[string]any{"initiationId": id, "kind": "task", "payload": "first", "title": id})
	if res["queued"] != float64(1) {
		t.Fatalf("an admitted signal must be queued (accepted, design D-4), got %v", res)
	}
	uid := firstTaskID(t, res)

	awaitTaskUIDSucceeded(t, frames, uid, 2*time.Minute)

	sig := waitSignalPhase(t, ctx, e, name, id, 2*time.Minute, agentopsv1alpha1.SignalRouted)
	if sig.Spec.SourceRef.Name != src.Name {
		t.Fatalf("the Signal the in.signal consumer created must name its SignalSource %q, got %q", src.Name, sig.Spec.SourceRef.Name)
	}
}

// ---- 4.1.b -------------------------------------------------------------

// 4.1.b A manager restart between accept and consume loses nothing: the
// signal is accepted (202) while the manager is mid-restart, and once the
// dust settles the SAME ingest-signal task still reaches a terminal state
// (waitTaskSettled's own succeeded-or-already-gone-past-its-grace) and the
// Signal it names still appears, routed.
//
// Mirrors TestAsyncManagerRestartAndBusServerRestartLoseNoAcceptedTask's own
// manager-half mechanics (async_system_test.go's assertManagerRestartLoses
// NoAcceptedTask: restartManagerSchedulable racing postInboundDuringRestart)
// for the ingest-signal task instead of the chat `/close` task that lane
// already covers for the "conv" queue — postInboundDuringRestart takes a raw
// URL and body, tied to neither surface, so it is reused verbatim against
// POST /signal/inbound.
func TestSignalIngestTaskSurvivesManagerRestartBetweenAcceptAndConsume(t *testing.T) {
	fullTier(t)
	e := requireEnv(t)
	ctx := context.Background()
	const name = "e2e-sigingest-restart"
	setupDedicatedSignalSource(t, ctx, e, name)
	auth := bearerPrefix + e.Values.AdapterToken

	stamp := fmt.Sprint(time.Now().UnixNano())
	id := "e2e-sigingest-restart-" + stamp
	body, _ := json.Marshal(map[string]any{"source": name, "signals": []any{
		map[string]any{"initiationId": id, "kind": "task", "payload": "restart", "title": id},
	}})

	var code int
	var accepted acceptedWire
	var postErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		code, accepted, postErr = postInboundDuringRestart(ctx, e.Manager.URL()+"/signal/inbound", auth, body, 6*time.Minute)
	}()
	if err := restartManagerSchedulable(ctx, e.Cluster); err != nil {
		t.Fatalf("restart the manager: %v", err)
	}
	wg.Wait()
	if postErr != nil || code != 202 || len(accepted.TaskIDs) != 1 {
		t.Fatalf("accepting the signal across the restart: code=%d err=%v accepted=%+v", code, postErr, accepted)
	}

	waitTaskSettled(t, e, auth, accepted.TaskIDs[0], 7*time.Minute)
	waitSignalPhase(t, ctx, e, name, id, 2*time.Minute, agentopsv1alpha1.SignalRouted)
}

// ---- 4.2 -----------------------------------------------------------------

// 4.2 Backoff into a dead letter against a surface that really fails — for
// in.signal/HandleIngestSignalTask, mirroring TestAsyncBackoffIntoADeadLetter
// AgainstARealFailure's own shape (async_system_test.go, for the
// out.<channel> consumer) for the OTHER durable consumer this change adds.
//
// What only a real cluster decides here, mirroring that lane's own choice of
// a failure the manager cannot distinguish from a genuine outage: nothing in
// HandleIngestSignalTask's own synchronous call path makes an outbound
// network call at all (ingesttask.go: answerCommands only ever enqueues,
// through chat.Router.HandleCommand -> Ops.EnqueueMessage, never dials out)
// — the one real external dependency its own write touches is the state
// store itself (design D-1: "it trades one durable record... for another"),
// through publishOne's s.Signals.Create. The real-cluster-only, utterly
// deterministic way to fail THAT create for a signal that already cleared
// accept — so the adapter's 202 never moves — without disturbing ANY other
// resource this pack's other lanes depend on (conversations, the command
// bus's own queue entries) is a ResourceQuota the admission controller
// enforces for real: `count/signals.agentops.dev: "0"` refuses every create
// of exactly this one kind.
//
// Corrupting the manager's own NATS_USER/NATS_PASSWORD (as 11.2's own
// assertCommandBusCredentialProjected reads them) would have been the wrong
// real failure to reach for here: at this pack's own chart default the
// state store and the command bus share ONE bundle and ONE credential
// Secret (async_system_test.go's own busCredentialSecret), so breaking the
// state store's connection that way breaks the command bus's own
// retry/backoff/dead-letter MECHANISM too, rather than exercising it against
// a real failure.
//
// publishOne's own default branch never calls consumer.WithCode — it
// returns a plain wrapped error, which the pool's codeFor (commandbus/
// consumer/pool.go) resolves through commandbus.ParseErrorCode on the
// error's own TEXT, which never matches the closed code vocabulary and
// normalizes to CodeInternal (retryable) whatever the real underlying
// reason is — so a quota-refused create backs off exactly as a transient
// one would, genuinely exercising the bus's own schedule (5 attempts, 1s to
// 30s with jitter) before the task goes dead, never failing straight to a
// terminal code the way the SignalSource lookup's own NotFound branch does.
//
// KUBE BACKEND ONLY, requireKube: under the chart's own default (nats) a
// Signal is never realized as a Kubernetes object at all, so a
// Signal-scoped ResourceQuota has nothing to enforce against. Unlike
// correlation_test.go's own kube/nats sibling pair
// (TestCorrelationRepeatOutsideCooldownRecursOnOpenConversation /
// TestCorrelationCooldownWindowSurvivesAManagerRestart), this property has
// no nats-backend sibling in this file — see this change's own test report
// for why, recorded as a real, named gap rather than silently left
// unexercised: the bus's own backoff-into-dead-letter MECHANICS are already
// proven backend-independent by the command-bus conformance suite (task
// 2.4/2.5) and, for this exact pack's own nats-backed install, by the
// structurally identical queue kind (a lease-and-retry queue, out.tg-ops)
// TestAsyncBackoffIntoADeadLetterAgainstARealFailure already exercises end
// to end. What is uniquely new here is HandleIngestSignalTask's OWN failure
// surface, which only the kube backend's Signal-as-object shape lets this
// pack reach without disabling the retry mechanism itself.
func TestAsyncSignalIngestBackoffIntoADeadLetterAgainstARealFailure(t *testing.T) {
	fullTier(t)
	requireKube(t)
	e := requireEnv(t)
	ctx := context.Background()
	const name = "e2e-sigingest-quota"
	setupDedicatedSignalSource(t, ctx, e, name)
	auth := bearerPrefix + e.Values.AdapterToken

	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{Namespace: Namespace, Name: "e2e-signal-ingest-deny"},
		Spec: corev1.ResourceQuotaSpec{
			Hard: corev1.ResourceList{"count/signals.agentops.dev": resource.MustParse("0")},
		},
	}
	t.Cleanup(func() {
		if err := e.K.Delete(context.Background(), quota); err != nil && !apierrors.IsNotFound(err) {
			t.Errorf("lifting the signal-create quota: %v", err)
		}
	})
	if err := e.K.Create(ctx, quota); err != nil {
		t.Fatalf("denying every Signal create: %v", err)
	}

	stamp := fmt.Sprint(time.Now().UnixNano())
	id := "e2e-sigingest-quota-" + stamp
	res := e.PostSignal(t, name, map[string]any{"initiationId": id, "kind": "task", "payload": "quota-denied", "title": id})
	if res["queued"] != float64(1) {
		t.Fatalf("accept must still succeed with the signal queued — the failure is downstream, never a retried-request 5xx: %v", res)
	}
	uid := firstTaskID(t, res)

	// Handed over: the task is claimed and is retrying against the genuinely
	// refused create.
	waitTaskState(t, e, auth, uid, "running", 2*time.Minute)

	// The real dead-letter record: the bus's own bounded retry is spent
	// against a failure the quota makes genuinely permanent.
	waitTaskState(t, e, auth, uid, "dead", 3*time.Minute)
	dead := deadLetters(t, e, auth, "in.signal")
	if !slices.ContainsFunc(dead, func(d taskStateWire) bool { return d.UID == uid }) {
		t.Fatalf("the dead task %s must be listed on GET /state/dead-letters?queue=in.signal", uid)
	}
}

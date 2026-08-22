package integration

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/httpapi"
)

// The answer an agent produced is durably written to status.runs[].result, but
// the op that DELIVERS it lived only in an in-memory queue. A manager restart in
// that window dropped the answer forever — visible in `kubectl get conversation
// -o yaml` and delivered to nobody. These tests are about that window.
//
// A "restart" here is a FRESH httpapi.Server: same Kubernetes objects, empty
// OpQueue. That is exactly what the new process sees, and it is why the CR has
// to carry the delivery fact — after a restart the queue's own dedup window is
// empty and cannot tell a delivered reply from an owed one.

// deliveryConv creates a conversation bound to the named channels, each already
// carrying a thread (a reply has nowhere to land without one).
func deliveryConv(t *testing.T, name, profile string, channels ...string) *agentopsv1alpha1.Conversation {
	t.Helper()
	ctx := context.Background()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: profile}
	for _, ch := range channels {
		conv.Spec.ChannelRefs = append(conv.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: ch})
	}
	if err := k8sClient.Create(ctx, conv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupConversation(t, conv.Name) })
	patch := client.MergeFrom(conv.DeepCopy())
	for _, ch := range channels {
		conv.Status.Threads = append(conv.Status.Threads, agentopsv1alpha1.ThreadBinding{
			Channel: ch, ThreadID: "t-" + ch,
		})
	}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}
	return conv
}

// answersFor returns the agent answers among a batch of ops.
func answersFor(ops []chat.Op) []chat.Op {
	var out []chat.Op
	for _, op := range ops {
		if op.Kind == chat.OpSend && op.Message != nil && op.Message.Kind == chat.MsgAnswer {
			out = append(out, op)
		}
	}
	return out
}

func loadConv(t *testing.T, name string) *agentopsv1alpha1.Conversation {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatal(err)
	}
	return &conv
}

func runNamed(t *testing.T, conv *agentopsv1alpha1.Conversation, runID string) *agentopsv1alpha1.RunStatus {
	t.Helper()
	for i := range conv.Status.Runs {
		if conv.Status.Runs[i].RunID == runID {
			return &conv.Status.Runs[i]
		}
	}
	t.Fatalf("run %q is not recorded on %s", runID, conv.Name)
	return nil
}

// completeOps finishes ops the caller already claimed, as a healthy adapter would.
func completeOps(t *testing.T, srv *httpapi.Server, ops []chat.Op) {
	t.Helper()
	for _, op := range ops {
		rec := adapterReq(srv, "POST", "/channel/ops/"+op.ID+"/done", chat.OpResult{}, testMasterToken)
		if rec.Code != 200 {
			t.Fatalf("complete %s: %d %s", op.ID, rec.Code, rec.Body.String())
		}
	}
}

// The core gap this change closes: the result was recorded, the op was never
// claimed, the process died. Reconciliation must re-derive the reply.
func TestRestartBetweenCompletionAndDeliveryReDerivesTheReply(t *testing.T) {
	mkProfile(t, "prof-deliver")
	mkChannel(t, "chan-deliver", "tg-deliver")
	conv := deliveryConv(t, "deliver-conv", "prof-deliver", "chan-deliver")

	before := apiServer()
	rec := adapterReq(before, "POST", "/work/done", map[string]any{
		"convo": conv.Name, "runId": "r-lost", "status": "succeeded", "result": "scaled `api` to 3",
	}, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}
	// The op exists in the dying process's queue and no adapter ever claims it.
	if got := len(answersFor(drainOps(t, before, "tg-deliver"))); got != 1 {
		t.Fatalf("fast path should have enqueued one answer, got %d", got)
	}
	if runNamed(t, loadConv(t, conv.Name), "r-lost").DeliveredTo("chan-deliver") {
		t.Fatal("an unclaimed op must not be recorded as delivered")
	}

	after := apiServer() // restart: same objects, empty queue
	reconcileWithOps(t, after, conv.Name)

	answers := answersFor(drainOps(t, after, "tg-deliver"))
	if len(answers) != 1 {
		t.Fatalf("want the reply re-derived exactly once, got %d", len(answers))
	}
	if body := answers[0].Message.Body; body != "scaled `api` to 3" {
		t.Fatalf("re-derived reply must say what the run said, got %q", body)
	}
	if id := answers[0].ID; id != chat.RunReplyOpID(conv.Name, "chan-deliver", "r-lost") {
		t.Fatalf("reply op id must be stable per conversation×channel×run, got %q", id)
	}

	// Completing it records the fact, and reconciliation stops owing it.
	completeOps(t, after, answers)
	if !runNamed(t, loadConv(t, conv.Name), "r-lost").DeliveredTo("chan-deliver") {
		t.Fatal("completing a reply op must mark the run delivered on that channel")
	}
	reconcileWithOps(t, after, conv.Name)
	if got := len(answersFor(drainOps(t, after, "tg-deliver"))); got != 0 {
		t.Fatalf("a delivered run must enqueue nothing, got %d", got)
	}
}

// Fan-out can be interrupted partway. The threads that already have the answer
// must not receive it again, and the ones that do not must still get it.
func TestPartiallyDeliveredFanOutCompletesWithoutDuplicating(t *testing.T) {
	mkProfile(t, "prof-partial")
	mkChannel(t, "chan-part-a", "tg-part-a")
	mkChannel(t, "chan-part-b", "tg-part-b")
	mkChannel(t, "chan-part-c", "tg-part-c")
	conv := deliveryConv(t, "partial-conv", "prof-partial", "chan-part-a", "chan-part-b", "chan-part-c")

	before := apiServer()
	if rec := adapterReq(before, "POST", "/work/done", map[string]any{
		"convo": conv.Name, "runId": "r-part", "status": "succeeded", "result": "done",
	}, testMasterToken); rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}
	// Only channel A's adapter got there before the process died.
	completeOps(t, before, answersFor(drainOps(t, before, "tg-part-a")))

	after := apiServer()
	reconcileWithOps(t, after, conv.Name)

	if got := len(answersFor(drainOps(t, after, "tg-part-a"))); got != 0 {
		t.Fatalf("the delivered thread must receive nothing further, got %d", got)
	}
	for _, adapter := range []string{"tg-part-b", "tg-part-c"} {
		if got := len(answersFor(drainOps(t, after, adapter))); got != 1 {
			t.Fatalf("%s was owed the reply, got %d", adapter, got)
		}
	}
}

// The migration hazard: on first observation after an upgrade, completed runs
// carry no delivery markers because the field did not exist. Enqueueing for them
// would re-post every recent answer to every bound thread.
func TestUpgradeBackfillsDeliveryWithoutRePosting(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-backfill")
	mkChannel(t, "chan-backfill", "tg-backfill")
	conv := deliveryConv(t, "backfill-conv", "prof-backfill", "chan-backfill")

	// A run as an OLDER manager wrote it: result recorded, delivery untracked.
	patch := client.MergeFrom(conv.DeepCopy())
	now := metav1.Now()
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{
		RunID: "r-old", Status: "succeeded", Result: "answered before the upgrade", FinishedAt: &now,
	}}
	if err := k8sClient.Status().Patch(ctx, conv, patch); err != nil {
		t.Fatal(err)
	}

	srv := apiServer()
	reconcileWithOps(t, srv, conv.Name)

	if got := len(answersFor(drainOps(t, srv, "tg-backfill"))); got != 0 {
		t.Fatalf("upgrading must re-post nothing, got %d answer(s)", got)
	}
	run := runNamed(t, loadConv(t, conv.Name), "r-old")
	if !run.DeliveryTracked || !run.DeliveredTo("chan-backfill") {
		t.Fatalf("a pre-upgrade run must be recorded delivered, got %+v", run)
	}

	// And the backfill is idempotent — a second pass neither sends nor grows it.
	reconcileWithOps(t, srv, conv.Name)
	if got := len(answersFor(drainOps(t, srv, "tg-backfill"))); got != 0 {
		t.Fatalf("second pass must stay silent, got %d", got)
	}
	if got := len(runNamed(t, loadConv(t, conv.Name), "r-old").Delivered); got != 1 {
		t.Fatalf("markers must not accumulate, got %d", got)
	}
}

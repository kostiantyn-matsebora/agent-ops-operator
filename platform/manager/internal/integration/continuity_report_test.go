package integration

import (
	"context"
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
)

// Losing context is the one failure that leaves a conversation looking entirely
// healthy — same phase, same run history, answers still arriving — while every
// answer is given without memory. So it is a condition, like every other health
// fact about a conversation, and the REASON is whatever the runtime said: the
// manager does not know where a given runtime keeps context and must not guess.

func reportContinuity(t *testing.T, srv *httpapi.Server, convo, runID, status, contextID, continuity, reason string) {
	t.Helper()
	body := map[string]any{
		"convo": convo, "runId": runID, "status": status, "result": "ok",
		"runtimeContextId": contextID, "continuity": continuity,
	}
	if reason != "" {
		body["continuityReason"] = reason
	}
	rec := adapterReq(srv, "POST", "/work/done", body, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}
}

func continuityCondition(t *testing.T, name string) *metav1.Condition {
	t.Helper()
	c := loadConversation(t, name)
	return apimeta.FindStatusCondition(c.Status.Conditions, controller.ConditionContextContinuity)
}

func TestUnavailableContextIsRecordedWithTheRuntimesOwnReason(t *testing.T) {
	mkProfile(t, "prof-cont-lost")
	conv := contextConv(t, "cont-lost", "prof-cont-lost")
	srv := apiServer()

	reportContinuity(t, srv, conv.Name, "r-1", "failed", "ctx-new",
		"unavailable", "no session files under /data/context/.claude/projects")

	cond := continuityCondition(t, conv.Name)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("a lost context must be recorded on the conversation, got %+v", cond)
	}
	// Verbatim: the manager adds no diagnosis of its own, because it does not
	// know whether this runtime keeps context on a volume or at a vendor API.
	if cond.Message != "no session files under /data/context/.claude/projects" {
		t.Fatalf("the runtime's reason must be recorded verbatim, got %q", cond.Message)
	}
	// The handle it established is still adopted — continuing a partial context
	// beats starting over.
	if got := loadConversation(t, conv.Name).ContextID(); got != "ctx-new" {
		t.Fatalf("handle from a failed run = %q", got)
	}
}

func TestASuccessfulContinuationClearsTheCondition(t *testing.T) {
	mkProfile(t, "prof-cont-heal")
	conv := contextConv(t, "cont-heal", "prof-cont-heal")
	srv := apiServer()

	reportContinuity(t, srv, conv.Name, "r-1", "failed", "ctx-a", "unavailable", "gone")
	if c := continuityCondition(t, conv.Name); c == nil || c.Status != metav1.ConditionFalse {
		t.Fatalf("precondition: %+v", c)
	}

	reportContinuity(t, srv, conv.Name, "r-2", "succeeded", "ctx-a", "continued", "")

	// The condition describes the PRESENT, not a history of every hiccup.
	if c := continuityCondition(t, conv.Name); c == nil || c.Status != metav1.ConditionTrue {
		t.Fatalf("a successful continuation must clear the condition, got %+v", c)
	}
}

// An addition to the contract must not make an existing runtime look broken.
func TestARuntimeMakingNoClaimSetsNoCondition(t *testing.T) {
	mkProfile(t, "prof-cont-silent")
	conv := contextConv(t, "cont-silent", "prof-cont-silent")
	srv := apiServer()

	// No `continuity` field at all — an older runtime image.
	reportRun(t, srv, conv.Name, "r-1", "ctx-quiet")

	if c := continuityCondition(t, conv.Name); c != nil {
		t.Fatalf("absent means NO CLAIM, not a loss: %+v", c)
	}
	if got := loadConversation(t, conv.Name).ContextID(); got != "ctx-quiet" {
		t.Fatalf("handle still recorded = %q", got)
	}
}

// Failing with nothing to read is why this path used to answer without context
// instead. The reason must reach the person waiting on the thread.
func TestAFailedRunsReasonReachesTheThread(t *testing.T) {
	mkProfile(t, "prof-cont-thread")
	mkChannel(t, "chan-cont", "tg-cont")
	conv := deliveryConv(t, "cont-thread", "prof-cont-thread", "chan-cont")
	srv := apiServer()

	rec := adapterReq(srv, "POST", "/work/done", map[string]any{
		"convo": conv.Name, "runId": "r-1", "status": "failed",
		"result":     "⚠️ **This conversation cannot be continued.** Its stored context is no longer available.",
		"continuity": "unavailable", "continuityReason": "no session files",
	}, testMasterToken)
	if rec.Code != 200 {
		t.Fatalf("work done: %d %s", rec.Code, rec.Body.String())
	}

	ops := drainOps(t, srv, "tg-cont")
	var msg *chat.Message
	for i := range ops {
		if ops[i].Kind == chat.OpSend && ops[i].Message != nil {
			msg = ops[i].Message
		}
	}
	if msg == nil {
		t.Fatal("a failed run must still reach the bound thread")
	}
	if msg.Kind != chat.MsgNotice || msg.Level != chat.NoticeWarn {
		t.Fatalf("a failure is a warning notice, got kind=%s level=%s", msg.Kind, msg.Level)
	}
	// The bare "run failed (failed)" is what made the old fallback look kind.
	if msg.Body == "❌ run failed (failed)" || msg.Body == "" {
		t.Fatalf("the reason was discarded: %q", msg.Body)
	}
	if want := "cannot be continued"; !contains(msg.Body, want) {
		t.Fatalf("thread message %q does not carry the explanation", msg.Body)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// An OUTAGE holds work; it does not destroy it. Under fail-fast alone a
// two-minute storage incident would report unavailability for every active
// conversation and permanently kill all of them, irreversibly, in the time it
// takes each to receive one message.
func TestAnOutageHoldsWorkRatherThanConsumingIt(t *testing.T) {
	mkProfile(t, "prof-outage")
	srv := apiServer()

	// Enough conversations reporting unavailability inside the window that the
	// evidence points at the infrastructure rather than at any one of them.
	var last string
	for i := 0; i < 4; i++ {
		name := "outage-" + string(rune('a'+i))
		conv := contextConv(t, name, "prof-outage")
		last = conv.Name
		// give the run an input to consume, and mark it inflight
		markInflight(t, conv.Name, "in-1", "r-1")
		reportContinuity(t, srv, conv.Name, "r-1", "failed", "", "unavailable", "store down")
	}

	// The LAST one landed after the breaker opened: its input must still be
	// pending, so the message is answered once contexts return.
	c := loadConversation(t, last)
	for _, id := range c.Status.ProcessedInputIDs {
		if id == "in-1" {
			t.Fatal("an outage consumed the message instead of holding it")
		}
	}
	cond := continuityCondition(t, last)
	if cond == nil || cond.Reason != "ContextStoreUnavailable" {
		t.Fatalf("an outage must be reported as such, got %+v", cond)
	}
}

// markInflight gives a conversation a pending input and marks it dispatched, so
// a completion report has something to consume or hold.
func markInflight(t *testing.T, name, inputID, runID string) {
	t.Helper()
	ctx := context.Background()
	c := loadConversation(t, name)
	patch := client.MergeFrom(c.DeepCopy())
	c.Spec.Inputs = append(c.Spec.Inputs, agentopsv1alpha1.InputItem{
		ID: inputID, Type: agentopsv1alpha1.InputTask, Payload: "hold me",
	})
	if err := k8sClient.Patch(ctx, c, patch); err != nil {
		t.Fatal(err)
	}
	c = loadConversation(t, name)
	sp := client.MergeFrom(c.DeepCopy())
	c.Status.Inflight = &agentopsv1alpha1.InflightRun{
		RunID: runID, InputIDs: []string{inputID}, DispatchedAt: metav1.Now(),
	}
	if err := k8sClient.Status().Patch(ctx, c, sp); err != nil {
		t.Fatal(err)
	}
}

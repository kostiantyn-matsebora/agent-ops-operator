// The way OUT of an unrecoverable context loss.
//
// Before this verb a conversation whose context store was destroyed could only
// fail every subsequent run — a promised context that cannot be reached FAILS
// rather than silently starting fresh — or be deleted, throwing away its
// threads and its whole recorded history for an unrelated reason.
package integration

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

func setContextHandle(t *testing.T, name, handle string) {
	t.Helper()
	var conv agentopsv1alpha1.Conversation
	key := types.NamespacedName{Namespace: ns, Name: name}
	if err := k8sClient.Get(context.Background(), key, &conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.RuntimeContextID = handle
	if err := k8sClient.Status().Patch(context.Background(), &conv, patch); err != nil {
		t.Fatal(err)
	}
}

func resetContext(t *testing.T, name, channel string) *httptest.ResponseRecorder {
	t.Helper()
	srv := apiServer()
	req := httptest.NewRequest("POST", "/channel/conversations/"+name+"/reset-context",
		strings.NewReader(`{"channel":"`+channel+`"}`))
	req.SetPathValue("name", name)
	rec := httptest.NewRecorder()
	srv.ResetContextForTest(rec, req)
	return rec
}

// The conversation survives the reset entirely — only its memory goes.
func TestResetClearsTheHandleAndKeepsEverythingElse(t *testing.T) {
	mkProfile(t, "p-reset")
	mkChannel(t, "ch-reset", "telegram")
	mkChanConv(t, "reset-1", "p-reset", "ch-reset")
	setContextHandle(t, "reset-1", "session-abc")

	rec := resetContext(t, "reset-1", "ch-reset")
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}

	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "reset-1"}, &conv); err != nil {
		t.Fatalf("the conversation must survive its reset: %v", err)
	}
	if conv.ContextID() != "" {
		t.Fatalf("context handle = %q, want cleared", conv.ContextID())
	}
	if len(conv.Spec.Inputs) == 0 {
		t.Fatal("inputs must survive: the reset removes memory, not work")
	}
	// And it must SAY it lost its memory — a conversation that silently
	// resumed without one is the failure this verb exists to make impossible.
	cond := apimeta.FindStatusCondition(conv.Status.Conditions, controller.ConditionContextContinuity)
	if cond == nil || cond.Status != metav1.ConditionFalse {
		t.Fatalf("the loss must be recorded on the conversation; got %+v", cond)
	}
	if !strings.Contains(strings.ToLower(cond.Message), "reset") {
		t.Fatalf("condition message = %q, want it to name the reset", cond.Message)
	}
}

// The retired spelling is cleared too: leaving it would let the dual-read that
// exists for upgrades resurrect the handle the reset just removed.
func TestResetClearsTheLegacyHandleToo(t *testing.T) {
	mkProfile(t, "p-reset2")
	mkChannel(t, "ch-reset2", "telegram")
	mkChanConv(t, "reset-2", "p-reset2", "ch-reset2")

	var conv agentopsv1alpha1.Conversation
	key := types.NamespacedName{Namespace: ns, Name: "reset-2"}
	if err := k8sClient.Get(context.Background(), key, &conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.SessionID = "legacy-session" // written by an older manager
	if err := k8sClient.Status().Patch(context.Background(), &conv, patch); err != nil {
		t.Fatal(err)
	}

	if rec := resetContext(t, "reset-2", "ch-reset2"); rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var after agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), key, &after); err != nil {
		t.Fatal(err)
	}
	if after.ContextID() != "" {
		t.Fatalf("legacy handle survived the reset: %q", after.ContextID())
	}
}

// Resetting a conversation that has no handle is a no-op success: the caller
// wanted a conversation with no stale handle and that is what they have.
func TestResetWithNoHandleIsANoOp(t *testing.T) {
	mkProfile(t, "p-reset3")
	mkChannel(t, "ch-reset3", "telegram")
	mkChanConv(t, "reset-3", "p-reset3", "ch-reset3")
	rec := resetContext(t, "reset-3", "ch-reset3")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"cleared":false`) {
		t.Fatalf("body = %s, want cleared:false", rec.Body.String())
	}
}

// THE guard. An automatic reset would be indistinguishable from the silent
// degradation the continuity rules exist to forbid: an agent quietly answering
// without its memory, with nobody able to tell it had lost one.
//
// So a failed continuation must NEVER clear the handle by itself. Only the
// explicit operator verb does, and only when someone chose it.
func TestAFailedContinuationNeverResetsByItself(t *testing.T) {
	mkProfile(t, "p-noauto")
	mkChannel(t, "ch-noauto", "telegram")
	mkChanConv(t, "noauto-1", "p-noauto", "ch-noauto")
	setContextHandle(t, "noauto-1", "session-keepme")

	// A run reports that it could not reach its context — the exact event an
	// automatic reset would hang itself on.
	var conv agentopsv1alpha1.Conversation
	key := types.NamespacedName{Namespace: ns, Name: "noauto-1"}
	if err := k8sClient.Get(context.Background(), key, &conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	apimeta.SetStatusCondition(&conv.Status.Conditions, metav1.Condition{
		Type: controller.ConditionContextContinuity, Status: metav1.ConditionFalse,
		Reason: "Unavailable", Message: "context store unreachable",
	})
	if err := k8sClient.Status().Patch(context.Background(), &conv, patch); err != nil {
		t.Fatal(err)
	}

	r := reconcilerWithCap(nil, 100)
	for i := 0; i < 3; i++ {
		reconcileWith(t, r, "noauto-1")
	}

	var after agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(), key, &after); err != nil {
		t.Fatal(err)
	}
	if after.ContextID() != "session-keepme" {
		t.Fatalf("the handle was cleared without an operator asking: %q — "+
			"an automatic reset is the silent degradation the rules forbid", after.ContextID())
	}
}

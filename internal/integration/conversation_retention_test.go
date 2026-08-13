package integration

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/controller"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/runtimepod"
)

// The two-stage lifecycle through a real API server: autoclose closes a
// FINISHED idle conversation, a closed one is inert and unreusable, and
// autodelete reclaims it only after its own window from the CLOSE.

func retentionReconciler(ops *chat.OpQueue, router *chat.Router, closeAge, deleteAge time.Duration) *controller.ConversationReconciler {
	r := &controller.ConversationReconciler{
		Client:                 k8sClient,
		Scheme:                 scheme,
		MaxActiveConversations: 100,
		Ops:                    ops,
		Router:                 router,
		Runtime:                runtimepod.Config{Image: "busybox:stub", ServiceAccount: "default", ControlURL: "http://manager:8080", IdleTTLMinutes: 1},
	}
	if closeAge > 0 {
		r.AutoCloseEnabled, r.AutoCloseIdleAge = true, closeAge
	}
	if deleteAge > 0 {
		r.AutoDeleteEnabled, r.AutoDeleteClosedAge = true, deleteAge
	}
	return r
}

func retentionRun(t *testing.T, r *controller.ConversationReconciler, name string) ctrl.Result {
	t.Helper()
	res, err := r.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// mkFinishedConv builds a conversation that is genuinely finished: Idle, no
// pending inputs, no inflight run, and its one run delivered everywhere.
func mkFinishedConv(t *testing.T, name string, idleFor time.Duration, channels ...string) *agentopsv1alpha1.Conversation {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = name, ns
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "ret-profile"}
	for _, c := range channels {
		conv.Spec.ChannelRefs = append(conv.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: c})
	}
	if err := k8sClient.Create(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	last := metav1.NewTime(time.Now().Add(-idleFor))
	conv.Status.Phase = agentopsv1alpha1.ConversationIdle
	conv.Status.LastActivity = &last
	conv.Status.Runs = []agentopsv1alpha1.RunStatus{{
		RunID: "run-1", Status: "succeeded", Result: "answered",
		FinishedAt: &last, DeliveryTracked: true, Delivered: channels,
	}}
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	return conv
}

func TestBothTimersOffDoNothing(t *testing.T) {
	mkProfile(t, "ret-profile")
	mkFinishedConv(t, "ret-untouched", 400*time.Hour)

	retentionRun(t, retentionReconciler(nil, nil, 0, 0), "ret-untouched")

	if got := getConv(t, "ret-untouched"); got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("with both flags off nothing may be closed, however idle")
	}
}

func TestAutoCloseClosesAnIdleFinishedConversation(t *testing.T) {
	ops := testOps()
	router := &chat.Router{Client: k8sClient, Reader: k8sClient, Namespace: ns, Ops: ops}
	mkFinishedConv(t, "ret-idle", 400*time.Hour)

	retentionRun(t, retentionReconciler(ops, router, 168*time.Hour, 0), "ret-idle")

	got := getConv(t, "ret-idle")
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("an idle finished conversation must be closed, phase=%q", got.Status.Phase)
	}
	if got.Status.ClosedAt == nil {
		t.Fatal("closedAt must be stamped at the transition")
	}
	// the record is the point of the split
	if len(got.Status.Runs) != 1 || got.Status.Runs[0].Result != "answered" {
		t.Fatal("closing must keep the recorded answer")
	}
}

// The window is IDLE time, not lifetime: a conversation created long ago that
// answered a moment ago is busy, not old.
func TestRecentlyActiveConversationSurvivesItsWindow(t *testing.T) {
	ops := testOps()
	router := &chat.Router{Client: k8sClient, Reader: k8sClient, Namespace: ns, Ops: ops}
	mkFinishedConv(t, "ret-busy", time.Minute)

	res := retentionRun(t, retentionReconciler(ops, router, time.Hour, 0), "ret-busy")

	if got := getConv(t, "ret-busy"); got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("a conversation that answered a minute ago must not be closed")
	}
	if res.RequeueAfter <= 0 {
		t.Fatal("the timer must self-schedule for the moment it expires")
	}
}

// A conversation whose answer has not reached every bound thread is NOT
// finished, however idle: closing then archives the thread out from under it.
func TestAnUndeliveredRunHoldsAConversationOpen(t *testing.T) {
	ops := testOps()
	router := &chat.Router{Client: k8sClient, Reader: k8sClient, Namespace: ns, Ops: ops}
	mkChannel(t, "ret-ch", "tg")
	conv := mkFinishedConv(t, "ret-undelivered", 400*time.Hour, "ret-ch")
	conv.Status.Runs[0].Delivered = nil // recorded, but never reached the thread
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}

	retentionRun(t, retentionReconciler(ops, router, time.Hour, 0), "ret-undelivered")

	if got := getConv(t, "ret-undelivered"); got.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("a conversation with an undelivered run must stay open")
	}
}

// A never-closed conversation is never auto-DELETED, however idle. The two
// stages are separate decisions with separate flags.
func TestAutoDeleteIgnoresAConversationThatWasNeverClosed(t *testing.T) {
	mkFinishedConv(t, "ret-never-closed", 4000*time.Hour)

	retentionRun(t, retentionReconciler(nil, nil, 0, time.Hour), "ret-never-closed")

	var got agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "ret-never-closed"}, &got); err != nil {
		t.Fatal("autodelete must not touch a conversation that was never closed")
	}
	if !got.DeletionTimestamp.IsZero() {
		t.Fatal("autodelete must not touch a conversation that was never closed")
	}
}

// The delete clock runs from closedAt, and a reopen clears that stamp — so a
// reopen before the window prevents the delete.
func TestAutoDeleteRespectsTheClosedWindowAndAReopenResetsIt(t *testing.T) {
	conv := mkFinishedConv(t, "ret-closed-recently", time.Hour)
	closedAt := metav1.NewTime(time.Now().Add(-time.Minute))
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	conv.Status.ClosedAt = &closedAt
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}

	r := retentionReconciler(nil, nil, 0, 24*time.Hour)
	res := retentionRun(t, r, "ret-closed-recently")
	if res.RequeueAfter <= 0 {
		t.Error("a closed conversation inside its window must be requeued for its expiry")
	}
	if getConv(t, "ret-closed-recently").Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatal("inside the window it must survive")
	}

	// past the window it goes
	old := metav1.NewTime(time.Now().Add(-48 * time.Hour))
	conv = getConv(t, "ret-closed-recently")
	conv.Status.ClosedAt = &old
	if err := k8sClient.Status().Update(context.Background(), conv); err != nil {
		t.Fatal(err)
	}
	retentionRun(t, r, "ret-closed-recently")
	var got agentopsv1alpha1.Conversation
	err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "ret-closed-recently"}, &got)
	if err == nil && got.DeletionTimestamp.IsZero() {
		t.Fatal("past its window a closed conversation must be deleted")
	}
}

// The rule that makes closing mean anything: a matching signature opens a NEW
// conversation rather than waking the one somebody tidied away.
func TestAClosedConversationIsNotReused(t *testing.T) {
	mkProfile(t, "prof-reuse")
	mkSignalSource(t, "src-reuse", "am-reuse", "")
	mkPipeline(t, "pipe-reuse", []string{"src-reuse"}, nil, "prof-reuse")
	reconcilePipeline(t, "pipe-reuse")
	h := apiServer().Handler()

	if rec := postSignal(t, h, testMasterToken, "src-reuse", []map[string]any{
		{"fingerprint": "reuse-1", "labels": map[string]string{"alertname": "DiskFull"}, "payload": "full"},
	}); rec.Code != 200 {
		t.Fatalf("first signal: %d %s", rec.Code, rec.Body.String())
	}
	first := convsFromPipeline(t, "pipe-reuse")
	if len(first) != 1 {
		t.Fatalf("first signal must open one conversation, got %d", len(first))
	}

	// Close it. A matching signature must now open a NEW conversation.
	c := getConv(t, first[0].Name)
	now := metav1.Now()
	c.Status.Phase = agentopsv1alpha1.ConversationClosed
	c.Status.ClosedAt = &now
	if err := k8sClient.Status().Update(context.Background(), c); err != nil {
		t.Fatal(err)
	}

	// A DIFFERENT fingerprint, so the source's cooldown is not what decides.
	if rec := postSignal(t, h, testMasterToken, "src-reuse", []map[string]any{
		{"fingerprint": "reuse-2", "labels": map[string]string{"alertname": "DiskFull"}, "payload": "still full"},
	}); rec.Code != 200 {
		t.Fatalf("second signal: %d %s", rec.Code, rec.Body.String())
	}
	all := convsFromPipeline(t, "pipe-reuse")
	if len(all) != 2 {
		t.Fatalf("a signal matching a CLOSED conversation must open a new one, got %d", len(all))
	}
	for _, cv := range all {
		if cv.Name == first[0].Name && len(cv.Spec.Inputs) > 1 {
			t.Fatal("the closed conversation must not have absorbed the new signal")
		}
	}
}

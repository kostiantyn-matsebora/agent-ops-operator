package httpapi

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
)

// workReportServerAt is workReportServer with a CALLER-CHOSEN dispatch stamp,
// set clearly in the past rather than effectively "now" — so a test reading
// it back has a real gap to assert against, rather than two timestamps a few
// microseconds apart that a wrong implementation could satisfy by accident.
func workReportServerAt(t *testing.T, dispatchedAt metav1.Time) *Server {
	t.Helper()
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "conv-a", "agent-ops"
	conv.Spec.RuntimeRef = &agentopsv1alpha1.ObjectRef{Name: "claude"}
	conv.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "r1", DispatchedAt: dispatchedAt}
	rt := &agentopsv1alpha1.AgentRuntime{}
	rt.Name, rt.Namespace = "claude", "agent-ops"
	rt.Spec.Image = "example/runtime-claude:1"

	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
		WithObjects(conv, rt).WithStatusSubresource(conv).Build()
	return &Server{Reader: c, Client: c, Namespace: "agent-ops", Activity: activity.New(64)}
}

// A recorded run must carry when it actually STARTED, not only when it
// finished. The console anchors a coordinator member's transcript position to
// its invoking run's own window (item #15 QA: a member is created mid-run,
// well before the run's own reasoning is recorded), and that anchor is
// meaningless without a real start time — before this, StartedAt was declared
// on the CRD but never written anywhere, so every run reported it empty.
//
// The dispatch stamp is pinned five minutes in the past rather than "now": a
// regression that set StartedAt to the run's FINISH time instead of its
// dispatch stamp would still satisfy a mere "FinishedAt not before StartedAt"
// check (the two would simply be equal), and workReportServer's own "now"
// stamp leaves too small a gap for that distinction to be reliable either.
// Exact equality against the dispatch stamp is what actually pins the fix.
func TestWorkDoneRecordsTheRunsTrueStartTime(t *testing.T) {
	// Truncated to the second: metav1.Time itself round-trips at only that
	// granularity (through the fake client's own status-subresource store,
	// matching the real API server), so comparing against a value that still
	// carries a sub-second component would fail on account of THAT, not on
	// account of anything this test is meant to pin.
	dispatchedAt := metav1.NewTime(time.Now().Add(-5 * time.Minute).Truncate(time.Second))
	s := workReportServerAt(t, dispatchedAt)

	rec := postWorkDone(t, s, map[string]any{
		"convo": "conv-a", "runId": "r1", "status": "succeeded", "result": "done",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}

	var conv agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, &conv); err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if len(conv.Status.Runs) != 1 {
		t.Fatalf("want exactly one recorded run, got %d", len(conv.Status.Runs))
	}
	run := conv.Status.Runs[0]
	if run.StartedAt == nil {
		t.Fatalf("StartedAt must be set from the run's own dispatch stamp, got nil: %+v", run)
	}
	if !run.StartedAt.Time.Equal(dispatchedAt.Time) {
		t.Fatalf("StartedAt must equal the run's own dispatch stamp exactly, got %v want %v",
			run.StartedAt.Time, dispatchedAt.Time)
	}
	if run.FinishedAt == nil {
		t.Fatalf("FinishedAt must still be set: %+v", run)
	}
	if !run.FinishedAt.Time.After(dispatchedAt.Time) {
		t.Fatalf("FinishedAt (%v) must be strictly after the dispatch stamp (%v)", run.FinishedAt.Time, dispatchedAt.Time)
	}
}

package httpapi

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// A recorded run must carry when it actually STARTED, not only when it
// finished. The console anchors a coordinator member's transcript position to
// its invoking run's own window (item #15 QA: a member is created mid-run,
// well before the run's own reasoning is recorded), and that anchor is
// meaningless without a real start time — before this, StartedAt was declared
// on the CRD but never written anywhere, so every run reported it empty.
func TestWorkDoneRecordsTheRunsTrueStartTime(t *testing.T) {
	s, _ := workReportServer(t)

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
	if run.FinishedAt == nil {
		t.Fatalf("FinishedAt must still be set: %+v", run)
	}
	if run.FinishedAt.Time.Before(run.StartedAt.Time) {
		t.Fatalf("StartedAt (%v) must not be after FinishedAt (%v)", run.StartedAt.Time, run.FinishedAt.Time)
	}
}

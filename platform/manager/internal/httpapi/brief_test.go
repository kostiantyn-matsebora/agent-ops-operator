package httpapi

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// The `brief` contract field (design D-I, coordinated-agents task 2.14):
// LATEST-WINS, same rule as RuntimeContextID, never derived from Result.

func TestWorkDoneRecordsTheReportedBrief(t *testing.T) {
	s, _ := workReportServer(t)
	rec := postWorkDone(t, s, map[string]any{
		"convo": "conv-a", "runId": "r1", "status": "succeeded", "result": "done",
		"brief": "the api pod restart loop",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Brief != "the api pod restart loop" {
		t.Fatalf("brief: %q", got.Status.Brief)
	}
}

func TestWorkDoneOmittingBriefLeavesTheStoredOneUntouched(t *testing.T) {
	s, _ := workReportServer(t)
	var conv agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, &conv); err != nil {
		t.Fatal(err)
	}
	conv.Status.Brief = "already on record"
	if err := s.Client.Status().Update(context.Background(), &conv); err != nil {
		t.Fatal(err)
	}

	rec := postWorkDone(t, s, map[string]any{
		"convo": "conv-a", "runId": "r1", "status": "succeeded", "result": "done",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Brief != "already on record" {
		t.Fatalf("an absent report must leave the stored brief alone, got %q", got.Status.Brief)
	}
}

func TestWorkDoneBoundsAnOverlongBriefToMaxBrief(t *testing.T) {
	s, _ := workReportServer(t)
	long := strings.Repeat("x", agentopsv1alpha1.MaxBrief+100)
	rec := postWorkDone(t, s, map[string]any{
		"convo": "conv-a", "runId": "r1", "status": "succeeded", "result": "done", "brief": long,
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Status.Brief) != agentopsv1alpha1.MaxBrief {
		t.Fatalf("brief length: %d", len(got.Status.Brief))
	}
}

func TestWorkDoneRecordsBriefOnAFailedRunToo(t *testing.T) {
	s, _ := workReportServer(t)
	rec := postWorkDone(t, s, map[string]any{
		"convo": "conv-a", "runId": "r1", "status": "failed", "result": "could not finish",
		"brief": "the api pod restart loop",
	})
	if rec.Code != 200 {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got agentopsv1alpha1.Conversation
	if err := s.Reader.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-a"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Brief != "the api pod restart loop" {
		t.Fatalf("a crash after the fact must not strand the brief either: %q", got.Status.Brief)
	}
}

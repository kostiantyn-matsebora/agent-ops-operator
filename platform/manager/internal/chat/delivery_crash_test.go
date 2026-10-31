package chat

import (
	"context"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// markDelivered is two writes (the run's delivery marker, then the thread's
// undelivered-reply note) where it used to be one patch. A crash between them
// leaves a run recorded delivered with the note still naming it. The delivery
// pass must tolerate that: no second send, the note settled.
func TestDeliveryPassToleratesACrashBetweenTheRunMarkerAndTheNote(t *testing.T) {
	ctx := context.Background()
	conv := testConv("conv-1")
	conv.Namespace = testNS
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "c1"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "t1", UndeliveredReply: "run-1"}}
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv, nsChannel("c1", "slack")).Build()
	store := newTestStore(c)
	q := &OpQueue{Store: store, Registry: NewRegistry()}
	r := &Router{Client: c, Reader: c, Store: store, Namespace: testNS, Ops: q}

	if err := store.AppendRun(ctx, "conv-1", state.Run{RunID: "run-1", Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	// The first write lands; the process dies before the second.
	if err := store.UpdateRun(ctx, "conv-1", "run-1", state.RunUpdate{MarkDelivered: []string{"c1"}}); err != nil {
		t.Fatal(err)
	}

	r.FanOutRunReply(ctx, recOf(t, store, "conv-1"), "run-1", AnswerMessage("a", "s"))

	if ops := drain(q, "slack"); len(ops) != 0 {
		t.Fatalf("a reply recorded delivered must not be sent again, got %+v", ops)
	}
	after := recOf(t, store, "conv-1")
	if b := after.Thread("c1"); b == nil || b.UndeliveredReply != "" {
		t.Fatalf("the delivery pass must settle the note naming a delivered reply, got %+v", b)
	}
	runs, err := store.ListRuns(ctx, "conv-1")
	if err != nil || len(runs) != 1 || !runs[0].DeliveredTo("c1") {
		t.Fatalf("the run must stay recorded delivered: %+v, %v", runs, err)
	}
}

// The mirror image: the run marker write never landed. The reply is still owed
// and the pass re-derives it, so a crash before either write loses nothing.
func TestDeliveryPassResendsAReplyWhoseRunMarkerNeverLanded(t *testing.T) {
	ctx := context.Background()
	conv := testConv("conv-1")
	conv.Namespace = testNS
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "c1"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "c1", ThreadID: "t1", UndeliveredReply: "run-1"}}
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(conv, nsChannel("c1", "slack")).Build()
	store := newTestStore(c)
	q := &OpQueue{Store: store, Registry: NewRegistry()}
	r := &Router{Client: c, Reader: c, Store: store, Namespace: testNS, Ops: q}
	if err := store.AppendRun(ctx, "conv-1", state.Run{RunID: "run-1", Status: "ok"}); err != nil {
		t.Fatal(err)
	}

	r.FanOutRunReply(ctx, recOf(t, store, "conv-1"), "run-1", AnswerMessage("a", "s"))

	if ops := drain(q, "slack"); len(ops) != 1 {
		t.Fatalf("an undelivered reply must be re-derived, got %+v", ops)
	}
	if b := recOf(t, store, "conv-1").Thread("c1"); b == nil || b.UndeliveredReply != "run-1" {
		t.Fatalf("the note must stay until delivery completes, got %+v", b)
	}
}

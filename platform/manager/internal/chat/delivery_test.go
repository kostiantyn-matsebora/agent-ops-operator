package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// DeliverInputs no longer fences an escalation channel on Status.EscalatedAt
// (coordinator-unconditional-channels, superseding design D-D's fence): a
// Coordinator root's channel is bound at CREATION, so its thread already
// exists before any `escalate` call, and nothing queued before that call is
// a "backlog" any more — both a pre- and a post-escalation input reach it,
// exactly as any other bound channel's would.
func TestDeliverInputsNeverFencesOnEscalatedAt(t *testing.T) {
	escalatedAt := metav1.NewTime(time.Now())
	before := metav1.NewTime(escalatedAt.Add(-time.Minute))
	after := metav1.NewTime(escalatedAt.Add(time.Minute))

	conv := testConv("co-root")
	conv.Namespace = testNS
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "esc-desk"}}
	conv.Spec.EscalationChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "esc-desk"}}
	conv.Status.EscalatedAt = &escalatedAt
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "esc-desk", ThreadID: "esc-thread-1"}}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{
		{
			ID: "pre", Type: agentopsv1alpha1.InputTask, Payload: "queued before escalation", ReceivedAt: before,
			Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: "member-1", Entry: "worker"},
		},
		{
			ID: "post", Type: agentopsv1alpha1.InputTask, Payload: "arrived after escalation", ReceivedAt: after,
			Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: "member-2", Entry: "worker"},
		},
	}

	ch := nsChannel("esc-desk", "esc-ta")
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).WithObjects(ch).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	DeliverInputs(context.Background(), c, q, conv)

	ops := drain(q, "esc-ta")
	if len(ops) != 2 {
		t.Fatalf("want BOTH inputs delivered, got %d ops: %+v", len(ops), ops)
	}
	bodies := opBody(ops[0]) + opBody(ops[1])
	if !strings.Contains(bodies, "queued before escalation") || !strings.Contains(bodies, "arrived after escalation") {
		t.Fatalf("both inputs must reach the already-bound thread, got %q", bodies)
	}
}

// An ordinary channel (one never named in EscalationChannelRefs) was never
// fenced even under the old design, and still is not — delivery never reads
// EscalatedAt at all any more, so this stays green on the same grounds the
// test above now does.
func TestDeliverInputsNeverFencesAChannelThatIsNotAnEscalationChannel(t *testing.T) {
	escalatedAt := metav1.NewTime(time.Now())
	before := metav1.NewTime(escalatedAt.Add(-time.Minute))

	conv := testConv("plain-conv")
	conv.Namespace = testNS
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "ordinary"}}
	conv.Status.EscalatedAt = &escalatedAt // set defensively; must not matter here
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "ordinary", ThreadID: "t1"}}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{
		{
			ID: "pre", Type: agentopsv1alpha1.InputTask, Payload: "an ordinary signal", ReceivedAt: before,
			Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginSignal, Name: "src"},
		},
	}

	ch := nsChannel("ordinary", "some-ta")
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).WithObjects(ch).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	DeliverInputs(context.Background(), c, q, conv)

	ops := drain(q, "some-ta")
	if len(ops) != 1 {
		t.Fatalf("an unrelated channel must never be fenced, got %d ops", len(ops))
	}
}

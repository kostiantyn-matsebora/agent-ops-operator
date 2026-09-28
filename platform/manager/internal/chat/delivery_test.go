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

// DeliverInputs fences an escalation channel on Status.EscalatedAt (design
// D-D, coordinated-agents task 3.2): `escalate` binds the channel precisely so
// the digest can be the thread's first post, so anything still queued from
// before that moment must not retroactively flood it.
func TestDeliverInputsFencesAnEscalationChannelOnEscalatedAt(t *testing.T) {
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
	if len(ops) != 1 {
		t.Fatalf("want exactly the post-escalation input delivered, got %d ops: %+v", len(ops), ops)
	}
	if got := opBody(ops[0]); !strings.Contains(got, "arrived after escalation") || strings.Contains(got, "queued before escalation") {
		t.Fatalf("delivered the wrong input: %q", got)
	}
}

// A channel bound OUTSIDE escalation (an ordinary Pipeline-driven conversation)
// carries no such moment and is never fenced, whatever EscalatedAt says —
// fencing is scoped to EscalationChannelRefs, not to "any escalated conversation".
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

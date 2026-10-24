package chat

import (
	"context"
	"encoding/json"
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

// A member's result reaching its parent (coordination-loop) is delivered to
// every bound channel exactly like any other input — coordinator-
// unconditional-channels established that, and TestDeliverInputsNeverFencesOnEscalatedAt
// above already pins it for the escalation fence specifically. What this test
// pins is the ADDITIONAL fact item #15 QA's bug 2 needed: the composed signal
// card carries its OriginKind, which is what lets ONE adapter — the console,
// which ALSO builds a richer invoke-card view of the same exchange — choose
// not to render a second, generic copy of it, without changing what any other
// adapter (which has no such view) receives or shows.
func TestDeliverInputsCarriesOriginKindOnTheComposedSignal(t *testing.T) {
	conv := testConv("coord-root")
	conv.Namespace = testNS
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "console"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "console", ThreadID: "t1"}}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{
		{
			ID: "member:member-xw4h5:r0", Type: agentopsv1alpha1.InputTask, Payload: "the ha report",
			ReceivedAt: metav1.Now(),
			Origin:     &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginMember, Name: "member-xw4h5", Entry: "ha-ops"},
		},
	}

	ch := nsChannel("console", "console-ta")
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).WithObjects(ch).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	DeliverInputs(context.Background(), c, q, conv)

	ops := drain(q, "console-ta")
	if len(ops) != 1 {
		t.Fatalf("want the member-result still delivered, got %d ops: %+v", len(ops), ops)
	}
	if ops[0].Message == nil || ops[0].Message.OriginKind != agentopsv1alpha1.OriginMember {
		t.Fatalf("want OriginKind = %q on the composed card, got %+v", agentopsv1alpha1.OriginMember, ops[0].Message)
	}
}

// Severity rides to the composed signal card exactly where Labels already
// does — read off the input's out-of-line ConversationInput — and is a
// structured field, not folded into the body or the labels map
// (signal-severity scenario "Severity rides with the card as a field").
func TestDeliverInputsCarriesSeverityFromTheConversationInput(t *testing.T) {
	conv := testConv("sev-conv")
	conv.Namespace = testNS
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "console"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "console", ThreadID: "t1"}}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{
		{
			ID: "in-1", Type: agentopsv1alpha1.InputTask, ReceivedAt: metav1.Now(),
			PayloadRef: &agentopsv1alpha1.ObjectRef{Name: "sev-conv-in-1"},
			Origin:     &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginSignal, Name: "vm-alerts"},
		},
	}

	ci := &agentopsv1alpha1.ConversationInput{}
	ci.Name, ci.Namespace = "sev-conv-in-1", testNS
	ci.Spec = agentopsv1alpha1.ConversationInputSpec{
		ConversationRef: agentopsv1alpha1.ObjectRef{Name: "sev-conv"},
		Type:            agentopsv1alpha1.InputTask,
		Payload:         "disk at 97%",
		Severity:        agentopsv1alpha1.SeverityWarning,
	}

	ch := nsChannel("console", "console-ta")
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).WithObjects(ch, ci).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	DeliverInputs(context.Background(), c, q, conv)

	ops := drain(q, "console-ta")
	if len(ops) != 1 || ops[0].Message == nil {
		t.Fatalf("want one card delivered, got %d ops: %+v", len(ops), ops)
	}
	if ops[0].Message.Severity != agentopsv1alpha1.SeverityWarning {
		t.Fatalf("want severity %q on the composed card, got %+v", agentopsv1alpha1.SeverityWarning, ops[0].Message)
	}
	raw, err := json.Marshal(ops[0].Message)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"severity":"warning"`) {
		t.Fatalf("a rated signal must serialize severity as a field, got %s", raw)
	}
}

// An unrated signal — the ordinary case today, since no shipped adapter sets
// the field yet — omits severity entirely from the wire message rather than
// serializing it as `""` (signal-severity scenario "An unrated signal
// carries no severity field").
func TestDeliverInputsOmitsSeverityWhenTheInputCarriesNone(t *testing.T) {
	conv := testConv("unrated-conv")
	conv.Namespace = testNS
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "console"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "console", ThreadID: "t1"}}
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{
		{
			ID: "in-1", Type: agentopsv1alpha1.InputTask, ReceivedAt: metav1.Now(),
			PayloadRef: &agentopsv1alpha1.ObjectRef{Name: "unrated-conv-in-1"},
			Origin:     &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginSignal, Name: "vm-alerts"},
		},
	}

	ci := &agentopsv1alpha1.ConversationInput{}
	ci.Name, ci.Namespace = "unrated-conv-in-1", testNS
	ci.Spec = agentopsv1alpha1.ConversationInputSpec{
		ConversationRef: agentopsv1alpha1.ObjectRef{Name: "unrated-conv"},
		Type:            agentopsv1alpha1.InputTask,
		Payload:         "disk at 97%",
		// Severity left at its zero value — unrated, today's only state.
	}

	ch := nsChannel("console", "console-ta")
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).WithObjects(ch, ci).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}

	DeliverInputs(context.Background(), c, q, conv)

	ops := drain(q, "console-ta")
	if len(ops) != 1 || ops[0].Message == nil {
		t.Fatalf("want one card delivered, got %d ops: %+v", len(ops), ops)
	}
	if ops[0].Message.Severity != "" {
		t.Fatalf("want no severity on an unrated card, got %+v", ops[0].Message)
	}
	raw, err := json.Marshal(ops[0].Message)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), `"severity"`) {
		t.Fatalf("an unrated signal must omit severity from the wire, got %s", raw)
	}
}

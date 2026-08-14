package chat

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/addressing"
)

// /close is the one command the REPLY path handles. Everything else a person
// types in a thread is the agent's business, so the tests here are mostly about
// what /close must NOT do: reach the agent, leave the conversation open, or
// close only the channel it arrived on.

const testNS = "agent-ops"

func closeTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := agentopsv1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// closeFixture builds a conversation bound to two channels, each with a thread.
func closeFixture(t *testing.T, objs ...client.Object) (*Router, *OpQueue, client.Client) {
	t.Helper()
	// WithStatusSubresource: closing is a STATUS write now, and the fake client
	// silently drops status patches for types it does not know have the
	// subresource — which would make every assertion here pass for the wrong
	// reason.
	c := fake.NewClientBuilder().WithScheme(closeTestScheme(t)).
		WithStatusSubresource(&agentopsv1alpha1.Conversation{}).
		WithObjects(objs...).Build()
	q := &OpQueue{Client: c, Namespace: testNS, Registry: NewRegistry()}
	return &Router{Client: c, Reader: c, Namespace: testNS, Ops: q}, q, c
}

func nsChannel(name, adapter string) *agentopsv1alpha1.Channel {
	ch := testChannel(name, adapter)
	ch.Namespace = testNS
	return ch
}

func boundConv(name string, channels ...string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Name, c.Namespace = name, testNS
	for _, ch := range channels {
		c.Spec.ChannelRefs = append(c.Spec.ChannelRefs, agentopsv1alpha1.ObjectRef{Name: ch})
		c.Status.Threads = append(c.Status.Threads, agentopsv1alpha1.ThreadBinding{
			Channel: ch, ThreadID: "thread-" + ch,
		})
	}
	return c
}

func drain(q *OpQueue, adapter string) []*Op {
	var out []*Op
	for {
		op := q.Claim(adapter)
		if op == nil {
			return out
		}
		out = append(out, op)
	}
}

// Closing is a state transition now, not a deletion: the object survives at
// phase Closed with its runs, its context handle and its volume state intact,
// which is what makes it reopenable. Deletion is a second verb with its own
// flag and its own clock.
func TestCloseInThreadClosesAndSaysGoodbyeOnEveryChannel(t *testing.T) {
	conv := boundConv("conv-1", "c1", "c2")
	r, q, c := closeFixture(t, nsChannel("c1", "slack"), nsChannel("c2", "teams"), conv)

	thread := "thread-c1"
	if err := r.HandleMessage(context.Background(), nsChannel("c1", "slack"),
		InboundMessage{ThreadID: &thread, Text: "/close"}); err != nil {
		t.Fatal(err)
	}

	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "conv-1"}, &got); err != nil {
		t.Fatalf("closing must NOT delete the conversation: %v", err)
	}
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("phase must be Closed, got %q", got.Status.Phase)
	}
	if got.Status.ClosedAt == nil {
		t.Fatal("closedAt must be stamped — it is the origin of the delete clock")
	}

	// the farewell reaches BOTH bound threads — closing ends the conversation,
	// not the surface the command arrived on
	for _, tc := range []struct{ adapter, thread string }{{"slack", "thread-c1"}, {"teams", "thread-c2"}} {
		ops := drain(q, tc.adapter)
		if len(ops) != 1 {
			t.Fatalf("%s: want one farewell op, got %d", tc.adapter, len(ops))
		}
		if ops[0].Kind != OpSend || ops[0].ThreadID == nil || *ops[0].ThreadID != tc.thread {
			t.Fatalf("%s: farewell op %+v", tc.adapter, ops[0])
		}
		if !strings.Contains(opBody(ops[0]), "closed") {
			t.Fatalf("%s: farewell text %q", tc.adapter, opBody(ops[0]))
		}
	}
}

func TestCloseIsNotHandedToTheAgent(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	// nothing may be appended even though the reply path is what saw the text
	r, _, c := closeFixture(t, nsChannel("c1", "slack"), conv)

	thread := "thread-c1"
	if err := r.HandleMessage(context.Background(), nsChannel("c1", "slack"),
		InboundMessage{ThreadID: &thread, Text: "/close@AgentOpsBot"}); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "conv-1"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("bot-suffixed /close must close too, got phase %q", got.Status.Phase)
	}
	if len(got.Spec.Inputs) != 0 {
		t.Fatalf("the command must not become an input: %+v", got.Spec.Inputs)
	}
}

func TestCloseNamesAbandonedWork(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	conv.Status.Inflight = &agentopsv1alpha1.InflightRun{RunID: "run-7"}
	r, q, _ := closeFixture(t, nsChannel("c1", "slack"), conv)

	thread := "thread-c1"
	if err := r.HandleMessage(context.Background(), nsChannel("c1", "slack"),
		InboundMessage{ThreadID: &thread, Text: "/close"}); err != nil {
		t.Fatal(err)
	}
	ops := drain(q, "slack")
	if len(ops) != 1 || !strings.Contains(opBody(ops[0]), "run-7") {
		t.Fatalf("farewell must name the abandoned run: %+v", ops)
	}
}

func TestOrdinaryReplyStillReachesTheAgent(t *testing.T) {
	conv := boundConv("conv-1", "c1")
	r, _, c := closeFixture(t, nsChannel("c1", "slack"), conv)

	thread := "thread-c1"
	// trailing text makes it an instruction, not the command
	if err := r.HandleMessage(context.Background(), nsChannel("c1", "slack"),
		InboundMessage{ThreadID: &thread, Text: "/close the incident once you have filed it"}); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: testNS, Name: "conv-1"}, &got); err != nil {
		t.Fatalf("conversation must survive: %v", err)
	}
	if len(got.Spec.Inputs) != 1 || got.Spec.Inputs[0].Type != agentopsv1alpha1.InputReply {
		t.Fatalf("reply input: %+v", got.Spec.Inputs)
	}
}

func TestCloseOnGeneralSurfaceAnswersWithUsage(t *testing.T) {
	r, q, c := closeFixture(t, nsChannel("c1", "slack"))
	cmd, ok := addressing.Parse("/close")
	if !ok {
		t.Fatal("parse")
	}
	if err := r.HandleCommand(context.Background(), nsChannel("c1", "slack"), cmd); err != nil {
		t.Fatal(err)
	}
	ops := drain(q, "slack")
	if len(ops) != 1 || ops[0].ThreadID != nil {
		t.Fatalf("want one general-surface reply, got %+v", ops)
	}
	if !strings.Contains(opBody(ops[0]), "/close") || strings.Contains(opBody(ops[0]), "Unknown agent") {
		t.Fatalf("must explain usage rather than report an unknown pipeline: %q", opBody(ops[0]))
	}
	var list agentopsv1alpha1.ConversationList
	if err := c.List(context.Background(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("general-surface /close created %d conversation(s)", len(list.Items))
	}
}

func TestIsCloseCommand(t *testing.T) {
	for text, want := range map[string]bool{
		"/close":              true,
		"/close@AgentOpsBot":  true,
		"/close ":             true,
		"/closer":             false,
		"/close:agent":        false,
		"/close file the doc": false,
		"close":               false,
		"please /close":       false,
	} {
		if got := isCloseCommand(strings.TrimSpace(text)); got != want {
			t.Errorf("isCloseCommand(%q) = %v, want %v", text, got, want)
		}
	}
}

// opBody is the message body an op would render from, or "" when the op carries
// no message. Tests assert on MEANING now — the markup is the adapter's.
func opBody(op *Op) string {
	if op == nil || op.Message == nil {
		return ""
	}
	return op.Message.Body
}

// A reopen is announced by the MANAGER on every bound thread — not synthesized
// by whichever adapter happens to implement it, which would leave the other
// surfaces silently starting to work again.
func TestReopenNoticeReachesEveryBoundThread(t *testing.T) {
	conv := boundConv("conv-1", "c1", "c2")
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	conv.Status.Reopens = 1
	r, q, _ := closeFixture(t, nsChannel("c1", "slack"), nsChannel("c2", "teams"), conv)

	r.FanOutReopenNotice(context.Background(), conv)

	for _, tc := range []struct{ adapter, thread string }{{"slack", "thread-c1"}, {"teams", "thread-c2"}} {
		ops := drain(q, tc.adapter)
		if len(ops) != 1 {
			t.Fatalf("%s: want one reopen notice, got %d", tc.adapter, len(ops))
		}
		if ops[0].ThreadID == nil || *ops[0].ThreadID != tc.thread {
			t.Fatalf("%s: notice went to %v", tc.adapter, ops[0].ThreadID)
		}
		if !strings.Contains(opBody(ops[0]), "reopened") {
			t.Fatalf("%s: notice text %q", tc.adapter, opBody(ops[0]))
		}
	}

	// Re-derived on every reconcile, so it must dedup — and a SECOND reopen
	// must get its own.
	r.FanOutReopenNotice(context.Background(), conv)
	if again := drain(q, "slack"); len(again) != 0 {
		t.Fatalf("a re-derived notice must dedup: %d", len(again))
	}
	conv.Status.Reopens = 2
	r.FanOutReopenNotice(context.Background(), conv)
	if second := drain(q, "slack"); len(second) != 1 {
		t.Fatalf("a second reopen is owed its own notice, got %d", len(second))
	}
}

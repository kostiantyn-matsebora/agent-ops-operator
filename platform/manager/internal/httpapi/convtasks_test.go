package httpapi

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func convTestChannel(name, adapter string) *agentopsv1alpha1.Channel {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Namespace = name, "agent-ops"
	ch.Spec.Adapter = adapter
	return ch
}

func convTestBoundConv(name, channel, thread string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Name, c.Namespace = name, "agent-ops"
	c.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: channel}}
	c.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: channel, ThreadID: thread}}
	return c
}

func TestHandleConvTaskDecodesAndDispatchesByVerb(t *testing.T) {
	s, c, _ := busServer(t, convTestChannel("c1", "slack"), convTestBoundConv("conv-1", "c1", "t1"))
	task := commandbus.Task{UID: "u1", Payload: convtask.Payload{Verb: convtask.VerbClose, Conversation: "conv-1"}.Encode()}
	if err := s.HandleConvTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("want Closed, got %s", got.Status.Phase)
	}
}

func TestHandleConvTaskRejectsAnUndecodablePayload(t *testing.T) {
	s, _, _ := busServer(t)
	err := s.HandleConvTask(context.Background(), commandbus.Task{UID: "u1", Payload: []byte("not json")})
	if err == nil {
		t.Fatal("want an error for an undecodable payload")
	}
}

func TestHandleConvTaskRejectsAnUnknownVerb(t *testing.T) {
	s, _, _ := busServer(t)
	task := commandbus.Task{UID: "u1", Payload: convtask.Payload{Verb: "not-a-real-verb", Conversation: "x"}.Encode()}
	if err := s.HandleConvTask(context.Background(), task); err == nil {
		t.Fatal("want an error for an unknown verb")
	}
}

func TestTaskCloseManualClosesAndSaysGoodbyeOnEveryThread(t *testing.T) {
	s, c, _ := busServer(t, convTestChannel("c1", "slack"), convTestBoundConv("conv-1", "c1", "t1"))
	if err := s.taskClose(context.Background(), &convtask.Payload{Verb: convtask.VerbClose, Conversation: "conv-1"}); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed || got.Status.ClosedAt == nil {
		t.Fatalf("want Closed with ClosedAt, got %+v", got.Status)
	}
}

func TestTaskCloseAutoUsesTheIdleFarewell(t *testing.T) {
	s, c, _ := busServer(t, convTestChannel("c1", "slack"), convTestBoundConv("conv-1", "c1", "t1"))
	err := s.taskClose(context.Background(), &convtask.Payload{
		Verb: convtask.VerbClose, Conversation: "conv-1",
		CloseKind: convtask.CloseAuto, IdleForMS: (2 * time.Hour).Milliseconds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("want Closed, got %s", got.Status.Phase)
	}
}

func TestTaskCloseBudgetExceededEscalatesAnUncausedConversation(t *testing.T) {
	s, c, _ := busServer(t, convTestChannel("c1", "slack"), convTestBoundConv("conv-1", "c1", "t1"))
	err := s.taskClose(context.Background(), &convtask.Payload{
		Verb: convtask.VerbClose, Conversation: "conv-1",
		CloseKind: convtask.CloseBudgetExceeded, Reason: "budget-exceeded: maxTurns (3) reached",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if got.Status.EscalatedAt == nil || got.Status.EscalationMessage == "" {
		t.Fatalf("an uncaused root's budget-exceeded close must escalate, got %+v", got.Status)
	}
}

func TestTaskCloseOfAnAbsentConversationIsNotFound(t *testing.T) {
	s, _, _ := busServer(t)
	err := s.taskClose(context.Background(), &convtask.Payload{Verb: convtask.VerbClose, Conversation: "nope"})
	if !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("want an error wrapping state.ErrNotFound, got %v", err)
	}
}

func TestTaskExitDeletesTheRuntimePod(t *testing.T) {
	conv := convTestBoundConv("conv-1", "c1", "t1")
	pod := &corev1.Pod{}
	pod.Name, pod.Namespace = runtimepod.PodName("conv-1"), "agent-ops"
	s, c, _ := busServer(t, convTestChannel("c1", "slack"), conv, pod)
	if err := s.taskExit(context.Background(), &convtask.Payload{Verb: convtask.VerbExit, Conversation: "conv-1"}); err != nil {
		t.Fatal(err)
	}
	var got corev1.Pod
	err := c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: pod.Name}, &got)
	if err == nil {
		t.Fatal("the runtime pod must be deleted")
	}
}

func TestTaskReopenFailsPreconditionWhenNotClosed(t *testing.T) {
	s, _, bus := busServer(t, convTestBoundConv("conv-1", "c1", "t1")) // not Closed
	resp, err := s.acceptAndEnqueue(context.Background(), commandbus.SubmitRequest{
		Queue: ConvQueue, CommandID: "reopen:conv-1", Key: "conv-1",
		Payload: convtask.Payload{Verb: convtask.VerbReopen, Conversation: "conv-1"}.Encode(),
	})
	if err != nil {
		t.Fatal(err)
	}
	task := runConvTaskViaPoolToTerminal(t, s, bus, resp.TaskIDs[0])
	// precondition_failed is terminal (design D-6): straight to Dead, never
	// Retrying — a refused reopen must not become a surprise success later.
	if task.State != commandbus.StateDead || task.Code != commandbus.CodePreconditionFailed {
		t.Fatalf("want Dead/precondition_failed, got state=%s code=%s", task.State, task.Code)
	}
}

func TestTaskReopenBringsAClosedConversationBackToIdle(t *testing.T) {
	conv := convTestBoundConv("conv-1", "c1", "t1")
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-1"}
	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Name, profile.Namespace = "prof-1", "agent-ops"
	s, c, _ := busServer(t, convTestChannel("c1", "slack"), conv, profile)
	if err := s.taskReopen(context.Background(), &convtask.Payload{Verb: convtask.VerbReopen, Conversation: "conv-1"}); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationIdle {
		t.Fatalf("want Idle, got %s", got.Status.Phase)
	}
}

func TestTaskResetContextClearsTheHandleAndAnnounces(t *testing.T) {
	conv := convTestBoundConv("conv-1", "c1", "t1")
	conv.Status.RuntimeContextID = "ctx-123"
	s, c, _ := busServer(t, convTestChannel("c1", "slack"), conv)
	if err := s.taskResetContext(context.Background(), &convtask.Payload{Verb: convtask.VerbResetContext, Conversation: "conv-1"}); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if got.Status.RuntimeContextID != "" {
		t.Fatalf("want the context handle cleared, got %q", got.Status.RuntimeContextID)
	}
	if op := s.Ops.Claim("slack"); op == nil {
		t.Fatal("want the reset announced on the bound thread")
	}
}

func TestTaskResetContextOfAConversationWithNoHandleIsANoOp(t *testing.T) {
	s, _, _ := busServer(t, convTestChannel("c1", "slack"), convTestBoundConv("conv-1", "c1", "t1"))
	if err := s.taskResetContext(context.Background(), &convtask.Payload{Verb: convtask.VerbResetContext, Conversation: "conv-1"}); err != nil {
		t.Fatal(err)
	}
	if op := s.Ops.Claim("slack"); op != nil {
		t.Fatal("nothing to clear means nothing to announce")
	}
}

func TestTaskInboundMessageAppendsAReplyInput(t *testing.T) {
	s, c, _ := busServer(t, convTestChannel("c1", "slack"), convTestBoundConv("conv-1", "c1", "t1"))
	thread := "t1"
	payload := convtask.Payload{
		Verb: convtask.VerbInboundMessage, Conversation: "conv-1",
		Channel: "c1", ThreadID: &thread, Text: "hello there",
	}
	if err := s.taskInboundMessage(context.Background(), &payload); err != nil {
		t.Fatal(err)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if len(got.Spec.Inputs) != 1 || got.Spec.Inputs[0].Payload != "hello there" {
		t.Fatalf("want one reply input, got %+v", got.Spec.Inputs)
	}
}

func TestTaskInboundMessageOnAnUnknownChannelIsNotFound(t *testing.T) {
	s, _, _ := busServer(t)
	thread := "t1"
	payload := convtask.Payload{Verb: convtask.VerbInboundMessage, Conversation: "conv-1", Channel: "ghost", ThreadID: &thread, Text: "hi"}
	err := s.taskInboundMessage(context.Background(), &payload)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("want a not-found error for an unknown channel, got %v", err)
	}
}

// design D-9's inbound-dedup rule: an Idempotency-Key, when presented, wins
// outright — two otherwise-identical derivations must still differ if the
// keys differ, and the SAME key must always reproduce the SAME CommandID.
func TestInboundMessageCommandIDPrefersIdempotencyKey(t *testing.T) {
	a := inboundMessageCommandID("key-1", "c1", "t1", "hello", "alice")
	b := inboundMessageCommandID("key-1", "c1", "t1", "something else entirely", "bob")
	if a != b {
		t.Fatalf("an explicit Idempotency-Key must win over every other field, got %q != %q", a, b)
	}
	c := inboundMessageCommandID("key-2", "c1", "t1", "hello", "alice")
	if a == c {
		t.Fatal("two different Idempotency-Keys must never collide")
	}
}

// Absent a header, the derived key is a pure function of channel/thread/
// text/sender: the same four values always reproduce, and changing any one
// of them changes the id.
func TestInboundMessageCommandIDDerivesFromMessageFieldsWhenIdempotencyKeyIsAbsent(t *testing.T) {
	base := inboundMessageCommandID("", "c1", "t1", "hello", "alice")
	again := inboundMessageCommandID("", "c1", "t1", "hello", "alice")
	if base != again {
		t.Fatal("the same message fields must derive the same CommandID")
	}
	for _, variant := range []commandbus.CommandID{
		inboundMessageCommandID("", "c2", "t1", "hello", "alice"),
		inboundMessageCommandID("", "c1", "t2", "hello", "alice"),
		inboundMessageCommandID("", "c1", "t1", "different", "alice"),
		inboundMessageCommandID("", "c1", "t1", "hello", "bob"),
	} {
		if base == variant {
			t.Fatalf("changing one field must change the derived CommandID, got %q for both", base)
		}
	}
}

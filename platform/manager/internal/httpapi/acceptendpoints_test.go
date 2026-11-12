// End-to-end accept tests (async-command-bus design D-9) for the four
// endpoints this change converts: 202 + invocationId/taskIds at accept,
// then the eventual effect observed by draining the real consumer.Pool —
// the shape this change's own hand-back report names as the test pattern
// for an endpoint that used to be synchronous.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

func acceptReq(s *Server, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Authorization", "Bearer "+s.AdapterToken)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// drainAllConvTasks claims and handles every task currently queued or
// retry-eligible on ConvQueue, in a bounded number of rounds — the "run the
// registered consumer pool" half of this package's test guidance, for a
// handler test that only cares about the eventual effect, not the code
// classification (statetasks_test.go/convtasks_test.go cover that).
func drainAllConvTasks(t *testing.T, s *Server, bus commandbus.Bus, taskIDs []string) {
	t.Helper()
	for range taskIDs {
		drainOneConvTask(t, s, bus)
	}
}

func acceptedIDs(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var out struct {
		TaskIDs []string `json:"taskIds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.TaskIDs
}

func TestHandleConversationReopenAcceptsThenReopens(t *testing.T) {
	conv := convTestBoundConv("conv-1", "c1", "t1")
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	conv.Spec.ProfileRef = agentopsv1alpha1.ObjectRef{Name: "prof-1"}
	profile := &agentopsv1alpha1.AgentProfile{}
	profile.Name, profile.Namespace = "prof-1", "agent-ops"
	s, c, bus := busServer(t, convTestChannel("c1", "slack"), conv, profile)

	rec := acceptReq(s, "POST", "/channel/conversations/conv-1/reopen", map[string]any{"channel": "c1"})
	if rec.Code != 202 {
		t.Fatalf("want 202, got %d: %s", rec.Code, rec.Body.String())
	}
	ids := acceptedIDs(t, rec)
	if len(ids) != 1 {
		t.Fatalf("want one taskId, got %v", ids)
	}
	// Accepted, not yet performed: the conversation is still Closed right
	// after the 202 — exactly the semantic the proposal names BREAKING.
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationClosed {
		t.Fatalf("accept must not perform the work: want still Closed, got %s", got.Status.Phase)
	}

	drainAllConvTasks(t, s, bus, ids)
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if got.Status.Phase != agentopsv1alpha1.ConversationIdle {
		t.Fatalf("once processed, want Idle, got %s", got.Status.Phase)
	}
}

func TestHandleConversationDeleteOfAnAbsentConversationSucceedsWithNoTask(t *testing.T) {
	s, _, _ := busServer(t)
	rec := acceptReq(s, "POST", "/channel/conversations/never-existed/delete", map[string]any{"channel": "c1"})
	if rec.Code != 202 {
		t.Fatalf("want 202 for an already-absent conversation, got %d: %s", rec.Code, rec.Body.String())
	}
	ids := acceptedIDs(t, rec)
	if len(ids) != 0 {
		t.Fatalf("nothing to do means no task, got %v", ids)
	}
}

func TestHandleConversationDeleteRefusesANotYetClosedConversationImmediately(t *testing.T) {
	s, _, _ := busServer(t, convTestChannel("c1", "slack"), convTestBoundConv("conv-1", "c1", "t1"))
	rec := acceptReq(s, "POST", "/channel/conversations/conv-1/delete", map[string]any{"channel": "c1"})
	if rec.Code != 409 {
		t.Fatalf("want 409 for a conversation that is not Closed, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleConversationDeleteAcceptsThenRemovesTheWholeCascade(t *testing.T) {
	root := convTestBoundConv("root-1", "c1", "t1")
	root.Status.Phase = agentopsv1alpha1.ConversationClosed
	mid := &agentopsv1alpha1.Conversation{}
	mid.Name, mid.Namespace = "mid-1", "agent-ops"
	mid.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "worker"}
	mid.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: "root-1"}
	mid.Status.Phase = agentopsv1alpha1.ConversationClosed
	s, c, bus := busServer(t, convTestChannel("c1", "slack"), root, mid)

	rec := acceptReq(s, "POST", "/channel/conversations/root-1/delete", map[string]any{"channel": "c1"})
	if rec.Code != 202 {
		t.Fatalf("want 202, got %d: %s", rec.Code, rec.Body.String())
	}
	ids := acceptedIDs(t, rec)
	if len(ids) != 2 {
		t.Fatalf("want one task per conversation in the cascade (2), got %v", ids)
	}
	drainAllConvTasks(t, s, bus, ids)

	for _, name := range []string{"root-1", "mid-1"} {
		var got agentopsv1alpha1.Conversation
		err := c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: name}, &got)
		if err == nil {
			t.Fatalf("%s must be gone once the cascade is processed", name)
		}
	}
}

func TestHandleContextResetAcceptsThenClearsAndAnnounces(t *testing.T) {
	conv := convTestBoundConv("conv-1", "c1", "t1")
	conv.Status.RuntimeContextID = "ctx-123"
	s, c, bus := busServer(t, convTestChannel("c1", "slack"), conv)

	rec := acceptReq(s, "POST", "/channel/conversations/conv-1/reset-context", map[string]any{"channel": "c1"})
	if rec.Code != 202 {
		t.Fatalf("BREAKING per proposal.md: want 202 (was 200), got %d: %s", rec.Code, rec.Body.String())
	}
	ids := acceptedIDs(t, rec)
	drainAllConvTasks(t, s, bus, ids)

	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if got.Status.RuntimeContextID != "" {
		t.Fatalf("want the handle cleared once processed, got %q", got.Status.RuntimeContextID)
	}
	if s.Ops.Claim("slack") == nil {
		t.Fatal("want the reset announced once processed")
	}
}

func TestHandleChannelInboundSelfInputIsRefusedImmediately(t *testing.T) {
	// A channel named identically to the conversation it would feed —
	// coordination-loop's guard, which accept.go checks WITHOUT a task.
	conv := &agentopsv1alpha1.Conversation{}
	conv.Name, conv.Namespace = "same-name", "agent-ops"
	conv.Spec.ChannelRefs = []agentopsv1alpha1.ObjectRef{{Name: "same-name"}}
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: "same-name", ThreadID: "t1"}}
	s, _, _ := busServer(t, convTestChannel("same-name", "slack"), conv)
	rec := acceptReq(s, "POST", "/channel/inbound", map[string]any{"channel": "same-name", "threadId": "t1", "text": "hi"})
	if rec.Code != 400 {
		t.Fatalf("want 400 for a self-input loop, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleChannelInboundOnAClosedConversationAnswersImmediatelyWithNoTask(t *testing.T) {
	conv := convTestBoundConv("conv-1", "c1", "t1")
	conv.Status.Phase = agentopsv1alpha1.ConversationClosed
	s, _, _ := busServer(t, convTestChannel("c1", "slack"), conv)
	rec := acceptReq(s, "POST", "/channel/inbound", map[string]any{"channel": "c1", "threadId": "t1", "text": "hello?"})
	if rec.Code != 202 {
		t.Fatalf("want 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if ids := acceptedIDs(t, rec); len(ids) != 0 {
		t.Fatalf("a closed-thread notice is immediate, no task — got %v", ids)
	}
	if s.Ops.Claim("slack") == nil {
		t.Fatal("want the closed-conversation notice enqueued immediately")
	}
}

func TestHandleChannelInboundAcceptsThenAppendsTheReply(t *testing.T) {
	s, c, bus := busServer(t, convTestChannel("c1", "slack"), convTestBoundConv("conv-1", "c1", "t1"))
	rec := acceptReq(s, "POST", "/channel/inbound", map[string]any{"channel": "c1", "threadId": "t1", "text": "what's the status?"})
	if rec.Code != 202 {
		t.Fatalf("want 202, got %d: %s", rec.Code, rec.Body.String())
	}
	ids := acceptedIDs(t, rec)
	if len(ids) != 1 {
		t.Fatalf("want one task, got %v", ids)
	}
	var got agentopsv1alpha1.Conversation
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if len(got.Spec.Inputs) != 0 {
		t.Fatalf("accept must not perform the work: want no input yet, got %+v", got.Spec.Inputs)
	}
	drainAllConvTasks(t, s, bus, ids)
	c.Get(context.Background(), types.NamespacedName{Namespace: "agent-ops", Name: "conv-1"}, &got)
	if len(got.Spec.Inputs) != 1 || got.Spec.Inputs[0].Payload != "what's the status?" {
		t.Fatalf("once processed, want one reply input, got %+v", got.Spec.Inputs)
	}
}

// Two identical inbound POSTs with no Idempotency-Key, inside the "conv"
// queue's dedup window, derive the SAME CommandID (design D-9) and
// therefore submit only ONE task — the second accept call is suppressed,
// not a second task racing the first.
func TestHandleChannelInboundRedeliveryWithoutIdempotencyKeyDedupsToOneTask(t *testing.T) {
	s, _, _ := busServer(t, convTestChannel("c1", "slack"), convTestBoundConv("conv-1", "c1", "t1"))
	body := map[string]any{"channel": "c1", "threadId": "t1", "text": "repeat me"}
	first := acceptReq(s, "POST", "/channel/inbound", body)
	second := acceptReq(s, "POST", "/channel/inbound", body)
	if first.Code != 202 || second.Code != 202 {
		t.Fatalf("want 202/202, got %d/%d", first.Code, second.Code)
	}
	firstIDs, secondIDs := acceptedIDs(t, first), acceptedIDs(t, second)
	if len(firstIDs) != 1 || len(secondIDs) != 1 || firstIDs[0] != secondIDs[0] {
		t.Fatalf("a redelivery with no Idempotency-Key must resolve to the SAME task, got %v and %v", firstIDs, secondIDs)
	}
}

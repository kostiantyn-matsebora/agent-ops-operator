package integration

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// The four scenarios signal-correlation-id's pure id-equality routing
// replaces label/signature grouping with: a repeat of an OPEN conversation's
// own id recurs into it, a never-seen id opens a fresh one adopting it, two
// distinct ids never fold together no matter what they share, and a CLOSED
// conversation is excluded from the match regardless of id equality.

// Open-match recurrence: once a conversation is open with a session, a
// repeat of the SAME initiationId lands on it as a recurrence rather than
// opening a second conversation.
func TestOpenConversationWithMatchingConversationIDGetsRecurrence(t *testing.T) {
	ctx := context.Background()
	mkProfile(t, "prof-corr-open")
	mkSignalSource(t, "src-corr-open", "corr-open", "")
	mkPipeline(t, "corr-open-pipe", []string{"src-corr-open"}, nil, "prof-corr-open")
	reconcilePipeline(t, "corr-open-pipe")

	h1 := apiServer().Handler()
	if rec := postSignal(t, h1, testMasterToken, "src-corr-open", []map[string]any{
		{"initiationId": "corr-open-1", "payload": "first"},
	}); rec.Code != 200 {
		t.Fatalf("first signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsFromPipeline(t, "corr-open-pipe")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	conv := convs[0]
	if conv.Spec.ConversationID != "corr-open-1" {
		t.Fatalf("conversation must adopt the initiationId, got %q", conv.Spec.ConversationID)
	}
	// a session now exists, as it would once the agent has run
	conv.Status.SessionID = "sess-corr-open"
	if err := k8sClient.Status().Update(ctx, &conv); err != nil {
		t.Fatal(err)
	}
	waitForRoutingCacheToObserve(t, &conv, conv.ResourceVersion)

	// a fresh server stands in for "outside the source's cooldown" without
	// waiting out its real window — but its cooldown map RECOVERS from the
	// source's durable record on first use, so that record must itself look
	// expired, or the repeat is suppressed before routing ever runs.
	backdateCooldown(t, "src-corr-open", "corr-open-1")
	h2 := apiServer().Handler()
	if rec := postSignal(t, h2, testMasterToken, "src-corr-open", []map[string]any{
		{"initiationId": "corr-open-1", "payload": "second"},
	}); rec.Code != 200 {
		t.Fatalf("repeat signal: %d %s", rec.Code, rec.Body.String())
	}

	after := convsFromPipeline(t, "corr-open-pipe")
	if len(after) != 1 {
		t.Fatalf("a matching id must recur into the open conversation, not open a second one, got %d", len(after))
	}
	if len(after[0].Spec.Inputs) != 2 || after[0].Spec.Inputs[1].Type != agentopsv1alpha1.InputRecurrence {
		t.Fatalf("recurrence input expected: %+v", after[0].Spec.Inputs)
	}
}

// Fresh-id creation: an initiationId never seen before opens a new
// conversation, adopting it verbatim as conversationId.
func TestFreshInitiationIDOpensANewConversation(t *testing.T) {
	mkProfile(t, "prof-corr-fresh")
	mkSignalSource(t, "src-corr-fresh", "corr-fresh", "")
	mkPipeline(t, "corr-fresh-pipe", []string{"src-corr-fresh"}, nil, "prof-corr-fresh")
	reconcilePipeline(t, "corr-fresh-pipe")

	h := apiServer().Handler()
	if rec := postSignal(t, h, testMasterToken, "src-corr-fresh", []map[string]any{
		{"initiationId": "corr-fresh-1", "payload": "hello"},
	}); rec.Code != 200 {
		t.Fatalf("signal: %d %s", rec.Code, rec.Body.String())
	}
	convs := convsFromPipeline(t, "corr-fresh-pipe")
	if len(convs) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(convs))
	}
	if convs[0].Spec.ConversationID != "corr-fresh-1" {
		t.Fatalf("a fresh id must be adopted verbatim, got %q", convs[0].Spec.ConversationID)
	}
}

// Distinct-ids-never-merge: two signals with different initiationIds never
// fold into one conversation, even sharing every label — equality routing
// carries no label-derived fallback.
func TestDistinctInitiationIDsNeverMerge(t *testing.T) {
	mkProfile(t, "prof-corr-distinct")
	mkSignalSource(t, "src-corr-distinct", "corr-distinct", "")
	mkPipeline(t, "corr-distinct-pipe", []string{"src-corr-distinct"}, nil, "prof-corr-distinct")
	reconcilePipeline(t, "corr-distinct-pipe")

	h := apiServer().Handler()
	labels := map[string]string{"alertname": "SameAlert", "namespace": "same-ns"}
	for _, id := range []string{"corr-distinct-a", "corr-distinct-b"} {
		if rec := postSignal(t, h, testMasterToken, "src-corr-distinct", []map[string]any{
			{"initiationId": id, "labels": labels, "payload": "firing"},
		}); rec.Code != 200 {
			t.Fatalf("signal %s: %d %s", id, rec.Code, rec.Body.String())
		}
	}
	convs := convsFromPipeline(t, "corr-distinct-pipe")
	if len(convs) != 2 {
		t.Fatalf("two distinct initiationIds sharing every label must never merge, want 2 conversations, got %d", len(convs))
	}
	ids := map[string]bool{}
	for _, c := range convs {
		ids[c.Spec.ConversationID] = true
		if len(c.Spec.Inputs) != 1 {
			t.Fatalf("%s: each conversation owns exactly its own input, got %+v", c.Name, c.Spec.Inputs)
		}
	}
	if !ids["corr-distinct-a"] || !ids["corr-distinct-b"] {
		t.Fatalf("each conversation must carry its own id, got %+v", ids)
	}
}

// Closed-conversation exclusion: a CLOSED conversation is never matched by
// equality routing even though its conversationId still equals the incoming
// initiationId — the repeat opens a new conversation rather than reusing a
// conversation somebody tidied away.
func TestClosedConversationExcludedFromEqualityRouting(t *testing.T) {
	mkProfile(t, "prof-corr-closed")
	mkSignalSource(t, "src-corr-closed", "corr-closed", "")
	mkPipeline(t, "corr-closed-pipe", []string{"src-corr-closed"}, nil, "prof-corr-closed")
	reconcilePipeline(t, "corr-closed-pipe")

	h1 := apiServer().Handler()
	if rec := postSignal(t, h1, testMasterToken, "src-corr-closed", []map[string]any{
		{"initiationId": "corr-closed-1", "payload": "first"},
	}); rec.Code != 200 {
		t.Fatalf("first signal: %d %s", rec.Code, rec.Body.String())
	}
	first := convsFromPipeline(t, "corr-closed-pipe")
	if len(first) != 1 {
		t.Fatalf("want 1 conversation, got %d", len(first))
	}
	c := getConv(t, first[0].Name)
	now := metav1.Now()
	c.Status.Phase = agentopsv1alpha1.ConversationClosed
	c.Status.ClosedAt = &now
	if err := k8sClient.Status().Update(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	waitForRoutingCacheToObserve(t, c, c.ResourceVersion)

	backdateCooldown(t, "src-corr-closed", "corr-closed-1")
	h2 := apiServer().Handler()
	if rec := postSignal(t, h2, testMasterToken, "src-corr-closed", []map[string]any{
		{"initiationId": "corr-closed-1", "payload": "second"},
	}); rec.Code != 200 {
		t.Fatalf("repeat signal: %d %s", rec.Code, rec.Body.String())
	}
	all := convsFromPipeline(t, "corr-closed-pipe")
	if len(all) != 2 {
		t.Fatalf("a signal matching a CLOSED conversation's id must open a new one, got %d", len(all))
	}
	for _, cv := range all {
		if cv.Name == first[0].Name && len(cv.Spec.Inputs) > 1 {
			t.Fatal("the closed conversation must not have absorbed the repeat")
		}
	}
}

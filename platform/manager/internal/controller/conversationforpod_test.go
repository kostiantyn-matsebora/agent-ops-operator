// conversationForPod resolves a live runtime pod to its conversation
// through the agentops.dev/conversation-id label alone (conversation-
// identity, Decision 7) — never the pod's own Name, and never by assuming a
// present label names a live record.
package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// A pod carrying no conversation-id label at all (an un-labelled or
// pre-upgrade pod) resolves to no conversation, and that is not an error —
// only a genuine read failure is.
func TestConversationForPodWithNoLabelResolvesToNothing(t *testing.T) {
	plain := claimTestConv("plain-1")
	c := fakeClaimClient(t, plain).Build()
	r := &ConversationReconciler{Client: c, Conversations: claimStore(c)}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "agentops-conv-plain-1"}}
	rec, ok, err := r.conversationForPod(context.Background(), pod)
	if err != nil {
		t.Fatalf("conversationForPod: %v", err)
	}
	if ok {
		t.Fatalf("a pod with no conversation-id label must resolve to no conversation (its Name matching one must not adopt it), got %+v", rec)
	}
}

// An ID naming no live record (a pod outliving its own deleted conversation,
// or one whose ID is simply unknown) resolves to nothing too, without error
// — never adopted by a record that happens to share the pod's Name.
func TestConversationForPodWithAnUnknownIDResolvesToNothingWithoutError(t *testing.T) {
	plain := claimTestConv("plain-1")
	c := fakeClaimClient(t, plain).Build()
	r := &ConversationReconciler{Client: c, Conversations: claimStore(c)}

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:   "agentops-conv-plain-1",
		Labels: map[string]string{runtimepod.LabelConversationID: "no-such-id"},
	}}
	rec, ok, err := r.conversationForPod(context.Background(), pod)
	if err != nil {
		t.Fatalf("conversationForPod: %v", err)
	}
	if ok {
		t.Fatalf("an unknown id must resolve to no conversation, got %+v", rec)
	}
}

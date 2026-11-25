package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// poolOf classifies by provenance alone (design D-1): CausedBy set is a
// member, everything else — including a record this call could not read —
// is a conversation, the conservative reading that never invents capacity
// for members.
func TestPoolOfClassifiesByProvenanceAndFailsClosedToConversations(t *testing.T) {
	member := state.ConversationRecord{CausedBy: &state.Provenance{Parent: "root"}}
	if got := poolOf(member, nil); got != poolMembers {
		t.Fatalf("a record with CausedBy set must be a member, got %q", got)
	}

	plain := state.ConversationRecord{}
	if got := poolOf(plain, nil); got != poolConversations {
		t.Fatalf("a record with no CausedBy must be a conversation, got %q", got)
	}

	// An unreadable record counts as a conversation even if the zero value
	// handed back happens to carry a CausedBy from some earlier call —
	// err != nil must win outright.
	unreadable := state.ConversationRecord{CausedBy: &state.Provenance{Parent: "root"}}
	if got := poolOf(unreadable, state.ErrNotFound); got != poolConversations {
		t.Fatalf("an unreadable record must count as a conversation, got %q", got)
	}
}

// splitPools looks up each live pod's own conversation and buckets it —
// unaffected by what order the pods were listed in, and an unreadable
// conversation (its record deleted before its pod) falls into
// conversations, never members.
func TestSplitPoolsBucketsPodsByTheirConversationsRecord(t *testing.T) {
	member := claimTestConv("member-1")
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1"}
	plain := claimTestConv("plain-1")
	c := fakeClaimClient(t, member, plain).Build()
	r := &ConversationReconciler{
		Client: c, Conversations: claimStore(c),
		Namespaces: namespace.Namespaces{Control: claimTestNS, Conversations: claimTestNS},
	}

	live := []corev1.Pod{
		podNamed("member-1"), podNamed("plain-1"), podNamed("gone-1"), // no matching record
	}
	split := r.splitPools(context.Background(), live)

	if names := podNames(split[poolMembers]); len(names) != 1 || names[0] != "member-1" {
		t.Fatalf("members pool = %v, want exactly [member-1]", names)
	}
	wantConversations := map[string]bool{"plain-1": true, "gone-1": true}
	if names := podNames(split[poolConversations]); len(names) != 2 || !wantConversations[names[0]] || !wantConversations[names[1]] {
		t.Fatalf("conversations pool = %v, want plain-1 and gone-1 (unreadable falls here)", names)
	}
}

// capFor never adds the two pools together — each reads its own field, and
// only its own.
func TestCapForReadsTheMatchingPoolOnly(t *testing.T) {
	r := &ConversationReconciler{MaxActiveConversations: 5, MaxActiveMembers: 9}
	if got := r.capFor(poolConversations); got != 5 {
		t.Fatalf("conversations cap = %d, want 5", got)
	}
	if got := r.capFor(poolMembers); got != 9 {
		t.Fatalf("members cap = %d, want 9", got)
	}
}

func podNamed(conv string) corev1.Pod {
	return corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:   "agentops-conv-" + conv,
		Labels: map[string]string{runtimepod.LabelConversationID: conv},
	}}
}

func podNames(pods []corev1.Pod) []string {
	out := make([]string, 0, len(pods))
	for _, p := range pods {
		out = append(out, p.Labels[runtimepod.LabelConversationID])
	}
	return out
}

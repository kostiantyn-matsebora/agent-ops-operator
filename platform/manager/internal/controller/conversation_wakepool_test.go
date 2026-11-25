package controller

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/namespace"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// pendingConv builds a Pending conversation, with or without CausedBy, and
// an explicit CreationTimestamp so FIFO order is deterministic.
func pendingConv(name string, member bool, age time.Duration) *agentopsv1alpha1.Conversation {
	conv := claimTestConv(name)
	conv.ObjectMeta.CreationTimestamp = metav1.NewTime(time.Now().Add(-age))
	conv.Status.Phase = agentopsv1alpha1.ConversationPending
	conv.Spec.Inputs = []agentopsv1alpha1.InputItem{{ID: "i1", Type: agentopsv1alpha1.InputTask, Payload: "do things"}}
	if member {
		conv.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1"}
	}
	return conv
}

func deletedPodFor(conv string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "agentops-conv-" + conv, Namespace: claimTestNS,
		Labels: map[string]string{runtimepod.LabelConversationID: conv},
	}}
}

// TestPodDeletionWakesOnlyItsOwnPool pins design D-4: a freed slot wakes the
// oldest PENDING conversations of the DELETED pod's own pool only. A freed
// member slot must never wake a pending conversation, and a freed
// conversation slot must never wake a pending member.
func TestPodDeletionWakesOnlyItsOwnPool(t *testing.T) {
	// The deleted pod's own conversation record (still around; only its pod
	// is gone) decides the pool — give it CausedBy so a freed MEMBER slot is
	// the one under test.
	memberActive := claimTestConv("member-active")
	memberActive.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1"}

	memberPending := pendingConv("member-pending", true, 2*time.Second)
	convPending := pendingConv("conv-pending", false, time.Second) // younger, other pool

	c := fakeClaimClient(t, memberActive, memberPending, convPending).Build()
	r := &ConversationReconciler{
		Client: c, Conversations: claimStore(c),
		Namespaces:             namespace.Namespaces{Control: claimTestNS, Conversations: claimTestNS},
		MaxActiveConversations: 5, MaxActiveMembers: 5,
	}

	reqs := r.MapRuntimePodToPending(context.Background(), deletedPodFor("member-active"))
	if len(reqs) != 1 || reqs[0].Name != "member-pending" {
		t.Fatalf("a freed member slot must wake the pending member only, got %+v", reqs)
	}
}

// The symmetric case: a freed CONVERSATION slot wakes a pending
// conversation, never a pending member — even one that is older and would
// otherwise win FIFO.
func TestPodDeletionNeverWakesTheOtherPool(t *testing.T) {
	convActive := claimTestConv("conv-active") // no CausedBy

	// The member is OLDER than the conversation, so a pool-blind FIFO would
	// wake it first. It must still lose to pool scoping.
	memberPending := pendingConv("member-pending", true, 10*time.Second)
	convPending := pendingConv("conv-pending", false, time.Second)

	c := fakeClaimClient(t, convActive, memberPending, convPending).Build()
	r := &ConversationReconciler{
		Client: c, Conversations: claimStore(c),
		Namespaces:             namespace.Namespaces{Control: claimTestNS, Conversations: claimTestNS},
		MaxActiveConversations: 5, MaxActiveMembers: 5,
	}

	reqs := r.MapRuntimePodToPending(context.Background(), deletedPodFor("conv-active"))
	if len(reqs) != 1 || reqs[0].Name != "conv-pending" {
		t.Fatalf("a freed conversation slot must wake the pending conversation only, got %+v", reqs)
	}
}

// The wake-up caps at the WOKEN pool's own ceiling, never the other pool's
// (design D-4): three pending members against a members cap of 1 wakes
// exactly one, the oldest.
func TestPodDeletionWakeUpCapsAtItsOwnPoolCeiling(t *testing.T) {
	memberActive := claimTestConv("member-active")
	memberActive.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1"}

	oldest := pendingConv("member-oldest", true, 30*time.Second)
	middle := pendingConv("member-middle", true, 20*time.Second)
	newest := pendingConv("member-newest", true, 10*time.Second)

	c := fakeClaimClient(t, memberActive, oldest, middle, newest).Build()
	r := &ConversationReconciler{
		Client: c, Conversations: claimStore(c),
		Namespaces:             namespace.Namespaces{Control: claimTestNS, Conversations: claimTestNS},
		MaxActiveConversations: 5, MaxActiveMembers: 1,
	}

	reqs := r.MapRuntimePodToPending(context.Background(), deletedPodFor("member-active"))
	if len(reqs) != 1 || reqs[0].Name != "member-oldest" {
		t.Fatalf("want exactly the oldest pending member woken (cap 1), got %+v", reqs)
	}
}

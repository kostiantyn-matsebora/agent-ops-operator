package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// pool names one of the two independent capacity pools
// (separate-member-slot-pool): conversations is the uncaused kind, members a
// Coordinator's own invoke. Admission, FIFO ordering, idle eviction and the
// freed-slot wake-up are all scoped to one pool — a full pool never blocks
// the other, and eviction never crosses between them (design D-3).
type pool string

const (
	poolConversations pool = "conversations"
	poolMembers       pool = "members"
)

// poolOf classifies a conversation record by provenance alone (design D-1):
// CausedBy set is a Coordinator member, everything else — including a
// record this call could not read, err != nil — counts as a conversation.
// That is the conservative reading: it never invents capacity for members.
func poolOf(conv state.ConversationRecord, err error) pool {
	if err == nil && conv.CausedBy != nil {
		return poolMembers
	}
	return poolConversations
}

// poolOfID looks up the conversation a pod's ID label names and classifies
// it — the same per-pod lookup eviction already did, now also deciding
// which pool the pod counts against. An absent or unknown ID reads as a
// conversation (poolOf's conservative reading).
func (r *ConversationReconciler) poolOfID(ctx context.Context, id string) pool {
	if id == "" {
		return poolConversations
	}
	list, err := r.Conversations.List(ctx, state.ByID, id)
	if err != nil || len(list) == 0 {
		return poolConversations
	}
	return poolOf(list[0], nil)
}

// capFor is a pool's own ceiling. The two are never pooled together — no
// comparison anywhere adds them (invariants.md, design D-2).
func (r *ConversationReconciler) capFor(p pool) int {
	if p == poolMembers {
		return r.MaxActiveMembers
	}
	return r.MaxActiveConversations
}

// splitPools divides live runtime pods into their two independent pools.
// liveRuntimePods itself stays pool-blind; every capacity decision goes
// through this helper instead, so no path keeps comparing against one
// shared count (design D-2).
func (r *ConversationReconciler) splitPools(ctx context.Context, live []corev1.Pod) map[pool][]corev1.Pod {
	out := map[pool][]corev1.Pod{poolConversations: nil, poolMembers: nil}
	for i := range live {
		p := r.poolOfID(ctx, live[i].Labels[runtimepod.LabelConversationID])
		out[p] = append(out[p], live[i])
	}
	return out
}

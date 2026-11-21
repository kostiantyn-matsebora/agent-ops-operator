package controller

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// slotHoldTTL is how long a just-made capacity decision is remembered while the
// pod list (a cache) catches up with it.
const slotHoldTTL = 30 * time.Second

// slots is the one thing conversations share: the active-conversation cap.
// Reconciles of different conversations run in parallel, so the decision "is
// there room" and the act that uses the room must be one step, and a decision
// taken moments ago must still count while the cached pod list cannot show it
// yet.
type slots struct {
	mu      sync.Mutex
	claimed map[string]time.Time    // conversation -> a pod about to exist
	evicted map[types.UID]time.Time // pod UID -> deleted, may still be listed
}

// occupancy is the pods that hold a slot now: those listed and not already
// evicted, plus the claims whose pod is not listed yet. Caller holds mu.
func (s *slots) occupancy(live []corev1.Pod, now time.Time) (pods []corev1.Pod, claimedOnly int) {
	listed := map[string]bool{}
	for _, p := range live {
		listed[p.Labels[runtimepod.LabelConversation]] = true
		if _, gone := s.evicted[p.UID]; gone {
			continue
		}
		pods = append(pods, p)
	}
	for name, at := range s.claimed {
		if listed[name] || now.Sub(at) > slotHoldTTL {
			delete(s.claimed, name)
			continue
		}
		claimedOnly++
	}
	for name, at := range s.evicted {
		if now.Sub(at) > slotHoldTTL {
			delete(s.evicted, name)
		}
	}
	return pods, claimedOnly
}

// reserveSlot decides, atomically with recording the decision, whether this
// conversation may take a slot, evicting an idle worker when that is what a
// slot costs. release undoes the claim when the pod could not be made.
func (r *ConversationReconciler) reserveSlot(ctx context.Context, conv string) (ok bool, release func(), err error) {
	r.slots.mu.Lock()
	defer r.slots.mu.Unlock()
	if r.slots.claimed == nil {
		r.slots.claimed, r.slots.evicted = map[string]time.Time{}, map[types.UID]time.Time{}
	}
	live, err := r.liveRuntimePods(ctx, r.Namespaces.Conversations)
	if err != nil {
		return false, nil, err
	}
	pods, claimed := r.slots.occupancy(live, time.Now())
	if _, mine := r.slots.claimed[conv]; !mine && len(pods)+claimed >= r.MaxActiveConversations {
		victim, made, err := r.evictIdleWorker(ctx, conv, pods)
		if !made {
			return false, nil, err
		}
		r.slots.evicted[victim] = time.Now()
	}
	r.slots.claimed[conv] = time.Now()
	return true, func() {
		r.slots.mu.Lock()
		delete(r.slots.claimed, conv)
		r.slots.mu.Unlock()
	}, nil
}

// freeSlots is how many slots are free right now, as admission sees them.
func (r *ConversationReconciler) freeSlots(ctx context.Context) (live []corev1.Pod, free int, err error) {
	r.slots.mu.Lock()
	defer r.slots.mu.Unlock()
	if r.slots.claimed == nil {
		r.slots.claimed, r.slots.evicted = map[string]time.Time{}, map[types.UID]time.Time{}
	}
	all, err := r.liveRuntimePods(ctx, r.Namespaces.Conversations)
	if err != nil {
		return nil, 0, err
	}
	pods, claimed := r.slots.occupancy(all, time.Now())
	return pods, r.MaxActiveConversations - len(pods) - claimed, nil
}

// awaitVisible waits, briefly, until the pod just created shows up in the
// pod list, then drops the claim: from then on the list itself counts it, and a
// pod deleted straight after must free its slot at once rather than linger as
// a claim.
func (r *ConversationReconciler) awaitVisible(ctx context.Context, conv string, release func()) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		live, err := r.liveRuntimePods(ctx, r.Namespaces.Conversations)
		if err != nil {
			return // the claim ages out on its own
		}
		for _, p := range live {
			if p.Labels[runtimepod.LabelConversation] == conv {
				release()
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

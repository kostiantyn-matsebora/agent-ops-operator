package controller

import (
	"context"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// slotHoldTTL is how long a just-made capacity decision is remembered while the
// pod list (a cache) catches up with it.
const slotHoldTTL = 30 * time.Second

// slots is the one thing conversations of a pool share: its own
// active-conversation cap. Reconciles of different conversations run in
// parallel, so the decision "is there room" and the act that uses the room
// must be one step, and a decision taken moments ago must still count while
// the cached pod list cannot show it yet.
//
// Both maps are keyed by pool FIRST (separate-member-slot-pool design D-2):
// a claim or an eviction only ever competes against its own pool's ceiling,
// never the other's. The claim map is keyed by the conversation's ID, not its
// Name: a pod lingering under a deleted conversation's Name must never be read
// as the slot of a new conversation created under that same Name
// (conversation-identity).
type slots struct {
	mu      sync.Mutex
	claimed map[pool]map[string]time.Time    // pool -> conversation ID -> a pod about to exist
	evicted map[pool]map[types.UID]time.Time // pool -> pod UID -> deleted, may still be listed
}

// ensure lazily initializes both pools' maps. Caller holds mu.
func (s *slots) ensure() {
	if s.claimed == nil {
		s.claimed = map[pool]map[string]time.Time{poolConversations: {}, poolMembers: {}}
		s.evicted = map[pool]map[types.UID]time.Time{poolConversations: {}, poolMembers: {}}
	}
}

// occupancy is the pods that hold a slot now in pool p: those listed and not
// already evicted, plus the claims whose pod is not listed yet. Caller holds
// mu. live is already scoped to p (splitPools).
func (s *slots) occupancy(p pool, live []corev1.Pod, now time.Time) (pods []corev1.Pod, claimedOnly int) {
	listed := map[string]bool{}
	for _, pd := range live {
		listed[pd.Labels[runtimepod.LabelConversationID]] = true
		if _, gone := s.evicted[p][pd.UID]; gone {
			continue
		}
		pods = append(pods, pd)
	}
	for id, at := range s.claimed[p] {
		if listed[id] || now.Sub(at) > slotHoldTTL {
			delete(s.claimed[p], id)
			continue
		}
		claimedOnly++
	}
	for uid, at := range s.evicted[p] {
		if now.Sub(at) > slotHoldTTL {
			delete(s.evicted[p], uid)
		}
	}
	return pods, claimedOnly
}

// reserveSlot decides, atomically with recording the decision, whether this
// conversation may take a slot IN ITS OWN POOL, evicting an idle worker of
// that same pool when that is what a slot costs (design D-3: eviction never
// crosses pools). release undoes the claim when the pod could not be made.
// The claim is keyed by the conversation's ID, matching the pod label
// occupancy reads.
func (r *ConversationReconciler) reserveSlot(ctx context.Context, conv *state.ConversationRecord) (ok bool, release func(), err error) {
	p := poolOf(*conv, nil)
	r.slots.mu.Lock()
	defer r.slots.mu.Unlock()
	r.slots.ensure()
	all, err := r.liveRuntimePods(ctx, r.Namespaces.Conversations)
	if err != nil {
		return false, nil, err
	}
	live := r.splitPools(ctx, all)[p]
	pods, claimed := r.slots.occupancy(p, live, time.Now())
	if _, mine := r.slots.claimed[p][conv.ID]; !mine && len(pods)+claimed >= r.capFor(p) {
		victim, made, err := r.evictIdleWorker(ctx, conv.ID, p, pods)
		if !made {
			return false, nil, err
		}
		r.slots.evicted[p][victim] = time.Now()
	}
	r.slots.claimed[p][conv.ID] = time.Now()
	return true, func() {
		r.slots.mu.Lock()
		delete(r.slots.claimed[p], conv.ID)
		r.slots.mu.Unlock()
	}, nil
}

// freeSlots is how many slots are free right now in pool p, as admission
// sees it.
func (r *ConversationReconciler) freeSlots(ctx context.Context, p pool) (live []corev1.Pod, free int, err error) {
	r.slots.mu.Lock()
	defer r.slots.mu.Unlock()
	r.slots.ensure()
	all, err := r.liveRuntimePods(ctx, r.Namespaces.Conversations)
	if err != nil {
		return nil, 0, err
	}
	own := r.splitPools(ctx, all)[p]
	pods, claimed := r.slots.occupancy(p, own, time.Now())
	return pods, r.capFor(p) - len(pods) - claimed, nil
}

// awaitVisible waits, briefly, until the pod just created shows up in the
// pod list, then drops the claim: from then on the list itself counts it, and a
// pod deleted straight after must free its slot at once rather than linger as
// a claim. convID is the conversation's ID, matching the pod's own label.
func (r *ConversationReconciler) awaitVisible(ctx context.Context, convID string, release func()) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		live, err := r.liveRuntimePods(ctx, r.Namespaces.Conversations)
		if err != nil {
			return // the claim ages out on its own
		}
		for _, p := range live {
			if p.Labels[runtimepod.LabelConversationID] == convID {
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

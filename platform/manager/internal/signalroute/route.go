// Package signalroute is the shared claimant-snapshot-and-route step (design
// D-3/D-4 of the signal-persistence change): one function both ingest's
// inline first routing attempt (internal/httpapi) and, later, the Signal
// reconciler's retries (internal/controller) call unchanged, so the two
// callers can never disagree about what a given claimant set means.
//
// It operates purely on a Signal CR's own spec/status — no in-memory routing
// state, matching design D-1/D-3's "no routing decision comes from process
// memory that was not just read from a Signal CR". Everything upstream of
// this package (persisting or absorbing the Signal, the pre-fan-out
// unclaimed/cooldown/capacity decision) stays the caller's: by the time
// RouteClaimants runs, the Signal already exists and is known not to be
// Dropped.
package signalroute

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/activity"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
)

// Deps bundles what RouteClaimants needs from its caller — the same shape a
// future reconciler supplies from its own set of dependencies, so the
// inline first attempt and a retry share code rather than agree by
// convention.
type Deps struct {
	// Client is used for every write: conversation create/patch, input
	// create, and the Signal's own status patch.
	Client client.Client
	// Reader serves the reads a routing attempt needs (claimant list,
	// reusable-conversation lookup, backlog count). A strong/APIReader in
	// production, same posture dispatch state elsewhere in the manager
	// takes.
	Reader client.Reader
	// Namespace every object above lives in — the manager is single-
	// namespace per install.
	Namespace string
	// Activity records conversation-created/input-queued hops. A nil log is
	// inert (internal/activity's own contract), so a caller that builds Deps
	// without one need not special-case it.
	Activity *activity.Log
	// Runtime is the bootstrap execution config SnapshotFor falls back to.
	Runtime runtimepod.Config
	// MaxQueued bounds the pending-conversation backlog a NEW conversation
	// may be created against. A claimant's create is refused (left Routing,
	// never Failed) while the backlog is at this bound — design D-8/
	// conversation-capacity's "the pending backlog is bounded".
	MaxQueued int
}

// RouteClaimants is the claimant-snapshot-and-route step (design D-4):
//
//   - If sig.Status.Claimants is empty, it is snapshotted ONCE from the
//     SignalSource's CURRENT servers (chat.PipelinesForSource) — one
//     SignalClaimant per claimant, state Routing. The snapshot never grows
//     or shrinks after this call returns: a later re-wire of who serves the
//     source never changes an in-flight signal's own fan-out.
//   - Every claimant currently Routing (and ONLY those — a terminal
//     claimant is never touched again) is attempted: reuse an open
//     conversation it already originated whose conversationId equals the
//     signal's initiationId, or create one. A create the pending backlog
//     refuses leaves that claimant Routing rather than failing it (design
//     D-8) — the caller is expected to call this again later (ingest's own
//     next absorbed arrival, or the reconciler's retry). A claimant no
//     longer served by the source is Skipped — the one condition this
//     change defines for that state.
//   - status.phase is recomputed via agentopsv1alpha1.RollupPhase after
//     every attempt, and the whole status (claimants, phase,
//     firstAttemptAt) is persisted via one Status().Patch.
//
// Idempotent by construction: a claimant already Routed or Skipped is never
// revisited, and the equality-reuse lookup is what makes re-finding an
// already-created conversation safe rather than a duplicate.
func RouteClaimants(ctx context.Context, deps Deps, sig *agentopsv1alpha1.Signal) error {
	before := sig.DeepCopy()

	if len(sig.Status.Claimants) == 0 {
		snapshotClaimants(ctx, deps, sig)
	}

	live := liveClaimantsByRef(ctx, deps, sig.Spec.SourceRef.Name)
	for i := range sig.Status.Claimants {
		c := &sig.Status.Claimants[i]
		if c.State != agentopsv1alpha1.ClaimantRouting {
			continue
		}
		claimant, ok := live[c.Ref]
		if !ok {
			c.State = agentopsv1alpha1.ClaimantSkipped
			c.Reason = "claimant no longer serves this source"
			continue
		}
		convRef, waiting, err := routeOneClaimant(ctx, deps, claimant, sig)
		if err != nil {
			return fmt.Errorf("route claimant %s/%s: %w", c.Ref.Kind, c.Ref.Name, err)
		}
		if waiting {
			continue // backlog full: stays Routing, retried later
		}
		c.State = agentopsv1alpha1.ClaimantRouted
		c.ConversationRef = convRef
	}

	sig.Status.Phase = agentopsv1alpha1.RollupPhase(sig.Status.Claimants)
	patch := client.MergeFrom(before)
	return deps.Client.Status().Patch(ctx, sig, patch)
}

// snapshotClaimants records the claimant set a Signal fans out to, exactly
// once, and stamps FirstAttemptAt — the origin the retry deadline
// (SIGNAL_RETRY_DEADLINE, a later task) measures from.
func snapshotClaimants(ctx context.Context, deps Deps, sig *agentopsv1alpha1.Signal) {
	claimants := chat.PipelinesForSource(ctx, deps.Reader, deps.Namespace, sig.Spec.SourceRef.Name)
	snapshot := make([]agentopsv1alpha1.SignalClaimant, 0, len(claimants))
	for _, c := range claimants {
		snapshot = append(snapshot, agentopsv1alpha1.SignalClaimant{
			Ref:   claimantRef(c),
			State: agentopsv1alpha1.ClaimantRouting,
		})
	}
	sig.Status.Claimants = snapshot
	now := metav1.Now()
	sig.Status.FirstAttemptAt = &now
}

// claimantRef renders a chat.Claimant's own agentopsv1alpha1.ClaimantRef —
// the two ClaimantKind vocabularies differ in case (chat's is lowercase,
// the CRD's is capitalized) because api/v1alpha1 is self-contained and
// cannot import internal/chat to share its type (the same reasoning
// SignalClaimant's own doc comment gives).
func claimantRef(c chat.Claimant) agentopsv1alpha1.ClaimantRef {
	kind := agentopsv1alpha1.ClaimantKindPipeline
	if c.ClaimantKind() == chat.ClaimantCoordinator {
		kind = agentopsv1alpha1.ClaimantKindCoordinator
	}
	return agentopsv1alpha1.ClaimantRef{Kind: kind, Name: c.GetName()}
}

// liveClaimantsByRef resolves the source's CURRENT servers, keyed by the
// same ClaimantRef a snapshot carries — what lets RouteClaimants tell a
// still-serving claimant from one whose Pipeline or Coordinator was
// deleted since the snapshot was taken.
func liveClaimantsByRef(ctx context.Context, deps Deps, sourceName string) map[agentopsv1alpha1.ClaimantRef]chat.Claimant {
	live := chat.PipelinesForSource(ctx, deps.Reader, deps.Namespace, sourceName)
	out := make(map[agentopsv1alpha1.ClaimantRef]chat.Claimant, len(live))
	for _, c := range live {
		out[claimantRef(c)] = c
	}
	return out
}

// BacklogFull reports whether the pending backlog has reached its bound.
// Only PENDING conversations count: an admitted one holds a pod (or is
// about to) and is already bounded by MAX_ACTIVE_CONVERSATIONS. Exported so
// a caller deciding a PRE-fan-out capacity drop (the chat lane, design D-8)
// can consult the same count this package's own create step gates on.
func BacklogFull(ctx context.Context, reader client.Reader, namespace string, maxQueued int) (bool, error) {
	var list agentopsv1alpha1.ConversationList
	if err := reader.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return false, err
	}
	n := 0
	for i := range list.Items {
		if list.Items[i].Status.Phase == agentopsv1alpha1.ConversationPending {
			n++
		}
	}
	return n >= maxQueued, nil
}

package chat

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/dispatch"
)

// ClaimantKind says which CRD a Claimant wraps, for a message or an event that
// must name it (design D-B).
type ClaimantKind string

const (
	ClaimantPipeline    ClaimantKind = "pipeline"
	ClaimantCoordinator ClaimantKind = "coordinator"
)

// Claimant is a wiring kind that may claim a signal source and be addressed by
// name from a chat surface — a Pipeline or a Coordinator, the two kinds
// `PipelinesForSource` and every caller that used to iterate `[]Pipeline` now
// iterate uniformly (design D-B).
//
// It embeds `dispatch.CapabilityHolder`, which both CRDs already satisfy: a
// Claimant's resolved capability is `dispatch.ResolveCapability(ctx, reader,
// claimant)`, and its name is `claimant.GetName()`. What a Claimant adds is
// the two properties a Pipeline and a Coordinator answer DIFFERENTLY — which
// kind it is, and what a conversation it opens binds at creation.
type Claimant interface {
	dispatch.CapabilityHolder
	// ClaimantKind says which kind this is.
	ClaimantKind() ClaimantKind
	// BoundChannelRefs are the channels a conversation this claimant opens
	// binds at CREATION. A Pipeline's own ChannelRefs; empty for a
	// Coordinator, whose `channelRefs` are reached only through `escalate`
	// (design D-D) — never at admission.
	BoundChannelRefs() []agentopsv1alpha1.ObjectRef
	// EscalationChannelRefs are snapshotted onto a new root conversation's
	// `spec.escalationChannelRefs` (design D-D) — nil for a Pipeline, which
	// escalates nothing.
	EscalationChannelRefs() []agentopsv1alpha1.ObjectRef
	// SnapshotBudget is the ConversationBudget a new conversation this
	// claimant opens should carry in status, or nil for a Pipeline — which
	// enforces none, so a Pipeline-rooted conversation carries no Budget at
	// all (design D-B, D-E).
	SnapshotBudget() *agentopsv1alpha1.ConversationBudget
}

// pipelineClaimant and coordinatorClaimant adapt the two CRDs to Claimant with
// no behaviour of their own, by embedding the pointer: every
// `dispatch.CapabilityHolder` method and every `client.Object` method comes
// along unchanged, and this file is the only place either CRD is read AS a
// claimant.
type pipelineClaimant struct {
	*agentopsv1alpha1.Pipeline
}

func (c pipelineClaimant) ClaimantKind() ClaimantKind { return ClaimantPipeline }

func (c pipelineClaimant) BoundChannelRefs() []agentopsv1alpha1.ObjectRef {
	return c.Spec.ChannelRefs
}

func (c pipelineClaimant) EscalationChannelRefs() []agentopsv1alpha1.ObjectRef { return nil }

func (c pipelineClaimant) SnapshotBudget() *agentopsv1alpha1.ConversationBudget { return nil }

type coordinatorClaimant struct {
	*agentopsv1alpha1.Coordinator
}

func (c coordinatorClaimant) ClaimantKind() ClaimantKind { return ClaimantCoordinator }

// BoundChannelRefs is always empty: a Coordinator's `channelRefs` are
// escalation targets, never bound at creation (design D-D, D-B).
func (c coordinatorClaimant) BoundChannelRefs() []agentopsv1alpha1.ObjectRef { return nil }

// EscalationChannelRefs snapshots the Coordinator's own `channelRefs` — a copy,
// so a later edit to the Coordinator never reaches a conversation already
// created (design D-D).
func (c coordinatorClaimant) EscalationChannelRefs() []agentopsv1alpha1.ObjectRef {
	return append([]agentopsv1alpha1.ObjectRef{}, c.Spec.ChannelRefs...)
}

// SnapshotBudget resolves `spec.limits` into the conversation's own budget,
// once: a zero field stays zero (unset, not zero — the ENFORCING code picks
// its own default, this is only the snapshot), and a relative deadline is
// resolved against NOW, since re-resolving it later would let the same
// duration mean a different moment depending on when it was read.
func (c coordinatorClaimant) SnapshotBudget() *agentopsv1alpha1.ConversationBudget {
	budget := &agentopsv1alpha1.ConversationBudget{}
	if limits := c.Spec.Limits; limits != nil {
		budget.MaxAgents = limits.MaxAgents
		budget.MaxTurns = limits.MaxTurns
		if limits.Deadline != nil {
			at := metav1.NewTime(time.Now().Add(limits.Deadline.Duration))
			budget.Deadline = &at
		}
	}
	return budget
}

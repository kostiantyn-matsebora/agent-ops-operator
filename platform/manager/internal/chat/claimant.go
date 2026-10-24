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
	// binds at CREATION — a Pipeline's own ChannelRefs, and (coordinator-
	// unconditional-channels) a Coordinator's own, UNCONDITIONALLY: any open
	// coordinator root is reachable by a human from the moment it exists,
	// exactly as a Pipeline-rooted conversation already is. `escalate` no
	// longer binds anything; it posts through a channel already open.
	BoundChannelRefs() []agentopsv1alpha1.ObjectRef
	// EscalationChannelRefs are snapshotted onto a new root conversation's
	// `spec.escalationChannelRefs` (design D-D) — nil for a Pipeline, which
	// escalates nothing. For a Coordinator this is now the SAME set
	// BoundChannelRefs returns (coordinator-unconditional-channels): kept as
	// its own field for provenance — "which channels did the Coordinator
	// itself declare" — distinct from `ChannelRefs`, which for a
	// chat-addressed root may additionally carry the addressing channel.
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

// BoundChannelRefs is the Coordinator's own `channelRefs`, bound at creation
// exactly like a Pipeline's (coordinator-unconditional-channels, superseding
// design D-D's "never bound at creation"): any open coordinator root must be
// reachable by a human whether or not its agent ever calls `escalate`.
func (c coordinatorClaimant) BoundChannelRefs() []agentopsv1alpha1.ObjectRef {
	return c.EscalationChannelRefs()
}

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

package runtimepod

import (
	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// Volume names one of the two things a conversation persists. The string is
// the claim-name suffix as well as the label, so the two cannot drift.
type Volume string

const (
	// VolumeContext: a conversation's accumulated context.
	VolumeContext Volume = "context"
	// VolumeWorkspace: the repository checkout, one subdirectory per
	// conversation.
	VolumeWorkspace Volume = "workspace"
)

// PipelineClaimName is the claim the MANAGER renders for a Pipeline binding
// that names a PersistentVolume.
//
// Derived rather than declared, and per (pipeline, volume), so that two routes
// naming two volumes never collide and re-reconciling one is idempotent. It is
// not a Pipeline field because an operator choosing the claim name would be
// choosing an object the manager then has to adopt or refuse.
func PipelineClaimName(pipeline string, vol Volume) string {
	return "agentops-" + pipeline + "-" + string(vol)
}

// ResolveClaim answers WHICH CLAIM one volume of one route resolves to, and it
// is the only function that answers it.
//
// THE CHAIN, and no other order:
//
//	pipeline.spec.persistence.<vol>  ->  releaseDefault  ->  "" (ephemeral)
//
// THE RUNTIME IS IN NO PART OF IT. An AgentRuntime declares neither volume, so
// there is nothing of a runtime's to fall through to — which is the whole point
// of the move: two Pipelines sharing one runtime resolve differently.
//
// releaseDefault is the chart's release-wide claim as it reaches the manager in
// its bootstrap configuration — the chart's own claim, an `existingClaim` an
// operator named, or empty where persistence is off. The manager does not know
// which of those it is looking at and does not need to: all three are already a
// claim name by the time they arrive.
//
// Called at conversation CREATION to write the snapshot, and by anything
// DISPLAYING a route's storage. Two callers computing this separately is two
// answers to "where does this route persist", differing at the moment it
// matters.
func ResolveClaim(pipelineName string, vol Volume,
	b *agentopsv1alpha1.PersistenceBinding, releaseDefault string) string {

	if b != nil {
		// A claim that already exists is mounted as named — nothing is created.
		if b.ClaimName != "" {
			return b.ClaimName
		}
		// A VOLUME is not mountable. The manager renders the claim on it, under
		// the derived name, and that name is what the conversation freezes.
		if b.VolumeName != "" && pipelineName != "" {
			return PipelineClaimName(pipelineName, vol)
		}
	}
	return releaseDefault
}

// ResolvePersistence resolves BOTH volumes of one route against the release
// defaults, returning the pair a Conversation snapshots.
//
// A nil Pipeline is the case where nothing originated from wiring at all, and
// it takes the release defaults exactly as an unbound route does.
func ResolvePersistence(pipeline *agentopsv1alpha1.Pipeline,
	defaults Config) (contextClaim, workspaceClaim string) {

	var name string
	var p *agentopsv1alpha1.PipelinePersistence
	if pipeline != nil {
		name = pipeline.Name
		p = pipeline.Spec.Persistence
	}
	var ctxBinding, wsBinding *agentopsv1alpha1.PersistenceBinding
	if p != nil {
		ctxBinding, wsBinding = p.Context, p.Workspace
	}
	return ResolveClaim(name, VolumeContext, ctxBinding, defaults.ContextPVC),
		ResolveClaim(name, VolumeWorkspace, wsBinding, defaults.WorkspacePVC)
}

package dispatch

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// CapabilityHolder is a wiring kind that carries an AgentCapability either
// INLINE or by a `capabilityRef`. Pipeline implements it; Coordinator will
// implement it the same way.
type CapabilityHolder interface {
	client.Object
	// CapabilityRef names the AgentCapability this holder references INSTEAD
	// of inlining a capability, or nil when it inlines one.
	CapabilityRef() *agentopsv1alpha1.ObjectRef
	// InlineCapability returns the holder's own embedded capability fields.
	// Meaningless when CapabilityRef is non-nil — the two are mutually
	// exclusive by CEL.
	InlineCapability() agentopsv1alpha1.AgentCapabilitySpec
}

// ResolveCapability is the ONE place a wiring kind's capability is read.
// Every reader of a Pipeline's (or a Coordinator's) profile, runtime,
// identity, tools, MCP servers or persistence calls this rather than the
// embedded fields directly — an inline capability and a referenced
// AgentCapability must resolve identically, and this is what makes that true
// in one place instead of in every caller.
func ResolveCapability(ctx context.Context, reader client.Reader, holder CapabilityHolder) (agentopsv1alpha1.AgentCapabilitySpec, error) {
	ref := holder.CapabilityRef()
	if ref == nil {
		return holder.InlineCapability(), nil
	}
	var capability agentopsv1alpha1.AgentCapability
	if err := reader.Get(ctx, types.NamespacedName{Namespace: holder.GetNamespace(), Name: ref.Name}, &capability); err != nil {
		return agentopsv1alpha1.AgentCapabilitySpec{}, err
	}
	return capability.Spec, nil
}

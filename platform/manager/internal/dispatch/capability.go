package dispatch

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
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

// ToolsetBindingOf is a capability's toolset binding as a conversation record
// carries it: the mode and the toolset names, copied. Nil in, nil out.
func ToolsetBindingOf(b *agentopsv1alpha1.ToolsetBinding) *state.ToolsetBinding {
	if b == nil {
		return nil
	}
	return &state.ToolsetBinding{Mode: b.Mode, Refs: refNames(b.Refs)}
}

// ToolingBindingOf is a capability's MCPConfig binding as a conversation
// record carries it. Nil in, nil out.
func ToolingBindingOf(b *agentopsv1alpha1.ToolingBinding) *state.ToolingBinding {
	if b == nil {
		return nil
	}
	return &state.ToolingBinding{Refs: refNames(b.Refs)}
}

func refNames(refs []agentopsv1alpha1.ObjectRef) []state.Ref {
	out := make([]state.Ref, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}

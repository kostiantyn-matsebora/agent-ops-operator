package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// validateCapabilitySpecRefs checks every reference an AgentCapabilitySpec
// names for existence, returning one string per missing one. SHARED between
// the Pipeline reconciler (validating its own inline capability, or the
// AgentCapability its capabilityRef names) and the AgentCapability
// reconciler — one validator, so the two can never drift on what "the
// capability's refs resolve" means.
//
// It does NOT check ProfileRef is set: whether a capability without a profile
// is valid depends on the EMBEDDING kind (a Pipeline fails Ready naming
// neither capabilityRef nor profileRef; an AgentCapability meant only to
// carry tools for another AgentCapability to extend is a future shape this
// does not need to forbid today).
func validateCapabilitySpecRefs(ctx context.Context, c client.Reader, namespace string,
	spec agentopsv1alpha1.AgentCapabilitySpec) []string {

	var missing []string
	missing = append(missing, checkProfileRef(ctx, c, namespace, spec.ProfileRef)...)
	missing = append(missing, checkRuntimeRef(ctx, c, namespace, spec.RuntimeRef)...)
	missing = append(missing, checkToolsetRefs(ctx, c, namespace, spec.Toolsets)...)
	missing = append(missing, checkMCPConfigRefs(ctx, c, namespace, spec.MCPConfigs)...)
	return missing
}

func checkProfileRef(ctx context.Context, c client.Reader, namespace string, ref *agentopsv1alpha1.ObjectRef) []string {
	if ref == nil {
		return nil
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &profile); err != nil {
		return []string{"agentprofile/" + ref.Name}
	}
	return nil
}

// checkRuntimeRef is checked only when NAMED. Absent, it resolves to the
// AgentRuntime called "default" through runtimepod's own precedence chain —
// that is not a miss, and probing for "default" here would validate a name
// this capability never wrote.
func checkRuntimeRef(ctx context.Context, c client.Reader, namespace string, ref *agentopsv1alpha1.ObjectRef) []string {
	if ref == nil {
		return nil
	}
	var rt agentopsv1alpha1.AgentRuntime
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &rt); err != nil {
		return []string{"agentruntime/" + ref.Name}
	}
	return nil
}

// checkToolsetRefs and checkMCPConfigRefs check refs only — the CRs' content
// is resolved at use time, so Ready checks existence, nothing else.
func checkToolsetRefs(ctx context.Context, c client.Reader, namespace string, toolsets *agentopsv1alpha1.ToolsetBinding) []string {
	if toolsets == nil {
		return nil
	}
	var missing []string
	for _, ref := range toolsets.Refs {
		var ts agentopsv1alpha1.MCPToolset
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &ts); err != nil {
			missing = append(missing, "mcptoolset/"+ref.Name)
		}
	}
	return missing
}

func checkMCPConfigRefs(ctx context.Context, c client.Reader, namespace string, mcpConfigs *agentopsv1alpha1.ToolingBinding) []string {
	if mcpConfigs == nil {
		return nil
	}
	var missing []string
	for _, ref := range mcpConfigs.Refs {
		var mc agentopsv1alpha1.MCPConfig
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, &mc); err != nil {
			missing = append(missing, "mcpconfig/"+ref.Name)
		}
	}
	return missing
}

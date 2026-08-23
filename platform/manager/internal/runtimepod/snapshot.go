package runtimepod

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// SnapshotFor resolves a Pipeline's execution wiring into the pair a new
// Conversation freezes: the RUNTIME NAME and the SERVICE ACCOUNT.
//
// ONE function for every origination path — the signal lane and the chat
// command both call it — because two paths reading the same Pipeline for the
// same fields is two chances to drift, and the thing they would drift on is
// which identity an agent runs as.
//
// THE SPLIT BETWEEN THEM IS THE REF/CONTENT RULE, AND IT IS NOT SYMMETRIC:
//
//   - The RUNTIME is a REF, so its resolution is frozen here. A conversation
//     created while its Pipeline named no runtime keeps the one it actually ran
//     on, and a later edit to that Pipeline — or to the deprecated profile ref
//     below it — moves only conversations created afterwards.
//   - The SERVICE ACCOUNT is frozen ONLY when the PIPELINE named one. Absent,
//     it stays empty and resolution falls through to the runtime's own account
//     at every pod build — because that account is the AgentRuntime's CONTENT,
//     and correcting it must heal running conversations exactly as correcting
//     an image does. Freezing it here would strand every existing conversation
//     on a name the operator has already fixed.
//
// Neither is read from the Pipeline again after this point. That is the whole
// guarantee: editing a Pipeline cannot change what identity an INFLIGHT
// conversation's next pod runs as.
func SnapshotFor(ctx context.Context, r client.Reader, namespace string,
	pipeline *agentopsv1alpha1.Pipeline) (*agentopsv1alpha1.ObjectRef, string) {

	if pipeline == nil {
		return nil, ""
	}
	sa := pipeline.Spec.ServiceAccountName

	if pipeline.Spec.RuntimeRef != nil && pipeline.Spec.RuntimeRef.Name != "" {
		return &agentopsv1alpha1.ObjectRef{Name: pipeline.Spec.RuntimeRef.Name}, sa
	}
	// DEPRECATED, one release: a profile applied before the upgrade named the
	// runtime, and freezing that name here is what stops a later profile edit
	// moving a conversation already running. Delete with
	// AgentProfileSpec.RuntimeRef.
	if pipeline.Spec.ProfileRef.Name != "" {
		var profile agentopsv1alpha1.AgentProfile
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: namespace, Name: pipeline.Spec.ProfileRef.Name}, &profile); err == nil {
			if profile.Spec.RuntimeRef != nil && profile.Spec.RuntimeRef.Name != "" {
				return &agentopsv1alpha1.ObjectRef{Name: profile.Spec.RuntimeRef.Name}, sa
			}
		}
	}
	// `default` is only nameable when it EXISTS. Snapshotting a name nothing
	// backs would turn the soft bootstrap fallback into a hard failure — a
	// named runtime must exist, and this one would not.
	var rt agentopsv1alpha1.AgentRuntime
	if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "default"}, &rt); err == nil {
		return &agentopsv1alpha1.ObjectRef{Name: rt.Name}, sa
	}
	// Nothing to name: the manager's bootstrap config answers, and it is not a
	// CR. An empty ref here resolves exactly as it always did.
	return nil, sa
}

package runtimepod

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// ResolveActingCoordinator is chat.Router.ResolveActingCoordinator's
// Name-only half, DUPLICATED rather than imported — chat already imports
// runtimepod (it builds a member's persistence snapshot), so the reverse
// import would cycle, the same shape DeriveCoordinatorToken is duplicated in
// mcptoken.go for. Exported so internal/controller, the one package that can
// import both, can pin the two against each other.
//
// It exists so ResolveFor — the one place in the pod-build path holding a
// Reader — can give a MEMBER conversation's pod (no coordinatorRef of its
// own, like the self-heal reaper) a usable AOPS_MCP_TOKEN: design D-A's walk
// reads the conversation's own coordinatorRef first, and otherwise walks
// causedBy to the UNCAUSED root and reads THAT root's coordinatorRef
// instead. RootName (needed only by the manager's bound checks on
// list_open_roots / the widened close) is deliberately not resolved here —
// Build only ever needs the Coordinator's NAME to derive a token.
func ResolveActingCoordinator(ctx context.Context, r client.Reader, namespace string, conv *agentopsv1alpha1.Conversation) (string, error) {
	if conv.Spec.CoordinatorRef != nil {
		return conv.Spec.CoordinatorRef.Name, nil
	}
	cur := conv
	for i := 0; i < 1000; i++ {
		if cur.Spec.CausedBy == nil {
			if cur.Spec.CoordinatorRef != nil {
				return cur.Spec.CoordinatorRef.Name, nil
			}
			return "", nil
		}
		var parent agentopsv1alpha1.Conversation
		if err := r.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cur.Spec.CausedBy.Parent}, &parent); err != nil {
			return "", err
		}
		cur = &parent
	}
	return "", fmt.Errorf("causedBy chain from %s did not terminate", conv.Name)
}

// resolveActingCoordinatorBestEffort is ResolveActingCoordinator for
// ResolveFor's own use: a broken or unreadable ancestry chain answers "" (no
// token) rather than failing an otherwise-ordinary pod build over a token
// this conversation may not even present.
func resolveActingCoordinatorBestEffort(ctx context.Context, r client.Reader, namespace string, conv *agentopsv1alpha1.Conversation) string {
	name, err := ResolveActingCoordinator(ctx, r, namespace, conv)
	if err != nil {
		return ""
	}
	return name
}

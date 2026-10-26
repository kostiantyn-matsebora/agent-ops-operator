package chat

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// MoveRuleName names the rule behind the one manager-origin input that exists
// today (conversation-provenance): the corrective input a Coordinator root
// receives after a successful turn that made no move. It is the input's
// `origin.name`, so a reader of `status.runs[].inputs[]` sees which rule wrote
// it.
const MoveRuleName = "move-per-turn"

// MoveInputID is the stable id of the corrective input one run's miss appends,
// so a repeated `/work/done` for the same run appends it once.
func MoveInputID(conversation, runID string) string {
	return MoveRuleName + ":" + conversation + ":" + runID
}

// AppendManagerInput appends one input the MANAGER itself wrote to a named
// conversation, origin `{kind: manager, name: move-per-turn}` (design D-C),
// through the same idempotent append a member's result takes: a Closed
// conversation receives nothing, a repeat of an id still pending is a no-op.
// Delivery to the conversation's bound threads is the reconciler's, by the
// existing per-destination rule — nothing here posts.
func (r *Router) AppendManagerInput(ctx context.Context, convName, inputID, payload string) error {
	item := agentopsv1alpha1.InputItem{
		ID: inputID, Type: agentopsv1alpha1.InputTask, Payload: payload, ReceivedAt: metav1.Now(),
		Origin: &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginManager, Name: MoveRuleName},
	}
	return r.appendInputIdempotent(ctx, convName, item)
}

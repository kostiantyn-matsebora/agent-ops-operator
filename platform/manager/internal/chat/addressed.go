package chat

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/addressing"
)

// AnswersInPlace reports whether a parsed chat command is one whose whole
// result is a reply: the listing, /close and /exit on a general surface, or a
// bare /<name> that asks for its task. Such a command opens and continues
// nothing, so it is not a message to queue — /signal/inbound hands it to
// Router.HandleCommand and stores no Signal.
//
// Everything else — an addressed /<name> <task> — is a message like any
// other, published as a Signal and routed by the consumer.
func AnswersInPlace(cmd addressing.Command) bool {
	return isListCommand(cmd.Pipeline) ||
		cmd.Pipeline == CloseCommand ||
		cmd.Pipeline == ExitCommand ||
		cmd.Rest == ""
}

// ResolveClaimant looks an addressed name up as a Pipeline, then a
// Coordinator: a plain Get by name, no claim check and no Ready check. A nil
// Claimant with a nil error means neither exists.
func ResolveClaimant(ctx context.Context, reader client.Reader, namespace, name string) (Claimant, error) {
	for _, kind := range []ClaimantKind{ClaimantPipeline, ClaimantCoordinator} {
		c, err := ClaimantByKind(ctx, reader, namespace, kind, name)
		if err != nil || c != nil {
			return c, err
		}
	}
	return nil, nil
}

// ClaimantByKind reads one claimant by kind and name. A nil Claimant with a nil
// error means it does not exist — the consumer's Skipped condition.
func ClaimantByKind(ctx context.Context, reader client.Reader, namespace string, kind ClaimantKind, name string) (Claimant, error) {
	key := types.NamespacedName{Namespace: namespace, Name: name}
	if kind == ClaimantCoordinator {
		var co agentopsv1alpha1.Coordinator
		if err := reader.Get(ctx, key, &co); err != nil {
			return nil, ignoreNotFound(err)
		}
		return coordinatorClaimant{&co}, nil
	}
	var pipe agentopsv1alpha1.Pipeline
	if err := reader.Get(ctx, key, &pipe); err != nil {
		return nil, ignoreNotFound(err)
	}
	return pipelineClaimant{&pipe}, nil
}

func ignoreNotFound(err error) error {
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

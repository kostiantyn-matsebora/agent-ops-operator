package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// SetupConversationIndexes registers the informer field index equality
// routing (internal/httpapi), reopen's at-most-one-open check and member
// reuse (internal/chat) all read: agentopsv1alpha1.IndexConversationID on
// Conversation.spec.conversationId.
//
// MUST run before the manager's cache starts serving reads — an index
// registered after Start is a startup error — which is why main.go calls
// this immediately after building the manager, ahead of every
// SetupWithManager call. Accepts the bare client.FieldIndexer interface
// rather than ctrl.Manager so a test harness can register the same index on
// a standalone cache.Cache.
func SetupConversationIndexes(idx client.FieldIndexer) error {
	return idx.IndexField(context.Background(), &agentopsv1alpha1.Conversation{},
		agentopsv1alpha1.IndexConversationID, ConversationIDIndexer)
}

// ConversationIDIndexer extracts the index key: the conversation's own
// correlation id, or no key at all for one that predates the field — a
// legacy conversation carries nothing to match and stays correctly
// invisible to every equality lookup (conversation-provenance: "never
// backfilled"). Exported so a fake.ClientBuilder.WithIndex in a test that
// cannot run an informer cache (internal/httpapi's own unit tests) can
// register the identical extractor rather than a drifting copy.
func ConversationIDIndexer(obj client.Object) []string {
	conv := obj.(*agentopsv1alpha1.Conversation)
	if conv.Spec.ConversationID == "" {
		return nil
	}
	return []string{conv.Spec.ConversationID}
}

// SetupSignalIndexes registers the informer field index absorb-by-
// initiationId (internal/httpapi's persist-then-ack step, and the Signal
// reconciler's own lookups) reads: agentopsv1alpha1.IndexSignalInitiationID
// on a composite source+initiationId key.
//
// MUST run before the manager's cache starts serving reads, exactly like
// SetupConversationIndexes above.
func SetupSignalIndexes(idx client.FieldIndexer) error {
	return idx.IndexField(context.Background(), &agentopsv1alpha1.Signal{},
		agentopsv1alpha1.IndexSignalInitiationID, SignalInitiationIDIndexer)
}

// SignalInitiationIDIndexer extracts the index key: the composite
// source+initiationId key every Signal carries from creation (both fields
// are required, so there is no "legacy object carries neither" case the way
// ConversationIDIndexer has to allow for). Exported so a fake.ClientBuilder
// in a test that cannot run an informer cache can register the identical
// extractor.
func SignalInitiationIDIndexer(obj client.Object) []string {
	sig := obj.(*agentopsv1alpha1.Signal)
	if sig.Spec.InitiationID == "" || sig.Spec.SourceRef.Name == "" {
		return nil
	}
	return []string{agentopsv1alpha1.SignalInitiationIndexKey(sig.Spec.SourceRef.Name, sig.Spec.InitiationID)}
}

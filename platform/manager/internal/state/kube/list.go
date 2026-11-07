package kube

import (
	"context"
	"fmt"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// List serves every state.DeclaredIndexes member with one full list of the
// namespace's Conversations, filtered in Go. The list comes from the apiserver
// unless WithCachedLists gave the store a cache, which production does: a full
// list of a live install (552 conversations, 6 MB) took over half a second, and
// closing one conversation walks the child lookup twice.
func (s *Store) List(ctx context.Context, index state.Index, key state.ListKey) ([]state.ConversationRecord, error) {
	if !isDeclaredIndex(index) {
		return nil, state.ErrUnknownIndex
	}

	var list agentopsv1alpha1.ConversationList
	if err := s.listReader().List(ctx, &list, client.InNamespace(s.namespace)); err != nil {
		return nil, fmt.Errorf("kube: list conversations: %w", err)
	}

	now := time.Now()
	var out []state.ConversationRecord
	for i := range list.Items {
		conv := &list.Items[i]
		if isExpired(conv, now) {
			s.expiryArmed.Store(true)
			continue
		}
		if !matchesIndex(conv, index, key) {
			continue
		}
		out = append(out, toRecord(conv))
	}
	return out, nil
}

func isDeclaredIndex(index state.Index) bool {
	for _, declared := range state.DeclaredIndexes {
		if declared == index {
			return true
		}
	}
	return false
}

// matchesIndex implements every member of state.DeclaredIndexes against
// one Conversation. Adding an Index to store.go without a case here is
// caught by internal/state/conformance walking the same declared slice
// (store.go's own comment on DeclaredIndexes), not by this function, which
// simply has nothing to match and returns false.
func matchesIndex(conv *agentopsv1alpha1.Conversation, index state.Index, key state.ListKey) bool {
	switch index {
	case state.ByConversationID:
		return conv.Spec.ConversationID == key.ConversationID
	case state.ByPipelineAndPhase:
		return conv.Spec.PipelineRef != nil &&
			conv.Spec.PipelineRef.Name == key.PipelineRef &&
			conv.Status.Phase == agentopsv1alpha1.ConversationPhase(key.Phase)
	case state.ByPhase:
		return conv.Status.Phase == agentopsv1alpha1.ConversationPhase(key.Phase)
	case state.ByCausedBy:
		return conv.Spec.CausedBy != nil && conv.Spec.CausedBy.Parent == key.CausedBy
	case state.ByNameBase:
		return conv.Labels[agentopsv1alpha1.LabelNameBase] == key.NameBase
	case state.ByAll:
		return true
	default:
		return false
	}
}

package kube

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// PutBlob stores blob under key as a ConversationInput — upserting: an
// existing object is overwritten in place, a missing one is created
// (store.go's Store.PutBlob: "replacing any value already there").
func (s *Store) PutBlob(ctx context.Context, key state.Key, blob state.Blob) error {
	var ci agentopsv1alpha1.ConversationInput
	err := s.client.Get(ctx, s.namespacedName(key), &ci)
	switch {
	case err == nil:
		applyBlob(&ci, blob)
		if err := s.client.Update(ctx, &ci); err != nil {
			return fmt.Errorf("kube: update blob %q: %w", key, err)
		}
		return nil
	case apierrors.IsNotFound(err):
		ci = agentopsv1alpha1.ConversationInput{}
		ci.Name, ci.Namespace = string(key), s.namespace
		applyBlob(&ci, blob)
		if err := s.client.Create(ctx, &ci); err != nil {
			return fmt.Errorf("kube: create blob %q: %w", key, err)
		}
		return nil
	default:
		return fmt.Errorf("kube: get blob %q before put: %w", key, err)
	}
}

// GetBlob returns the blob stored under key, or ErrNotFound.
func (s *Store) GetBlob(ctx context.Context, key state.Key) (state.Blob, error) {
	var ci agentopsv1alpha1.ConversationInput
	if err := s.client.Get(ctx, s.namespacedName(key), &ci); err != nil {
		if apierrors.IsNotFound(err) {
			return state.Blob{}, state.ErrNotFound
		}
		return state.Blob{}, fmt.Errorf("kube: get blob %q: %w", key, err)
	}
	return toBlob(&ci), nil
}

// DeleteBlob removes a blob. Not an error when the key is already absent —
// "a deleted key reads absent" (store.go's Store.DeleteBlob), not an error.
func (s *Store) DeleteBlob(ctx context.Context, key state.Key) error {
	ci := &agentopsv1alpha1.ConversationInput{}
	ci.Name, ci.Namespace = string(key), s.namespace
	if err := s.client.Delete(ctx, ci); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("kube: delete blob %q: %w", key, err)
	}
	return nil
}

func applyBlob(ci *agentopsv1alpha1.ConversationInput, blob state.Blob) {
	ci.Spec.ConversationRef = agentopsv1alpha1.ObjectRef{Name: blob.ConversationName}
	ci.Spec.Type = agentopsv1alpha1.InputType(blob.Type)
	ci.Spec.Payload = blob.Payload
	ci.Spec.Labels = blob.Labels
	ci.Spec.Severity = agentopsv1alpha1.Severity(blob.Severity)
}

func toBlob(ci *agentopsv1alpha1.ConversationInput) state.Blob {
	return state.Blob{
		ConversationName: ci.Spec.ConversationRef.Name,
		Type:             string(ci.Spec.Type),
		Payload:          ci.Spec.Payload,
		Labels:           ci.Spec.Labels,
		Severity:         string(ci.Spec.Severity),
	}
}

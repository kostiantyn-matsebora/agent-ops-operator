package kube

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// blobs is the kube backend of state.Blobs: a queued input's out-of-line body as
// a ConversationInput, the one object a blob can be.
type blobs struct {
	base
}

// OpenBlobs returns the state.Blobs capability, scoped to namespace.
func OpenBlobs(c client.WithWatch, namespace string, opts ...Option) state.Blobs {
	return &blobs{base: newBase(c, namespace, newConfig(opts))}
}

// Put stores blob under key as a ConversationInput, upserting: an existing
// object is overwritten in place, a missing one is created ("replacing any
// value already there").
func (s *blobs) Put(ctx context.Context, key state.Key, blob state.Blob) error {
	var ci agentopsv1alpha1.ConversationInput
	err := s.client.Get(ctx, s.namespacedName(string(key)), &ci)
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

// Get returns the blob stored under key, or ErrNotFound.
func (s *blobs) Get(ctx context.Context, key state.Key) (state.Blob, error) {
	var ci agentopsv1alpha1.ConversationInput
	if err := s.client.Get(ctx, s.namespacedName(string(key)), &ci); err != nil {
		if apierrors.IsNotFound(err) {
			return state.Blob{}, state.ErrNotFound
		}
		return state.Blob{}, fmt.Errorf("kube: get blob %q: %w", key, err)
	}
	return toBlob(&ci), nil
}

// Delete removes a blob. Not an error when the key is already absent: "a
// deleted key reads absent".
func (s *blobs) Delete(ctx context.Context, key state.Key) error {
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

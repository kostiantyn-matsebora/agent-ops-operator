package kube

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// AppendRun adds run to key's append-only run history (store.go's
// Store.AppendRun: "takes no revision ... never fails with a conflict").
//
// The kube backend keeps runs embedded in status.runs[] (design D-5), so an
// append is a read-append-CAS loop against the status subresource, retried
// on the apiserver's own optimistic-concurrency conflict — appends commute,
// so retrying is always correct. statusUpdateMaxAttempts bounds it; only a
// caller that genuinely starved for that many consecutive retries (not
// expected in practice — see store.go's ErrConflict doc) ever sees
// ErrConflict out of this call.
func (s *Store) AppendRun(ctx context.Context, key state.Key, run state.Run) error {
	rs := toRunStatus(run)
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		conv := &agentopsv1alpha1.Conversation{}
		if err := s.client.Get(ctx, s.namespacedName(key), conv); err != nil {
			if apierrors.IsNotFound(err) {
				return state.ErrNotFound
			}
			return fmt.Errorf("kube: get conversation %q to append run: %w", key, err)
		}
		conv.Status.Runs = append(conv.Status.Runs, rs)
		if err := s.client.Status().Update(ctx, conv); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return fmt.Errorf("kube: append run to conversation %q: %w", key, err)
		}
		return nil
	}
	return state.ErrConflict
}

// ListRuns returns key's run history in append order (store.go's
// Store.ListRuns), a pure read of status.runs[] — no retry loop needed.
func (s *Store) ListRuns(ctx context.Context, key state.Key) ([]state.Run, error) {
	var conv agentopsv1alpha1.Conversation
	if err := s.client.Get(ctx, s.namespacedName(key), &conv); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, state.ErrNotFound
		}
		return nil, fmt.Errorf("kube: get conversation %q to list runs: %w", key, err)
	}
	if len(conv.Status.Runs) == 0 {
		return nil, nil
	}
	out := make([]state.Run, len(conv.Status.Runs))
	for i, rs := range conv.Status.Runs {
		out[i] = toRun(rs)
	}
	return out, nil
}

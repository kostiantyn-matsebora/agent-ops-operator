package kube

import (
	"context"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// maxRunHistory bounds status.runs[] on the stored object, so a long-lived
// conversation cannot grow its CR without limit. It is the bound the work
// report always applied itself; owning it here keeps every writer inside it.
const maxRunHistory = 10

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
		if len(conv.Status.Runs) > maxRunHistory {
			conv.Status.Runs = conv.Status.Runs[len(conv.Status.Runs)-maxRunHistory:]
		}
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
	return toRuns(conv.Status.Runs), nil
}

// toRuns maps a status.runs list; an empty one is nil.
func toRuns(in []agentopsv1alpha1.RunStatus) []state.Run {
	if len(in) == 0 {
		return nil
	}
	out := make([]state.Run, len(in))
	for i, rs := range in {
		out[i] = toRun(rs)
	}
	return out
}

// UpdateRun applies upd to the run named runID (store.go's Store.UpdateRun),
// a read-apply-CAS loop on status.runs[] like AppendRun's. Every RunUpdate
// field only grows, so a retry after a lost race re-applies it to the
// fresh object and is always correct; an update the run already satisfies
// writes nothing.
func (s *Store) UpdateRun(ctx context.Context, key state.Key, runID string, upd state.RunUpdate) error {
	for attempt := 0; attempt < statusUpdateMaxAttempts; attempt++ {
		conv := &agentopsv1alpha1.Conversation{}
		if err := s.client.Get(ctx, s.namespacedName(key), conv); err != nil {
			if apierrors.IsNotFound(err) {
				return state.ErrNotFound
			}
			return fmt.Errorf("kube: get conversation %q to update run: %w", key, err)
		}
		run := runByID(conv.Status.Runs, runID)
		if run == nil {
			return state.ErrNotFound
		}
		if !applyRunUpdate(run, upd) {
			return nil
		}
		if err := s.client.Status().Update(ctx, conv); err != nil {
			if apierrors.IsConflict(err) {
				continue
			}
			return fmt.Errorf("kube: update run %q of conversation %q: %w", runID, key, err)
		}
		return nil
	}
	return state.ErrConflict
}

func runByID(runs []agentopsv1alpha1.RunStatus, runID string) *agentopsv1alpha1.RunStatus {
	for i := range runs {
		if runs[i].RunID == runID {
			return &runs[i]
		}
	}
	return nil
}

// applyRunUpdate grows run by upd and reports whether anything changed.
func applyRunUpdate(run *agentopsv1alpha1.RunStatus, upd state.RunUpdate) bool {
	changed := false
	if upd.MarkDeliveryTracked && !run.DeliveryTracked {
		run.DeliveryTracked, changed = true, true
	}
	if upd.MarkRoutedToParent && !run.RoutedToParent {
		run.RoutedToParent, changed = true, true
	}
	for _, ch := range upd.MarkDelivered {
		if !slices.Contains(run.Delivered, ch) {
			run.Delivered, changed = append(run.Delivered, ch), true
		}
	}
	return changed
}

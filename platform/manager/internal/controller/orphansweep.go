package controller

import (
	"context"
	"errors"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/runtimepod"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// DefaultOrphanSweepInterval is how often the sweeper looks, and how long it
// waits after starting before the first look.
const DefaultOrphanSweepInterval = 5 * time.Minute

// orphanMinAge keeps the sweeper off an object created a moment ago, so it can
// never race a reconcile that is still building one.
const orphanMinAge = 2 * time.Minute

// OrphanSweeper deletes runtime pods and MCP ConfigMaps whose conversation has
// no record. It is the second net under the explicit release when conversations
// are not Kubernetes objects (ConversationReconciler.NoOwnerReference): there
// is no owner reference for the garbage collector to follow, so a pod whose
// record vanished while the manager was down, or whose delete event was missed,
// would otherwise stay until someone removed it by hand.
//
// It reads the record by name — the same read the reconciler makes — and acts
// only on a definite ErrNotFound, never on any other error, so an unreachable
// store sweeps nothing. The first look comes one interval after start, which
// lets a one-time import finish first.
type OrphanSweeper struct {
	Client        client.Client
	Conversations state.Collection[state.ConversationRecord]
	Namespace     string
	// Interval is the period between sweeps; zero means
	// DefaultOrphanSweepInterval.
	Interval time.Duration
}

// Start runs until ctx ends. It is a manager Runnable, so it runs only on the
// replica holding the leader Lease.
func (s *OrphanSweeper) Start(ctx context.Context) error {
	interval := s.Interval
	if interval <= 0 {
		interval = DefaultOrphanSweepInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	log := logf.FromContext(ctx).WithName("orphan-sweeper")
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if n, err := s.Sweep(ctx, time.Now()); err != nil {
				log.Error(err, "orphan sweep")
			} else if n > 0 {
				log.Info("deleted orphaned conversation workloads", "count", n)
			}
		}
	}
}

// Sweep makes one pass and returns how many objects it deleted.
func (s *OrphanSweeper) Sweep(ctx context.Context, now time.Time) (int, error) {
	deleted := 0

	pods := &corev1.PodList{}
	if err := s.Client.List(ctx, pods, client.InNamespace(s.Namespace),
		client.HasLabels{runtimepod.LabelConversation}); err != nil {
		return deleted, err
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if now.Sub(pod.CreationTimestamp.Time) < orphanMinAge {
			continue
		}
		gone, err := s.recordAbsent(ctx, pod.Labels[runtimepod.LabelConversation])
		if err != nil {
			return deleted, err
		}
		if gone {
			if err := s.Client.Delete(ctx, pod, client.GracePeriodSeconds(0)); err != nil && !apierrors.IsNotFound(err) {
				return deleted, err
			}
			deleted++
		}
	}

	cms := &corev1.ConfigMapList{}
	if err := s.Client.List(ctx, cms, client.InNamespace(s.Namespace)); err != nil {
		return deleted, err
	}
	prefix := convMCPConfigMapName("")
	for i := range cms.Items {
		cm := &cms.Items[i]
		if !strings.HasPrefix(cm.Name, prefix) || now.Sub(cm.CreationTimestamp.Time) < orphanMinAge {
			continue
		}
		gone, err := s.recordAbsent(ctx, strings.TrimPrefix(cm.Name, prefix))
		if err != nil {
			return deleted, err
		}
		if gone {
			if err := s.Client.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
				return deleted, err
			}
			deleted++
		}
	}
	return deleted, nil
}

// recordAbsent is true only when the store says, definitely, that no record
// has this name. Any other outcome — found, or an error — is "do not delete".
func (s *OrphanSweeper) recordAbsent(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	_, _, err := s.Conversations.Get(ctx, state.Key(name))
	if err == nil {
		return false, nil
	}
	if errors.Is(err, state.ErrNotFound) {
		return true, nil
	}
	return false, err
}

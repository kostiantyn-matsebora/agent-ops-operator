package chat

import (
	"context"
	"sort"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

// Pipeline resolution. EVERY origination resolves the same way: by the
// pipeline that CLAIMS the signal source, against READY pipelines only — a
// broken pipeline never silently swallows routing; its sources behave as
// unclaimed until it is fixed.
//
// There is deliberately no PipelineForChannel. Channels are shareable across
// pipelines on purpose, so "which pipeline answers for this channel" has no
// defensible answer — the oldest-Ready tiebreak that used to supply one is
// gone along with channel origination itself. A channel carries conversations;
// a claimed source starts them.

// readyPipelines lists Ready pipelines oldest-first (name tiebreak) — the
// deterministic claim order for a source with more than one claimant.
func readyPipelines(ctx context.Context, c client.Reader, namespace string) []agentopsv1alpha1.Pipeline {
	var list agentopsv1alpha1.PipelineList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil
	}
	var ready []agentopsv1alpha1.Pipeline
	for i := range list.Items {
		if apimeta.IsStatusConditionTrue(list.Items[i].Status.Conditions, "Ready") {
			ready = append(ready, list.Items[i])
		}
	}
	sort.Slice(ready, func(i, j int) bool {
		if !ready[i].CreationTimestamp.Time.Equal(ready[j].CreationTimestamp.Time) {
			return ready[i].CreationTimestamp.Time.Before(ready[j].CreationTimestamp.Time)
		}
		return ready[i].Name < ready[j].Name
	})
	return ready
}

// PipelineForSource returns the Ready pipeline claiming a signal source
// (oldest claimant), or nil.
func PipelineForSource(ctx context.Context, c client.Reader, namespace, source string) *agentopsv1alpha1.Pipeline {
	for _, p := range readyPipelines(ctx, c, namespace) {
		for _, ref := range p.Spec.SignalSourceRefs {
			if ref.Name == source {
				cp := p
				return &cp
			}
		}
	}
	return nil
}

package chat

import (
	"context"
	"sort"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

// Pipeline resolution: pipeline-first with fallback. Both entry points
// (signal routing and chat inbound) resolve against READY pipelines only —
// a broken pipeline never silently swallows routing; its sources/channels
// behave as unclaimed until it is fixed.

// readyPipelines lists Ready pipelines oldest-first (name tiebreak) — the
// deterministic claim order everywhere.
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

// PipelineForChannel returns the oldest Ready pipeline referencing a channel
// (the deterministic originator for inbound on multi-pipeline channels), or nil.
func PipelineForChannel(ctx context.Context, c client.Reader, namespace, channel string) *agentopsv1alpha1.Pipeline {
	for _, p := range readyPipelines(ctx, c, namespace) {
		for _, ref := range p.Spec.ChannelRefs {
			if ref.Name == channel {
				cp := p
				return &cp
			}
		}
	}
	return nil
}

// CapabilityPipelineForProfile returns the Ready Pipeline that declares a
// profile's BASELINE capabilities: one naming this profile with no sources and
// no channels. Such a Pipeline routes nothing — it exists to say what this
// agent may do when a conversation reaches it through a path that has no
// routing pipeline of its own (POST /task without one, /<profile> commands).
//
// Routing pipelines are deliberately NOT eligible. Picking one of them would
// have to choose among routes that exist precisely because they grant DIFFERENT
// capabilities, and "whichever was created first" is not a defensible answer.
//
// Exactly one may apply: two baselines for one profile return nil, so the
// ambiguity surfaces as missing capabilities plus a condition on both Pipelines
// rather than a silent winner.
func CapabilityPipelineForProfile(ctx context.Context, c client.Reader, namespace, profile string) *agentopsv1alpha1.Pipeline {
	var found *agentopsv1alpha1.Pipeline
	for _, p := range readyPipelines(ctx, c, namespace) {
		if !IsCapabilityPipeline(&p) || p.Spec.ProfileRef.Name != profile {
			continue
		}
		if found != nil {
			return nil // ambiguous — the reconciler reports it on both
		}
		cp := p
		found = &cp
	}
	return found
}

// IsCapabilityPipeline reports whether a Pipeline declares a baseline rather
// than routing anything: no sources, no channels.
func IsCapabilityPipeline(p *agentopsv1alpha1.Pipeline) bool {
	return len(p.Spec.SignalSourceRefs) == 0 && len(p.Spec.ChannelRefs) == 0
}

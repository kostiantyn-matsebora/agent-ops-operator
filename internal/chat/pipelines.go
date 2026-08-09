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

// ReadyPipelines lists Ready pipelines oldest-first (name tiebreak) — the
// deterministic claim order for a source with more than one claimant. Exported
// so a caller attributing MANY conversations (a metrics scrape) can list once
// instead of once per conversation.
func ReadyPipelines(ctx context.Context, c client.Reader, namespace string) []agentopsv1alpha1.Pipeline {
	return readyPipelines(ctx, c, namespace)
}

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

// PipelineForConversation INFERS which pipeline a conversation came from, by
// matching the bindings the conversation materialized against the pipelines
// that could have produced them. It returns nil when nothing matches or when
// more than one does.
//
// A Conversation carries no pipelineRef — the bindings are materialized state,
// the wiring is not snapshotted — so attribution is inference and MUST stay
// honest about it: ambiguous means nil, never "the first plausible one". The
// only use is labelling (telemetry, the console); nothing routes on it, so a
// blank answer costs a label rather than a decision.
func PipelineForConversation(ctx context.Context, c client.Reader, namespace string,
	conv *agentopsv1alpha1.Conversation) *agentopsv1alpha1.Pipeline {

	return MatchPipeline(readyPipelines(ctx, c, namespace), conv)
}

// MatchPipeline is PipelineForConversation over an already-listed candidate set.
func MatchPipeline(candidates []agentopsv1alpha1.Pipeline, conv *agentopsv1alpha1.Conversation) *agentopsv1alpha1.Pipeline {
	if conv == nil {
		return nil
	}
	var match *agentopsv1alpha1.Pipeline
	for _, p := range candidates {
		if p.Spec.ProfileRef.Name != conv.Spec.ProfileRef.Name {
			continue
		}
		if !sameRefs(p.Spec.ChannelRefs, conv.Spec.ChannelRefs) {
			continue
		}
		if !sameToolsets(p.Spec.Toolsets, conv.Spec.Toolsets) || !sameMCPConfigs(p.Spec.MCPConfigs, conv.Spec.MCPConfigs) {
			continue
		}
		if match != nil {
			return nil // ambiguous: two pipelines wire identically
		}
		cp := p
		match = &cp
	}
	return match
}

// sameRefs compares two ref lists as SETS: a conversation's channel set may
// have gained the surface it originated on, and order carries no meaning here.
func sameRefs(a, b []agentopsv1alpha1.ObjectRef) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, r := range a {
		seen[r.Name]++
	}
	for _, r := range b {
		seen[r.Name]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

// sameToolsets / sameMCPConfigs compare capability bindings by their ORDERED
// refs — order is meaningful (tools concatenate, server keys overlay later-wins),
// so the same set in a different order is a different binding.
func sameToolsets(a, b *agentopsv1alpha1.ToolsetBinding) bool {
	var ar, br []agentopsv1alpha1.ObjectRef
	var am, bm string
	if a != nil {
		ar, am = a.Refs, a.Mode
	}
	if b != nil {
		br, bm = b.Refs, b.Mode
	}
	if len(ar) == 0 && len(br) == 0 {
		return true // neither binds tools: the mode has nothing to compose
	}
	return am == bm && orderedRefsEqual(ar, br)
}

func sameMCPConfigs(a, b *agentopsv1alpha1.ToolingBinding) bool {
	var ar, br []agentopsv1alpha1.ObjectRef
	if a != nil {
		ar = a.Refs
	}
	if b != nil {
		br = b.Refs
	}
	return orderedRefsEqual(ar, br)
}

func orderedRefsEqual(a, b []agentopsv1alpha1.ObjectRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}

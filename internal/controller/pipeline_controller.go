package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
)

// Pipeline condition types.
const (
	// ConditionSourceConflict: an older pipeline already claims a referenced
	// signal source (one pipeline per source; oldest claimant wins).
	ConditionSourceConflict = "SourceConflict"
	// ConditionBaselineConflict: another capability-only pipeline (no sources,
	// no channels) already declares this profile's baseline. Unlike a source
	// conflict there is no "oldest wins" — neither applies, because guessing
	// which baseline an operator meant is worse than granting nothing.
	ConditionBaselineConflict = "BaselineConflict"
)

// PipelineReconciler validates pipeline wiring and keeps its conditions
// current. It creates nothing — routing reads Ready pipelines at decision
// time (pipeline-first, source-level fallback).
type PipelineReconciler struct {
	client.Client
}

// Reconcile validates one Pipeline.
func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var p agentopsv1alpha1.Pipeline
	if err := r.Get(ctx, req.NamespacedName, &p); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var missing []string
	for _, ref := range p.Spec.SignalSourceRefs {
		var src agentopsv1alpha1.SignalSource
		if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ref.Name}, &src); err != nil {
			missing = append(missing, "signalsource/"+ref.Name)
		}
	}
	for _, ref := range p.Spec.ChannelRefs {
		var ch agentopsv1alpha1.Channel
		if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ref.Name}, &ch); err != nil {
			missing = append(missing, "channel/"+ref.Name)
		}
	}
	var profile agentopsv1alpha1.AgentProfile
	if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: p.Spec.ProfileRef.Name}, &profile); err != nil {
		missing = append(missing, "agentprofile/"+p.Spec.ProfileRef.Name)
	}
	// tooling bindings: refs only — the CRs' content is resolved at use time,
	// so Ready checks existence, nothing else.
	if p.Spec.Toolsets != nil {
		for _, ref := range p.Spec.Toolsets.Refs {
			var ts agentopsv1alpha1.MCPToolset
			if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ref.Name}, &ts); err != nil {
				missing = append(missing, "mcptoolset/"+ref.Name)
			}
		}
	}
	if p.Spec.MCPConfigs != nil {
		for _, ref := range p.Spec.MCPConfigs.Refs {
			var mc agentopsv1alpha1.MCPConfig
			if err := r.Get(ctx, types.NamespacedName{Namespace: p.Namespace, Name: ref.Name}, &mc); err != nil {
				missing = append(missing, "mcpconfig/"+ref.Name)
			}
		}
	}

	conflicts, err := r.sourceConflicts(ctx, &p)
	if err != nil {
		return ctrl.Result{}, err
	}
	baselineRivals, err := r.baselineConflicts(ctx, &p)
	if err != nil {
		return ctrl.Result{}, err
	}

	patch := client.MergeFrom(p.DeepCopy())
	conflictCond := metav1.Condition{Type: ConditionSourceConflict, Status: metav1.ConditionFalse, Reason: "NoConflict"}
	if len(conflicts) > 0 {
		conflictCond.Status = metav1.ConditionTrue
		conflictCond.Reason = "SourceConflict"
		conflictCond.Message = "sources already claimed by older pipelines: " + strings.Join(conflicts, "; ")
	}
	apimeta.SetStatusCondition(&p.Status.Conditions, conflictCond)

	baselineCond := metav1.Condition{Type: ConditionBaselineConflict, Status: metav1.ConditionFalse, Reason: "NoConflict"}
	if len(baselineRivals) > 0 {
		baselineCond.Status = metav1.ConditionTrue
		baselineCond.Reason = "DuplicateBaseline"
		baselineCond.Message = fmt.Sprintf(
			"profile %q already has a capability baseline declared by: %s — neither applies until one is removed",
			p.Spec.ProfileRef.Name, strings.Join(baselineRivals, ", "))
	}
	apimeta.SetStatusCondition(&p.Status.Conditions, baselineCond)

	ready := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "WiringValid"}
	switch {
	case len(missing) > 0:
		ready.Status = metav1.ConditionFalse
		ready.Reason = "MissingReferences"
		ready.Message = "unresolved references: " + strings.Join(missing, ", ")
	case len(conflicts) > 0:
		ready.Status = metav1.ConditionFalse
		ready.Reason = "SourceConflict"
		ready.Message = conflictCond.Message
	}
	apimeta.SetStatusCondition(&p.Status.Conditions, ready)
	return ctrl.Result{}, r.Status().Patch(ctx, &p, patch)
}

// baselineConflicts lists OTHER capability-only pipelines declaring the same
// profile's baseline. Deliberately symmetric — every rival is reported on every
// side, with no oldest-wins rule — because a baseline is what an agent may do
// when nothing else says, and silently picking one of two answers to that is
// worse than granting nothing until the operator resolves it.
func (r *PipelineReconciler) baselineConflicts(ctx context.Context, p *agentopsv1alpha1.Pipeline) ([]string, error) {
	if !chat.IsCapabilityPipeline(p) {
		return nil, nil
	}
	var list agentopsv1alpha1.PipelineList
	if err := r.List(ctx, &list, client.InNamespace(p.Namespace)); err != nil {
		return nil, err
	}
	var rivals []string
	for i := range list.Items {
		other := &list.Items[i]
		if other.Name == p.Name || !chat.IsCapabilityPipeline(other) {
			continue
		}
		if other.Spec.ProfileRef.Name == p.Spec.ProfileRef.Name {
			rivals = append(rivals, other.Name)
		}
	}
	sort.Strings(rivals)
	return rivals, nil
}

// sourceConflicts lists sources of this pipeline already claimed by an OLDER
// pipeline (creation time, name tiebreak).
func (r *PipelineReconciler) sourceConflicts(ctx context.Context, p *agentopsv1alpha1.Pipeline) ([]string, error) {
	var list agentopsv1alpha1.PipelineList
	if err := r.List(ctx, &list, client.InNamespace(p.Namespace)); err != nil {
		return nil, err
	}
	mine := map[string]bool{}
	for _, ref := range p.Spec.SignalSourceRefs {
		mine[ref.Name] = true
	}
	var conflicts []string
	for i := range list.Items {
		other := &list.Items[i]
		if other.Name == p.Name {
			continue
		}
		older := other.CreationTimestamp.Time.Before(p.CreationTimestamp.Time) ||
			(other.CreationTimestamp.Time.Equal(p.CreationTimestamp.Time) && other.Name < p.Name)
		if !older {
			continue
		}
		for _, ref := range other.Spec.SignalSourceRefs {
			if mine[ref.Name] {
				conflicts = append(conflicts, fmt.Sprintf("%s (by %s)", ref.Name, other.Name))
			}
		}
	}
	return conflicts, nil
}

// SetupWithManager wires the controller: pipelines, plus referenced-kind
// events mapped back to the pipelines naming them (and pipeline events to
// same-source pipelines so conflicts converge when the older claimant goes).
func (r *PipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	allPipelines := func(ctx context.Context, namespace string) []ctrl.Request {
		var list agentopsv1alpha1.PipelineList
		if err := r.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return nil
		}
		var reqs []ctrl.Request
		for i := range list.Items {
			reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
		}
		return reqs
	}
	mapAny := func(ctx context.Context, obj client.Object) []ctrl.Request {
		// referenced kinds are few and pipelines fewer — requeue them all
		return allPipelines(ctx, obj.GetNamespace())
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.Pipeline{}).
		Watches(&agentopsv1alpha1.SignalSource{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.Channel{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.AgentProfile{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.MCPToolset{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.MCPConfig{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.Pipeline{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Complete(r)
}

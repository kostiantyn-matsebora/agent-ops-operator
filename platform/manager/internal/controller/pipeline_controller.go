package controller

import (
	"context"
	"strings"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// PipelineReconciler validates pipeline wiring and keeps its conditions
// current. It creates nothing — routing reads Ready pipelines at decision
// time.
//
// There is NO source-exclusivity check here, and adding one back would be a
// regression. A SignalSource is shareable exactly as a Channel is: any number
// of Ready pipelines may list one, and a signal admitted there fans out to
// every one of them, one conversation each. Whether two pipelines watch one
// source is the ADOPTER's decision. The rule that used to live here
// (`sourceConflicts`, oldest claimant wins, newer at Ready=False) existed to
// keep a single invisible default for bare chat messages, and charged every
// source kind for it; that ambiguity is now handled where it actually occurs,
// in the chat lane, by refusing rather than guessing.
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

	patch := client.MergeFrom(p.DeepCopy())
	// MIGRATION (one release): clear a SourceConflict left by a manager that
	// still enforced exclusivity. Deleting the writer does not delete what it
	// already wrote, so without this the condition is IMMORTAL — and it lands
	// exactly on the pipelines this change unblocks, which are the ones whose
	// operators are most likely to look. Seen live on upgrade: a pipeline
	// created seconds before the rollout kept `SourceConflict=True` beside a
	// perfectly valid `Ready=True`.
	apimeta.RemoveStatusCondition(&p.Status.Conditions, "SourceConflict")

	ready := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "WiringValid"}
	if len(missing) > 0 {
		ready.Status = metav1.ConditionFalse
		ready.Reason = "MissingReferences"
		ready.Message = "unresolved references: " + strings.Join(missing, ", ")
	}
	apimeta.SetStatusCondition(&p.Status.Conditions, ready)
	return ctrl.Result{}, r.Status().Patch(ctx, &p, patch)
}

// SetupWithManager wires the controller: pipelines, plus referenced-kind
// events mapped back to the pipelines naming them. A pipeline no longer
// watches its SIBLINGS — that watch existed so a conflict could converge when
// the older claimant went away, and there are no conflicts left to converge.
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
		Complete(r)
}

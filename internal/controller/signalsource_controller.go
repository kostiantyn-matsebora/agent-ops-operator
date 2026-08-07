package controller

import (
	"context"
	"fmt"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
)

// ConditionWired answers "does any Ready Pipeline claim this source" — the
// pipeline-only-wiring diagnosis for signals being dropped.
const ConditionWired = "Wired"

// SignalSourceReconciler keeps the Served and Wired conditions current on
// every SignalSource — the signal sibling of ChannelReconciler. Built-in
// types (alertmanagerWebhook, hosted by the manager's own HTTP surface) are
// always served; everything else needs a Ready SignalAdapter or an
// adapter-reported Ready condition (hand-deployed adapters). Wiring is
// pipeline-only: sources route signals only while a Ready Pipeline claims
// them.
type SignalSourceReconciler struct {
	client.Client
}

// Reconcile sets Served on one SignalSource.
func (r *SignalSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var src agentopsv1alpha1.SignalSource
	if err := r.Get(ctx, req.NamespacedName, &src); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// no built-in signal types: every type needs a serving adapter
	cond := metav1.Condition{Type: ConditionServed, Status: metav1.ConditionFalse, Reason: "NoServingImplementation",
		Message: fmt.Sprintf("no Ready SignalAdapter serves type %q (hand-deployed adapters report per-source readiness on the Ready condition)", src.Spec.Type)}
	if name, ready := r.readyAdapter(ctx, &src); ready {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "AdapterReady"
		cond.Message = fmt.Sprintf("served by SignalAdapter %q", name)
	} else if c := apimeta.FindStatusCondition(src.Status.Conditions, "Ready"); c != nil && c.Status == metav1.ConditionTrue {
		// a hand-deployed adapter (no CR) proved itself via the status
		// contract — don't contradict it
		cond.Status = metav1.ConditionTrue
		cond.Reason = "AdapterReported"
		cond.Message = "an adapter reported this source Ready through the contract"
	}

	// Wired: pipeline-only wiring — a source routes signals only while a Ready
	// Pipeline claims it.
	wired := metav1.Condition{Type: ConditionWired, Status: metav1.ConditionFalse, Reason: "NoPipelineClaim",
		Message: "no Ready Pipeline references this source — signals are dropped until one does"}
	if p := chat.PipelineForSource(ctx, r.Client, src.Namespace, src.Name); p != nil {
		wired.Status = metav1.ConditionTrue
		wired.Reason = "PipelineClaim"
		wired.Message = fmt.Sprintf("wired by Pipeline %q", p.Name)
	}

	servedSame := false
	if existing := apimeta.FindStatusCondition(src.Status.Conditions, ConditionServed); existing != nil &&
		existing.Status == cond.Status && existing.Reason == cond.Reason {
		servedSame = true
	}
	wiredSame := false
	if existing := apimeta.FindStatusCondition(src.Status.Conditions, ConditionWired); existing != nil &&
		existing.Status == wired.Status && existing.Message == wired.Message {
		wiredSame = true
	}
	if servedSame && wiredSame {
		return ctrl.Result{}, nil
	}
	patch := client.MergeFrom(src.DeepCopy())
	apimeta.SetStatusCondition(&src.Status.Conditions, cond)
	apimeta.SetStatusCondition(&src.Status.Conditions, wired)
	return ctrl.Result{}, r.Status().Patch(ctx, &src, patch)
}

func (r *SignalSourceReconciler) readyAdapter(ctx context.Context, src *agentopsv1alpha1.SignalSource) (string, bool) {
	var list agentopsv1alpha1.SignalAdapterList
	if err := r.List(ctx, &list, client.InNamespace(src.Namespace)); err != nil {
		return "", false
	}
	for i := range list.Items {
		a := &list.Items[i]
		if a.Spec.Type == src.Spec.Type && apimeta.IsStatusConditionTrue(a.Status.Conditions, ConditionReady) {
			return a.Name, true
		}
	}
	return "", false
}

// SetupWithManager wires the controller: SignalSources, SignalAdapter events
// mapped onto the sources of that adapter's type, and Pipeline events mapped
// onto all sources (claim changes flip Wired).
func (r *SignalSourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapAdapter := func(ctx context.Context, obj client.Object) []ctrl.Request {
		a, ok := obj.(*agentopsv1alpha1.SignalAdapter)
		if !ok {
			return nil
		}
		var list agentopsv1alpha1.SignalSourceList
		if err := r.List(ctx, &list, client.InNamespace(a.Namespace)); err != nil {
			return nil
		}
		var reqs []ctrl.Request
		for i := range list.Items {
			if list.Items[i].Spec.Type == a.Spec.Type {
				reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
			}
		}
		return reqs
	}
	mapPipeline := func(ctx context.Context, obj client.Object) []ctrl.Request {
		var list agentopsv1alpha1.SignalSourceList
		if err := r.List(ctx, &list, client.InNamespace(obj.GetNamespace())); err != nil {
			return nil
		}
		var reqs []ctrl.Request
		for i := range list.Items {
			reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
		}
		return reqs
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.SignalSource{}).
		Watches(&agentopsv1alpha1.SignalAdapter{}, handler.EnqueueRequestsFromMapFunc(mapAdapter)).
		Watches(&agentopsv1alpha1.Pipeline{}, handler.EnqueueRequestsFromMapFunc(mapPipeline)).
		Complete(r)
}

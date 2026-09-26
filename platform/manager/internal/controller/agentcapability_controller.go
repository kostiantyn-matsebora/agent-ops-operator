package controller

import (
	"context"
	"strings"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// AgentCapabilityReconciler validates an AgentCapability's own references —
// the SAME check a Pipeline runs on an inline capability, through the same
// function, so a referenced and an inline capability are never validated by
// two different rules.
//
// An AgentCapability wires nothing itself: it carries no signal sources and
// no channels, so there is nothing here to admit or fan out. A capability
// nothing references is INERT, not invalid — that is the ordinary state of
// one meant to be shared.
type AgentCapabilityReconciler struct {
	client.Client
}

// Reconcile validates one AgentCapability.
func (r *AgentCapabilityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var capability agentopsv1alpha1.AgentCapability
	if err := r.Get(ctx, req.NamespacedName, &capability); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	missing := validateCapabilitySpecRefs(ctx, r.Client, capability.Namespace, capability.Spec)

	patch := client.MergeFrom(capability.DeepCopy())
	ready := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "WiringValid"}
	if len(missing) > 0 {
		ready.Status = metav1.ConditionFalse
		ready.Reason = "MissingReferences"
		ready.Message = "unresolved references: " + strings.Join(missing, ", ")
	}
	apimeta.SetStatusCondition(&capability.Status.Conditions, ready)
	return ctrl.Result{}, r.Status().Patch(ctx, &capability, patch)
}

// SetupWithManager wires the controller: capabilities, plus referenced-kind
// events mapped back to the capabilities naming them.
func (r *AgentCapabilityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	allCapabilities := func(ctx context.Context, namespace string) []ctrl.Request {
		var list agentopsv1alpha1.AgentCapabilityList
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
		return allCapabilities(ctx, obj.GetNamespace())
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.AgentCapability{}).
		Watches(&agentopsv1alpha1.AgentProfile{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.AgentRuntime{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.MCPToolset{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Watches(&agentopsv1alpha1.MCPConfig{}, handler.EnqueueRequestsFromMapFunc(mapAny)).
		Complete(r)
}

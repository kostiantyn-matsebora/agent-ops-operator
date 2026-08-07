package controller

import (
	"context"
	"fmt"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/internal/chat"
)

// ConditionServed answers "does anything serve this channel's type" — the
// channel-level diagnosis for ops silently queueing forever (e.g. a typo'd
// spec.type).
const ConditionServed = "Served"

// ChannelReconciler keeps the Served condition current on every Channel.
type ChannelReconciler struct {
	client.Client
	// Registry resolves in-process (built-in) channel types.
	Registry *chat.Registry
}

// Reconcile sets Served on one Channel.
func (r *ChannelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var ch agentopsv1alpha1.Channel
	if err := r.Get(ctx, req.NamespacedName, &ch); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	cond := metav1.Condition{Type: ConditionServed, Status: metav1.ConditionFalse, Reason: "NoServingImplementation",
		Message: fmt.Sprintf("no in-process provider or Ready ChannelAdapter named %q (hand-deployed adapters report per-channel readiness on the Ready condition)", ch.Spec.Type)}

	switch {
	case r.Registry != nil && r.Registry.Resolve(ch.Spec.Type) != nil:
		cond.Status = metav1.ConditionTrue
		cond.Reason = "InProcessProvider"
		cond.Message = fmt.Sprintf("type %q is served in-process by the manager", ch.Spec.Type)
	default:
		if name, ready := r.readyAdapter(ctx, &ch); ready {
			cond.Status = metav1.ConditionTrue
			cond.Reason = "AdapterReady"
			cond.Message = fmt.Sprintf("served by ChannelAdapter %q", name)
		} else if c := apimeta.FindStatusCondition(ch.Status.Conditions, "Ready"); c != nil && c.Status == metav1.ConditionTrue {
			// a hand-deployed adapter (no CR) proved itself via the status
			// contract — don't contradict it
			cond.Status = metav1.ConditionTrue
			cond.Reason = "AdapterReported"
			cond.Message = "an adapter reported this channel Ready through the contract"
		}
	}

	if apimeta.IsStatusConditionPresentAndEqual(ch.Status.Conditions, ConditionServed, cond.Status) {
		if existing := apimeta.FindStatusCondition(ch.Status.Conditions, ConditionServed); existing != nil && existing.Reason == cond.Reason {
			return ctrl.Result{}, nil
		}
	}
	patch := client.MergeFrom(ch.DeepCopy())
	apimeta.SetStatusCondition(&ch.Status.Conditions, cond)
	return ctrl.Result{}, r.Status().Patch(ctx, &ch, patch)
}

// readyAdapter resolves the adapter the channel names in spec.type — the
// adapter CR's NAME is the routing key.
func (r *ChannelReconciler) readyAdapter(ctx context.Context, ch *agentopsv1alpha1.Channel) (string, bool) {
	var a agentopsv1alpha1.ChannelAdapter
	if err := r.Get(ctx, types.NamespacedName{Namespace: ch.Namespace, Name: ch.Spec.Type}, &a); err != nil {
		return "", false
	}
	if apimeta.IsStatusConditionTrue(a.Status.Conditions, ConditionReady) {
		return a.Name, true
	}
	return "", false
}

// SetupWithManager wires the controller: Channels, plus ChannelAdapter events
// mapped onto the channels naming that adapter (readiness transitions flip
// Served without any Channel edit).
func (r *ChannelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapAdapter := func(ctx context.Context, obj client.Object) []ctrl.Request {
		a, ok := obj.(*agentopsv1alpha1.ChannelAdapter)
		if !ok {
			return nil
		}
		var list agentopsv1alpha1.ChannelList
		if err := r.List(ctx, &list, client.InNamespace(a.Namespace)); err != nil {
			return nil
		}
		var reqs []ctrl.Request
		for i := range list.Items {
			if list.Items[i].Spec.Type == a.Name {
				reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
			}
		}
		return reqs
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.Channel{}).
		Watches(&agentopsv1alpha1.ChannelAdapter{}, handler.EnqueueRequestsFromMapFunc(mapAdapter)).
		Complete(r)
}

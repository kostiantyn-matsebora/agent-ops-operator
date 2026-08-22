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
		Message: fmt.Sprintf("no in-process provider or Ready ChannelAdapter named %q (hand-deployed adapters report per-channel readiness on the Ready condition)", ch.Spec.Adapter)}

	// the adapter CR is both the readiness source and the schema declarer
	adapter := r.adapter(ctx, &ch)

	switch {
	case r.Registry != nil && r.Registry.Resolve(ch.Spec.Adapter) != nil:
		cond.Status = metav1.ConditionTrue
		cond.Reason = "InProcessProvider"
		cond.Message = fmt.Sprintf("adapter %q is served in-process by the manager", ch.Spec.Adapter)
	default:
		if adapter != nil && apimeta.IsStatusConditionTrue(adapter.Status.Conditions, ConditionReady) {
			cond.Status = metav1.ConditionTrue
			cond.Reason = "AdapterReady"
			cond.Message = fmt.Sprintf("served by ChannelAdapter %q", adapter.Name)
		} else if c := apimeta.FindStatusCondition(ch.Status.Conditions, "Ready"); c != nil && c.Status == metav1.ConditionTrue {
			// a hand-deployed adapter (no CR) proved itself via the status
			// contract — don't contradict it
			cond.Status = metav1.ConditionTrue
			cond.Reason = "AdapterReported"
			cond.Message = "an adapter reported this channel Ready through the contract"
		}
	}

	// advisory: validates spec.config against whatever schema the adapter CR
	// declares, and changes nothing else about serving
	var configCond *metav1.Condition
	if adapter != nil {
		configCond = validateAgainstAdapter(adapter.Spec.ConfigSchema, ch.Spec.Config, adapter.Name)
	}

	if servedUnchanged(ch.Status.Conditions, cond) && configValidUnchanged(ch.Status.Conditions, configCond) {
		return ctrl.Result{}, nil
	}
	patch := client.MergeFrom(ch.DeepCopy())
	apimeta.SetStatusCondition(&ch.Status.Conditions, cond)
	applyConfigValid(&ch.Status.Conditions, configCond)
	return ctrl.Result{}, r.Status().Patch(ctx, &ch, patch)
}

// servedUnchanged reports whether the Served condition already says this.
func servedUnchanged(conds []metav1.Condition, want metav1.Condition) bool {
	existing := apimeta.FindStatusCondition(conds, ConditionServed)
	return existing != nil && existing.Status == want.Status && existing.Reason == want.Reason
}

// configValidUnchanged reports whether ConfigValid already matches, including
// the "should be absent" case.
func configValidUnchanged(conds []metav1.Condition, want *metav1.Condition) bool {
	existing := apimeta.FindStatusCondition(conds, ConditionConfigValid)
	if want == nil {
		return existing == nil
	}
	return existing != nil && existing.Status == want.Status &&
		existing.Reason == want.Reason && existing.Message == want.Message
}

// adapter resolves the ChannelAdapter the channel names in spec.adapter — the
// adapter CR's NAME is the routing key. nil when none exists.
func (r *ChannelReconciler) adapter(ctx context.Context, ch *agentopsv1alpha1.Channel) *agentopsv1alpha1.ChannelAdapter {
	var a agentopsv1alpha1.ChannelAdapter
	if err := r.Get(ctx, types.NamespacedName{Namespace: ch.Namespace, Name: ch.Spec.Adapter}, &a); err != nil {
		return nil
	}
	return &a
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
			if list.Items[i].Spec.Adapter == a.Name {
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

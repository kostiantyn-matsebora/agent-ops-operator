package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// ConditionWired answers "does any Ready Pipeline claim this source" — the
// pipeline-only-wiring diagnosis for signals being dropped.
const ConditionWired = "Wired"

// SignalSourceReconciler keeps the Served and Wired conditions current on
// every SignalSource — the signal sibling of ChannelReconciler. Every type
// needs a Ready SignalAdapter named by spec.type, or an adapter-reported
// Ready condition (hand-deployed adapters). Wiring is pipeline-only: sources
// route signals only while a Ready Pipeline claims them.
type SignalSourceReconciler struct {
	client.Client
	// Signals and Cooldowns release a deleted source's signals and cooldown
	// window: the manager's own act on the deletion, so nothing depends on an
	// owner reference being honoured (state-store design D-14).
	Signals   state.Collection[state.SignalRecord]
	Cooldowns state.Collection[state.CooldownWindow]
	// AdapterState releases a deleted SignalSource's own adapter-state keys
	// (state-store-generic-contract design D-4), through the typed
	// collection rather than a raw annotation reach.
	AdapterState state.Collection[string]
}

// Reconcile sets Served on one SignalSource.
func (r *SignalSourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var src agentopsv1alpha1.SignalSource
	if err := r.Get(ctx, req.NamespacedName, &src); err != nil {
		if apierrors.IsNotFound(err) {
			releaser := &SourceReleaser{Signals: r.Signals, Cooldowns: r.Cooldowns, Live: liveSignalSource(r.Client, req.Namespace)}
			return ctrl.Result{}, releaser.Release(ctx, req.Name)
		}
		return ctrl.Result{}, err
	}

	// the adapter CR is both the readiness source and the schema declarer
	adapter := r.adapter(ctx, &src)

	cond := servedCondition(&src, adapter)
	wired := r.wiredCondition(ctx, &src)

	servedSame := conditionMatches(src.Status.Conditions, cond, false)
	wiredSame := conditionMatches(src.Status.Conditions, wired, true)
	// advisory: never affects Served, Wired, or ingestion
	var configCond *metav1.Condition
	if adapter != nil {
		configCond = validateAgainstAdapter(adapter.Spec.ConfigSchema, src.Spec.Config, adapter.Name)
	}

	if servedSame && wiredSame && configValidUnchanged(src.Status.Conditions, configCond) {
		return ctrl.Result{}, nil
	}
	patch := client.MergeFrom(src.DeepCopy())
	apimeta.SetStatusCondition(&src.Status.Conditions, cond)
	apimeta.SetStatusCondition(&src.Status.Conditions, wired)
	applyConfigValid(&src.Status.Conditions, configCond)
	return ctrl.Result{}, r.Status().Patch(ctx, &src, patch)
}

// conditionMatches reports whether the stored condition of want's type already
// says what want says. Served compares the reason, Wired compares the message.
func conditionMatches(conds []metav1.Condition, want metav1.Condition, byMessage bool) bool {
	existing := apimeta.FindStatusCondition(conds, want.Type)
	if existing == nil || existing.Status != want.Status {
		return false
	}
	if byMessage {
		return existing.Message == want.Message
	}
	return existing.Reason == want.Reason
}

// servedCondition is the Served condition: no built-in signal types, so every
// type needs a serving adapter.
func servedCondition(src *agentopsv1alpha1.SignalSource, adapter *agentopsv1alpha1.SignalAdapter) metav1.Condition {
	cond := metav1.Condition{Type: ConditionServed, Status: metav1.ConditionFalse, Reason: "NoServingImplementation",
		Message: fmt.Sprintf("no Ready SignalAdapter named %q (hand-deployed adapters report per-source readiness on the Ready condition)", src.Spec.Adapter)}
	if adapter != nil && apimeta.IsStatusConditionTrue(adapter.Status.Conditions, ConditionReady) {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "AdapterReady"
		cond.Message = fmt.Sprintf("served by SignalAdapter %q", adapter.Name)
	} else if c := apimeta.FindStatusCondition(src.Status.Conditions, "Ready"); c != nil && c.Status == metav1.ConditionTrue {
		// a hand-deployed adapter (no CR) proved itself via the status
		// contract — don't contradict it
		cond.Status = metav1.ConditionTrue
		cond.Reason = "AdapterReported"
		cond.Message = "an adapter reported this source Ready through the contract"
	}
	return cond
}

// wiredCondition is the Wired condition.
func (r *SignalSourceReconciler) wiredCondition(ctx context.Context, src *agentopsv1alpha1.SignalSource) metav1.Condition {
	// Wired: claimant-only wiring — a source routes signals only while a Ready
	// Pipeline OR Coordinator lists it (design D-B: a Coordinator claims a
	// source exactly as a Pipeline does). The message names ALL of them, never
	// just the first: a source several claimants watch fans each signal out to
	// every one, so the COUNT is what an operator needs to predict behaviour —
	// how many conversations one signal opens, and (on a chat source) whether a
	// bare message is unambiguous or gets refused with the choices.
	wired := metav1.Condition{Type: ConditionWired, Status: metav1.ConditionFalse, Reason: "NoPipelineClaim",
		Message: "no Ready Pipeline or Coordinator references this source — signals are dropped until one does"}
	if servers := chat.PipelinesForSource(ctx, r.Client, src.Namespace, src.Name); len(servers) > 0 {
		names := make([]string, 0, len(servers))
		for i := range servers {
			names = append(names, strconv.Quote(servers[i].GetName()))
		}
		wired.Status = metav1.ConditionTrue
		wired.Reason = "PipelineClaim"
		wired.Message = fmt.Sprintf("wired by %d claimant(s): %s — each opens its own conversation per signal",
			len(names), strings.Join(names, ", "))
	}
	return wired
}

// adapter resolves the SignalAdapter the source names in spec.adapter — the
// adapter CR's NAME is the routing key. nil when none exists.
func (r *SignalSourceReconciler) adapter(ctx context.Context, src *agentopsv1alpha1.SignalSource) *agentopsv1alpha1.SignalAdapter {
	var a agentopsv1alpha1.SignalAdapter
	if err := r.Get(ctx, types.NamespacedName{Namespace: src.Namespace, Name: src.Spec.Adapter}, &a); err != nil {
		return nil
	}
	return &a
}

// SetupWithManager wires the controller: SignalSources, SignalAdapter events
// mapped onto the sources naming that adapter, and Pipeline/Coordinator events
// mapped onto all sources (claim changes flip Wired — a Coordinator claims a
// source exactly as a Pipeline does, design D-B).
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
			if list.Items[i].Spec.Adapter == a.Name {
				reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
			}
		}
		return reqs
	}
	// mapClaimant requeues every source on any Pipeline or Coordinator event —
	// either kind's claim can flip Wired, and a targeted diff of WHICH claim
	// changed buys nothing a reconcile of every source doesn't already do
	// cheaply.
	mapClaimant := func(ctx context.Context, obj client.Object) []ctrl.Request {
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
	// releaseOnDelete mirrors ChannelReconciler's own (channel_controller.go):
	// a pure side effect reading the deleted SignalSource's own tombstone to
	// release its adapter-state keys, enqueueing nothing.
	releaseOnDelete := func(ctx context.Context, obj client.Object) []ctrl.Request {
		releaseAdapterStateKeys(ctx, r.AdapterState, state.SurfaceSignalSource, obj)
		return nil
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentopsv1alpha1.SignalSource{}).
		Watches(&agentopsv1alpha1.SignalAdapter{}, handler.EnqueueRequestsFromMapFunc(mapAdapter)).
		Watches(&agentopsv1alpha1.Pipeline{}, handler.EnqueueRequestsFromMapFunc(mapClaimant)).
		Watches(&agentopsv1alpha1.Coordinator{}, handler.EnqueueRequestsFromMapFunc(mapClaimant)).
		Watches(&agentopsv1alpha1.SignalSource{}, handler.EnqueueRequestsFromMapFunc(releaseOnDelete),
			builder.WithPredicates(deleteOnlyPredicate())).
		Complete(r)
}

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

// notReadySuffix and coordinatorRefLabel are the recurring fragments of a
// problem string, named once so they read the same everywhere they appear.
const (
	notReadySuffix      = " not ready"
	coordinatorRefLabel = ": coordinator/"
)

// CoordinatorReconciler validates one Coordinator's wiring: its own
// capability, the sources and channels it names, and every `agents[]` entry —
// through the SAME `validateCapabilitySpecRefs` a Pipeline and an
// AgentCapability already use, so a capability is never validated by a third
// rule.
//
// It creates nothing and originates no conversation. Routing and invocation
// are later phases of `coordinated-agents` (design D-B onward) — a Coordinator
// nobody applies, or that nothing yet reads, changes nothing.
type CoordinatorReconciler struct {
	client.Client
}

// Reconcile validates one Coordinator.
func (r *CoordinatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var co agentopsv1alpha1.Coordinator
	if err := r.Get(ctx, req.NamespacedName, &co); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	missing := coordinatorProblems(ctx, r.Client, &co, map[string]bool{co.Name: true})

	// A name held by both a Coordinator and a Pipeline is reported HERE, on
	// the Coordinator — addressing (`/<name>`) cannot tell the two apart, and
	// this is the object whose own Ready this rule was written against
	// (design D-B). The Pipeline's own Ready is untouched.
	var p agentopsv1alpha1.Pipeline
	if err := r.Get(ctx, types.NamespacedName{Namespace: co.Namespace, Name: co.Name}, &p); err == nil {
		missing = append(missing, "pipeline/"+co.Name+" holds the same name")
	}

	patch := client.MergeFrom(co.DeepCopy())
	ready := metav1.Condition{Type: "Ready", Status: metav1.ConditionTrue, Reason: "WiringValid"}
	if len(missing) > 0 {
		ready.Status = metav1.ConditionFalse
		ready.Reason = "MissingReferences"
		ready.Message = "unresolved references: " + strings.Join(missing, ", ")
	}
	apimeta.SetStatusCondition(&co.Status.Conditions, ready)
	return ctrl.Result{}, r.Status().Patch(ctx, &co, patch)
}

// coordinatorProblems computes the reachable-Ready problems for one
// Coordinator. It is used both for the object's own reconcile and, nested,
// to decide whether the Coordinator behind an `agents[]` `coordinatorRef`
// entry is itself Ready — through this same function, so a directly
// reconciled Coordinator and one only reached through nesting are validated
// identically (design D-B).
//
// `visited` carries the set of Coordinator names already on the current
// path. A `coordinatorRef` naming one already in it is a STATIC cycle: it is
// reported as a problem rather than recursed into again, since that
// recursion would never terminate. This is the static counterpart to the
// live, `causedBy`-walking cycle guard `invoke` runs later (design D-E2).
func coordinatorProblems(ctx context.Context, c client.Reader, co *agentopsv1alpha1.Coordinator, visited map[string]bool) []string {
	var missing []string

	if co.Spec.AgentRef != nil {
		var capability agentopsv1alpha1.AgentCapability
		if err := c.Get(ctx, types.NamespacedName{Namespace: co.Namespace, Name: co.Spec.AgentRef.Name}, &capability); err != nil {
			missing = append(missing, "agentcapability/"+co.Spec.AgentRef.Name)
		} else if !apimeta.IsStatusConditionTrue(capability.Status.Conditions, "Ready") {
			missing = append(missing, "agentcapability/"+co.Spec.AgentRef.Name+notReadySuffix)
		}
	} else if co.Spec.ProfileRef == nil {
		missing = append(missing, "profileRef or capabilityRef")
	} else {
		missing = append(missing, validateCapabilitySpecRefs(ctx, c, co.Namespace, co.Spec.AgentCapabilitySpec)...)
	}

	for _, ref := range co.Spec.SignalSourceRefs {
		var src agentopsv1alpha1.SignalSource
		if err := c.Get(ctx, types.NamespacedName{Namespace: co.Namespace, Name: ref.Name}, &src); err != nil {
			missing = append(missing, "signalsource/"+ref.Name)
		}
	}
	for _, ref := range co.Spec.ChannelRefs {
		var ch agentopsv1alpha1.Channel
		if err := c.Get(ctx, types.NamespacedName{Namespace: co.Namespace, Name: ref.Name}, &ch); err != nil {
			missing = append(missing, "channel/"+ref.Name)
		}
	}

	for _, entry := range co.Spec.Agents {
		missing = append(missing, coordinatorEntryProblems(ctx, c, co.Namespace, entry, visited)...)
	}
	return missing
}

// coordinatorEntryProblems validates one `agents[]` entry: its ref exists,
// and what it names is itself Ready. A `coordinatorRef` recurses through
// coordinatorProblems, carrying `visited` forward.
func coordinatorEntryProblems(ctx context.Context, c client.Reader, namespace string,
	entry agentopsv1alpha1.CoordinatorAgentEntry, visited map[string]bool) []string {

	label := "agents[" + entry.Name + "]"
	switch {
	case entry.CapabilityRef != nil:
		var capability agentopsv1alpha1.AgentCapability
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: entry.CapabilityRef.Name}, &capability); err != nil {
			return []string{label + ": agentcapability/" + entry.CapabilityRef.Name}
		}
		if !apimeta.IsStatusConditionTrue(capability.Status.Conditions, "Ready") {
			return []string{label + ": agentcapability/" + entry.CapabilityRef.Name + notReadySuffix}
		}
		return nil
	case entry.CoordinatorRef != nil:
		name := entry.CoordinatorRef.Name
		if visited[name] {
			return []string{label + coordinatorRefLabel + name + " cycle"}
		}
		var nested agentopsv1alpha1.Coordinator
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &nested); err != nil {
			return []string{label + coordinatorRefLabel + name}
		}
		nestedVisited := make(map[string]bool, len(visited)+1)
		for k := range visited {
			nestedVisited[k] = true
		}
		nestedVisited[name] = true
		if nestedProblems := coordinatorProblems(ctx, c, &nested, nestedVisited); len(nestedProblems) > 0 {
			return []string{label + coordinatorRefLabel + name + notReadySuffix}
		}
		return nil
	default:
		// CEL already refuses an entry naming neither ref at admission; this
		// is unreachable against the API server and kept only for a fixture
		// or a fake client that skips validation.
		return []string{label + ": capabilityRef or coordinatorRef"}
	}
}

// coordinatorReferencedKinds are the kinds a Coordinator's wiring can name.
// An event on any of them may change some Coordinator's Ready, so each is
// watched with the same requeue-all mapping. Table-driven rather than one
// fluent `.Watches()` call per kind, so registering one more referenced kind
// is a slice entry, not a repeated builder line.
var coordinatorReferencedKinds = []client.Object{
	&agentopsv1alpha1.SignalSource{},
	&agentopsv1alpha1.Channel{},
	&agentopsv1alpha1.AgentProfile{},
	&agentopsv1alpha1.AgentRuntime{},
	&agentopsv1alpha1.MCPToolset{},
	&agentopsv1alpha1.MCPConfig{},
	&agentopsv1alpha1.AgentCapability{},
	&agentopsv1alpha1.Pipeline{},
}

// allCoordinatorRequests lists every Coordinator in a namespace as reconcile
// requests. Standalone rather than a closure so it is testable against a
// fake client with no manager and no envtest.
func allCoordinatorRequests(ctx context.Context, c client.Reader, namespace string) []ctrl.Request {
	var list agentopsv1alpha1.CoordinatorList
	if err := c.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil
	}
	var reqs []ctrl.Request
	for i := range list.Items {
		reqs = append(reqs, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}

// SetupWithManager wires the controller: coordinators, plus referenced-kind
// events mapped back to the coordinators naming them.
func (r *CoordinatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	mapAny := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []ctrl.Request {
		return allCoordinatorRequests(ctx, r.Client, obj.GetNamespace())
	})
	b := ctrl.NewControllerManagedBy(mgr).For(&agentopsv1alpha1.Coordinator{})
	for _, kind := range coordinatorReferencedKinds {
		b = b.Watches(kind, mapAny)
	}
	return b.Complete(r)
}

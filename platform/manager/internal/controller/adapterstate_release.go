package controller

import (
	"context"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// adapterStateAnnotationPrefix restates internal/state/kube's
// AdapterStateAnnotationPrefix. structure.md's module boundary keeps this
// package from importing internal/state/kube (the backend) at all, and the
// deleted httpapi.StateAnnotationPrefix is gone with the handlers that moved
// onto Stores.AdapterState (task 5.2) — so every holder of this one fact
// restates it under its own name, exactly as
// internal/state/kube/adapterstate_test.go's handlerPrefix already does.
// internal/state/static_test.go's
// TestStaticCheckGuardsTheAdapterStateAnnotationPrefix flags this literal
// anywhere outside this one file and the kube backend (design D-8).
const adapterStateAnnotationPrefix = "agentops.dev/adapter-state-"

// releaseAdapterStateKeys deletes every adapter-state key a deleted Channel
// or SignalSource carried, read off the object's OWN last-known annotations
// — the only record of which keys ever existed, since AdapterState declares
// no index to list them (internal/state/stores.go) and the object is
// already gone by the time an ordinary Get of it would report NotFound.
//
// obj is the tombstone handler.EnqueueRequestsFromMapFunc hands a MapFunc on
// a Delete event — the same mechanism MapRuntimePodToPending already reads a
// deleted pod's own labels through (conversation_controller.go). Reading a
// Channel's or SignalSource's own annotations this way is reading
// CONFIGURATION, not reaching around the state contract: both kinds stay on
// the Kubernetes API under every backend (state-store-generic-contract
// design, Non-Goals), so their own metadata is not the pluggable state this
// package is otherwise guarded away from.
//
// Under the kube backend this is a defensive no-op in practice: deleting the
// Channel/SignalSource takes its annotations — and so this same state —
// with it in the same stroke (design D-4: the adapter-state collection IS
// that annotation on that object). It exists for a backend that does not
// colocate the two, so a deleted CR never leaves an orphaned key an
// unrelated later CR of the same name could inherit.
func releaseAdapterStateKeys(ctx context.Context, coll state.Collection[string], surface string, obj client.Object) {
	if coll == nil || obj == nil {
		return
	}
	name := obj.GetName()
	for k := range obj.GetAnnotations() {
		suffix, ok := strings.CutPrefix(k, adapterStateAnnotationPrefix)
		if !ok || suffix == "" {
			continue
		}
		// Best effort: the object is already gone, so there is no condition
		// left to record a failure on and no caller waiting on an error —
		// logged for an operator reading manager logs, same posture as the
		// member-routing backstop logs beside it in server.go.
		if err := coll.Delete(ctx, state.AdapterStateKey(surface, name, suffix)); err != nil {
			log.FromContext(ctx).Info("releasing adapter state of a deleted object",
				"surface", surface, "name", name, "key", suffix, "error", err.Error())
		}
	}
}

// deleteOnlyPredicate restricts a Watches source to Delete events, the same
// shape MapRuntimePodToPending's own predicate already is
// (conversation_controller.go) — repeated here because
// builder.WithPredicates takes its own predicate.Funcs value per call.
func deleteOnlyPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return false },
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
	}
}

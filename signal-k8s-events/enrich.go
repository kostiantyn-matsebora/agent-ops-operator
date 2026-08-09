package main

// Signal enrichment: the labels an Event cannot carry on its own.
//
// The point of `workload` is grouping. The bundle used to group by
// [namespace, kind, name], where name is a POD name — unique per replica and
// regenerated on every rollout — so conversation count scaled with
// pods × rollouts and the manager's window reuse could never fire, because the
// signature never repeated. Resolving the owning controller instead gives a key
// that is stable across rollouts and shared by every replica.

// reservedLabels are the label keys this adapter defines. Pod labels are
// copied in alongside them but may never overwrite one: a user label named
// `name` or `namespace` would otherwise silently rewrite the signal's identity
// and, through signatureLabels, its grouping.
var reservedLabels = map[string]bool{
	"alertgroup": true,
	"alertname":  true,
	"namespace":  true,
	"kind":       true,
	"name":       true,
	"severity":   true,
	"source":     true,
	"workload":   true,
	"node":       true,
}

// enrichment is what the object cache can add to one event's signal.
type enrichment struct {
	// Workload is "<Kind>/<name>" of the owning controller, or empty when it
	// could not be resolved.
	Workload string
	Node     string
	Labels   map[string]string
}

// enrich resolves the involved object's workload, node, and labels.
//
// Two distinct "no answer" cases, deliberately handled differently:
//
//   - a kind the cache does NOT track (Node, PVC, Job, HPA, Ingress, any CRD)
//     is its OWN workload. There is no controller to find and no degradation
//     involved — a Node is a workload.
//
//   - a kind the cache DOES track but has no entry for (a pod that has not
//     landed yet, or a cold cache) yields NO workload label at all. Falling
//     back to the pod's own name here would silently reinstate exactly the
//     per-pod grouping this change exists to remove; leaving it empty groups
//     the unresolvable ones together, which errs toward fewer conversations
//     rather than more.
func (a *adapter) enrich(ev *Event) enrichment {
	kind, name, ns := ev.InvolvedObject.Kind, ev.InvolvedObject.Name, ev.Namespace()
	if a.cache == nil {
		return enrichment{}
	}
	if !a.cache.tracks(kind) {
		if kind != "" && name != "" {
			return enrichment{Workload: kind + "/" + name}
		}
		return enrichment{}
	}

	out := enrichment{}
	if wKind, wName, ok := a.cache.Workload(ns, kind, name); ok {
		out.Workload = wKind + "/" + wName
	}
	if oi, known := a.cache.Get(ns, kind, name); known && oi != nil {
		out.Node = oi.Node
		out.Labels = oi.Labels
	}
	return out
}

// applyTo writes the enrichment into a signal's label map, never overwriting a
// reserved key.
func (e enrichment) applyTo(labels map[string]string) {
	if e.Workload != "" {
		labels["workload"] = e.Workload
	}
	if e.Node != "" {
		labels["node"] = e.Node
	}
	for k, v := range e.Labels {
		if reservedLabels[k] {
			continue
		}
		labels[k] = v
	}
}

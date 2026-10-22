package main

// Signal enrichment: the labels an Event cannot carry on its own.
//
// The point of `workload` is correlation: the initiationId keys on it
// (events.go). Keying on the involved object instead — a POD name, unique per
// replica and regenerated on every rollout — made conversation count scale
// with pods × rollouts. Resolving the owning controller gives a key that is
// stable across rollouts and shared by every replica.

// reservedLabels are the label keys this adapter defines. Pod labels are
// copied in alongside them but may never overwrite one: a user label named
// `name`, `namespace` or `workload` would otherwise silently rewrite the
// signal's identity — and, through `workload`, its initiationId.
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
//   - a kind the cache does NOT track (PVC, Job, HPA, Ingress, any CRD) is its
//     OWN workload. There is no controller to find and no degradation
//     involved. Node is NOT in this list: it is tracked, for drain awareness
//     (drain.go) — but a Node still resolves to its own workload either way,
//     since it carries no owner reference to walk.
//
//   - a kind the cache DOES track but has no entry for (a pod that has not
//     landed yet, or a cold cache) yields NO workload label at all — never
//     the pod's own name dressed up as a workload. deliver holds such an
//     event for one bounded wait first (ownerhold.go), so it normally
//     arrives here resolved; one still unresolved when the wait expires is
//     keyed by normalize on the object itself, which is what the id means
//     for an object whose owner cannot be found.
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

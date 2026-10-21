package ingest

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// FilterRequirement is the matchExpressions-shaped test a caller asks this
// package to evaluate — field-for-field what
// agentopsv1alpha1.AttributeFilterRequirement carries, kept as its own type
// so this package stays independent of the API types (self-contained
// modules: structure.md).
type FilterRequirement struct {
	Key      string
	Operator string
	Values   []string
}

// Filter is requirements compiled once, ready to test many signals without
// re-converting or re-validating the same list per signal. A batch ingest
// call has one requirement list and many signals — compiling once and
// reusing is what keeps evaluating a whole batch linear in the number of
// signals rather than the signals times the requirement count.
type Filter struct {
	selector labels.Selector
	// malformed is set when the requirements could not build a valid
	// selector (see Admits's own fail-closed contract) — captured here
	// rather than returned from Compile so a caller gets one thing to hold
	// per filter, not a (*Filter, error) pair to thread through a loop.
	malformed bool
}

// Compile builds requirements into a Filter once. An empty list compiles to
// a Filter that admits everything, matching Admits's own empty-list case.
func Compile(requirements []FilterRequirement) *Filter {
	if len(requirements) == 0 {
		return &Filter{selector: labels.Everything()}
	}
	exprs := make([]metav1.LabelSelectorRequirement, 0, len(requirements))
	for _, r := range requirements {
		exprs = append(exprs, metav1.LabelSelectorRequirement{
			Key:      r.Key,
			Operator: metav1.LabelSelectorOperator(r.Operator),
			Values:   r.Values,
		})
	}
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchExpressions: exprs})
	if err != nil {
		return &Filter{malformed: true}
	}
	return &Filter{selector: selector}
}

// Admits reports whether attrs satisfies every requirement this Filter was
// compiled from — the same semantics a Kubernetes label selector applies,
// since each requirement was converted into a metav1.LabelSelectorRequirement
// and handed to metav1.LabelSelectorAsSelector/Selector.Matches rather than
// reimplemented.
//
// A requirement that failed to construct a valid Requirement at Compile time
// (e.g. In/NotIn with no values, or Exists/DoesNotExist with some) FAILS
// CLOSED here: the whole selector is treated as never-satisfied rather than
// skipping just that one requirement, so a malformed filter can only narrow
// admission, never widen it (design: "A malformed value/operator combination
// fails closed").
func (f *Filter) Admits(attrs map[string]string) bool {
	if f.malformed {
		return false
	}
	return f.selector.Matches(labels.Set(attrs))
}

// Admits is the single-shot convenience form of Compile(requirements).Admits
// (attrs) — same fail-closed contract, same empty-list "admits everything"
// case. Prefer Compile directly when testing many signals against one
// requirement list, since this re-compiles on every call.
func Admits(requirements []FilterRequirement, attrs map[string]string) bool {
	return Compile(requirements).Admits(attrs)
}

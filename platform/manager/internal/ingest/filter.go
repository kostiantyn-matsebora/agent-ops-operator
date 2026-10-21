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

// Admits reports whether labels satisfies every requirement — the same
// semantics a Kubernetes label selector applies, since each requirement is
// converted into a metav1.LabelSelectorRequirement and handed to
// metav1.LabelSelectorAsSelector/Selector.Matches rather than reimplemented.
//
// A requirement that fails to construct a valid Requirement (e.g. In/NotIn
// with no values, or Exists/DoesNotExist with some) FAILS CLOSED: the whole
// selector is treated as never-satisfied rather than skipping just that one
// requirement, so a malformed filter can only narrow admission, never widen
// it (design: "A malformed value/operator combination fails closed").
//
// An empty requirements list always admits — the empty-filter case is the
// caller's to special-case for the "zero behavior change" guarantee, but
// Admits itself already returns true for it since an empty selector matches
// everything.
func Admits(requirements []FilterRequirement, attrs map[string]string) bool {
	if len(requirements) == 0 {
		return true
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
		return false
	}
	return selector.Matches(labels.Set(attrs))
}

package integration

import (
	"context"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// signalsource-attribute-filter: "An unknown operator is rejected at
// creation" — the CRD's +kubebuilder:validation:Enum marker on
// AttributeFilterRequirement.Operator must be ENFORCED BY THE API SERVER,
// not merely documented. Admission-time enum enforcement is exactly what a
// unit test (which never talks to an API server) cannot exercise, so this
// is the one scenario from the delta spec that belongs in the envtest tier.
func TestSignalSourceRejectsUnknownAttributeFilterOperator(t *testing.T) {
	src := &agentopsv1alpha1.SignalSource{}
	src.Name, src.Namespace = "attrfilter-bad-operator", ns
	src.Spec.Adapter = "attrfilter-envtest"
	src.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{
		{Key: "team", Operator: "Frobnicate", Values: []string{"payments"}},
	}
	err := k8sClient.Create(context.Background(), src)
	if err == nil {
		t.Fatal("want the API server to reject an unknown attributeFilters[].operator, got no error")
	}
	if !apierrors.IsInvalid(err) {
		t.Fatalf("want a validation error, got %v", err)
	}
}

// A known operator, by contrast, is admitted.
func TestSignalSourceAcceptsKnownAttributeFilterOperators(t *testing.T) {
	for _, op := range []agentopsv1alpha1.AttributeFilterOperator{
		agentopsv1alpha1.AttributeFilterIn, agentopsv1alpha1.AttributeFilterNotIn,
		agentopsv1alpha1.AttributeFilterExists, agentopsv1alpha1.AttributeFilterDoesNotExist,
	} {
		t.Run(string(op), func(t *testing.T) {
			src := &agentopsv1alpha1.SignalSource{}
			src.Name, src.Namespace = "attrfilter-ok-"+strings.ToLower(string(op)), ns
			src.Spec.Adapter = "attrfilter-envtest"
			req := agentopsv1alpha1.AttributeFilterRequirement{Key: "team", Operator: op}
			if op == agentopsv1alpha1.AttributeFilterIn || op == agentopsv1alpha1.AttributeFilterNotIn {
				req.Values = []string{"payments"}
			}
			src.Spec.AttributeFilters = []agentopsv1alpha1.AttributeFilterRequirement{req}
			if err := k8sClient.Create(context.Background(), src); err != nil {
				t.Fatalf("operator %q must be accepted: %v", op, err)
			}
		})
	}
}

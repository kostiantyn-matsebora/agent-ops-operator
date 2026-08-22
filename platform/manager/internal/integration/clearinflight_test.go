package integration

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/api/v1alpha1"
)

// clearInflight releases the current run and queues another input, so a test
// can dispatch a second work unit from the same conversation.
func clearInflight(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Inflight = nil
	if err := k8sClient.Status().Patch(ctx, &conv, patch); err != nil {
		t.Fatal(err)
	}
	spec := client.MergeFrom(conv.DeepCopy())
	conv.Spec.Inputs = append(conv.Spec.Inputs,
		agentopsv1alpha1.InputItem{ID: "again-" + name, Type: agentopsv1alpha1.InputTask, Payload: "again"})
	if err := k8sClient.Patch(ctx, &conv, spec); err != nil {
		t.Fatal(err)
	}
}

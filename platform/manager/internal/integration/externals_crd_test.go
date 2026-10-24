package integration

import (
	"context"
	"strings"
	"testing"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

// The CRDs the chart ships admit externals on both adapter kinds, and the API
// server refuses a kind outside the declared vocabulary.
func TestAdapterCRDsAdmitExternals(t *testing.T) {
	ctx := context.Background()
	sa := &agentopsv1alpha1.SignalAdapter{}
	sa.Name, sa.Namespace = "externals-sa", ns
	sa.Spec.Image = "example/adapter:1"
	sa.Spec.Externals = []agentopsv1alpha1.ExternalRef{{Name: "Alertmanager", Kind: agentopsv1alpha1.ExternalSender}}
	if err := k8sClient.Create(ctx, sa); err != nil {
		t.Fatalf("a SignalAdapter declaring a sender must be admitted: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, sa) })

	ca := &agentopsv1alpha1.ChannelAdapter{}
	ca.Name, ca.Namespace = "externals-ca", ns
	ca.Spec.Image = "example/adapter:1"
	ca.Spec.Externals = []agentopsv1alpha1.ExternalRef{{Name: "Telegram Bot API", Kind: agentopsv1alpha1.ExternalAPI}}
	if err := k8sClient.Create(ctx, ca); err != nil {
		t.Fatalf("a ChannelAdapter declaring an api must be admitted: %v", err)
	}
	t.Cleanup(func() { _ = k8sClient.Delete(ctx, ca) })

	var got agentopsv1alpha1.ChannelAdapter
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(ca), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Externals) != 1 || got.Spec.Externals[0].Name != "Telegram Bot API" {
		t.Fatalf("externals must survive the round trip, not be pruned: %+v", got.Spec.Externals)
	}

	bad := &agentopsv1alpha1.SignalAdapter{}
	bad.Name, bad.Namespace = "externals-bad", ns
	bad.Spec.Image = "example/adapter:1"
	bad.Spec.Externals = []agentopsv1alpha1.ExternalRef{{Name: "Somewhere", Kind: "database"}}
	err := k8sClient.Create(ctx, bad)
	if err == nil {
		_ = k8sClient.Delete(ctx, bad)
		t.Fatal("an external kind outside sender, api and kubernetes must be refused")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Fatalf("the refusal names the field: %v", err)
	}
}

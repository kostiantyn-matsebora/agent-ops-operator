package httpapi

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
)

func causedConv(name, parent string) *agentopsv1alpha1.Conversation {
	c := &agentopsv1alpha1.Conversation{}
	c.Name, c.Namespace = name, "agent-ops"
	if parent != "" {
		c.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: parent}
		c.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: parent}
	}
	return c
}

func conversationExists(t *testing.T, c client.Client, name string) bool {
	t.Helper()
	err := c.Get(context.Background(), client.ObjectKey{Namespace: "agent-ops", Name: name}, &agentopsv1alpha1.Conversation{})
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	return err == nil
}

func TestCascadeDeleteMembersDeletesTheSubtreeAndTrustsTheFieldOverTheLabel(t *testing.T) {
	decoy := causedConv("decoy", "other")
	decoy.Labels[agentopsv1alpha1.LabelCausedBy] = "root" // label says root, field says other
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(
		causedConv("root", ""), causedConv("mid", "root"), causedConv("leaf", "mid"), decoy,
	).Build()
	s := &Server{Client: c, Reader: c, Namespace: "agent-ops"}

	if err := s.cascadeDeleteMembers(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"mid", "leaf"} {
		if conversationExists(t, c, gone) {
			t.Errorf("%s survived the cascade", gone)
		}
	}
	for _, kept := range []string{"root", "decoy"} {
		if !conversationExists(t, c, kept) {
			t.Errorf("%s was deleted though the cascade must not touch it", kept)
		}
	}
}

func TestCascadeDeleteMembersToleratesNotFoundAndReportsOtherErrors(t *testing.T) {
	build := func(del func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error) *Server {
		c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).
			WithObjects(causedConv("m", "root")).
			WithInterceptorFuncs(interceptor.Funcs{Delete: del}).Build()
		return &Server{Client: c, Reader: c, Namespace: "agent-ops"}
	}

	gone := build(func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
		return apierrors.NewNotFound(schema.GroupResource{Resource: "conversations"}, "m")
	})
	if err := gone.cascadeDeleteMembers(context.Background(), "root"); err != nil {
		t.Fatalf("NotFound must be tolerated: %v", err)
	}

	boom := errors.New("boom")
	failing := build(func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error { return boom })
	if err := failing.cascadeDeleteMembers(context.Background(), "root"); !errors.Is(err, boom) {
		t.Fatalf("a delete failure must surface, got %v", err)
	}
}

func TestCascadeDeleteMembersReportsAListFailure(t *testing.T) {
	boom := errors.New("list failed")
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error { return boom },
	}).Build()
	s := &Server{Client: c, Reader: c, Namespace: "agent-ops"}
	if err := s.cascadeDeleteMembers(context.Background(), "root"); !errors.Is(err, boom) {
		t.Fatalf("a list failure must surface, got %v", err)
	}
}

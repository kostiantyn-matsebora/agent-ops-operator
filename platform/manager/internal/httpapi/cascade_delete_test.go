package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
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

// routerFor wires a bare chat.Router over c, the one dependency
// deleteCascadeRequests needs (Router.Descendants) that cascade_delete_test.go
// used not to need back when the cascade walked the tree itself.
func routerFor(t *testing.T, c client.WithWatch, ns string) *chat.Router {
	t.Helper()
	store := testConversations(t, c, ns)
	return &chat.Router{Client: c, Reader: c, Conversations: store, Namespace: ns}
}

// async-command-bus task 5.10: deleteCascadeOrder is pure — no client, no
// bus — so the deepest-first/root-last ordering is tested directly, against
// a flat descendant list exactly as chat.Router.Descendants returns one
// (any order, trusted only through CausedBy.Parent).
func TestDeleteCascadeOrderIsDeepestFirstRootLast(t *testing.T) {
	descendants := []state.ConversationRecord{
		{Name: "grandchild", CausedBy: &state.Provenance{Parent: "mid"}},
		{Name: "mid", CausedBy: &state.Provenance{Parent: "root"}},
		{Name: "other-grandchild", CausedBy: &state.Provenance{Parent: "mid"}},
	}
	got := deleteCascadeOrder("root", descendants)
	if len(got) != 4 || got[len(got)-1] != "root" {
		t.Fatalf("want 4 names ending with root, got %v", got)
	}
	pos := map[string]int{}
	for i, n := range got {
		pos[n] = i
	}
	if pos["grandchild"] >= pos["mid"] || pos["other-grandchild"] >= pos["mid"] {
		t.Fatalf("a child must precede its own parent, got %v", got)
	}
	if pos["mid"] >= pos["root"] {
		t.Fatalf("root must be last, got %v", got)
	}
}

// A decoy whose LABEL claims "root" but whose CausedBy.Parent field names a
// different conversation must be ordered (and later deleted) under its
// TRUE parent — the label is a hint for Descendants' own index, the field
// is the fact deleteCascadeOrder trusts.
func TestDeleteCascadeOrderTrustsTheFieldOverTheLabel(t *testing.T) {
	decoy := state.ConversationRecord{Name: "decoy", CausedBy: &state.Provenance{Parent: "other"}}
	got := deleteCascadeOrder("root", []state.ConversationRecord{decoy})
	if len(got) != 1 || got[0] != "root" {
		t.Fatalf("a decoy naming a DIFFERENT parent must never appear under root's own order, got %v", got)
	}
}

func TestDeleteCascadeRequestsBuildsOneTaskPerConversationKeyedOnTheRoot(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(
		causedConv("root", ""), causedConv("mid", "root"), causedConv("leaf", "mid"),
	).Build()
	s := &Server{Client: c, Reader: c, Conversations: testConversations(t, c, "agent-ops"),
		Namespace: "agent-ops", Router: routerFor(t, c, "agent-ops")}

	reqs, err := s.deleteCascadeRequests(context.Background(), "root")
	if err != nil {
		t.Fatal(err)
	}
	if len(reqs) != 3 {
		t.Fatalf("want one task per conversation in the subtree (3), got %d", len(reqs))
	}
	for _, r := range reqs {
		if r.Queue != ConvQueue {
			t.Fatalf("every cascade task must be on %q, got %q", ConvQueue, r.Queue)
		}
		if string(r.Key) != "root" {
			t.Fatalf("every cascade task must share the root's own name as Key, got %q", r.Key)
		}
	}
	// deepest-first, root last — "leaf" and "mid" strictly precede "root".
	order := make([]string, len(reqs))
	for i, r := range reqs {
		var p convtask.Payload
		if err := json.Unmarshal(r.Payload, &p); err != nil {
			t.Fatal(err)
		}
		order[i] = p.Conversation
	}
	if order[len(order)-1] != "root" {
		t.Fatalf("root must be the LAST task, got order %v", order)
	}
}

// taskDelete (async-command-bus design D-6/task 5.7): a conversation already
// gone — another cascade member, a racing delete — SUCCEEDS rather than
// going dead on not_found.
func TestTaskDeleteOfAnAbsentConversationSucceeds(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).Build()
	s := &Server{Client: c, Reader: c, Conversations: testConversations(t, c, "agent-ops"), Namespace: "agent-ops"}
	if err := s.taskDelete(context.Background(), &convtask.Payload{Verb: convtask.VerbDelete, Conversation: "never-existed"}); err != nil {
		t.Fatalf("deleting an absent conversation must succeed, got %v", err)
	}
}

func TestTaskDeleteRemovesTheNamedConversation(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(causedConv("m", "")).Build()
	s := &Server{Client: c, Reader: c, Conversations: testConversations(t, c, "agent-ops"), Namespace: "agent-ops"}
	if err := s.taskDelete(context.Background(), &convtask.Payload{Verb: convtask.VerbDelete, Conversation: "m"}); err != nil {
		t.Fatal(err)
	}
	if conversationExists(t, c, "m") {
		t.Fatal("m must be gone")
	}
}

func TestTaskDeleteReportsAnUnrecognizedStoreFailure(t *testing.T) {
	boom := errors.New("boom")
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(causedConv("m", "")).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error { return boom },
		}).Build()
	s := &Server{Client: c, Reader: c, Conversations: testConversations(t, c, "agent-ops"), Namespace: "agent-ops"}
	if err := s.taskDelete(context.Background(), &convtask.Payload{Verb: convtask.VerbDelete, Conversation: "m"}); !errors.Is(err, boom) {
		t.Fatalf("a delete failure must surface, got %v", err)
	}
}

func TestDeleteCascadeRequestsReportsADescendantsFailure(t *testing.T) {
	boom := errors.New("list failed")
	c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithInterceptorFuncs(interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error { return boom },
	}).Build()
	s := &Server{Client: c, Reader: c, Conversations: testConversations(t, c, "agent-ops"),
		Namespace: "agent-ops", Router: routerFor(t, c, "agent-ops")}
	if _, err := s.deleteCascadeRequests(context.Background(), "root"); !errors.Is(err, boom) {
		t.Fatalf("a list failure must surface, got %v", err)
	}
}


// A name reused by a later conversation must not have its delete swallowed as
// a repeat of the previous one's: the command id carries the record's ID.
func TestDeleteCommandIDDiffersAcrossTwoLivesOfOneName(t *testing.T) {
	idFor := func(uid string) commandbus.CommandID {
		conv := causedConv("root", "")
		conv.UID = types.UID(uid)
		c := fake.NewClientBuilder().WithScheme(stateTestScheme(t)).WithObjects(conv).Build()
		s := &Server{Client: c, Reader: c, Conversations: testConversations(t, c, "agent-ops"),
			Namespace: "agent-ops", Router: routerFor(t, c, "agent-ops")}
		reqs, err := s.deleteCascadeRequests(context.Background(), "root")
		if err != nil || len(reqs) != 1 {
			t.Fatalf("requests: %v %v", reqs, err)
		}
		return reqs[0].CommandID
	}
	first, second := idFor("life-one"), idFor("life-two")
	if first == second {
		t.Fatalf("two lives of one name share the delete id %q", first)
	}
}

// async-command-bus task 5.4: closeBudgetExceeded and autoCloseConversation
// ENQUEUE a "conv" task when Bus is set, and fall back to calling
// chat.Router directly, inline, when it is not — every OTHER reconciler
// test in this package builds a ConversationReconciler with no Bus at all,
// and must keep working unchanged.
package controller

import (
	"context"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// routerOver builds a bare chat.Router over store (and c, for the Channel
// lookups a farewell's eachBoundThread performs — none here, since
// claimTestConv binds no channel) — enough for CloseBudgetExceeded/
// AutoCloseConversation's own farewell+patch, the inline fallback path
// these tests confirm is unchanged when Bus is nil.
func routerOver(c client.Client, store state.Collection[state.ConversationRecord]) *chat.Router {
	q := &chat.OpQueue{Conversations: store, Registry: chat.NewRegistry(), Bus: memory.Open()}
	return &chat.Router{Client: c, Reader: c, Conversations: store, Namespace: claimTestNS, Ops: q}
}

func TestCloseBudgetExceededEnqueuesAConvTaskWhenBusIsSet(t *testing.T) {
	c := fakeClaimClient(t, claimTestConv("c-1")).Build()
	store := claimStore(c)
	bus := memory.Open()
	r := &ConversationReconciler{Conversations: store, Bus: bus, Router: routerOver(c, store)}
	conv := claimRec(t, store, "c-1")

	if err := r.closeBudgetExceeded(context.Background(), conv, "budget-exceeded: deadline passed"); err != nil {
		t.Fatal(err)
	}
	// c-1 has no causedBy descendants, so the cascade (design D-7, task 5.3)
	// submits exactly ONE request — its own unchanged top-level task.
	task, err := bus.Claim(context.Background(), convtask.Queue, "test")
	if err != nil {
		t.Fatal(err)
	}
	p, err := convtask.Decode(task.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if p.Verb != convtask.VerbClose || p.CloseKind != convtask.CloseBudgetExceeded || p.Conversation != "c-1" {
		t.Fatalf("want a close/budget-exceeded task for c-1, got %+v", p)
	}
	if task.Key != commandbus.Key("c-1") {
		t.Fatalf("want the conversation's own name as Key, got %q", task.Key)
	}
}

func TestAutoCloseConversationEnqueuesAConvTaskWhenBusIsSet(t *testing.T) {
	c := fakeClaimClient(t, claimTestConv("c-1")).Build()
	store := claimStore(c)
	bus := memory.Open()
	r := &ConversationReconciler{Conversations: store, Bus: bus, Router: routerOver(c, store)}
	conv := claimRec(t, store, "c-1")

	if err := r.autoCloseConversation(context.Background(), conv, 90*time.Minute); err != nil {
		t.Fatal(err)
	}
	task, err := bus.Claim(context.Background(), convtask.Queue, "test")
	if err != nil {
		t.Fatal(err)
	}
	p, err := convtask.Decode(task.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if p.Verb != convtask.VerbClose || p.CloseKind != convtask.CloseAuto || p.Conversation != "c-1" {
		t.Fatalf("want a close/auto task for c-1, got %+v", p)
	}
	if time.Duration(p.IdleForMS)*time.Millisecond != 90*time.Minute {
		t.Fatalf("want the idle window round-tripped, got %dms", p.IdleForMS)
	}
}

// A conversation with LIVE causedBy members enqueues one CascadeMember task
// per member, deepest-first, ahead of the root's own request, all sharing
// the ROOT's name as Key (design D-7, task 5.3) — this package's own
// analogue of httpapi's TestDeleteCascadeRequestsBuildsOneTaskPerConversationKeyedOnTheRoot.
func TestCloseBudgetExceededEnqueuesTheCascadeMembersBeforeTheRoot(t *testing.T) {
	root := claimTestConv("root-1")
	member := claimTestConv("member-1")
	member.Spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: "root-1", Entry: "worker"}
	member.Labels = map[string]string{agentopsv1alpha1.LabelCausedBy: "root-1"}
	c := fakeClaimClient(t, root, member).Build()
	store := claimStore(c)
	bus := memory.Open()
	r := &ConversationReconciler{Conversations: store, Bus: bus, Router: routerOver(c, store)}
	conv := claimRec(t, store, "root-1")

	if err := r.closeBudgetExceeded(context.Background(), conv, "budget-exceeded: deadline passed"); err != nil {
		t.Fatal(err)
	}
	first, err := bus.Claim(context.Background(), convtask.Queue, "test")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := convtask.Decode(first.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if !fp.CascadeMember || fp.Conversation != "member-1" {
		t.Fatalf("want member-1's own CascadeMember task first, got %+v", fp)
	}
	if first.Key != commandbus.Key("root-1") {
		t.Fatalf("want the ROOT's own name as Key, got %q", first.Key)
	}
	// member-1's task settles before root-1's becomes claimable — they share
	// a Key, so the bus serializes them in submission order.
	if err := bus.Settle(context.Background(), first.UID, first.Lease.Token); err != nil {
		t.Fatal(err)
	}
	second, err := bus.Claim(context.Background(), convtask.Queue, "test")
	if err != nil {
		t.Fatal(err)
	}
	sp, err := convtask.Decode(second.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if sp.CascadeMember || sp.Conversation != "root-1" || sp.CloseKind != convtask.CloseBudgetExceeded {
		t.Fatalf("want root-1's own close/budget-exceeded task last, got %+v", sp)
	}
}

// Bus nil (every other test in this package, and any reconciler built
// without one) keeps the EXACT inline behaviour this change found: the
// Router call happens synchronously, nothing is submitted anywhere.
func TestCloseBudgetExceededFallsBackToTheRouterWhenBusIsNil(t *testing.T) {
	c := fakeClaimClient(t, claimTestConv("c-1")).Build()
	store := claimStore(c)
	r := &ConversationReconciler{Conversations: store, Router: routerOver(c, store)}
	conv := claimRec(t, store, "c-1")

	if err := r.closeBudgetExceeded(context.Background(), conv, "digest"); err != nil {
		t.Fatal(err)
	}
	// conv is uncaused: CloseBudgetExceeded's one closing path for an
	// uncaused root is Escalate, not a close — design D-D/D-E.
	got := claimRec(t, store, "c-1")
	if got.EscalatedAt == nil || got.EscalationMessage != "digest" {
		t.Fatalf("with no Bus, the Router must run inline, got %+v", got)
	}
}

func TestAutoCloseConversationFallsBackToTheRouterWhenBusIsNil(t *testing.T) {
	c := fakeClaimClient(t, claimTestConv("c-1")).Build()
	store := claimStore(c)
	r := &ConversationReconciler{Conversations: store, Router: routerOver(c, store)}
	conv := claimRec(t, store, "c-1")

	if err := r.autoCloseConversation(context.Background(), conv, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	got := claimRec(t, store, "c-1")
	if got.Phase != state.PhaseClosed {
		t.Fatalf("with no Bus, the Router must close inline, got phase %q", got.Phase)
	}
}

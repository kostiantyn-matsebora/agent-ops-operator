// /exit against a real API server: the half of capacity release that automatic
// eviction cannot serve. Eviction only runs when something is WAITING; with
// nothing waiting, an idle pod holds its slot until the idle TTL expires.
//
// These two tests pin the pair of claims that make the command worth having —
// the slot is genuinely free (a Pending conversation is admitted without any
// TTL passing), and the conversation is genuinely intact (its next input gets a
// fresh pod and its context handle is still there).
package integration

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/httpapi"
)

// clearCapacityPool empties the namespace's WAITING SET before a capacity test.
//
// Admission is FIFO over every conversation in the namespace that needs a worker
// and holds no pod — so leftover fixtures from earlier tests, which nothing in
// this suite reclaims, sit ahead of anything created here and starve it. That is
// the shared-pool problem clearRuntimePods already solves for pods; the waiting
// set is its other half, and a capacity test needs both to be exclusive.
//
// Only abandoned conversations are removed: those still owed work with no pod
// backing them. Tests run sequentially, so these belong to runs that have
// finished.
func clearCapacityPool(t *testing.T) {
	t.Helper()
	clearRuntimePods(t)
	ctx := context.Background()
	var list agentopsv1alpha1.ConversationList
	if err := k8sClient.List(ctx, &list, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	for i := range list.Items {
		c := list.Items[i]
		if len(c.Spec.Inputs) == 0 || c.Status.Phase == agentopsv1alpha1.ConversationClosed {
			continue
		}
		_ = k8sClient.Delete(ctx, &c)
		dropCloseFinalizer(t, c.Name)
	}
}

// makeIdleOnThread gives a conversation a thread on a channel and marks its inputs
// processed — the state a finished conversation is in, and the only state from
// which /exit acts.
func makeIdleOnThread(t *testing.T, name, channel, thread, contextID string) {
	t.Helper()
	ctx := context.Background()
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, &conv); err != nil {
		t.Fatal(err)
	}
	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.Threads = []agentopsv1alpha1.ThreadBinding{{Channel: channel, ThreadID: thread}}
	for _, in := range conv.Spec.Inputs {
		conv.Status.ProcessedInputIDs = append(conv.Status.ProcessedInputIDs, in.ID)
	}
	conv.Status.RuntimeContextID = contextID
	if err := k8sClient.Status().Patch(ctx, &conv, patch); err != nil {
		t.Fatal(err)
	}
}

func sendExit(t *testing.T, srv *httpapi.Server, channel, thread string) {
	t.Helper()
	var ch agentopsv1alpha1.Channel
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: channel}, &ch); err != nil {
		t.Fatal(err)
	}
	if err := srv.Router.HandleMessage(context.Background(), &ch,
		chat.InboundMessage{ThreadID: &thread, Text: "/exit"}); err != nil {
		t.Fatal(err)
	}
}

// The claim that justifies the command, and the SHAPE OF IT MATTERS: with
// nothing waiting for capacity, nothing evicts, so the idle pod holds its slot,
// its checkout and whatever its runtime keeps resident until the idle TTL
// expires. /exit is the only way to end that interval early.
//
// Note what this test deliberately does NOT claim. Automatic eviction already
// frees an idle pod for a conversation that IS waiting — createRuntimePod evicts
// the longest-idle evictable pod at the cap — so "a pending conversation gets
// admitted" would have been true before this change and proves nothing about
// it. The property here is the one eviction cannot supply: the pod is gone at
// the moment it is asked for, with no waiter and no TTL involved.
func TestExitReleasesTheSlotWithNothingWaiting(t *testing.T) {
	clearCapacityPool(t)
	mkProfile(t, "prof-exit")
	mkChannel(t, "chan-exit", "tg-exit")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 1)

	mkChanConv(t, "exit-active", "prof-exit", "chan-exit")
	reconcileWith(t, rc, "exit-active")
	if !podExists(t, "exit-active") {
		t.Fatal("setup: the conversation should hold the only slot")
	}
	makeIdleOnThread(t, "exit-active", "chan-exit", "thread-exit", "ctx-1")

	sendExit(t, srv, "chan-exit", "thread-exit")

	if podExists(t, "exit-active") {
		t.Fatal("/exit must delete the runtime pod immediately")
	}
	// still there, still not closed: that is the entire difference from /close
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "exit-active"}, &conv); err != nil {
		t.Fatalf("/exit must not delete the conversation: %v", err)
	}
	if conv.Status.Phase == agentopsv1alpha1.ConversationClosed {
		t.Fatal("/exit must not close the conversation")
	}
	if len(conv.Status.Threads) != 1 {
		t.Fatalf("thread bindings must survive: %+v", conv.Status.Threads)
	}

	// the freed slot is real: the next conversation is admitted outright rather
	// than parked, and no eviction had to run to make room for it
	mkChanConv(t, "exit-next", "prof-exit", "chan-exit")
	reconcileWith(t, rc, "exit-next")
	if got := phaseOf(t, "exit-next"); got == agentopsv1alpha1.ConversationPending {
		t.Fatal("the released slot was not free")
	}
	if !podExists(t, "exit-next") {
		t.Fatal("the next conversation must get the released slot")
	}
}

// A release must not disturb the admission path it borrows. Eviction would have
// promoted this waiter on its own, so this is a NON-REGRESSION check on the
// interaction, not evidence for the feature.
func TestExitLeavesFIFOPromotionIntact(t *testing.T) {
	clearCapacityPool(t)
	mkProfile(t, "prof-fifoexit")
	mkChannel(t, "chan-fifoexit", "tg-fifoexit")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 1)

	mkChanConv(t, "fifoexit-active", "prof-fifoexit", "chan-fifoexit")
	reconcileWith(t, rc, "fifoexit-active")
	mkChanConv(t, "fifoexit-waiting", "prof-fifoexit", "chan-fifoexit")
	reconcileWith(t, rc, "fifoexit-waiting")
	if got := phaseOf(t, "fifoexit-waiting"); got != agentopsv1alpha1.ConversationPending {
		t.Fatalf("setup: waiting conversation phase = %q, want Pending", got)
	}

	makeIdleOnThread(t, "fifoexit-active", "chan-fifoexit", "thread-fifoexit", "ctx-2")
	sendExit(t, srv, "chan-fifoexit", "thread-fifoexit")

	reconcileWith(t, rc, "fifoexit-waiting")
	if got := phaseOf(t, "fifoexit-waiting"); got == agentopsv1alpha1.ConversationPending {
		t.Fatal("the waiting conversation was not admitted after the release")
	}
	if !podExists(t, "fifoexit-waiting") {
		t.Fatal("the admitted conversation must get a runtime pod")
	}
}

// A release is not an ending: the next input provisions a fresh pod and the
// context handle it resumes from is the one that was there before.
func TestConversationResumesAfterExit(t *testing.T) {
	clearCapacityPool(t)
	mkProfile(t, "prof-resume")
	mkChannel(t, "chan-resume", "tg-resume")
	srv := apiServer()
	rc := reconcilerWithCap(srv.Ops, 5)

	mkChanConv(t, "resume-1", "prof-resume", "chan-resume")
	reconcileWith(t, rc, "resume-1")
	makeIdleOnThread(t, "resume-1", "chan-resume", "thread-resume", "ctx-keep")

	sendExit(t, srv, "chan-resume", "thread-resume")
	if podExists(t, "resume-1") {
		t.Fatal("setup: the pod should have been released")
	}

	// an ordinary reply on the same thread
	var ch agentopsv1alpha1.Channel
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "chan-resume"}, &ch); err != nil {
		t.Fatal(err)
	}
	thread := "thread-resume"
	if err := srv.Router.HandleMessage(context.Background(), &ch,
		chat.InboundMessage{ThreadID: &thread, Text: "and now check the disks"}); err != nil {
		t.Fatal(err)
	}
	reconcileWith(t, rc, "resume-1")

	if !podExists(t, "resume-1") {
		t.Fatal("the next input must provision a fresh runtime pod")
	}
	var conv agentopsv1alpha1.Conversation
	if err := k8sClient.Get(context.Background(),
		types.NamespacedName{Namespace: ns, Name: "resume-1"}, &conv); err != nil {
		t.Fatal(err)
	}
	if conv.ContextID() != "ctx-keep" {
		t.Fatalf("the context handle must be unchanged by the release: %q", conv.ContextID())
	}
}

package integration

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
)

// testConsumer builds the Signal consumer the way main.go does, over the
// API server last built by apiServer(): the same Ops, activity log, cooldown
// map and runtime config the HTTP surface was given.
func testConsumer() *controller.SignalReconciler {
	s := lastServer
	maxQueued := s.MaxQueuedConversations
	if maxQueued <= 0 {
		maxQueued = 50
	}
	return &controller.SignalReconciler{
		Client: k8sClient, Reader: k8sClient, Activity: s.Activity, Ops: s.Ops, Cooldowns: s.Cooldowns,
		Runtime: s.Runtime, MaxQueued: maxQueued, Identity: "integration-replica",
		OrderRetry: time.Millisecond,
	}
}

func signalNames() map[string]bool {
	var list agentopsv1alpha1.SignalList
	if err := k8sClient.List(context.Background(), &list, client.InNamespace(ns)); err != nil {
		panic(err)
	}
	out := map[string]bool{}
	for i := range list.Items {
		out[list.Items[i].Name] = true
	}
	return out
}

// consumeNewSignals stands in for the leader's consumer pool: it reconciles
// every Signal created since `before` until each is terminal or only waiting,
// (envtest runs no garbage collector, and every test names its own sources, so
// what a test leaves behind is inert to the others).
func consumeNewSignals(before map[string]bool) {
	var fresh []string
	for name := range signalNames() {
		if !before[name] {
			fresh = append(fresh, name)
		}
	}
	sort.Strings(fresh)
	consumer := testConsumer()
	for round := 0; round < 25; round++ {
		progressed := false
		for _, name := range fresh {
			var sig agentopsv1alpha1.Signal
			if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &sig); err != nil {
				continue
			}
			if sig.Status.Phase == agentopsv1alpha1.SignalRouted || sig.Status.Phase == agentopsv1alpha1.SignalDropped ||
				sig.Status.Phase == agentopsv1alpha1.SignalFailed {
				continue
			}
			rv := sig.ResourceVersion
			if _, err := consumer.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: name}}); err != nil {
				panic(fmt.Sprintf("consume %s: %v", name, err))
			}
			if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: name}, &sig); err == nil && sig.ResourceVersion != rv {
				progressed = true
			}
		}
		if !progressed {
			return
		}
	}
}

// inputText is the message an input carries: inline, or — for everything the
// consumer routes — read from the ConversationInput the queue entry cites.
func inputText(t *testing.T, in agentopsv1alpha1.InputItem) string {
	t.Helper()
	if in.PayloadRef == nil {
		return in.Payload
	}
	var ci agentopsv1alpha1.ConversationInput
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: in.PayloadRef.Name}, &ci); err != nil {
		t.Fatalf("read input %s: %v", in.PayloadRef.Name, err)
	}
	return ci.Spec.Payload
}

// inputSender is who typed an input, from whichever lane carried it.
func inputSender(t *testing.T, in agentopsv1alpha1.InputItem) string {
	t.Helper()
	if in.PayloadRef == nil {
		return in.OriginSender(nil)
	}
	var ci agentopsv1alpha1.ConversationInput
	if err := k8sClient.Get(context.Background(), types.NamespacedName{Namespace: ns, Name: in.PayloadRef.Name}, &ci); err != nil {
		t.Fatalf("read input %s: %v", in.PayloadRef.Name, err)
	}
	return in.OriginSender(ci.Spec.Labels)
}

// droppedForCooldown counts the stored Signals of one initiationId the
// consumer dropped as inside the source's cooldown.
func droppedForCooldown(t *testing.T, initiationID string) int {
	t.Helper()
	var list agentopsv1alpha1.SignalList
	if err := k8sClient.List(context.Background(), &list, client.InNamespace(ns)); err != nil {
		t.Fatal(err)
	}
	n := 0
	for i := range list.Items {
		s := &list.Items[i]
		if s.Spec.InitiationID == initiationID && s.Status.Phase == agentopsv1alpha1.SignalDropped &&
			s.Status.Reason == agentopsv1alpha1.SignalDropCooldown {
			n++
		}
	}
	return n
}

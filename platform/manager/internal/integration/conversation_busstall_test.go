package integration

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
)

// stalledBus accepts nothing: every Submit waits until its context ends, the
// way the real bus behaves while its mirror cannot catch up.
type stalledBus struct{ commandbus.Bus }

func (stalledBus) Submit(ctx context.Context, _ commandbus.SubmitRequest) (commandbus.Task, error) {
	<-ctx.Done()
	return commandbus.Task{}, ctx.Err()
}

// Starting the agent must not wait on posting to a chat channel. A command bus
// that accepts nothing used to hold every reconcile for its whole deadline
// before the pod was created, so no conversation ever started although every
// slot was free.
func TestAStalledCommandBusDoesNotStopAConversationFromStarting(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-stall")
	mkChannel(t, "chan-stall", "tg-stall")
	mkChanConv(t, "stall-one", "prof-stall", "chan-stall")

	ops := &chat.OpQueue{Conversations: stores.Conversations, Runs: stores.Runs,
		Registry: chat.NewRegistry(), Bus: stalledBus{memory.Open()}}
	rc := reconcilerWithCap(ops, 100) // the suite leaves older waiting conversations ahead in FIFO order
	rc.ReconcileTimeout = 5 * time.Second
	rc.ChatBudget = 500 * time.Millisecond

	start := time.Now()
	_, _ = rc.Reconcile(context.Background(),
		ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "stall-one"}})

	if !podExists(t, "stall-one") {
		t.Fatalf("no runtime pod after %s: the start waited on a bus that accepts nothing", time.Since(start).Round(time.Millisecond))
	}
}

package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// hangingRuns is a state store whose read of ONE conversation never returns
// until its context ends: the shape of a call waiting on a store that never
// answers.
type hangingRuns struct {
	state.Appended[state.ConversationRecord, state.Run]
	hang string
}

func (h hangingRuns) GetWithItems(ctx context.Context, k state.Key) (state.ConversationRecord, state.Revision, []state.Run, error) {
	if string(k) == h.hang {
		<-ctx.Done()
		var rev state.Revision
		return state.ConversationRecord{}, rev, nil, ctx.Err()
	}
	return h.Appended.GetWithItems(ctx, k)
}

// TestAHungStoreCallDoesNotHoldAReconcileForever: a reconcile has a deadline,
// so a store call that never answers becomes an error the queue retries
// instead of a worker held for good.
func TestAHungStoreCallDoesNotHoldAReconcileForever(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-hang")
	mkChannel(t, "chan-hang", "tg-hang")
	mkChanConv(t, "hang-one", "prof-hang", "chan-hang")

	rc := reconcilerWithCap(apiServer().Ops, 5)
	rc.Runs = hangingRuns{Appended: rc.Runs, hang: "hang-one"}
	rc.ReconcileTimeout = 300 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		_, err := rc.Reconcile(context.Background(),
			ctrl.Request{NamespacedName: types.NamespacedName{Namespace: ns, Name: "hang-one"}})
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a reconcile that hit its deadline must return an error so it is retried")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a hung store call held the reconcile far past its deadline")
	}
}

// TestConcurrentAdmissionNeverExceedsTheCap: with many reconciles running at
// once, the active-conversation cap still holds.
func TestConcurrentAdmissionNeverExceedsTheCap(t *testing.T) {
	clearRuntimePods(t)
	mkProfile(t, "prof-conc")
	mkChannel(t, "chan-conc", "tg-conc")
	const cap, n = 2, 8
	rc := reconcilerWithCap(apiServer().Ops, cap)

	for i := 0; i < n; i++ {
		mkChanConv(t, fmt.Sprintf("conc-%d", i), "prof-conc", "chan-conc")
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = rc.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{
				Namespace: ns, Name: fmt.Sprintf("conc-%d", i)}})
		}(i)
	}
	wg.Wait()

	pods := 0
	for i := 0; i < n; i++ {
		if podExists(t, fmt.Sprintf("conc-%d", i)) {
			pods++
		}
	}
	if pods > cap {
		t.Fatalf("%d runtime pods with a cap of %d", pods, cap)
	}
}

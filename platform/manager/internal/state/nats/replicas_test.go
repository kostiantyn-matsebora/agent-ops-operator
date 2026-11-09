package nats

import (
	"context"
	"testing"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// Two managers share one state server. Whatever the leader writes, a replica
// that is not the leader must read and watch, with no importer or sweep of its
// own, and must keep doing so when the leader goes away.
func TestAReplicaThatIsNotTheLeaderServesReadsAndFeeds(t *testing.T) {
	conn := sharedTestConn(t)
	scope := newScope(t)
	leader := openBackend(t, conn, scope)
	follower := openBackend(t, conn, scope)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	feed, err := follower.Stores.Runs.WatchWithItems(ctx, "")
	if err != nil {
		t.Fatalf("follower feed: %v", err)
	}
	key := state.Key("shared-1")
	if _, err := leader.Stores.Conversations.Create(ctx, key, state.ConversationRecord{Name: "shared-1", Phase: "Active"}); err != nil {
		t.Fatal(err)
	}
	if err := leader.Stores.Runs.Append(ctx, key, state.Run{RunID: "r1"}); err != nil {
		t.Fatal(err)
	}

	eventually(t, "the follower lists what the leader wrote", func() bool {
		got, err := follower.Stores.Conversations.List(ctx, state.ByPhase, "Active")
		return err == nil && len(got) == 1
	})
	sawItem := false
	for !sawItem {
		select {
		case ev := <-feed:
			sawItem = ev.Type == state.EventPut && ev.Key == key && len(ev.Record.Items) == 1
		case <-ctx.Done():
			t.Fatal("the follower's feed never carried the leader's run")
		}
	}

	// The leader goes away mid-feed. The follower keeps serving, and its
	// feed keeps delivering what a new leader writes.
	leader.Close()
	next := openBackend(t, conn, scope)
	if _, err := state.Mutate(ctx, next.Stores.Conversations, key, func(r *state.ConversationRecord) error {
		r.Phase = state.PhaseClosed
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case ev := <-feed:
			if ev.Type == state.EventPut && ev.Key == key && ev.Record.Record.Phase == state.PhaseClosed {
				return
			}
		case <-ctx.Done():
			t.Fatal("the follower's feed did not survive a change of leader")
		}
	}
}

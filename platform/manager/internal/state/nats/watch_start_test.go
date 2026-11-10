package nats

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// A watch opened with no resume point means "everything from now". "Now" has
// to be fixed when Watch returns, not when the server gets around to creating
// the consumer: a record written in between belongs to the caller's feed, and
// a reconciler that opens its feed and is then told nothing about a record it
// has not seen waits for ever.
func TestAWatchOpenedWithNoResumePointDeliversWhatIsWrittenRightAfter(t *testing.T) {
	b := openBackend(t, sharedTestConn(t), newScope(t))
	for i := 0; i < 150; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		ch, err := b.Stores.Conversations.Watch(ctx, "")
		if err != nil {
			cancel()
			t.Fatalf("Watch: %v", err)
		}
		name := fmt.Sprintf("right-after-%d", i)
		mustConversation(t, b, name)
		got := false
		for !got {
			select {
			case ev, ok := <-ch:
				if !ok {
					cancel()
					t.Fatalf("round %d: the feed closed before delivering %s", i, name)
				}
				got = ev.Type == state.EventPut && string(ev.Key) == name
			case <-ctx.Done():
				cancel()
				t.Fatalf("round %d: %s was written right after Watch returned and never delivered", i, name)
			}
		}
		cancel()
	}
}

// The server may drop a consumer from under a watch (a leader change, a missed
// heartbeat), and the client then recreates it. A watch opened with no resume
// point must be recreated from the SAME point, not from whatever the stream's
// tip is by then: found live, a reconciler's feed reset 30s after it opened
// and never heard of the conversations created meanwhile, so they sat with no
// status and no pod.
func TestAWatchWithNoResumePointLosesNothingWhenItsConsumerIsRecreated(t *testing.T) {
	conn := sharedTestConn(t)
	scope := newScope(t)
	b := openBackend(t, conn, scope)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ch, err := b.Stores.Conversations.Watch(ctx, "")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	dropWatchConsumers(t, ctx, conn, scope)
	// A burst, so the writes straddle the moment the client recreates the
	// consumer: whichever it is, none may be missed.
	const n = 40
	want := map[string]bool{}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("written-while-the-consumer-was-gone-%d", i)
		want[name] = true
		mustConversation(t, b, name)
	}
	for len(want) > 0 {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatal("the feed closed instead of recovering")
			}
			if ev.Type == state.EventPut {
				delete(want, string(ev.Key))
			}
		case <-ctx.Done():
			t.Fatalf("%d of %d records written while the watch's consumer was gone were never delivered", len(want), n)
		}
	}
}

// dropWatchConsumers deletes every watch consumer on the conversations stream
// of scope, leaving the mirror's (a last-per-subject push consumer) alone.
func dropWatchConsumers(t *testing.T, ctx context.Context, conn *Conn, scope string) {
	t.Helper()
	names := conn.js.StreamNames(ctx)
	for name := range names.Name() {
		if !strings.HasPrefix(name, "KV_") || !strings.Contains(name, scope) || !strings.HasSuffix(name, "conversations") {
			continue
		}
		stream, err := conn.js.Stream(ctx, name)
		if err != nil {
			t.Fatalf("stream %s: %v", name, err)
		}
		consumers := stream.ListConsumers(ctx)
		dropped := 0
		for info := range consumers.Info() {
			if info.Config.DeliverPolicy == jetstream.DeliverLastPerSubjectPolicy {
				continue
			}
			if err := stream.DeleteConsumer(ctx, info.Name); err != nil {
				t.Fatalf("delete consumer %s: %v", info.Name, err)
			}
			dropped++
		}
		if dropped == 0 {
			t.Fatal("no watch consumer found to drop")
		}
		return
	}
	t.Fatal("conversations stream not found")
}

// The invariant behind the two tests above. An ordered consumer that is reset
// before it has delivered anything reuses its ORIGINAL delivery policy, and the
// server resolves "new" afresh at that moment. So a watch must never be opened
// with it: it must name the sequence it starts after, which survives a reset.
func TestNoWatchConsumerIsOpenedWithTheNewPolicy(t *testing.T) {
	conn := sharedTestConn(t)
	scope := newScope(t)
	b := openBackend(t, conn, scope)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for name, open := range map[string]func() error{
		"conversations": func() error { _, err := b.Stores.Conversations.Watch(ctx, ""); return err },
		"signals":       func() error { _, err := b.Stores.Signals.Watch(ctx, ""); return err },
	} {
		if err := open(); err != nil {
			t.Fatalf("Watch %s: %v", name, err)
		}
	}
	names := conn.js.StreamNames(ctx)
	for stream := range names.Name() {
		if !strings.Contains(stream, scope) {
			continue
		}
		st, err := conn.js.Stream(ctx, stream)
		if err != nil {
			t.Fatalf("stream %s: %v", stream, err)
		}
		infos := st.ListConsumers(ctx)
		for info := range infos.Info() {
			if info.Config.DeliverPolicy == jetstream.DeliverNewPolicy {
				t.Errorf("stream %s: consumer %s was opened with the new policy, a reset would skip what was written meanwhile", stream, info.Name)
			}
		}
	}
}

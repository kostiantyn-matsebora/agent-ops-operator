package nats

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/entrylog"
)

// failingJS is a JetStream whose every stream lookup fails, recording when
// each one was made. Only Stream is ever called on it: the mirror gives up
// before it reaches a consumer.
type failingJS struct {
	jetstream.JetStream
	mu sync.Mutex
	at map[string][]time.Time
}

func (f *failingJS) Stream(_ context.Context, name string) (jetstream.Stream, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.at[name] = append(f.at[name], time.Now())
	return nil, errors.New("injected")
}

func (f *failingJS) times(name string) []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.at[name]...)
}

// The queue mirror retries a watch it cannot open with a growing pause, for
// the reason internal/state/nats's mirror does: every attempt is a consumer
// created on a server that may already be slow to create them (issue 525).
func TestTheQueueMirrorPausesLongerAfterEachFailedWatch(t *testing.T) {
	const base = 20 * time.Millisecond
	prev := mirrorRetryDelay
	mirrorRetryDelay = base
	t.Cleanup(func() { mirrorRetryDelay = prev })

	js := &failingJS{at: map[string][]time.Time{}}
	m := newMirror(js, "log", "dead", entrylog.New(nil), nil)
	t.Cleanup(m.stop)

	deadline := time.Now().Add(5 * time.Second)
	for len(js.times("log")) < 6 {
		if time.Now().After(deadline) {
			t.Fatalf("only %d attempts were made", len(js.times("log")))
		}
		time.Sleep(5 * time.Millisecond)
	}
	at := js.times("log")
	first, last := at[1].Sub(at[0]), at[5].Sub(at[4])
	if last < 4*first {
		t.Fatalf("the pause before attempt 6 was %v and before attempt 2 was %v: the retry delay does not grow", last, first)
	}
}

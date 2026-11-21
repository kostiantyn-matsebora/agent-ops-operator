package nats

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// timedScript records when each WatchAll call was made and lets a test decide,
// per call, to fail it, relay it or let it through. It is watchScript with a
// clock, because the property under test is how far apart the attempts are.
type timedScript struct {
	jetstream.KeyValue
	mu      sync.Mutex
	at      []time.Time
	fail    func(call int) bool
	watched chan *proxyWatcher
}

func (s *timedScript) WatchAll(ctx context.Context, o ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	s.mu.Lock()
	s.at = append(s.at, time.Now())
	call := len(s.at)
	fail := s.fail
	s.mu.Unlock()
	if fail(call) {
		return nil, errInjected
	}
	inner, err := s.KeyValue.WatchAll(ctx, o...)
	if err != nil {
		return nil, err
	}
	p := relay(inner, -1)
	if s.watched != nil {
		select {
		case s.watched <- p:
		default:
		}
	}
	return p, nil
}

func (s *timedScript) times() []time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Time(nil), s.at...)
}

// A watch that cannot be created is retried with a growing pause. A fixed
// one-second retry against a server that is already struggling to create
// consumers adds one more creation attempt every second, per bucket, per
// manager replica — and every attempt that times out can leave a consumer
// behind that the server cannot clean up (issue 525).
func TestAMirrorPausesLongerAfterEachFailedWatch(t *testing.T) {
	const base = 20 * time.Millisecond
	withRetryDelay(t, base)
	kv := testBucket(t, 1)
	script := &timedScript{KeyValue: kv, fail: func(int) bool { return true }}
	m := newMirror[state.Run](script)
	t.Cleanup(m.stop)

	eventually(t, "six attempts were made", func() bool { return len(script.times()) >= 6 })
	at := script.times()
	first := at[1].Sub(at[0])
	last := at[5].Sub(at[4])
	if last < 4*first {
		t.Fatalf("the pause before attempt 6 was %v and before attempt 2 was %v: the retry delay does not grow", last, first)
	}
}

// What a healthy connection earns is a fast first retry: after a watch that
// really replayed the bucket, the pause starts over from the base delay and
// does not carry the failures that came before it.
func TestAMirrorStartsOverFromTheBaseDelayAfterAWatchThatCaughtUp(t *testing.T) {
	const base = 20 * time.Millisecond
	withRetryDelay(t, base)
	kv := testBucket(t, 2)
	script := &timedScript{KeyValue: kv, watched: make(chan *proxyWatcher, 4),
		fail: func(call int) bool { return call <= 4 || call == 6 }}
	m := newMirror[state.Run](script)
	t.Cleanup(m.stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.waitReady(ctx); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	healthy := <-script.watched
	dropped := time.Now()
	healthy.drop()

	eventually(t, "the retry after the drop was made", func() bool { return len(script.times()) >= 6 })
	retry := script.times()[5].Sub(dropped)
	// Four failures in a row would have grown the pause to 8x the base.
	if retry > 4*base {
		t.Fatalf("the retry after a healthy watch waited %v, want about the base %v: the failures before it were carried over", retry, base)
	}
}

// The pause stops growing at a ceiling, so a long outage is still noticed
// within a bounded time once the server is back.
func TestAMirrorPauseStopsGrowingAtTheCeiling(t *testing.T) {
	const base = 10 * time.Millisecond
	withRetryDelay(t, base)
	prev := mirrorRetryMax
	mirrorRetryMax = 4 * base
	t.Cleanup(func() { mirrorRetryMax = prev })
	kv := testBucket(t, 1)
	script := &timedScript{KeyValue: kv, fail: func(int) bool { return true }}
	m := newMirror[state.Run](script)
	t.Cleanup(m.stop)

	eventually(t, "nine attempts were made", func() bool { return len(script.times()) >= 9 })
	at := script.times()
	for i := 6; i < 9; i++ {
		if gap := at[i].Sub(at[i-1]); gap > 4*mirrorRetryMax {
			t.Fatalf("the pause before attempt %d was %v, over the ceiling %v", i+1, gap, mirrorRetryMax)
		}
	}
}

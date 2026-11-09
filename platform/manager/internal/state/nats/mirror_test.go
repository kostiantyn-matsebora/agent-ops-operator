package nats

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// proxyWatcher relays a real watcher through a channel the test can end,
// standing in for a connection that drops mid-watch.
type proxyWatcher struct {
	jetstream.KeyWatcher
	out  chan jetstream.KeyValueEntry
	once sync.Once
}

func (p *proxyWatcher) Updates() <-chan jetstream.KeyValueEntry { return p.out }
func (p *proxyWatcher) drop()                                   { p.once.Do(func() { close(p.out) }) }

func relay(real jetstream.KeyWatcher, stopAfter int) *proxyWatcher {
	p := &proxyWatcher{KeyWatcher: real, out: make(chan jetstream.KeyValueEntry)}
	go func() {
		n := 0
		for e := range real.Updates() {
			if stopAfter >= 0 && n >= stopAfter {
				p.drop()
				return
			}
			n++
			select {
			case p.out <- e:
			case <-time.After(5 * time.Second):
				return
			}
		}
		p.drop()
	}()
	return p
}

// watchScript decides what each WatchAll call of a bucket does.
type watchScript struct {
	jetstream.KeyValue
	mu    sync.Mutex
	calls int
	// on is called with the 1-based call number: fail it, relay it (cutting it
	// after stopAfter entries, -1 for never), or hold it open without entries.
	on      func(call int) (err error, stopAfter int, silent bool)
	watched chan *proxyWatcher
}

func (s *watchScript) WatchAll(ctx context.Context, o ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	s.mu.Lock()
	s.calls++
	call, on := s.calls, s.on
	s.mu.Unlock()
	err, stopAfter, silent := on(call)
	if err != nil {
		return nil, err
	}
	real, werr := s.KeyValue.WatchAll(ctx, o...)
	if werr != nil {
		return nil, werr
	}
	if silent {
		return &proxyWatcher{KeyWatcher: real, out: make(chan jetstream.KeyValueEntry)}, nil
	}
	p := relay(real, stopAfter)
	if s.watched != nil {
		select {
		case s.watched <- p:
		default:
		}
	}
	return p, nil
}

func withRetryDelay(t *testing.T, d time.Duration) {
	t.Helper()
	prev := mirrorRetryDelay
	mirrorRetryDelay = d
	t.Cleanup(func() { mirrorRetryDelay = prev })
}

// testBucket is a bare KV bucket seeded with n envelopes of runs under keys
// k0..k(n-1), outside any Backend.
func testBucket(t *testing.T, n int) jetstream.KeyValue {
	t.Helper()
	b := openBackend(t, sharedTestConn(t), newScope(t))
	for i := 0; i < n; i++ {
		put(t, b.runsKV, "k"+string(rune('0'+i)))
	}
	return b.runsKV
}

func put(t *testing.T, kv jetstream.KeyValue, key string) {
	t.Helper()
	if _, err := kv.Put(context.Background(), key, mustEnvelope(t)); err != nil {
		t.Fatalf("put %q: %v", key, err)
	}
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("never: %s", what)
}

func keysOf(m *mirror[state.Run]) map[state.Key]bool {
	out := map[state.Key]bool{}
	for k := range m.snapshot() {
		out[k] = true
	}
	return out
}

// A server that cannot be reached when the manager starts must delay a list,
// not break it for good.
func TestAMirrorWhoseFirstWatchFailsStillServesOnceItConnects(t *testing.T) {
	withRetryDelay(t, 10*time.Millisecond)
	kv := testBucket(t, 2)
	script := &watchScript{KeyValue: kv, on: func(call int) (error, int, bool) {
		if call <= 2 {
			return errInjected, -1, false
		}
		return nil, -1, false
	}}
	m := newMirror[state.Run](script)
	t.Cleanup(m.stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.waitReady(ctx); err != nil {
		t.Fatalf("waitReady = %v: a failed first watch poisoned the mirror for good", err)
	}
	if got := keysOf(m); len(got) != 2 {
		t.Fatalf("mirror holds %v, want both keys", got)
	}
}

// A watch that ends before it has replayed the bucket must not be taken for a
// finished catch-up.
func TestAMirrorIsNotReadyOnAPartialReplay(t *testing.T) {
	withRetryDelay(t, time.Hour)
	kv := testBucket(t, 3)
	script := &watchScript{KeyValue: kv, on: func(call int) (error, int, bool) {
		if call == 1 {
			return nil, 1, false // one entry, then the connection drops
		}
		return nil, -1, true // and the reconnect never delivers
	}}
	m := newMirror[state.Run](script)
	t.Cleanup(m.stop)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err := m.waitReady(ctx)
	if err == nil {
		t.Fatalf("waitReady returned with %d of 3 records replayed", len(keysOf(m)))
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waitReady = %v, want the caller's deadline", err)
	}
}

// What was deleted while the watch was down must leave the mirror when it
// reconnects, or a list serves a record that no longer exists.
func TestAMirrorDropsWhatWasDeletedWhileItWasDisconnected(t *testing.T) {
	withRetryDelay(t, 10*time.Millisecond)
	kv := testBucket(t, 2)
	script := &watchScript{KeyValue: kv, watched: make(chan *proxyWatcher, 4), on: func(call int) (error, int, bool) {
		return nil, -1, false
	}}
	m := newMirror[state.Run](script)
	t.Cleanup(m.stop)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.waitReady(ctx); err != nil {
		t.Fatalf("waitReady: %v", err)
	}
	first := <-script.watched

	// Hold the reconnect back while the record is deleted behind the mirror.
	script.mu.Lock()
	script.on = func(call int) (error, int, bool) { return errInjected, -1, false }
	script.mu.Unlock()
	first.drop()
	time.Sleep(50 * time.Millisecond)
	if err := kv.Delete(ctx, "k1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	script.mu.Lock()
	script.on = func(call int) (error, int, bool) { return nil, -1, false }
	script.mu.Unlock()

	eventually(t, "the deleted record left the mirror", func() bool { return !keysOf(m)["k1"] })
	if !keysOf(m)["k0"] {
		t.Fatal("a surviving record left the mirror")
	}
}

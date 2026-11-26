package system

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func waitFor1(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A forward whose connections reset is killed, so the supervisor relaunches it.
func TestWatchForwardKillsAForwardWhoseConnectionsReset(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close() // a port-forward pinned to a dead pod: accepted, then dropped
		}
	}()
	var kills, stop atomic.Int32
	go watchForward("http://"+l.Addr().String()+"/healthz", 5*time.Millisecond, 200*time.Millisecond,
		func() bool { return stop.Load() == 1 }, func() { kills.Add(1) })
	waitFor1(t, "the dead forward to be killed", func() bool { return kills.Load() >= 1 })
	stop.Store(1)
}

// Any HTTP answer, a 404 included, proves traffic flows: nothing is killed.
func TestWatchForwardLeavesAForwardThatAnswersAlone(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	var kills, stop atomic.Int32
	done := make(chan struct{})
	go func() {
		watchForward(srv.URL, 5*time.Millisecond, 200*time.Millisecond,
			func() bool { return stop.Load() == 1 }, func() { kills.Add(1) })
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	stop.Store(1)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("watchForward did not return once stopped")
	}
	if kills.Load() != 0 {
		t.Fatalf("a forward that answers must never be killed, got %d kills", kills.Load())
	}
}

// One failed request is not enough: a single blip must not churn the forward.
func TestWatchForwardNeedsTwoFailuresInARow(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			hj, _ := w.(http.Hijacker)
			c, _, _ := hj.Hijack()
			c.Close()
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	var kills, stop atomic.Int32
	go watchForward(srv.URL, 5*time.Millisecond, 200*time.Millisecond,
		func() bool { return stop.Load() == 1 }, func() { kills.Add(1) })
	time.Sleep(200 * time.Millisecond)
	stop.Store(1)
	if kills.Load() != 0 {
		t.Fatalf("one failure then success must not kill the forward, got %d kills", kills.Load())
	}
}

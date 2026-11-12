package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// A refusing transport: the first `refuse` dials fail the way a Service does
// while a rollout swaps the pod behind it, then the real server answers.
type refusingTransport struct {
	refuse int32
	dials  atomic.Int32
	next   http.RoundTripper
	err    error
}

func (r *refusingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.dials.Add(1) <= r.refuse {
		return nil, r.err
	}
	return r.next.RoundTrip(req)
}

func refused() error {
	return &net.OpError{Op: "dial", Net: "tcp", Err: &net.OpError{Op: "connect", Err: syscall.ECONNREFUSED}}
}

func retryingManager(t *testing.T, tr *refusingTransport) (*Manager, *atomic.Int32) {
	t.Helper()
	var served atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		served.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	tr.next = http.DefaultTransport
	prev := dialRetryPause
	dialRetryPause = time.Millisecond
	t.Cleanup(func() { dialRetryPause = prev })
	return &Manager{BaseURL: srv.URL, Token: "t", HTTP: &http.Client{Transport: tr}}, &served
}

// A message typed in the console while the manager is rolling is not lost to a
// refused connection: nothing reached any server, so sending it again is safe.
func TestAMessageRefusedAtTheDoorIsSentAgain(t *testing.T) {
	tr := &refusingTransport{refuse: 3, err: refused()}
	m, served := retryingManager(t, tr)

	if _, err := m.do(context.Background(), "POST", "/channel/inbound", map[string]string{"text": "/close"}, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
	if served.Load() != 1 {
		t.Fatalf("the manager saw the message %d times, want exactly once", served.Load())
	}
}

func TestARefusedConnectionIsRetriedOnlyAFewTimes(t *testing.T) {
	tr := &refusingTransport{refuse: 1000, err: refused()}
	m, served := retryingManager(t, tr)

	if _, err := m.do(context.Background(), "POST", "/channel/inbound", map[string]string{"text": "x"}, nil); err == nil {
		t.Fatal("a manager that never answers was reported as reached")
	}
	if got := tr.dials.Load(); got != int32(dialAttempts) {
		t.Fatalf("dialled %d times, want %d", got, dialAttempts)
	}
	if served.Load() != 0 {
		t.Fatalf("served %d", served.Load())
	}
}

// Only a refusal is safe to repeat. A reset after the request was written may
// have been acted on, and sending it again would act twice.
func TestAnErrorAfterTheRequestWasWrittenIsNotRetried(t *testing.T) {
	tr := &refusingTransport{refuse: 1000, err: errors.New("EOF")}
	m, _ := retryingManager(t, tr)

	if _, err := m.do(context.Background(), "POST", "/channel/inbound", map[string]string{"text": "x"}, nil); err == nil {
		t.Fatal("expected the error")
	}
	if got := tr.dials.Load(); got != 1 {
		t.Fatalf("dialled %d times, an ambiguous failure must not be repeated", got)
	}
}

func TestRetryingARefusedConnectionStopsWhenTheCallerGivesUp(t *testing.T) {
	tr := &refusingTransport{refuse: 1000, err: refused()}
	m, _ := retryingManager(t, tr)
	dialRetryPause = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	if _, err := m.do(ctx, "POST", "/channel/inbound", map[string]string{"text": "x"}, nil); err == nil {
		t.Fatal("expected an error")
	}
	if got := tr.dials.Load(); got >= int32(dialAttempts) {
		t.Fatalf("kept dialling (%d) after the caller's context ended", got)
	}
}

package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// upstreamRecorder stands in for the manager.
type upstreamRecorder struct {
	paths []string
	body  string
}

func (u *upstreamRecorder) start(t *testing.T) *url.URL {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.paths = append(u.paths, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(u.body))
	}))
	t.Cleanup(srv.Close)
	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// The ORDER is the guarantee: the manager records the context handle from
// /work/done, so a checkpoint that happened after the report could name a
// context whose bytes were never persisted.
func TestCheckpointHappensBeforeCompletionReachesTheManager(t *testing.T) {
	up := &upstreamRecorder{body: "ok"}
	var order []string
	p := &Proxy{
		Upstream: up.start(t),
		Hooks: Hooks{
			BeforeWorkDone: func() { order = append(order, "checkpoint") },
		},
	}
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/work/done", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	order = append(order, "forwarded")

	if len(order) != 2 || order[0] != "checkpoint" || order[1] != "forwarded" {
		t.Fatalf("order = %v, want checkpoint before the report reaches the manager", order)
	}
	if len(up.paths) != 1 || up.paths[0] != "POST /work/done" {
		t.Fatalf("upstream saw %v, want the completion forwarded", up.paths)
	}
}

// Restore must complete before the runtime is given anything to do. Handing out
// work first lets the agent start against an empty context tree and write a fresh
// context over the one it was meant to continue.
func TestRestoreHappensBeforeTheFirstWorkUnit(t *testing.T) {
	up := &upstreamRecorder{body: "unit"}
	var order []string
	p := &Proxy{
		Upstream: up.start(t),
		Hooks: Hooks{
			BeforeFirstWork: func() error { order = append(order, "restore"); return nil },
			WorkHandedOut:   func() { order = append(order, "handed-out") },
		},
	}
	srv := httptest.NewServer(p)
	defer srv.Close()

	for i := 0; i < 3; i++ {
		resp, err := http.Get(srv.URL + "/work?convo=c1")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	if order[0] != "restore" {
		t.Fatalf("order = %v, want restore first", order)
	}
	// Once per pod, not once per poll: a runtime long-polls continuously, and
	// restoring on every poll would overwrite the agent's live work.
	restores := 0
	for _, o := range order {
		if o == "restore" {
			restores++
		}
	}
	if restores != 1 {
		t.Fatalf("restore ran %d times, want exactly once per pod", restores)
	}
}

// A failed restore must not hand out work. Starting fresh silently is the
// degradation the continuity rules exist to prevent.
func TestFailedRestoreRefusesToHandOutWork(t *testing.T) {
	up := &upstreamRecorder{body: "unit"}
	p := &Proxy{
		Upstream: up.start(t),
		Hooks: Hooks{
			BeforeFirstWork: func() error { return errors.New("volume gone") },
		},
	}
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/work?convo=c1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if len(up.paths) != 0 {
		t.Fatalf("upstream was contacted despite a failed restore: %v", up.paths)
	}
}

// The proxy knows whether a run is inflight, which is how a checkpoint labels
// itself quiesced or best-effort without the runtime telling it anything.
func TestInflightTracksWorkBoundaries(t *testing.T) {
	up := &upstreamRecorder{body: "unit"}
	p := &Proxy{Upstream: up.start(t)}
	srv := httptest.NewServer(p)
	defer srv.Close()

	if p.Inflight() {
		t.Fatal("nothing handed out yet")
	}
	resp, _ := http.Get(srv.URL + "/work?convo=c1")
	resp.Body.Close()
	if !p.Inflight() {
		t.Fatal("a handed-out unit must read as inflight")
	}
	resp2, _ := http.Post(srv.URL+"/work/done", "application/json", strings.NewReader(`{}`))
	resp2.Body.Close()
	if p.Inflight() {
		t.Fatal("a completed unit must clear inflight")
	}
}

// Everything that is not a work boundary is forwarded untouched — the proxy is
// a forwarder, not a parser.
func TestUnrelatedPathsAreForwardedUntouched(t *testing.T) {
	up := &upstreamRecorder{body: "ok"}
	var hooks int
	p := &Proxy{
		Upstream: up.start(t),
		Hooks: Hooks{
			BeforeFirstWork: func() error { hooks++; return nil },
			BeforeWorkDone:  func() { hooks++ },
		},
	}
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if hooks != 0 {
		t.Fatal("an unrelated path must not trigger a restore or a checkpoint")
	}
	if len(up.paths) != 1 || up.paths[0] != "GET /healthz" {
		t.Fatalf("upstream saw %v, want the request forwarded", up.paths)
	}
}

// A trailing slash must not slip past boundary detection and leave context
// unpersisted.
func TestPathIsIgnoresTrailingSlash(t *testing.T) {
	if !pathIs("/work/done/", "/work/done") || !pathIs("/work/done", "/work/done") {
		t.Fatal("a trailing slash must not hide a work boundary")
	}
	if pathIs("/work/doneish", "/work/done") {
		t.Fatal("a longer path must not match")
	}
}

// A work-boundary checkpoint must be labelled QUIESCED.
//
// It was not: inflight was cleared after the hook, so the checkpoint still saw
// a run in flight and every work-boundary copy came out best-effort. A restore
// would then believe it had never captured a clean copy. Only a live run
// surfaced it — the ordering test above passes either way.
func TestWorkBoundaryCheckpointIsQuiesced(t *testing.T) {
	up := &upstreamRecorder{body: "ok"}
	var inflightAtCheckpoint bool
	p := &Proxy{Upstream: up.start(t)}
	p.Hooks = Hooks{
		BeforeWorkDone: func() { inflightAtCheckpoint = p.Inflight() },
	}
	srv := httptest.NewServer(p)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/work?convo=c1")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !p.Inflight() {
		t.Fatal("precondition: a handed-out unit is inflight")
	}

	resp2, err := http.Post(srv.URL+"/work/done", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()

	if inflightAtCheckpoint {
		t.Fatal("a checkpoint at a work boundary must NOT see a run in flight, " +
			"or it labels itself best-effort and the quiesced distinction is lost")
	}
}

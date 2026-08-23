package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// The proxy is how this sidecar learns work boundaries WITHOUT any runtime image
// changing.
//
// Two alternatives were rejected. Having each runtime image do its own copying
// is the least machinery, but every backend an adopter brings — aider, a local
// model, something custom — must then reimplement it with its own failure
// handling, and writing it once is the entire reason a sidecar exists. Having
// the runtime SIGNAL the sidecar over a socket gives better semantics but
// reintroduces exactly that per-runtime work, only smaller.
//
// A runtime already receives CONTROL_URL from env, already long-polls
// GET /work, and already posts POST /work/done, because the work contract
// requires all three. Pointing CONTROL_URL at localhost therefore buys the
// boundaries for free.
//
// The costs are real and accepted: this sits on the critical path for work
// dispatch, and a hung sidecar presents as a hung /work. It is kept a
// forwarding proxy with no parsing beyond the two paths it acts on, and a
// sidecar failure fails the pod, which the manager's existing reap handles.

// Hooks are what the proxy calls at the boundaries it observes.
type Hooks struct {
	// BeforeFirstWork runs before the runtime is given its FIRST unit, and its
	// error fails that request. Restoring after handing out work would let the
	// agent start against an empty context tree and write a fresh context over
	// the one it was supposed to continue.
	BeforeFirstWork func() error
	// WorkHandedOut marks a run as inflight, so a checkpoint taken now knows to
	// label itself best-effort rather than quiesced.
	WorkHandedOut func()
	// BeforeWorkDone runs BEFORE the completion reaches the manager.
	//
	// This ordering is the guarantee: the manager records the runtime's context
	// handle from that report, so reporting first would allow a recorded handle
	// whose context was never persisted — and the next run would then fail a
	// continuation that should have worked.
	BeforeWorkDone func()
}

// Proxy forwards the work contract to the real manager, observing boundaries.
type Proxy struct {
	Upstream *url.URL
	Hooks    Hooks

	mu       sync.Mutex
	restored bool
	inflight bool
	rp       *httputil.ReverseProxy
	once     sync.Once
}

// Inflight reports whether a unit has been handed out and not yet completed.
// A checkpoint consults it to label the generation honestly.
func (p *Proxy) Inflight() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.inflight
}

func (p *Proxy) handler() *httputil.ReverseProxy {
	p.once.Do(func() {
		p.rp = httputil.NewSingleHostReverseProxy(p.Upstream)
		// The work contract's long poll waits ~25s. The default transport is
		// fine for that, but the proxy must NOT buffer: /work returns a unit
		// the runtime should see immediately, and FlushInterval -1 streams
		// rather than accumulating.
		p.rp.FlushInterval = -1
		p.rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
			// 502 rather than a panic: the runtime retries its long poll, and a
			// manager that is briefly unreachable must not kill the pod.
			http.Error(w, "context-sync: upstream unreachable: "+err.Error(), http.StatusBadGateway)
		}
	})
	return p.rp
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case isWorkRequest(r):
		if err := p.ensureRestored(); err != nil {
			// Failing the request is right: handing out work against a volume
			// that was supposed to hold a context, and does not, produces a run
			// that silently starts fresh.
			http.Error(w, "context-sync: restore failed: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		p.mark(true)
		if p.Hooks.WorkHandedOut != nil {
			p.Hooks.WorkHandedOut()
		}
	case isWorkDone(r):
		// Clear inflight FIRST, then checkpoint, then forward.
		//
		// The order of these two matters and cost a live run to find. The
		// runtime is REPORTING completion, so the unit is finished before the
		// checkpoint starts — but marking after the hook meant the checkpoint
		// still saw a run in flight and labelled itself best-effort. Every
		// work-boundary copy came out mislabelled, which defeats the whole
		// quiesced distinction: a restore would believe it had never once
		// captured a clean copy.
		//
		// Forwarding still happens LAST, which is the guarantee that matters —
		// see Hooks.BeforeWorkDone.
		p.mark(false)
		if p.Hooks.BeforeWorkDone != nil {
			p.Hooks.BeforeWorkDone()
		}
	}
	p.handler().ServeHTTP(w, r)
}

func (p *Proxy) mark(inflight bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inflight = inflight
}

// ensureRestored runs the restore exactly once per pod.
func (p *Proxy) ensureRestored() error {
	p.mu.Lock()
	if p.restored {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	var err error
	if p.Hooks.BeforeFirstWork != nil {
		err = p.Hooks.BeforeFirstWork()
	}
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.restored = true
	p.mu.Unlock()
	return nil
}

// isWorkRequest matches the long poll that hands out a unit.
func isWorkRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && pathIs(r.URL.Path, "/work")
}

// isWorkDone matches the completion report.
func isWorkDone(r *http.Request) bool {
	return r.Method == http.MethodPost && pathIs(r.URL.Path, "/work/done")
}

// pathIs compares ignoring a trailing slash, so neither spelling can slip past
// the boundary detection and leave context unpersisted.
func pathIs(got, want string) bool {
	return strings.TrimSuffix(got, "/") == want
}

// Timeouts for the sidecar's own listener. Generous read/write because /work is
// a long poll — a 25-second wait must not be cut off by the server that is
// merely forwarding it.
const (
	proxyReadTimeout  = 60 * time.Second
	proxyWriteTimeout = 60 * time.Second
)

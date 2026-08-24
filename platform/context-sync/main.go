package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Configuration arrives as env, injected by the manager's pod builder. No flags:
// this is never run by hand, and one source beats two that can disagree.
const (
	envListen     = "LISTEN_ADDR"           // where the runtime's CONTROL_URL points
	envUpstream   = "CONTROL_URL_UPSTREAM"  // the real manager
	envLive       = "CONTEXT_LIVE_DIR"      // pod-local tree the agent writes to
	envStore      = "CONTEXT_STORE_DIR"     // this conversation's dir on the volume
	envPaths      = "CONTEXT_SYNC_PATHS"    // newline-separated include globs
	envExclude    = "CONTEXT_SYNC_EXCLUDE"  // newline-separated exclude globs
	envInterval   = "CONTEXT_SYNC_INTERVAL" // Go duration; 0 = work boundaries only
	envRetain     = "CONTEXT_SYNC_RETAIN"
	envConvo      = "CONVO_ID"
	envReportURL  = "CONTEXT_REPORT_URL" // where operations are reported
	envAuthHeader = "CONTEXT_REPORT_TOKEN"
)

type config struct {
	listen    string
	upstream  *url.URL
	live      string
	store     string
	paths     []string
	exclude   []string
	interval  time.Duration
	retain    int
	convo     string
	reportURL string
	token     string
}

func loadConfig() (config, error) {
	var c config
	c.listen = envOr(envListen, ":8099")
	c.live = envOr(envLive, "/data/context")
	c.store = os.Getenv(envStore)
	c.paths = lines(os.Getenv(envPaths))
	c.exclude = lines(os.Getenv(envExclude))
	c.convo = os.Getenv(envConvo)
	c.reportURL = os.Getenv(envReportURL)
	c.token = os.Getenv(envAuthHeader)

	raw := os.Getenv(envUpstream)
	if raw == "" {
		return c, fmt.Errorf("%s is required: without it there is nothing to forward to", envUpstream)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return c, fmt.Errorf("%s %q: %w", envUpstream, raw, err)
	}
	c.upstream = u

	if c.store == "" {
		return c, fmt.Errorf("%s is required: without it there is nowhere durable to keep context", envStore)
	}
	// An empty include list would persist NOTHING while appearing configured,
	// which is the silent failure this whole change exists to end.
	if len(c.paths) == 0 {
		return c, fmt.Errorf("%s is required and must name at least one path", envPaths)
	}

	c.interval = 2 * time.Minute
	if v := os.Getenv(envInterval); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("%s %q: %w", envInterval, v, err)
		}
		c.interval = d // 0 is legitimate: work-boundary checkpoints only
	}
	c.retain = 3
	if v := os.Getenv(envRetain); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return c, fmt.Errorf("%s %q must be a positive integer", envRetain, v)
		}
		c.retain = n
	}
	return c, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// syncer owns the live tree, the durable store, and the last manifest.
type syncer struct {
	cfg   config
	store *Store

	mu       sync.Mutex
	last     Manifest
	lastAt   time.Time
	reporter *reporter
	inflight func() bool
}

// checkpoint copies the live context if it changed, and reports what happened.
//
// The skip is the point. On a two-minute cadence over a conversation that is
// sitting idle, skipping means the fragile filesystem is not touched at all —
// where a conditional-but-full copy would touch it 720 times a day.
func (s *syncer) checkpoint(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	m, err := Scan(s.cfg.live, s.cfg.paths, s.cfg.exclude)
	if err != nil {
		log.Printf("context-sync: scan failed: %v", err)
		s.reporter.report(event{Kind: "context.failed", Reason: reason, Error: err.Error()})
		return
	}
	if !m.Changed(s.last) {
		// Reported, not silent: "nothing changed" and "nothing ran" are
		// different facts, and an operator looking at a stale context needs to
		// tell them apart.
		s.reporter.report(event{Kind: "context.skip", Reason: reason,
			DurationMs: time.Since(start).Milliseconds()})
		return
	}

	quiesced := s.inflight == nil || !s.inflight()
	meta, err := s.store.Checkpoint(s.cfg.live, m, quiesced, time.Now())
	if err != nil {
		log.Printf("context-sync: checkpoint failed: %v", err)
		s.reporter.report(event{Kind: "context.failed", Reason: reason, Error: err.Error()})
		return
	}
	s.last, s.lastAt = m, time.Now()
	s.reporter.report(event{
		Kind: "context.checkpoint", Reason: reason,
		Bytes: meta.Bytes, Files: meta.Files, Quiesced: quiesced,
		DurationMs: time.Since(start).Milliseconds(),
	})
}

// restore brings the durable context into the live tree before any work runs.
func (s *syncer) restore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	meta, ok, err := s.store.Restore(s.cfg.live)
	if err != nil {
		s.reporter.report(event{Kind: "context.failed", Reason: "restore", Error: err.Error()})
		return err
	}
	// Seed the manifest from what was restored, so the first checkpoint after a
	// start does not re-copy a context that has not changed.
	m, serr := Scan(s.cfg.live, s.cfg.paths, s.cfg.exclude)
	if serr == nil {
		s.last = m
	}
	s.reporter.report(event{
		Kind: "context.restore", Reason: "start", Found: ok,
		Bytes: m.Bytes(), Files: len(m.Entries), Quiesced: meta.Quiesced,
		DurationMs: time.Since(start).Milliseconds(),
	})
	return nil
}

// sinceLast reports how long ago a checkpoint actually wrote something.
func (s *syncer) sinceLast() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastAt.IsZero() {
		return time.Duration(1<<62 - 1)
	}
	return time.Since(s.lastAt)
}

// event is one reported operation. The manager turns it into activity, which
// yields the console's per-conversation view AND the metrics from a single
// instrumentation pass.
type event struct {
	Kind         string `json:"kind"`
	Conversation string `json:"conversation"`
	Reason       string `json:"reason,omitempty"`
	Bytes        int64  `json:"bytes,omitempty"`
	Files        int    `json:"files,omitempty"`
	Quiesced     bool   `json:"quiesced,omitempty"`
	Found        bool   `json:"found,omitempty"`
	DurationMs   int64  `json:"durationMs,omitempty"`
	Error        string `json:"error,omitempty"`
}

type reporter struct {
	url   string
	token string
	convo string
	c     *http.Client
}

// report posts one operation. Best-effort by design: telemetry must never be
// able to fail a checkpoint that already succeeded.
func (r *reporter) report(e event) {
	if r == nil || r.url == "" {
		return
	}
	e.Conversation = r.convo
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, r.url, bytes.NewReader(b))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	resp, err := r.c.Do(req)
	if err != nil {
		log.Printf("context-sync: reporting %s failed: %v", e.Kind, err)
		return
	}
	_ = resp.Body.Close()
}

func main() {
	log.SetFlags(0)
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("context-sync: %v", err)
	}

	s := &syncer{
		cfg:      cfg,
		store:    &Store{Root: cfg.store, Retain: cfg.retain},
		reporter: &reporter{url: cfg.reportURL, token: cfg.token, convo: cfg.convo, c: &http.Client{Timeout: 10 * time.Second}},
	}

	proxy := &Proxy{
		Upstream: cfg.upstream,
		Hooks: Hooks{
			BeforeFirstWork: s.restore,
			BeforeWorkDone:  func() { s.checkpoint("work-done") },
		},
	}
	s.inflight = proxy.Inflight

	srv := &http.Server{
		Addr:         cfg.listen,
		Handler:      proxy,
		ReadTimeout:  proxyReadTimeout,
		WriteTimeout: proxyWriteTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if cfg.interval > 0 {
		go s.tick(ctx, cfg.interval)
	} else {
		log.Printf("context-sync: periodic checkpointing disabled; work boundaries only")
	}

	go func() {
		<-ctx.Done()
		// The FINAL checkpoint. This is what covers every ordinary end of a
		// pod — an idle timeout, /exit, an eviction, a close, a node drain —
		// and it must finish inside the termination grace period, which the
		// manager sizes for it.
		log.Printf("context-sync: shutting down; taking a final checkpoint")
		s.checkpoint("shutdown")
		shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	log.Printf("context-sync: listening on %s, forwarding to %s (interval %s, retain %d)",
		cfg.listen, cfg.upstream, cfg.interval, cfg.retain)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("context-sync: %v", err)
	}
}

// tick runs the periodic checkpoint, DEBOUNCED against the last one that
// actually wrote. A work-boundary checkpoint moments ago makes the timer's
// nothing but noise against the volume.
func (s *syncer) tick(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if s.sinceLast() < every/2 {
				continue
			}
			s.checkpoint("interval")
		}
	}
}

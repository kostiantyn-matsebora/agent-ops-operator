// signal-pulse: webhook-receiving signal adapter for Pulse
// (rcourtman/pulse, monitors Proxmox/Docker/K8s/TrueNAS). Serves
// SignalSources whose spec.adapter names its SignalAdapter CR
// (ADAPTER_NAME, injected by the reconciler = the CR name) against the
// operator's signal contract:
//
//	webhook:  POST /webhook/{source}   (LISTEN_ADDR, default :8080; 1 MiB cap)
//	sources:  GET /signal/sources?adapter=<ADAPTER_NAME> (15s poll)
//	push:     POST /signal/inbound     (normalized; alert lane)
//
// Registration reads the served source's OWN spec.config.baseUrl and
// credentialsSecretRef (register.go) on every registryLoop poll tick — this
// bundle ships exactly one default source, so there is no generalized,
// hand-authored-source registration path to build, but the SignalAdapter
// CRD has no pod-env passthrough, so the source's own config/credential is
// the only delivery mechanism that actually reaches this pod (see
// register.go's doc comment). The adapter calls Pulse's own
// POST /api/notifications/webhooks to create or repair a Generic webhook
// receiver pointing at its own /webhook/<source> endpoint. The webhook is
// served regardless of registration outcome — an unregistered sender means
// no alerts arrive, not that the endpoint refuses them.
//
// payload.go owns both the registered Go-template payload body and the
// parser that decodes it on receipt, so the two halves cannot drift apart.
// signature.go verifies HMAC-SHA256 over timestamp + "." + body
// (X-Pulse-Timestamp / X-Pulse-Signature) when a source's credential
// carries a signing secret; unsigned mode accepts anything, parity with
// signals/alertmanager's opt-in-per-source credential posture.
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, ADAPTER_NAME (default "pulse" —
// in-cluster the reconciler injects the SignalAdapter CR name), LISTEN_ADDR
// (default ":8080") — all reconciler-injected, contract parity with every
// other signal adapter. No Pulse-specific env, and no connectivity
// information of the adapter's own either (same posture as
// channels/telegram and signals/ha) — the delivery URL registered with
// Pulse rides the served SignalSource itself (register.go), the same as
// its base URL.
package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// sanitizeLogText strips CR/LF before a value reaches a log line, so a
// manager response or a source name cannot forge a second line (the
// analyzer's S5145 recognizes chained ReplaceAll, not %q).
func sanitizeLogText(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\n", "_"), "\r", "_")
}

type adapter struct {
	mgr        *Manager
	sourceType string // ADAPTER_NAME
	listen     string

	mu sync.Mutex
	// sources maps each served source name to its resolved signing secret
	// ("" = unsigned mode) — resolved fresh every registryLoop poll tick
	// from that source's own spec.config/credentialsSecretRef, never cached
	// longer than one refresh cycle.
	sources  map[string]string
	reported map[string]string
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}

// baseContext supplies the parent for main's signal-derived context. A test
// swaps it to inject a context it can cancel directly, so it can call the
// real main() in-process and stop it exactly as a real SIGTERM would —
// without sending any OS signal that could also reach other tests sharing
// this process.
var baseContext = context.Background

// envOr reads an env var, falling back to def when unset — split out from
// main() so the defaulting itself is a plain unit test, never exercised
// only by binding a real port.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	sourceType := envOr("ADAPTER_NAME", "pulse")
	listen := envOr("LISTEN_ADDR", ":8080")
	a := &adapter{
		mgr:        NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		sourceType: sourceType,
		listen:     listen,
		sources:    map[string]string{},
		reported:   map[string]string{},
	}
	ctx, stop := signal.NotifyContext(baseContext(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("signal-pulse adapter starting (adapter=%s, listen=%s)", sourceType, listen)

	go a.registryLoop(ctx)

	srv := &http.Server{Addr: listen, Handler: a.handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("webhook server: %v", err)
	}
}

// registryLoop keeps the served-source map fresh (cron-adapter pattern) and
// reconciles each source's Pulse registration on the same tick, so a source
// unclaimed or deleted stops being accepted within one poll interval, a
// newly wired one starts being accepted within the same window, and a
// rotated token or a Pulse outage heals without a pod restart.
func (a *adapter) registryLoop(ctx context.Context) {
	for ctx.Err() == nil {
		a.refreshSources(ctx)
		select {
		case <-ctx.Done():
		case <-time.After(15 * time.Second):
		}
	}
}

func (a *adapter) refreshSources(ctx context.Context) {
	infos, err := a.mgr.Sources(ctx, a.sourceType)
	if err != nil {
		log.Printf("list sources: %q", sanitizeLogText(err.Error()))
		return
	}
	next := map[string]string{}
	for _, info := range infos {
		next[info.Name] = a.reconcileRegistration(ctx, info)
	}
	a.mu.Lock()
	a.sources = next
	a.mu.Unlock()
}

// served reports whether a source name is currently listed by the manager,
// and the signing secret to verify its inbound webhooks against ("" =
// unsigned mode).
func (a *adapter) served(name string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	secret, ok := a.sources[name]
	return secret, ok
}

// servedName returns the manager-listed spelling of a served source, so a log
// line carries the registry's own value rather than the caller's path segment.
func (a *adapter) servedName(name string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k := range a.sources {
		if k == name {
			return k
		}
	}
	return ""
}

// report publishes a source's Ready state, skipping unchanged repeats so the
// registration loop does not spam the status endpoint every tick.
func (a *adapter) report(ctx context.Context, source, reason, message string) {
	key := reason + "|" + message
	a.mu.Lock()
	unchanged := a.reported[source] == key
	a.mu.Unlock()
	if unchanged {
		return
	}
	if err := a.mgr.ReportStatus(ctx, source, true, reason, message); err != nil {
		log.Printf("report status %q: %q", sanitizeLogText(source), sanitizeLogText(err.Error()))
		return
	}
	a.mu.Lock()
	a.reported[source] = key
	a.mu.Unlock()
}

func (a *adapter) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /webhook/{source}", a.handleWebhook)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func (a *adapter) handleWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("source")
	signingSecret, served := a.served(name)
	if !served {
		// a retried Pulse delivery covers the source-poll lag
		writeJSON(w, 404, map[string]string{"error": "unknown or unserved source"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "read body"})
		return
	}

	// signature verification is gated on a configured signing secret —
	// unsigned mode (no secret) accepts anything, matching
	// signals/alertmanager's opt-in-per-source credential posture.
	if signingSecret != "" {
		if !verifySignature(signingSecret, r.Header.Get("X-Pulse-Timestamp"), r.Header.Get("X-Pulse-Signature"), body, time.Now()) {
			writeJSON(w, 401, map[string]string{"error": "invalid or missing signature"})
			return
		}
	}

	event, err := parsePulseEvent(body)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid Pulse webhook JSON"})
		return
	}

	signals := normalize(event)
	if len(signals) == 0 {
		writeJSON(w, 200, map[string]any{"queued": 0, "reason": "not a firing alert"})
		return
	}
	res, err := a.mgr.Inbound(r.Context(), name, signals)
	if err != nil {
		// the registry's own spelling, never the caller's path segment, so it cannot forge log lines.
		log.Printf("inbound %q: %q", sanitizeLogText(a.servedName(name)), sanitizeLogText(err.Error()))
		writeJSON(w, 502, map[string]string{"error": "manager rejected the signals"})
		return
	}
	writeJSON(w, 200, res)
}

// normalize converts one parsed Pulse event into contract signals. Only
// "alert" (firing) events produce one — "resolved" produces none, so Pulse
// sees a 2xx and does not retry, but nothing is pushed (pulse-signal-adapter
// spec, "Firing alerts normalize into the signal contract, resolved alerts
// do not"). No grouping/dedup here — the manager owns that.
func normalize(e pulseEvent) []Signal {
	if e.Event != "alert" {
		return nil
	}
	// Severity stays OUT of the title on purpose — the console's signal card
	// drops a label row whose value already appears as a bare word in the
	// title (platform/console/manager.go's labelRows, TestSignalCardIsStructured),
	// so a "🔍 <resource> — <severity>" title would silently swallow the
	// severity label row out of the table it belongs in.
	title := "🔍 " + e.Resource
	payloadJSON, _ := json.MarshalIndent(e, "", "  ")
	return []Signal{{
		Fingerprint: e.fingerprint(),
		Labels: map[string]string{
			"resource": e.Resource, "resourceType": e.ResourceType,
			"severity": e.Severity, "node": e.Node,
		},
		Title:   title,
		Payload: string(payloadJSON),
	}}
}

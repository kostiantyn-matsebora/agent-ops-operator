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
// Registration is pod-environment-driven, not SignalSource-driven
// (design.md Decision 1 — this bundle ships exactly one default source, so
// the generalized, per-source, CR-carried registration path
// signals/alertmanager uses would add indirection this adapter has no use
// for). At pod start, and on a bounded retry loop thereafter (register.go),
// the adapter calls Pulse's own POST /api/notifications/webhooks to create
// or repair a Generic webhook receiver pointing at its own
// /webhook/<source> endpoint. The webhook is served regardless of
// registration outcome — an unregistered sender means no alerts arrive, not
// that the endpoint refuses them.
//
// payload.go owns both the registered Go-template payload body and the
// parser that decodes it on receipt, so the two halves cannot drift apart.
// signature.go verifies HMAC-SHA256 over timestamp + "." + body
// (X-Pulse-Timestamp / X-Pulse-Signature) when PULSE_SIGNING_SECRET is
// configured; unsigned mode accepts anything, parity with
// signals/alertmanager's opt-in-per-source credential posture.
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, ADAPTER_NAME (default "pulse" —
// in-cluster the reconciler injects the SignalAdapter CR name), LISTEN_ADDR
// (default ":8080"), POD_NAMESPACE, PULSE_URL, PULSE_API_TOKEN,
// PULSE_SOURCE_NAME, PULSE_SIGNING_SECRET.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type adapter struct {
	mgr        *Manager
	sourceType string // ADAPTER_NAME
	podNS      string // downward-API namespace, "" when unset
	listen     string
	reg        pulseRegistration

	mu       sync.Mutex
	sources  map[string]bool // served source names, learned from the manager
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

func main() {
	sourceType := os.Getenv("ADAPTER_NAME")
	if sourceType == "" {
		sourceType = "pulse"
	}
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}
	a := &adapter{
		mgr:        NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		sourceType: sourceType,
		podNS:      os.Getenv("POD_NAMESPACE"),
		listen:     listen,
		reg:        loadRegistration(),
		sources:    map[string]bool{},
		reported:   map[string]string{},
	}
	ctx, stop := signal.NotifyContext(baseContext(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("signal-pulse adapter starting (adapter=%s, listen=%s)", sourceType, listen)

	go a.registryLoop(ctx)
	go a.registerLoop(ctx)

	srv := &http.Server{Addr: listen, Handler: a.handler()}
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

// registryLoop keeps the served-source map fresh (cron-adapter pattern), so
// a source unclaimed or deleted stops being accepted within one poll
// interval, and a newly wired one starts being accepted within the same
// window.
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
		log.Printf("list sources: %v", err)
		return
	}
	next := map[string]bool{}
	for _, info := range infos {
		next[info.Name] = true
	}
	a.mu.Lock()
	a.sources = next
	a.mu.Unlock()
}

// served reports whether a source name is currently listed by the manager.
func (a *adapter) served(name string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sources[name]
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
		log.Printf("report status %s: %v", source, err)
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
	if !a.served(name) {
		// a retried Pulse delivery covers the source-poll lag
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown or unserved source %q", name)})
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
	if secret := a.reg.SigningSecret; secret != "" {
		if !verifySignature(secret, r.Header.Get("X-Pulse-Timestamp"), r.Header.Get("X-Pulse-Signature"), body, time.Now()) {
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
		log.Printf("inbound %s: %v", name, err)
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
	title := "🔍 " + e.Resource
	if e.Severity != "" {
		title += " — " + e.Severity
	}
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

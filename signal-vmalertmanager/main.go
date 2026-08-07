// signal-vmalertmanager: webhook-receiving signal adapter. Serves
// SignalSources whose spec.type names its SignalAdapter CR (SOURCE_TYPE,
// injected by the reconciler = the CR name) against the operator's signal
// contract by hosting its OWN HTTP endpoint accepting the standard
// Alertmanager webhook format — VictoriaMetrics VMAlertmanager is the
// packaged story, but any Alertmanager-compatible sender works:
//
//	webhook:  POST /webhook/{source}   (LISTEN_ADDR, default :8080; 1 MiB cap)
//	sources:  GET /signal/sources?type=<SOURCE_TYPE> (15s poll)
//	push:     POST /signal/inbound     (normalized; alert lane)
//
// Normalization keeps built-in-path parity: firing-only, fingerprint verbatim
// (sorted-label-hash fallback), raw labels, "🔍 alertname — namespace" title,
// per-alert JSON payload. NO adapter-side grouping — signature grouping,
// cooldown, window reuse, and recurrence stay manager-side from
// SignalSource.spec.grouping (and routing requires the source's Pipeline
// claim: unwired sources drop with Wired=False).
//
// Auth is opt-in per source: a source with credentialsSecretRef advertises a
// credentialEnvPrefix; the adapter then requires Authorization: Bearer
// matching the projected <prefix>TOKEN value (constant-time). Uncredentialed
// sources accept anonymous posts (parity with the built-in endpoint's
// ClusterIP-only posture).
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, SOURCE_TYPE (default
// "vm-alertmanager" — hand-deployed instances set it themselves; in-cluster
// the reconciler injects the CR name), LISTEN_ADDR (default ":8080" —
// in-cluster the reconciler injects it from SignalAdapter.spec.port),
// projected AGENTOPS_CRED_* vars.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// amAlert is one entry of the standard Alertmanager webhook payload.
type amAlert struct {
	Status       string            `json:"status"`
	Fingerprint  string            `json:"fingerprint"`
	StartsAt     string            `json:"startsAt"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	GeneratorURL string            `json:"generatorURL"`
}

type amPayload struct {
	Alerts []amAlert `json:"alerts"`
}

type adapter struct {
	mgr        *Manager
	sourceType string

	mu       sync.Mutex
	sources  map[string]string // source name -> credentialEnvPrefix ("" = open)
	reported map[string]bool   // Ready reported (avoid spam)
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env %s", key)
	}
	return v
}

func main() {
	sourceType := os.Getenv("SOURCE_TYPE")
	if sourceType == "" {
		sourceType = "vm-alertmanager"
	}
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}
	a := &adapter{
		mgr:        NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		sourceType: sourceType,
		sources:    map[string]string{},
		reported:   map[string]bool{},
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("signal-vmalertmanager adapter starting (type=%s, listen=%s)", sourceType, listen)

	go a.registryLoop(ctx)

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

// registryLoop keeps the served-source map fresh (cron-adapter pattern) and
// reports Ready once per source — there is no config to validate; a source of
// this type is served as soon as it exists.
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
	next := map[string]string{}
	for _, info := range infos {
		next[info.Name] = info.CredentialEnvPrefix
		a.mu.Lock()
		done := a.reported[info.Name]
		a.mu.Unlock()
		if !done {
			// name the webhook path so `kubectl get signalsource -o yaml`
			// tells the operator exactly what to target
			msg := fmt.Sprintf("served by signal-vmalertmanager — POST <service>/webhook/%s", info.Name)
			if err := a.mgr.ReportStatus(ctx, info.Name, true, "AdapterReady", msg); err == nil {
				a.mu.Lock()
				a.reported[info.Name] = true
				a.mu.Unlock()
			}
		}
	}
	a.mu.Lock()
	a.sources = next
	a.mu.Unlock()
}

// source resolves a served source's credential prefix (second return: served).
func (a *adapter) source(name string) (string, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prefix, ok := a.sources[name]
	return prefix, ok
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
	prefix, served := a.source(name)
	if !served {
		// VMAlertmanager retries cover the source-poll lag
		writeJSON(w, 404, map[string]string{"error": fmt.Sprintf("unknown or unserved source %q", name)})
		return
	}
	// opt-in bearer auth from the projected source credential (fail closed
	// when the source is credentialed but the projection is missing)
	if prefix != "" {
		want := os.Getenv(prefix + "TOKEN")
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			writeJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "read body"})
		return
	}
	var payload amPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid Alertmanager webhook JSON"})
		return
	}

	signals := normalize(payload.Alerts)
	if len(signals) == 0 {
		writeJSON(w, 200, map[string]any{"queued": 0, "reason": "no firing alerts"})
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

// normalize converts firing alerts to contract signals with built-in-path
// parity. No grouping/dedup here — the manager owns that.
func normalize(alerts []amAlert) []Signal {
	var signals []Signal
	for _, alert := range alerts {
		if alert.Status != "firing" {
			continue
		}
		fp := alert.Fingerprint
		if fp == "" {
			fp = labelFingerprint(alert.Labels)
		}
		title := "🔍 " + alert.Labels["alertname"]
		if ns := alert.Labels["namespace"]; ns != "" {
			title += " — " + ns
		}
		payloadDoc := map[string]any{
			"labels": alert.Labels, "annotations": alert.Annotations,
			"startsAt": alert.StartsAt, "generatorURL": alert.GeneratorURL,
		}
		payloadJSON, _ := json.MarshalIndent(payloadDoc, "", "  ")
		signals = append(signals, Signal{
			Fingerprint: fp,
			Labels:      alert.Labels,
			Title:       title,
			Payload:     string(payloadJSON),
		})
	}
	return signals
}

// labelFingerprint derives a deterministic fingerprint from sorted label
// pairs — the fallback when a sender omits Alertmanager's own fingerprint
// (the inbound contract rejects empty fingerprints).
func labelFingerprint(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s=%s\n", k, labels[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// signal-alertmanager: webhook-receiving signal adapter. Serves
// SignalSources whose spec.adapter names its SignalAdapter CR (ADAPTER_NAME,
// injected by the reconciler = the CR name) against the operator's signal
// contract by hosting its OWN HTTP endpoint accepting the standard
// Alertmanager webhook format — VictoriaMetrics VMAlertmanager is the
// packaged story, but any Alertmanager-compatible sender works:
//
//	webhook:  POST /webhook/{source}   (LISTEN_ADDR, default :8080; 1 MiB cap)
//	sources:  GET /signal/sources?adapter=<ADAPTER_NAME> (15s poll)
//	push:     POST /signal/inbound     (normalized; alert lane)
//
// Normalization keeps built-in-path parity: firing-only, raw labels,
// "🔍 alertname — namespace" title, per-alert JSON payload. initiationId is
// the webhook's own groupKey verbatim (Alertmanager's group_by decision,
// made upstream) — every firing alert of one group correlates to one
// conversation — with a sorted-groupLabels-hash fallback for a nonstandard
// sender that omits groupKey. The adapter correlates nothing beyond that —
// cooldown and recurrence stay manager-side, keyed on initiationId (and
// routing requires the source's Pipeline claim: unwired sources drop with
// Wired=False).
//
// Auth is opt-in per source: a source with credentialsSecretRef advertises a
// credentialEnvPrefix; the adapter then requires Authorization: Bearer
// matching the projected <prefix>TOKEN value (constant-time). Uncredentialed
// sources accept anonymous posts (parity with the built-in endpoint's
// ClusterIP-only posture).
//
// Environment: MANAGER_URL, ADAPTER_TOKEN, ADAPTER_NAME (default
// "vm-alertmanager" — hand-deployed instances set it themselves; in-cluster
// the reconciler injects the CR name), LISTEN_ADDR (default ":8080" —
// in-cluster the reconciler injects it from SignalAdapter.spec.port),
// POD_NAMESPACE (downward API, always injected by the reconciler — without it
// the adapter cannot compute its own webhook URL and registers nothing),
// projected AGENTOPS_CRED_* vars.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
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

// amPayload is the standard Alertmanager webhook payload. GroupKey is
// Alertmanager's own group_by decision — the correlation id every alert of
// this group adopts verbatim. GroupLabels feeds the fallback for a
// nonstandard sender that posts no GroupKey.
type amPayload struct {
	GroupKey    string            `json:"groupKey"`
	GroupLabels map[string]string `json:"groupLabels"`
	Alerts      []amAlert         `json:"alerts"`
}

type adapter struct {
	mgr        *Manager
	sourceType string
	podNS      string // downward-API namespace ("" only when hand-deployed without it)
	listen     string
	kube       *kubeClient // nil = no in-cluster access
	kubeReason string      // why kube is nil (instruction path)

	mu       sync.Mutex
	sources  map[string]string // source name -> credentialEnvPrefix ("" = open)
	reported map[string]string // last reported reason|message (avoid spam)
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
		sourceType = "vm-alertmanager"
	}
	listen := os.Getenv("LISTEN_ADDR")
	if listen == "" {
		listen = ":8080"
	}
	kube, kubeReason := newInClusterClient(sourceType)
	a := &adapter{
		mgr:        NewManager(mustEnv("MANAGER_URL"), mustEnv("ADAPTER_TOKEN")),
		sourceType: sourceType,
		podNS:      os.Getenv("POD_NAMESPACE"),
		listen:     listen,
		kube:       kube,
		kubeReason: kubeReason,
		sources:    map[string]string{},
		reported:   map[string]string{},
	}
	ctx, stop := signal.NotifyContext(baseContext(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("signal-alertmanager adapter starting (adapter=%s, listen=%s)", sourceType, listen)

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

// registryLoop keeps the served-source map fresh (cron-adapter pattern),
// reconciles sender registration for sources that ask for it, and reports
// each source's Ready state. Everything reruns every poll, so a registration
// that failed for want of RBAC heals as soon as the permission is granted —
// no restart.
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
		reason, msg := a.reconcileRegistration(ctx, info)
		a.report(ctx, info.Name, reason, msg)
	}
	a.mu.Lock()
	a.sources = next
	a.mu.Unlock()
}

// reconcileRegistration ensures the sender-side config for one source and
// returns the Ready reason/message to publish. A source is served either way:
// the webhook is live whether or not the sender could be configured for it,
// so registration trouble reports instructions rather than unserving.
func (a *adapter) reconcileRegistration(ctx context.Context, info SourceInfo) (string, string) {
	reg, err := parseRegister(info.Config)
	if err != nil {
		// a block that does not parse is never an absent one: say what failed
		// and attempt no registration until the block is corrected
		return reasonRegistrationManual, manualInstructions(info.Name, a.podNS, a.manualURL(info.Name), err.Error())
	}
	if reg == nil {
		// no registration asked for: name the path an operator should target
		return reasonAdapterReady, fmt.Sprintf("served by signal-alertmanager — POST %s", a.endpoint(info.Name))
	}
	// default to our own namespace: vm-operator selects VMAlertmanagerConfigs
	// from everywhere by default, so this needs no cross-namespace grant
	ns := reg.Namespace
	if ns == "" {
		ns = a.podNS
	}
	if a.podNS == "" {
		// No namespace fact means no webhook URL. A placeholder written into
		// the sender's live config looks registered and delivers nothing, so
		// computing no URL means registering no object.
		cause := "own webhook URL unknown: POD_NAMESPACE is unset — fill in the adapter's namespace"
		if a.kube == nil {
			// both causes at once, or fixing the first only reveals the second
			cause += "; " + a.kubeReason
		}
		return reasonRegistrationManual, manualInstructions(info.Name, ns, a.manualURL(info.Name), cause)
	}
	return a.register(ctx, info.Name, ns, reg)
}

// register ensures the sender-side object for a source whose webhook URL and
// target namespace are both known, and returns the Ready reason/message.
func (a *adapter) register(ctx context.Context, source, ns string, reg *registerSpec) (string, string) {
	url := a.endpoint(source)
	if a.kube == nil {
		return reasonRegistrationManual, manualInstructions(source, ns, url, a.kubeReason)
	}
	ref, err := a.kube.ensureRegistration(ctx, source, ns, url, reg)
	if err != nil {
		cause := err.Error()
		if errors.Is(err, errForbidden) {
			cause += "; " + accountRemediation(a.sourceType)
		}
		return reasonRegistrationManual, manualInstructions(source, ns, url, cause)
	}
	return reasonAdapterReady, fmt.Sprintf(
		"served by signal-alertmanager — POST %s; sender registered in VMAlertmanagerConfig %s", url, ref)
}

// Ready reasons this adapter reports for a source.
const (
	reasonAdapterReady       = "AdapterReady"
	reasonRegistrationManual = "RegistrationManual"
)

// namespaceSlot stands in for the adapter's namespace in a URL shape the
// operator must complete by hand.
const namespaceSlot = "<adapter-namespace>"

// manualInstructions renders the degraded message: why registration could not
// happen, and exactly what to do instead. An unknown namespace (no
// POD_NAMESPACE, no block namespace) is shown as the slot to fill in, so the
// object to create is always named in full.
func manualInstructions(source, namespace, url, cause string) string {
	if namespace == "" {
		namespace = namespaceSlot
	}
	target := namespace + "/" + registrationName(source)
	return fmt.Sprintf(
		"served, but could not register the sender automatically (%s). Configure it by hand: "+
			"create VMAlertmanagerConfig %s (or add a webhook receiver to your Alertmanager) posting to %s",
		cause, target, url)
}

// manualURL is the URL an operator should configure by hand: the real one
// when computable, otherwise its shape with the unknown namespace left as a
// slot to fill in — shown in a message, never written into the sender.
func (a *adapter) manualURL(source string) string {
	if a.podNS == "" {
		return webhookURL(a.sourceType, namespaceSlot, a.listen, source)
	}
	return a.endpoint(source)
}

// endpoint is this adapter's webhook URL for a source (in-cluster form when
// the reconciler injected the pod namespace; path-only otherwise).
func (a *adapter) endpoint(source string) string {
	if a.podNS == "" {
		return "<service>/webhook/" + source
	}
	return webhookURL(a.sourceType, a.podNS, a.listen, source)
}

// report publishes a source's Ready state, skipping unchanged repeats so the
// 15s loop does not spam the status endpoint.
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

	signals := normalize(payload, bodyDigest(body))
	if len(signals) == 0 {
		writeJSON(w, 200, map[string]any{"queued": 0, "reason": "no firing alerts"})
		return
	}
	res, err := a.mgr.Inbound(r.Context(), name, signals)
	if err != nil {
		// %q escapes control characters in the manager's rejection message,
		// which can echo back webhook-sourced content (initiationId, labels)
		// — unescaped, a crafted value could forge log lines (CWE log
		// injection, gosecurity:S5145).
		log.Printf("inbound %q: %q", name, err.Error())
		writeJSON(w, 502, map[string]string{"error": "manager rejected the signals"})
		return
	}
	writeJSON(w, 200, res)
}

// normalize converts firing alerts to contract signals with built-in-path
// parity. Every signal of this webhook body shares one initiationId — the
// group's own correlation id, resolved once per payload, never per alert.
// No dedup/cooldown here — the manager owns that, keyed on that id.
//
// digest is the hash of the raw request body; Alertmanager re-delivers a
// failed notification byte-identical, so it makes each alert's signalId stable
// across the retry and different for any changed body.
func normalize(payload amPayload, digest string) []Signal {
	initiationID := payload.GroupKey
	if initiationID == "" {
		initiationID = groupLabelsFallbackID(payload.GroupLabels)
	}
	var signals []Signal
	for i, alert := range payload.Alerts {
		if alert.Status != "firing" {
			continue
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
			InitiationID: initiationID,
			SignalID:     signalID(digest, alert.Fingerprint, i),
			Labels:       alert.Labels,
			Title:        title,
			Payload:      string(payloadJSON),
		})
	}
	return signals
}

// bodyDigest returns the hex SHA-256 of a raw webhook body.
func bodyDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// signalID is am:<body digest>:<alert fingerprint, or its index when the
// sender omits one>.
func signalID(digest, fingerprint string, index int) string {
	if fingerprint == "" {
		fingerprint = strconv.Itoa(index)
	}
	return "am:" + digest + ":" + fingerprint
}

// groupLabelsFallbackID derives a deterministic id from sorted groupLabels
// pairs — the fallback when a nonstandard sender omits Alertmanager's own
// groupKey (the inbound contract rejects an empty initiationId).
func groupLabelsFallbackID(labels map[string]string) string {
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

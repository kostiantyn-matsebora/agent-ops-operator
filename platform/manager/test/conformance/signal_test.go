//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The signal adapter conformance set. Every adapter: normalized emission
// (fingerprint, labels, kind, payload; no adapter-side grouping), bearer
// auth, source scoping, and a rejected post that is retried or surfaced —
// never dropped behind a healthy-looking process. A chat-originating adapter
// additionally carries the channel label on every signal.

// accepted returns the accepted posts for a source.
func accepted(mgr *FakeManager, source string) []SignalPost {
	var out []SignalPost
	for _, p := range mgr.SignalPosts() {
		if p.Source == source && !p.Rejected {
			out = append(out, p)
		}
	}
	return out
}

func attempts(mgr *FakeManager, source string) int {
	n := 0
	for _, p := range mgr.SignalPosts() {
		if p.Source == source {
			n++
		}
	}
	return n
}

func assertNormalized(t *testing.T, post SignalPost, source string) {
	t.Helper()
	if !post.Bearer {
		t.Fatalf("a signal post must present the bearer token")
	}
	if post.Source != source {
		t.Fatalf("the post must name the source it serves: %q", post.Source)
	}
	if len(post.Signals) == 0 {
		t.Fatalf("an empty post")
	}
	for _, s := range post.Signals {
		if fp, _ := s["fingerprint"].(string); fp == "" {
			t.Fatalf("every signal needs a fingerprint: %v", s)
		}
		if _, ok := s["payload"]; !ok {
			t.Fatalf("every signal carries a payload: %v", s)
		}
	}
}

// ---- signal-cron -----------------------------------------------------------

func TestSignalCronConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	mgr.ServeSources(SourceInfo{Name: "tick", Config: json.RawMessage(`{"schedule":"* * * * *","input":"echo tick","title":"tick"}`)})
	// A cursor two minutes back makes the first evaluation fire at once
	// instead of waiting for the next minute boundary.
	mgr.Seed("signal", "tick", "last-fire", time.Now().Add(-2*time.Minute).UTC().Format(time.RFC3339))
	// Reject first: a failed post must be retried, not dropped.
	mgr.RejectSignals(500)

	p := start(t, "signal-cron", build(t, "signals/cron"), contractEnv(mgr, "cron", 0))
	waitFor(t, "a first (rejected) post", 30*time.Second, func() bool { return attempts(mgr, "tick") >= 1 })
	waitFor(t, "a retry of the rejected post", 45*time.Second, func() bool { return attempts(mgr, "tick") >= 2 })
	if p.Exited() {
		t.Fatalf("a rejected post must not kill the adapter:\n%s", p.Output())
	}
	mgr.RejectSignals(0)
	waitFor(t, "an accepted post", 45*time.Second, func() bool { return len(accepted(mgr, "tick")) >= 1 })
	post := accepted(mgr, "tick")[0]
	assertNormalized(t, post, "tick")
	s := post.Signals[0]
	if s["kind"] != "job" || !strings.HasPrefix(s["fingerprint"].(string), "tick@") || s["payload"] != "echo tick" {
		t.Fatalf("cron signal shape: %v", s)
	}
	if mgr.State("signal", "tick", "last-fire") == "" {
		t.Fatalf("the cursor must be persisted through the state API")
	}
	if mgr.Unauthorized() != 0 {
		t.Fatalf("%d unauthenticated requests", mgr.Unauthorized())
	}
}

// ---- signal-alertmanager ---------------------------------------------------

func TestSignalAlertmanagerConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	mgr.ServeSources(SourceInfo{Name: "vm-alerts", Config: json.RawMessage(`{}`)})
	port := freePort(t)
	p := start(t, "signal-alertmanager", build(t, "signals/alertmanager"), contractEnv(mgr, "alertmanager", port))
	p.Port = port
	waitHealthy(t, p)
	waitFor(t, "the adapter to list its sources", 10*time.Second, func() bool { return mgr.Listed("signal", "alertmanager") > 0 })
	// The adapter refreshes its served set on its own cadence; the first
	// webhook may race it, so retry until the source is known.
	body := fixture(t, "alertmanager-webhook.json")
	var code int
	var resp string
	waitFor(t, "the webhook to accept the fixture", 20*time.Second, func() bool {
		code, resp = postJSON(t, p.URL()+"/webhook/vm-alerts", body, nil)
		return code == 200
	})
	if !strings.Contains(resp, `"queued":2`) {
		t.Fatalf("two firing alerts must be queued: %d %s", code, resp)
	}
	waitFor(t, "the post", 10*time.Second, func() bool { return len(accepted(mgr, "vm-alerts")) >= 1 })
	post := accepted(mgr, "vm-alerts")[0]
	assertNormalized(t, post, "vm-alerts")
	// No adapter-side grouping: one signal per firing alert, each its own fingerprint.
	if len(post.Signals) != 2 || post.Signals[0]["fingerprint"] != "fp-a" || post.Signals[1]["fingerprint"] != "fp-c" {
		t.Fatalf("expected the two firing alerts as two signals: %v", post.Signals)
	}
	if _, hasKind := post.Signals[0]["kind"]; hasKind {
		t.Fatalf("alert-lane signals carry no kind: %v", post.Signals[0])
	}
	// An unknown source is refused, not silently accepted.
	if code, _ := postJSON(t, p.URL()+"/webhook/nope", body, nil); code != 404 {
		t.Fatalf("unknown source must be 404, got %d", code)
	}
	// A rejected post is SURFACED to the caller: the webhook fails.
	mgr.RejectSignals(500)
	code, resp = postJSON(t, p.URL()+"/webhook/vm-alerts", body, nil)
	if code/100 == 2 {
		t.Fatalf("a rejected manager post must fail the webhook, got %d %s", code, resp)
	}
	if p.Exited() {
		t.Fatalf("a rejected post must not kill the adapter")
	}
}

// ---- signal-telegram (chat-originating) ------------------------------------

func TestSignalTelegramConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	mgr.ServeSources(SourceInfo{Name: "tg-chat", Config: json.RawMessage(`{"chatId":"-1001234567890","channel":"telegram-ops"}`)})
	port := freePort(t)
	p := start(t, "signal-telegram", build(t, "signals/telegram"), contractEnv(mgr, "telegram", port))
	p.Port = port
	waitHealthy(t, p)
	waitFor(t, "the adapter to list its sources", 10*time.Second, func() bool { return mgr.Listed("signal", "telegram") > 0 })
	body := fixture(t, "telegram-update-message.json")
	waitFor(t, "the update to be accepted", 20*time.Second, func() bool {
		code, _ := postJSON(t, p.URL()+"/updates", body, nil)
		return code/100 == 2
	})
	waitFor(t, "the post", 10*time.Second, func() bool { return len(accepted(mgr, "tg-chat")) >= 1 })
	post := accepted(mgr, "tg-chat")[0]
	assertNormalized(t, post, "tg-chat")
	s := post.Signals[0]
	labels, _ := s["labels"].(map[string]any)
	if s["kind"] != "chat" || s["fingerprint"] != "tg-77" || s["payload"] != "check the disk" {
		t.Fatalf("chat signal shape: %v", s)
	}
	if labels["agentops.dev/channel"] != "telegram-ops" {
		t.Fatalf("a chat signal MUST carry the channel label: %v", labels)
	}
	if labels["agentops.dev/sender"] != "operator" {
		t.Fatalf("the sender label: %v", labels)
	}
	// Rejected → surfaced to the router as a failed push.
	mgr.RejectSignals(500)
	code, resp := postJSON(t, p.URL()+"/updates", body, nil)
	if code/100 == 2 {
		t.Fatalf("a rejected manager post must fail the push, got %d %s", code, resp)
	}
}

// ---- signal-k8s-events -----------------------------------------------------

func TestSignalK8sEventsConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	api := NewFakeAPIServer(t)
	p := k8sEventsStartAdapterAndAssertWatch(t, mgr, api)
	k8sEventsAssertEventBecomesNormalizedSignal(t, mgr, api)
	k8sEventsAssertRejectedPostRetriedOrReported(t, mgr, api, p)
}

func k8sEventsEventPayload(now, name string) map[string]any {
	return map[string]any{
		"apiVersion": "v1", "kind": "Event",
		"metadata":       map[string]any{"name": name, "namespace": "apps", "creationTimestamp": now},
		"involvedObject": map[string]any{"kind": "Pod", "name": "api-7d9f8b6c4-x2k9q", "namespace": "apps"},
		"reason":         "BackOff", "type": "Warning", "count": 3,
		"message":        "Back-off restarting failed container api in pod api-7d9f8b6c4-x2k9q",
		"firstTimestamp": now, "lastTimestamp": now,
	}
}

func k8sEventsStartAdapterAndAssertWatch(t *testing.T, mgr *FakeManager, api *FakeAPIServer) *Process {
	t.Helper()
	mgr.ServeSources(SourceInfo{Name: "events", Config: json.RawMessage(`{"namespaces":["apps"]}`)})
	env := append(contractEnv(mgr, "k8s-events", 0), api.Env()...)
	p := start(t, "signal-k8s-events", build(t, "signals/k8s-events"), env)
	waitFor(t, "the adapter to list its sources", 15*time.Second, func() bool { return mgr.Listed("signal", "k8s-events") > 0 })
	waitFor(t, "a watch on the namespace's events", 30*time.Second, func() bool {
		for _, r := range api.Requests() {
			if strings.Contains(r, "/namespaces/apps/events") && strings.Contains(r, "watch=") {
				return true
			}
		}
		return false
	})
	if p.Exited() {
		t.Fatalf("exited:\n%s", p.Output())
	}
	return p
}

func k8sEventsAssertEventBecomesNormalizedSignal(t *testing.T, mgr *FakeManager, api *FakeAPIServer) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	api.Push("/api/v1/namespaces/apps/events", "ADDED", k8sEventsEventPayload(now, "api-backoff.1"))
	waitFor(t, "the event to become a signal", 60*time.Second, func() bool { return len(accepted(mgr, "events")) >= 1 })
	post := accepted(mgr, "events")[0]
	assertNormalized(t, post, "events")
	s := post.Signals[0]
	if !strings.Contains(fmt.Sprint(s["payload"]), "BackOff") {
		t.Fatalf("the payload must carry the event: %v", s)
	}
	labels, _ := s["labels"].(map[string]any)
	if labels["namespace"] != "apps" {
		t.Fatalf("the namespace label: %v", labels)
	}
}

// k8sEventsAssertRejectedPostRetriedOrReported: a rejected post is retried
// or reported — never silently dropped.
func k8sEventsAssertRejectedPostRetriedOrReported(t *testing.T, mgr *FakeManager, api *FakeAPIServer, p *Process) {
	t.Helper()
	mgr.RejectSignals(500)
	before := attempts(mgr, "events")
	now := time.Now().UTC().Format(time.RFC3339)
	api.Push("/api/v1/namespaces/apps/events", "ADDED", k8sEventsEventPayload(now, "api-backoff.2"))
	waitFor(t, "the rejected post attempt", 60*time.Second, func() bool { return attempts(mgr, "events") > before })
	afterFirst := attempts(mgr, "events")
	waitFor(t, "a retry or a status report about the rejection", 90*time.Second, func() bool {
		if attempts(mgr, "events") > afterFirst {
			return true
		}
		for _, r := range mgr.StatusReports() {
			if r.Name == "events" && !r.Ready {
				return true
			}
		}
		return false
	})
	if p.Exited() {
		t.Fatalf("a rejected post must not kill the adapter")
	}
}

// ---- signal-ha -------------------------------------------------------------

func TestSignalHAConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	ha := NewFakeHA(t, "ha-long-lived-token")
	p := haStartAdapterAndAssertAuthenticatedSubscription(t, mgr, ha)
	rec := haAssertLogRecordBecomesNormalizedSignal(t, mgr, ha)
	haAssertRejectedPostRetriedOrReported(t, mgr, ha, p, rec)
}

func haStartAdapterAndAssertAuthenticatedSubscription(t *testing.T, mgr *FakeManager, ha *FakeHA) *Process {
	t.Helper()
	mgr.ServeSources(SourceInfo{Name: "ha", CredentialEnvPrefix: "AGENTOPS_CRED_HA_",
		Config: json.RawMessage(`{"endpoint":"` + ha.Endpoint() + `","backfill":false}`)})
	env := append(contractEnv(mgr, "home-assistant", 0), "AGENTOPS_CRED_HA_token=ha-long-lived-token")
	p := start(t, "signal-ha", build(t, "signals/ha"), env)
	waitFor(t, "an authenticated subscription", 30*time.Second, func() bool {
		for _, c := range ha.Commands() {
			if c == "subscribe_events" {
				return true
			}
		}
		return false
	})
	if ha.Authed() != 1 {
		t.Fatalf("exactly one session must authenticate, got %d", ha.Authed())
	}
	return p
}

func haAssertLogRecordBecomesNormalizedSignal(t *testing.T, mgr *FakeManager, ha *FakeHA) map[string]any {
	t.Helper()
	rec := map[string]any{
		"name": "homeassistant.components.zwave_js", "level": "ERROR",
		"message":   []any{"Failed to connect to the Z-Wave JS server"},
		"source":    []any{"components/zwave_js/__init__.py", 212},
		"timestamp": float64(time.Now().Unix()), "first_occurred": float64(time.Now().Unix()), "count": 1,
	}
	waitFor(t, "the pushed log event to reach a session", 10*time.Second, func() bool { return ha.PushLog(rec) >= 1 })
	waitFor(t, "the log record to become a signal", 60*time.Second, func() bool { return len(accepted(mgr, "ha")) >= 1 })
	post := accepted(mgr, "ha")[0]
	assertNormalized(t, post, "ha")
	if !strings.Contains(fmt.Sprint(post.Signals[0]["payload"]), "zwave_js") {
		t.Fatalf("the payload must carry the record: %v", post.Signals[0])
	}
	return rec
}

// haAssertRejectedPostRetriedOrReported: rejected → retried or reported.
func haAssertRejectedPostRetriedOrReported(t *testing.T, mgr *FakeManager, ha *FakeHA, p *Process, rec map[string]any) {
	t.Helper()
	mgr.RejectSignals(500)
	before := attempts(mgr, "ha")
	rec2 := map[string]any{}
	for k, v := range rec {
		rec2[k] = v
	}
	rec2["name"] = "homeassistant.components.mqtt"
	rec2["source"] = []any{"components/mqtt/client.py", 88}
	ha.PushLog(rec2)
	waitFor(t, "the rejected post attempt", 60*time.Second, func() bool { return attempts(mgr, "ha") > before })
	afterFirst := attempts(mgr, "ha")
	waitFor(t, "a retry or a status report about the rejection", 90*time.Second, func() bool {
		if attempts(mgr, "ha") > afterFirst {
			return true
		}
		for _, r := range mgr.StatusReports() {
			if r.Name == "ha" && !r.Ready {
				return true
			}
		}
		return false
	})
	if p.Exited() {
		t.Fatalf("a rejected post must not kill the adapter")
	}
}

//go:build conformance

package conformance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The signal adapter conformance set. Every adapter: normalized emission
// (initiationId, labels, kind, payload; routing/grouping is id-equality,
// never adapter-side), bearer auth, source scoping, and a rejected post that
// is retried or surfaced — never dropped behind a healthy-looking process. A
// chat-originating adapter additionally carries the channel label on every
// signal.

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

// maxSignalIDBytes is the contract's bound on a signalId.
const maxSignalIDBytes = 256

// signalIDsOf returns the signalId of every signal in a post, in order,
// failing the test on one that is missing or outside the contract's bound
// (1 to 256 bytes, printable ASCII without spaces).
func signalIDsOf(t *testing.T, post SignalPost) []string {
	t.Helper()
	ids := make([]string, 0, len(post.Signals))
	for _, s := range post.Signals {
		id, _ := s["signalId"].(string)
		if id == "" || len(id) > maxSignalIDBytes {
			t.Fatalf("every signal needs a signalId of 1 to %d bytes: %v", maxSignalIDBytes, s)
		}
		for _, r := range id {
			if r <= ' ' || r > '~' {
				t.Fatalf("signalId must be printable ASCII without spaces: %q", id)
			}
		}
		ids = append(ids, id)
	}
	return ids
}

// assertResendRepeatsSignalIDs: the same message sent again (after a 5xx, or a
// redelivery) carries the SAME signalIds, so the manager stores it once.
func assertResendRepeatsSignalIDs(t *testing.T, first, again SignalPost) {
	t.Helper()
	a, b := signalIDsOf(t, first), signalIDsOf(t, again)
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Fatalf("a re-sent message must repeat its signalIds: %v, then %v", a, b)
	}
}

// assertDifferentMessageHasDifferentSignalIDs: no signalId of other appears in
// first.
func assertDifferentMessageHasDifferentSignalIDs(t *testing.T, first, other SignalPost) {
	t.Helper()
	seen := map[string]bool{}
	for _, id := range signalIDsOf(t, first) {
		seen[id] = true
	}
	for _, id := range signalIDsOf(t, other) {
		if seen[id] {
			t.Fatalf("a different message reused signalId %q", id)
		}
	}
}

// rejected returns the rejected posts for a source, oldest first.
func rejected(mgr *FakeManager, source string) []SignalPost {
	var out []SignalPost
	for _, p := range mgr.SignalPosts() {
		if p.Source == source && p.Rejected && len(p.Signals) > 0 {
			out = append(out, p)
		}
	}
	return out
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
	signalIDsOf(t, post)
	for _, s := range post.Signals {
		if id, _ := s["initiationId"].(string); id == "" {
			t.Fatalf("every signal needs an initiationId: %v", s)
		}
		if _, ok := s["payload"]; !ok {
			t.Fatalf("every signal carries a payload: %v", s)
		}
	}
}

// assertRetriedBatchRepeatsSignalIDs: where the adapter retried the batch the
// manager refused, the retry carries the SAME signalIds as the first attempt.
// An adapter that reports the failure instead of retrying has nothing to
// compare, and passes.
func assertRetriedBatchRepeatsSignalIDs(t *testing.T, mgr *FakeManager, source string) {
	t.Helper()
	rej := rejected(mgr, source)
	if len(rej) < 2 {
		return
	}
	assertResendRepeatsSignalIDs(t, rej[0], rej[1])
}

// ---- signal-cron -----------------------------------------------------------

func TestSignalCronConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	mgr.ServeSources(SourceInfo{Name: "tick", Config: json.RawMessage(`{"schedule":"0 * * * *","input":"echo tick","title":"tick"}`)})
	// An hourly schedule with a cursor over an hour back makes the first
	// evaluation fire at once, and keeps the tick (so the signalId) the same
	// across the retries below except at the hour boundary itself.
	mgr.Seed("signal", "tick", "last-fire", time.Now().Add(-61*time.Minute).UTC().Format(time.RFC3339))
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
	// The same tick re-fired after the 5xx repeats its signalId.
	rej := rejected(mgr, "tick")
	assertResendRepeatsSignalIDs(t, rej[0], rej[1])
	assertResendRepeatsSignalIDs(t, rej[len(rej)-1], post)
	s := post.Signals[0]
	// initiationId is the source's own job identity, not a per-tick value —
	// every tick of this job shares it (design: "cron | the source's job
	// identity — all ticks, one conversation").
	if s["kind"] != "job" || s["initiationId"] != "tick" || s["payload"] != "echo tick" {
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
	// One signal per firing alert, but both correlate to the SAME group: the
	// webhook's own groupKey, carried verbatim as every signal's
	// initiationId (design: "alertmanager | webhook groupKey verbatim").
	const wantGroupKey = `{}/{alertname="A"}`
	if len(post.Signals) != 2 ||
		post.Signals[0]["initiationId"] != wantGroupKey || post.Signals[1]["initiationId"] != wantGroupKey {
		t.Fatalf("expected the two firing alerts as two signals sharing the group's initiationId: %v", post.Signals)
	}
	if _, hasKind := post.Signals[0]["kind"]; hasKind {
		t.Fatalf("alert-lane signals carry no kind: %v", post.Signals[0])
	}
	// An unknown source is refused, not silently accepted, and the refusal
	// names the source it refused.
	if code, resp := postJSON(t, p.URL()+"/webhook/nope", body, nil); code != 404 || !strings.Contains(resp, `nope`) {
		t.Fatalf("unknown source must be 404 naming the source, got %d %s", code, resp)
	}
	// A different body is a different message: no signalId carries over.
	changed := append(append([]byte{}, body...), '\n')
	if code, resp := postJSON(t, p.URL()+"/webhook/vm-alerts", changed, nil); code != 200 {
		t.Fatalf("a changed body must be accepted: %d %s", code, resp)
	}
	acc := accepted(mgr, "vm-alerts")
	assertDifferentMessageHasDifferentSignalIDs(t, acc[0], acc[len(acc)-1])
	// A rejected post is SURFACED to the caller: the webhook fails, and the
	// re-delivery of the identical body repeats every signalId.
	mgr.RejectSignals(500)
	code, resp = postJSON(t, p.URL()+"/webhook/vm-alerts", body, nil)
	if code/100 == 2 {
		t.Fatalf("a rejected manager post must fail the webhook, got %d %s", code, resp)
	}
	rej := rejected(mgr, "vm-alerts")
	assertResendRepeatsSignalIDs(t, acc[0], rej[len(rej)-1])
	mgr.RejectSignals(0)
	if code, resp := postJSON(t, p.URL()+"/webhook/vm-alerts", body, nil); code != 200 {
		t.Fatalf("the re-delivery must be accepted: %d %s", code, resp)
	}
	acc = accepted(mgr, "vm-alerts")
	assertResendRepeatsSignalIDs(t, rej[len(rej)-1], acc[len(acc)-1])
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
	if s["kind"] != "chat" || s["initiationId"] != "tg-tg-chat-77" || s["payload"] != "check the disk" {
		t.Fatalf("chat signal shape: %v", s)
	}
	if labels["agentops.dev/channel"] != "telegram-ops" {
		t.Fatalf("a chat signal MUST carry the channel label: %v", labels)
	}
	if labels["agentops.dev/sender"] != "operator" {
		t.Fatalf("the sender label: %v", labels)
	}
	if ids := signalIDsOf(t, post); ids[0] != "tg-tg-chat-77" {
		t.Fatalf("signalId = %v, want tg-<source>-<update id>", ids)
	}
	// A different update is a different message.
	next := bytes.Replace(body, []byte(`"update_id": 77`), []byte(`"update_id": 78`), 1)
	if bytes.Equal(next, body) {
		t.Fatalf("the fixture no longer carries update_id 77 — update this case")
	}
	if code, resp := postJSON(t, p.URL()+"/updates", next, nil); code/100 != 2 {
		t.Fatalf("the next update must be accepted: %d %s", code, resp)
	}
	acc := accepted(mgr, "tg-chat")
	assertDifferentMessageHasDifferentSignalIDs(t, acc[0], acc[len(acc)-1])
	// Rejected → surfaced to the router as a failed push; the router's
	// redelivery of the same update repeats the signalId.
	mgr.RejectSignals(500)
	code, resp := postJSON(t, p.URL()+"/updates", body, nil)
	if code/100 == 2 {
		t.Fatalf("a rejected manager post must fail the push, got %d %s", code, resp)
	}
	rej := rejected(mgr, "tg-chat")
	assertResendRepeatsSignalIDs(t, acc[0], rej[len(rej)-1])
}

// ---- signal-k8s-events -----------------------------------------------------

func TestSignalK8sEventsConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	api := NewFakeAPIServer(t)
	p := k8sEventsStartAdapterAndAssertWatch(t, mgr, api, `{"namespaces":["apps"]}`)
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

func k8sEventsStartAdapterAndAssertWatch(t *testing.T, mgr *FakeManager, api *FakeAPIServer, config string) *Process {
	t.Helper()
	mgr.ServeSources(SourceInfo{Name: "events", Config: json.RawMessage(config)})
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
	defer assertRetriedBatchRepeatsSignalIDs(t, mgr, "events")
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
	p := haStartAdapterAndAssertAuthenticatedSubscription(t, mgr, ha, `{"endpoint":"`+ha.Endpoint()+`","backfill":false}`)
	rec := haAssertLogRecordBecomesNormalizedSignal(t, mgr, ha)
	haAssertRejectedPostRetriedOrReported(t, mgr, ha, p, rec)
}

func haStartAdapterAndAssertAuthenticatedSubscription(t *testing.T, mgr *FakeManager, ha *FakeHA, config string) *Process {
	t.Helper()
	mgr.ServeSources(SourceInfo{Name: "ha", CredentialEnvPrefix: "AGENTOPS_CRED_HA_", Config: json.RawMessage(config)})
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
	defer assertRetriedBatchRepeatsSignalIDs(t, mgr, "ha")
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

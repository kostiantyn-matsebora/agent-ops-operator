//go:build conformance

package conformance

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"text/template"
	"time"
)

// ---- signal-pulse (push-mode, registers its own sender) --------------------
//
// Pulse sends outgoing webhooks only, rendering a payload template the
// adapter registers. So the suite never hand-writes a Pulse body: it reads
// the template the binary registered with the fake Pulse, renders it the way
// Pulse does, signs it with the registered secret, and posts it to the
// registered URL. What reaches the adapter is exactly what the adapter asked
// Pulse to send.

const (
	pulseSource        = "pulse-alerts"
	pulseAPIToken      = "pulse-api-token"
	pulseSigningSecret = "conformance-signing-secret"
)

// pulseAlert is Pulse's notification context for one alert — the field
// names a Generic webhook template may reference. Rendering with
// missingkey=error makes a template naming a field outside this set fail the
// suite instead of silently rendering "<no value>".
//
// The names are signals/pulse's documented assumption (payload.go): `.Event`
// was confirmed against a live Pulse, `.StartTime` against Pulse's published
// docs only. This set is that assumption, restated — not a capture.
func pulseAlert(event, id, resource, startTime string) map[string]any {
	return map[string]any{
		"Event": event, "ID": id, "ResourceName": resource, "ResourceType": "vm",
		"Node": "pve1", "Level": "critical", "Message": "CPU usage above 95% on " + resource,
		"Value": "97", "StartTime": startTime,
	}
}

func TestSignalPulseConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	pulse := NewFakePulse(t, pulseAPIToken)
	p, hook := pulseStartAdapterAndAssertRegistration(t, mgr, pulse)
	first := pulseAssertFiringAlertBecomesNormalizedSignal(t, mgr, p, hook)
	pulseAssertRedeliveryRepeatsTheID(t, mgr, p, hook, first)
	pulseAssertUnrelatedAlertGetsItsOwnID(t, mgr, p, hook, first)
	pulseAssertResolvedAndUnsignedPushNothing(t, mgr, p, hook)
	pulseAssertUnknownSourceRefusalNamesIt(t, p)
	pulseAssertRejectedPostSurfaced(t, mgr, p, hook)
	if mgr.Unauthorized() != 0 || pulse.Unauthorized() != 0 {
		t.Fatalf("unauthenticated requests: manager %d, pulse %d", mgr.Unauthorized(), pulse.Unauthorized())
	}
}

func pulseStartAdapterAndAssertRegistration(t *testing.T, mgr *FakeManager, pulse *FakePulse) (*Process, PulseWebhook) {
	t.Helper()
	port := freePort(t)
	webhookURL := fmt.Sprintf("http://127.0.0.1:%d/webhook/%s", port, pulseSource)
	cfg, _ := json.Marshal(map[string]string{"baseUrl": pulse.URL(), "webhookUrl": webhookURL})
	mgr.ServeSources(SourceInfo{Name: pulseSource, CredentialEnvPrefix: "AGENTOPS_CRED_PULSE_", Config: cfg})
	env := append(contractEnv(mgr, "pulse", port),
		"AGENTOPS_CRED_PULSE_token="+pulseAPIToken,
		"AGENTOPS_CRED_PULSE_signingSecret="+pulseSigningSecret)
	p := start(t, "signal-pulse", build(t, "signals/pulse"), env)
	p.Port = port
	waitHealthy(t, p)
	waitFor(t, "the webhook to be registered with Pulse", 20*time.Second, func() bool { return len(pulse.Webhooks()) > 0 })
	hooks := pulse.Webhooks()
	if len(hooks) != 1 {
		t.Fatalf("exactly one webhook must be registered, got %d: %+v", len(hooks), hooks)
	}
	hook := hooks[0]
	registered, err := url.Parse(hook.URL)
	if err != nil {
		t.Fatalf("the registered URL does not parse: %v", err)
	}
	// The registered URL carries a rotation-detection digest of the signing
	// secret on its query string (signals/pulse/register.go) — Pulse masks
	// every registered header's VALUE on read, so the digest cannot ride in
	// a header. Check the digest first, then compare the target ignoring
	// that query, rather than an exact string match.
	wantDigest := sha256.Sum256([]byte(pulseSigningSecret))
	if got := registered.Query().Get("agentops_secret_digest"); got != "sha256:"+hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("the registered URL must carry a digest of the signing secret for rotation detection, got %q in %s", got, hook.URL)
	}
	registered.RawQuery = ""
	if registered.String() != webhookURL || !hook.Enabled || hook.SigningSecret != pulseSigningSecret {
		t.Fatalf("the registered webhook must target this adapter's endpoint, enabled, with the source's secret: %+v", hook)
	}
	waitFor(t, "a Ready report naming the registration", 10*time.Second, func() bool {
		for _, r := range mgr.StatusReports() {
			if r.Name == pulseSource && r.Ready && strings.Contains(r.Message, webhookURL) {
				return true
			}
		}
		return false
	})
	return p, hook
}

// pulseDeliver renders the registered template, signs it as Pulse does
// (v1= HMAC-SHA256 over timestamp + "." + body) and posts it, with the
// registered headers Pulse attaches to every delivery.
func pulseDeliver(t *testing.T, url string, hook PulseWebhook, alert map[string]any, signed bool) (int, string) {
	t.Helper()
	tmpl, err := template.New("pulse").Option("missingkey=error").Parse(hook.Template)
	if err != nil {
		t.Fatalf("the registered template does not parse: %v", err)
	}
	var body bytes.Buffer
	if err := tmpl.Execute(&body, alert); err != nil {
		t.Fatalf("the registered template names a field Pulse's context lacks: %v", err)
	}
	headers := map[string]string{}
	for k, v := range hook.Headers {
		headers[k] = v
	}
	if signed {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(hook.SigningSecret))
		mac.Write([]byte(ts + "."))
		mac.Write(body.Bytes())
		headers["X-Pulse-Timestamp"] = ts
		headers["X-Pulse-Signature"] = "v1=" + hex.EncodeToString(mac.Sum(nil))
	}
	return postJSON(t, url, body.Bytes(), headers)
}

func pulseAssertFiringAlertBecomesNormalizedSignal(t *testing.T, mgr *FakeManager, p *Process, hook PulseWebhook) map[string]any {
	t.Helper()
	alert := pulseAlert("alert", "cpu-vm-101", "vm-101", "2026-10-21T08:00:00Z")
	// The served set is published just after registration, so the first
	// delivery may race it — Pulse would retry, and so does the suite.
	var code int
	var resp string
	waitFor(t, "the signed delivery to be accepted", 20*time.Second, func() bool {
		code, resp = pulseDeliver(t, hook.URL, hook, alert, true)
		return code == 200
	})
	if !strings.Contains(resp, `"queued":1`) {
		t.Fatalf("one firing alert must be queued: %d %s", code, resp)
	}
	posts := accepted(mgr, pulseSource)
	if len(posts) != 1 || len(posts[0].Signals) != 1 {
		t.Fatalf("one firing alert must push exactly one signal: %+v", posts)
	}
	assertNormalized(t, posts[0], pulseSource)
	s := posts[0].Signals[0]
	id, _ := s["initiationId"].(string)
	if !strings.Contains(id, "vm-101") || !strings.Contains(id, "cpu-vm-101") {
		t.Fatalf("the initiationId must join the resource and the alert identity: %q", id)
	}
	if _, hasKind := s["kind"]; hasKind {
		t.Fatalf("alert-lane signals carry no kind: %v", s)
	}
	labels, _ := s["labels"].(map[string]any)
	if labels["resource"] != "vm-101" || labels["severity"] != "critical" {
		t.Fatalf("the resource and severity labels: %v", labels)
	}
	return s
}

// pulseAssertRedeliveryRepeatsTheID: Pulse retrying the same occurrence
// mints the same id, so the manager's cooldown can absorb it.
func pulseAssertRedeliveryRepeatsTheID(t *testing.T, mgr *FakeManager, p *Process, hook PulseWebhook, first map[string]any) {
	t.Helper()
	alert := pulseAlert("alert", "cpu-vm-101", "vm-101", "2026-10-21T08:00:00Z")
	if code, resp := pulseDeliver(t, hook.URL, hook, alert, true); code != 200 {
		t.Fatalf("a redelivery must be accepted: %d %s", code, resp)
	}
	posts := accepted(mgr, pulseSource)
	if len(posts) != 2 {
		t.Fatalf("the redelivery must be pushed, not deduplicated adapter-side: %d posts", len(posts))
	}
	if got := posts[1].Signals[0]["initiationId"]; got != first["initiationId"] {
		t.Fatalf("a redelivered occurrence must repeat its initiationId: %v, then %v", first["initiationId"], got)
	}
}

func pulseAssertUnrelatedAlertGetsItsOwnID(t *testing.T, mgr *FakeManager, p *Process, hook PulseWebhook, first map[string]any) {
	t.Helper()
	alert := pulseAlert("alert", "offline-nas-1", "nas-1", "2026-10-21T08:01:00Z")
	if code, resp := pulseDeliver(t, hook.URL, hook, alert, true); code != 200 {
		t.Fatalf("an unrelated alert must be accepted: %d %s", code, resp)
	}
	posts := accepted(mgr, pulseSource)
	got := posts[len(posts)-1].Signals[0]["initiationId"]
	if got == first["initiationId"] {
		t.Fatalf("two unrelated alerts must never share an initiationId: both %v", got)
	}
}

func pulseAssertResolvedAndUnsignedPushNothing(t *testing.T, mgr *FakeManager, p *Process, hook PulseWebhook) {
	t.Helper()
	before := attempts(mgr, pulseSource)
	resolved := pulseAlert("resolved", "cpu-vm-101", "vm-101", "2026-10-21T08:00:00Z")
	if code, resp := pulseDeliver(t, hook.URL, hook, resolved, true); code != 200 {
		t.Fatalf("a resolved event must be acknowledged so Pulse does not retry: %d %s", code, resp)
	}
	firing := pulseAlert("alert", "cpu-vm-102", "vm-102", "2026-10-21T08:02:00Z")
	if code, resp := pulseDeliver(t, hook.URL, hook, firing, false); code != 401 {
		t.Fatalf("an unsigned delivery to a source with a signing secret must be 401: %d %s", code, resp)
	}
	if n := attempts(mgr, pulseSource); n != before {
		t.Fatalf("neither a resolved event nor an unsigned delivery may push a signal: %d new posts", n-before)
	}
}

// pulseAssertUnknownSourceRefusalNamesIt: a post for a source this adapter
// does not serve is refused with 404, and the body says which source.
func pulseAssertUnknownSourceRefusalNamesIt(t *testing.T, p *Process) {
	t.Helper()
	code, resp := postJSON(t, p.URL()+"/webhook/not-a-pulse-source", []byte(`{"event":"alert","resource":"vm-1"}`), nil)
	if code != 404 {
		t.Fatalf("an unknown source must be 404, got %d %s", code, resp)
	}
	if !strings.Contains(resp, "not-a-pulse-source") {
		t.Fatalf("the 404 body must name the refused source: %s", resp)
	}
}

// pulseAssertRejectedPostSurfaced: a rejected manager post fails the
// delivery, so Pulse retries it — never a 2xx over a dropped signal. The
// retry, once the manager accepts again, repeats the rejected post's
// initiationId.
func pulseAssertRejectedPostSurfaced(t *testing.T, mgr *FakeManager, p *Process, hook PulseWebhook) {
	t.Helper()
	mgr.RejectSignals(500)
	alert := pulseAlert("alert", "cpu-vm-103", "vm-103", "2026-10-21T08:03:00Z")
	code, resp := pulseDeliver(t, hook.URL, hook, alert, true)
	if code/100 == 2 {
		t.Fatalf("a rejected manager post must fail the delivery, got %d %s", code, resp)
	}
	if p.Exited() {
		t.Fatalf("a rejected post must not kill the adapter")
	}
	rejectedID := pulseLastRejectedID(t, mgr)
	mgr.RejectSignals(0)
	if code, resp := pulseDeliver(t, hook.URL, hook, alert, true); code != 200 {
		t.Fatalf("Pulse's retry of the failed delivery must be accepted: %d %s", code, resp)
	}
	posts := accepted(mgr, pulseSource)
	if got := posts[len(posts)-1].Signals[0]["initiationId"]; got != rejectedID {
		t.Fatalf("a re-send after a rejected post must repeat its initiationId: %q, then %v", rejectedID, got)
	}
}

// pulseLastRejectedID is the initiationId of the most recent rejected post
// for the pulse source.
func pulseLastRejectedID(t *testing.T, mgr *FakeManager) string {
	t.Helper()
	posts := mgr.SignalPosts()
	for i := len(posts) - 1; i >= 0; i-- {
		if posts[i].Source == pulseSource && posts[i].Rejected && len(posts[i].Signals) > 0 {
			id, _ := posts[i].Signals[0]["initiationId"].(string)
			if id == "" {
				t.Fatalf("the rejected post carried no initiationId: %v", posts[i].Signals[0])
			}
			return id
		}
	}
	t.Fatalf("the rejected delivery never reached the manager — a 5xx over no post is not a surfaced rejection")
	return ""
}

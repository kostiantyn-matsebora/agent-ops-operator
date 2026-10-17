package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePulse stands in for Pulse's own notifications-webhooks API: a simple
// in-memory list keyed by name, with counters so a test can assert exactly
// how many create/update calls actually happened — the idempotence this
// adapter claims is a claim about CALL COUNTS, not just end state.
type fakePulse struct {
	mu       sync.Mutex
	webhooks map[string]pulseWebhookConfig
	nextID   int
	creates  int
	updates  int
	fail     bool // when true, every call 500s (simulates Pulse being down)
}

func (f *fakePulse) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/notifications/webhooks", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.fail {
			http.Error(w, "pulse unavailable", 500)
			return
		}
		list := make([]pulseWebhookConfig, 0, len(f.webhooks))
		for _, wh := range f.webhooks {
			list = append(list, wh)
		}
		_ = json.NewEncoder(w).Encode(list)
	})
	mux.HandleFunc("POST /api/notifications/webhooks", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.fail {
			http.Error(w, "pulse unavailable", 500)
			return
		}
		var wh pulseWebhookConfig
		_ = json.NewDecoder(r.Body).Decode(&wh)
		f.nextID++
		wh.ID = strconv.Itoa(f.nextID)
		f.webhooks[wh.Name] = wh
		f.creates++
		_ = json.NewEncoder(w).Encode(wh)
	})
	mux.HandleFunc("PUT /api/notifications/webhooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.fail {
			http.Error(w, "pulse unavailable", 500)
			return
		}
		var wh pulseWebhookConfig
		_ = json.NewDecoder(r.Body).Decode(&wh)
		wh.ID = r.PathValue("id")
		f.webhooks[wh.Name] = wh
		f.updates++
		_ = json.NewEncoder(w).Encode(wh)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestValidHTTPURLRejectsShapes pins validHTTPURL's rejection branches —
// unparseable, a non-http(s) scheme, no host, and embedded userinfo all
// must fail, so a regression that made it a no-op would be caught here
// rather than only by a production misconfiguration.
func TestValidHTTPURLRejectsShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
		want bool // true = accepted
	}{
		{"valid http", "http://pulse.example.org", true},
		{"valid https", "https://pulse.example.org:8443", true},
		{"unparseable", "http://[::1", false},
		{"non-http scheme", "ftp://pulse.example.org", false},
		{"scheme-less", "pulse.example.org", false},
		{"no host", "http://", false},
		{"embedded userinfo", "http://user:pass@pulse.example.org", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validHTTPURL(tc.url)
			if tc.want && err != nil {
				t.Fatalf("expected %q to be accepted, got: %v", tc.url, err)
			}
			if !tc.want && err == nil {
				t.Fatalf("expected %q to be rejected, got no error", tc.url)
			}
		})
	}
}

// TestParseRegisterConfigRejectsInvalidURLs closes parseRegisterConfig's two
// per-field error branches: syntactically valid JSON carrying a
// syntactically invalid baseUrl or webhookUrl must fail, not just
// non-JSON input (TestReconcileRegistrationInvalidConfigReportsOnce covers
// that case already).
func TestParseRegisterConfigRejectsInvalidURLs(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
	}{
		{"bad baseUrl", `{"baseUrl":"not-a-url"}`},
		{"bad webhookUrl", `{"baseUrl":"https://pulse.example.org","webhookUrl":"not-a-url"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseRegisterConfig(json.RawMessage(tc.json))
			if err == nil {
				t.Fatalf("expected an error, got config: %+v", cfg)
			}
		})
	}
}

// TestEnsureWebhookCreateThenUpdateIdempotent is task 1.6's verification:
// the first call creates, a repeat of the same desired state is a no-op
// (left alone), and a changed desired state updates in place rather than
// creating a second webhook.
func TestEnsureWebhookCreateThenUpdateIdempotent(t *testing.T) {
	f := &fakePulse{webhooks: map[string]pulseWebhookConfig{}}
	srv := f.server(t)
	client := newPulseClient(srv.URL, "tok")

	desired := desiredWebhook("pulse-default", "http://agentops-signal-pulse.ns.svc:8080/webhook/pulse-default", "s3cret", nil)
	if err := client.ensureWebhook(context.Background(), desired); err != nil {
		t.Fatalf("first ensureWebhook (create): %v", err)
	}
	f.mu.Lock()
	if f.creates != 1 || f.updates != 0 || len(f.webhooks) != 1 {
		t.Fatalf("expected exactly one create: creates=%d updates=%d webhooks=%d", f.creates, f.updates, len(f.webhooks))
	}
	f.mu.Unlock()

	// repeating the identical desired state must not create or update again
	if err := client.ensureWebhook(context.Background(), desired); err != nil {
		t.Fatalf("second ensureWebhook (idempotent repeat): %v", err)
	}
	f.mu.Lock()
	if f.creates != 1 || f.updates != 0 {
		t.Fatalf("an unchanged repeat must be a no-op: creates=%d updates=%d", f.creates, f.updates)
	}
	f.mu.Unlock()

	// a changed desired state (different URL) must update the existing
	// webhook in place, never create a second one
	changed := desiredWebhook("pulse-default", "http://agentops-signal-pulse.ns.svc:9090/webhook/pulse-default", "s3cret", nil)
	if err := client.ensureWebhook(context.Background(), changed); err != nil {
		t.Fatalf("third ensureWebhook (update): %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.creates != 1 || f.updates != 1 || len(f.webhooks) != 1 {
		t.Fatalf("a drifted repeat must repair in place, not create a second webhook: creates=%d updates=%d webhooks=%d",
			f.creates, f.updates, len(f.webhooks))
	}
	if f.webhooks["agentops-pulse-default"].URL != changed.URL {
		t.Fatalf("the repaired webhook must carry the new URL: %+v", f.webhooks["agentops-pulse-default"])
	}
}

// TestDesiredWebhookCarriesCustomHeaders closes the gap this adapter used
// to have against Pulse's real WebhookConfig.Headers field (confirmed
// against Pulse's own internal/notifications/notifications.go) — a header
// a WAF or gateway in front of this adapter requires, independent of HMAC
// signing, must actually reach the registered object.
func TestDesiredWebhookCarriesCustomHeaders(t *testing.T) {
	f := &fakePulse{webhooks: map[string]pulseWebhookConfig{}}
	srv := f.server(t)
	client := newPulseClient(srv.URL, "tok")

	headers := map[string]string{"X-Gateway-Key": "shared-secret"}
	desired := desiredWebhook("pulse-default", "http://x/webhook/pulse-default", "s3cret", headers)
	if err := client.ensureWebhook(context.Background(), desired); err != nil {
		t.Fatalf("create: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	got := f.webhooks["agentops-pulse-default"].Headers
	if got["X-Gateway-Key"] != "shared-secret" {
		t.Fatalf("expected the custom header to reach the registered webhook, got: %+v", got)
	}
}

// TestEnsureWebhookSigningSecretNeverCompared asserts the secret-blind
// comparison register.go documents: Pulse not echoing the secret back on
// read must never be mistaken for drift that re-triggers an update every
// cycle.
func TestEnsureWebhookSigningSecretNeverCompared(t *testing.T) {
	f := &fakePulse{webhooks: map[string]pulseWebhookConfig{}}
	srv := f.server(t)
	client := newPulseClient(srv.URL, "tok")

	desired := desiredWebhook("pulse-default", "http://x/webhook/pulse-default", "s3cret", nil)
	if err := client.ensureWebhook(context.Background(), desired); err != nil {
		t.Fatalf("create: %v", err)
	}
	// simulate Pulse redacting the secret on read
	f.mu.Lock()
	wh := f.webhooks["agentops-pulse-default"]
	wh.SigningSecret = ""
	f.webhooks["agentops-pulse-default"] = wh
	f.mu.Unlock()

	if err := client.ensureWebhook(context.Background(), desired); err != nil {
		t.Fatalf("repeat after redaction: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updates != 0 {
		t.Fatalf("a redacted secret on read must not be treated as drift: updates=%d", f.updates)
	}
}

// TestReconcileRegistrationInitialFailureThenSuccessHeals is task 1.7's
// verification: an initial failure (Pulse down) degrades the Ready reason
// without crashing, and a later successful retry heals it — both through
// the adapter's own reconcileRegistration, called directly twice rather
// than through registryLoop's ticker, exactly as it would on consecutive
// polls of the served source.
func TestReconcileRegistrationInitialFailureThenSuccessHeals(t *testing.T) {
	mgr := &fakeManager{}
	mgrSrv := mgr.server(t)
	pulse := &fakePulse{webhooks: map[string]pulseWebhookConfig{}, fail: true}
	pulseSrv := pulse.server(t)

	a := &adapter{
		mgr:        NewManager(mgrSrv.URL, "tok"),
		sourceType: "pulse",
		listen:     ":8080",
		sources:    map[string]string{},
		reported:   map[string]string{},
	}
	info := SourceInfo{
		Name:                "pulse-default",
		Config:              json.RawMessage(`{"baseUrl":"` + pulseSrv.URL + `","webhookUrl":"http://agentops-signal-pulse.agent-ops.svc:8080/webhook/pulse-default"}`),
		CredentialEnvPrefix: "AGENTOPS_CRED_PULSE_DEFAULT_",
	}
	t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_token", "tok")

	// Pulse unreachable: registration fails, but must not crash or panic —
	// the loop simply reports degraded and lets the next tick retry.
	a.reconcileRegistration(context.Background(), info)
	mgr.mu.Lock()
	if len(mgr.statuses) != 1 || mgr.statuses[0]["reason"] != "PulseRegistrationFailed" {
		t.Fatalf("expected one degraded status report, got: %+v", mgr.statuses)
	}
	if mgr.statuses[0]["ready"] != true {
		t.Fatalf("the webhook is still served while registration is degraded, so Ready must stay true: %+v", mgr.statuses[0])
	}
	mgr.mu.Unlock()

	// Pulse recovers: the very next reconcile must succeed and heal Ready,
	// with no restart of anything.
	pulse.mu.Lock()
	pulse.fail = false
	pulse.mu.Unlock()
	a.reconcileRegistration(context.Background(), info)

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if len(mgr.statuses) != 2 || mgr.statuses[1]["reason"] != "PulseRegistered" {
		t.Fatalf("expected a healed status report after the retry, got: %+v", mgr.statuses)
	}
	pulse.mu.Lock()
	defer pulse.mu.Unlock()
	if pulse.creates != 1 {
		t.Fatalf("the healed retry must have actually registered the webhook with Pulse: creates=%d", pulse.creates)
	}
}

// TestReconcileRegistrationNotConfiguredReportsOnce closes the branch where
// the source is served but carries no spec.config.baseUrl — served, but
// nothing to register against.
func TestReconcileRegistrationNotConfiguredReportsOnce(t *testing.T) {
	mgr := &fakeManager{}
	mgrSrv := mgr.server(t)
	a := &adapter{
		mgr:      NewManager(mgrSrv.URL, "tok"),
		sources:  map[string]string{},
		reported: map[string]string{},
	}
	a.reconcileRegistration(context.Background(), SourceInfo{Name: "pulse-default"})
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if len(mgr.statuses) != 1 || mgr.statuses[0]["reason"] != "RegistrationNotConfigured" {
		t.Fatalf("expected one RegistrationNotConfigured report, got: %+v", mgr.statuses)
	}
}

// TestReconcileRegistrationInvalidConfigReportsOnce closes
// parseRegisterConfig's error branch via the handler: a served source whose
// spec.config is not valid JSON must degrade rather than crash.
func TestReconcileRegistrationInvalidConfigReportsOnce(t *testing.T) {
	mgr := &fakeManager{}
	mgrSrv := mgr.server(t)
	a := &adapter{
		mgr:      NewManager(mgrSrv.URL, "tok"),
		sources:  map[string]string{},
		reported: map[string]string{},
	}
	a.reconcileRegistration(context.Background(), SourceInfo{Name: "pulse-default", Config: json.RawMessage(`{not-json`)})
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if len(mgr.statuses) != 1 || mgr.statuses[0]["reason"] != "InvalidConfig" {
		t.Fatalf("expected one InvalidConfig report, got: %+v", mgr.statuses)
	}
}

// TestReconcileRegistrationUsesConfiguredWebhookURL is the adapter's
// dumb-transport verification: it registers exactly the spec.config.
// webhookUrl the source carries, verbatim — never deriving one of its own
// (same posture as channels/telegram and signals/ha). Covers both the
// chart-computed in-cluster form and an operator-set external one; the
// adapter cannot tell them apart and must not try to.
func TestReconcileRegistrationUsesConfiguredWebhookURL(t *testing.T) {
	for _, webhookURL := range []string{
		"http://agentops-signal-pulse.agent-ops.svc:8080/webhook/pulse-default",
		"https://pulse-adapter.example.org/webhook/pulse-default",
	} {
		t.Run(webhookURL, func(t *testing.T) {
			mgr := &fakeManager{}
			mgrSrv := mgr.server(t)
			pulse := &fakePulse{webhooks: map[string]pulseWebhookConfig{}}
			pulseSrv := pulse.server(t)

			a := &adapter{
				mgr:        NewManager(mgrSrv.URL, "tok"),
				sourceType: "pulse",
				listen:     ":8080",
				sources:    map[string]string{},
				reported:   map[string]string{},
			}
			info := SourceInfo{
				Name:                "pulse-default",
				Config:              json.RawMessage(`{"baseUrl":"` + pulseSrv.URL + `","webhookUrl":"` + webhookURL + `"}`),
				CredentialEnvPrefix: "AGENTOPS_CRED_PULSE_DEFAULT_",
			}
			t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_token", "tok")

			a.reconcileRegistration(context.Background(), info)

			pulse.mu.Lock()
			defer pulse.mu.Unlock()
			wh, ok := pulse.webhooks["agentops-pulse-default"]
			if !ok {
				t.Fatal("expected the webhook to be registered")
			}
			if wh.URL != webhookURL {
				t.Fatalf("expected %q to be registered verbatim, got %q", webhookURL, wh.URL)
			}
		})
	}
}

// TestReconcileRegistrationNotConfiguredWithoutWebhookURL closes the branch
// where a source carries baseUrl but no webhookUrl — the adapter must
// refuse to guess one, reporting the same not-configured state it already
// does for a missing baseUrl.
func TestReconcileRegistrationNotConfiguredWithoutWebhookURL(t *testing.T) {
	mgr := &fakeManager{}
	mgrSrv := mgr.server(t)
	pulse := &fakePulse{webhooks: map[string]pulseWebhookConfig{}}
	pulseSrv := pulse.server(t)

	a := &adapter{
		mgr:      NewManager(mgrSrv.URL, "tok"),
		sources:  map[string]string{},
		reported: map[string]string{},
	}
	info := SourceInfo{
		Name:                "pulse-default",
		Config:              json.RawMessage(`{"baseUrl":"` + pulseSrv.URL + `"}`),
		CredentialEnvPrefix: "AGENTOPS_CRED_PULSE_DEFAULT_",
	}
	t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_token", "tok")

	a.reconcileRegistration(context.Background(), info)

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if len(mgr.statuses) != 1 || mgr.statuses[0]["reason"] != "RegistrationNotConfigured" {
		t.Fatalf("expected one RegistrationNotConfigured report, got: %+v", mgr.statuses)
	}
	pulse.mu.Lock()
	defer pulse.mu.Unlock()
	if pulse.creates != 0 {
		t.Fatalf("must not register anything without a configured webhookUrl: creates=%d", pulse.creates)
	}
}

// TestSigningSecretFlowsFromCredentialToVerification closes the gap every
// other registration test leaves open: they only ever set the `token` env
// var, and signature_test.go's own end-to-end test injects a secret
// directly into a.sources, bypassing pulseCredential/reconcileRegistration
// entirely. This drives the real path start to finish — the chart's
// `signingSecret` Secret key (chart/charts/pulse/templates/credentials.yaml)
// projected as AGENTOPS_CRED_<prefix>signingSecret, resolved by
// pulseCredential, returned by reconcileRegistration, and written into
// a.sources by refreshSources — then proves it by actually verifying a
// signed webhook through the real handler. A key-name drift anywhere in
// that chain would show up here as a wrongly-accepted or wrongly-rejected
// request, not just a missing map entry.
func TestSigningSecretFlowsFromCredentialToVerification(t *testing.T) {
	mgr := &fakeManager{sources: []SourceInfo{{
		Name:                "pulse-default",
		CredentialEnvPrefix: "AGENTOPS_CRED_PULSE_DEFAULT_",
	}}}
	mgrSrv := mgr.server(t)
	pulse := &fakePulse{webhooks: map[string]pulseWebhookConfig{}}
	pulseSrv := pulse.server(t)
	mgr.sources[0].Config = json.RawMessage(`{"baseUrl":"` + pulseSrv.URL + `","webhookUrl":"https://pulse-adapter.example.org/webhook/pulse-default"}`)

	t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_token", "tok")
	t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_signingSecret", "s3cret")

	a := &adapter{
		mgr:      NewManager(mgrSrv.URL, "tok"),
		sources:  map[string]string{},
		reported: map[string]string{},
	}
	a.refreshSources(context.Background())

	if got, ok := a.served("pulse-default"); !ok || got != "s3cret" {
		t.Fatalf("expected the resolved signingSecret to reach a.sources via refreshSources, got %q (served=%v)", got, ok)
	}

	h := a.handler()
	body := firingBody("sig-flow")
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	// a correctly signed request, using the same secret the real chart Secret
	// key and pulseCredential's lookup actually agree on, must be accepted.
	req := httptest.NewRequest("POST", "/webhook/pulse-default", strings.NewReader(body))
	req.Header.Set("X-Pulse-Timestamp", ts)
	req.Header.Set("X-Pulse-Signature", sign(t, "s3cret", ts, []byte(body)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("correctly signed request: %d %s", rec.Code, rec.Body.String())
	}

	// a request signed with a DIFFERENT secret (what a key-name drift between
	// the chart and this adapter would actually produce) must be rejected.
	req = httptest.NewRequest("POST", "/webhook/pulse-default", strings.NewReader(body))
	req.Header.Set("X-Pulse-Timestamp", ts)
	req.Header.Set("X-Pulse-Signature", sign(t, "wrong-secret", ts, []byte(body)))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("wrongly signed request must be rejected: %d %s", rec.Code, rec.Body.String())
	}
}

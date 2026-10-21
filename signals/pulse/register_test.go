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
	// redactSecret, when true, blanks signingSecret on every GET — Pulse's
	// real read posture — while headers come back verbatim.
	redactSecret bool
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
			if f.redactSecret {
				wh.SigningSecret = ""
			}
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
		{"embedded userinfo", "http://user:pass@example.org", false},
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

// TestNormalizeCorrelationStrategy is task 1.1's verification: the explicit
// opt-in "id" passes through verbatim, and everything else — absent, empty,
// the already-canonical "id+startTime", or any unrecognized string — lands
// on the default.
func TestNormalizeCorrelationStrategy(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"absent", "", correlationStrategyIDStartTime},
		{"empty explicit", "", correlationStrategyIDStartTime},
		{"id", "id", correlationStrategyID},
		{"id+startTime", "id+startTime", correlationStrategyIDStartTime},
		{"unrecognized", "bogus", correlationStrategyIDStartTime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeCorrelationStrategy(tc.raw); got != tc.want {
				t.Fatalf("normalizeCorrelationStrategy(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestParseRegisterConfigCarriesCorrelationStrategy closes the config
// parsing side of task 1.1: the raw spec.config.correlationStrategy value
// reaches registerConfig verbatim (normalization happens one layer up, in
// reconcileRegistration, never here — parseRegisterConfig only decodes).
func TestParseRegisterConfigCarriesCorrelationStrategy(t *testing.T) {
	cfg, err := parseRegisterConfig(json.RawMessage(`{"baseUrl":"https://pulse.example.org","correlationStrategy":"id"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.CorrelationStrategy != "id" {
		t.Fatalf("expected correlationStrategy to decode verbatim, got %q", cfg.CorrelationStrategy)
	}
}

// TestReconcileRegistrationResolvesCorrelationStrategy is task 1.3's
// wiring-level verification: reconcileRegistration resolves and returns the
// normalized correlationStrategy from the source's own config, once per
// call — whatever a served source's registration state (configured,
// unconfigured, or invalid), the strategy resolution does not depend on it.
func TestReconcileRegistrationResolvesCorrelationStrategy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
		want   string
	}{
		{"explicit id", `{"baseUrl":"https://pulse.example.org","webhookUrl":"https://adapter.example.org/webhook/pulse-default","correlationStrategy":"id"}`,
			correlationStrategyID},
		{"explicit id+startTime", `{"baseUrl":"https://pulse.example.org","webhookUrl":"https://adapter.example.org/webhook/pulse-default","correlationStrategy":"id+startTime"}`,
			correlationStrategyIDStartTime},
		{"absent defaults", `{"baseUrl":"https://pulse.example.org","webhookUrl":"https://adapter.example.org/webhook/pulse-default"}`,
			correlationStrategyIDStartTime},
		{"unrecognized defaults", `{"baseUrl":"https://pulse.example.org","webhookUrl":"https://adapter.example.org/webhook/pulse-default","correlationStrategy":"bogus"}`,
			correlationStrategyIDStartTime},
		{"not configured for registration still resolves", `{"correlationStrategy":"id"}`, correlationStrategyID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := &fakeManager{}
			mgrSrv := mgr.server(t)
			pulse := &fakePulse{webhooks: map[string]pulseWebhookConfig{}}
			pulseSrv := pulse.server(t)
			a := &adapter{
				mgr:      NewManager(mgrSrv.URL, "tok"),
				sources:  map[string]sourceState{},
				reported: map[string]string{},
			}
			cfg := strings.Replace(tc.config, "https://pulse.example.org", pulseSrv.URL, 1)
			info := SourceInfo{
				Name:                "pulse-default",
				Config:              json.RawMessage(cfg),
				CredentialEnvPrefix: "AGENTOPS_CRED_PULSE_DEFAULT_",
			}
			t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_token", "tok")

			_, strategy := a.reconcileRegistration(context.Background(), info)
			if strategy != tc.want {
				t.Fatalf("resolved correlationStrategy = %q, want %q", strategy, tc.want)
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
		sources:    map[string]sourceState{},
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
		sources:  map[string]sourceState{},
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
// spec.config is not valid JSON must degrade rather than crash. It also
// closes this branch's own correlationStrategy return (normalizeCorrelationStrategy("")),
// which TestReconcileRegistrationResolvesCorrelationStrategy's table does not
// reach — there is no parseable correlationStrategy to assert on when the
// whole config is unparseable JSON, so a source misconfigured this way still
// resolves to the safe default rather than an empty/invalid strategy string.
func TestReconcileRegistrationInvalidConfigReportsOnce(t *testing.T) {
	mgr := &fakeManager{}
	mgrSrv := mgr.server(t)
	a := &adapter{
		mgr:      NewManager(mgrSrv.URL, "tok"),
		sources:  map[string]sourceState{},
		reported: map[string]string{},
	}
	_, strategy := a.reconcileRegistration(context.Background(), SourceInfo{Name: "pulse-default", Config: json.RawMessage(`{not-json`)})
	if strategy != correlationStrategyIDStartTime {
		t.Fatalf("an unparseable config must still resolve correlationStrategy to the default, got %q", strategy)
	}
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
				sources:    map[string]sourceState{},
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
		sources:  map[string]sourceState{},
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
		sources:  map[string]sourceState{},
		reported: map[string]string{},
	}
	a.refreshSources(context.Background())

	if got, strategy, ok := a.served("pulse-default"); !ok || got != "s3cret" || strategy != correlationStrategyIDStartTime {
		t.Fatalf("expected the resolved signingSecret to reach a.sources via refreshSources, got %q strategy=%q (served=%v)", got, strategy, ok)
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

// TestRotatedSigningSecretRepairsDespiteRedaction is task 1.1's
// verification: against a Pulse that redacts the stored secret on read, an
// unchanged secret is still a no-op, and a rotated one is detected through
// the registered digest header and updated in place, carrying the new
// secret — driven through reconcileRegistration exactly as consecutive
// ensure passes would run it.
func TestRotatedSigningSecretRepairsDespiteRedaction(t *testing.T) {
	mgr := &fakeManager{}
	mgrSrv := mgr.server(t)
	pulse := &fakePulse{webhooks: map[string]pulseWebhookConfig{}, redactSecret: true}
	pulseSrv := pulse.server(t)
	a := &adapter{
		mgr:      NewManager(mgrSrv.URL, "tok"),
		sources:  map[string]sourceState{},
		reported: map[string]string{},
	}
	info := SourceInfo{
		Name:                "pulse-default",
		Config:              json.RawMessage(`{"baseUrl":"` + pulseSrv.URL + `","webhookUrl":"https://pulse-adapter.example.org/webhook/pulse-default"}`),
		CredentialEnvPrefix: "AGENTOPS_CRED_PULSE_DEFAULT_",
	}
	t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_token", "tok")
	t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_signingSecret", "old-secret")

	a.reconcileRegistration(context.Background(), info)
	a.reconcileRegistration(context.Background(), info)
	pulse.mu.Lock()
	if pulse.creates != 1 || pulse.updates != 0 {
		t.Fatalf("an unchanged secret behind redaction must be a no-op: creates=%d updates=%d", pulse.creates, pulse.updates)
	}
	pulse.mu.Unlock()

	t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_signingSecret", "new-secret")
	if secret, _ := a.reconcileRegistration(context.Background(), info); secret != "new-secret" {
		t.Fatalf("the rotated secret must be what inbound verification uses, got %q", secret)
	}

	pulse.mu.Lock()
	defer pulse.mu.Unlock()
	if pulse.creates != 1 || pulse.updates != 1 || len(pulse.webhooks) != 1 {
		t.Fatalf("a rotated secret must repair the webhook in place: creates=%d updates=%d webhooks=%d",
			pulse.creates, pulse.updates, len(pulse.webhooks))
	}
	wh := pulse.webhooks["agentops-pulse-default"]
	if wh.SigningSecret != "new-secret" {
		t.Fatalf("the repaired webhook must carry the new secret, got %q", wh.SigningSecret)
	}
	if got := wh.Headers[secretDigestHeader]; got != secretDigest("new-secret") {
		t.Fatalf("the repaired webhook must carry the new secret's digest, got %q", got)
	}
}

// TestRegisteredHeadersCarryDigestBesideOperatorHeaders pins
// registeredHeaders: the digest is "sha256:<hex>", sits beside the
// operator's own headers without mutating the parsed config map, wins a name
// collision, and is absent when no secret is configured.
func TestRegisteredHeadersCarryDigestBesideOperatorHeaders(t *testing.T) {
	// pinned vector: sha256("s3cret"), computed independently of this module
	const wantDigest = "sha256:1ec1c26b50d5d3c58d9583181af8076655fe00756bf7285940ba3670f99fcba0"
	if got := secretDigest("s3cret"); got != wantDigest {
		t.Fatalf("digest = %q, want %q", got, wantDigest)
	}

	operator := map[string]string{"X-Gateway-Key": "shared", secretDigestHeader: "operator-value"}
	got := registeredHeaders(operator, "s3cret")
	if got["X-Gateway-Key"] != "shared" || got[secretDigestHeader] != secretDigest("s3cret") {
		t.Fatalf("expected operator header kept and digest winning the collision, got %+v", got)
	}
	if operator[secretDigestHeader] != "operator-value" {
		t.Fatalf("the parsed config map must not be mutated: %+v", operator)
	}

	if got := registeredHeaders(map[string]string{"X-Gateway-Key": "shared"}, ""); got[secretDigestHeader] != "" {
		t.Fatalf("no secret must register no digest, got %+v", got)
	}
	if got := registeredHeaders(nil, ""); got != nil {
		t.Fatalf("no headers and no secret must register none, got %+v", got)
	}
}

// TestSigningSecretEnforcedWithoutRegistration is task 1.2's verification,
// the hand-registered-Pulse case: a source carrying a signing secret but no
// usable registration config — absent, incomplete or invalid — still
// enforces the secret (TestReconcileRegistrationInvalidConfigKeepsSecret
// covers non-JSON config, which the fake manager cannot carry). A correctly
// signed post verifies, a wrongly signed
// or unsigned one is refused with 401 and pushes nothing.
func TestSigningSecretEnforcedWithoutRegistration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
	}{
		{"no config", ""},
		{"no webhookUrl", `{"baseUrl":"https://pulse.example.org"}`},
		{"invalid baseUrl", `{"baseUrl":"not-a-url","webhookUrl":"https://pulse-adapter.example.org/webhook/pulse-default"}`},
		{"mistyped", `{"baseUrl":42}`},
	} {
		t.Run(tc.name, func(t *testing.T) { assertSigningSecretEnforced(t, tc.config) })
	}
}

// assertSigningSecretEnforced posts unsigned, wrongly signed and correctly
// signed bodies at a source carrying config, and expects only the last to push.
func assertSigningSecretEnforced(t *testing.T, config string) {
	src := SourceInfo{Name: "pulse-default", CredentialEnvPrefix: "AGENTOPS_CRED_PULSE_DEFAULT_"}
	if config != "" {
		src.Config = json.RawMessage(config)
	}
	t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_signingSecret", "s3cret")
	f := &fakeManager{sources: []SourceInfo{src}}
	h := testAdapter(t, f).handler()
	body := firingBody("hand-registered")
	ts := strconv.FormatInt(time.Now().Unix(), 10)

	signed := func(secret string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/webhook/pulse-default", strings.NewReader(body))
		req.Header.Set("X-Pulse-Timestamp", ts)
		req.Header.Set("X-Pulse-Signature", sign(t, secret, ts, []byte(body)))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := post(t, h, "/webhook/pulse-default", body); rec.Code != 401 {
		t.Fatalf("an unsigned post must be refused while a secret is configured: %d %s", rec.Code, rec.Body.String())
	}
	if rec := signed("wrong-secret"); rec.Code != 401 {
		t.Fatalf("a wrongly signed post must be refused: %d %s", rec.Code, rec.Body.String())
	}
	if rec := signed("s3cret"); rec.Code != 200 {
		t.Fatalf("a post signed by a hand-registered Pulse must verify: %d %s", rec.Code, rec.Body.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.inbounds) != 1 {
		t.Fatalf("exactly the verified post must push: %d pushes", len(f.inbounds))
	}
}

// TestReconcileRegistrationInvalidConfigKeepsSecret closes task 1.2's
// non-JSON branch directly: a source whose spec.config does not parse at all
// still returns its configured signing secret for enforcement.
func TestReconcileRegistrationInvalidConfigKeepsSecret(t *testing.T) {
	mgr := &fakeManager{}
	mgrSrv := mgr.server(t)
	a := &adapter{
		mgr:      NewManager(mgrSrv.URL, "tok"),
		sources:  map[string]sourceState{},
		reported: map[string]string{},
	}
	t.Setenv("AGENTOPS_CRED_PULSE_DEFAULT_signingSecret", "s3cret")
	secret, _ := a.reconcileRegistration(context.Background(), SourceInfo{
		Name:                "pulse-default",
		Config:              json.RawMessage(`{not-json`),
		CredentialEnvPrefix: "AGENTOPS_CRED_PULSE_DEFAULT_",
	})
	if secret != "s3cret" {
		t.Fatalf("an invalid config must not disable signature enforcement, got secret %q", secret)
	}
}

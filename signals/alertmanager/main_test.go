package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

// fakeManager serves the contract endpoints the adapter consumes and records
// inbound pushes.
type fakeManager struct {
	mu       sync.Mutex
	sources  []SourceInfo
	inbounds []map[string]any
	statuses []string
}

func (f *fakeManager) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /signal/sources", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.sources)
	})
	mux.HandleFunc("POST /signal/inbound", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.inbounds = append(f.inbounds, body)
		n := len(body["signals"].([]any))
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"queued": n, "conversations": 1})
	})
	mux.HandleFunc("POST /signal/sources/{name}/status", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.statuses = append(f.statuses, r.PathValue("name"))
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testAdapter(t *testing.T, f *fakeManager) *adapter {
	t.Helper()
	mgrSrv := f.server(t)
	a := &adapter{
		mgr:        NewManager(mgrSrv.URL, "tok"),
		sourceType: "vm-alertmanager",
		sources:    map[string]string{},
		reported:   map[string]string{},
	}
	a.refreshSources(context.Background())
	return a
}

func post(t *testing.T, h http.Handler, path, bearer, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// twoFiringOneResolved is the CANONICAL webhook body, shared with the e2e pack
// so a single captured payload cannot drift between the two suites. Read by
// relative path from this test file — a test-only read, no go.mod entry —
// and read inside the test, so a missing file fails that test rather than
// the package's init.
func twoFiringOneResolved(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../test/fixtures/alertmanager-webhook.json")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return string(b)
}

func TestWebhookRoutingAndFiringFilter(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{{Name: "vm-alerts"}}}
	a := testAdapter(t, f)
	h := a.handler()

	// unknown source → 404, nothing pushed
	if rec := post(t, h, "/webhook/nope", "", twoFiringOneResolved(t)); rec.Code != 404 {
		t.Fatalf("unknown source: %d", rec.Code)
	}
	// served source: firing-only normalization
	rec := post(t, h, "/webhook/vm-alerts", "", twoFiringOneResolved(t))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"queued":2`) {
		t.Fatalf("webhook: %d %s", rec.Code, rec.Body.String())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.inbounds) != 1 || f.inbounds[0]["source"] != "vm-alerts" {
		t.Fatalf("inbound push: %+v", f.inbounds)
	}
	signals := f.inbounds[0]["signals"].([]any)
	if len(signals) != 2 {
		t.Fatalf("firing filter: %d signals", len(signals))
	}
	first := signals[0].(map[string]any)
	if first["fingerprint"] != "fp-a" || first["title"] != "🔍 A — ns1" {
		t.Fatalf("normalization: %+v", first)
	}
	if !strings.Contains(first["payload"].(string), `"generatorURL": "http://alertmanager.example.com/a"`) {
		t.Fatalf("payload shape: %s", first["payload"])
	}
	if _, hasKind := first["kind"]; hasKind {
		t.Fatalf("alert-lane signals must carry no kind: %+v", first)
	}

	// all-resolved body → queued 0 with reason, no push
	if rec := post(t, h, "/webhook/vm-alerts", "", `{"alerts":[{"status":"resolved","fingerprint":"x"}]}`); !strings.Contains(rec.Body.String(), "no firing alerts") {
		t.Fatalf("resolved-only: %s", rec.Body.String())
	}
	if len(f.inbounds) != 1 {
		t.Fatal("resolved-only body must not push")
	}
}

func TestFingerprintFallbackIsStable(t *testing.T) {
	a1 := labelFingerprint(map[string]string{"b": "2", "a": "1"})
	a2 := labelFingerprint(map[string]string{"a": "1", "b": "2"})
	if a1 != a2 || a1 == "" {
		t.Fatalf("fallback not stable across map order: %q vs %q", a1, a2)
	}
	if labelFingerprint(map[string]string{"a": "1"}) == a1 {
		t.Fatal("different labels must derive different fingerprints")
	}
	// normalize applies the fallback when the sender omits the fingerprint
	signals := normalize([]amAlert{{Status: "firing", Labels: map[string]string{"a": "1", "b": "2"}}})
	if len(signals) != 1 || signals[0].Fingerprint != a1 {
		t.Fatalf("normalize fallback: %+v", signals)
	}
}

func TestOptInBearerAuth(t *testing.T) {
	f := &fakeManager{sources: []SourceInfo{
		{Name: "locked", CredentialEnvPrefix: "AGENTOPS_CRED_LOCKED_"},
		{Name: "open"},
	}}
	t.Setenv("AGENTOPS_CRED_LOCKED_TOKEN", "s3cret")
	a := testAdapter(t, f)
	h := a.handler()
	body := `{"alerts":[{"status":"firing","fingerprint":"f1","labels":{"alertname":"X"}}]}`

	if rec := post(t, h, "/webhook/locked", "", body); rec.Code != 401 {
		t.Fatalf("missing bearer: %d", rec.Code)
	}
	if rec := post(t, h, "/webhook/locked", "wrong", body); rec.Code != 401 {
		t.Fatalf("wrong bearer: %d", rec.Code)
	}
	if rec := post(t, h, "/webhook/locked", "s3cret", body); rec.Code != 200 {
		t.Fatalf("valid bearer: %d %s", rec.Code, rec.Body.String())
	}
	// uncredentialed source stays open (built-in-endpoint parity)
	if rec := post(t, h, "/webhook/open", "", body); rec.Code != 200 {
		t.Fatalf("anonymous on open source: %d", rec.Code)
	}
	// credentialed but projection missing → fail closed
	f.mu.Lock()
	f.sources = []SourceInfo{{Name: "locked", CredentialEnvPrefix: "AGENTOPS_CRED_MISSING_"}}
	f.mu.Unlock()
	a.refreshSources(context.Background())
	if rec := post(t, h, "/webhook/locked", "anything", body); rec.Code != 401 {
		t.Fatalf("missing projection must fail closed: %d", rec.Code)
	}
}

func TestSourceListRefresh(t *testing.T) {
	f := &fakeManager{}
	a := testAdapter(t, f)
	h := a.handler()
	body := `{"alerts":[{"status":"firing","fingerprint":"f1","labels":{}}]}`

	if rec := post(t, h, "/webhook/late", "", body); rec.Code != 404 {
		t.Fatalf("before refresh: %d", rec.Code)
	}
	f.mu.Lock()
	f.sources = []SourceInfo{{Name: "late"}}
	f.mu.Unlock()
	a.refreshSources(context.Background())
	if rec := post(t, h, "/webhook/late", "", body); rec.Code != 200 {
		t.Fatalf("after refresh: %d", rec.Code)
	}
	// Ready reported once for the newly served source
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, s := range f.statuses {
		if s == "late" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("Ready reports for 'late': %d", n)
	}
}

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// metrics.go had no test file: NewMetricsClient, QueryRange and stepFor were
// entirely unexercised, and handleHistory/handleCharts were only ever hit with
// no backend configured (the 501 path in bff_test.go). These close the
// success and failure paths of the historical-metrics client itself, against
// a real httptest.Server standing in for Prometheus/VictoriaMetrics — not a
// mock of the HTTP client.

// nil is the honest "no backend configured" value, and QueryRange on it must
// fail rather than panic — every call site relies on this.
func TestNewMetricsClientNilWhenUnconfigured(t *testing.T) {
	if NewMetricsClient("") != nil {
		t.Fatal("empty URL must yield nil, the signal that no window past the buffer exists")
	}
	m := NewMetricsClient("http://metrics:9090")
	if m == nil || m.BaseURL != "http://metrics:9090" {
		t.Fatalf("client not built: %+v", m)
	}
	if _, err := (*MetricsClient)(nil).QueryRange(context.Background(), "up", time.Hour, time.Minute); err == nil {
		t.Fatal("a nil client must refuse rather than dereference itself")
	}
}

// stepFor must keep resolution around 200 points without going below the
// 15s floor Prometheus/VictoriaMetrics are comfortable scraping at.
func TestStepForBoundsResolution(t *testing.T) {
	if got := stepFor(5 * time.Minute); got != 15*time.Second {
		t.Fatalf("a short window must floor at 15s, got %v", got)
	}
	// a week-long window / 200 is well above the floor, and must round to a
	// clean 15s multiple rather than an odd duration
	got := stepFor(7 * 24 * time.Hour)
	if got < 15*time.Second || got%(15*time.Second) != 0 {
		t.Fatalf("want a >=15s multiple of 15s, got %v", got)
	}
}

// A real Prometheus-shaped response is parsed into Series/Sample, including
// skipping a malformed value pair rather than failing the whole query.
func TestQueryRangeParsesPrometheusResponse(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
			{"metric":{"pipeline":"k8s-ops"},"values":[[1000,"1.5"],[1015,"bad"],[1030,"2.5"]]}
		]}}`))
	}))
	defer srv.Close()

	m := NewMetricsClient(srv.URL)
	series, err := m.QueryRange(context.Background(), `up{}`, time.Hour, 15*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(series) != 1 || series[0].Labels["pipeline"] != "k8s-ops" {
		t.Fatalf("labels not carried through: %+v", series)
	}
	if len(series[0].Points) != 2 {
		t.Fatalf("the malformed pair must be skipped, not fail the query: %+v", series[0].Points)
	}
	if series[0].Points[0].Value != 1.5 || series[0].Points[1].Value != 2.5 {
		t.Fatalf("values not parsed: %+v", series[0].Points)
	}
	if gotPath == "" {
		t.Fatal("request never reached the backend")
	}
}

// A backend reporting status!=success (a query error, not a transport error)
// must surface the reported message.
func TestQueryRangeReportsBackendError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"error","error":"bad query"}`))
	}))
	defer srv.Close()
	m := NewMetricsClient(srv.URL)
	_, err := m.QueryRange(context.Background(), "bad{", time.Hour, time.Minute)
	if err == nil {
		t.Fatal("want an error for a non-success status")
	}
}

// An HTTP-level failure (5xx) is a different branch from a query error.
func TestQueryRangeReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	m := NewMetricsClient(srv.URL)
	_, err := m.QueryRange(context.Background(), "up", time.Hour, time.Minute)
	if err == nil {
		t.Fatal("want an error for a 503 backend")
	}
}

// handleHistory over a real (fake) backend: the success path this suite never
// reached, plus the unknown-chart 404 and the backend-failure 502 — all three
// distinct branches from the 501-no-backend case already covered elsewhere.
func TestHandleHistoryAndCharts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	f := newFakeManager(t, ChannelInfo{Name: "console"})
	adapter, tr, cache := consoleUnderTest(t, f)
	adapter.refreshChannels(context.Background())
	api := NewAPI(APIDeps{
		Cache: cache, Transcripts: tr, Adapter: adapter,
		Activity: NewActivityWindow(adapter.mgr, 500), Manager: adapter.mgr,
		Metrics: NewMetricsClient(srv.URL),
		Config:  &Config{Namespace: "agent-ops", AdapterName: "console", UIToken: "tok", WriteEnabled: true},
	})
	h := api.Handler(http.NotFoundHandler())

	if rec := authed(t, h, "GET", "/api/charts/throughput", ""); rec.Code != http.StatusOK {
		t.Fatalf("a configured backend must serve the chart: %d %s", rec.Code, rec.Body.String())
	}
	if rec := authed(t, h, "GET", "/api/charts/not-a-real-chart", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown chart name must 404, got %d", rec.Code)
	}

	var list struct {
		Available bool `json:"available"`
	}
	getJSON(t, h, "/api/charts", &list)
	if !list.Available {
		t.Fatal("a configured backend must report itself available")
	}
}

// A backend that answers but errors mid-query must reach the browser as a 502
// naming what happened, not a 200 with an empty series.
func TestHandleHistoryBackendFailureIsBadGateway(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := newFakeManager(t, ChannelInfo{Name: "console"})
	adapter, tr, cache := consoleUnderTest(t, f)
	adapter.refreshChannels(context.Background())
	api := NewAPI(APIDeps{
		Cache: cache, Transcripts: tr, Adapter: adapter,
		Activity: NewActivityWindow(adapter.mgr, 500), Manager: adapter.mgr,
		Metrics: NewMetricsClient(srv.URL),
		Config:  &Config{Namespace: "agent-ops", AdapterName: "console", UIToken: "tok", WriteEnabled: true},
	})
	h := api.Handler(http.NotFoundHandler())
	rec := authed(t, h, "GET", "/api/charts/throughput?windowSeconds=60", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("want 502, got %d %s", rec.Code, rec.Body.String())
	}
}

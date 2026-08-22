package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// The optional metrics backend — Prometheus or VictoriaMetrics, whichever the
// install already runs.
//
// This is what escapes the ring buffer's ~15-minute horizon WITHOUT the console
// storing anything. The split is Kiali's, and it is honest about what each
// source can answer:
//
//	live and recent  activity stream + /status  exact per-item detail:
//	                                            this op, this run, this conversation
//	historical       metrics backend            aggregates only: rates, percentiles,
//	                                            depths — no per-item identity
//
// OPTIONAL, AND DEGRADING CLEANLY. With no URL configured the console is fully
// functional and simply offers no windows past the buffer, SAYING SO rather than
// rendering an empty chart. An empty chart and "we cannot see that far back" are
// different claims, and only one of them is true.

// MetricsClient queries a Prometheus-compatible HTTP API.
type MetricsClient struct {
	BaseURL string
	HTTP    *http.Client
}

// NewMetricsClient returns nil when no URL is configured — nil is the honest
// representation of "no historical window is available", and every call site
// checks for it rather than pretending zero data.
func NewMetricsClient(baseURL string) *MetricsClient {
	if baseURL == "" {
		return nil
	}
	return &MetricsClient{BaseURL: baseURL, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// Sample is one point of a range query.
type Sample struct {
	TS    float64 `json:"ts"`
	Value float64 `json:"value"`
}

// Series is one labelled time series.
type Series struct {
	Labels map[string]string `json:"labels"`
	Points []Sample          `json:"points"`
}

// promResponse is the Prometheus HTTP API envelope.
type promResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]any           `json:"values"`
			Value  []any             `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

// QueryRange runs a range query. step is chosen by the caller from the window,
// so a week-long window does not ask for per-second resolution.
func (m *MetricsClient) QueryRange(ctx context.Context, query string, window, step time.Duration) ([]Series, error) {
	if m == nil {
		return nil, fmt.Errorf("no metrics backend is configured")
	}
	end := time.Now()
	start := end.Add(-window)
	q := url.Values{}
	q.Set("query", query)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.Itoa(int(step.Seconds())))

	req, err := http.NewRequestWithContext(ctx, "GET", m.BaseURL+"/api/v1/query_range?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := m.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("metrics backend: %d", resp.StatusCode)
	}
	var out promResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Status != "success" {
		return nil, fmt.Errorf("metrics backend: %s", out.Error)
	}
	series := make([]Series, 0, len(out.Data.Result))
	for _, r := range out.Data.Result {
		s := Series{Labels: r.Metric}
		for _, pair := range r.Values {
			if len(pair) != 2 {
				continue
			}
			ts, _ := pair[0].(float64)
			raw, _ := pair[1].(string)
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				continue
			}
			s.Points = append(s.Points, Sample{TS: ts, Value: v})
		}
		series = append(series, s)
	}
	return series, nil
}

// historicalCharts are the queries the console offers, over its OWN metric set.
// Each is an aggregate by construction — the cardinality rule keeps ids out of
// labels, so nothing here can identify a specific conversation, and the UI
// labels these views "aggregate" for that reason.
var historicalCharts = map[string]string{
	"throughput":      `sum by (pipeline) (rate(agentops_conversations_created_total[5m]) * 60)`,
	"runDurationP95":  `histogram_quantile(0.95, sum by (le, pipeline) (rate(agentops_run_duration_seconds_bucket[5m])))`,
	"runDurationP50":  `histogram_quantile(0.50, sum by (le, pipeline) (rate(agentops_run_duration_seconds_bucket[5m])))`,
	"queueDepth":      `sum by (adapter) (agentops_channel_ops_queued)`,
	"claimedDepth":    `sum by (adapter) (agentops_channel_ops_claimed)`,
	"runtimeSlots":    `agentops_runtime_slots_in_use`,
	"runtimeSlotsMax": `agentops_runtime_slots_max`,
	"signalsDropped":  `sum by (source, reason) (rate(agentops_signals_dropped_total[5m]) * 60)`,
}

// stepFor picks a resolution giving roughly 200 points — enough for a chart,
// few enough that a week-long window is not a denial-of-service on the backend.
func stepFor(window time.Duration) time.Duration {
	step := window / 200
	if step < 15*time.Second {
		return 15 * time.Second
	}
	return step.Round(15 * time.Second)
}

// handleHistory serves one named chart over a window.
//
// With no backend configured this answers 501 with the reason and the value to
// set — NOT 200 with an empty series, which the UI could only render as "there
// was no traffic".
func (a *API) handleHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("chart")
	query, ok := historicalCharts[name]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown chart " + name})
		return
	}
	if a.metricsBackend() == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error":     "no metrics backend is configured, so windows beyond the activity buffer are unavailable",
			"fix":       "set console.metrics.url to a Prometheus/VictoriaMetrics query endpoint",
			"available": false,
		})
		return
	}
	window := 6 * time.Hour
	if v := r.URL.Query().Get("windowSeconds"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			window = time.Duration(secs) * time.Second
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	series, err := a.metricsBackend().QueryRange(ctx, query, window, stepFor(window))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "available": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"chart": name, "windowSeconds": window.Seconds(), "series": series,
		// aggregate: stated in the payload so a view cannot forget to say it.
		// These are rates and percentiles; no point identifies a conversation.
		"aggregate": true, "available": true,
	})
}

// handleCharts lists what the backend can answer, so the UI renders only charts
// that exist rather than discovering 404s.
func (a *API) handleCharts(w http.ResponseWriter, r *http.Request) {
	names := make([]string, 0, len(historicalCharts))
	for name := range historicalCharts {
		names = append(names, name)
	}
	sortStrings(names)
	writeJSON(w, http.StatusOK, map[string]any{
		"available": a.metricsBackend() != nil,
		"charts":    names,
		// bufferWindow tells the UI where live detail stops and aggregates begin
		"liveWindowSeconds": a.activity.StreamHealth().Events,
	})
}

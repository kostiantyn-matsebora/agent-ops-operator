package main

import (
	"net/url"
	"testing"
)

// --- 2.3: one test per read tool ---------------------------------------

func TestGetResourcesForwardsToResourcesEndpoint(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	fp.on("/api/resources", 200, map[string]any{"resources": []any{map[string]any{"name": "pve-1"}}})
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-1"}, "get_resources", map[string]any{"type": "node"})
	if isErr {
		t.Fatalf("want success, got %q", text)
	}
	if !contains(text, "pve-1") {
		t.Fatalf("want pulse's own response surfaced, got %q", text)
	}
	got := fp.requests[0]
	if got.method != "GET" || got.path != "/api/resources" {
		t.Fatalf("want GET /api/resources, got %s %s", got.method, got.path)
	}
	if q, _ := url.ParseQuery(got.query); q.Get("type") != "node" {
		t.Fatalf("want type=node forwarded as a query param, got %q", got.query)
	}
}

func TestGetMetricsForwardsToMetricsEndpoint(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	fp.on("/api/metrics", 200, map[string]any{"series": []any{1, 2, 3}})
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-1"}, "get_metrics",
		map[string]any{"resource": "pve-1", "metric": "cpu", "range": "1h"})
	if isErr {
		t.Fatalf("want success, got %q", text)
	}
	got := fp.requests[0]
	if got.method != "GET" || got.path != "/api/metrics" {
		t.Fatalf("want GET /api/metrics, got %s %s", got.method, got.path)
	}
	q, _ := url.ParseQuery(got.query)
	if q.Get("resource") != "pve-1" || q.Get("metric") != "cpu" || q.Get("range") != "1h" {
		t.Fatalf("want all three args forwarded as query params, got %q", got.query)
	}
}

func TestGetMetricsRequiresAResourceArgument(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-1"}, "get_metrics", map[string]any{})
	if !isErr || !contains(text, "need") {
		t.Fatalf("want an argument refusal, got %q (isError=%v)", text, isErr)
	}
	if len(fp.requests) != 0 {
		t.Fatalf("a malformed call must not reach pulse, got %d", len(fp.requests))
	}
}

func TestGetAlertsForwardsToAlertsEndpoint(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	fp.on("/api/alerts", 200, map[string]any{"alerts": []any{map[string]any{"id": "a-1"}}})
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-1"}, "get_alerts", map[string]any{"status": "active"})
	if isErr {
		t.Fatalf("want success, got %q", text)
	}
	if !contains(text, "a-1") {
		t.Fatalf("want pulse's own response surfaced, got %q", text)
	}
	got := fp.requests[0]
	if got.method != "GET" || got.path != "/api/alerts" {
		t.Fatalf("want GET /api/alerts, got %s %s", got.method, got.path)
	}
	if q, _ := url.ParseQuery(got.query); q.Get("status") != "active" {
		t.Fatalf("want status=active forwarded, got %q", got.query)
	}
}

func TestGetStorageHistoryForwardsToStorageHistoryEndpoint(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	fp.on("/api/storage/history", 200, map[string]any{"samples": []any{1, 2}})
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-1"}, "get_storage_history",
		map[string]any{"resource": "pve-1", "range": "24h"})
	if isErr {
		t.Fatalf("want success, got %q", text)
	}
	got := fp.requests[0]
	if got.method != "GET" || got.path != "/api/storage/history" {
		t.Fatalf("want GET /api/storage/history, got %s %s", got.method, got.path)
	}
	q, _ := url.ParseQuery(got.query)
	if q.Get("resource") != "pve-1" || q.Get("range") != "24h" {
		t.Fatalf("want both args forwarded, got %q", got.query)
	}
}

func TestGetStorageHistoryRequiresAResourceArgument(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-1"}, "get_storage_history", map[string]any{})
	if !isErr || !contains(text, "need") {
		t.Fatalf("want an argument refusal, got %q (isError=%v)", text, isErr)
	}
	if len(fp.requests) != 0 {
		t.Fatalf("a malformed call must not reach pulse, got %d", len(fp.requests))
	}
}

// --- 2.4: one test per write tool ---------------------------------------

func TestSetMaintenanceWindowForwardsToOperatorStateEndpoint(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	fp.on("/api/operator-state/maintenance-window", 200, map[string]any{"ok": true})
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-write"}, "set_maintenance_window", map[string]any{
		"resource": "pve-1", "start": "2026-10-16T00:00:00Z", "end": "2026-10-16T02:00:00Z", "reason": "upgrade",
	})
	if isErr {
		t.Fatalf("want success, got %q", text)
	}
	got := fp.requests[0]
	if got.method != "POST" || got.path != "/api/operator-state/maintenance-window" {
		t.Fatalf("want POST /api/operator-state/maintenance-window, got %s %s", got.method, got.path)
	}
	if got.token != "tok-write" {
		t.Fatalf("want the caller's own token forwarded, got %q", got.token)
	}
	if got.body["resource"] != "pve-1" || got.body["start"] != "2026-10-16T00:00:00Z" ||
		got.body["end"] != "2026-10-16T02:00:00Z" || got.body["reason"] != "upgrade" {
		t.Fatalf("want the full body forwarded, got %+v", got.body)
	}
}

func TestSetMaintenanceWindowRequiresResourceStartAndEnd(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-write"}, "set_maintenance_window", map[string]any{"resource": "pve-1"})
	if !isErr || !contains(text, "need") {
		t.Fatalf("want an argument refusal, got %q (isError=%v)", text, isErr)
	}
	if len(fp.requests) != 0 {
		t.Fatalf("a malformed call must not reach pulse, got %d", len(fp.requests))
	}
}

func TestSetCriticalityTagForwardsToOperatorStateEndpoint(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	fp.on("/api/operator-state/criticality-tag", 200, map[string]any{"ok": true})
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-write"}, "set_criticality_tag",
		map[string]any{"resource": "pve-1", "criticality": "high"})
	if isErr {
		t.Fatalf("want success, got %q", text)
	}
	got := fp.requests[0]
	if got.method != "POST" || got.path != "/api/operator-state/criticality-tag" {
		t.Fatalf("want POST /api/operator-state/criticality-tag, got %s %s", got.method, got.path)
	}
	if got.body["resource"] != "pve-1" || got.body["criticality"] != "high" {
		t.Fatalf("want the full body forwarded, got %+v", got.body)
	}
}

func TestSetCriticalityTagRequiresResourceAndCriticality(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	s := newServer(ts.URL)

	text, isErr := toolText(t, s, caller{Token: "tok-write"}, "set_criticality_tag", map[string]any{"resource": "pve-1"})
	if !isErr || !contains(text, "need") {
		t.Fatalf("want an argument refusal, got %q (isError=%v)", text, isErr)
	}
	if len(fp.requests) != 0 {
		t.Fatalf("a malformed call must not reach pulse, got %d", len(fp.requests))
	}
}

// --- 2.5: an under-scoped token is relayed and refused by Pulse itself ---

// TestUnderScopedTokenIsRelayedAndRefusedByPulseNotLocally is task 2.5's
// test — the spec's "Two callers, two scopes, two outcomes" scenario. Pulse,
// not this server, is what tells the two callers apart: mcp-pulse makes the
// identical call shape for both and only differs in which token it forwards.
func TestUnderScopedTokenIsRelayedAndRefusedByPulseNotLocally(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	fp.on("/api/operator-state/criticality-tag", 403, map[string]any{"error": "token lacks monitoring:write scope"})
	s := newServer(ts.URL)

	readOnlyArgs := map[string]any{"resource": "pve-1", "criticality": "high"}
	text, isErr := toolText(t, s, caller{Token: "read-only-tok"}, "set_criticality_tag", readOnlyArgs)
	if !isErr {
		t.Fatalf("want pulse's 403 relayed as a tool error, got success %q", text)
	}
	if !contains(text, "monitoring:write") {
		t.Fatalf("want pulse's own refusal text surfaced verbatim, got %q", text)
	}
	if len(fp.requests) != 1 {
		t.Fatalf("want the call to reach pulse rather than being short-circuited locally, got %d requests", len(fp.requests))
	}
	if fp.requests[0].token != "read-only-tok" {
		t.Fatalf("want the under-scoped token forwarded unchanged, not substituted or dropped, got %q", fp.requests[0].token)
	}

	// A second caller with the write scope, same call shape, succeeds —
	// mcp-pulse made no distinction beyond relaying each token.
	fp.on("/api/operator-state/criticality-tag", 200, map[string]any{"ok": true})
	text2, isErr2 := toolText(t, s, caller{Token: "read-write-tok"}, "set_criticality_tag", readOnlyArgs)
	if isErr2 {
		t.Fatalf("want the write-scoped caller to succeed, got %q", text2)
	}
}

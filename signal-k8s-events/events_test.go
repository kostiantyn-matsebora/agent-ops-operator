package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustFilter(t *testing.T, cfg string) *filter {
	t.Helper()
	f, err := parseConfig(json.RawMessage(cfg))
	if err != nil {
		t.Fatalf("parseConfig(%s): %v", cfg, err)
	}
	return f
}

func evt(severity, ns, kind, name, reason string) Event {
	var e Event
	e.Type, e.Reason = severity, reason
	e.InvolvedObject.Kind, e.InvolvedObject.Name, e.InvolvedObject.Namespace = kind, name, ns
	e.Metadata.Namespace = ns
	return e
}

// Zero configuration has to work — it is what the bundle renders — and it must
// default to Warning: Normal events are the cluster's background hum and would
// bury real problems in conversations.
func TestParseConfigDefaultsToWarningOnly(t *testing.T) {
	for _, raw := range []string{"", "null", "{}"} {
		f := mustFilter(t, raw)
		if !f.severities["Warning"] || f.severities["Normal"] || len(f.severities) != 1 {
			t.Fatalf("config %q: want Warning only, got %v", raw, f.severities)
		}
		if got := f.Scopes(); len(got) != 1 || got[0] != "" {
			t.Fatalf("config %q: absent namespaces must mean cluster-wide, got %v", raw, got)
		}
	}
}

func TestParseConfigRejectsUnknownSeverity(t *testing.T) {
	_, err := parseConfig(json.RawMessage(`{"severities":["Warning","Critical"]}`))
	if err == nil {
		t.Fatal("unknown severity must be rejected — a typo would silently emit nothing")
	}
	if !strings.Contains(err.Error(), "Critical") || !strings.Contains(err.Error(), "severities") {
		t.Fatalf("error must name the offending value and field: %v", err)
	}
}

func TestParseConfigRejectsMalformedJSON(t *testing.T) {
	if _, err := parseConfig(json.RawMessage(`{"severities":`)); err == nil {
		t.Fatal("malformed config must be rejected")
	}
}

func TestFilterSeverityAndNamespace(t *testing.T) {
	f := mustFilter(t, `{"severities":["Warning"],"namespaces":["prod"]}`)
	warnProd := evt("Warning", "prod", "Pod", "api-1", "BackOff")
	if !f.Matches(&warnProd) {
		t.Fatal("in-scope warning must match")
	}
	normalProd := evt("Normal", "prod", "Pod", "api-1", "Pulled")
	if f.Matches(&normalProd) {
		t.Fatal("Normal must not match a Warning-only source")
	}
	warnDev := evt("Warning", "dev", "Pod", "api-1", "BackOff")
	if f.Matches(&warnDev) {
		t.Fatal("out-of-namespace event must not match")
	}
	if got := f.Scopes(); len(got) != 1 || got[0] != "prod" {
		t.Fatalf("scopes: %v", got)
	}
}

func TestFilterIncludeAndExcludeReasons(t *testing.T) {
	inc := mustFilter(t, `{"includeReasons":["BackOff"]}`)
	backoff := evt("Warning", "prod", "Pod", "p", "BackOff")
	failed := evt("Warning", "prod", "Pod", "p", "FailedMount")
	if !inc.Matches(&backoff) || inc.Matches(&failed) {
		t.Fatal("includeReasons must restrict to the listed reasons")
	}

	exc := mustFilter(t, `{"excludeReasons":["BackOff"]}`)
	if exc.Matches(&backoff) || !exc.Matches(&failed) {
		t.Fatal("excludeReasons must drop the listed reasons and keep the rest")
	}

	// exclude wins over include for the same reason
	both := mustFilter(t, `{"includeReasons":["BackOff"],"excludeReasons":["BackOff"]}`)
	if both.Matches(&backoff) {
		t.Fatal("excludeReasons must be applied after includeReasons")
	}
}

// Kubernetes recreates Event objects for a recurring problem. If the
// fingerprint tracked the Event object, every repeat of one crash loop would
// open a new conversation — the exact spam this design avoids.
func TestFingerprintSurvivesEventRecreation(t *testing.T) {
	first := evt("Warning", "prod", "Pod", "api-1", "BackOff")
	first.Metadata.Name, first.Metadata.UID = "api-1.17a", "uid-1"
	first.LastTimestamp, first.Count = "2026-08-08T10:00:00Z", 3

	// same problem, brand new Event object hours later
	second := evt("Warning", "prod", "Pod", "api-1", "BackOff")
	second.Metadata.Name, second.Metadata.UID = "api-1.99z", "uid-2"
	second.LastTimestamp, second.Count = "2026-08-08T18:30:00Z", 1

	a, b := normalize("k8s-events", &first), normalize("k8s-events", &second)
	if a.Fingerprint != b.Fingerprint {
		t.Fatalf("fingerprint must key on object+reason:\n%s\n%s", a.Fingerprint, b.Fingerprint)
	}
	if a.Fingerprint != "k8s-events@prod/Pod/api-1/BackOff" {
		t.Fatalf("fingerprint shape: %s", a.Fingerprint)
	}

	// a different object, or a different reason, is a different problem
	other := evt("Warning", "prod", "Pod", "api-2", "BackOff")
	reason := evt("Warning", "prod", "Pod", "api-1", "FailedMount")
	if normalize("k8s-events", &other).Fingerprint == a.Fingerprint ||
		normalize("k8s-events", &reason).Fingerprint == a.Fingerprint {
		t.Fatal("distinct object or reason must produce a distinct fingerprint")
	}
}

func TestNormalizeLabelsAndPayload(t *testing.T) {
	e := evt("Warning", "prod", "Pod", "api-1", "BackOff")
	e.Message = "Back-off restarting failed container"
	e.Count = 7
	s := normalize("cluster-events", &e)

	want := map[string]string{
		"alertgroup": "k8s-events", "alertname": "BackOff", "namespace": "prod",
		"kind": "Pod", "name": "api-1", "severity": "Warning", "source": "cluster-events",
	}
	for k, v := range want {
		if s.Labels[k] != v {
			t.Fatalf("label %s: got %q want %q", k, s.Labels[k], v)
		}
	}
	if s.Kind != "alert" {
		t.Fatalf("events are the alert lane, got %q", s.Kind)
	}
	if s.Title != "BackOff: Pod/api-1" {
		t.Fatalf("title: %q", s.Title)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(s.Payload), &payload); err != nil {
		t.Fatalf("payload must be JSON the agent can read: %v", err)
	}
	if payload["message"] != "Back-off restarting failed container" || payload["count"] != float64(7) {
		t.Fatalf("payload lost detail: %v", payload)
	}
}

// Cluster-scoped objects carry no involvedObject namespace; the event's own
// namespace is the fallback so the fingerprint never contains an empty segment.
func TestNamespaceFallsBackToEventNamespace(t *testing.T) {
	var e Event
	e.Type, e.Reason = "Warning", "NodeNotReady"
	e.InvolvedObject.Kind, e.InvolvedObject.Name = "Node", "worker-3"
	e.Metadata.Namespace = "default"
	if got := e.Namespace(); got != "default" {
		t.Fatalf("namespace fallback: %q", got)
	}
}

func TestEventWhenPrefersMostSpecificTimestamp(t *testing.T) {
	var e Event
	e.Metadata.CreationTimestamp = "2026-08-08T09:00:00Z"
	e.FirstTimestamp = "2026-08-08T10:00:00Z"
	e.LastTimestamp = "2026-08-08T11:00:00Z"
	if got := e.When().Format("15:04"); got != "11:00" {
		t.Fatalf("lastTimestamp must win: %s", got)
	}

	// newer event sources leave lastTimestamp null and set eventTime (MicroTime)
	e.LastTimestamp = ""
	e.EventTime = "2026-08-08T12:34:56.789012Z"
	if got := e.When().Format("15:04:05"); got != "12:34:56" {
		t.Fatalf("eventTime microsecond form must parse: %s", got)
	}

	var empty Event
	if !empty.When().IsZero() {
		t.Fatal("no timestamps must yield the zero time, never epoch")
	}
}

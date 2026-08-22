package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// An adapter with nowhere to connect must SAY SO. The cluster-events adapter
// can default its data source (the pod's own API server); this one cannot, so
// an empty config is a configuration error rather than a working default.
func TestMissingEndpointIsLoud(t *testing.T) {
	for _, raw := range []string{``, `{}`, `null`, `{"levels":["ERROR"]}`} {
		_, err := parseConfig(json.RawMessage(raw))
		if err == nil {
			t.Fatalf("config %q: expected an error naming the endpoint", raw)
		}
		if !strings.Contains(err.Error(), "endpoint") {
			t.Fatalf("config %q: error must name the missing field, got %v", raw, err)
		}
	}
}

func TestEndpointMustBeAnHTTPURL(t *testing.T) {
	for _, ep := range []string{"ha.example.org", "ftp://ha", "https://"} {
		if _, err := parseConfig(json.RawMessage(`{"endpoint":"` + ep + `"}`)); err == nil {
			t.Fatalf("endpoint %q should be rejected", ep)
		}
	}
}

func TestDefaults(t *testing.T) {
	f, err := parseConfig(json.RawMessage(`{"endpoint":"https://ha.example.org"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !f.levels["ERROR"] || !f.levels["CRITICAL"] {
		t.Fatalf("expected ERROR and CRITICAL by default, got %v", f.levels)
	}
	if f.levels["WARNING"] {
		t.Fatal("WARNING is Home Assistant's background hum and must not be on by default")
	}
	if !f.backfill {
		t.Fatal("backfill defaults on: a restart that skipped what happened while it was down is lossy")
	}
	// An empty rule list must still match, with no dwell.
	if r, ok := f.Rule(&Signal{Labels: map[string]string{"level": "ERROR"}}); !ok || r.dwell != 0 {
		t.Fatalf("empty rules should compile to a catch-all with no dwell, got %+v (ok=%v)", r, ok)
	}
}

func TestLevelValidation(t *testing.T) {
	if _, err := parseConfig(json.RawMessage(`{"endpoint":"http://ha","levels":["Warning","error"]}`)); err != nil {
		t.Fatalf("levels should be case-insensitive: %v", err)
	}
	if _, err := parseConfig(json.RawMessage(`{"endpoint":"http://ha","levels":["LOUD"]}`)); err == nil {
		t.Fatal("expected an unknown level to be rejected")
	}
}

func TestBackfillCanBeDisabled(t *testing.T) {
	f, err := parseConfig(json.RawMessage(`{"endpoint":"http://ha","backfill":false}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.backfill {
		t.Fatal("backfill:false must be honoured")
	}
}

func TestIntegrationOf(t *testing.T) {
	cases := map[string]string{
		"homeassistant.components.zwave_js.climate": "zwave_js",
		"homeassistant.components.hue":              "hue",
		"custom_components.frigate.camera":          "frigate",
		"homeassistant.core":                        "homeassistant.core",
		"":                                          "unknown",
	}
	for logger, want := range cases {
		if got := integrationOf(logger); got != want {
			t.Errorf("integrationOf(%q) = %q, want %q", logger, got, want)
		}
	}
}

// The fingerprint keys on Home Assistant's own dedup identity, never on the
// occurrence: a recurring error must collapse into one conversation.
func TestNormalizeFingerprintIsStableAcrossOccurrences(t *testing.T) {
	rec := logRecord{
		Name:      "homeassistant.components.zwave_js.climate",
		Message:   []string{"Failed to set temperature"},
		Level:     "ERROR",
		Source:    []json.RawMessage{json.RawMessage(`"components/zwave_js/climate.py"`), json.RawMessage(`412`)},
		Timestamp: 1755782400,
		Count:     1,
	}
	first := normalize("ha-logs", &rec)

	rec.Timestamp = 1755786000
	rec.Count = 9
	later := normalize("ha-logs", &rec)

	if first.Fingerprint != later.Fingerprint {
		t.Fatalf("fingerprint changed with the occurrence: %q vs %q", first.Fingerprint, later.Fingerprint)
	}
	want := "ha-logs@homeassistant.components.zwave_js.climate@components/zwave_js/climate.py:412"
	if first.Fingerprint != want {
		t.Fatalf("fingerprint = %q, want %q", first.Fingerprint, want)
	}
	if first.Labels["integration"] != "zwave_js" || first.Labels["level"] != "ERROR" {
		t.Fatalf("labels = %v", first.Labels)
	}
	if first.Kind != "alert" {
		t.Fatalf("kind = %q, want alert", first.Kind)
	}
	if first.MatchText != "Failed to set temperature" {
		t.Fatalf("MatchText = %q", first.MatchText)
	}
	if _, leaked := first.Labels["message"]; leaked {
		t.Fatal("the message must not be a label")
	}
	// MatchText must never reach the wire.
	b, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "Failed to set temperature") &&
		!strings.Contains(string(b), `"payload"`) {
		t.Fatal("unexpected serialization shape")
	}
	var wire map[string]any
	if err := json.Unmarshal(b, &wire); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := wire["MatchText"]; present {
		t.Fatal("MatchText must not be serialized")
	}
}

func TestNormalizeTitleIsBounded(t *testing.T) {
	rec := logRecord{
		Name:    "homeassistant.components.hue",
		Message: []string{strings.Repeat("x", 400) + "\nsecond line"},
		Level:   "ERROR",
	}
	sig := normalize("ha-logs", &rec)
	if len([]rune(sig.Title)) > maxTitleText+40 {
		t.Fatalf("title is unbounded: %d runes", len([]rune(sig.Title)))
	}
	if strings.Contains(sig.Title, "\n") {
		t.Fatal("a title must stay one line")
	}
}

func TestFilterMatchesLevelOnly(t *testing.T) {
	f, err := parseConfig(json.RawMessage(`{"endpoint":"http://ha","levels":["ERROR"]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !f.Matches(&logRecord{Level: "ERROR"}) {
		t.Fatal("ERROR should match")
	}
	if f.Matches(&logRecord{Level: "WARNING"}) {
		t.Fatal("WARNING should not match")
	}
}

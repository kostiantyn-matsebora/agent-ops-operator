package main

import (
	"strings"
	"testing"
	"time"
)

func sigWith(labels map[string]string, text string) Signal {
	return Signal{Labels: labels, MatchText: text}
}

// The whole point of an ordered list: the earlier rule decides, and the later
// one is not consulted even when it also matches.
func TestFirstMatchWins(t *testing.T) {
	rs, err := compileRules([]Rule{
		{Matchers: []string{`integration="zwave_js"`}, Action: "drop"},
		{Matchers: []string{`level="ERROR"`}, For: "5m"},
		{Matchers: nil, For: "3m"},
	}, Route{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	got, ok := rs.Match(matchLabels(ptr(sigWith(map[string]string{
		"integration": "zwave_js", "level": "ERROR",
	}, "anything"))))
	if !ok || !got.drop || got.index != 0 {
		t.Fatalf("expected the drop rule at index 0, got %+v (ok=%v)", got, ok)
	}

	got, ok = rs.Match(matchLabels(ptr(sigWith(map[string]string{
		"integration": "hue", "level": "ERROR",
	}, "anything"))))
	if !ok || got.index != 1 || got.dwell != 5*time.Minute {
		t.Fatalf("expected rules[1] with a 5m dwell, got %+v", got)
	}

	got, ok = rs.Match(matchLabels(ptr(sigWith(map[string]string{
		"integration": "hue", "level": "WARNING",
	}, "anything"))))
	if !ok || got.index != 2 || got.dwell != 3*time.Minute {
		t.Fatalf("expected the catch-all, got %+v", got)
	}
}

// `message` is match-only: rules can read the record's text, and the text never
// becomes a label (which would key grouping on the exact wording).
func TestMessageIsMatchOnly(t *testing.T) {
	rs, err := compileRules([]Rule{
		{Matchers: []string{`message=~".*Config entry .* will retry.*"`}, For: "10m"},
		{Matchers: nil, For: "3m"},
	}, Route{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	sig := sigWith(map[string]string{"integration": "hue", "level": "ERROR"},
		"Config entry 'Bridge' for hue integration not ready yet, will retry")
	got, ok := rs.Match(matchLabels(&sig))
	if !ok || got.index != 0 {
		t.Fatalf("expected the message rule to match, got %+v", got)
	}
	if _, leaked := sig.Labels["message"]; leaked {
		t.Fatal("the message must not become a label")
	}
}

// A catch-all anywhere but last makes every later rule dead. That is the
// ordering mistake people actually make, and it is exactly decidable.
func TestCatchAllMustBeLast(t *testing.T) {
	rs, err := compileRules([]Rule{
		{Matchers: nil, For: "3m"},
		{Matchers: []string{`level="ERROR"`}, For: "0"},
	}, Route{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(rs.warnings) != 1 || !strings.Contains(rs.warnings[0], "catch-all") {
		t.Fatalf("expected a shadowing warning, got %v", rs.warnings)
	}
}

func TestIntegrationRulesTranslateToDrops(t *testing.T) {
	rules := integrationRules([]string{"zwave_js"}, []string{"hue"})
	rs, err := compileRules(append(rules, Rule{Matchers: nil, For: "1m"}), Route{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// Not in the include list -> dropped by the leading rule.
	got, _ := rs.Match(map[string]string{"integration": "mqtt"})
	if !got.drop {
		t.Fatalf("expected an integration outside includeIntegrations to drop, got %+v", got)
	}
	// Explicitly excluded -> dropped by the second.
	got, _ = rs.Match(map[string]string{"integration": "hue"})
	if !got.drop {
		t.Fatalf("expected an excluded integration to drop, got %+v", got)
	}
	got, _ = rs.Match(map[string]string{"integration": "zwave_js"})
	if got.drop {
		t.Fatalf("expected an included integration to survive, got %+v", got)
	}
}

func TestMatcherErrorsNameTheMatcher(t *testing.T) {
	for _, bad := range []string{`level`, `=~"x"`, `level=~"("`, ``} {
		if _, err := compileRules([]Rule{{Matchers: []string{bad}}}, Route{}); err == nil {
			t.Fatalf("expected %q to be rejected", bad)
		}
	}
}

func TestRegexMatchersAreAnchored(t *testing.T) {
	rs, err := compileRules([]Rule{{Matchers: []string{`integration=~"zwave"`}, Action: "drop"}}, Route{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got, _ := rs.Match(map[string]string{"integration": "zwave_js"}); got.drop {
		t.Fatal("an unanchored regex matched a longer value — Alertmanager anchors both ends")
	}
}

func TestUnknownActionRejected(t *testing.T) {
	if _, err := compileRules([]Rule{{Action: "silence"}}, Route{}); err == nil {
		t.Fatal("expected an unknown action to be rejected")
	}
}

func TestHalfEmptyInhibitRuleRejected(t *testing.T) {
	_, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{`integration="hub"`},
	}}})
	if err == nil {
		t.Fatal("an inhibit rule with an empty half would silence everything")
	}
}

func ptr(s Signal) *Signal { return &s }

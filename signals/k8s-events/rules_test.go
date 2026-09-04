package main

import (
	"strings"
	"testing"
	"time"
)

func labelsFor(reason string, extra map[string]string) map[string]string {
	l := map[string]string{"alertname": reason, "reason": reason, "namespace": "prod", "kind": "Pod"}
	for k, v := range extra {
		l[k] = v
	}
	return l
}

func TestMatcherOperators(t *testing.T) {
	cases := []struct {
		matcher string
		labels  map[string]string
		want    bool
	}{
		{`reason="BackOff"`, labelsFor("BackOff", nil), true},
		{`reason="BackOff"`, labelsFor("Failed", nil), false},
		{`reason!="BackOff"`, labelsFor("Failed", nil), true},
		{`reason!="BackOff"`, labelsFor("BackOff", nil), false},
		{`reason=~"BackOff|Failed"`, labelsFor("Failed", nil), true},
		{`reason=~"BackOff|Failed"`, labelsFor("Unhealthy", nil), false},
		{`reason!~"BackOff|Failed"`, labelsFor("Unhealthy", nil), true},
		{`reason!~"BackOff|Failed"`, labelsFor("BackOff", nil), false},
		// a label key with dots and a slash, as real pod labels have
		{`app.kubernetes.io/part-of="infra"`, labelsFor("BackOff", map[string]string{"app.kubernetes.io/part-of": "infra"}), true},
		// an absent label is the empty string, so != on it is true
		{`team!="platform"`, labelsFor("BackOff", nil), true},
	}
	for _, tc := range cases {
		m, err := parseMatcher(tc.matcher)
		if err != nil {
			t.Fatalf("%s: %v", tc.matcher, err)
		}
		if got := m.matches(tc.labels); got != tc.want {
			t.Errorf("%s against %v: got %v want %v", tc.matcher, tc.labels["reason"], got, tc.want)
		}
	}
}

// Anchoring is what keeps `reason=~"Failed"` from also matching FailedMount.
// Alertmanager anchors; matching its syntax but not its semantics would be the
// worst of both.
func TestRegexMatchersAreAnchored(t *testing.T) {
	m, err := parseMatcher(`reason=~"Failed"`)
	if err != nil {
		t.Fatal(err)
	}
	if !m.matches(labelsFor("Failed", nil)) {
		t.Fatal("the exact reason must match")
	}
	if m.matches(labelsFor("FailedMount", nil)) {
		t.Fatal("a longer reason must NOT match an unanchored-looking pattern")
	}
}

func TestMatcherSyntaxErrors(t *testing.T) {
	for _, bad := range []string{
		`reason`,             // no operator
		`reason>"BackOff"`,   // unsupported operator
		`="BackOff"`,         // no label
		`reason=~"[unclosed`, // invalid regex
		``,                   // empty
	} {
		if _, err := parseMatcher(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestRulesAreOrderedFirstMatchWins(t *testing.T) {
	rs, err := compileRules([]Rule{
		{Matchers: []string{`reason="Unhealthy"`}, Action: "drop"},
		{Matchers: []string{`reason=~"Unhealthy|BackOff"`}, For: "10m"},
		{Matchers: nil, For: "3m"},
	}, Route{})
	if err != nil {
		t.Fatal(err)
	}

	r, ok := rs.Match(labelsFor("Unhealthy", nil))
	if !ok || !r.drop {
		t.Fatalf("the FIRST matching rule must win: %+v ok=%v", r, ok)
	}
	r, _ = rs.Match(labelsFor("BackOff", nil))
	if r.drop || r.dwell != 10*time.Minute {
		t.Fatalf("BackOff must take the second rule: drop=%v dwell=%v", r.drop, r.dwell)
	}
	r, _ = rs.Match(labelsFor("SomethingNovel", nil))
	if r.drop || r.dwell != 3*time.Minute {
		t.Fatalf("an unlisted reason must take the catch-all: drop=%v dwell=%v", r.drop, r.dwell)
	}
}

// The ordering mistake people actually make.
func TestCatchAllBeforeOtherRulesWarns(t *testing.T) {
	rs, err := compileRules([]Rule{
		{Matchers: nil, For: "3m"},
		{Matchers: []string{`reason="BackOff"`}, For: "5m"},
	}, Route{})
	if err != nil {
		t.Fatal("a misordered rule set must WARN, not fail: " + err.Error())
	}
	if len(rs.warnings) != 1 || !strings.Contains(rs.warnings[0], "catch-all") {
		t.Fatalf("expected an unreachable-rule warning: %v", rs.warnings)
	}
}

func TestWellOrderedRulesDoNotWarn(t *testing.T) {
	rs, err := compileRules([]Rule{
		{Matchers: []string{`reason="BackOff"`}, For: "5m"},
		{Matchers: nil, For: "3m"},
	}, Route{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.warnings) != 0 {
		t.Fatalf("a correctly ordered set must not warn: %v", rs.warnings)
	}
}

func TestRuleValidationErrors(t *testing.T) {
	cases := []struct {
		name  string
		rules []Rule
		route Route
		want  string
	}{
		{"bad duration", []Rule{{For: "soon"}}, Route{}, "not a duration"},
		{"negative duration", []Rule{{For: "-5m"}}, Route{}, "negative"},
		{"unknown action", []Rule{{Action: "silence"}}, Route{}, "not a known action"},
		{"bad matcher", []Rule{{Matchers: []string{"reason"}}}, Route{}, "no supported operator"},
		{"half an inhibit rule", nil,
			Route{InhibitRules: []InhibitRule{{SourceMatchers: []string{`reason="NodeNotReady"`}}}},
			"both sourceMatchers and targetMatchers"},
	}
	for _, tc := range cases {
		_, err := compileRules(tc.rules, tc.route)
		if err == nil {
			t.Errorf("%s: expected an error", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error should name the problem (%q), got %v", tc.name, tc.want, err)
		}
	}
}

// The error must name WHICH rule, or a long rule set is unfixable from a
// condition message.
func TestValidationErrorNamesTheRuleIndex(t *testing.T) {
	_, err := compileRules([]Rule{
		{Matchers: []string{`reason="BackOff"`}},
		{Matchers: []string{`reason="Failed"`}},
		{Matchers: []string{`nonsense`}},
	}, Route{})
	if err == nil || !strings.Contains(err.Error(), "rules[2]") {
		t.Fatalf("error must name the offending rule index: %v", err)
	}
}

// `reason` is a match-only alias for `alertname`: rules read the way an
// operator thinks, while the emitted label vocabulary is unchanged.
func TestReasonIsAMatchOnlyAlias(t *testing.T) {
	e := evt("Warning", "prod", "Pod", "api-1", "BackOff")
	sig := normalize("src", &e, enrichment{})
	if _, emitted := sig.Labels["reason"]; emitted {
		t.Fatal("`reason` must not be emitted as a signal label")
	}
	if got := matchLabels(&sig)["reason"]; got != "BackOff" {
		t.Fatalf("`reason` must be available to matchers: %q", got)
	}
}

func TestEscalateAfterObjectsDefaults(t *testing.T) {
	rs, err := compileRules([]Rule{
		{Matchers: nil, For: "10m"},
	}, Route{})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := rs.Match(labelsFor("Unhealthy", nil))
	if r.escalate != defaultEscalateAfterObjects {
		t.Fatalf("escalation must default: got %d want %d", r.escalate, defaultEscalateAfterObjects)
	}
}

// A malformed matcher in either half of an inhibit rule must be reported by
// NAME, distinguishing sourceMatchers from targetMatchers — parseMatchers'
// error is wrapped separately for each in compileRules and neither wrap was
// exercised before (the existing "half an inhibit rule" case only reaches the
// later "both required" check with two matchers that already parse fine).
func TestInhibitRuleMatcherSyntaxErrorsAreLocated(t *testing.T) {
	if _, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{"not a matcher"},
		TargetMatchers: []string{`reason="X"`},
	}}}); err == nil || !strings.Contains(err.Error(), "sourceMatchers") {
		t.Fatalf("a bad sourceMatchers entry must be named as such, got %v", err)
	}
	if _, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{`reason="X"`},
		TargetMatchers: []string{"not a matcher"},
	}}}); err == nil || !strings.Contains(err.Error(), "targetMatchers") {
		t.Fatalf("a bad targetMatchers entry must be named as such, got %v", err)
	}
}

// Alertmanager accepts an unquoted, bare value — unquote's fallthrough path,
// never exercised because every other matcher test quotes its values.
func TestBareUnquotedMatcherValueIsAccepted(t *testing.T) {
	m, err := parseMatcher(`reason=BackOff`)
	if err != nil {
		t.Fatalf("a bare value must be accepted: %v", err)
	}
	if !m.matches(labelsFor("BackOff", nil)) {
		t.Fatal("a bare-value matcher must match like a quoted one")
	}
}

func TestInhibitRulesCompile(t *testing.T) {
	rs, err := compileRules(nil, Route{InhibitRules: []InhibitRule{{
		SourceMatchers: []string{`reason="NodeNotReady"`},
		TargetMatchers: []string{`reason=~"Unhealthy|FailedScheduling"`},
		Equal:          []string{"node"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs.inhibits) != 1 || len(rs.inhibits[0].equal) != 1 {
		t.Fatalf("inhibit rule did not compile: %+v", rs.inhibits)
	}
}

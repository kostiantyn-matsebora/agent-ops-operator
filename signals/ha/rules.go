package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// The rules engine: Alertmanager-syntax matchers over a signal's labels,
// evaluated in order, first match wins.
//
// Borrowed field-for-field from the cluster-events adapter, deliberately. That
// vocabulary was worked out for this exact problem — what counts as a problem,
// how long it must hold, and which consequences to suppress — and a second
// spelling would be a second thing to learn and a second place to fix.
//
// The config is deliberately shaped as Prometheus + Alertmanager rather than
// Alertmanager alone. `rules` is the PROMETHEUS half — what counts as a
// problem and for how long it must hold (`for`). `route` is the ALERTMANAGER
// half — inhibition. Dwell is NOT spelled `group_wait`: Alertmanager's
// group_wait batches a group before its first notification and is not `for:`,
// which does not exist in Alertmanager at all. Spelling one as the other would
// be an Alertmanager term meaning something Alertmanager does not mean.

// Rule is one entry of `config.rules` as written by the user.
type Rule struct {
	// Matchers select records; empty = catch-all.
	Matchers []string `json:"matchers,omitempty"`
	// For is the dwell: hold the record this long, then re-check the condition
	// before emitting. "0"/absent emits immediately, which is what a condition
	// describing something already COMPLETED requires — a dwell would re-check
	// and find the recovered state, erasing the incident.
	For string `json:"for,omitempty"`
	// Action "drop" suppresses outright. Empty means dwell-and-verify.
	Action string `json:"action,omitempty"`
	// EscalateAfterObjects emits early once this many distinct source locations
	// of one integration are pending. 0 = use the default.
	EscalateAfterObjects int `json:"escalateAfterObjects,omitempty"`
}

// InhibitRule suppresses consequences of a cause that is already reported.
type InhibitRule struct {
	SourceMatchers []string `json:"sourceMatchers,omitempty"`
	TargetMatchers []string `json:"targetMatchers,omitempty"`
	// Equal names labels that must match on both for the inhibition to apply.
	Equal []string `json:"equal,omitempty"`
}

// Route is the Alertmanager half of the config.
//
// Inhibition only. The cluster-events adapter also carries the TIME axis
// (timeIntervals / muteTimeIntervals); this one does not, and an unknown key is
// an error rather than a window that silently never fires.
type Route struct {
	InhibitRules []InhibitRule `json:"inhibitRules,omitempty"`
}

// defaultEscalateAfterObjects: one failing code path in an integration is
// churn, several at once is an outage — and the premise a long dwell rests on
// stops holding.
const defaultEscalateAfterObjects = 3

// ---- matchers ---------------------------------------------------------------

type matchOp int

const (
	opEqual matchOp = iota
	opNotEqual
	opRegex
	opNotRegex
)

type matcher struct {
	label string
	op    matchOp
	value string
	re    *regexp.Regexp
}

// operators are tried longest-first so "!=" is not read as "=" and "=~" is not
// read as "=".
var operators = []struct {
	tok string
	op  matchOp
}{
	{"=~", opRegex},
	{"!~", opNotRegex},
	{"!=", opNotEqual},
	{"=", opEqual},
}

// parseMatcher accepts Alertmanager matcher syntax over a flat label map.
// Only the four documented operators are supported; anything else is an error
// naming the matcher, reported on the source's Ready condition.
func parseMatcher(s string) (matcher, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return matcher{}, fmt.Errorf("empty matcher")
	}
	for _, cand := range operators {
		idx := strings.Index(trimmed, cand.tok)
		if idx <= 0 {
			continue
		}
		// "!=" and "!~" both start with '!'; make sure "=" does not win on a
		// string that actually carries one of them at the same position.
		if cand.tok == "=" && idx > 0 && (trimmed[idx-1] == '!' || trimmed[idx-1] == '=') {
			continue
		}
		label := strings.TrimSpace(trimmed[:idx])
		raw := strings.TrimSpace(trimmed[idx+len(cand.tok):])
		if label == "" {
			return matcher{}, fmt.Errorf("matcher %q has no label name", s)
		}
		value, err := unquote(raw)
		if err != nil {
			return matcher{}, fmt.Errorf("matcher %q: %w", s, err)
		}
		m := matcher{label: label, op: cand.op, value: value}
		if cand.op == opRegex || cand.op == opNotRegex {
			// Anchored like Alertmanager: a regex matcher matches the WHOLE
			// value, so `reason=~"Failed"` does not also match "FailedMount".
			re, err := regexp.Compile("^(?:" + value + ")$")
			if err != nil {
				return matcher{}, fmt.Errorf("matcher %q: invalid regular expression: %w", s, err)
			}
			m.re = re
		}
		return m, nil
	}
	return matcher{}, fmt.Errorf("matcher %q uses no supported operator (expected =, !=, =~ or !~)", s)
}

func unquote(raw string) (string, error) {
	if len(raw) >= 2 && (raw[0] == '"' && raw[len(raw)-1] == '"' || raw[0] == '\'' && raw[len(raw)-1] == '\'') {
		return raw[1 : len(raw)-1], nil
	}
	if strings.ContainsAny(raw, `"'`) {
		return "", fmt.Errorf("value %s is not properly quoted", raw)
	}
	return raw, nil // bare values are accepted, as Alertmanager does
}

func (m matcher) matches(labels map[string]string) bool {
	got := labels[m.label]
	switch m.op {
	case opEqual:
		return got == m.value
	case opNotEqual:
		return got != m.value
	case opRegex:
		return m.re.MatchString(got)
	case opNotRegex:
		return !m.re.MatchString(got)
	}
	return false
}

func parseMatchers(raw []string) ([]matcher, error) {
	out := make([]matcher, 0, len(raw))
	for _, s := range raw {
		m, err := parseMatcher(s)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func allMatch(ms []matcher, labels map[string]string) bool {
	for _, m := range ms {
		if !m.matches(labels) {
			return false
		}
	}
	return true
}

// ---- compiled rules ---------------------------------------------------------

type compiledRule struct {
	matchers []matcher
	dwell    time.Duration
	drop     bool
	escalate int
	// index is the rule's position in the configured list, for diagnostics.
	index int
}

type compiledInhibit struct {
	source []matcher
	target []matcher
	equal  []string
}

// ruleSet is a validated, ordered policy.
type ruleSet struct {
	rules    []compiledRule
	inhibits []compiledInhibit
	// warnings are non-fatal problems (an unreachable rule) reported on the
	// source's Ready condition without failing the source.
	warnings []string
}

func parseDuration(s string) (time.Duration, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a duration (expected forms like 30s, 5m, 1h)", s)
	}
	if d < 0 {
		return 0, fmt.Errorf("%q is negative", s)
	}
	return d, nil
}

// compileRules validates the ordered rule list.
//
// An EMPTY rule list compiles to a single catch-all with NO dwell, so a source
// configured with nothing but an endpoint emits what it sees. Dwell arrives
// only when the chart (or the user) ships rules that ask for it.
func compileRules(rules []Rule, route Route) (*ruleSet, error) {
	rs := &ruleSet{}
	for i, r := range rules {
		ms, err := parseMatchers(r.Matchers)
		if err != nil {
			return nil, fmt.Errorf("rules[%d]: %w", i, err)
		}
		d, err := parseDuration(r.For)
		if err != nil {
			return nil, fmt.Errorf("rules[%d].for: %w", i, err)
		}
		action := strings.ToLower(strings.TrimSpace(r.Action))
		if action != "" && action != "drop" {
			return nil, fmt.Errorf("rules[%d].action: %q is not a known action (expected \"drop\" or absent)", i, r.Action)
		}
		esc := r.EscalateAfterObjects
		if esc <= 0 {
			esc = defaultEscalateAfterObjects
		}
		rs.rules = append(rs.rules, compiledRule{
			matchers: ms, dwell: d, drop: action == "drop", escalate: esc, index: i,
		})
	}
	if len(rs.rules) == 0 {
		rs.rules = append(rs.rules, compiledRule{index: 0})
	}

	for i, ir := range route.InhibitRules {
		src, err := parseMatchers(ir.SourceMatchers)
		if err != nil {
			return nil, fmt.Errorf("route.inhibitRules[%d].sourceMatchers: %w", i, err)
		}
		tgt, err := parseMatchers(ir.TargetMatchers)
		if err != nil {
			return nil, fmt.Errorf("route.inhibitRules[%d].targetMatchers: %w", i, err)
		}
		if len(src) == 0 || len(tgt) == 0 {
			return nil, fmt.Errorf("route.inhibitRules[%d]: both sourceMatchers and targetMatchers are required "+
				"(an inhibit rule with an empty half would silence everything)", i)
		}
		rs.inhibits = append(rs.inhibits, compiledInhibit{source: src, target: tgt, equal: ir.Equal})
	}

	rs.warnings = shadowedRules(rs.rules)
	return rs, nil
}

// shadowedRules reports rules that can never be reached.
//
// Only the decidable case is reported: anything after a catch-all. Deciding
// general subsumption between regex matcher sets is not worth attempting, and a
// wrong warning about a rule that DOES fire would be worse than none — but
// "rules after the catch-all" is the ordering mistake people actually make, and
// it is exact.
func shadowedRules(rules []compiledRule) []string {
	for i, r := range rules {
		if len(r.matchers) == 0 && i < len(rules)-1 {
			return []string{fmt.Sprintf(
				"rules[%d] is a catch-all, so rules[%d..%d] can never match — move the catch-all last",
				i, i+1, len(rules)-1)}
		}
	}
	return nil
}

// Match returns the first rule matching these labels. A rule set always ends in
// something that matches, because compileRules guarantees a catch-all exists
// only when the user wrote one — so a miss is possible and means "no rule
// applies", which callers treat as emit-immediately.
func (rs *ruleSet) Match(labels map[string]string) (compiledRule, bool) {
	for _, r := range rs.rules {
		if allMatch(r.matchers, labels) {
			return r, true
		}
	}
	return compiledRule{}, false
}

// matchLabels builds the label map rules are evaluated against: the signal's
// own labels plus `message` — the log record's text — as a MATCH-ONLY alias.
//
// The alias is never emitted. A message is free text with unbounded
// cardinality, so a label carrying it would key a fingerprint on the exact
// wording and open a fresh conversation for every variation. But rules must be
// able to read it: Home Assistant records carry no `reason` field, so level and
// logger alone cannot tell "failed to set up, will retry" apart from "invalid
// credentials", and those two want opposite dwells.
func matchLabels(sig *Signal) map[string]string {
	out := make(map[string]string, len(sig.Labels)+1)
	for k, v := range sig.Labels {
		out[k] = v
	}
	if _, taken := out["message"]; !taken {
		out["message"] = sig.MatchText
	}
	return out
}

// integrationRules translates includeIntegrations/excludeIntegrations into
// equivalent leading drop rules, so selection by integration has exactly ONE
// implementation and the simple form cannot drift from the rules engine.
// Include is applied before exclude.
func integrationRules(include, exclude []string) []Rule {
	var out []Rule
	if len(include) > 0 {
		out = append(out, Rule{
			Matchers: []string{`integration!~"` + alternation(include) + `"`},
			Action:   "drop",
		})
	}
	if len(exclude) > 0 {
		out = append(out, Rule{
			Matchers: []string{`integration=~"` + alternation(exclude) + `"`},
			Action:   "drop",
		})
	}
	return out
}

func alternation(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			quoted = append(quoted, regexp.QuoteMeta(v))
		}
	}
	return strings.Join(quoted, "|")
}

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// sourceConfig is this adapter's interpretation of SignalSource spec.config.
// Opaque to the operator: the manager never parses it, and validation errors
// come back to the user as this source's Ready condition.
type sourceConfig struct {
	// Severities are Event `type` values to emit. Default ["Warning"] — Normal
	// events are the cluster's background hum and would bury real problems.
	Severities []string `json:"severities,omitempty"`
	// Namespaces to watch; empty or absent = every namespace the granted RBAC
	// allows.
	Namespaces []string `json:"namespaces,omitempty"`
	// IncludeReasons, when non-empty, restricts to these Event reasons.
	IncludeReasons []string `json:"includeReasons,omitempty"`
	// ExcludeReasons drops these reasons; applied after IncludeReasons.
	ExcludeReasons []string `json:"excludeReasons,omitempty"`
	// Rules are the Prometheus half of the policy: ordered, first-match-wins
	// selection with a dwell (`for`) and an optional `action: drop`. Empty
	// means one catch-all with no dwell — today's behavior exactly.
	Rules []Rule `json:"rules,omitempty"`
	// Route is the Alertmanager half: inhibition.
	Route Route `json:"route,omitempty"`
	// IncludeOwnNamespace disables the THIRD self-exclusion mechanism only —
	// events in the operator's own namespace stop being dropped wholesale. Set
	// it when your own workloads are co-located with agent-ops. The name-prefix
	// and owner/label mechanisms still apply and are not configurable: they are
	// what keeps a failing runtime pod from opening a conversation about
	// itself, and an editable loop breaker is not a loop breaker.
	IncludeOwnNamespace bool `json:"includeOwnNamespace,omitempty"`
}

// validSeverities are the only two values core/v1 Event.type takes.
var validSeverities = map[string]bool{"Normal": true, "Warning": true}

// filter is a validated config ready to match events against.
type filter struct {
	severities map[string]bool
	namespaces map[string]bool // empty = all
	include    map[string]bool // empty = all
	exclude    map[string]bool
	// includeOwnNamespace relaxes self-exclusion mechanism 3 only.
	includeOwnNamespace bool
	// rules is the compiled suppression policy. Legacy include/exclude reasons
	// are translated into leading drop rules, so this is the single path.
	rules *ruleSet
}

// parseConfig validates a source's raw config into a filter. An absent or empty
// config is valid and yields the defaults — the zero-configuration case has to
// work, since that is what the bundle renders.
func parseConfig(raw json.RawMessage) (*filter, error) {
	cfg := sourceConfig{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("spec.config is not valid JSON for the k8s-events adapter: %w", err)
		}
	}
	f := &filter{
		severities:          map[string]bool{},
		namespaces:          set(cfg.Namespaces),
		include:             set(cfg.IncludeReasons),
		exclude:             set(cfg.ExcludeReasons),
		includeOwnNamespace: cfg.IncludeOwnNamespace,
	}
	// Legacy reason filters become leading drop rules so there is ONE
	// evaluation path; sources written against the previous config are
	// unaffected because an otherwise-empty rule list compiles to a catch-all
	// with no dwell.
	rules := append(legacyRules(cfg.IncludeReasons, cfg.ExcludeReasons), cfg.Rules...)
	compiled, err := compileRules(rules, cfg.Route)
	if err != nil {
		return nil, fmt.Errorf("spec.config: %w", err)
	}
	f.rules = compiled

	if len(cfg.Severities) == 0 {
		f.severities["Warning"] = true
		return f, nil
	}
	for _, s := range cfg.Severities {
		if !validSeverities[s] {
			return nil, fmt.Errorf("spec.config.severities: %q is not a Kubernetes Event type (expected %s)",
				s, strings.Join(sortedKeys(validSeverities), " or "))
		}
		f.severities[s] = true
	}
	return f, nil
}

// Matches reports whether an event is in this source's SCOPE: the right
// severity, in a watched namespace.
//
// Reason filtering deliberately does NOT live here any more. Legacy
// includeReasons/excludeReasons are translated into leading drop rules by
// parseConfig, so selection by reason has exactly one implementation and
// legacy and modern config cannot drift apart.
func (f *filter) Matches(ev *Event) bool {
	if !f.severities[ev.Type] {
		return false
	}
	if len(f.namespaces) > 0 && !f.namespaces[ev.Namespace()] {
		return false
	}
	return true
}

// Rule returns the policy for a normalized signal: the first matching rule, or
// false when no rule applies (which callers treat as emit-immediately).
func (f *filter) Rule(sig *Signal) (compiledRule, bool) {
	if f.rules == nil {
		return compiledRule{}, false
	}
	return f.rules.Match(matchLabels(sig))
}

// Scopes returns the namespaces this source needs watched: the configured list,
// or [""] meaning cluster-wide.
func (f *filter) Scopes() []string {
	if len(f.namespaces) == 0 {
		return []string{""}
	}
	return sortedKeys(f.namespaces)
}

// normalize turns one Event into a contract signal for a source.
//
// The fingerprint deliberately excludes the event's own name/UID and its
// timestamps: Kubernetes recreates Event objects for a recurring problem, and a
// fingerprint that changed with them would open a fresh conversation on every
// repeat of the same crash loop. Keyed on the OBJECT and REASON instead, a
// flapping pod collapses into one conversation under the manager's cooldown.
func normalize(source string, ev *Event, enr enrichment) Signal {
	ns := ev.Namespace()
	kind, name := ev.InvolvedObject.Kind, ev.InvolvedObject.Name

	payload, _ := json.MarshalIndent(map[string]any{
		"namespace": ns,
		"kind":      kind,
		"name":      name,
		"reason":    ev.Reason,
		"severity":  ev.Type,
		"message":   ev.Message,
		"count":     ev.Count,
		"firstSeen": ev.FirstTimestamp,
		"lastSeen":  ev.LastTimestamp,
	}, "", "  ")

	labels := map[string]string{
		"alertgroup": "k8s-events",
		"alertname":  ev.Reason,
		"namespace":  ns,
		"kind":       kind,
		"name":       name,
		"severity":   ev.Type,
		"source":     source,
	}
	enr.applyTo(labels)

	// The title names the WORKLOAD when one is known: a conversation grouped by
	// workload titled after one of its pods reads as being about that pod.
	subject := fmt.Sprintf("%s/%s", kind, name)
	if enr.Workload != "" {
		subject = enr.Workload
	}

	return Signal{
		Fingerprint: fmt.Sprintf("%s@%s/%s/%s/%s", source, ns, kind, name, ev.Reason),
		Labels:      labels,
		Title:       fmt.Sprintf("%s: %s", ev.Reason, subject),
		Payload:     string(payload),
		Kind:        "alert",
	}
}

func set(vals []string) map[string]bool {
	if len(vals) == 0 {
		return nil
	}
	out := make(map[string]bool, len(vals))
	for _, v := range vals {
		if v = strings.TrimSpace(v); v != "" {
			out[v] = true
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

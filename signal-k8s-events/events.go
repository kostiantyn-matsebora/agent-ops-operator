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
}

// validSeverities are the only two values core/v1 Event.type takes.
var validSeverities = map[string]bool{"Normal": true, "Warning": true}

// filter is a validated config ready to match events against.
type filter struct {
	severities map[string]bool
	namespaces map[string]bool // empty = all
	include    map[string]bool // empty = all
	exclude    map[string]bool
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
		severities: map[string]bool{},
		namespaces: set(cfg.Namespaces),
		include:    set(cfg.IncludeReasons),
		exclude:    set(cfg.ExcludeReasons),
	}
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

// Matches reports whether an event should produce a signal for this source.
func (f *filter) Matches(ev *Event) bool {
	if !f.severities[ev.Type] {
		return false
	}
	if len(f.namespaces) > 0 && !f.namespaces[ev.Namespace()] {
		return false
	}
	if len(f.include) > 0 && !f.include[ev.Reason] {
		return false
	}
	return !f.exclude[ev.Reason]
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
func normalize(source string, ev *Event) Signal {
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

	return Signal{
		Fingerprint: fmt.Sprintf("%s@%s/%s/%s/%s", source, ns, kind, name, ev.Reason),
		Labels: map[string]string{
			"alertgroup": "k8s-events",
			"alertname":  ev.Reason,
			"namespace":  ns,
			"kind":       kind,
			"name":       name,
			"severity":   ev.Type,
			"source":     source,
		},
		Title:   fmt.Sprintf("%s: %s/%s", ev.Reason, kind, name),
		Payload: string(payload),
		Kind:    "alert",
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

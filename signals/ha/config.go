package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// sourceConfig is this adapter's interpretation of SignalSource spec.config.
// Opaque to the operator: the manager never parses it, and validation errors
// come back to the user as this source's Ready condition.
type sourceConfig struct {
	// Endpoint is the Home Assistant base URL, e.g. https://ha.example.org.
	// REQUIRED — an adapter with nowhere to connect must say so rather than run
	// and report nothing.
	Endpoint string `json:"endpoint,omitempty"`
	// Levels are the log levels to act on. Default ["ERROR", "CRITICAL"] —
	// WARNING is Home Assistant's background hum (deprecation notices, slow
	// updates) and would bury real problems.
	Levels []string `json:"levels,omitempty"`
	// IncludeIntegrations, when non-empty, restricts to these integrations.
	IncludeIntegrations []string `json:"includeIntegrations,omitempty"`
	// ExcludeIntegrations drops these; applied after IncludeIntegrations.
	ExcludeIntegrations []string `json:"excludeIntegrations,omitempty"`
	// Rules are the Prometheus half of the policy: ordered, first-match-wins
	// selection with a dwell (`for`) and an optional `action: drop`. Empty
	// means one catch-all with no dwell.
	Rules []Rule `json:"rules,omitempty"`
	// Route is the Alertmanager half: inhibition.
	Route Route `json:"route,omitempty"`
	// Backfill reads the existing system_log listing on connect and reports
	// what is newer than the cursor. Default true: a restart that skipped the
	// errors logged while it was down would make the lane quietly lossy.
	Backfill *bool `json:"backfill,omitempty"`
}

// validLevels are the Python logging levels Home Assistant emits.
var validLevels = map[string]bool{
	"DEBUG": true, "INFO": true, "WARNING": true, "ERROR": true, "CRITICAL": true,
}

// filter is a validated config ready to match records against.
type filter struct {
	endpoint string
	levels   map[string]bool
	backfill bool
	// rules is the compiled suppression policy. Integration include/exclude
	// lists are translated into leading drop rules, so this is the single path.
	rules *ruleSet
}

// parseConfig validates a source's raw config into a filter.
//
// Unlike the cluster-events adapter, an EMPTY config is invalid here: that one
// finds its data source through the pod's own ServiceAccount, while this one
// cannot guess where Home Assistant lives.
func parseConfig(raw json.RawMessage) (*filter, error) {
	cfg := sourceConfig{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("spec.config is not valid JSON for the ha adapter: %w", err)
		}
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("spec.config.endpoint is required — set it to the Home Assistant base URL " +
			"(e.g. https://ha.example.org); this adapter has no way to discover it")
	}
	if _, err := wsURLFor(cfg.Endpoint); err != nil {
		return nil, fmt.Errorf("spec.config.endpoint: %w", err)
	}

	f := &filter{
		endpoint: strings.TrimSpace(cfg.Endpoint),
		levels:   map[string]bool{},
		backfill: cfg.Backfill == nil || *cfg.Backfill,
	}
	rules := append(integrationRules(cfg.IncludeIntegrations, cfg.ExcludeIntegrations), cfg.Rules...)
	compiled, err := compileRules(rules, cfg.Route)
	if err != nil {
		return nil, fmt.Errorf("spec.config: %w", err)
	}
	f.rules = compiled

	if len(cfg.Levels) == 0 {
		f.levels["ERROR"], f.levels["CRITICAL"] = true, true
		return f, nil
	}
	for _, l := range cfg.Levels {
		up := strings.ToUpper(strings.TrimSpace(l))
		if !validLevels[up] {
			return nil, fmt.Errorf("spec.config.levels: %q is not a Home Assistant log level (expected one of %s)",
				l, strings.Join(sortedKeys(validLevels), ", "))
		}
		f.levels[up] = true
	}
	return f, nil
}

// Matches reports whether a record is in this source's SCOPE: the right level.
//
// Integration filtering deliberately does NOT live here. Include/exclude lists
// are translated into leading drop rules by parseConfig, so selection by
// integration has exactly one implementation.
func (f *filter) Matches(rec *logRecord) bool {
	return f.levels[strings.ToUpper(rec.Level)]
}

// Rule returns the policy for a normalized signal: the first matching rule, or
// false when no rule applies (which callers treat as emit-immediately).
func (f *filter) Rule(sig *Signal) (compiledRule, bool) {
	if f.rules == nil {
		return compiledRule{}, false
	}
	return f.rules.Match(matchLabels(sig))
}

// integrationOf derives the integration a logger belongs to.
//
// Home Assistant's loggers are hierarchical and the domain sits in a known
// position: homeassistant.components.<domain>.* for built-ins,
// custom_components.<domain>.* for everything installed by hand. Anything else
// is core, and the logger's own name IS the integration — never empty, because
// this label is what conversations group on and an empty grouping key would
// merge every unrelated core error into one conversation.
func integrationOf(logger string) string {
	switch {
	case strings.HasPrefix(logger, "homeassistant.components."):
		return firstSegment(strings.TrimPrefix(logger, "homeassistant.components."))
	case strings.HasPrefix(logger, "custom_components."):
		return firstSegment(strings.TrimPrefix(logger, "custom_components."))
	case logger == "":
		return "unknown"
	default:
		return logger
	}
}

// configEntriesLogger is the core Home Assistant logger that reports
// config-entry setup failures. Unlike every other logger this adapter reads,
// its NAME carries no domain — homeassistant.components.<domain> and
// custom_components.<domain> are the only prefixes integrationOf strips, and
// this logger matches neither. The domain instead sits inside the message
// text, which is why normalize() special-cases it below.
const configEntriesLogger = "homeassistant.config_entries"

// configEntryDomainPatterns extract the real Home Assistant domain from a
// homeassistant.config_entries message, one per format string that logger
// actually uses (the same shapes the shipped default rule already matches by
// keyword). Anchored to each format string's LEADING literal structure —
// never a bare "for (\w+)" — so free text elsewhere in a message cannot be
// misattributed as a domain, and each capture is restricted to a valid Python
// identifier shape (HA domains are lowercase snake_case module names). The
// domain capture itself is bounded by a word boundary, never end-of-string:
// anchoring on `$` would silently fail to match — falling back to the logger
// name with nothing logged — the moment any wording HA appends after the
// domain differs from what was observed live.
var configEntryDomainPatterns = []*regexp.Regexp{
	// "Error setting up entry <title> for <domain>"
	regexp.MustCompile(`(?s)^Error setting up entry .+ for ([a-z][a-z0-9_]*)\b`),
	// "Setup failed for '<domain>': <error>"
	regexp.MustCompile(`(?s)^Setup failed for '([a-z][a-z0-9_]*)':`),
	// "Config entry '<title>' for <domain> integration not ready yet: ..."
	// "Config entry '<title>' for <domain> could not ..."
	// "Config entry '<title>' for <domain> is not ready yet ..."
	regexp.MustCompile(`(?s)^Config entry '.+' for ([a-z][a-z0-9_]*) (?:integration not ready yet|could not|is not ready)`),
}

// domainFromConfigEntryMessage reports the domain a config-entry setup
// failure names, or "" when the message matches none of the recognized
// shapes — callers fall back to the logger-derived identity in that case,
// exactly as they did before this function existed.
func domainFromConfigEntryMessage(text string) string {
	for _, re := range configEntryDomainPatterns {
		if m := re.FindStringSubmatch(text); m != nil {
			return m[1]
		}
	}
	return ""
}

func firstSegment(s string) string {
	if i := strings.Index(s, "."); i > 0 {
		return s[:i]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

// maxTitleText bounds how much of a log message reaches the title. Long enough
// to identify the error, short enough that a chat thread name stays readable.
const maxTitleText = 120

// normalize turns one log record into a contract signal for a source.
//
// The fingerprint keys on the LOGGER and SOURCE LOCATION — Home Assistant's own
// deduplication key — and never on the timestamp or the occurrence count. A
// recurring error must keep one identity, or every repeat would open a fresh
// conversation instead of collapsing under the manager's cooldown.
func normalize(source string, rec *logRecord) Signal {
	integration := integrationOf(rec.Name)
	text := rec.Text()
	if rec.Name == configEntriesLogger {
		if domain := domainFromConfigEntryMessage(text); domain != "" {
			integration = domain
		}
	}

	payload, _ := json.MarshalIndent(map[string]any{
		"integration":   integration,
		"logger":        rec.Name,
		"level":         strings.ToUpper(rec.Level),
		"message":       text,
		"location":      rec.Location(),
		"count":         rec.Count,
		"firstOccurred": timeString(epoch(rec.FirstOccurred)),
		"lastOccurred":  timeString(rec.At()),
		"exception":     rec.Exception,
	}, "", "  ")

	labels := map[string]string{
		"alertgroup":  "ha-logs",
		"alertname":   integration,
		"integration": integration,
		"logger":      rec.Name,
		"level":       strings.ToUpper(rec.Level),
		"location":    rec.Location(),
		"source":      source,
	}

	return Signal{
		Fingerprint: fmt.Sprintf("%s@%s", source, rec.Key()),
		Labels:      labels,
		Title:       fmt.Sprintf("%s: %s", integration, truncate(oneLine(text), maxTitleText)),
		Payload:     string(payload),
		Kind:        "alert",
		MatchText:   text,
	}
}

func oneLine(s string) string {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}

func timeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

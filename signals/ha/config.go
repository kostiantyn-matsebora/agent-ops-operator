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
	// Surfaces switches and tunes the health surfaces beside the log. Kept raw
	// so it can be decoded STRICTLY: a misspelt surface or knob here would
	// otherwise be a silently ignored key, which for a switch means a surface
	// somebody believes is off.
	Surfaces json.RawMessage `json:"surfaces,omitempty"`
}

// surfacesConfig is `surfaces`: one object per surface, each with `enabled`
// and the one knob that surface has.
type surfacesConfig struct {
	ConfigEntries *surfaceConfig `json:"configEntries"`
	Repairs       *surfaceConfig `json:"repairs"`
	Sensors       *surfaceConfig `json:"sensors"`
	Updates       *surfaceConfig `json:"updates"`
}

// surfaceConfig is one surface's switch and knob. Each knob belongs to ONE
// surface and is refused on any other: a `states` list under `sensors` is a
// mistake, not a no-op.
type surfaceConfig struct {
	Enabled *bool `json:"enabled"`
	// States are the config entry states that count (configEntries).
	States []string `json:"states"`
	// Severities are the repair severities that count (repairs).
	Severities []string `json:"severities"`
	// DeviceClasses are the binary_sensor device classes watched (sensors).
	DeviceClasses []string `json:"deviceClasses"`
}

// surfacePolicy is one surface, compiled: on or off, and what counts.
type surfacePolicy struct {
	enabled bool
	accept  map[string]bool
}

// surfacePolicies is every surface, compiled, with the defaults applied.
type surfacePolicies struct {
	configEntries surfacePolicy
	repairs       surfacePolicy
	sensors       surfacePolicy
	updates       surfacePolicy
}

// defaultSurfaces: config entries, repairs and sensors on; the update digest
// off — a list of pending packages is not an incident until somebody says
// they want to hear about it. Warning-severity repairs are deprecation and
// version notices, the same background hum `levels` keeps out of the log lane.
func defaultSurfaces() surfacePolicies {
	return surfacePolicies{
		configEntries: surfacePolicy{enabled: true, accept: setOf("setup_retry", "setup_error", "migration_error")},
		repairs:       surfacePolicy{enabled: true, accept: setOf("critical", "error")},
		sensors:       surfacePolicy{enabled: true, accept: setOf("problem", "connectivity")},
		updates:       surfacePolicy{enabled: false},
	}
}

func setOf(values ...string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

// parseSurfaces validates the `surfaces` block against the defaults.
func parseSurfaces(raw json.RawMessage) (surfacePolicies, error) {
	out := defaultSurfaces()
	if len(raw) == 0 || string(raw) == "null" {
		return out, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var cfg surfacesConfig
	if err := dec.Decode(&cfg); err != nil {
		return out, fmt.Errorf("spec.config.surfaces: %w (the surfaces are configEntries, repairs, sensors, updates; "+
			"each takes `enabled` and its own knob)", err)
	}
	type knob struct {
		name   string
		values []string
	}
	apply := func(name string, c *surfaceConfig, p *surfacePolicy, own string, vocab map[string]bool, knobs ...knob) error {
		if c == nil {
			return nil
		}
		if c.Enabled != nil {
			p.enabled = *c.Enabled
		}
		for _, k := range knobs {
			if k.name != own && len(k.values) > 0 {
				return fmt.Errorf("spec.config.surfaces.%s: `%s` is not a knob of this surface", name, k.name)
			}
			if k.name != own || len(k.values) == 0 {
				continue
			}
			accept := map[string]bool{}
			for _, v := range k.values {
				v = strings.ToLower(strings.TrimSpace(v))
				if !vocab[v] {
					return fmt.Errorf("spec.config.surfaces.%s.%s: %q is not one of %s", name, own, v,
						strings.Join(sortedKeys(vocab), ", "))
				}
				accept[v] = true
			}
			p.accept = accept
		}
		return nil
	}
	knobsOf := func(c *surfaceConfig) []knob {
		if c == nil {
			return nil
		}
		return []knob{{"states", c.States}, {"severities", c.Severities}, {"deviceClasses", c.DeviceClasses}}
	}
	sensorClasses := map[string]bool{}
	for class := range sensorFaultState {
		sensorClasses[class] = true
	}
	for _, step := range []error{
		apply("configEntries", cfg.ConfigEntries, &out.configEntries, "states", configEntryStates, knobsOf(cfg.ConfigEntries)...),
		apply("repairs", cfg.Repairs, &out.repairs, "severities", repairSeverities, knobsOf(cfg.Repairs)...),
		apply("sensors", cfg.Sensors, &out.sensors, "deviceClasses", sensorClasses, knobsOf(cfg.Sensors)...),
		apply("updates", cfg.Updates, &out.updates, "", nil, knobsOf(cfg.Updates)...),
	} {
		if step != nil {
			return out, step
		}
	}
	return out, nil
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
	// surfaces is which health surfaces are read beside the log, and what
	// counts on each.
	surfaces surfacePolicies
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
	surfaces, err := parseSurfaces(cfg.Surfaces)
	if err != nil {
		return nil, err
	}
	f.surfaces = surfaces

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
	// `levels` scopes LOG records. A surface condition is in scope by the
	// surface being enabled; a rule is how one is silenced from there.
	if rec.Surface != "" {
		return true
	}
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
	surface := rec.Surface
	if surface == "" {
		surface = surfaceLog
	} else if rec.Integration != "" {
		integration = rec.Integration
	}

	fields := map[string]any{
		"surface":       surface,
		"integration":   integration,
		"logger":        rec.Name,
		"level":         strings.ToUpper(rec.Level),
		"message":       text,
		"location":      rec.Location(),
		"count":         rec.Count,
		"firstOccurred": timeString(epoch(rec.FirstOccurred)),
		"lastOccurred":  timeString(rec.At()),
		"exception":     rec.Exception,
	}
	for k, v := range rec.Extra {
		fields[k] = v
	}
	payload, _ := json.MarshalIndent(fields, "", "  ")

	labels := map[string]string{
		"alertgroup":  "ha-logs",
		"alertname":   integration,
		"integration": integration,
		"logger":      rec.Name,
		"level":       strings.ToUpper(rec.Level),
		"location":    rec.Location(),
		"source":      source,
		"surface":     surface,
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

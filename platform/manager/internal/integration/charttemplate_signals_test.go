package integration

import (
	"regexp"
	"strings"
	"testing"
)

// signalEnv is the manager Deployment's env var name -> chart default pairs
// the signal-persistence change introduced. The chart passes the value
// through untouched; the manager owns parsing and the fallback to its default.
var signalEnv = map[string]string{
	"SIGNAL_CONSUMER_CONCURRENCY": "4",
	"SIGNAL_RETRY_DEADLINE":       "1h",
	"MAX_SIGNALS_PER_SOURCE":      "200",
	"SIGNAL_TTL_ROUTED":           "8h",
	"SIGNAL_TTL_DROPPED":          "4h",
	"SIGNAL_TTL_FAILED":           "72h",
}

func envValue(t *testing.T, out, name string) string {
	t.Helper()
	m := regexp.MustCompile(`- name: ` + name + `\n\s+value: "([^"]*)"`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("manager env %s is not rendered", name)
	}
	return m[1]
}

func TestSignalPersistenceValuesRenderWithDefaults(t *testing.T) {
	out := helmTemplate(t)
	for name, want := range signalEnv {
		if got := envValue(t, out, name); got != want {
			t.Fatalf("%s defaults to %q, want %q", name, got, want)
		}
	}
}

func TestSignalPersistenceValuesCanBeOverridden(t *testing.T) {
	out := helmTemplate(t,
		"--set", "signalConsumerConcurrency=8", "--set", "signalRetryDeadline=30m",
		"--set", "maxSignalsPerSource=50", "--set", "signalTtlRouted=6h",
		"--set", "signalTtlDropped=2h", "--set", "signalTtlFailed=24h")
	for name, want := range map[string]string{
		"SIGNAL_CONSUMER_CONCURRENCY": "8", "SIGNAL_RETRY_DEADLINE": "30m", "MAX_SIGNALS_PER_SOURCE": "50",
		"SIGNAL_TTL_ROUTED": "6h", "SIGNAL_TTL_DROPPED": "2h", "SIGNAL_TTL_FAILED": "24h",
	} {
		if got := envValue(t, out, name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

// An invalid duration is passed through verbatim — the manager warns and
// falls back to its own default; the render must not be the place it fails.
func TestSignalPersistenceInvalidDurationPassesThrough(t *testing.T) {
	out := helmTemplate(t, "--set", "signalTtlFailed=soon")
	if got := envValue(t, out, "SIGNAL_TTL_FAILED"); !strings.EqualFold(got, "soon") {
		t.Fatalf("invalid duration must pass through, got %q", got)
	}
}

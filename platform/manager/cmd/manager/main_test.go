package main

import (
	"testing"
	"time"
)

// The env-reading helpers main() wires the whole process from -- pure enough
// to need no mocking, but never pinned, which is why every one of them still
// carried its zero-value fallback UNVERIFIED.

func TestEnvFallsBackOnlyWhenUnset(t *testing.T) {
	t.Setenv("AGENTOPS_TEST_ENV", "")
	if got := env("AGENTOPS_TEST_ENV", "default"); got != "default" {
		t.Fatalf("unset: got %q, want the default", got)
	}
	t.Setenv("AGENTOPS_TEST_ENV", "set")
	if got := env("AGENTOPS_TEST_ENV", "default"); got != "set" {
		t.Fatalf("set: got %q, want the env value", got)
	}
}

func TestEnvIntRejectsUnparseableAndNonPositive(t *testing.T) {
	for _, tc := range []struct {
		name, val string
		want      int
	}{
		{"unset", "", 7},
		{"not a number", "nope", 7},
		{"zero", "0", 7},
		{"negative", "-1", 7},
		{"positive", "3", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENTOPS_TEST_ENVINT", tc.val)
			if got := envInt("AGENTOPS_TEST_ENVINT", 7); got != tc.want {
				t.Fatalf("envInt(%q) = %d, want %d", tc.val, got, tc.want)
			}
		})
	}
}

func TestEnvIntNonNegativeKeepsZero(t *testing.T) {
	for _, tc := range []struct {
		name, val string
		want      int
	}{
		{"unset", "", 7},
		{"not a number", "nope", 7},
		{"zero", "0", 0},
		{"negative", "-1", 7},
		{"positive", "3", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGENTOPS_TEST_ENVINT", tc.val)
			if got := envIntNonNegative("AGENTOPS_TEST_ENVINT", 7); got != tc.want {
				t.Fatalf("envIntNonNegative(%q) = %d, want %d", tc.val, got, tc.want)
			}
		})
	}
}

func TestEnvBoolOnlyRecognisesExplicitTrueSpellings(t *testing.T) {
	for _, tc := range []struct{ val string }{{"true"}, {"1"}, {"yes"}, {"on"}, {"TRUE"}, {" on "}} {
		t.Setenv("AGENTOPS_TEST_ENVBOOL", tc.val)
		if !envBool("AGENTOPS_TEST_ENVBOOL") {
			t.Fatalf("%q should read as true", tc.val)
		}
	}
	for _, tc := range []struct{ val string }{{""}, {"false"}, {"0"}, {"maybe"}} {
		t.Setenv("AGENTOPS_TEST_ENVBOOL", tc.val)
		if envBool("AGENTOPS_TEST_ENVBOOL") {
			t.Fatalf("%q must default closed, not open -- a typo must decline, not enable", tc.val)
		}
	}
}

func TestEnvDurationZerosOutOnAnythingNotPositive(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want time.Duration
	}{
		{"", 0}, {"nope", 0}, {"0h", 0}, {"-5m", 0}, {"30m", 30 * time.Minute},
	} {
		t.Setenv("AGENTOPS_TEST_ENVDUR", tc.val)
		if got := envDuration("AGENTOPS_TEST_ENVDUR"); got != tc.want {
			t.Fatalf("envDuration(%q) = %v, want %v", tc.val, got, tc.want)
		}
	}
}

func TestEnvDurationOrFallsBackToTheDefaultNotZero(t *testing.T) {
	t.Setenv("AGENTOPS_TEST_ENVDUROR", "")
	if got := envDurationOr("AGENTOPS_TEST_ENVDUROR", 5*time.Minute); got != 5*time.Minute {
		t.Fatalf("unset must take the default, not zero: got %v", got)
	}
	t.Setenv("AGENTOPS_TEST_ENVDUROR", "1h")
	if got := envDurationOr("AGENTOPS_TEST_ENVDUROR", 5*time.Minute); got != time.Hour {
		t.Fatalf("set value must win: got %v", got)
	}
}

// envSeconds reads a plain integer seconds value, never a Go duration
// string, and zero or unset defers to the caller's own default.
func TestEnvSecondsReadsWholeSecondsAndDefersOnZeroOrUnset(t *testing.T) {
	for _, tc := range []struct {
		val  string
		want time.Duration
	}{
		{"", 0}, {"0", 0}, {"-5", 0}, {"nope", 0}, {"90", 90 * time.Second},
	} {
		t.Setenv("AGENTOPS_TEST_ENVSECONDS", tc.val)
		if got := envSeconds("AGENTOPS_TEST_ENVSECONDS"); got != tc.want {
			t.Fatalf("envSeconds(%q) = %v, want %v", tc.val, got, tc.want)
		}
	}
}

// maxActiveConversations reports the DEPRECATED spelling was used only when
// it was the one that actually supplied the value -- the new name always
// wins when both are set.
func TestMaxActiveConversationsPrefersTheNewSpelling(t *testing.T) {
	t.Setenv("MAX_ACTIVE_CONVERSATIONS", "")
	t.Setenv("MAX_RUNTIMES", "")
	if n, dep := maxActiveConversations(); n != 5 || dep {
		t.Fatalf("default: got (%d, %v), want (5, false)", n, dep)
	}

	t.Setenv("MAX_RUNTIMES", "9")
	if n, dep := maxActiveConversations(); n != 9 || !dep {
		t.Fatalf("deprecated alone: got (%d, %v), want (9, true)", n, dep)
	}

	t.Setenv("MAX_ACTIVE_CONVERSATIONS", "3")
	if n, dep := maxActiveConversations(); n != 3 || dep {
		t.Fatalf("both set: got (%d, %v), want the new spelling (3, false)", n, dep)
	}
}

func TestCommandFromEnvParsesTheJSONArrayOrReturnsNil(t *testing.T) {
	t.Setenv("RUNTIME_COMMAND_JSON", "")
	if got := commandFromEnv(); got != nil {
		t.Fatalf("unset must be nil, got %v", got)
	}
	t.Setenv("RUNTIME_COMMAND_JSON", "not json")
	if got := commandFromEnv(); got != nil {
		t.Fatalf("unparseable must be nil, got %v", got)
	}
	t.Setenv("RUNTIME_COMMAND_JSON", `["sh","-c","echo hi"]`)
	got := commandFromEnv()
	if len(got) != 3 || got[0] != "sh" || got[2] != "echo hi" {
		t.Fatalf("got %v", got)
	}
}

// HOME_PVC IS GONE, NOT DUAL-READ, and this pins the difference.
//
// The sessionId precedent does not apply and reading it as though it did is the
// trap: that was a field RENAMED IN PLACE, so an alias pointed at something
// real. The concept behind HOME_PVC moved to a DIFFERENT CR — persistence is
// the Pipeline's now — so an alias would resolve to a field that is not there,
// and the only honest behaviour is to ignore it loudly. The chart and the
// manager ship together in this release, so there is no window to cover.
func TestTheRetiredBootstrapSpellingIsNotRead(t *testing.T) {
	t.Setenv("CONTEXT_PVC", "")
	t.Setenv("HOME_PVC", "agentops-home")

	if got := bootstrapContextClaim(); got != "" {
		t.Fatalf("HOME_PVC was honoured (%q). It is retired, not aliased: an install still setting it "+
			"must get an ephemeral volume and read the CHANGELOG, not a quiet resolution to a "+
			"concept that lives on another object now", got)
	}

	t.Setenv("CONTEXT_PVC", "agentops-context")
	if got := bootstrapContextClaim(); got != "agentops-context" {
		t.Fatalf("CONTEXT_PVC = %q, want the claim it names", got)
	}
}

// signal-persistence: "A bad duration falls back loudly" — an unparseable or
// non-positive SIGNAL_TTL_ROUTED yields the default AND a warning that names
// the variable. An unset one is the default without a warning.
func TestEnvDurationOrWarnsNamingTheVariableOnABadValue(t *testing.T) {
	var warned []string
	prev := warn
	warn = func(msg string, kv ...any) {
		for i := 0; i+1 < len(kv); i += 2 {
			if kv[i] == "variable" {
				warned = append(warned, kv[i+1].(string))
			}
		}
	}
	t.Cleanup(func() { warn = prev })

	for _, bad := range []string{"soon", "-1h", "0s"} {
		t.Setenv("SIGNAL_TTL_ROUTED", bad)
		if got := envDurationOr("SIGNAL_TTL_ROUTED", 8*time.Hour); got != 8*time.Hour {
			t.Fatalf("%q: want the 8h default, got %v", bad, got)
		}
	}
	if len(warned) != 3 || warned[0] != "SIGNAL_TTL_ROUTED" {
		t.Fatalf("each bad value warns, naming the variable: %v", warned)
	}

	warned = nil
	t.Setenv("SIGNAL_TTL_ROUTED", "")
	envDurationOr("SIGNAL_TTL_ROUTED", 8*time.Hour)
	t.Setenv("SIGNAL_TTL_ROUTED", "6h")
	if got := envDurationOr("SIGNAL_TTL_ROUTED", 8*time.Hour); got != 6*time.Hour {
		t.Fatalf("override: %v", got)
	}
	if len(warned) != 0 {
		t.Fatalf("unset and valid values must not warn: %v", warned)
	}
}

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

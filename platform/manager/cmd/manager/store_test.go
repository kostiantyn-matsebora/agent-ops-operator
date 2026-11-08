package main

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestStateBackendDefaultsToKube(t *testing.T) {
	for _, tc := range []struct{ name, val string }{
		{"unset", ""},
		{"blank", "  "},
		{"explicit", "kube"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(stateBackendEnv, tc.val)
			got, err := stateBackendFromEnv()
			if err != nil || got != backendKube {
				t.Fatalf("got (%q, %v), want the kube backend", got, err)
			}
		})
	}
}

func TestStateBackendRefusesAnUnknownNameAndNamesIt(t *testing.T) {
	t.Setenv(stateBackendEnv, "nosuch")
	got, err := stateBackendFromEnv()
	if err == nil || !strings.Contains(err.Error(), `"nosuch"`) {
		t.Fatalf("got (%q, %v), want an error naming nosuch", got, err)
	}
}

func TestStateBackendAcceptsNats(t *testing.T) {
	t.Setenv(stateBackendEnv, "nats")
	got, err := stateBackendFromEnv()
	if err != nil || got != backendNats {
		t.Fatalf("got (%q, %v), want the nats backend", got, err)
	}
}

// newNatsStores must fail fast, before connecting to anything, when the
// nats backend is selected with no NATS_URL — the same "stops before it
// connects" posture stateBackendFromEnv itself has.
func TestNewNatsStoresRequiresNatsURL(t *testing.T) {
	t.Setenv("NATS_URL", "")
	if _, err := newNatsStores(nil, "agent-ops"); err == nil {
		t.Fatal("want an error with NATS_URL unset")
	}
}

// runMainEnv switches TestMainExitsOnAnUnknownStateBackend's re-executed copy
// of the test binary into running main itself.
const runMainEnv = "AGENTOPS_TEST_RUN_MAIN"

// main must stop the process, not log and carry on, when the backend is one it
// does not carry. Proved on a real process: the test binary re-executes itself
// as the manager, with no cluster reachable, which also shows the check runs
// before anything connects to one.
func TestMainExitsOnAnUnknownStateBackend(t *testing.T) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainExitsOnAnUnknownStateBackend$")
	cmd.Env = append(os.Environ(), runMainEnv+"=1", stateBackendEnv+"=nosuch")
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("want a non-zero exit, got %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "nosuch") {
		t.Fatalf("the refusal must name the value:\n%s", out)
	}
}

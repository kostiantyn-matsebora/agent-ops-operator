package main

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	commandbusnats "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/nats"
)

func TestCommandBusBackendFromEnv(t *testing.T) {
	for _, tc := range []struct {
		name, val, want string
	}{
		{"unset", "", commandBusBackendMemory},
		{"blank", "  ", commandBusBackendMemory},
		{"explicit memory", commandBusBackendMemory, commandBusBackendMemory},
		{"nats", commandBusBackendNats, commandBusBackendNats},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(commandBusBackendEnv, tc.val)
			got, err := commandBusBackendFromEnv()
			if err != nil || got != tc.want {
				t.Fatalf("got (%q, %v), want (%q, nil)", got, err, tc.want)
			}
		})
	}
}

func TestCommandBusBackendFromEnvRefusesAnUnknownNameAndNamesIt(t *testing.T) {
	t.Setenv(commandBusBackendEnv, "nosuch")
	got, err := commandBusBackendFromEnv()
	if err == nil || !strings.Contains(err.Error(), `"nosuch"`) {
		t.Fatalf("got (%q, %v), want an error naming nosuch", got, err)
	}
}

// commandBusBackendFromEnv must refuse memory outright once more than one
// replica is in play (design D-13, task 4.4) — never silently resolve to a
// backend that keeps its tasks invisible to every other replica. Checked
// here, before any cluster or server is touched, exactly like the unknown-
// name refusal above.
func TestCommandBusBackendFromEnvRefusesMemoryAboveOneReplica(t *testing.T) {
	t.Setenv(commandBusBackendEnv, commandBusBackendMemory)
	t.Setenv(managerReplicasEnv, "3")
	_, err := commandBusBackendFromEnv()
	if err == nil {
		t.Fatal("want an error, got none")
	}
	for _, want := range []string{commandBusBackendMemory, "3"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
}

// The refusal also applies to the default (COMMANDBUS_BACKEND unset, which
// resolves to memory) — a deployment that forgot to set it above one
// replica must fail exactly as loudly as one that set it explicitly.
func TestCommandBusBackendFromEnvRefusesTheDefaultAboveOneReplica(t *testing.T) {
	t.Setenv(managerReplicasEnv, "2")
	if _, err := commandBusBackendFromEnv(); err == nil {
		t.Fatal("want an error, got none")
	}
}

func TestCommandBusBackendFromEnvAcceptsMemoryAtOneReplica(t *testing.T) {
	t.Setenv(commandBusBackendEnv, commandBusBackendMemory)
	t.Setenv(managerReplicasEnv, "1")
	got, err := commandBusBackendFromEnv()
	if err != nil || got != commandBusBackendMemory {
		t.Fatalf("got (%q, %v), want (%q, nil)", got, err, commandBusBackendMemory)
	}
}

// Replicas is irrelevant to the nats backend — only memory loses visibility
// across replicas.
func TestCommandBusBackendFromEnvAcceptsNatsAboveOneReplica(t *testing.T) {
	t.Setenv(commandBusBackendEnv, commandBusBackendNats)
	t.Setenv(managerReplicasEnv, "3")
	got, err := commandBusBackendFromEnv()
	if err != nil || got != commandBusBackendNats {
		t.Fatalf("got (%q, %v), want (%q, nil)", got, err, commandBusBackendNats)
	}
}

func TestManagerReplicasDefaultsToOne(t *testing.T) {
	for _, tc := range []struct{ name, val string }{
		{"unset", ""},
		{"unparseable", "many"},
		{"zero", "0"},
		{"negative", "-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(managerReplicasEnv, tc.val)
			if got := managerReplicas(); got != 1 {
				t.Fatalf("managerReplicas() = %d, want 1", got)
			}
		})
	}
}

func TestManagerReplicasReadsAnExplicitCount(t *testing.T) {
	t.Setenv(managerReplicasEnv, "3")
	if got := managerReplicas(); got != 3 {
		t.Fatalf("managerReplicas() = %d, want 3", got)
	}
}

func TestNewCommandBusOpensMemory(t *testing.T) {
	bus, err := newCommandBus(testManager(t), "agent-ops", commandBusBackendMemory)
	if err != nil || bus == nil {
		t.Fatalf("got (%v, %v), want a bus and no error", bus, err)
	}
}

func TestNewCommandBusRefusesAnUnknownBackend(t *testing.T) {
	if _, err := newCommandBus(testManager(t), "agent-ops", "nosuch"); err == nil || !strings.Contains(err.Error(), `"nosuch"`) {
		t.Fatalf("got %v, want an error naming nosuch", err)
	}
}

func TestCommandBusBackendNeverReadsTheStateStoresNatsVariables(t *testing.T) {
	// design D-1: "it never reads NATS_*" — only COMMANDBUS_NATS_URL
	// selects the server, even when the state store's own NATS_URL is set
	// to something that would otherwise connect.
	t.Setenv("NATS_URL", "nats://127.0.0.1:1")
	if _, err := newCommandBus(testManager(t), "agent-ops", commandBusBackendNats); err == nil || !strings.Contains(err.Error(), commandBusNatsURLEnv) {
		t.Fatalf("newCommandBus = %v, want a failure naming %s, not NATS_URL", err, commandBusNatsURLEnv)
	}
}

// withSingleNodeReplicas narrows every Open call this test makes to one
// replica — the embedded, single-process nats-server startTestNats starts
// refuses the backend's own 3-replica production default outright
// ("replicas > 1 not supported in non-clustered mode").
func withSingleNodeReplicas(t *testing.T) {
	t.Helper()
	old := commandBusNatsOptions
	commandBusNatsOptions = []commandbusnats.Option{commandbusnats.WithReplicas(1)}
	t.Cleanup(func() { commandBusNatsOptions = old })
}

func TestNewCommandBusOpensNats(t *testing.T) {
	withSingleNodeReplicas(t)
	srv := startTestNats(t)
	t.Setenv(commandBusNatsURLEnv, srv.ClientURL())
	bus, err := newCommandBus(testManager(t), "agent-ops", commandBusBackendNats)
	if err != nil || bus == nil {
		t.Fatalf("got (%v, %v), want a bus and no error", bus, err)
	}
}

func TestNewNatsCommandBusRequiresTheURL(t *testing.T) {
	t.Setenv(commandBusNatsURLEnv, "")
	if _, err := newNatsCommandBus(testManager(t), "agent-ops"); err == nil || !strings.Contains(err.Error(), commandBusNatsURLEnv) {
		t.Fatalf("got %v, want an error naming %s", err, commandBusNatsURLEnv)
	}
}

func TestNewNatsCommandBusRefusesAnUnreachableServer(t *testing.T) {
	t.Setenv(commandBusNatsURLEnv, "nats://127.0.0.1:1")
	if _, err := newNatsCommandBus(testManager(t), "agent-ops"); err == nil {
		t.Fatal("want an error connecting to a closed port")
	}
}

func TestNewNatsCommandBusFailsWhenTheBucketsCannotBePlaced(t *testing.T) {
	srv := startTestNats(t)
	t.Setenv(commandBusNatsURLEnv, srv.ClientURL())

	oldTimeout, oldInterval := commandBusOpenTimeout, commandBusOpenRetryInterval
	commandBusOpenTimeout, commandBusOpenRetryInterval = 2*time.Second, 200*time.Millisecond
	t.Cleanup(func() { commandBusOpenTimeout, commandBusOpenRetryInterval = oldTimeout, oldInterval })

	// Replicas is a Config.Replicas field on commandbus/nats, not an env
	// var this package reads — the failure this test wants is an
	// unreachable connection instead, exercised identically to the state
	// store's own "cannot place a replicated bucket" case by closing the
	// connection out from under Open before it can bootstrap anything.
	conn, err := commandbusnats.Connect(commandbusnats.Config{URL: srv.ClientURL()})
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()

	done := make(chan error, 1)
	go func() {
		_, err := openCommandBusWhenReady(context.Background(), conn, "agent-ops", commandBusOpenTimeout, commandBusOpenRetryInterval)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want an error opening buckets on a closed connection")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("openCommandBusWhenReady did not give up within its own timeout")
	}
}

// commandBusReadinessCheck reports not ready while the server is
// unreachable, and ready while it answers (task 4.3).
func TestCommandBusReadinessCheck(t *testing.T) {
	withSingleNodeReplicas(t)
	srv := startTestNats(t)
	t.Setenv(commandBusNatsURLEnv, srv.ClientURL())
	if _, err := newNatsCommandBus(testManager(t), "agent-ops"); err != nil {
		t.Fatal(err)
	}

	conn, err := commandbusnats.Connect(commandbusnats.Config{URL: srv.ClientURL()})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	check := commandBusReadinessCheck(conn)
	req := httptest.NewRequest("GET", "/readyz", nil)
	if err := check(req); err != nil {
		t.Fatalf("a reachable server: %v, want ready", err)
	}
	conn.Close()
	if err := check(req); err == nil {
		t.Fatal("want not-ready once the server is unreachable")
	}
}

// main must stop the process, not log and carry on, when COMMANDBUS_BACKEND
// names one this binary does not carry — proved on a real process exactly
// as TestMainExitsOnAnUnknownStateBackend proves it for STATE_BACKEND.
// Resolved before the cluster config is ever loaded (see main's own
// ordering), so this never depends on a reachable cluster being available.
func TestMainExitsOnAnUnknownCommandBusBackend(t *testing.T) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainExitsOnAnUnknownCommandBusBackend$")
	cmd.Env = append(os.Environ(), runMainEnv+"=1", commandBusBackendEnv+"=nosuch")
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("want a non-zero exit, got %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "nosuch") {
		t.Fatalf("the refusal must name the value:\n%s", out)
	}
}

// main must stop the process when the memory backend is selected above one
// replica, rather than starting a bus no other replica can ever see — also
// resolved before the cluster config is loaded.
func TestMainExitsWhenMemoryIsSelectedAboveOneReplica(t *testing.T) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainExitsWhenMemoryIsSelectedAboveOneReplica$")
	cmd.Env = append(os.Environ(), runMainEnv+"=1", commandBusBackendEnv+"="+commandBusBackendMemory, managerReplicasEnv+"=2")
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("want a non-zero exit, got %v\n%s", err, out)
	}
	if !strings.Contains(string(out), commandBusBackendMemory) {
		t.Fatalf("the refusal must name the backend:\n%s", out)
	}
}

func TestChannelFeedAdaptersDefaultToTheConsoleAndAnEmptyValueMeansNone(t *testing.T) {
	t.Setenv(channelFeedAdaptersEnv, "")
	if got := channelFeedAdapters(); len(got) != 0 {
		t.Fatalf("an explicitly empty value means no feed adapter, got %v", got)
	}
	t.Setenv(channelFeedAdaptersEnv, "console, mobile ,")
	if got := strings.Join(channelFeedAdapters(), "|"); got != "console|mobile" {
		t.Fatalf("adapters = %q", got)
	}
}

func TestChannelFeedRetentionIsReadFromTheEnvironment(t *testing.T) {
	inv := channelFeedInventory()
	if p := inv[0].Policy; p.MaxAge != 30*time.Second || p.MaxEntries != 5000 {
		t.Fatalf("default retention = %+v, want 30s and 5000 entries", p)
	}
	t.Setenv(channelFeedRetentionEnv, "2h")
	t.Setenv(channelFeedMaxEntriesEnv, "100")
	if p := channelFeedInventory()[0].Policy; p.MaxAge != 2*time.Hour || p.MaxEntries != 100 {
		t.Fatalf("retention = %+v, want 2h and 100 entries", p)
	}
}

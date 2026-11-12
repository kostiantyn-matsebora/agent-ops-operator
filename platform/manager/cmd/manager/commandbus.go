package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	commandbusmemory "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/memory"
	commandbusnats "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus/nats"
)

// commandBusBackendEnv selects the command bus backend (design D-1 of
// openspec/changes/async-command-bus/), independent of STATE_BACKEND — an
// install may run the kube state store with the nats command bus, or the
// reverse. Unset means the memory backend: safe for a direct run or a
// single-replica install, refused outright above one replica (task 4.4).
const commandBusBackendEnv = "COMMANDBUS_BACKEND"

const (
	commandBusBackendMemory = "memory"
	commandBusBackendNats   = "nats"
)

// commandBusNatsURLEnv, commandBusNatsUserEnv and commandBusNatsPasswordEnv
// are the command bus's OWN variables (design D-1: "it never reads
// NATS_*") — a shared name with the state store's own NATS_URL/NATS_USER/
// NATS_PASSWORD would let one install's two backends silently share a
// connection the design explicitly keeps independent.
const (
	commandBusNatsURLEnv      = "COMMANDBUS_NATS_URL"
	commandBusNatsUserEnv     = "COMMANDBUS_NATS_USER"
	commandBusNatsPasswordEnv = "COMMANDBUS_NATS_PASSWORD"
)

// commandBusConsumerConcurrencyEnv sizes every consumer.Pool this process
// runs (task 4.2) — one knob for all of them, matching
// SIGNAL_CONSUMER_CONCURRENCY's own shape one section over. Registering a
// real pool per queue is section 5/6's job; this env var exists now so
// their wiring has nothing left to invent.
const commandBusConsumerConcurrencyEnv = "COMMANDBUS_CONSUMER_CONCURRENCY"

// defaultCommandBusConsumerConcurrency is a small, safe starting point — a
// queue bench settles the real number later (design D-13's own "numbers
// the proposal names explicitly as provisional").
const defaultCommandBusConsumerConcurrency = 4

// commandBusConsumerConcurrency resolves the per-pool concurrency every
// consumer.Pool this process runs is built with — read once here so
// section 5/6's own wiring, registering the real per-queue pools against
// the Bus this file opens, has nothing left to invent.
func commandBusConsumerConcurrency() int {
	return envInt(commandBusConsumerConcurrencyEnv, defaultCommandBusConsumerConcurrency)
}

// managerReplicasEnv names this release's manager replica count. The chart
// wires it from .Values.replicas, the SAME value that already sets the
// Deployment's own spec.replicas — not a second, independently-settable
// number — so there is exactly one place "how many replicas" is decided.
// Unset (a direct run, outside the chart) defaults to one.
const managerReplicasEnv = "MANAGER_REPLICAS"

// commandBusOpenTimeout bounds how long startup waits for the command
// bus's own buckets to be placed — a cluster still forming its quorum
// cannot place a replicated bucket yet, exactly the reasoning
// newNatsStores's own natsOpenTimeout already carries for the state store.
var commandBusOpenTimeout = 2 * time.Minute

// commandBusOpenRetryInterval is how long newNatsCommandBus waits between
// attempts while commandBusOpenTimeout has not yet elapsed.
var commandBusOpenRetryInterval = 3 * time.Second

// commandBusNatsOptions lets a test narrow the Options commandbusnats.Open
// is called with — WithReplicas(1) against a single-node embedded
// nats-server, which refuses the backend's own 3-replica default outright.
// Production passes none, so a real install gets commandbus/nats's own
// DefaultReplicas, matching the chart's three-server bundle (design D-13).
var commandBusNatsOptions []commandbusnats.Option

// commandBusBackendFromEnv resolves the selected backend name and refuses,
// by name, both a backend this binary does not carry AND memory selected
// above one replica (design D-13, task 4.4) — pure, reading only env vars,
// so main calls it before anything touches a cluster or a server, exactly
// as it already does for stateBackendFromEnv.
func commandBusBackendFromEnv() (string, error) {
	name := strings.TrimSpace(os.Getenv(commandBusBackendEnv))
	switch name {
	case "":
		name = commandBusBackendMemory
	case commandBusBackendMemory, commandBusBackendNats:
	default:
		return "", fmt.Errorf("%s=%q is not a command bus backend this manager carries (known: %s, %s)",
			commandBusBackendEnv, name, commandBusBackendMemory, commandBusBackendNats)
	}
	if replicas := managerReplicas(); name == commandBusBackendMemory && replicas > 1 {
		return "", fmt.Errorf(
			"%s=%s is refused with %s=%d: the memory backend keeps its tasks in one process's heap, "+
				"invisible to every other replica — set %s=%s for a multi-replica install",
			commandBusBackendEnv, commandBusBackendMemory, managerReplicasEnv, replicas,
			commandBusBackendEnv, commandBusBackendNats)
	}
	return name, nil
}

// managerReplicas resolves this release's replica count. envInt's own
// "unparseable or non-positive falls back to def" is exactly the floor a
// replica count needs — zero or negative replicas is not a real
// deployment, so it is read as the single-process default instead of a
// value that would make memory's refusal above never trigger.
func managerReplicas() int {
	return envInt(managerReplicasEnv, 1)
}

// newCommandBus opens backend — already resolved and validated by
// commandBusBackendFromEnv — and registers the nats backend's readiness
// check (task 4.3); the memory backend keeps nothing that can ever be
// unreachable, so it registers none. Building the consumer.Pool(s) that
// claim from the returned Bus is section 5/6's job, once a real
// HandlerFunc exists to register against it; this function only ever
// returns an opened commandbus.Bus.
func newCommandBus(mgr ctrl.Manager, namespace, backend string) (commandbus.Bus, error) {
	switch backend {
	case commandBusBackendMemory:
		return commandbusmemory.Open(), nil
	case commandBusBackendNats:
		return newNatsCommandBus(mgr, namespace)
	}
	return nil, fmt.Errorf("%s=%q is not a command bus backend this manager carries", commandBusBackendEnv, backend)
}

// newNatsCommandBus reads the command bus's own nats variables (never
// NATS_*, design D-1), connects, and opens the bus scoped to namespace —
// the same scoping newStateStores's own nats path uses, so the two
// backends may still share one server without their buckets colliding
// (commandbus/nats's own package doc).
func newNatsCommandBus(mgr ctrl.Manager, namespace string) (commandbus.Bus, error) {
	url := strings.TrimSpace(os.Getenv(commandBusNatsURLEnv))
	if url == "" {
		return nil, fmt.Errorf("commandbus/nats: %s is required when %s=%s", commandBusNatsURLEnv, commandBusBackendEnv, commandBusBackendNats)
	}
	conn, err := commandbusnats.Connect(commandbusnats.Config{
		URL:      url,
		User:     os.Getenv(commandBusNatsUserEnv),
		Password: os.Getenv(commandBusNatsPasswordEnv),
	})
	if err != nil {
		return nil, fmt.Errorf("command bus: %w", err)
	}

	bus, err := openCommandBusWhenReady(context.Background(), conn, namespace, commandBusOpenTimeout, commandBusOpenRetryInterval, commandBusNatsOptions...)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("opening the nats command bus: %w", err)
	}

	if err := mgr.AddReadyzCheck("command-bus", commandBusReadinessCheck(conn)); err != nil {
		return nil, fmt.Errorf("registering the command bus's readiness check: %w", err)
	}
	return bus, nil
}

// openCommandBusWhenReady retries commandbusnats.Open until it succeeds or
// ctx's timeout elapses — the same "a forming cluster cannot place a
// replicated bucket yet" wait newStateStores's nats path gets from
// natsstate.OpenStoresWhenReady, reimplemented here rather than imported
// because commandbus/nats, by design (D-1: "neither imports the other"),
// exposes no retrying variant of its own Open.
func openCommandBusWhenReady(ctx context.Context, conn *commandbusnats.Conn, scope string, timeout, interval time.Duration, opts ...commandbusnats.Option) (*commandbusnats.Bus, error) {
	deadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		bus, err := commandbusnats.Open(deadline, conn, scope, opts...)
		if err == nil {
			return bus, nil
		}
		select {
		case <-deadline.Done():
			return nil, fmt.Errorf("%w (last attempt: %v)", deadline.Err(), err)
		case <-time.After(interval):
		}
	}
}

// commandBusReadinessCheck reports not ready while the nats server cannot
// be reached (design D-13: "the manager reports not ready while the bus is
// unreachable") — unconditional, unlike the state store's own readiness
// check, because the command bus carries no leader-gated one-time import
// to also gate on.
func commandBusReadinessCheck(conn *commandbusnats.Conn) healthz.Checker {
	return func(req *http.Request) error {
		return conn.Reachable(req.Context())
	}
}

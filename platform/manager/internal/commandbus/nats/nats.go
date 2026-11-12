// Package nats is the production commandbus.Bus backend (design D-1 of
// openspec/changes/async-command-bus/): JetStream KV buckets and their
// watches, no delivery stream, exactly the shape the transport spike
// (task 1, see tasks.md section 1's recorded verdict) settled on.
//
// It is independent of internal/state/nats on purpose (design D-1: "Neither
// imports the other"). Both talk to nats.go, but a shared connection or
// credential would turn a coincidence — both happen to use NATS — into a
// coupling the design explicitly rejects. commandbus/nats is the only
// package besides internal/state/nats allowed to import nats.go, enforced
// by TestOnlyTheNatsBackendImportsTheNatsClientLibrary
// (internal/state/static_test.go), extended to name both.
//
// Three buckets, prefixed by Scope so an adopter may still point both
// backends at one server without collision (design D-1):
//
//   - "tasks": every task, keyed by UID, the ledger D-2 describes. Order,
//     lease and backoff all live in this one record — the transport (the
//     bucket's own watch) only stores and wakes.
//   - "ids": CommandID (scoped by queue and kind) to the task UID currently
//     suppressing it, for Submit's dedup lookup (design D-4).
//   - "dead": a standing copy of every task that reached Dead, written the
//     instant it does (archiveDead) so DeadLetters never depends on the
//     retention sweep having run yet. The sweep (sweep.go) purges a settled
//     task's "tasks" record once it stops suppressing, but "dead" keeps its
//     own, separately capped, copy.
package nats

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// DefaultReplicas mirrors internal/state/nats's own default: a
// three-server cluster with no single point of failure, applied to every
// bucket Open bootstraps.
const DefaultReplicas = 3

// Config is everything Connect needs. Reading it from the environment
// (COMMANDBUS_NATS_URL and friends, design D-1) is the chart-wiring
// change's job (task 8), not this one — this package takes a Config value
// and never reaches into os.Getenv itself.
type Config struct {
	// URL is the client connection string, one or more comma-separated
	// server URLs.
	URL string
	// User and Password authenticate the connection when set.
	User     string
	Password string
	// Replicas is the replica count every bucket is created with. Zero
	// means DefaultReplicas.
	Replicas int
	// ConnectTimeout bounds Connect's own dial. Zero means nats.go's own
	// default.
	ConnectTimeout time.Duration
}

func (c Config) replicas() int {
	if c.Replicas > 0 {
		return c.Replicas
	}
	return DefaultReplicas
}

// Conn is a live connection to the server, the one handle Open bootstraps
// every bucket from.
type Conn struct {
	nc  *nats.Conn
	js  jetstream.JetStream
	cfg Config
}

// Connect dials the server and opens its JetStream context.
func Connect(cfg Config) (*Conn, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("commandbus/nats: Config.URL is empty")
	}
	opts := []nats.Option{
		nats.Name("agentops-manager-commandbus"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second),
	}
	if cfg.ConnectTimeout > 0 {
		opts = append(opts, nats.Timeout(cfg.ConnectTimeout))
	}
	if cfg.User != "" || cfg.Password != "" {
		opts = append(opts, nats.UserInfo(cfg.User, cfg.Password))
	}
	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("commandbus/nats: connect %q: %w", cfg.URL, err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("commandbus/nats: open jetstream: %w", err)
	}
	return &Conn{nc: nc, js: js, cfg: cfg}, nil
}

// Close drains and closes the underlying connection. Closing a nil Conn is
// not a crash — mirrors internal/state/nats.Conn.Close, same reason.
func (c *Conn) Close() {
	if c == nil || c.nc == nil {
		return
	}
	c.nc.Close()
}

// Reachable reports whether the server answers now. A production wiring
// (task 8) reports the manager not ready while this fails (design D-13).
func (c *Conn) Reachable(ctx context.Context) error {
	if _, err := c.js.AccountInfo(ctx); err != nil {
		return fmt.Errorf("commandbus/nats: server unreachable: %w", err)
	}
	return nil
}

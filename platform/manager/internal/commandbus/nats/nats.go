// Package nats is the production commandbus.Bus backend (design D-1 and D-2
// of openspec/changes/async-command-bus/): two JetStream streams of immutable
// entries and a watch-fed local mirror, exactly the shape the transport spike
// (task 1, see tasks.md section 1's recorded verdict) settled on, rebuilt
// append-only by section 14.
//
// It is independent of internal/state/nats on purpose (design D-1: "Neither
// imports the other"). Both talk to nats.go, but a shared connection or
// credential would turn a coincidence — both happen to use NATS — into a
// coupling the design explicitly rejects. commandbus/nats is the only
// package besides internal/state/nats allowed to import nats.go, enforced
// by TestOnlyTheNatsBackendImportsTheNatsClientLibrary
// (internal/state/static_test.go), extended to name both.
//
// Two streams, prefixed by Scope so an adopter may still point both backends
// at one server without collision (design D-1):
//
//   - "cmdbus_log": every queue's log. Subjects are
//     cb.<scope>.log.<queue>.<key>.msg (the message, appended once at Submit)
//     and cb.<scope>.log.<queue>.<key>.ev (a claim, a failure, a completion,
//     each a further appended entry). A task's state is folded from its own
//     entries, the last one deciding, and its position in the queue is the
//     sequence the stream gave its message. Nothing in it is ever updated. A
//     message that is consumed (succeeded) is removed with its entries, like a
//     regular queue's, after a short grace (WithSettledGrace) so a replica still
//     catching up and a Feed consumer see them first; that removal is the only
//     delete the bus makes and it never touches an unsettled entry. Everything
//     else, unconsumed and dead alike, is retained by the stream's own age
//     (seven days) and count limit. A resubmit of a consumed command id is
//     suppressed for the queue's dedup window by a short in-memory mark each
//     replica's mirror keeps, not by a retained entry.
//   - "cmdbus_dead": a standing copy of every task that reached dead, a log of
//     its own with its own limits, so DeadLetters reaches further back than the
//     queues do.
//
// An exclusive claim is an append to the key's ev subject, conditional on that
// subject's last sequence (JetStream's expected-last-subject-sequence): of two
// claims racing for one key exactly one lands, and the other is a rejected
// append, never a lost update. A submit is ONE append, so a claim landing the
// instant a task becomes visible cannot fail it (the 10071 incident that
// motivated section 14).
//
// The folding, planning and ordering rules live in commandbus/entrylog, which
// commandbus/memory runs too; this package owns only the transport.
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

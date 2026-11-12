// Package nats is the "nats" backend of the state contract (design D-1 of
// nats-state-store-default, openspec/changes/nats-state-store-default/): the
// manager's live state kept in JetStream KV buckets and one Object Store,
// behind the generic capabilities of internal/state, instead of as custom
// resources (internal/state/kube).
//
// One KV bucket per kind (D-1: conversations, runs, signals, cooldowns,
// receipts, adapterstate) keeps per-kind history, limits and replica count
// independent, and this package is the only one importing nats.go — a
// static test in internal/state (static_test.go) extends
// TestNoManagerPackageReachesAConversationExceptThroughTheStore's sibling
// check to enforce it.
//
// Connect opens the client and the JetStream context; Open, OpenExpiring,
// OpenAppended and OpenBlobs each bootstrap their own bucket and return the
// matching capability, mirroring internal/state/kube's shape with buckets
// standing in for object kinds and a KV entry's own revision standing in for
// a resourceVersion (D-2).
package nats

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// DefaultReplicas is the bucket replica count this backend asks for when
// NATS_REPLICAS is unset: a three-server cluster with no single point of
// failure (design D-8).
const DefaultReplicas = 3

// Config is everything Connect needs, read from the environment by the
// caller (cmd/manager) and never from a mounted file: the manager mounts no
// volume and reads no secret (invariants.md), so NATS_URL and the
// credentials arrive as plain env vars, chart-injected by valueFrom.
type Config struct {
	// URL is the client connection string (design D-8: NATS_URL), one or
	// more comma-separated server URLs.
	URL string
	// User and Password authenticate the connection when set. Both empty
	// means no auth, which only a local, unrestricted test server allows.
	User     string
	Password string
	// Replicas is the replica count every bucket and the object store are
	// created with (design D-8: "set at bucket creation by the manager,
	// from NATS_REPLICAS"). Zero means DefaultReplicas.
	Replicas int
	// ConnectTimeout bounds Connect's own dial. Zero means a sane default.
	ConnectTimeout time.Duration
}

// replicas resolves the configured replica count, DefaultReplicas for zero.
func (c Config) replicas() int {
	if c.Replicas > 0 {
		return c.Replicas
	}
	return DefaultReplicas
}

// ConfigFromEnv reads NATS_URL, NATS_USER, NATS_PASSWORD and NATS_REPLICAS
// (task 2.1/2.8). NATS_URL is required: a backend with nowhere to dial is a
// misconfiguration to fail fast on, named, before anything tries to connect.
func ConfigFromEnv() (Config, error) {
	url := strings.TrimSpace(os.Getenv("NATS_URL"))
	if url == "" {
		return Config{}, fmt.Errorf("nats: NATS_URL is not set")
	}
	cfg := Config{
		URL:      url,
		User:     os.Getenv("NATS_USER"),
		Password: os.Getenv("NATS_PASSWORD"),
	}
	if raw := strings.TrimSpace(os.Getenv("NATS_REPLICAS")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("nats: NATS_REPLICAS=%q is not a positive integer", raw)
		}
		cfg.Replicas = n
	}
	return cfg, nil
}

// Conn is a live connection to the server, the one handle every bucket and
// the object store open from (design D-1: "a client in internal/state/nats").
type Conn struct {
	nc  *nats.Conn
	js  jetstream.JetStream
	cfg Config
}

// Connect dials the server and opens its JetStream context. The connection
// is not scoped to an install; a Scope (open.go) prefixes every bucket name
// so one process can serve more than one install's isolation boundary, as
// kube's own Backend is scoped by namespace at construction (state-store
// design D-2).
func Connect(cfg Config) (*Conn, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, fmt.Errorf("nats: Config.URL is empty")
	}
	opts := []nats.Option{
		nats.Name("agentops-manager"),
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
		return nil, fmt.Errorf("nats: connect %q: %w", cfg.URL, err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: open jetstream: %w", err)
	}
	return &Conn{nc: nc, js: js, cfg: cfg}, nil
}

// Close drains and closes the underlying connection.
func (c *Conn) Close() {
	if c == nil || c.nc == nil {
		return
	}
	c.nc.Close()
}

// Reachable reports whether the server answers now, the readiness check task
// 2.8 wires: the manager reports not ready while this fails.
func (c *Conn) Reachable(ctx context.Context) error {
	if _, err := c.js.AccountInfo(ctx); err != nil {
		return fmt.Errorf("nats: server unreachable: %w", err)
	}
	return nil
}

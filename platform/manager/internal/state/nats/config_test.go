package nats

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

func TestConfigFromEnv(t *testing.T) {
	t.Run("url is required", func(t *testing.T) {
		t.Setenv("NATS_URL", "  ")
		if _, err := ConfigFromEnv(); err == nil {
			t.Fatal("an unset NATS_URL must be refused")
		}
	})
	t.Run("reads every variable", func(t *testing.T) {
		t.Setenv("NATS_URL", "nats://x:4222")
		t.Setenv("NATS_USER", "u")
		t.Setenv("NATS_PASSWORD", "p")
		t.Setenv("NATS_REPLICAS", "1")
		cfg, err := ConfigFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.URL != "nats://x:4222" || cfg.User != "u" || cfg.Password != "p" || cfg.Replicas != 1 {
			t.Fatalf("unexpected config %+v", cfg)
		}
	})
	t.Run("replicas default when unset", func(t *testing.T) {
		t.Setenv("NATS_URL", "nats://x:4222")
		t.Setenv("NATS_REPLICAS", "")
		cfg, err := ConfigFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Replicas != 0 || cfg.replicas() != DefaultReplicas {
			t.Fatalf("unset replicas must resolve to the default, got %+v", cfg)
		}
	})
	for _, bad := range []string{"abc", "0", "-2"} {
		t.Run("rejects replicas "+bad, func(t *testing.T) {
			t.Setenv("NATS_URL", "nats://x:4222")
			t.Setenv("NATS_REPLICAS", bad)
			if _, err := ConfigFromEnv(); err == nil {
				t.Fatalf("NATS_REPLICAS=%q must be refused", bad)
			}
		})
	}
}

func TestConnectRefusesAnEmptyURL(t *testing.T) {
	if _, err := Connect(Config{URL: " "}); err == nil {
		t.Fatal("an empty URL must be refused before any dial")
	}
}

func TestParseRevision(t *testing.T) {
	if got, err := parseRevision(formatRevision(42)); err != nil || got != 42 {
		t.Fatalf("round trip: %d %v", got, err)
	}
	if _, err := parseRevision(state.Revision("not-a-number")); err == nil {
		t.Fatal("a revision this backend never minted must be refused")
	}
}

func TestConnReachableThenClosed(t *testing.T) {
	conn, err := Connect(Config{URL: testServer.ClientURL(), Replicas: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Reachable(context.Background()); err != nil {
		t.Fatalf("a live server must answer: %v", err)
	}
	conn.Close()
	if err := conn.Reachable(context.Background()); err == nil {
		t.Fatal("a closed connection must report the server unreachable")
	}
	var nilConn *Conn
	nilConn.Close() // must not panic
}

func TestErrNotExpiringNamesTheKind(t *testing.T) {
	if err := errNotExpiring("widgets"); err == nil || err.Error() == "" {
		t.Fatal("errNotExpiring must return a descriptive error")
	}
}

func TestConflictOr(t *testing.T) {
	c := &collection[struct{}]{}
	if err := c.conflictOr(jetstream.ErrKeyExists, "op", "k"); !errors.Is(err, state.ErrConflict) {
		t.Fatalf("a lost compare-and-set must map to ErrConflict, got %v", err)
	}
	cause := errors.New("boom")
	err := c.conflictOr(cause, "op", "k")
	if errors.Is(err, state.ErrConflict) || !errors.Is(err, cause) {
		t.Fatalf("any other failure must be wrapped, not mapped: %v", err)
	}
}

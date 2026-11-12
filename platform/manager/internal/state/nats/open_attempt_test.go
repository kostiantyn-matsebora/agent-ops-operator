package nats

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// A JetStream API request that gets no reply waits for its whole context. A
// server still forming its cluster drops requests that arrive before it can
// place a stream, so an attempt handed the entire startup budget could use all
// of it on one lost request and never retry. Measured on a real install: the
// buckets existed, healthy, and the manager had still given up on a single
// attempt that never heard back.

// blackHole is a NATS server that accepts clients and answers pings, and never
// answers anything else. It counts the JetStream API requests it was sent.
type blackHole struct {
	ln   net.Listener
	mu   sync.Mutex
	subj []string
}

func newBlackHole(t *testing.T) *blackHole {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &blackHole{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go b.serve(c)
		}
	}()
	return b
}

func (b *blackHole) url() string { return "nats://" + b.ln.Addr().String() }

func (b *blackHole) apiRequests() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, s := range b.subj {
		if strings.HasPrefix(s, "$JS.API.") {
			n++
		}
	}
	return n
}

func (b *blackHole) serve(c net.Conn) {
	defer c.Close()
	fmt.Fprint(c, `INFO {"server_id":"x","server_name":"x","version":"2.10.29","proto":1,"headers":true,"max_payload":1048576,"jetstream":true}`+"\r\n")
	r := bufio.NewReader(c)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "PING":
			fmt.Fprint(c, "PONG\r\n")
		case "PUB", "HPUB":
			b.mu.Lock()
			b.subj = append(b.subj, f[1])
			b.mu.Unlock()
			size, _ := strconv.Atoi(f[len(f)-1])
			if _, err := r.Discard(size + 2); err != nil {
				return
			}
		}
	}
}

func TestEveryOpenAttemptIsBoundedSoALostRequestIsRetried(t *testing.T) {
	srv := newBlackHole(t)
	conn, err := Connect(Config{URL: srv.url(), Replicas: 1, ConnectTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(conn.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = OpenStoresWhenReady(ctx, conn, newScope(t), 20*time.Millisecond, WithOpenAttemptTimeout(300*time.Millisecond))
	if err == nil {
		t.Fatal("opened against a server that answers nothing")
	}
	if got := srv.apiRequests(); got < 3 {
		t.Fatalf("the server saw %d API requests in 3s, want at least 3: one lost request used the whole budget", got)
	}
}

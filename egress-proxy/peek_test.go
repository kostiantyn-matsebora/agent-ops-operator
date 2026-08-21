package main

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"
)

// THE BUG A LIVE CALL FOUND, and it stalled everything.
//
// bufio.Peek(n) blocks until n bytes arrive. A fixed window therefore waits for
// bytes an HTTP request never sends, so the proxy held every connection open
// until the client timed out — including the work contract, which meant the
// policy was never learned and every tool was then refused against an empty
// allowlist. It presents as an agent whose tools all stopped working.
func TestPeekDoesNotWaitForBytesThatNeverCome(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	req := "POST /mcp HTTP/1.1\r\nHost: agentops-mcp-k8s.agent-ops.svc:8080\r\n" +
		"Content-Type: application/json\r\n\r\n{}"
	go func() { _, _ = client.Write([]byte(req)) }() // ...then waits for a reply, sending nothing more

	done := make(chan string, 1)
	go func() { done <- peekHost(bufio.NewReader(server)) }()

	select {
	case got := <-done:
		if got != "agentops-mcp-k8s.agent-ops.svc:8080" {
			t.Fatalf("Host = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("peekHost blocked waiting for bytes the client will never send")
	}
}

// The peek must leave the request intact for whoever serves the connection.
func TestPeekConsumesNothing(t *testing.T) {
	req := "GET /work?convo=c1 HTTP/1.1\r\nHost: agentops-manager.agent-ops.svc:8080\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(req))

	if got := peekHost(br); got != "agentops-manager.agent-ops.svc:8080" {
		t.Fatalf("Host = %q", got)
	}
	rest, _ := br.Peek(len(req))
	if string(rest) != req {
		t.Fatal("the peek consumed part of the request")
	}
}

// A connection that is not HTTP has no Host, and must be carried rather than
// waited on.
func TestNonHTTPHasNoHost(t *testing.T) {
	br := bufio.NewReader(strings.NewReader("\x16\x03\x01\x02\x00binary-tls-hello"))
	if got := peekHost(br); got != "" {
		t.Fatalf("want no host for non-HTTP, got %q", got)
	}
}

// A Host-looking line in the BODY must not be mistaken for the header.
func TestBodyCannotSupplyTheHost(t *testing.T) {
	req := "POST /mcp HTTP/1.1\r\nHost: real.svc:8080\r\n\r\n{\"x\":\"Host: fake.svc:9999\"}"
	if got := peekHost(bufio.NewReader(strings.NewReader(req))); got != "real.svc:8080" {
		t.Fatalf("Host = %q, want the header not the body", got)
	}
}

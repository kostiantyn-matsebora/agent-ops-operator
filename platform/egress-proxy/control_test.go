package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// managerSide answers one /work long-poll with the given unit, and records what
// it received so the test can prove the stream was forwarded unmodified.
func managerSide(t *testing.T, unit string) (net.Conn, chan string) {
	t.Helper()
	srv, up := pair(t)
	got := make(chan string, 4)
	go func() {
		br := bufio.NewReader(srv)
		for {
			req, err := http.ReadRequest(br)
			if err != nil {
				return
			}
			got <- req.URL.Path
			resp := &http.Response{
				StatusCode: 200, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
				Request:       req,
				Header:        http.Header{"Content-Type": []string{"application/json"}},
				Body:          io.NopCloser(strings.NewReader(unit)),
				ContentLength: int64(len(unit)),
			}
			_ = resp.Write(srv)
		}
	}()
	return up, got
}

// Task 2.7 — the decision arrives on the work unit, with no API call and no
// credential. This is the mechanism the whole design rests on.
func TestPolicyIsLearnedFromTheWorkUnit(t *testing.T) {
	unit := `{"id":"u1","convo":"c1","allowedTools":"mcp__kubernetes__pods_list,Bash","toolsMode":"merge"}`
	client, proxySide := pair(t)
	upstream, seen := managerSide(t, unit)

	state := newPolicy()
	go serveControl(bufio.NewReader(proxySide), proxySide, upstream, state)

	req, _ := http.NewRequest("GET", "http://manager:8080/work?convo=c1", nil)
	go func() { _ = req.Write(client) }()
	resp, err := http.ReadResponse(bufio.NewReader(client), req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)

	if string(body) != unit {
		t.Fatalf("the work unit must reach the runtime unmodified:\n got %s\nwant %s", body, unit)
	}
	if p := <-seen; p != "/work" {
		t.Fatalf("forwarded path = %q", p)
	}
	if !state.ready() {
		t.Fatal("the proxy must have learned the decision from the unit it just forwarded")
	}
	if !state.permits("mcp__kubernetes__pods_list") {
		t.Fatal("the granted tool must be permitted after the unit")
	}
	if state.permits("mcp__kubernetes__pods_delete") {
		t.Fatal("only what the unit granted may be permitted")
	}
}

// The unit's allowedTools is the WIRING's half. The agent definition's own
// tools: frontmatter lives in a repo the agent can write to, so it must never
// widen what is enforced. This pins that the proxy reads one field and not the
// composed result.
func TestOnlyTheWiringHalfIsEnforced(t *testing.T) {
	state := newPolicy()
	learn([]byte(`{"allowedTools":"mcp__kubernetes__pods_list","toolsMode":"merge"}`), state)

	if state.permits("mcp__kubernetes__pods_delete") {
		t.Fatal("merge mode must not be read as widening what this proxy enforces")
	}
}

// /work/done and /work/context report RESULTS and carry no grant. Reading one
// as a decision would let a report change what is enforced.
func TestOnlyTheWorkPathCarriesADecision(t *testing.T) {
	for _, p := range []string{"/work/done", "/work/context", "/healthz"} {
		if isWorkPath(p) {
			t.Fatalf("%q must not be read as a work unit", p)
		}
	}
	if !isWorkPath("/work") {
		t.Fatal("/work is the one path that carries a decision")
	}
}

// A malformed work unit must not touch the policy at all — a corrupt body
// silently clearing the allowlist would be worse than leaving the previous
// grant in place. Every other learn() test uses a well-formed body, so the
// json.Unmarshal error return was never taken.
func TestMalformedWorkUnitLeavesThePolicyUntouched(t *testing.T) {
	state := newPolicy()
	state.set([]string{"mcp__kubernetes__pods_list"})
	learn([]byte("not json at all"), state)
	if !state.permits("mcp__kubernetes__pods_list") {
		t.Fatal("a malformed unit must not clear or alter what was already granted")
	}
}

// serveControl's non-keepalive exit was never taken — every other test sends
// one request and lets the goroutine idle rather than ending the loop.
func TestServeControlStopsOnConnectionClose(t *testing.T) {
	unit := `{"id":"u1","allowedTools":"mcp__kubernetes__pods_list"}`
	client, proxySide := pair(t)
	upstream, seen := managerSide(t, unit)

	state := newPolicy()
	go serveControl(bufio.NewReader(proxySide), proxySide, upstream, state)

	req, _ := http.NewRequest("GET", "http://manager:8080/work?convo=c1", nil)
	req.Header.Set("Connection", "close")
	go func() { _ = req.Write(client) }()
	resp, err := http.ReadResponse(bufio.NewReader(client), req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	if p := <-seen; p != "/work" {
		t.Fatalf("forwarded path = %q", p)
	}

	// serveControl must have returned rather than looped again: a second
	// request must never reach the manager side.
	select {
	case p := <-seen:
		t.Fatalf("a second request reached the manager after Connection: close: %q", p)
	case <-time.After(200 * time.Millisecond):
	}
}

// Task 2.2 — the common case must stay boring. Everything that is not MCP or
// the work contract is copied through byte for byte.
func TestNonMCPTrafficIsCopiedUnchanged(t *testing.T) {
	client, proxySide := pair(t)
	upstream, echo := pair(t)

	payload := strings.Repeat("opaque-bytes-", 4096)
	go pipe(proxySide, upstream)
	go func() {
		buf, _ := io.ReadAll(io.LimitReader(echo, int64(len(payload))))
		_, _ = echo.Write(buf)
	}()

	go func() { _, _ = client.Write([]byte(payload)) }()
	back := make([]byte, len(payload))
	if _, err := io.ReadFull(client, back); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, []byte(payload)) {
		t.Fatal("bytes were altered in transit; this path carries git and the LLM API")
	}
}

package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// envOr had no test at all: the config it reads (LISTEN_PORT) is the one
// value runProxy trusts without validation beyond strconv.Atoi.
func TestEnvOrFallsBackOnlyWhenUnset(t *testing.T) {
	os.Unsetenv("EGRESS_TEST_ENVOR")
	if got := envOr("EGRESS_TEST_ENVOR", "fallback"); got != "fallback" {
		t.Fatalf("got %q, want the fallback", got)
	}
	t.Setenv("EGRESS_TEST_ENVOR", "set-value")
	if got := envOr("EGRESS_TEST_ENVOR", "fallback"); got != "set-value" {
		t.Fatalf("got %q, want the set value", got)
	}
}

// runProxy's own argument/environment handling was entirely untested — every
// serveMCP/serveControl test calls those functions directly, never through
// the command entry point. Both of these return before `serve` is ever
// called, so neither risks blocking on the Accept loop.
func TestRunProxyRejectsAnUnknownFlag(t *testing.T) {
	if err := runProxy([]string{"--not-a-real-flag"}); err == nil {
		t.Fatal("an unrecognised flag must be refused")
	}
}

func TestRunProxyFailsOnAMalformedListenPort(t *testing.T) {
	t.Setenv("LISTEN_PORT", "not-a-number")
	if err := runProxy(nil); err == nil {
		t.Fatal("a malformed LISTEN_PORT must fail runProxy before anything binds")
	}
}

// route's own dispatch switch was 0% — every existing test drives serveMCP,
// serveControl or pipeBuffered's sibling `pipe` directly. These three prove
// route actually picks among them based on what classifyBy reports.
func TestRouteDispatchesMCPTraffic(t *testing.T) {
	e := &endpoints{spec: []entry{{key: "kubernetes", host: "127.0.0.1", port: "8080"}}}
	e.resolve()
	cfg := proxyConfig{endpoints: e}
	state := newPolicy()
	state.set([]string{"mcp__kubernetes__pods_list"})

	client, proxySide := pair(t)
	upstream, seen := mcpServer(t, `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`)

	go route(bufio.NewReader(proxySide), proxySide, upstream, "127.0.0.1:8080", cfg, state)
	postTo(client, toolCall("pods_list"))

	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("route did not hand an MCP-classified connection to serveMCP: %s", body)
	}
	select {
	case <-seen:
	default:
		t.Fatal("the call never reached the upstream MCP server")
	}
}

func TestRouteDispatchesControlTraffic(t *testing.T) {
	e := newEndpoints("", "http://127.0.0.1:8081")
	cfg := proxyConfig{endpoints: e}
	state := newPolicy()

	client, proxySide := pair(t)
	upstream, seen := managerSide(t, `{"id":"u1","allowedTools":"mcp__kubernetes__pods_list"}`)

	go route(bufio.NewReader(proxySide), proxySide, upstream, "127.0.0.1:8081", cfg, state)

	req, _ := http.NewRequest("GET", "http://manager:8080/work?convo=c1", nil)
	go func() { _ = req.Write(client) }()
	resp, err := http.ReadResponse(bufio.NewReader(client), req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	if p := <-seen; p != "/work" {
		t.Fatalf("forwarded path = %q", p)
	}
	if !state.ready() {
		t.Fatal("route must hand a control-classified connection to serveControl, which learns the policy")
	}
}

func TestRouteDispatchesEverythingElseAsOpaque(t *testing.T) {
	e := &endpoints{spec: []entry{{key: "kubernetes", host: "127.0.0.1", port: "8080"}}}
	e.resolve()
	cfg := proxyConfig{endpoints: e}
	state := newPolicy()

	client, proxySide := pair(t)
	upstream, dest := pair(t)

	go route(bufio.NewReader(proxySide), proxySide, upstream, "203.0.113.9:443", cfg, state)

	go func() { _, _ = client.Write([]byte("opaque-payload")) }()
	buf := make([]byte, len("opaque-payload"))
	if _, err := io.ReadFull(dest, buf); err != nil {
		t.Fatalf("an unmatched destination must still be carried through: %v", err)
	}
	if string(buf) != "opaque-payload" {
		t.Fatalf("bytes were altered: %q", buf)
	}
}

// pipeBuffered backs route's default (opaque) path and was never called
// directly — the only other full-duplex test exercises `pipe`, whose
// CloseWrite branch is a coincidence of net.Pipe not implementing it at all.
// Real TCP connections here so the half-close is a genuine FIN, not a skipped
// type assertion.
func TestPipeBufferedCopiesBothDirectionsAndPropagatesHalfClose(t *testing.T) {
	agentLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer agentLn.Close()
	destLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer destLn.Close()

	proxyClientCh := make(chan net.Conn, 1)
	go func() { c, _ := agentLn.Accept(); proxyClientCh <- c }()
	agentConn, err := net.Dial("tcp", agentLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer agentConn.Close()
	proxyClientConn := <-proxyClientCh
	defer proxyClientConn.Close()

	upstreamCh := make(chan net.Conn, 1)
	go func() { c, _ := destLn.Accept(); upstreamCh <- c }()
	upstreamConn, err := net.Dial("tcp", destLn.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	destConn := <-upstreamCh
	defer destConn.Close()

	done := make(chan struct{})
	go func() {
		pipeBuffered(bufio.NewReader(proxyClientConn), proxyClientConn, upstreamConn)
		close(done)
	}()

	if _, err := agentConn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(destConn, buf); err != nil {
		t.Fatalf("agent -> upstream: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("got %q, want ping", buf)
	}

	if _, err := destConn.Write([]byte("pong")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(agentConn, buf); err != nil {
		t.Fatalf("upstream -> agent: %v", err)
	}
	if string(buf) != "pong" {
		t.Fatalf("got %q, want pong", buf)
	}

	// The agent is done sending. pipeBuffered must propagate that as a real
	// half-close to upstream, not merely stop copying silently.
	if err := agentConn.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	n, err := destConn.Read(buf)
	if n != 0 || err != io.EOF {
		t.Fatalf("half-close was not propagated to upstream: n=%d err=%v", n, err)
	}

	destConn.Close()
	<-done
}

// serve()'s own Accept loop was 0%. The error return is deterministic; the
// success path is proven by actually dialing in and observing handle() do
// its real job (failing closed on an unmediated connection).
func TestServeFailsWhenTheListenPortIsInvalid(t *testing.T) {
	if err := serve(proxyConfig{listenPort: -1}); err == nil {
		t.Fatal("an invalid listen port must fail serve rather than panic or hang")
	}
}

func TestServeAcceptsConnectionsAndDispatchesThemToHandle(t *testing.T) {
	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserve.Addr().(*net.TCPAddr).Port
	reserve.Close()

	cfg := proxyConfig{listenPort: port, endpoints: newEndpoints("", "")}
	errCh := make(chan error, 1)
	go func() { errCh <- serve(cfg) }()

	var conn net.Conn
	for i := 0; i < 50; i++ {
		conn, err = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("serve never started listening: %v", err)
	}
	defer conn.Close()

	// handle() cannot recover an original destination from a plain dial (it
	// is not a redirected connection) and must close it — observed here as
	// EOF, which is the proof serve() actually dispatched to handle.
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err != io.EOF {
		t.Fatalf("expected the unmediated connection to be dropped, got %v", err)
	}
	select {
	case err := <-errCh:
		t.Fatalf("serve returned unexpectedly: %v", err)
	default:
		// serve()'s Accept loop is intentionally left running: production
		// never stops it either, and a goroutine blocked in accept(2) costs
		// nothing for the rest of the test binary's life.
	}
}

// The other branch originalDestination has that handle()'s real path above
// does not reach: a connection that is not even a *net.TCPConn.
func TestOriginalDestinationRejectsNonTCPConnections(t *testing.T) {
	client, _ := pair(t) // net.Pipe, deliberately not a *net.TCPConn
	if _, err := originalDestination(client); err == nil {
		t.Fatal("a non-TCP connection must not silently produce a destination")
	}
}

// isSelfConnection is the guard against the self-connection loop found on
// CI: some kernels return a plain, non-redirected connection's own local
// address from SO_ORIGINAL_DST instead of erroring, which would otherwise
// have handle() dial itself, get re-accepted, and dial itself again without
// bound until the process runs out of OS threads.
func TestIsSelfConnectionDetectsTheProxysOwnAddress(t *testing.T) {
	if !isSelfConnection("127.0.0.1:15001", "127.0.0.1:15001") {
		t.Fatal("a destination equal to the listener's own address must be detected as a self-connection")
	}
	if isSelfConnection("192.0.2.5:443", "127.0.0.1:15001") {
		t.Fatal("a genuinely different destination must not be flagged as a self-connection")
	}
}

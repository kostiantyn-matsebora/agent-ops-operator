package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// pair returns two connected halves, so a test can drive the proxy over a real
// socket without needing a redirect.
func pair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	c1, c2 := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })
	return c1, c2
}

// mcpServer answers one request with the given body, recording what it saw.
// A nil channel entry means the server was never reached, which is what a
// refusal must look like from the server's side.
func mcpServer(t *testing.T, reply string) (net.Conn, chan *http.Request) {
	t.Helper()
	srv, up := pair(t)
	seen := make(chan *http.Request, 4)
	go func() {
		br := bufio.NewReader(srv)
		for {
			req, err := http.ReadRequest(br)
			if err != nil {
				return
			}
			body, _ := io.ReadAll(req.Body)
			req.Body = io.NopCloser(bytes.NewReader(body))
			seen <- req
			resp := &http.Response{
				StatusCode: 200, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
				Request: req,
				Header:  http.Header{"Content-Type": []string{"application/json"}},
				Body:    io.NopCloser(strings.NewReader(reply)),
				ContentLength: int64(len(reply)),
			}
			_ = resp.Write(srv)
		}
	}()
	return up, seen
}

func toolCall(name string) string {
	return `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"` + name + `"}}`
}

func postTo(conn net.Conn, body string) {
	req, _ := http.NewRequest("POST", "http://mcp/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	go func() { _ = req.Write(conn) }()
}

// Task 2.5 — the core of the change. An ungranted call must not reach the
// server, and the agent must be told why in its own protocol.
func TestUngrantedCallIsRefusedBeforeTheServerSeesIt(t *testing.T) {
	client, proxySide := pair(t)
	upstream, seen := mcpServer(t, `{"jsonrpc":"2.0","id":7,"result":{}}`)

	state := newPolicy()
	state.set([]string{"mcp__kubernetes__pods_list"})
	go serveMCP(bufio.NewReader(proxySide), proxySide, upstream, "kubernetes", state)

	postTo(client, toolCall("pods_delete"))
	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatalf("no answer to the agent: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)

	var msg struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &msg) != nil || msg.Error == nil {
		t.Fatalf("want an MCP error, got %s", body)
	}
	if !strings.Contains(msg.Error.Message, "mcp__kubernetes__pods_delete") {
		t.Fatalf("the refusal must name the tool: %q", msg.Error.Message)
	}
	select {
	case req := <-seen:
		t.Fatalf("the refused call reached the server: %s", req.URL)
	default:
	}
}

// The other half: a granted call must be ordinary. An enforcement layer that
// breaks the permitted path is worse than none.
func TestGrantedCallReachesTheServerUnchanged(t *testing.T) {
	client, proxySide := pair(t)
	upstream, seen := mcpServer(t, `{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`)

	state := newPolicy()
	state.set([]string{"mcp__kubernetes__*"})
	go serveMCP(bufio.NewReader(proxySide), proxySide, upstream, "kubernetes", state)

	postTo(client, toolCall("pods_list"))
	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"ok":true`) {
		t.Fatalf("the server's answer must reach the agent unmodified: %s", body)
	}
	select {
	case req := <-seen:
		if req.Method != "POST" {
			t.Fatalf("method = %q", req.Method)
		}
	default:
		t.Fatal("the granted call never reached the server")
	}
}

// The refusal for "nothing dispatched yet" must be distinguishable from
// "your wiring does not grant this" — they send an operator to different files.
func TestRefusalBeforeAnyWorkUnitSaysSo(t *testing.T) {
	client, proxySide := pair(t)
	upstream, _ := mcpServer(t, `{}`)
	go serveMCP(bufio.NewReader(proxySide), proxySide, upstream, "kubernetes", newPolicy())

	postTo(client, toolCall("pods_list"))
	resp, _ := http.ReadResponse(bufio.NewReader(client), nil)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "no work unit") {
		t.Fatalf("the initial closed state must say what it is: %s", body)
	}
}

// Task 2.6 — discovery and invocation share one predicate, so an agent is never
// shown a tool that would be refused.
func TestListingIsFilteredToWhatIsCallable(t *testing.T) {
	listing := `{"jsonrpc":"2.0","id":1,"result":{"tools":[` +
		`{"name":"pods_list"},{"name":"pods_delete"},{"name":"pods_exec"}]}}`
	client, proxySide := pair(t)
	upstream, _ := mcpServer(t, listing)

	state := newPolicy()
	state.set([]string{"mcp__kubernetes__pods_list"})
	go serveMCP(bufio.NewReader(proxySide), proxySide, upstream, "kubernetes", state)

	postTo(client, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	if !strings.Contains(got, "pods_list") {
		t.Fatalf("the granted tool must still be advertised: %s", got)
	}
	for _, hidden := range []string{"pods_delete", "pods_exec"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("%q was advertised but would be refused: %s", hidden, got)
		}
	}
}

// Task 2.10 — a mistyped pattern grants nothing, and that must be legible.
func TestGrantingNothingIsDetectable(t *testing.T) {
	listing := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"pods_list"}]}}`
	state := newPolicy()
	state.set([]string{"mcp__kubernets__*"}) // the typo an operator actually makes

	out, changed := filterListing([]byte(listing), "kubernetes", state)
	if !changed {
		t.Fatal("a listing that grants nothing must be rewritten, not passed through")
	}
	if strings.Contains(string(out), "pods_list") {
		t.Fatalf("nothing should survive the filter: %s", out)
	}
}

// A batch is judged as a whole. Answering half of one is a shape no client asked
// for.
func TestBatchWithAnUngrantedCallIsRefused(t *testing.T) {
	client, proxySide := pair(t)
	upstream, seen := mcpServer(t, `{}`)

	state := newPolicy()
	state.set([]string{"mcp__kubernetes__pods_list"})
	go serveMCP(bufio.NewReader(proxySide), proxySide, upstream, "kubernetes", state)

	postTo(client, `[`+toolCall("pods_list")+`,`+toolCall("pods_delete")+`]`)
	resp, _ := http.ReadResponse(bufio.NewReader(client), nil)
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "pods_delete") {
		t.Fatalf("the batch must be refused naming the ungranted member: %s", body)
	}
	select {
	case <-seen:
		t.Fatal("no part of a refused batch may reach the server")
	default:
	}
}

// Anything that is not a tools/call passes untouched — initialize, ping,
// resources, prompts. Enforcement is about tool invocation, not about being in
// the way.
func TestNonCallMessagesArePassedThrough(t *testing.T) {
	client, proxySide := pair(t)
	upstream, seen := mcpServer(t, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"x"}}`)
	go serveMCP(bufio.NewReader(proxySide), proxySide, upstream, "kubernetes", newPolicy()) // nothing granted

	postTo(client, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	resp, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "protocolVersion") {
		t.Fatalf("initialize must reach the server even with nothing granted: %s", body)
	}
	select {
	case <-seen:
	default:
		t.Fatal("initialize never reached the server")
	}
}

package main

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

// Task 8.1 — the whole point of the change, end to end.
//
// The scenario is the real one: a pipeline binds READ tools only, and the agent
// has a shell. It does not use its mcp.json, it does not use the CLI. It opens
// a socket to the MCP server directly and calls a mutating tool, exactly as
// `curl` in a runtime pod would.
//
// Before this change that call succeeded, and the server acted on it.
func TestShellCapableAgentCannotEscapeItsToolset(t *testing.T) {
	// The MCP server: it would happily do whatever it is asked.
	server, reached := mcpServer(t, `{"jsonrpc":"2.0","id":1,"result":{"deleted":true}}`)

	// The work unit the manager dispatched. Read tools only.
	state := newPolicy()
	learn([]byte(`{"allowedTools":"mcp__kubernetes__pods_list,mcp__kubernetes__pods_get"}`), state)

	agent, proxySide := pair(t)
	go serveMCP(bufio.NewReader(proxySide), proxySide, server, "kubernetes", state)

	// The agent, bypassing every piece of its own configuration.
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"pods_delete",` +
		`"arguments":{"name":"the-database","namespace":"prod"}}}`
	req, _ := http.NewRequest("POST", "http://agentops-mcp-k8s:8080/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = int64(len(body))
	go func() { _ = req.Write(agent) }()

	resp, err := http.ReadResponse(bufio.NewReader(agent), req)
	if err != nil {
		t.Fatalf("the agent got no answer at all: %v", err)
	}
	answer, _ := io.ReadAll(resp.Body)

	select {
	case got := <-reached:
		t.Fatalf("THE CALL REACHED THE SERVER (%s). The toolset is still advisory.", got.URL)
	default:
	}
	if !strings.Contains(string(answer), "not granted") {
		t.Fatalf("the agent must be told why, in MCP terms, got: %s", answer)
	}
	if strings.Contains(string(answer), "deleted") {
		t.Fatal("the server's answer reached the agent, so the call was executed")
	}
}

// The same agent, calling something its wiring DID grant, must be unaffected.
// An enforcement layer that breaks the permitted path is worse than none.
func TestTheSameAgentKeepsWhatItWasGranted(t *testing.T) {
	server, reached := mcpServer(t, `{"jsonrpc":"2.0","id":1,"result":{"items":[]}}`)
	state := newPolicy()
	learn([]byte(`{"allowedTools":"mcp__kubernetes__pods_list"}`), state)

	agent, proxySide := pair(t)
	go serveMCP(bufio.NewReader(proxySide), proxySide, server, "kubernetes", state)
	postTo(agent, toolCall("pods_list"))

	resp, err := http.ReadResponse(bufio.NewReader(agent), nil)
	if err != nil {
		t.Fatal(err)
	}
	answer, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(answer), "items") {
		t.Fatalf("a granted call must work exactly as before: %s", answer)
	}
	select {
	case <-reached:
	default:
		t.Fatal("the granted call never reached the server")
	}
}

// And the agent's other traffic — the LLM API, git — is untouched. This is the
// path that carries everything the agent does that is not MCP, so its
// correctness matters more than the enforcement itself.
func TestTheSameAgentsOtherTrafficIsUntouched(t *testing.T) {
	agent, proxySide := pair(t)
	remote, echo := pair(t)
	go pipe(proxySide, remote)

	go func() {
		buf := make([]byte, 11)
		if _, err := io.ReadFull(echo, buf); err != nil {
			return
		}
		_, _ = echo.Write([]byte("HTTP/1.1 200"))
	}()

	go func() { _, _ = agent.Write([]byte("GET / HTTP")) }()
	_ = agent.(net.Conn)
	go func() { _, _ = agent.Write([]byte("/")) }()

	buf := make([]byte, 12)
	if _, err := io.ReadFull(agent, buf); err != nil {
		t.Fatalf("opaque traffic did not survive the proxy: %v", err)
	}
	if string(buf) != "HTTP/1.1 200" {
		t.Fatalf("bytes were altered: %q", buf)
	}
}

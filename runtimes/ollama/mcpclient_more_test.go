package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// readMCPConfig was entirely untested (0%): a missing file is "no servers",
// not an error; a present file is parsed; malformed JSON is a reported error.

func TestReadMCPConfigMissingFileIsNoServers(t *testing.T) {
	cfg, err := readMCPConfig(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil || len(cfg.Servers) != 0 {
		t.Errorf("cfg=%+v err=%v", cfg, err)
	}
}

func TestReadMCPConfigParsesServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(path, []byte(`{"mcpServers":{"a":{"command":"foo","args":["x"]}}}`), 0o644)
	cfg, err := readMCPConfig(path)
	if err != nil || len(cfg.Servers) != 1 || cfg.Servers["a"].Command != "foo" {
		t.Errorf("cfg=%+v err=%v", cfg, err)
	}
}

func TestReadMCPConfigMalformedJSONIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	os.WriteFile(path, []byte("not json"), 0o644)
	if _, err := readMCPConfig(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("want an error naming the path, got %v", err)
	}
}

// Connect over a REAL streamable-HTTP transport, against the official SDK's
// own server-side handler served by httptest -- no mocking of the protocol.
// This exercises the success-logging branch of Connect (never reached by the
// existing dead-server test, which only exercises the failure path), the URL
// transport construction in connectOne, ListTools end to end via the real
// session, a real tool call round-trip, and Close() on a live session.
func TestConnectListToolsCallAndCloseOverRealHTTPTransport(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "http-probe", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "ping", Description: "pings"},
		func(_ context.Context, _ *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil, nil
		})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	httpSrv := httptest.NewServer(handler)
	defer httpSrv.Close()

	var logs []string
	logf := func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }
	m := &MCPClient{CallTimeout: 5 * time.Second}
	m.Connect(context.Background(), mcpConfig{Servers: map[string]mcpServer{"http": {URL: httpSrv.URL}}}, logf)

	connected := false
	for _, l := range logs {
		if strings.Contains(l, "http: connected") {
			connected = true
		}
	}
	if !connected {
		t.Fatalf("want a real connection logged, got %v", logs)
	}

	r := NewRegistry()
	m.ListTools(context.Background(), r, logf)
	tool, ok := r.Get("mcp__http__ping")
	if !ok {
		t.Fatalf("tool not registered from a real tools/list: %v", r.order)
	}
	text, isErr, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil || isErr || text != "pong" {
		t.Errorf("real round trip: text=%q isErr=%v err=%v", text, isErr, err)
	}

	// Close must not panic, and must actually end the session: a call made
	// directly against the closed session afterwards fails rather than
	// silently succeeding.
	session := m.sessions["http"]
	m.Close()
	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "ping"}); err == nil {
		t.Error("a call against a closed session must fail")
	}
}

// m.call's content-rendering loop has two branches TestMCPToolsAreRegisteredAndCalled
// does not reach: a non-text Content item (marshalled as JSON) and an empty
// Content list falling back to StructuredContent.
func TestCallRendersNonTextContentAndStructuredContentFallback(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "probe2", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "image"}, func(_ context.Context, _ *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: []byte("x"), MIMEType: "image/png"}}}, nil, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "structured"}, func(_ context.Context, _ *mcp.CallToolRequest, in struct{}) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}, nil, nil
	})
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	session, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	m := &MCPClient{CallTimeout: 5 * time.Second}

	text, isErr, err := m.call(ctx, session, "image", json.RawMessage(`{}`))
	if err != nil || isErr || !strings.Contains(text, "image/png") {
		t.Errorf("non-text content must be rendered as JSON: %q %v %v", text, isErr, err)
	}

	text, isErr, err = m.call(ctx, session, "structured", json.RawMessage(`{}`))
	if err != nil || isErr || !strings.Contains(text, `"ok":true`) {
		t.Errorf("empty Content must fall back to StructuredContent: %q %v %v", text, isErr, err)
	}
}

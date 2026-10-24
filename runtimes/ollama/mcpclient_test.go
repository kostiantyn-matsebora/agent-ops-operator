package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type echoIn struct {
	Text string `json:"text"`
}

func TestMCPToolsAreRegisteredAndCalled(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "probe", Version: "0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "echo", Description: "echoes"}, func(_ context.Context, _ *mcp.CallToolRequest, in echoIn) (*mcp.CallToolResult, any, error) {
		if in.Text == "boom" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "refused"}}}, nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + in.Text}}}, nil, nil
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

	// Drive the registration path the runtime uses, with the session already
	// connected (the transport is the only thing the in-memory pair differs in).
	m := &MCPClient{CallTimeout: 5 * time.Second, sessions: map[string]*mcp.ClientSession{}}
	r := NewRegistry()
	var n int
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		schema, _ := json.Marshal(tool.InputSchema)
		name := tool.Name
		r.Add(Tool{Name: "mcp__probe__" + name, Description: tool.Description, Schema: schema,
			Call: func(ctx context.Context, args json.RawMessage) (string, bool, error) {
				return m.call(ctx, session, name, args)
			}})
		n++
	}
	if n != 1 {
		t.Fatalf("tools/list: %d", n)
	}
	tool, _ := r.Get("mcp__probe__echo")
	if !strings.Contains(string(tool.Schema), `"text"`) {
		t.Errorf("schema must reach the model: %s", tool.Schema)
	}
	text, isErr, err := tool.Call(ctx, json.RawMessage(`{"text":"hi"}`))
	if err != nil || isErr || text != "echo: hi" {
		t.Errorf("%q %v %v", text, isErr, err)
	}
	if text, isErr, _ := tool.Call(ctx, json.RawMessage(`{"text":"boom"}`)); !isErr || text != "refused" {
		t.Errorf("tool error must be passed through: %q %v", text, isErr)
	}
	if text, isErr, _ := tool.Call(ctx, json.RawMessage(`[1]`)); !isErr || !strings.Contains(text, "not a JSON object") {
		t.Errorf("malformed: %q %v", text, isErr)
	}
}

func TestUnreachableServerIsSkipped(t *testing.T) {
	var logs []string
	r := NewRegistry()
	m := &MCPClient{CallTimeout: time.Second}
	m.Connect(context.Background(), mcpConfig{Servers: map[string]mcpServer{
		"dead": {Command: "/nonexistent/binary"},
		"none": {},
	}}, func(f string, a ...any) { logs = append(logs, f) })
	m.ListTools(context.Background(), r, func(f string, a ...any) { logs = append(logs, f) })
	if len(r.order) != 0 || len(logs) != 2 {
		t.Errorf("both servers must be logged and skipped: tools=%v logs=%v", r.order, logs)
	}
}

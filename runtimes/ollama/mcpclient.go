// MCP servers from the compiled mcp.json the manager mounts, connected over the
// official SDK and advertised as mcp__<server>__<tool>.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpConfig mirrors what internal/mcpcompile renders. Env values may carry
// ${VAR} references to pod env, resolved here exactly as claude-code would.
type mcpConfig struct {
	Servers map[string]mcpServer `json:"mcpServers"`
}

type mcpServer struct {
	Type    string            `json:"type,omitempty"`
	URL     string            `json:"url,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCPClient owns the sessions for one pod's lifetime.
type MCPClient struct {
	CallTimeout time.Duration
	sessions    map[string]*mcp.ClientSession
}

// readMCPConfig reads $MCP_CONFIG; a missing file is no servers.
func readMCPConfig(path string) (mcpConfig, error) {
	var cfg mcpConfig
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Connect opens a session to every server. A server that fails to connect is
// LOGGED with the consequence and skipped; the run continues with the rest.
//
// It registers NO tools. Listing happens per unit, in ListTools, because under
// egress mediation the proxy in the pod grants nothing until it has seen the
// work unit go by on /work — a tools/list made at startup comes back empty,
// and the reference runtime never met that because its CLI connects per run.
func (m *MCPClient) Connect(ctx context.Context, cfg mcpConfig, logf func(string, ...any)) {
	m.sessions = map[string]*mcp.ClientSession{}
	names := make([]string, 0, len(cfg.Servers))
	for n := range cfg.Servers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		session, err := m.connectOne(ctx, name, cfg.Servers[name])
		if err != nil {
			logf("[mcp] %s: not connected: %v — its tools are unavailable", name, err)
			continue
		}
		m.sessions[name] = session
		logf("[mcp] %s: connected", name)
	}
}

// ListTools registers every connected server's tools, as mcp__<server>__<tool>,
// into a registry built for ONE unit. Called after the unit is polled and
// before the loop runs, so what is listed is what this unit is granted.
func (m *MCPClient) ListTools(ctx context.Context, r *Registry, logf func(string, ...any)) {
	names := make([]string, 0, len(m.sessions))
	for n := range m.sessions {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		session := m.sessions[name]
		var count int
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				logf("[mcp] %s: tools/list: %v", name, err)
				break
			}
			schema, _ := json.Marshal(tool.InputSchema)
			toolName := tool.Name
			r.Add(Tool{Name: "mcp__" + name + "__" + toolName, Description: tool.Description, Schema: schema,
				Call: func(ctx context.Context, args json.RawMessage) (string, bool, error) {
					return m.call(ctx, session, toolName, args)
				}})
			count++
		}
		logf("[mcp] %s: %d tool(s) listed", name, count)
	}
}

func (m *MCPClient) connectOne(ctx context.Context, name string, srv mcpServer) (*mcp.ClientSession, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "agentops-runtime-ollama", Version: version}, nil)
	var transport mcp.Transport
	switch {
	case srv.URL != "":
		transport = &mcp.StreamableClientTransport{Endpoint: srv.URL, HTTPClient: newHTTPClient(0)}
	case srv.Command != "":
		cmd := exec.Command(srv.Command, srv.Args...)
		cmd.Env = os.Environ()
		for k, v := range srv.Env {
			cmd.Env = append(cmd.Env, k+"="+os.ExpandEnv(v))
		}
		cmd.Stderr = os.Stderr
		transport = &mcp.CommandTransport{Command: cmd}
	default:
		return nil, fmt.Errorf("neither url nor command")
	}
	return client.Connect(ctx, transport, nil)
}

func (m *MCPClient) call(ctx context.Context, s *mcp.ClientSession, tool string, raw json.RawMessage) (string, bool, error) {
	var args map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return toolErr("%s: arguments are not a JSON object: %v", tool, err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}
	ctx, cancel := context.WithTimeout(ctx, m.CallTimeout)
	defer cancel()
	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return toolErr("%s: %v", tool, err)
	}
	var b strings.Builder
	for _, c := range res.Content {
		switch c := c.(type) {
		case *mcp.TextContent:
			b.WriteString(c.Text)
		default:
			j, _ := json.Marshal(c)
			b.Write(j)
		}
		b.WriteByte('\n')
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		j, _ := json.Marshal(res.StructuredContent)
		b.Write(j)
	}
	return strings.TrimRight(b.String(), "\n"), res.IsError, nil
}

// Close ends every session.
func (m *MCPClient) Close() {
	for _, s := range m.sessions {
		_ = s.Close()
	}
}

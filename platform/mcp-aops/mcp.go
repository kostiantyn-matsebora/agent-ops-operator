// The MCP protocol surface: JSON-RPC 2.0 over a single streamable-HTTP
// endpoint. Every tool call completes within its own request (design D-F),
// so the server keeps no session state between them — the "streamable" half
// of the transport is unused here on purpose: a single JSON response is
// exactly what the spec allows a server to answer with when it has nothing
// further to stream.
package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const protocolVersion = "2025-06-18"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// caller is who presented this request, read from the headers the runtime
// pod's own MCPConfig entry sets from ITS OWN env (never from a tool
// argument a model could pick on its own behalf — see main.go's doc
// comment). Exactly one of Conversation/Channel is normally set; the server
// forwards whichever it has and lets the manager decide the rest.
type caller struct {
	Token        string
	Conversation string
	Channel      string
}

func callerFromRequest(r *http.Request) caller {
	return caller{
		Token:        strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
		Conversation: r.Header.Get("X-Aops-Conversation"),
		Channel:      r.Header.Get("X-Aops-Channel"),
	}
}

type server struct {
	client *managerClient
}

func newServer(managerURL string) *server {
	return &server{client: newManagerClient(managerURL)}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /mcp", s.handleMCP)
	return mux
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *server) handleMCP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeRPCError(w, nil, codeParseError, "could not read the request body")
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeRPCError(w, nil, codeParseError, "invalid JSON-RPC request")
		return
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		writeRPCError(w, req.ID, codeInvalidRequest, `need {"jsonrpc":"2.0","method":...}`)
		return
	}

	// A NOTIFICATION carries no id and gets no response body — the two this
	// server ever receives are both fire-and-forget from the client's side.
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"
	if req.Method == "notifications/initialized" || req.Method == "notifications/cancelled" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	who := callerFromRequest(r)
	result, rpcErr := s.dispatch(r.Context(), who, req.Method, req.Params)
	if isNotification {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if rpcErr != nil {
		writeRPCError(w, req.ID, rpcErr.Code, rpcErr.Message)
		return
	}
	writeJSON(w, 200, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	writeJSON(w, 200, rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}})
}

// dispatch is the whole method table. Every branch not size gained "case" of
// its own delegates to a same-shaped helper so the table stays scannable.
func (s *server) dispatch(ctx context.Context, who caller, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "agentops-coordinate", "version": "1.0.0"},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefinitions}, nil
	case "tools/call":
		return s.dispatchToolCall(ctx, who, params)
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "unknown method " + method}
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *server) dispatchToolCall(ctx context.Context, who caller, raw json.RawMessage) (any, *rpcError) {
	var p toolCallParams
	if err := json.Unmarshal(raw, &p); err != nil || p.Name == "" {
		return nil, &rpcError{Code: codeInvalidParams, Message: `need {"name":...,"arguments":{...}}`}
	}
	impl, ok := toolImpls[p.Name]
	if !ok {
		return nil, &rpcError{Code: codeInvalidParams, Message: "unknown tool " + p.Name}
	}
	text, isErr := impl(ctx, s.client, who, p.Arguments)
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	}, nil
}

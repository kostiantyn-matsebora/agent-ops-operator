package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeManager records every request it receives and answers with a canned
// response per path, so tests can assert both what mcp-aops forwarded and
// what it did with the manager's reply.
type fakeManager struct {
	t         *testing.T
	responses map[string]fakeResponse
	requests  []fakeRequest
}

type fakeResponse struct {
	status int
	body   any
}

type fakeRequest struct {
	path  string
	token string
	body  map[string]any
}

func newFakeManager(t *testing.T) (*fakeManager, *httptest.Server) {
	fm := &fakeManager{t: t, responses: map[string]fakeResponse{}}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		fm.requests = append(fm.requests, fakeRequest{
			path: r.URL.Path, token: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), body: body,
		})
		resp, ok := fm.responses[r.URL.Path]
		if !ok {
			w.WriteHeader(500)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "no fake response for " + r.URL.Path})
			return
		}
		w.WriteHeader(resp.status)
		_ = json.NewEncoder(w).Encode(resp.body)
	}))
	return fm, ts
}

func (fm *fakeManager) on(path string, status int, body any) {
	fm.responses[path] = fakeResponse{status: status, body: body}
}

func rpcCall(t *testing.T, s *server, who caller, method string, params any) rpcResponse {
	t.Helper()
	raw, _ := json.Marshal(params)
	req := rpcRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: method, Params: raw}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/mcp", strings.NewReader(string(body)))
	httpReq.Header.Set("Authorization", "Bearer "+who.Token)
	if who.Conversation != "" {
		httpReq.Header.Set("X-Aops-Conversation", who.Conversation)
	}
	if who.Channel != "" {
		httpReq.Header.Set("X-Aops-Channel", who.Channel)
	}
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httpReq)
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out rpcResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid JSON-RPC response: %v (%s)", err, rec.Body.String())
	}
	return out
}

func TestInitializeReportsProtocolVersionAndToolsCapability(t *testing.T) {
	_, ts := newFakeManager(t)
	defer ts.Close()
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{}, "initialize", map[string]any{})
	if out.Error != nil {
		t.Fatalf("unexpected error: %+v", out.Error)
	}
	result, ok := out.Result.(map[string]any)
	if !ok || result["protocolVersion"] != protocolVersion {
		t.Fatalf("want protocolVersion %q in result, got %+v", protocolVersion, out.Result)
	}
}

func TestToolsListReturnsAllNineTools(t *testing.T) {
	_, ts := newFakeManager(t)
	defer ts.Close()
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{}, "tools/list", map[string]any{})
	result := out.Result.(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 9 {
		t.Fatalf("want 9 tools, got %d", len(tools))
	}
	want := map[string]bool{
		"list_agents": true, "list_conversations": true, "get_conversation": true, "get_tree": true,
		"invoke": true, "close": true, "escalate": true, "read": true, "list_open_roots": true,
	}
	for _, raw := range tools {
		name := raw.(map[string]any)["name"].(string)
		if !want[name] {
			t.Fatalf("unexpected tool %q", name)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Fatalf("missing tools: %v", want)
	}
}

func TestUnknownMethodReportsMethodNotFound(t *testing.T) {
	_, ts := newFakeManager(t)
	defer ts.Close()
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{}, "not/a/real/method", map[string]any{})
	if out.Error == nil || out.Error.Code != codeMethodNotFound {
		t.Fatalf("want a methodNotFound error, got %+v", out)
	}
}

func TestNotificationsInitializedGetsNoBodyAnd202(t *testing.T) {
	_, ts := newFakeManager(t)
	defer ts.Close()
	s := newServer(ts.URL)

	req := rpcRequest{JSONRPC: "2.0", Method: "notifications/initialized"}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/mcp", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	s.handler().ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("want 202, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("want no body for a notification, got %q", rec.Body.String())
	}
}

func TestToolsCallForwardsTheCallersTokenAndIdentity(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on("/coordinate/invoke", 200, map[string]any{"member": "member-1", "status": "created"})
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{Token: "tok-1", Conversation: "root-1"}, "tools/call",
		map[string]any{"name": "invoke", "arguments": map[string]any{"agent": "worker", "task": "do it"}})
	if out.Error != nil {
		t.Fatalf("unexpected protocol error: %+v", out.Error)
	}
	result := out.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("want success, got %+v", result)
	}

	if len(fm.requests) != 1 {
		t.Fatalf("want one request to the manager, got %d", len(fm.requests))
	}
	got := fm.requests[0]
	if got.path != "/coordinate/invoke" || got.token != "tok-1" || got.body["conversation"] != "root-1" {
		t.Fatalf("want the caller's own token and conversation forwarded, got %+v", got)
	}
}

func TestToolsCallOfAnUnknownToolIsInvalidParams(t *testing.T) {
	_, ts := newFakeManager(t)
	defer ts.Close()
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{}, "tools/call", map[string]any{"name": "not-a-tool", "arguments": map[string]any{}})
	if out.Error == nil || out.Error.Code != codeInvalidParams {
		t.Fatalf("want invalidParams, got %+v", out)
	}
}

func TestToolsCallWithNoIdentitySurfacesAsAToolErrorNotAProtocolError(t *testing.T) {
	_, ts := newFakeManager(t)
	defer ts.Close()
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{Token: "tok-1"}, "tools/call",
		map[string]any{"name": "read", "arguments": map[string]any{}})
	if out.Error != nil {
		t.Fatalf("a missing identity is a TOOL error, not a protocol error, got %+v", out.Error)
	}
	result := out.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Fatalf("want isError true, got %+v", result)
	}
}

func TestToolsCallSurfacesAManagerRefusalAsAToolError(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on("/coordinate/invoke", 403, map[string]any{"error": "no such agents[] entry"})
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{Token: "tok-1", Conversation: "root-1"}, "tools/call",
		map[string]any{"name": "invoke", "arguments": map[string]any{"agent": "nope", "task": "x"}})
	result := out.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Fatalf("want isError true for a manager refusal, got %+v", result)
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "no such agents") {
		t.Fatalf("want the manager's own refusal text surfaced, got %q", text)
	}
}

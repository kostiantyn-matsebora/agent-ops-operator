package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakePulse records every request it receives and answers with a canned
// response per path, so tests can assert both what mcp-pulse forwarded and
// what it did with Pulse's reply.
type fakePulse struct {
	t         *testing.T
	responses map[string]fakeResponse
	requests  []fakeRequest
}

type fakeResponse struct {
	status int
	body   any
}

type fakeRequest struct {
	method string
	path   string
	query  string
	token  string
	body   map[string]any
}

func newFakePulse(t *testing.T) (*fakePulse, *httptest.Server) {
	fp := &fakePulse{t: t, responses: map[string]fakeResponse{}}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		fp.requests = append(fp.requests, fakeRequest{
			method: r.Method, path: r.URL.Path, query: r.URL.RawQuery,
			token: strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), body: body,
		})
		resp, ok := fp.responses[r.URL.Path]
		if !ok {
			w.WriteHeader(500)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "no fake response for " + r.URL.Path})
			return
		}
		w.WriteHeader(resp.status)
		_ = json.NewEncoder(w).Encode(resp.body)
	}))
	return fp, ts
}

func (fp *fakePulse) on(path string, status int, body any) {
	fp.responses[path] = fakeResponse{status: status, body: body}
}

func rpcCall(t *testing.T, s *server, who caller, method string, params any) rpcResponse {
	t.Helper()
	raw, _ := json.Marshal(params)
	req := rpcRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: method, Params: raw}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/mcp", strings.NewReader(string(body)))
	if who.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+who.Token)
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

// toolText runs one tools/call and returns the content text and isError.
func toolText(t *testing.T, s *server, who caller, tool string, args map[string]any) (string, bool) {
	t.Helper()
	out := rpcCall(t, s, who, "tools/call", map[string]any{"name": tool, "arguments": args})
	result := out.Result.(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	isErr, _ := result["isError"].(bool)
	return text, isErr
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestInitializeReportsProtocolVersionAndToolsCapability(t *testing.T) {
	_, ts := newFakePulse(t)
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

func TestToolsListReturnsAllSixTools(t *testing.T) {
	_, ts := newFakePulse(t)
	defer ts.Close()
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{}, "tools/list", map[string]any{})
	result := out.Result.(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 6 {
		t.Fatalf("want 6 tools, got %d", len(tools))
	}
	want := map[string]bool{
		"get_resources": true, "get_metrics": true, "get_alerts": true, "get_storage_history": true,
		"set_maintenance_window": true, "set_criticality_tag": true,
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
	_, ts := newFakePulse(t)
	defer ts.Close()
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{}, "not/a/real/method", map[string]any{})
	if out.Error == nil || out.Error.Code != codeMethodNotFound {
		t.Fatalf("want a methodNotFound error, got %+v", out)
	}
}

func TestNotificationsInitializedGetsNoBodyAnd202(t *testing.T) {
	_, ts := newFakePulse(t)
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

func TestToolsCallOfAnUnknownToolIsInvalidParams(t *testing.T) {
	_, ts := newFakePulse(t)
	defer ts.Close()
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{}, "tools/call", map[string]any{"name": "not-a-tool", "arguments": map[string]any{}})
	if out.Error == nil || out.Error.Code != codeInvalidParams {
		t.Fatalf("want invalidParams, got %+v", out)
	}
}

// --- 2.2: token forwarding ---------------------------------------------

// TestOutboundAuthorizationMatchesInboundExactly is task 2.2's test: the
// header mcp-pulse sends to Pulse must equal, byte for byte, the bearer
// token presented on the inbound MCP call — no re-derivation, no
// normalization, no truncation.
func TestOutboundAuthorizationMatchesInboundExactly(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	fp.on("/api/resources", 200, map[string]any{"resources": []any{}})
	s := newServer(ts.URL)

	const inbound = "monitoring-read-scoped-token-42"
	text, isErr := toolText(t, s, caller{Token: inbound}, "get_resources", map[string]any{})
	if isErr {
		t.Fatalf("want success, got %q", text)
	}
	if len(fp.requests) != 1 {
		t.Fatalf("want one request to pulse, got %d", len(fp.requests))
	}
	if fp.requests[0].token != inbound {
		t.Fatalf("want the outbound token to equal the inbound one %q, got %q", inbound, fp.requests[0].token)
	}
}

func TestNoTokenMeansNoAuthorizationHeaderAtAll(t *testing.T) {
	fp, ts := newFakePulse(t)
	defer ts.Close()
	fp.on("/api/resources", 200, map[string]any{"resources": []any{}})
	s := newServer(ts.URL)

	toolText(t, s, caller{}, "get_resources", map[string]any{})
	if len(fp.requests) != 1 || fp.requests[0].token != "" {
		t.Fatalf("want no bearer token forwarded when none was presented, got %+v", fp.requests)
	}
}

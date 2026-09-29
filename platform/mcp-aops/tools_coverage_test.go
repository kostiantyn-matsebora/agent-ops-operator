package main

import (
	"context"
	"testing"
)

// toolText runs one tools/call and returns the content text and isError.
func toolText(t *testing.T, s *server, who caller, tool string, args map[string]any) (string, bool) {
	t.Helper()
	out := rpcCall(t, s, who, "tools/call", map[string]any{"name": tool, "arguments": args})
	result := out.Result.(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	isErr, _ := result["isError"].(bool)
	return text, isErr
}

func TestEveryConversationToolNeedsAnIdentity(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	s := newServer(ts.URL)
	for _, tool := range []string{"list_agents", "list_conversations", "get_conversation", "get_tree", "invoke", "close", "escalate", "read"} {
		text, isErr := toolText(t, s, caller{Token: "tok-1"}, tool, map[string]any{})
		if !isErr || !contains(text, "X-Aops-Conversation") {
			t.Fatalf("%s: want an identity refusal, got %q (isError=%v)", tool, text, isErr)
		}
	}
	if len(fm.requests) != 0 {
		t.Fatalf("no manager call may be made without an identity, got %d", len(fm.requests))
	}
}

func TestForwardingToolsPostToTheirManagerPath(t *testing.T) {
	cases := []struct {
		tool, path string
		args       map[string]any
		wantKey    string
		wantVal    any
	}{
		{"list_agents", "/coordinate/agents", map[string]any{}, "conversation", "root-1"},
		{"get_tree", "/coordinate/tree", map[string]any{"name": "member-1"}, "target", "member-1"},
		{"get_conversation", "/coordinate/read", map[string]any{"name": "member-1"}, "target", "member-1"},
		{"read", "/coordinate/read", map[string]any{"name": "member-1"}, "target", "member-1"},
		{"invoke", "/coordinate/invoke", map[string]any{"agent": "a", "task": "do"}, "task", "do"},
		{"escalate", "/coordinate/escalate", map[string]any{"message": "help"}, "message", "help"},
		{"close", "/coordinate/close", map[string]any{"conversation": "member-1", "reason": "done"}, "target", "member-1"},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) { assertForwards(t, tc.tool, tc.path, tc.args, tc.wantKey, tc.wantVal) })
	}
}

func assertForwards(t *testing.T, tool, path string, args map[string]any, key string, val any) {
	t.Helper()
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on(path, 200, map[string]any{"ok": true})
	s := newServer(ts.URL)
	text, isErr := toolText(t, s, caller{Token: "tok-1", Conversation: "root-1"}, tool, args)
	if isErr {
		t.Fatalf("want success, got %q", text)
	}
	if len(fm.requests) != 1 || fm.requests[0].body[key] != val {
		t.Fatalf("want one call with %s=%v, got %+v", key, val, fm.requests)
	}
}

func TestToolsReportAManagerRefusalAsToolError(t *testing.T) {
	cases := map[string]struct {
		path string
		args map[string]any
	}{
		"list_agents":        {"/coordinate/agents", map[string]any{}},
		"list_conversations": {"/coordinate/tree", map[string]any{}},
		"get_conversation":   {"/coordinate/read", map[string]any{}},
		"get_tree":           {"/coordinate/tree", map[string]any{}},
		"invoke":             {"/coordinate/invoke", map[string]any{"agent": "a", "task": "t"}},
		"close":              {"/coordinate/close", map[string]any{"reason": "r"}},
		"escalate":           {"/coordinate/escalate", map[string]any{"message": "m"}},
		"read":               {"/coordinate/read", map[string]any{}},
	}
	for tool, tc := range cases {
		t.Run(tool, func(t *testing.T) { assertRefusal(t, tool, tc.path, tc.args) })
	}
}

func assertRefusal(t *testing.T, tool, path string, args map[string]any) {
	t.Helper()
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on(path, 403, map[string]any{"error": "outside your reach"})
	s := newServer(ts.URL)
	text, isErr := toolText(t, s, caller{Token: "tok-1", Conversation: "root-1"}, tool, args)
	if !isErr || !contains(text, "outside your reach") {
		t.Fatalf("want the manager's own refusal as a tool error, got %q (isError=%v)", text, isErr)
	}
}

func TestChannelReaderRefusalIsAToolError(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on("/coordinate/read", 403, map[string]any{"error": "no such channel"})
	s := newServer(ts.URL)
	for _, tool := range []string{"list_conversations", "get_conversation"} {
		text, isErr := toolText(t, s, caller{Token: "tok-1", Channel: "voice-desk"}, tool, map[string]any{"name": "conv-1"})
		if !isErr || !contains(text, "no such channel") {
			t.Fatalf("%s: got %q (isError=%v)", tool, text, isErr)
		}
	}
}

func TestToolsRefuseMissingRequiredArguments(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	s := newServer(ts.URL)
	who := caller{Token: "tok-1", Conversation: "root-1"}
	for tool, args := range map[string]map[string]any{
		"invoke":   {"agent": "a"},
		"close":    {},
		"escalate": {},
	} {
		if text, isErr := toolText(t, s, who, tool, args); !isErr || !contains(text, "need") {
			t.Fatalf("%s: want an argument refusal, got %q (isError=%v)", tool, text, isErr)
		}
	}
	if len(fm.requests) != 0 {
		t.Fatalf("a malformed call must not reach the manager, got %d", len(fm.requests))
	}
}

func TestManagerErrorAndTransportFailureText(t *testing.T) {
	me := &managerError{Status: 500, Body: "boom"}
	if me.Error() == "" {
		t.Fatal("managerError must render")
	}
	if text, isErr := errText(me); text != "boom" || !isErr {
		t.Fatalf("got %q %v", text, isErr)
	}
	c := newManagerClient("http://127.0.0.1:1")
	if err := c.post(context.Background(),"/x", "", map[string]any{}, nil); err == nil {
		t.Fatal("want a transport error")
	}
	if text, isErr := jsonText(make(chan int)); !isErr || text == "" {
		t.Fatalf("an unmarshalable value must be an error text, got %q", text)
	}
}

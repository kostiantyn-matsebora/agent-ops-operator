package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestFlattenTreeIncludesRootAndEveryDescendant(t *testing.T) {
	tree := map[string]any{
		"name": "root-1",
		"members": []any{
			map[string]any{
				"name": "member-1",
				"members": []any{
					map[string]any{"name": "grandchild-1"},
				},
			},
			map[string]any{"name": "member-2"},
		},
	}
	got := flattenTree(tree)
	var names []string
	for _, n := range got {
		names = append(names, n["name"].(string))
	}
	want := []string{"root-1", "member-1", "grandchild-1", "member-2"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	// "members" must never leak into a flattened node's own projection.
	for _, n := range got {
		if _, has := n["members"]; has {
			t.Fatalf("flattened node %v must not carry its own members key", n)
		}
	}
}

func TestFlattenTreeOfALeafIsJustItself(t *testing.T) {
	got := flattenTree(map[string]any{"name": "leaf-1"})
	if len(got) != 1 || got[0]["name"] != "leaf-1" {
		t.Fatalf("want just the leaf, got %v", got)
	}
}

func TestListConversationsUsesTheChannelProjectionForAChannelReader(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on("/coordinate/read", 200, map[string]any{"conversations": []any{
		map[string]any{"name": "conv-1"}, map[string]any{"name": "conv-2"},
	}})
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{Token: "tok-1", Channel: "voice-desk"}, "tools/call",
		map[string]any{"name": "list_conversations", "arguments": map[string]any{}})
	result := out.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("want success, got %+v", result)
	}
	if fm.requests[0].body["channel"] != "voice-desk" {
		t.Fatalf("want the channel forwarded, got %+v", fm.requests[0].body)
	}
}

func TestListConversationsFlattensTheSubtreeForACoordinatorCaller(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on("/coordinate/tree", 200, map[string]any{
		"name":    "root-1",
		"members": []any{map[string]any{"name": "member-1"}},
	})
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{Token: "tok-1", Conversation: "root-1"}, "tools/call",
		map[string]any{"name": "list_conversations", "arguments": map[string]any{}})
	result := out.Result.(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !contains(text, "root-1") || !contains(text, "member-1") {
		t.Fatalf("want both root and member flattened into the list, got %s", text)
	}
}

func TestGetConversationFiltersTheChannelProjectionByName(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on("/coordinate/read", 200, map[string]any{"conversations": []any{
		map[string]any{"name": "conv-1", "brief": "the one we want"},
		map[string]any{"name": "conv-2", "brief": "not this one"},
	}})
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{Token: "tok-1", Channel: "voice-desk"}, "tools/call",
		map[string]any{"name": "get_conversation", "arguments": map[string]any{"name": "conv-1"}})
	result := out.Result.(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !contains(text, "the one we want") || contains(text, "not this one") {
		t.Fatalf("want only conv-1's projection, got %s", text)
	}
}

func TestGetConversationRefusesANameOutsideTheChannelProjection(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on("/coordinate/read", 200, map[string]any{"conversations": []any{
		map[string]any{"name": "conv-1"},
	}})
	s := newServer(ts.URL)

	out := rpcCall(t, s, caller{Token: "tok-1", Channel: "voice-desk"}, "tools/call",
		map[string]any{"name": "get_conversation", "arguments": map[string]any{"name": "stranger-1"}})
	result := out.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); !isErr {
		t.Fatalf("want an error for a name outside the channel's own projection, got %+v", result)
	}
}

func TestCloseDefaultsTargetToEmptyMeaningTheCallerItself(t *testing.T) {
	fm, ts := newFakeManager(t)
	defer ts.Close()
	fm.on("/coordinate/close", 200, map[string]any{"ok": true})
	s := newServer(ts.URL)

	rpcCall(t, s, caller{Token: "tok-1", Conversation: "root-1"}, "tools/call",
		map[string]any{"name": "close", "arguments": map[string]any{"reason": "done"}})
	got := fm.requests[0].body
	if got["target"] != "" || got["reason"] != "done" || got["conversation"] != "root-1" {
		t.Fatalf("got %+v", got)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

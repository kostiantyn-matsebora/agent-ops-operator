// The eight tools design D-F names, each a thin forward to one manager
// /coordinate/* call. No tool ever reads or sets the caller's own identity —
// that comes from the headers callerFromRequest read, never from an argument
// a model could fill in on its own behalf.
package main

import (
	"context"
	"encoding/json"
)

// toolImpl runs one tool call and reports its result as MCP "content" text,
// plus whether it is an error (isError) — an MCP tool failure is reported to
// the MODEL as ordinary content, never as a JSON-RPC protocol error, so the
// agent can read why and decide what to do next.
type toolImpl func(ctx context.Context, c *managerClient, who caller, args json.RawMessage) (text string, isError bool)

var toolImpls = map[string]toolImpl{
	"list_agents":        toolListAgents,
	"list_conversations": toolListConversations,
	"get_conversation":   toolGetConversation,
	"get_tree":           toolGetTree,
	"invoke":             toolInvoke,
	"close":              toolClose,
	"escalate":           toolEscalate,
	"read":               toolRead,
}

// jsonText renders v as the tool's success text, or an error text when v
// cannot be marshaled (never happens for the plain maps every tool decodes
// into, but a nil check here beats a panic reaching the transport).
func jsonText(v any) (string, bool) {
	b, err := json.Marshal(v)
	if err != nil {
		return err.Error(), true
	}
	return string(b), false
}

func errText(err error) (string, bool) {
	if me, ok := err.(*managerError); ok {
		return me.Body, true
	}
	return err.Error(), true
}

func needsIdentity() (string, bool) {
	return "this call presents no coordinated-conversation identity (missing X-Aops-Conversation)", true
}

func toolListAgents(ctx context.Context, c *managerClient, who caller, _ json.RawMessage) (string, bool) {
	if who.Conversation == "" {
		return needsIdentity()
	}
	var out map[string]any
	if err := c.post(ctx, "/coordinate/agents", who.Token,
		map[string]any{"conversation": who.Conversation}, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

func toolListConversations(ctx context.Context, c *managerClient, who caller, _ json.RawMessage) (string, bool) {
	if who.Channel != "" {
		var out map[string]any
		if err := c.post(ctx, "/coordinate/read", who.Token, map[string]any{"channel": who.Channel}, &out); err != nil {
			return errText(err)
		}
		return jsonText(out)
	}
	if who.Conversation == "" {
		return needsIdentity()
	}
	var tree map[string]any
	if err := c.post(ctx, "/coordinate/tree", who.Token, map[string]any{"conversation": who.Conversation}, &tree); err != nil {
		return errText(err)
	}
	return jsonText(map[string]any{"conversations": flattenTree(tree)})
}

// flattenTree walks a /coordinate/tree response (design D-F's nested
// treeNode shape) into a flat list, root included — the same information
// get_tree returns, shaped for a caller that just wants "what exists".
func flattenTree(node map[string]any) []map[string]any {
	if node == nil {
		return nil
	}
	self := map[string]any{}
	for k, v := range node {
		if k != "members" {
			self[k] = v
		}
	}
	out := []map[string]any{self}
	members, _ := node["members"].([]any)
	for _, m := range members {
		if child, ok := m.(map[string]any); ok {
			out = append(out, flattenTree(child)...)
		}
	}
	return out
}

type nameArg struct {
	Name string `json:"name,omitempty"`
}

func toolGetConversation(ctx context.Context, c *managerClient, who caller, raw json.RawMessage) (string, bool) {
	var args nameArg
	_ = json.Unmarshal(raw, &args)
	if who.Channel != "" {
		var out map[string]any
		if err := c.post(ctx, "/coordinate/read", who.Token, map[string]any{"channel": who.Channel}, &out); err != nil {
			return errText(err)
		}
		convs, _ := out["conversations"].([]any)
		for _, item := range convs {
			if proj, ok := item.(map[string]any); ok && proj["name"] == args.Name {
				return jsonText(proj)
			}
		}
		return "no such conversation on this channel", true
	}
	if who.Conversation == "" {
		return needsIdentity()
	}
	body := map[string]any{"conversation": who.Conversation}
	if args.Name != "" {
		body["target"] = args.Name
	}
	var out map[string]any
	if err := c.post(ctx, "/coordinate/read", who.Token, body, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

func toolGetTree(ctx context.Context, c *managerClient, who caller, raw json.RawMessage) (string, bool) {
	if who.Conversation == "" {
		return needsIdentity()
	}
	var args nameArg
	_ = json.Unmarshal(raw, &args)
	body := map[string]any{"conversation": who.Conversation}
	if args.Name != "" {
		body["target"] = args.Name
	}
	var out map[string]any
	if err := c.post(ctx, "/coordinate/tree", who.Token, body, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

type invokeArgs struct {
	Agent string `json:"agent"`
	Task  string `json:"task"`
}

func toolInvoke(ctx context.Context, c *managerClient, who caller, raw json.RawMessage) (string, bool) {
	if who.Conversation == "" {
		return needsIdentity()
	}
	var args invokeArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Agent == "" || args.Task == "" {
		return `need {"agent":...,"task":...}`, true
	}
	var out map[string]any
	if err := c.post(ctx, "/coordinate/invoke", who.Token,
		map[string]any{"conversation": who.Conversation, "agent": args.Agent, "task": args.Task}, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

type closeArgs struct {
	Conversation string `json:"conversation,omitempty"`
	Reason       string `json:"reason"`
}

func toolClose(ctx context.Context, c *managerClient, who caller, raw json.RawMessage) (string, bool) {
	if who.Conversation == "" {
		return needsIdentity()
	}
	var args closeArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Reason == "" {
		return `need {"reason":...} (and optionally "conversation" for a direct member)`, true
	}
	var out map[string]any
	if err := c.post(ctx, "/coordinate/close", who.Token,
		map[string]any{"conversation": who.Conversation, "target": args.Conversation, "reason": args.Reason}, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

type escalateArgs struct {
	Message string `json:"message"`
}

func toolEscalate(ctx context.Context, c *managerClient, who caller, raw json.RawMessage) (string, bool) {
	if who.Conversation == "" {
		return needsIdentity()
	}
	var args escalateArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Message == "" {
		return `need {"message":...}`, true
	}
	var out map[string]any
	if err := c.post(ctx, "/coordinate/escalate", who.Token,
		map[string]any{"conversation": who.Conversation, "message": args.Message}, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

func toolRead(ctx context.Context, c *managerClient, who caller, raw json.RawMessage) (string, bool) {
	if who.Conversation == "" {
		return needsIdentity()
	}
	var args nameArg
	_ = json.Unmarshal(raw, &args)
	body := map[string]any{"conversation": who.Conversation}
	if args.Name != "" {
		body["target"] = args.Name
	}
	var out map[string]any
	if err := c.post(ctx, "/coordinate/read", who.Token, body, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

// toolDefinitions is tools/list's whole answer — name, a one-line
// description an agent picks the right tool from, and a JSON Schema for its
// arguments.
var toolDefinitions = []map[string]any{
	{
		"name":        "list_agents",
		"description": "List the agents[] entries (name and description) this coordinating conversation may invoke.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "list_conversations",
		"description": "List the conversations in this caller's own reach: its coordination subtree, or the conversations bound to its channel.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "get_conversation",
		"description": "Get one conversation's bounded projection (name, title, brief, phase, pipeline) by name. Omit name for the caller itself.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
			"name": map[string]any{"type": "string"},
		}},
	},
	{
		"name":        "get_tree",
		"description": "Get the nested coordination tree rooted at a conversation in this caller's own subtree. Omit name for the caller's own tree.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
			"name": map[string]any{"type": "string"},
		}},
	},
	{
		"name":        "invoke",
		"description": "Invoke one of this Coordinator's agents[] entries with a task. Returns at once (created or attached); the result arrives later as an input.",
		"inputSchema": map[string]any{"type": "object", "required": []string{"agent", "task"}, "properties": map[string]any{
			"agent": map[string]any{"type": "string"},
			"task":  map[string]any{"type": "string"},
		}},
	},
	{
		"name":        "close",
		"description": "Close a conversation with a reason: the caller itself, or one of its own direct members. Never a deeper descendant.",
		"inputSchema": map[string]any{"type": "object", "required": []string{"reason"}, "properties": map[string]any{
			"conversation": map[string]any{"type": "string", "description": "target to close; omit to close the caller itself"},
			"reason":       map[string]any{"type": "string"},
		}},
	},
	{
		"name":        "escalate",
		"description": "Escalate the calling conversation with a message. On the uncaused root this opens a human thread; on a nested member it closes and bubbles the message to its parent.",
		"inputSchema": map[string]any{"type": "object", "required": []string{"message"}, "properties": map[string]any{
			"message": map[string]any{"type": "string"},
		}},
	},
	{
		"name":        "read",
		"description": "Read one conversation's bounded projection, at any depth within the caller's own subtree. Omit name for the caller itself.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
			"name": map[string]any{"type": "string"},
		}},
	},
}

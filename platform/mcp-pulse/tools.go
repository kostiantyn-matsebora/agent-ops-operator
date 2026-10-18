// The tool set this server exposes: a small, enumerated mapping onto
// Pulse's REST API (design decision 2 — this repository owns the mapping,
// Pulse's exact surface is not vendored here). Each tool is a pure forward:
// it relays the caller's token and args, and returns Pulse's response or
// error verbatim. No tool ever reads or sets the caller's own identity —
// that comes from callerFromRequest's Authorization header, never from an
// argument a model could fill in on its own behalf.
//
// Two risk tiers share this one server (design decision 3):
//
//   - Read tools (inventory/resources, metrics/charts, alerts, storage
//     history) back the pulse-observability toolset.
//   - Write tools (maintenance window, criticality tag) are the two
//     Community-tier operator-state endpoints Pulse exposes without its
//     Pro-gated action-governance flow. They back the pulse-admin toolset.
//
// The wall between the two tiers is which pre-scoped Pulse API token the
// caller's bound MCPConfig supplied, never anything decided in here —
// requirement "mcp-pulse forwards the caller's own token and decides no
// reach itself".
package main

import (
	"context"
	"encoding/json"
	"net/url"
)

// toolImpl runs one tool call and reports its result as MCP "content" text,
// plus whether it is an error (isError) — an MCP tool failure is reported to
// the MODEL as ordinary content, never as a JSON-RPC protocol error, so the
// agent can read why and decide what to do next. A non-2xx Pulse response
// (including a 403 for an under-scoped token) surfaces exactly this way,
// never as a short-circuited local refusal.
type toolImpl func(ctx context.Context, c *pulseClient, who caller, args json.RawMessage) (text string, isError bool)

var toolImpls = map[string]toolImpl{
	"get_resources":          toolGetResources,
	"get_metrics":            toolGetMetrics,
	"get_alerts":             toolGetAlerts,
	"get_storage_history":    toolGetStorageHistory,
	"set_maintenance_window": toolSetMaintenanceWindow,
	"set_criticality_tag":    toolSetCriticalityTag,
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

// errText surfaces a *pulseError's own status and body verbatim — the
// caller's refusal reason, not one this server invents — falling back to the
// bare Go error for anything else (a dial failure, a bad JSON body).
func errText(err error) (string, bool) {
	if pe, ok := err.(*pulseError); ok {
		return pe.Body, true
	}
	return err.Error(), true
}

// --- read tools -------------------------------------------------------

type resourcesArgs struct {
	Type string `json:"type,omitempty"`
}

// toolGetResources forwards to GET /api/resources — Pulse's inventory of
// monitored nodes/VMs/containers. An optional type filter narrows it (e.g.
// "node", "vm", "container"); omitted it returns everything the token can
// see.
func toolGetResources(ctx context.Context, c *pulseClient, who caller, raw json.RawMessage) (string, bool) {
	var args resourcesArgs
	_ = json.Unmarshal(raw, &args)
	q := url.Values{}
	if args.Type != "" {
		q.Set("type", args.Type)
	}
	var out any
	if err := c.get(ctx, "/api/resources", q, who.Token, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

type metricsArgs struct {
	Resource string `json:"resource"`
	Metric   string `json:"metric,omitempty"`
	Range    string `json:"range,omitempty"`
}

// toolGetMetrics forwards to GET /api/metrics — time-series chart data
// (CPU/memory/disk/network) for one resource. resource is required; metric
// narrows to one series, range bounds the window (e.g. "1h", "24h"), both
// left to Pulse's own default when omitted.
func toolGetMetrics(ctx context.Context, c *pulseClient, who caller, raw json.RawMessage) (string, bool) {
	var args metricsArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Resource == "" {
		return `need {"resource":...}`, true
	}
	q := url.Values{"resource": {args.Resource}}
	if args.Metric != "" {
		q.Set("metric", args.Metric)
	}
	if args.Range != "" {
		q.Set("range", args.Range)
	}
	var out any
	if err := c.get(ctx, "/api/metrics", q, who.Token, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

type alertsArgs struct {
	Status string `json:"status,omitempty"`
}

// toolGetAlerts forwards to GET /api/alerts — Pulse's active/historical
// alert list, the same surface its outgoing webhook (signals/pulse) reports
// a firing event from. status optionally narrows to e.g. "active" or "all",
// left to Pulse's own default when omitted.
func toolGetAlerts(ctx context.Context, c *pulseClient, who caller, raw json.RawMessage) (string, bool) {
	var args alertsArgs
	_ = json.Unmarshal(raw, &args)
	q := url.Values{}
	if args.Status != "" {
		q.Set("status", args.Status)
	}
	var out any
	if err := c.get(ctx, "/api/alerts", q, who.Token, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

type storageHistoryArgs struct {
	Resource string `json:"resource"`
	Range    string `json:"range,omitempty"`
}

// toolGetStorageHistory forwards to GET /api/storage/history — historical
// disk/storage usage for one resource, the data an investigation reaches
// for behind a capacity alert. resource is required, range bounds the
// window.
func toolGetStorageHistory(ctx context.Context, c *pulseClient, who caller, raw json.RawMessage) (string, bool) {
	var args storageHistoryArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Resource == "" {
		return `need {"resource":...}`, true
	}
	q := url.Values{"resource": {args.Resource}}
	if args.Range != "" {
		q.Set("range", args.Range)
	}
	var out any
	if err := c.get(ctx, "/api/storage/history", q, who.Token, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

// --- write tools (Community-tier operator-state, pulse-admin only) ----

type maintenanceWindowArgs struct {
	Resource string `json:"resource"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Reason   string `json:"reason,omitempty"`
}

// toolSetMaintenanceWindow forwards to
// POST /api/operator-state/maintenance-window — one of Pulse's two
// scope_only operator-state writes (design decision 1's Community-tier
// substitute for the Pro-gated action-governance flow). resource/start/end
// are required; Pulse's own scoped token is what decides whether this
// caller may write it at all.
func toolSetMaintenanceWindow(ctx context.Context, c *pulseClient, who caller, raw json.RawMessage) (string, bool) {
	var args maintenanceWindowArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Resource == "" || args.Start == "" || args.End == "" {
		return `need {"resource":...,"start":...,"end":...}`, true
	}
	body := map[string]any{"resource": args.Resource, "start": args.Start, "end": args.End}
	if args.Reason != "" {
		body["reason"] = args.Reason
	}
	var out any
	if err := c.post(ctx, "/api/operator-state/maintenance-window", who.Token, body, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

type criticalityTagArgs struct {
	Resource    string `json:"resource"`
	Criticality string `json:"criticality"`
}

// toolSetCriticalityTag forwards to
// POST /api/operator-state/criticality-tag — the second scope_only
// operator-state write, recording an operator's own judgement of a
// resource's criticality (e.g. "low"/"normal"/"high"). Both fields are
// required.
func toolSetCriticalityTag(ctx context.Context, c *pulseClient, who caller, raw json.RawMessage) (string, bool) {
	var args criticalityTagArgs
	if err := json.Unmarshal(raw, &args); err != nil || args.Resource == "" || args.Criticality == "" {
		return `need {"resource":...,"criticality":...}`, true
	}
	body := map[string]any{"resource": args.Resource, "criticality": args.Criticality}
	var out any
	if err := c.post(ctx, "/api/operator-state/criticality-tag", who.Token, body, &out); err != nil {
		return errText(err)
	}
	return jsonText(out)
}

// toolDefinitions is tools/list's whole answer — name, a one-line
// description an agent picks the right tool from, and a JSON Schema for its
// arguments.
var toolDefinitions = []map[string]any{
	{
		"name":        "get_resources",
		"description": "List Pulse's monitored inventory (nodes, VMs, containers), optionally filtered by type.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
			"type": map[string]any{"type": "string", "description": "e.g. node, vm, container"},
		}},
	},
	{
		"name":        "get_metrics",
		"description": "Get time-series chart data (CPU/memory/disk/network) for one resource.",
		"inputSchema": map[string]any{"type": "object", "required": []string{"resource"}, "properties": map[string]any{
			"resource": map[string]any{"type": "string"},
			"metric":   map[string]any{"type": "string", "description": "narrow to one series, e.g. cpu"},
			"range":    map[string]any{"type": "string", "description": "e.g. 1h, 24h"},
		}},
	},
	{
		"name":        "get_alerts",
		"description": "List Pulse's active or historical alerts.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
			"status": map[string]any{"type": "string", "description": "e.g. active, all"},
		}},
	},
	{
		"name":        "get_storage_history",
		"description": "Get historical disk/storage usage for one resource.",
		"inputSchema": map[string]any{"type": "object", "required": []string{"resource"}, "properties": map[string]any{
			"resource": map[string]any{"type": "string"},
			"range":    map[string]any{"type": "string", "description": "e.g. 1h, 24h"},
		}},
	},
	{
		"name":        "set_maintenance_window",
		"description": "Record a maintenance window for one resource (Community-tier operator-state write, scope-enforced by Pulse's own token).",
		"inputSchema": map[string]any{"type": "object", "required": []string{"resource", "start", "end"}, "properties": map[string]any{
			"resource": map[string]any{"type": "string"},
			"start":    map[string]any{"type": "string", "description": "RFC3339 timestamp"},
			"end":      map[string]any{"type": "string", "description": "RFC3339 timestamp"},
			"reason":   map[string]any{"type": "string"},
		}},
	},
	{
		"name":        "set_criticality_tag",
		"description": "Tag one resource's operator-judged criticality (Community-tier operator-state write, scope-enforced by Pulse's own token).",
		"inputSchema": map[string]any{"type": "object", "required": []string{"resource", "criticality"}, "properties": map[string]any{
			"resource":    map[string]any{"type": "string"},
			"criticality": map[string]any{"type": "string", "description": "e.g. low, normal, high"},
		}},
	},
}

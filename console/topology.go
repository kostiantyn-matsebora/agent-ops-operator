package main

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// The wiring graph, derived from CR spec and reported conditions only.
//
// The console asserts NOTHING the cluster does not already say. Every node's
// health is a condition some reconciler wrote (Ready / Served / Wired), and
// every edge is a reference some spec contains. That is the property worth
// protecting: the graph can disagree with `kubectl` only by way of a stale
// cache, never by way of an opinion.

// Ref is an ObjectRef as it appears in specs.
type Ref struct {
	Name string `json:"name"`
}

// pipelineSpec is the console's read of Pipeline.spec.
type pipelineSpec struct {
	SignalSourceRefs []Ref `json:"signalSourceRefs,omitempty"`
	ChannelRefs      []Ref `json:"channelRefs,omitempty"`
	ProfileRef       Ref   `json:"profileRef"`
	Toolsets         *struct {
		Mode string `json:"mode,omitempty"`
		Refs []Ref  `json:"refs,omitempty"`
	} `json:"toolsets,omitempty"`
	MCPConfigs *struct {
		Refs []Ref `json:"refs,omitempty"`
	} `json:"mcpConfigs,omitempty"`
}

// servedSpec is the shared shape of Channel.spec and SignalSource.spec as far
// as the graph cares: which adapter implementation serves it.
type servedSpec struct {
	Adapter string `json:"adapter,omitempty"`
}

// profileSpec is the console's read of AgentProfile.spec (identity only —
// profiles carry no capabilities, so there is nothing else here to draw).
type profileSpec struct {
	RuntimeRef Ref `json:"runtimeRef,omitempty"`
	Repository *struct {
		URL string `json:"url,omitempty"`
		Ref string `json:"ref,omitempty"`
	} `json:"repository,omitempty"`
}

func decodeSpec[T any](raw json.RawMessage) T {
	var v T
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

// Health is a node's rendered state, derived from conditions alone.
type Health string

const (
	// HealthOK: every condition the kind reports that the console reads is True.
	HealthOK Health = "ok"
	// HealthBad: at least one such condition is False.
	HealthBad Health = "bad"
	// HealthUnknown: the kind DOES report health, but has not yet — a fresh
	// object, or a reconciler that has not run. Worth looking at.
	HealthUnknown Health = "unknown"
	// HealthNone: the kind asserts no health at all, so there is nothing to
	// wait for. Distinct from unknown on purpose: AgentProfile and AgentRuntime
	// have a conditions field that nothing writes, and rendering them as
	// perpetually "unknown" would put a pending-looking node on every graph
	// forever.
	HealthNone Health = "none"
)

// healthConditions are the condition types the console reads, per node kind.
// Only these; a condition nobody lists here never colors a node.
//
// A kind absent from this map reports nothing (HealthNone). Keep it that way:
// listing a condition type no reconciler writes manufactures a permanent
// warning out of nothing.
var healthConditions = map[string][]string{
	"pipelines":       {"Ready"},
	"channels":        {"Served"},
	"signalsources":   {"Served", "Wired"},
	"channeladapters": {"Ready"},
	"signaladapters":  {"Ready"},
}

// Node is one vertex of the topology graph.
type Node struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // resource plural
	Name   string `json:"name"`
	Health Health `json:"health"`
	// Reason/Message carry the FAILING condition verbatim — the whole point of
	// showing an unclaimed source is showing why it drops signals.
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	// Detached marks a node no pipeline references (an unclaimed source, an
	// unwired channel): rendered off the wiring, since that IS its state.
	Detached bool `json:"detached,omitempty"`
	// Active/Recent are live conversation counts, set for pipeline nodes.
	Active int `json:"active"`
	Recent int `json:"recent"`
}

// Edge is one directed wiring reference.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	// Kind labels why the edge exists: feeds | answers | posts | served-by | uses.
	Kind string `json:"kind"`
	// Dangling marks a reference to an object that does not exist — drawn as a
	// broken edge to a placeholder rather than silently omitted.
	Dangling bool `json:"dangling,omitempty"`

	// Traffic is the windowed activity on this edge, from recorded hops. Nil
	// means NO EVENTS in the window — rendered as visibly idle, which is a
	// different statement from "this edge does not exist".
	Traffic *EdgeTraffic `json:"traffic,omitempty"`
}

// EdgeTraffic is what actually moved along an edge.
type EdgeTraffic struct {
	Events       int     `json:"events"`
	Errors       int     `json:"errors"`
	RatePerMin   float64 `json:"ratePerMin"`
	P50LatencyMs int64   `json:"p50LatencyMs,omitempty"`
	MaxLatencyMs int64   `json:"maxLatencyMs,omitempty"`
	LastTS       string  `json:"lastTs,omitempty"`
	// Unconfirmed: the manager enqueued and no adapter confirmed delivery.
	// Rendered distinctly from success — adapter reporting is optional, and an
	// adapter that reports nothing must not look like one that delivered.
	Unconfirmed bool `json:"unconfirmed,omitempty"`
}

// Topology is the whole graph, ready to render.
type Topology struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
	// WindowSeconds is the traffic window these rates were computed over.
	WindowSeconds float64 `json:"windowSeconds,omitempty"`
	// EventNodeKind maps the ACTIVITY vocabulary onto graph node ids, so the
	// browser can animate a live event without a second naming scheme. It is
	// served rather than hardcoded in the SPA because the manager owns the
	// vocabulary and this is the one place the two meet.
	EventNodeKinds map[string]string `json:"eventNodeKinds"`
}

// eventNodeKinds translates an activity event's node kind to a resource plural.
// The manager names nodes in singular-ish kebab ("signal-source"); the graph
// keys on resource plurals. Anything unmapped (manager, conversation) is not a
// wiring node and animates nothing.
var eventNodeKinds = map[string]string{
	"signal-adapter":  "signaladapters",
	"signal-source":   "signalsources",
	"pipeline":        "pipelines",
	"profile":         "agentprofiles",
	"runtime":         "agentruntimes",
	"channel":         "channels",
	"channel-adapter": "channeladapters",
	"toolset":         "mcptoolsets",
	"mcp-config":      "mcpconfigs",
}

// applyTraffic attaches windowed edge stats to the graph.
//
// A hop is not always ONE drawn edge, and this is where that is reconciled.
// Three mismatches would otherwise leave most of the flow invisible — which is
// exactly what a first look at the live graph showed, with only
// source -> pipeline animating while everything else moved in silence:
//
//  1. DIRECTION. A signal travels adapter -> source; the graph draws that
//     relationship as source -> adapter (`served-by`). Traffic on an edge means
//     "these two exchanged something", so matching is UNDIRECTED.
//  2. PATHS. A run is one hop pipeline -> runtime, but the wiring reaches the
//     runtime through the profile (pipeline -answers-> profile -uses-> runtime).
//     One hop lights both edges, because the work crossed both.
//  3. NON-WIRING ENDPOINTS. Ops and inbound messages name a CONVERSATION, which
//     the wiring graph has no node for. The conversation is attributed to its
//     pipeline and the hop is credited to that pipeline's edge — the movement
//     is real and it did cross that edge.
//
// What is still credited to nothing: hops whose endpoints resolve to no wiring
// pair at all. Those belong to the per-conversation view, and inventing an edge
// for them would draw wiring that does not exist.
func (a *API) applyTraffic(t *Topology, stats []EdgeStat, window time.Duration) {
	merged := map[string]*EdgeTraffic{}
	credit := func(id string, s *EdgeStat) {
		cur := merged[id]
		if cur == nil {
			cur = &EdgeTraffic{}
			merged[id] = cur
		}
		cur.Events += s.Events
		cur.Errors += s.Errors
		cur.RatePerMin += s.RatePerMin
		if s.P50LatencyMs > cur.P50LatencyMs {
			cur.P50LatencyMs = s.P50LatencyMs
		}
		if s.MaxLatencyMs > cur.MaxLatencyMs {
			cur.MaxLatencyMs = s.MaxLatencyMs
		}
		if s.LastTS > cur.LastTS {
			cur.LastTS = s.LastTS
		}
		cur.Unconfirmed = cur.Unconfirmed || s.Unconfirmed
	}

	// Index the drawn edges so a resolved pair can be found in either direction.
	drawn := map[string]string{}
	for _, e := range t.Edges {
		drawn[e.From+"|"+e.To] = e.From + "->" + e.To
		drawn[e.To+"|"+e.From] = e.From + "->" + e.To
	}
	link := func(a, b string, s *EdgeStat) bool {
		if id, ok := drawn[a+"|"+b]; ok {
			credit(id, s)
			return true
		}
		return false
	}

	for i := range stats {
		s := &stats[i]
		for _, pair := range a.wiringPairs(s.From, s.To) {
			link(pair[0], pair[1], s)
		}
	}

	for i := range t.Edges {
		if tr := merged[t.Edges[i].From+"->"+t.Edges[i].To]; tr != nil {
			t.Edges[i].Traffic = tr
		}
	}
	t.WindowSeconds = window.Seconds()
	t.EventNodeKinds = eventNodeKinds
}

// wiringPairs resolves one hop's endpoints into the wiring node pairs it
// crossed. Empty when the hop belongs to no drawn edge.
func (a *API) wiringPairs(from, to NodeRef) [][2]string {
	fromID, fromOK := a.wiringNode(from)
	toID, toOK := a.wiringNode(to)

	// A run reaches its runtime THROUGH the profile, so a pipeline<->runtime hop
	// lights both legs of that path.
	if fromOK && toOK {
		if pair := a.expandRunPath(fromID, toID); pair != nil {
			return pair
		}
		return [][2]string{{fromID, toID}}
	}

	// One endpoint is a conversation (ops, inbound). Credit its pipeline: that
	// is the wiring the movement travelled, and the conversation view shows the
	// per-conversation detail.
	if fromOK && to.Kind == "conversation" {
		if p := a.pipelineOfConversation(to.Name); p != "" {
			return [][2]string{{fromID, nodeID("pipelines", p)}}
		}
	}
	if toOK && from.Kind == "conversation" {
		if p := a.pipelineOfConversation(from.Name); p != "" {
			return [][2]string{{nodeID("pipelines", p), toID}}
		}
	}
	return nil
}

// wiringNode maps an activity node reference to a graph node id.
func (a *API) wiringNode(n NodeRef) (string, bool) {
	kind, ok := eventNodeKinds[n.Kind]
	if !ok || n.Name == "" {
		return "", false
	}
	return nodeID(kind, n.Name), true
}

// expandRunPath turns a pipeline<->runtime hop into the two edges the wiring
// actually connects them with.
func (a *API) expandRunPath(fromID, toID string) [][2]string {
	pipelineID, runtimeID := fromID, toID
	if strings.HasPrefix(toID, "pipelines/") {
		pipelineID, runtimeID = toID, fromID
	}
	if !strings.HasPrefix(pipelineID, "pipelines/") || !strings.HasPrefix(runtimeID, "agentruntimes/") {
		return nil
	}
	p := a.cache.Get("pipelines", strings.TrimPrefix(pipelineID, "pipelines/"))
	if p == nil {
		return nil
	}
	profile := decodeSpec[pipelineSpec](p.Spec).ProfileRef.Name
	if profile == "" {
		return nil
	}
	return [][2]string{
		{pipelineID, nodeID("agentprofiles", profile)},
		{nodeID("agentprofiles", profile), runtimeID},
	}
}

// pipelineOfConversation attributes a conversation, cached per request-ish by
// the cheapness of the underlying list.
func (a *API) pipelineOfConversation(name string) string {
	conv := a.cache.Get("conversations", name)
	if conv == nil {
		return ""
	}
	return AttributePipeline(conv, a.cache.List("pipelines"))
}

func nodeID(kind, name string) string { return kind + "/" + name }

// health reads the conditions the console cares about for a kind.
func health(obj *Object) (Health, string, string) {
	types := healthConditions[obj.Kind]
	if len(types) == 0 {
		return HealthNone, "", ""
	}
	seen := 0
	for _, t := range types {
		c := obj.Condition(t)
		if c == nil {
			continue
		}
		seen++
		if c.Status != "True" {
			return HealthBad, c.Reason, c.Message
		}
	}
	if seen == 0 {
		return HealthUnknown, "", ""
	}
	return HealthOK, "", ""
}

func newNode(obj *Object) Node {
	h, reason, msg := health(obj)
	return Node{
		ID: nodeID(obj.Kind, obj.Metadata.Name), Kind: obj.Kind, Name: obj.Metadata.Name,
		Health: h, Reason: reason, Message: msg,
	}
}

// missingNode stands in for a reference that resolves to nothing, so a typo in
// `spec.adapter` or a deleted channel shows up as a broken edge instead of
// disappearing from the picture.
func missingNode(kind, name string) Node {
	return Node{
		ID: nodeID(kind, name), Kind: kind, Name: name, Health: HealthBad,
		Reason: "NotFound", Message: "referenced " + Singular[kind] + " " + name + " does not exist",
	}
}

// BuildTopology derives the graph from the cache.
func BuildTopology(c *Cache) Topology {
	nodes := map[string]Node{}
	var edges []Edge
	referenced := map[string]bool{}

	add := func(obj *Object) {
		id := nodeID(obj.Kind, obj.Metadata.Name)
		if _, ok := nodes[id]; !ok {
			nodes[id] = newNode(obj)
		}
	}
	// reference resolves a ref to a node, materializing a placeholder when the
	// target is missing, and reports whether it existed.
	reference := func(kind, name string) bool {
		id := nodeID(kind, name)
		referenced[id] = true
		if obj := c.Get(kind, name); obj != nil {
			if _, ok := nodes[id]; !ok {
				nodes[id] = newNode(obj)
			}
			return true
		}
		if _, ok := nodes[id]; !ok {
			nodes[id] = missingNode(kind, name)
		}
		return false
	}

	// ALL NINE KINDS, not just the wiring spine. "What can this agent actually
	// reach" is a question about MCPToolsets, MCPConfigs and AgentRuntimes, so
	// they are on the graph and the Display panel folds them away — rather than
	// being absent and unfoldable.
	for _, kind := range []string{
		"signalsources", "channels", "agentprofiles", "agentruntimes",
		"pipelines", "channeladapters", "signaladapters", "mcptoolsets", "mcpconfigs",
	} {
		for _, obj := range c.List(kind) {
			add(obj)
		}
	}

	// adapter edges: a Channel/SignalSource names its serving implementation
	for _, kind := range []string{"channels", "signalsources"} {
		adapterKind := "channeladapters"
		if kind == "signalsources" {
			adapterKind = "signaladapters"
		}
		for _, obj := range c.List(kind) {
			spec := decodeSpec[servedSpec](obj.Spec)
			if spec.Adapter == "" {
				continue
			}
			// An adapter that does not exist is the diagnosable case: the
			// channel already reports Served=False, and the broken edge says
			// which name failed to resolve.
			exists := reference(adapterKind, spec.Adapter)
			edges = append(edges, Edge{
				From: nodeID(kind, obj.Metadata.Name), To: nodeID(adapterKind, spec.Adapter),
				Kind: "served-by", Dangling: !exists,
			})
		}
	}

	// wiring edges: sources → pipeline → profile, pipeline → channels
	for _, obj := range c.List("pipelines") {
		spec := decodeSpec[pipelineSpec](obj.Spec)
		pid := nodeID("pipelines", obj.Metadata.Name)
		for _, ref := range spec.SignalSourceRefs {
			exists := reference("signalsources", ref.Name)
			edges = append(edges, Edge{From: nodeID("signalsources", ref.Name), To: pid, Kind: "feeds", Dangling: !exists})
		}
		if spec.ProfileRef.Name != "" {
			exists := reference("agentprofiles", spec.ProfileRef.Name)
			edges = append(edges, Edge{From: pid, To: nodeID("agentprofiles", spec.ProfileRef.Name), Kind: "answers", Dangling: !exists})
		}
		for _, ref := range spec.ChannelRefs {
			exists := reference("channels", ref.Name)
			edges = append(edges, Edge{From: pid, To: nodeID("channels", ref.Name), Kind: "posts", Dangling: !exists})
		}
		// capability edges: the wiring is the ONLY place tool access is
		// declared, so these edges ARE the answer to "what may this route do".
		if spec.Toolsets != nil {
			for _, ref := range spec.Toolsets.Refs {
				exists := reference("mcptoolsets", ref.Name)
				edges = append(edges, Edge{From: pid, To: nodeID("mcptoolsets", ref.Name), Kind: "uses", Dangling: !exists})
			}
		}
		if spec.MCPConfigs != nil {
			for _, ref := range spec.MCPConfigs.Refs {
				exists := reference("mcpconfigs", ref.Name)
				edges = append(edges, Edge{From: pid, To: nodeID("mcpconfigs", ref.Name), Kind: "uses", Dangling: !exists})
			}
		}
	}

	// profile → runtime: what EXECUTES the agent. Runtime selection stays on the
	// profile (a Pipeline choosing a ServiceAccount would make pipeline-edit
	// rights a privilege escalation), so the edge starts at the profile.
	for _, obj := range c.List("agentprofiles") {
		spec := decodeSpec[profileSpec](obj.Spec)
		name := spec.RuntimeRef.Name
		if name == "" {
			if c.Get("agentruntimes", "default") == nil {
				continue // falls back to bootstrap config: no node to point at
			}
			name = "default"
		}
		exists := reference("agentruntimes", name)
		edges = append(edges, Edge{
			From: nodeID("agentprofiles", obj.Metadata.Name), To: nodeID("agentruntimes", name),
			Kind: "uses", Dangling: !exists,
		})
	}

	// live activity per pipeline
	for pid, counts := range activityByPipeline(c) {
		if n, ok := nodes[pid]; ok {
			n.Active, n.Recent = counts.active, counts.recent
			nodes[pid] = n
		}
	}

	// Detached = nothing wires it. For a SignalSource that is the
	// signal-dropping state the Wired condition already reports; drawing it
	// off to the side is what makes it findable without kubectl.
	out := make([]Node, 0, len(nodes))
	for id, n := range nodes {
		if n.Kind == "signalsources" || n.Kind == "channels" {
			n.Detached = !referenced[id]
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
	return Topology{Nodes: out, Edges: edges}
}

type activityCount struct{ active, recent int }

// activityByPipeline counts conversations per pipeline node: active =
// currently inflight, recent = every attributed conversation still present.
func activityByPipeline(c *Cache) map[string]activityCount {
	out := map[string]activityCount{}
	pipelines := c.List("pipelines")
	for _, conv := range c.List("conversations") {
		pipeline := AttributePipeline(conv, pipelines)
		if pipeline == "" {
			continue
		}
		id := nodeID("pipelines", pipeline)
		counts := out[id]
		counts.recent++
		if conversationView(conv).Status.Inflight != nil {
			counts.active++
		}
		out[id] = counts
	}
	return out
}

// LabelPipeline is honored when present, but nothing in this system writes it
// today: a Conversation records the wiring it MATERIALIZED (profileRef,
// channelRefs, toolsets), never the Pipeline that produced it. Attribution is
// therefore inferred — see AttributePipeline.
const LabelPipeline = "agentops.dev/pipeline"

// AttributePipeline maps a Conversation back to the Pipeline that originated
// it, and returns "" when that cannot be established.
//
// A Conversation carries no pipelineRef by design, so this reconstructs the
// link from the bindings it snapshotted: the profile must match and the
// pipeline's channels must all be bound (the router appends the originating
// channel, so the conversation's set can be a superset). An exact channel-set
// match wins over a subset match; genuine ambiguity — two pipelines with the
// same profile and channels — returns "" rather than guessing, and the UI
// shows the conversation as unattributed.
//
// Re-wiring a pipeline detaches its older conversations here. That is honest:
// they were started by wiring that no longer exists.
func AttributePipeline(conv *Object, pipelines []*Object) string {
	if name := conv.Metadata.Labels[LabelPipeline]; name != "" {
		for _, p := range pipelines {
			if p.Metadata.Name == name {
				return name
			}
		}
	}
	view := conversationView(conv)
	convChannels := map[string]bool{}
	for _, ref := range view.Spec.ChannelRefs {
		convChannels[ref.Name] = true
	}
	var exact, subset []string
	for _, p := range pipelines {
		spec := decodeSpec[pipelineSpec](p.Spec)
		if spec.ProfileRef.Name != view.Spec.ProfileRef.Name {
			continue
		}
		covered := true
		for _, ref := range spec.ChannelRefs {
			if !convChannels[ref.Name] {
				covered = false
				break
			}
		}
		if !covered {
			continue
		}
		if len(spec.ChannelRefs) == len(convChannels) {
			exact = append(exact, p.Metadata.Name)
		} else {
			subset = append(subset, p.Metadata.Name)
		}
	}
	if len(exact) == 1 {
		return exact[0]
	}
	if len(exact) == 0 && len(subset) == 1 {
		return subset[0]
	}
	return ""
}

package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// The conversations surface: a filterable list, a full detail view, the
// per-conversation graph and sequence, origination, and replies.

// ConversationFilter is the server-side filter set. Filtering happens HERE, not
// in the browser: an event storm makes thousands of conversations, and shipping
// them all so the client can hide most is how a viewer becomes an API-server
// problem.
type ConversationFilter struct {
	Phase    string
	Pipeline string
	Profile  string
	Channel  string
	Errored  bool
	MaxAge   float64 // seconds; 0 = no bound
	Search   string
}

func (f ConversationFilter) matches(s ConversationSummary, now float64) bool {
	switch {
	case f.Phase != "" && !strings.EqualFold(s.Phase, f.Phase):
		return false
	case f.Pipeline != "" && s.Pipeline != f.Pipeline:
		return false
	case f.Profile != "" && s.Profile != f.Profile:
		return false
	case f.Search != "" && !strings.Contains(strings.ToLower(s.Name+" "+s.Title), strings.ToLower(f.Search)):
		return false
	}
	if f.Channel != "" {
		bound := false
		for _, t := range s.Threads {
			if t.Channel == f.Channel {
				bound = true
			}
		}
		if !bound {
			return false
		}
	}
	if f.Errored && !s.Errored {
		return false
	}
	if f.MaxAge > 0 && s.AgeSeconds > f.MaxAge {
		return false
	}
	return true
}

func parseFilter(r *http.Request) ConversationFilter {
	q := r.URL.Query()
	f := ConversationFilter{
		Phase: q.Get("phase"), Pipeline: q.Get("pipeline"), Profile: q.Get("profile"),
		Channel: q.Get("channel"), Search: q.Get("q"),
	}
	f.Errored, _ = strconv.ParseBool(q.Get("errored"))
	if v, err := strconv.ParseFloat(q.Get("maxAgeSeconds"), 64); err == nil {
		f.MaxAge = v
	}
	return f
}

// conversationPageSize bounds one page.
const conversationPageSize = 50

func (a *API) handleConversations(w http.ResponseWriter, r *http.Request) {
	filter := parseFilter(r)
	limit := conversationPageSize
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 && n <= 500 {
		limit = n
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	pipelines := a.cache.List("pipelines")
	consoleChannel := a.adapter.PrimaryChannel()
	var all []ConversationSummary
	for _, o := range a.cache.List("conversations") {
		s := summarize(o, pipelines, consoleChannel)
		s.RunCount = len(s.Runs)
		// Run history is DROPPED from list rows: a result is a whole agent
		// message, and thousands of them do not belong in a listing.
		s.Runs = nil
		if filter.matches(s, 0) {
			all = append(all, s)
		}
	}
	// newest activity first — the only ordering that stays useful at thousands
	sort.Slice(all, func(i, j int) bool { return all[i].sortKey() > all[j].sortKey() })

	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := all[offset:end]
	if page == nil {
		page = []ConversationSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": page, "total": total, "offset": offset, "limit": limit,
		// facets let the UI build filter dropdowns from what EXISTS rather than
		// from a hardcoded list that drifts from the cluster
		"facets": a.conversationFacets(),
	})
}

func (a *API) conversationFacets() map[string][]string {
	phases, profiles, pipelines, channels := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	pipelineObjs := a.cache.List("pipelines")
	for _, o := range a.cache.List("conversations") {
		v := conversationView(o)
		if v.Status.Phase != "" {
			phases[v.Status.Phase] = true
		}
		if v.Spec.ProfileRef.Name != "" {
			profiles[v.Spec.ProfileRef.Name] = true
		}
		if p := AttributePipeline(o, pipelineObjs); p != "" {
			pipelines[p] = true
		}
		for _, t := range v.Status.Threads {
			channels[t.Channel] = true
		}
	}
	return map[string][]string{
		"phase": sortedKeys(phases), "profile": sortedKeys(profiles),
		"pipeline": sortedKeys(pipelines), "channel": sortedKeys(channels),
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (a *API) handleConversation(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	obj := a.cache.Get("conversations", name)
	if obj == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation " + name + " not found"})
		return
	}
	summary := summarize(obj, a.cache.List("pipelines"), a.adapter.PrimaryChannel())
	var messages []Message
	archived := false
	if summary.ConsoleThread != "" {
		messages = a.transcripts.Thread(summary.ConsoleThread)
		archived = a.transcripts.Archived(summary.ConsoleThread)
	}
	out := map[string]any{
		"conversation": summary,
		"object":       obj,
		"yaml":         objectYAML(obj),
		"transcript":   messages,
		// archived: a close-topic op ended this thread. The transcript stays
		// readable; there is just nothing left to reply to.
		"archived": archived,
		"events":   a.activity.ForConversation(name),
	}
	if pod := a.cache.Get("pods", summary.RuntimePod); pod != nil {
		ps := decodeSpec[podStatusView](pod.Status)
		out["runtimePodStatus"] = map[string]any{
			"phase": ps.Phase, "problem": podProblem(ps), "node": decodeSpec[podSpecView](pod.Spec).NodeName,
		}
	}
	// When the console is NOT joined, say why and hand over the exact patch.
	// The console never edits a Pipeline — showing the edit is the answer.
	if !summary.Joined {
		out["joinHint"] = a.joinHint(summary)
	}
	writeJSON(w, http.StatusOK, out)
}

// joinHint explains why there is no composer and what to change.
func (a *API) joinHint(s ConversationSummary) map[string]string {
	consoleChannel := a.adapter.PrimaryChannel()
	if consoleChannel == "" {
		return map[string]string{
			"reason": "this console serves no Channel, so it can hold no thread",
			"fix":    "create a Channel with spec.adapter: " + a.adapterName,
		}
	}
	if s.Pipeline == "" {
		return map[string]string{
			"reason": "this conversation is not attributable to a pipeline (its bindings match none, or match several), " +
				"so there is no single wiring to patch",
			"fix": "add " + consoleChannel + " to the channelRefs of the Pipeline that originated it",
		}
	}
	return map[string]string{
		"reason": "the console Channel is not in this conversation's pipeline, so no console thread was created",
		"fix": "kubectl patch pipeline " + s.Pipeline + " --type=json -p " +
			`'[{"op":"add","path":"/spec/channelRefs/-","value":{"name":"` + consoleChannel + `"}}]'`,
		"note": "this affects NEW conversations only — bindings are materialized at creation",
	}
}

// ---- per-conversation graph ---------------------------------------------------

// ConversationGraph is the forensic view: what this conversation involved and
// what moved between those things.
type ConversationGraph struct {
	Topology
	Events []ActivityEvent `json:"events"`
	// Diverged reports that the attributed Pipeline's CURRENT wiring differs
	// from what this conversation recorded. The graph still shows what it RAN
	// with — that is the question a per-conversation graph exists to answer:
	// what could this agent reach when it did that?
	Diverged bool     `json:"diverged"`
	Pipeline string   `json:"pipeline,omitempty"`
	Drift    []string `json:"drift,omitempty"`
}

func (a *API) handleConversationGraph(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	obj := a.cache.Get("conversations", name)
	if obj == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation " + name + " not found"})
		return
	}
	writeJSON(w, http.StatusOK, a.conversationGraph(obj))
}

// conversationGraph builds the elements from the Conversation's OWN recorded
// bindings, never from the Pipeline's current spec.
//
// This is the load-bearing choice. A Conversation snapshots what it
// materialized, so after a re-wire this graph still shows the capabilities that
// run actually had — and reports that the current wiring differs. Reading the
// live Pipeline instead would silently rewrite history, and the forensic value
// of a per-conversation graph is precisely that it does not.
func (a *API) conversationGraph(obj *Object) ConversationGraph {
	v := conversationView(obj)
	convID := nodeID("conversations", obj.Metadata.Name)
	g := ConversationGraph{}
	g.Nodes = []Node{{
		ID: convID, Kind: "conversations", Name: obj.Metadata.Name,
		Health: HealthNone,
	}}
	g.Edges = []Edge{}

	addRef := func(kind, name, edgeKind string, from string) {
		if name == "" {
			return
		}
		id := nodeID(kind, name)
		if o := a.cache.Get(kind, name); o != nil {
			g.Nodes = append(g.Nodes, newNode(o))
		} else {
			// The object is GONE but the conversation recorded it. That is a
			// fact worth drawing, not an omission: the run used it.
			g.Nodes = append(g.Nodes, missingNode(kind, name))
		}
		g.Edges = append(g.Edges, Edge{From: from, To: id, Kind: edgeKind,
			Dangling: a.cache.Get(kind, name) == nil})
	}

	profile := v.Spec.ProfileRef.Name
	addRef("agentprofiles", profile, "answers", convID)
	if prof := a.cache.Get("agentprofiles", profile); prof != nil {
		runtime := decodeSpec[profileSpec](prof.Spec).RuntimeRef.Name
		if runtime == "" && a.cache.Get("agentruntimes", "default") != nil {
			runtime = "default"
		}
		addRef("agentruntimes", runtime, "uses", nodeID("agentprofiles", profile))
	}
	for _, ref := range v.Spec.ChannelRefs {
		addRef("channels", ref.Name, "posts", convID)
		if ch := a.cache.Get("channels", ref.Name); ch != nil {
			if adapter := decodeSpec[servedSpec](ch.Spec).Adapter; adapter != "" {
				addRef("channeladapters", adapter, "served-by", nodeID("channels", ref.Name))
			}
		}
	}
	// The capability layer the conversation MATERIALIZED — the whole reason this
	// graph is built from recorded bindings.
	for _, ref := range v.Spec.Toolsets.refs() {
		addRef("mcptoolsets", ref, "uses", convID)
	}
	for _, ref := range v.Spec.MCPConfigs.refs() {
		addRef("mcpconfigs", ref, "uses", convID)
	}
	if v.Status.RuntimePod != "" {
		if pod := a.cache.Get("pods", v.Status.RuntimePod); pod != nil {
			g.Nodes = append(g.Nodes, Node{
				ID: nodeID("pods", v.Status.RuntimePod), Kind: "pods", Name: v.Status.RuntimePod,
				Health: podHealth(pod),
			})
			g.Edges = append(g.Edges, Edge{From: convID, To: nodeID("pods", v.Status.RuntimePod), Kind: "uses"})
		}
	}

	g.Events = a.activity.ForConversation(obj.Metadata.Name)
	g.Pipeline = AttributePipeline(obj, a.cache.List("pipelines"))
	if g.Pipeline != "" {
		if p := a.cache.Get("pipelines", g.Pipeline); p != nil {
			g.Drift = wiringDrift(decodeSpec[pipelineSpec](p.Spec), v)
			g.Diverged = len(g.Drift) > 0
		}
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	return g
}

func podHealth(pod *Object) Health {
	if podProblem(decodeSpec[podStatusView](pod.Status)) != "" {
		return HealthBad
	}
	return HealthOK
}

// wiringDrift lists the ways the pipeline's current spec differs from what the
// conversation recorded.
func wiringDrift(now pipelineSpec, conv convView) []string {
	var drift []string
	if now.ProfileRef.Name != conv.Spec.ProfileRef.Name {
		drift = append(drift, "profile: now "+now.ProfileRef.Name+", ran as "+conv.Spec.ProfileRef.Name)
	}
	if a, b := refNames(now.ChannelRefs), refNames(conv.Spec.ChannelRefs); !sameSet(a, b) {
		drift = append(drift, "channels: now ["+strings.Join(a, " ")+"], ran with ["+strings.Join(b, " ")+"]")
	}
	nowTools := []string{}
	if now.Toolsets != nil {
		nowTools = refNames(now.Toolsets.Refs)
	}
	if ran := conv.Spec.Toolsets.refs(); !sameOrder(nowTools, ran) {
		drift = append(drift, "toolsets: now ["+strings.Join(nowTools, " ")+"], ran with ["+strings.Join(ran, " ")+"]")
	}
	nowMCP := []string{}
	if now.MCPConfigs != nil {
		nowMCP = refNames(now.MCPConfigs.Refs)
	}
	if ran := conv.Spec.MCPConfigs.refs(); !sameOrder(nowMCP, ran) {
		drift = append(drift, "mcpConfigs: now ["+strings.Join(nowMCP, " ")+"], ran with ["+strings.Join(ran, " ")+"]")
	}
	return drift
}

func refNames(refs []Ref) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}

func sameOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

// ---- writes -------------------------------------------------------------------

// startRequest is POST /api/conversations. It names a SOURCE, never a pipeline:
// who answers is DECLARED by the Pipeline that claimed that source, which is
// the origination invariant's actual point. A pipeline field here would put the
// choice back in the caller's hands.
type startRequest struct {
	Source string `json:"source,omitempty"`
	Task   string `json:"task"`
}

func (a *API) handleStart(w http.ResponseWriter, r *http.Request) {
	var in startRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `need {"task":"…"}`})
		return
	}
	if strings.TrimSpace(in.Task) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task is required"})
		return
	}
	if a.originator == nil {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "this console holds no signal identity, so it cannot start conversations",
			"fix":   "declare a SignalAdapter with servedBy pointing at this ChannelAdapter, plus a SignalSource",
		})
		return
	}
	source := in.Source
	if source == "" {
		source = a.originator.Source()
	}
	if source != a.originator.Source() {
		// One console holds ONE signal identity, so it can originate from one
		// source. Naming another is refused rather than silently redirected.
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "this console originates only from source " + a.originator.Source(),
		})
		return
	}
	// Refuse BEFORE posting when nothing claims the source, so the operator gets
	// the Wired=False reason rather than a signal that vanishes.
	if src := a.cache.Get("signalsources", source); src != nil {
		if c := src.Condition("Wired"); c != nil && c.Status != "True" {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":  "this console's signal source is not claimed by any Ready Pipeline, so nothing would answer",
				"reason": c.Reason, "message": c.Message,
				"fix": "add " + source + " to a Ready Pipeline's signalSourceRefs",
			})
			return
		}
	}

	reason, err := a.originator.Start(r.Context(), a.adapter.PrimaryChannel(), Identity(r), in.Task)
	if err != nil {
		log.Printf("origination failed: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if reason != "" {
		// The manager dropped it and said why. That reason is the answer, not an
		// error to translate.
		writeJSON(w, http.StatusConflict, map[string]string{"error": reason})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{
		"source": source,
		"note":   "the claiming Pipeline answers; the conversation appears in the list once created",
	})
}

func (a *API) handleSend(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `need {"text":"…"}`})
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "text is required"})
		return
	}
	msg, err := a.adapter.Send(r.Context(), r.PathValue("name"), Identity(r), in.Text)
	if err != nil {
		if errors.Is(err, errNotJoined) {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "this conversation has no console thread — add the console channel to its pipeline's channels[] to join it",
			})
			return
		}
		log.Printf("inbound send: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "sending failed"})
		return
	}
	writeJSON(w, http.StatusAccepted, msg)
}

// plural renders "1 source" / "2 sources".
func plural(n int, word string) string {
	s := word
	if n != 1 {
		s += "s"
	}
	return strconv.Itoa(n) + " " + s
}

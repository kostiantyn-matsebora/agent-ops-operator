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
	// Unread narrows to conversations whose CONSOLE thread has activity newer
	// than its watermark. Evaluated server-side like every other filter, so a
	// narrowed list still reports a correct total and pages correctly.
	Unread bool
	MaxAge float64 // seconds; 0 = no bound
	Search string
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
	if f.Unread && !s.Unread {
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
	f.Unread, _ = strconv.ParseBool(q.Get("unread"))
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
	// Unreadness is answered for WHOEVER IS ASKING. With no salt projected, or
	// under a shared token, this is "" and every viewer gets the channel-wide
	// answer — which is the behaviour before per-identity marks existed.
	reader := a.adapter.ReaderKey(Identity(r))
	var all []ConversationSummary
	unreadTotal := 0
	for _, o := range a.cache.List("conversations") {
		s := summarize(o, pipelines, consoleChannel, reader)
		s.RunCount = len(s.Runs)
		// Run history is DROPPED from list rows: a result is a whole agent
		// message, and thousands of them do not belong in a listing.
		s.Runs = nil
		// Counted BEFORE the filter, always: a count that moved because the
		// view narrowed would let a filter hide a backlog without saying so.
		if s.Unread {
			unreadTotal++
		}
		if filter.matches(s, 0) {
			all = append(all, s)
		}
	}
	// newest activity first — the only ordering that stays useful at thousands
	sort.Slice(all, func(i, j int) bool { return all[i].sortKey() > all[j].sortKey() })

	total := len(all)
	// count-only: the navigation badge wants the number, not a page of rows it
	// would immediately throw away.
	if ok, _ := strconv.ParseBool(r.URL.Query().Get("count")); ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"items": []ConversationSummary{}, "total": total, "unreadTotal": unreadTotal,
			"offset": 0, "limit": 0,
		})
		return
	}
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
		"unreadTotal": unreadTotal,
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
	out := a.ConversationView(obj, a.adapter.ReaderKey(Identity(r)))
	summary := out["conversation"].(ConversationSummary)
	var messages []Message
	if summary.ConsoleThread != "" {
		messages = a.transcripts.Thread(summary.ConsoleThread)
		// MERGE with the durable record, always — never only when the buffer
		// looks empty. Conditioning on emptiness broke the moment a reader
		// typed a reply: their own message made the buffer non-empty, the
		// durable answers stopped being served, and the history vanished
		// mid-conversation.
		messages = mergeTranscript(summary.ConsoleThread, a.adapter.PrimaryChannel(), messages, summary.Runs)
	}
	out["transcript"] = messages
	out["events"] = a.activity.ForConversation(name)
	writeJSON(w, http.StatusOK, out)
}

// ConversationView projects one conversation into what its page shows, MINUS
// the two parts that arrive on streams of their own: the transcript, which the
// message events append to, and the activity events.
//
// The stream sends this on a conversations delta, so a browser holding the page
// writes the new phase, run and bindings straight in. Splitting the transcript
// out is what keeps that cheap: a run advancing changes the object, and the
// answer it produced arrives once, as a message, rather than again inside every
// later delta.
func (a *API) ConversationView(obj *Object, reader string) map[string]any {
	summary := summarize(obj, a.cache.List("pipelines"), a.adapter.PrimaryChannel(), reader)
	// Archived — "there is nothing here to reply to" — is read from the
	// CONVERSATION's phase first, and only then from this console's own
	// transcript state.
	//
	// The transcript flag is set when a close-topic op is handled and lives in
	// memory for one console session. That was sufficient while closing DELETED
	// the conversation: the row disappeared, so nothing had to survive a
	// restart. Closing is now a state the object keeps, and a console restart
	// wiped the flag off every closed conversation and put the composer back —
	// so a closed conversation looked typeable, and the manager answered each
	// message with "this conversation is closed".
	//
	// The CR is the source of truth and survives everything; the flag stays as
	// the transitional signal for a thread archived before the phase was
	// observed.
	archived := strings.EqualFold(summary.Phase, "Closed")
	if summary.ConsoleThread != "" {
		archived = archived || a.transcripts.Archived(summary.ConsoleThread)
	}
	out := map[string]any{
		"conversation": summary,
		"object":       obj,
		"yaml":         objectYAML(obj),
		// archived: a close-topic op ended this thread. The transcript stays
		// readable; there is just nothing left to reply to.
		"archived": archived,
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
	return out
}

// ConversationRow projects one conversation into the row its listing shows.
//
// Run history is dropped exactly as the listing drops it: a result is a whole
// agent message, and a delta that carried thousands of them per change would be
// heavier than the re-fetch it replaces.
func (a *API) ConversationRow(obj *Object, reader string) ConversationSummary {
	s := summarize(obj, a.cache.List("pipelines"), a.adapter.PrimaryChannel(), reader)
	s.RunCount = len(s.Runs)
	s.Runs = nil
	return s
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

	// Nothing is remembered here. The message this starts comes BACK as an
	// ordinary delivery carrying its own sender, and a reload reads it from the
	// conversation's record — which is what the hint map, the text matching and
	// the as-typed recovery were all standing in for.
	reason, err := a.originator.Start(r.Context(), a.adapter.PrimaryChannel(), Identity(r),
		a.adapter.ReaderKey(Identity(r)), in.Task)
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
	// Your own message advances YOUR OWN watermark and nobody else's. Without
	// this the conversation you just replied to comes back unread to you the
	// moment the manager stamps lastActivity — while staying correctly unread
	// for colleagues who have not seen the exchange.
	a.stampRead(r, r.PathValue("name"))
	writeJSON(w, http.StatusAccepted, msg)
}

// stampRead marks a conversation read for the acting reader, best-effort: it is
// bookkeeping about what that person has already seen, so a failure must never
// fail the action that caused it. Silent when the console resolves no reader
// (no salt, or an unidentified request) — there is no per-person mark to move,
// and advancing the channel-wide one would clear the badge for everybody.
func (a *API) stampRead(r *http.Request, name string) {
	reader := a.adapter.ReaderKey(Identity(r))
	if reader == "" {
		return
	}
	obj := a.cache.Get("conversations", name)
	if obj == nil {
		return
	}
	s := summarize(obj, a.cache.List("pipelines"), a.adapter.PrimaryChannel(), reader)
	if s.ConsoleThread == "" {
		return
	}
	if _, err := a.adapter.ReportRead(r.Context(), reader,
		[]ReadReport{{Conversation: name, ReadAt: s.sortKey()}}); err != nil {
		log.Printf("stamp read %s: %v", name, err)
	}
}

// ---- bulk close ---------------------------------------------------------------

// closeRequest is POST /api/conversations/close. It carries NAMES and a flag,
// and nothing else: no filter, no query, no "everything matching". What may be
// closed is what the operator selected, so a mis-set filter can never close more
// than one screen of conversations.
//
// IncludeWorking is the opt-in that abandons in-progress runs. It is the only
// thing the caller decides — the phase itself is read server-side, because a
// phase asserted by the browser is stale by definition and would make the caller
// the author of its own authorization.
type closeRequest struct {
	Names          []string `json:"names"`
	IncludeWorking bool     `json:"includeWorking,omitempty"`
}

// Close outcomes. `skipped` is a conversation this console could not or would
// not close and said why; `failed` is one it tried to close and could not.
const (
	closeOutcomeClosed  = "closed"
	closeOutcomeSkipped = "skipped"
	closeOutcomeFailed  = "failed"
)

// CloseResult is one conversation's outcome. A batch reports one of these per
// requested name — "12 of 15 closed" is the only honest summary of a batch, and
// a single verdict cannot carry the reasons.
type CloseResult struct {
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason,omitempty"`
}

// handleBulkClose ends a batch of conversations by posting `/close` on each
// one's console thread — the same command, the same code path, the same
// archiving and teardown a person typing it gets.
//
// It is a FAN-OUT OF `/close`, deliberately, and not a manager-side batch verb:
// a second implementation of ending a conversation would be free to drift from
// the first, and the first divergence (a farewell not posted, a finalizer not
// run, a slot not released) would be found in production. It also keeps the
// console's defining invariant literally true — its only write anywhere is
// POST /channel/inbound.
//
// The walk is sequential and never aborts: a failing name is recorded and the
// walk continues, because a batch that stops on its first bad row is a batch
// nobody can reason about.
func (a *API) handleBulkClose(w http.ResponseWriter, r *http.Request) {
	var in closeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `need {"names":["…"]}`})
		return
	}
	if len(in.Names) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "names is required"})
		return
	}
	// The cap is the list page size, enforced HERE and not only in the selection
	// UI: the blast radius must equal one screen of conversations regardless of
	// what the client sends.
	if len(in.Names) > conversationPageSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "a close batch is limited to " + strconv.Itoa(conversationPageSize) + " conversations",
		})
		return
	}

	identity := Identity(r)
	results := make([]CloseResult, 0, len(in.Names))
	for _, name := range in.Names {
		results = append(results, a.closeOne(r, name, in.IncludeWorking, identity))
	}
	totals := map[string]int{closeOutcomeClosed: 0, closeOutcomeSkipped: 0, closeOutcomeFailed: 0}
	for _, res := range results {
		totals[res.Outcome]++
	}
	// 200 for a mixed batch: the request was executed, and a partial result is
	// the normal outcome rather than a transport failure.
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"closed":  totals[closeOutcomeClosed],
		"skipped": totals[closeOutcomeSkipped],
		"failed":  totals[closeOutcomeFailed],
	})
}

// closeOne decides and performs one conversation's close. Every decision is
// taken from cached CR state — the client supplies names and a flag, nothing
// else.
func (a *API) closeOne(r *http.Request, name string, includeWorking bool, identity string) CloseResult {
	obj := a.cache.Get("conversations", name)
	if obj == nil {
		return CloseResult{Name: name, Outcome: closeOutcomeFailed,
			Reason: "no such conversation — it may already have been closed"}
	}
	if obj.Metadata.DeletionTimestamp != "" {
		return CloseResult{Name: name, Outcome: closeOutcomeSkipped,
			Reason: "already closing"}
	}
	if !includeWorking && strings.EqualFold(conversationView(obj).Status.Phase, "Working") {
		return CloseResult{Name: name, Outcome: closeOutcomeSkipped,
			Reason: "working — closing it would abandon the run in progress"}
	}
	if _, _, ok := a.adapter.ThreadFor(name); !ok {
		return CloseResult{Name: name, Outcome: closeOutcomeSkipped, Reason: notJoinedReason}
	}
	if _, err := a.adapter.Send(r.Context(), name, identity, "/close"); err != nil {
		if errors.Is(err, errNotJoined) {
			// lost the thread between the check and the post — the same answer
			return CloseResult{Name: name, Outcome: closeOutcomeSkipped, Reason: notJoinedReason}
		}
		log.Printf("bulk close %s: %v", name, err)
		return CloseResult{Name: name, Outcome: closeOutcomeFailed, Reason: "closing failed"}
	}
	log.Printf("console write: action=bulk-close identity=%s conversation=%s", identity, name)
	return CloseResult{Name: name, Outcome: closeOutcomeClosed}
}

// handleMarkRead marks the selected conversations' console threads read.
//
// The request carries NAMES ONLY. The watermark for each is read here off the
// conversation's own cached state, so the browser cannot report having seen
// activity it never rendered — and the manager clamps and enforces
// monotonicity on top of that.
//
// Selection-scoped and bounded at one page, for the reason bulk close is: there
// is no "mark everything matching the filter", because the blast radius must
// equal one screen of conversations.
func (a *API) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Names []string `json:"names"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `need {"names":["…"]}`})
		return
	}
	if len(in.Names) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "names is required"})
		return
	}
	if len(in.Names) > conversationPageSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "a read batch is limited to " + strconv.Itoa(conversationPageSize) + " conversations",
		})
		return
	}

	consoleChannel := a.adapter.PrimaryChannel()
	pipelines := a.cache.List("pipelines")
	reader := a.adapter.ReaderKey(Identity(r))
	reports := make([]ReadReport, 0, len(in.Names))
	results := make([]ReadResult, 0, len(in.Names))
	for _, name := range in.Names {
		obj := a.cache.Get("conversations", name)
		if obj == nil {
			results = append(results, ReadResult{Name: name, Outcome: closeOutcomeFailed,
				Reason: "no such conversation"})
			continue
		}
		s := summarize(obj, pipelines, consoleChannel, reader)
		reports = append(reports, ReadReport{Conversation: name, ReadAt: s.sortKey()})
	}
	if len(reports) > 0 {
		out, err := a.adapter.ReportRead(r.Context(), reader, reports)
		if err != nil {
			log.Printf("mark read: %v", err)
			for _, rep := range reports {
				results = append(results, ReadResult{Name: rep.Conversation,
					Outcome: closeOutcomeFailed, Reason: "marking read failed"})
			}
		} else {
			results = append(results, out...)
		}
	}
	totals := map[string]int{}
	for _, res := range results {
		totals[res.Outcome]++
	}
	// Attributed like every other action this console takes, and NOT behind the
	// write gate: a watermark instructs no agent and starts no work.
	log.Printf("console read: action=mark-read identity=%s conversations=%d", Identity(r), len(in.Names))
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"marked":  totals["marked"], "skipped": totals[closeOutcomeSkipped], "failed": totals[closeOutcomeFailed],
	})
}

// notJoinedReason names the FIX rather than the failure: reach is bounded by
// the threads this console holds, and the binding that would extend it is the
// pipeline's channels[].
const notJoinedReason = "the console holds no thread on this conversation — " +
	"add the console channel to its pipeline's channels[] to join it"

// Agent is one addressable pipeline offered by the composer's typeahead.
// Entry is one thing a person may type, as the composer offers it.
//
// NAMED FOR WHAT IT IS. It used to be `Agent`, which was wrong twice: an agent
// in this project is a DEFINITION inside a profile's repository, and this list
// has never held one. What a message addresses is a PIPELINE.
type Entry struct {
	// Kind is `builtin` or `pipeline`.
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Description is menu text — for a pipeline, the profile answering for it.
	Description string `json:"description,omitempty"`
	// Icon is how the entry is recognised in a list — an emoji, or nothing.
	// Drawn by the surface; the manager interprets it no further.
	Icon string `json:"icon,omitempty"`
	// Position is `general` (the composer that starts a conversation) or
	// `thread` (the one attached to an existing conversation). The two take
	// disjoint sets, and the console is a surface that can honour the
	// difference.
	Position string `json:"position"`
	// Profile is what tells two pipelines apart when their names do not.
	Profile string `json:"profile,omitempty"`
}

// handleVocabulary lists what a message can address, for the composer
// typeahead.
//
// READY ONLY, and that is not a detail: `/agents` filters the same way, so
// filtering differently here would make the surface answer one question two
// ways. An unready pipeline names wiring that does not resolve, so offering it
// would invite a request that cannot be served.
//
// Everything comes from the cache the console already list/watches — no new
// RBAC, no manager endpoint, no CRD field. The listing is ADVISORY: a stale
// entry produces an addressed message to an unknown pipeline, which the router
// already answers with "unknown agent".
//
// It is not scoped to the console's own signal source, because addressing is
// not scoped either — a command resolves by NAME, with no claim check. Scoping
// the list would hide pipelines that would in fact answer.
func (a *API) handleVocabulary(w http.ResponseWriter, r *http.Request) {
	// PROJECTED FROM THE MANAGER'S OWN VOCABULARY, not walked from the Pipeline
	// cache a second time. The typeahead here and a chat transport's command
	// menu answer the same question, and two derivations of one fact drift —
	// which is the whole reason the manager publishes it.
	if a.mgr != nil {
		if v, err := a.mgr.Vocabulary(r.Context()); err == nil {
			out := make([]Entry, 0, len(v.Entries))
			for _, e := range v.Entries {
				out = append(out, Entry{
					Kind: e.Kind, Name: e.Name, Description: e.Description,
					Icon: e.Icon, Position: e.Position, Profile: e.Profile,
				})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
			writeJSON(w, http.StatusOK, map[string]any{"entries": out, "revision": v.Revision})
			return
		}
	}
	// The manager is unreachable or predates the vocabulary. Fall back to the
	// Pipeline view this console already holds: a composer with no listing is
	// worse than one built from a second source, and this path offers exactly
	// what the old endpoint did.
	out := []Entry{}
	for _, p := range a.cache.List("pipelines") {
		if c := p.Condition("Ready"); c == nil || c.Status != "True" {
			continue
		}
		profile := decodeSpec[pipelineSpec](p.Spec).ProfileRef.Name
		out = append(out, Entry{
			Kind: "pipeline", Name: p.Metadata.Name, Position: "general",
			Description: profile, Profile: profile,
			Icon: decodeSpec[pipelineSpec](p.Spec).Icon,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}

// plural renders "1 source" / "2 sources".
func plural(n int, word string) string {
	s := word
	if n != 1 {
		s += "s"
	}
	return strconv.Itoa(n) + " " + s
}

// ---- delete and reopen --------------------------------------------------------
//
// Both exist because closing stopped deleting. A CLOSED conversation is inert
// but intact: it holds no thread, so neither verb can be a command posted on
// one, and both are manager verbs the console CALLS. The console still performs
// no Kubernetes write.

const deleteOutcomeDeleted = "deleted"

// handleBulkDelete reclaims a batch of CLOSED conversations.
//
// Mirrors bulk close in every mechanical respect — the same page-size bound
// enforced server-side, the same explicit names, the same per-item outcomes and
// totals, the same never-abort walk. What it does NOT mirror is implicitness: a
// name that is not already Closed is SKIPPED with "close it first", never
// closed on the way through. A close-then-delete batch would do the
// irreversible thing to conversations that were still working, behind a
// confirmation that named only the delete.
func (a *API) handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	var in closeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `need {"names":["…"]}`})
		return
	}
	if len(in.Names) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "names is required"})
		return
	}
	if len(in.Names) > conversationPageSize {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "a delete batch is limited to " + strconv.Itoa(conversationPageSize) + " conversations",
		})
		return
	}

	identity := Identity(r)
	results := make([]CloseResult, 0, len(in.Names))
	for _, name := range in.Names {
		results = append(results, a.deleteOne(r, name, identity))
	}
	totals := map[string]int{deleteOutcomeDeleted: 0, closeOutcomeSkipped: 0, closeOutcomeFailed: 0}
	for _, res := range results {
		totals[res.Outcome]++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"deleted": totals[deleteOutcomeDeleted],
		"skipped": totals[closeOutcomeSkipped],
		"failed":  totals[closeOutcomeFailed],
	})
}

// deleteOne decides and performs one conversation's deletion. Every decision is
// taken from cached CR state; the client supplies names and nothing else.
func (a *API) deleteOne(r *http.Request, name, identity string) CloseResult {
	obj := a.cache.Get("conversations", name)
	if obj == nil {
		return CloseResult{Name: name, Outcome: closeOutcomeFailed,
			Reason: "no such conversation — it may already have been deleted"}
	}
	if obj.Metadata.DeletionTimestamp != "" {
		return CloseResult{Name: name, Outcome: closeOutcomeSkipped, Reason: "already deleting"}
	}
	if phase := conversationView(obj).Status.Phase; !strings.EqualFold(phase, "Closed") {
		return CloseResult{Name: name, Outcome: closeOutcomeSkipped,
			Reason: "close it first — deleting removes its recorded answers, which is not something to do to a live conversation"}
	}
	if err := a.adapter.Delete(r.Context(), name); err != nil {
		if errors.Is(err, errNotJoined) {
			return CloseResult{Name: name, Outcome: closeOutcomeSkipped, Reason: notJoinedReason}
		}
		log.Printf("bulk delete %s: %v", name, err)
		return CloseResult{Name: name, Outcome: closeOutcomeFailed, Reason: "deleting failed"}
	}
	log.Printf("console write: action=bulk-delete identity=%s conversation=%s", identity, name)
	return CloseResult{Name: name, Outcome: deleteOutcomeDeleted}
}

// handleReopen brings ONE closed conversation back. Per-row on purpose: a bulk
// reopen would re-materialise threads across every bound channel at once.
func (a *API) handleReopen(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	obj := a.cache.Get("conversations", name)
	if obj == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such conversation"})
		return
	}
	if phase := conversationView(obj).Status.Phase; !strings.EqualFold(phase, "Closed") {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "only a closed conversation can be reopened; this one is " + strings.ToLower(phase)})
		return
	}
	if err := a.adapter.Reopen(r.Context(), name); err != nil {
		if errors.Is(err, errNotJoined) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": notJoinedReason})
			return
		}
		// The manager refuses a reopen whose wiring is gone and NAMES the
		// missing object; pass that through rather than flattening it, because
		// "reopen failed" sends nobody anywhere.
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	log.Printf("console write: action=reopen identity=%s conversation=%s", Identity(r), name)
	writeJSON(w, http.StatusOK, map[string]string{"outcome": "reopened", "name": name})
}

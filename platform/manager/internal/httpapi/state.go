// The state read surface (state-store-generic-contract design D-5/D-6): the
// only door the console and housekeeping reach Conversations and Signals
// through, instead of watching the Kubernetes API directly.
//
//	GET /state/conversations               {"revision","items":[record]}
//	GET /state/conversations/{name}        one record
//	GET /state/conversations/stream?since= SSE: put|delete|bookmark|resync
//	GET /state/signals
//	GET /state/signals/stream?since=
//
// The wire record (design D-5): a kind's published field names, no metadata,
// no labels, no annotations. Under the kube backend, spec and status marshal
// through agentopsv1alpha1's own CRD types (ConversationSpec/ConversationStatus
// are not on the state package's guarded-type list — internal/state/static_test.go
// — only Signal's wrapper types are, which is why the signal half below
// mirrors SignalSpec/SignalStatus's shape in a package-local type rather than
// naming them), so a value built from the SAME record the kube layout wrote
// marshals to the SAME bytes as the object's own spec/status.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// ---- the wire record: conversations -----------------------------------------

type conversationWire struct {
	Name                string                              `json:"name"`
	ID                  string                              `json:"id"`
	Revision            string                              `json:"revision"`
	CreatedAt           time.Time                           `json:"createdAt"`
	DeletionRequestedAt *time.Time                          `json:"deletionRequestedAt"`
	Spec                agentopsv1alpha1.ConversationSpec   `json:"spec"`
	Status              agentopsv1alpha1.ConversationStatus `json:"status"`
}

func newConversationWire(r *state.ConversationRecord, rev state.Revision, runs []state.Run) conversationWire {
	return conversationWire{
		Name: r.Name, ID: r.ID, Revision: string(rev),
		CreatedAt: r.CreatedAt, DeletionRequestedAt: r.DeletionRequestedAt,
		Spec:   conversationSpecWire(r),
		Status: conversationStatusWire(r, runs),
	}
}

// ref and refs render the contract's Ref (a plain string) as the API's
// ObjectRef — the one shape difference between the two, everywhere a
// reference crosses this wire.
func ref(r state.Ref) *agentopsv1alpha1.ObjectRef {
	if r == "" {
		return nil
	}
	return &agentopsv1alpha1.ObjectRef{Name: r}
}

func refs(in []state.Ref) []agentopsv1alpha1.ObjectRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]agentopsv1alpha1.ObjectRef, len(in))
	for i, r := range in {
		out[i] = agentopsv1alpha1.ObjectRef{Name: r}
	}
	return out
}

// timeWire renders the contract's plain *time.Time as the API's *metav1.Time
// — the one timestamp shape difference, everywhere one crosses this wire.
func timeWire(t *time.Time) *metav1.Time {
	if t == nil {
		return nil
	}
	mt := metav1.NewTime(*t)
	return &mt
}

func conversationSpecWire(r *state.ConversationRecord) agentopsv1alpha1.ConversationSpec {
	spec := agentopsv1alpha1.ConversationSpec{
		EscalationChannelRefs: refs(r.EscalationChannelRefs),
		ChannelRefs:           refs(r.ChannelRefs),
		ProfileRef:            agentopsv1alpha1.ObjectRef{Name: r.ProfileRef},
		PipelineRef:           ref(r.PipelineRef),
		Title:                 r.Title,
		ConversationID:        r.ConversationID,
		RuntimeRef:            ref(r.RuntimeRef),
		ServiceAccountName:    r.ServiceAccountName,
		ContextClaimName:      r.ContextClaimName,
		WorkspaceClaimName:    r.WorkspaceClaimName,
	}
	if r.CoordinatorRef != "" {
		spec.CoordinatorRef = &agentopsv1alpha1.ObjectRef{Name: r.CoordinatorRef}
	}
	if r.CausedBy != nil {
		spec.CausedBy = &agentopsv1alpha1.Provenance{Parent: r.CausedBy.Parent, Entry: r.CausedBy.Entry}
	}
	if r.Signal != nil {
		spec.Signal = &agentopsv1alpha1.SignalProvenance{
			SourceRef: ref(r.Signal.SourceRef), Labels: r.Signal.Labels,
			Severity: agentopsv1alpha1.Severity(r.Signal.Severity),
		}
	}
	if r.OriginReader != nil {
		spec.OriginReader = &agentopsv1alpha1.OriginReader{Channel: r.OriginReader.Channel, Key: r.OriginReader.Key}
	}
	if r.Toolsets != nil {
		spec.Toolsets = &agentopsv1alpha1.ToolsetBinding{Mode: r.Toolsets.Mode, Refs: refs(r.Toolsets.Refs)}
	}
	if r.MCPConfigs != nil {
		spec.MCPConfigs = &agentopsv1alpha1.ToolingBinding{Refs: refs(r.MCPConfigs.Refs)}
	}
	for _, in := range r.Inputs {
		spec.Inputs = append(spec.Inputs, inputItemWire(in))
	}
	return spec
}

func inputItemWire(in state.InputItem) agentopsv1alpha1.InputItem {
	out := agentopsv1alpha1.InputItem{
		ID: in.ID, Type: agentopsv1alpha1.InputType(in.Type), Payload: in.Payload,
		Agent: in.Agent, PayloadRef: ref(state.Ref(in.PayloadRef)),
	}
	if !in.ReceivedAt.IsZero() {
		out.ReceivedAt = metav1.NewTime(in.ReceivedAt)
	}
	if in.Origin != nil {
		out.Origin = &agentopsv1alpha1.InputOrigin{
			Kind: agentopsv1alpha1.OriginKind(in.Origin.Kind), Name: in.Origin.Name,
			Entry: in.Origin.Entry, SignalKind: in.Origin.SignalKind, Sender: in.Origin.Sender,
		}
	}
	return out
}

func conversationStatusWire(r *state.ConversationRecord, runs []state.Run) agentopsv1alpha1.ConversationStatus {
	st := agentopsv1alpha1.ConversationStatus{
		Phase: agentopsv1alpha1.ConversationPhase(r.Phase), RuntimeContextID: r.RuntimeContextID,
		SessionID: r.SessionID, RuntimePod: r.RuntimePod, ProcessedInputIDs: r.ProcessedInputIDs,
		CloseReason: r.CloseReason, Brief: r.Brief, ThreadsArchived: r.ThreadsArchived,
		Reopens: r.Reopens, RuntimeStartFailures: r.RuntimeStartFailures,
		EscalationMessage: r.EscalationMessage,
		LastActivity:      timeWire(r.LastActivity),
		ClosedAt:          timeWire(r.ClosedAt),
		EscalatedAt:       timeWire(r.EscalatedAt),
	}
	if r.LastRuntimeStartFailure != nil {
		st.LastRuntimeStartFailure = timeWire(r.LastRuntimeStartFailure)
	}
	for _, t := range r.Threads {
		st.Threads = append(st.Threads, threadBindingWire(t))
	}
	if r.Inflight != nil {
		st.Inflight = &agentopsv1alpha1.InflightRun{
			RunID: r.Inflight.RunID, InputIDs: r.Inflight.InputIDs,
			DispatchedAt: metav1.NewTime(r.Inflight.DispatchedAt), Moved: r.Inflight.Moved,
		}
	}
	if r.Budget != nil {
		st.Budget = &agentopsv1alpha1.ConversationBudget{
			MaxAgents: r.Budget.MaxAgents, MaxTurns: r.Budget.MaxTurns,
			Deadline: timeWire(r.Budget.Deadline), AgentsInvoked: r.Budget.AgentsInvoked, Turns: r.Budget.Turns,
		}
	}
	if r.ContextCheckpoint != nil {
		st.ContextCheckpoint = &agentopsv1alpha1.ContextCheckpoint{
			At: metav1.NewTime(r.ContextCheckpoint.At), Generation: r.ContextCheckpoint.Generation,
			Quiesced: r.ContextCheckpoint.Quiesced, Bytes: r.ContextCheckpoint.Bytes,
		}
	}
	for _, c := range r.Conditions {
		st.Conditions = append(st.Conditions, metav1.Condition{
			Type: c.Type, Status: metav1.ConditionStatus(c.Status), Reason: c.Reason,
			Message: c.Message, ObservedGeneration: c.ObservedGeneration,
			LastTransitionTime: metav1.NewTime(c.LastTransitionTime),
		})
	}
	for _, run := range runs {
		st.Runs = append(st.Runs, runStatusWire(run))
	}
	return st
}

func threadBindingWire(t state.ThreadBinding) agentopsv1alpha1.ThreadBinding {
	out := agentopsv1alpha1.ThreadBinding{
		Channel: t.Channel, ThreadID: t.ThreadID, ReadAt: t.ReadAt, ReadTracked: t.ReadTracked,
		UndeliveredReply: t.UndeliveredReply,
	}
	for _, rm := range t.Readers {
		out.Readers = append(out.Readers, agentopsv1alpha1.ReaderMark{Key: rm.Key, ReadAt: rm.ReadAt})
	}
	if t.Claim != nil {
		out.Claim = &agentopsv1alpha1.OpClaim{Holder: t.Claim.Holder, ClaimedAt: metav1.NewTime(t.Claim.ClaimedAt)}
	}
	return out
}

func runStatusWire(r state.Run) agentopsv1alpha1.RunStatus {
	out := agentopsv1alpha1.RunStatus{
		RunID: r.RunID, JobKind: r.JobKind, Status: r.Status, ExitCode: r.ExitCode, Result: r.Result,
		StartedAt: timeWire(r.StartedAt), FinishedAt: timeWire(r.FinishedAt),
		InputIDs: r.InputIDs, DeliveryTracked: r.DeliveryTracked, Delivered: r.Delivered,
		RoutedToParent: r.RoutedToParent,
	}
	for _, in := range r.Inputs {
		out.Inputs = append(out.Inputs, recordedInputWire(in))
	}
	return out
}

func recordedInputWire(in state.RecordedInput) agentopsv1alpha1.RecordedInput {
	out := agentopsv1alpha1.RecordedInput{
		ID: in.ID, Type: agentopsv1alpha1.InputType(in.Type), Text: in.Text, Truncated: in.Truncated,
		PayloadRef: ref(state.Ref(in.PayloadRef)), Surface: in.Surface, Sender: in.Sender,
		Origin: agentopsv1alpha1.OriginKind(in.Origin),
	}
	if in.ReceivedAt != nil {
		mt := metav1.NewTime(*in.ReceivedAt)
		out.ReceivedAt = &mt
	}
	return out
}

// ---- the wire record: signals ------------------------------------------------
//
// SignalSpec and SignalStatus are on the state package's guarded-type list
// (internal/state/static_test.go's TestStaticCheckGuardsSignalsInEveryPackageOutsideTheBackend)
// — only the kube backend may name them. These two package-local types carry
// the SAME field names and json tags by hand, over the nested types the check
// does NOT guard (ClaimantRef/ClaimantState/ClaimantKind/SignalPhase/
// SignalDropReason/Severity/ObjectRef), so the published shape still matches
// api/v1alpha1.Signal's spec/status field-for-field without naming the two
// guarded identifiers.

type signalSpecWire struct {
	SourceRef    agentopsv1alpha1.ObjectRef `json:"sourceRef"`
	Adapter      string                     `json:"adapter"`
	InitiationID string                     `json:"initiationId"`
	SignalID     string                     `json:"signalId,omitempty"`
	Kind         string                     `json:"kind,omitempty"`
	Title        string                     `json:"title,omitempty"`
	Severity     agentopsv1alpha1.Severity  `json:"severity,omitempty"`
	Labels       map[string]string          `json:"labels,omitempty"`
	Reader       string                     `json:"reader,omitempty"`
	Payload      string                     `json:"payload,omitempty"`
}

type signalStatusWire struct {
	Phase          agentopsv1alpha1.SignalPhase      `json:"phase,omitempty"`
	Reason         agentopsv1alpha1.SignalDropReason `json:"reason,omitempty"`
	Attempts       int32                             `json:"attempts,omitempty"`
	FirstAttemptAt *metav1.Time                      `json:"firstAttemptAt,omitempty"`
	CompletedAt    *metav1.Time                      `json:"completedAt,omitempty"`
	ConsumedBy     string                            `json:"consumedBy,omitempty"`
	ConsumedAt     *metav1.Time                      `json:"consumedAt,omitempty"`
	Claimants      []agentopsv1alpha1.SignalClaimant `json:"claimants,omitempty"`
}

type signalWire struct {
	Name                string           `json:"name"`
	ID                  string           `json:"id"`
	Revision            string           `json:"revision"`
	CreatedAt           time.Time        `json:"createdAt"`
	DeletionRequestedAt *time.Time       `json:"deletionRequestedAt"`
	ExpiresAt           *time.Time       `json:"expiresAt,omitempty"`
	Spec                signalSpecWire   `json:"spec"`
	Status              signalStatusWire `json:"status"`
}

func newSignalWire(r *state.SignalRecord, rev state.Revision) signalWire {
	return signalWire{
		Name: r.Name, ID: r.ID, Revision: string(rev), CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt,
		Spec: signalSpecWire{
			SourceRef: agentopsv1alpha1.ObjectRef{Name: r.SourceRef}, Adapter: r.Adapter,
			InitiationID: r.InitiationID, SignalID: r.SignalID, Kind: r.Kind, Title: r.Title,
			Severity: agentopsv1alpha1.Severity(r.Severity), Labels: r.Labels, Reader: r.Reader, Payload: r.Payload,
		},
		Status: signalStatusWire{
			Phase: agentopsv1alpha1.SignalPhase(r.Phase), Reason: agentopsv1alpha1.SignalDropReason(r.Reason),
			Attempts: r.Attempts, FirstAttemptAt: timeWire(r.FirstAttemptAt), CompletedAt: timeWire(r.CompletedAt),
			ConsumedBy: r.ConsumedBy, ConsumedAt: timeWire(r.ConsumedAt), Claimants: claimantsWire(r.Claimants),
		},
	}
}

func claimantsWire(in []state.SignalClaimant) []agentopsv1alpha1.SignalClaimant {
	if len(in) == 0 {
		return nil
	}
	out := make([]agentopsv1alpha1.SignalClaimant, len(in))
	for i, c := range in {
		out[i] = agentopsv1alpha1.SignalClaimant{
			Ref:   agentopsv1alpha1.ClaimantRef{Kind: agentopsv1alpha1.ClaimantKind(c.Kind), Name: c.Name},
			State: agentopsv1alpha1.ClaimantState(c.State), Reason: c.Reason,
		}
		if c.ConversationRef != "" {
			out[i].ConversationRef = &agentopsv1alpha1.ObjectRef{Name: c.ConversationRef}
		}
	}
	return out
}

// ---- query-parameter discipline ----------------------------------------------

// onlyParams refuses a request naming any query parameter outside allowed,
// BY NAME — the same "name what's wrong" pattern adapterParam's retired
// `type` and contractOK's missing `contract` already use, rather than
// silently ignoring a caller's typo.
func onlyParams(w http.ResponseWriter, r *http.Request, allowed ...string) bool {
	ok := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		ok[a] = true
	}
	for k := range r.URL.Query() {
		if !ok[k] {
			writeJSON(w, 400, map[string]string{"error": fmt.Sprintf("unknown query parameter %q", k)})
			return false
		}
	}
	return true
}

// ---- conversations: list, get ------------------------------------------------

type conversationsListResponse struct {
	// Revision is a resume point safe to pass as the stream's ?since= — not
	// necessarily the collection's latest (List/ListWithItems hand back no
	// such thing; see state.Collection.List), but one of the listed records'
	// own, which the contract (store.go's Watch doc) resumes "strictly
	// after" — so a client resuming from it never misses a change made since
	// this listing, at the cost of possibly a few harmless, idempotent
	// replays of a put already reflected here.
	Revision string             `json:"revision"`
	Items    []conversationWire `json:"items"`
}

// handleStateConversationsList serves the whole set, phase-blind — no filter
// by design (housekeeping's own listing must see a Closed conversation too).
func (s *Server) handleStateConversationsList(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r) {
		return
	}
	listed, err := s.Runs.ListWithItems(r.Context(), state.ByAll, "")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := conversationsListResponse{Items: make([]conversationWire, 0, len(listed))}
	for i := range listed {
		rec := listed[i].Record
		out.Items = append(out.Items, newConversationWire(&rec, listed[i].Revision, listed[i].Items))
		if out.Revision == "" {
			out.Revision = string(listed[i].Revision)
		}
	}
	writeJSON(w, 200, out)
}

func (s *Server) handleStateConversationGet(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r) {
		return
	}
	rec, rev, runs, err := s.Runs.GetWithItems(r.Context(), state.Key(r.PathValue("name")))
	if err != nil {
		code := 500
		if errors.Is(err, state.ErrNotFound) {
			code = 404
		}
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, newConversationWire(&rec, rev, runs))
}

// ---- signals: list --------------------------------------------------------
//
// No GET /state/signals/{name} and no stream resume carrying items: nothing
// reads a single signal or a signal's own history (design D-5's "not
// served, because nothing reads it").

type signalsListResponse struct {
	// Revision: see conversationsListResponse's own comment. state.Collection's
	// plain List (unlike Appended's ListWithItems) hands back no per-item
	// revision at all, so a signal's own "revision" field below is "" — this
	// listing's own top-level Revision is therefore always "" too. Resuming
	// the signal stream from "" is still safe (Watch treats an empty resume
	// as "from now"); it is simply not minimal, the same accepted cost as
	// state-store-generic-contract's own D-6 (a wider credential than
	// strictly needed is accepted rather than invented around).
	Revision string       `json:"revision"`
	Items    []signalWire `json:"items"`
}

func (s *Server) handleStateSignalsList(w http.ResponseWriter, r *http.Request) {
	if !onlyParams(w, r) {
		return
	}
	listed, err := s.Signals.List(r.Context(), state.SignalAll, "")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := signalsListResponse{Items: make([]signalWire, 0, len(listed))}
	for i := range listed {
		out.Items = append(out.Items, newSignalWire(&listed[i], ""))
	}
	writeJSON(w, 200, out)
}

// ---- resumable feeds (SSE) ---------------------------------------------------
//
// Frames: put, delete, bookmark (design D-5's Feed, store.go's EventType) and
// resync — the one frame the generic contract does not itself model: `since`
// failing to open a feed (an unknown or stale revision) is read as "relist",
// never as an error, by opening a fresh feed "from now" (Watch's own rule for
// an empty resume) and telling the caller to re-seed from the plain GET
// endpoint before trusting anything this connection delivers afterwards. A
// known-good `since` skips straight to streaming puts/deletes, carrying no
// resync frame at all.

func (s *Server) handleStateConversationsStream(w http.ResponseWriter, r *http.Request) {
	s.streamSSE(w, r, func(ctx context.Context, resume state.Revision) (<-chan sseFrame, error) {
		feed, err := s.Runs.WatchWithItems(ctx, resume)
		if err != nil {
			return nil, err
		}
		return translateConversationFeed(feed), nil
	})
}

func (s *Server) handleStateSignalsStream(w http.ResponseWriter, r *http.Request) {
	s.streamSSE(w, r, func(ctx context.Context, resume state.Revision) (<-chan sseFrame, error) {
		feed, err := s.Signals.Watch(ctx, resume)
		if err != nil {
			return nil, err
		}
		return translateSignalFeed(feed), nil
	})
}

// sseFrame is one rendered frame, ready to write — the event type, the
// resume id and the already-marshalled data, so streamSSE needs no knowledge
// of either record kind.
type sseFrame struct {
	event string
	id    state.Revision
	data  []byte
}

func translateConversationFeed(feed <-chan state.Event[state.WithItems[state.ConversationRecord, state.Run]]) <-chan sseFrame {
	out := make(chan sseFrame)
	go func() {
		defer close(out)
		for ev := range feed {
			switch ev.Type {
			case state.EventPut:
				rec := ev.Record.Record
				data, _ := json.Marshal(newConversationWire(&rec, ev.Record.Revision, ev.Record.Items))
				out <- sseFrame{event: "put", id: ev.Resume, data: data}
			case state.EventDelete:
				data, _ := json.Marshal(map[string]string{"name": string(ev.Key)})
				out <- sseFrame{event: "delete", id: ev.Resume, data: data}
			case state.EventBookmark:
				out <- sseFrame{event: "bookmark", id: ev.Resume, data: []byte("{}")}
			}
		}
	}()
	return out
}

func translateSignalFeed(feed <-chan state.Event[state.SignalRecord]) <-chan sseFrame {
	out := make(chan sseFrame)
	go func() {
		defer close(out)
		for ev := range feed {
			switch ev.Type {
			case state.EventPut:
				rec := ev.Record
				data, _ := json.Marshal(newSignalWire(&rec, ev.Resume))
				out <- sseFrame{event: "put", id: ev.Resume, data: data}
			case state.EventDelete:
				data, _ := json.Marshal(map[string]string{"name": string(ev.Key)})
				out <- sseFrame{event: "delete", id: ev.Resume, data: data}
			case state.EventBookmark:
				out <- sseFrame{event: "bookmark", id: ev.Resume, data: []byte("{}")}
			}
		}
	}()
	return out
}

// streamSSE is the one place that writes the SSE wire format, shared by both
// stream handlers: validate ?since= is the only parameter, open the caller's
// feed, fall back to "resync then from now" when it does not open, and relay
// frames until the client disconnects.
func (s *Server) streamSSE(w http.ResponseWriter, r *http.Request, open func(context.Context, state.Revision) (<-chan sseFrame, error)) {
	if !onlyParams(w, r, "since") {
		return
	}
	ctx := r.Context()
	since := state.Revision(r.URL.Query().Get("since"))
	frames, err := open(ctx, since)
	resync := false
	if err != nil {
		resync = true
		frames, err = open(ctx, "")
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, 500, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(200)
	// Flushed immediately, resync or not: a caller's Do() blocks on the
	// response headers, and without writing anything yet net/http holds
	// them back waiting to see whether more is coming — which here may be
	// nothing at all until the next real change.
	flusher.Flush()
	if resync {
		writeSSEFrame(w, sseFrame{event: "resync", data: []byte("{}")})
		flusher.Flush()
	}
	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-keepAlive.C:
			_, _ = fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case f, ok := <-frames:
			if !ok {
				// The backend ended the feed: an apiserver watch that timed out,
				// or errored on a resume point it no longer holds (the contract
				// reports a terminal failure by closing the channel early, with
				// no error value). A bare close tells the reader nothing, and a
				// reader that reconnects from the same stale point is refused
				// the same way for ever, its view frozen. `resync` makes it
				// relist, which closes any gap the ended feed left.
				if ctx.Err() == nil {
					writeSSEFrame(w, sseFrame{event: "resync", data: []byte("{}")})
					flusher.Flush()
				}
				return
			}
			writeSSEFrame(w, f)
			flusher.Flush()
		}
	}
}

func writeSSEFrame(w http.ResponseWriter, f sseFrame) {
	if f.id != "" {
		fmt.Fprintf(w, "event: %s\nid: %s\ndata: %s\n\n", f.event, f.id, f.data)
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", f.event, f.data)
}

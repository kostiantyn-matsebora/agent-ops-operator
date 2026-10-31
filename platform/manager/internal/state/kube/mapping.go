package kube

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// This file is the one place state.ConversationRecord (and state.Run,
// state.Blob) is mapped to and from the v1alpha1 CRD types it mirrors
// (design D-1: "the kube backend maps records to the existing CRDs"). Every
// function pair is named toRecord/applySpecAndMeta+applyStatus for the
// control record, and toX/toAPIX for every nested shape, so a field added to
// record.go without its pair here is easy to spot by the asymmetry.

// toRecord maps a Conversation to the record it carries (api -> state),
// used by Get/List/Watch.
func toRecord(conv *agentopsv1alpha1.Conversation) state.ConversationRecord {
	s := conv.Spec
	st := conv.Status

	rec := state.ConversationRecord{
		Name: conv.Name,

		CoordinatorRef:        objectRefToRef(s.CoordinatorRef),
		EscalationChannelRefs: objectRefsToRefs(s.EscalationChannelRefs),
		ChannelRefs:           objectRefsToRefs(s.ChannelRefs),
		ProfileRef:            s.ProfileRef.Name,
		PipelineRef:           objectRefToRef(s.PipelineRef),
		Title:                 s.Title,
		ConversationID:        s.ConversationID,
		RuntimeRef:            objectRefToRef(s.RuntimeRef),
		ServiceAccountName:    s.ServiceAccountName,
		ContextClaimName:      s.ContextClaimName,
		WorkspaceClaimName:    s.WorkspaceClaimName,
		Inputs:                toStateInputItems(s.Inputs),

		Phase:                   string(st.Phase),
		Threads:                 toStateThreadBindings(st.Threads),
		RuntimeContextID:        st.RuntimeContextID,
		SessionID:               st.SessionID,
		RuntimePod:              st.RuntimePod,
		ProcessedInputIDs:       st.ProcessedInputIDs,
		LastActivity:            metaTimeToTimePtr(st.LastActivity),
		ClosedAt:                metaTimeToTimePtr(st.ClosedAt),
		EscalatedAt:             metaTimeToTimePtr(st.EscalatedAt),
		EscalationMessage:       st.EscalationMessage,
		CloseReason:             st.CloseReason,
		Brief:                   st.Brief,
		ThreadsArchived:         st.ThreadsArchived,
		Reopens:                 st.Reopens,
		RuntimeStartFailures:    st.RuntimeStartFailures,
		LastRuntimeStartFailure: metaTimeToTimePtr(st.LastRuntimeStartFailure),
		Conditions:              toStateConditions(st.Conditions),

		ExpiresAt: expiresAtFromAnnotations(conv.Annotations),
	}

	if s.CausedBy != nil {
		rec.CausedBy = &state.Provenance{Parent: s.CausedBy.Parent, Entry: s.CausedBy.Entry}
	}
	if s.Signal != nil {
		rec.Signal = &state.SignalProvenance{
			SourceRef: objectRefToRef(s.Signal.SourceRef),
			Labels:    s.Signal.Labels,
			Severity:  string(s.Signal.Severity),
		}
	}
	if s.OriginReader != nil {
		rec.OriginReader = &state.OriginReader{Channel: s.OriginReader.Channel, Key: s.OriginReader.Key}
	}
	if s.Toolsets != nil {
		rec.Toolsets = &state.ToolsetBinding{Mode: s.Toolsets.Mode, Refs: objectRefsToRefs(s.Toolsets.Refs)}
	}
	if s.MCPConfigs != nil {
		rec.MCPConfigs = &state.ToolingBinding{Refs: objectRefsToRefs(s.MCPConfigs.Refs)}
	}
	if st.Inflight != nil {
		rec.Inflight = &state.InflightRun{
			RunID:        st.Inflight.RunID,
			InputIDs:     st.Inflight.InputIDs,
			DispatchedAt: st.Inflight.DispatchedAt.Time,
			Moved:        st.Inflight.Moved,
		}
	}
	if st.Budget != nil {
		rec.Budget = &state.ConversationBudget{
			MaxAgents:     st.Budget.MaxAgents,
			MaxTurns:      st.Budget.MaxTurns,
			Deadline:      metaTimeToTimePtr(st.Budget.Deadline),
			AgentsInvoked: st.Budget.AgentsInvoked,
			Turns:         st.Budget.Turns,
		}
	}
	if st.ContextCheckpoint != nil {
		rec.ContextCheckpoint = &state.ContextCheckpoint{
			At:         st.ContextCheckpoint.At.Time,
			Generation: st.ContextCheckpoint.Generation,
			Quiesced:   st.ContextCheckpoint.Quiesced,
			Bytes:      st.ContextCheckpoint.Bytes,
		}
	}
	return rec
}

// applySpecAndMeta writes rec's spec-shaped fields, plus the ExpiresAt
// annotation, onto conv (state -> api). Paired with applyStatus below —
// see store.go's Update for why the two are separate apiserver calls.
func applySpecAndMeta(conv *agentopsv1alpha1.Conversation, rec state.ConversationRecord) {
	s := &conv.Spec

	s.CausedBy = nil
	if rec.CausedBy != nil {
		s.CausedBy = &agentopsv1alpha1.Provenance{Parent: rec.CausedBy.Parent, Entry: rec.CausedBy.Entry}
	}
	s.CoordinatorRef = refToObjectRef(rec.CoordinatorRef)
	s.EscalationChannelRefs = refsToObjectRefs(rec.EscalationChannelRefs)
	s.ChannelRefs = refsToObjectRefs(rec.ChannelRefs)
	s.ProfileRef = agentopsv1alpha1.ObjectRef{Name: rec.ProfileRef}
	s.PipelineRef = refToObjectRef(rec.PipelineRef)

	s.Signal = nil
	if rec.Signal != nil {
		s.Signal = &agentopsv1alpha1.SignalProvenance{
			SourceRef: refToObjectRef(rec.Signal.SourceRef),
			Labels:    rec.Signal.Labels,
			Severity:  agentopsv1alpha1.Severity(rec.Signal.Severity),
		}
	}
	s.Title = rec.Title
	s.ConversationID = rec.ConversationID

	s.OriginReader = nil
	if rec.OriginReader != nil {
		s.OriginReader = &agentopsv1alpha1.OriginReader{Channel: rec.OriginReader.Channel, Key: rec.OriginReader.Key}
	}

	s.Toolsets = nil
	if rec.Toolsets != nil {
		s.Toolsets = &agentopsv1alpha1.ToolsetBinding{Mode: rec.Toolsets.Mode, Refs: refsToObjectRefs(rec.Toolsets.Refs)}
	}
	s.MCPConfigs = nil
	if rec.MCPConfigs != nil {
		s.MCPConfigs = &agentopsv1alpha1.ToolingBinding{Refs: refsToObjectRefs(rec.MCPConfigs.Refs)}
	}

	s.RuntimeRef = refToObjectRef(rec.RuntimeRef)
	s.ServiceAccountName = rec.ServiceAccountName
	s.ContextClaimName = rec.ContextClaimName
	s.WorkspaceClaimName = rec.WorkspaceClaimName
	s.Inputs = toAPIInputItems(rec.Inputs)

	setExpiresAtAnnotation(conv, rec.ExpiresAt)
}

// applyStatus writes rec's status-shaped fields onto conv (state -> api).
// Deliberately never touches conv.Status.Runs: runs are the append-only
// sub-record AppendRun/ListRuns own (design D-5), never part of a
// Create/Update record.
func applyStatus(conv *agentopsv1alpha1.Conversation, rec state.ConversationRecord) {
	st := &conv.Status

	st.Phase = agentopsv1alpha1.ConversationPhase(rec.Phase)
	st.Threads = toAPIThreadBindings(rec.Threads)
	st.RuntimeContextID = rec.RuntimeContextID
	st.SessionID = rec.SessionID
	st.RuntimePod = rec.RuntimePod

	st.Inflight = nil
	if rec.Inflight != nil {
		st.Inflight = &agentopsv1alpha1.InflightRun{
			RunID:        rec.Inflight.RunID,
			InputIDs:     rec.Inflight.InputIDs,
			DispatchedAt: metav1.NewTime(rec.Inflight.DispatchedAt),
			Moved:        rec.Inflight.Moved,
		}
	}

	st.ProcessedInputIDs = rec.ProcessedInputIDs
	st.LastActivity = timeToMetaTimePtr(rec.LastActivity)
	st.ClosedAt = timeToMetaTimePtr(rec.ClosedAt)

	st.Budget = nil
	if rec.Budget != nil {
		st.Budget = &agentopsv1alpha1.ConversationBudget{
			MaxAgents:     rec.Budget.MaxAgents,
			MaxTurns:      rec.Budget.MaxTurns,
			Deadline:      timeToMetaTimePtr(rec.Budget.Deadline),
			AgentsInvoked: rec.Budget.AgentsInvoked,
			Turns:         rec.Budget.Turns,
		}
	}

	st.EscalatedAt = timeToMetaTimePtr(rec.EscalatedAt)
	st.EscalationMessage = rec.EscalationMessage
	st.CloseReason = rec.CloseReason
	st.Brief = rec.Brief
	st.ThreadsArchived = rec.ThreadsArchived
	st.Reopens = rec.Reopens

	st.ContextCheckpoint = nil
	if rec.ContextCheckpoint != nil {
		st.ContextCheckpoint = &agentopsv1alpha1.ContextCheckpoint{
			At:         metav1.NewTime(rec.ContextCheckpoint.At),
			Generation: rec.ContextCheckpoint.Generation,
			Quiesced:   rec.ContextCheckpoint.Quiesced,
			Bytes:      rec.ContextCheckpoint.Bytes,
		}
	}

	st.RuntimeStartFailures = rec.RuntimeStartFailures
	st.LastRuntimeStartFailure = timeToMetaTimePtr(rec.LastRuntimeStartFailure)
	st.Conditions = toAPIConditions(rec.Conditions)
}

// toRun maps one recorded run (api -> state), for ListRuns.
func toRun(rs agentopsv1alpha1.RunStatus) state.Run {
	return state.Run{
		RunID:           rs.RunID,
		JobKind:         rs.JobKind,
		Status:          rs.Status,
		ExitCode:        rs.ExitCode,
		Result:          rs.Result,
		StartedAt:       metaTimeToTimePtr(rs.StartedAt),
		FinishedAt:      metaTimeToTimePtr(rs.FinishedAt),
		InputIDs:        rs.InputIDs,
		DeliveryTracked: rs.DeliveryTracked,
		Delivered:       rs.Delivered,
		RoutedToParent:  rs.RoutedToParent,
		Inputs:          toStateRecordedInputs(rs.Inputs),
	}
}

// toRunStatus maps one run to append (state -> api), for AppendRun.
func toRunStatus(r state.Run) agentopsv1alpha1.RunStatus {
	return agentopsv1alpha1.RunStatus{
		RunID:           r.RunID,
		JobKind:         r.JobKind,
		Status:          r.Status,
		ExitCode:        r.ExitCode,
		Result:          r.Result,
		StartedAt:       timeToMetaTimePtr(r.StartedAt),
		FinishedAt:      timeToMetaTimePtr(r.FinishedAt),
		InputIDs:        r.InputIDs,
		DeliveryTracked: r.DeliveryTracked,
		Delivered:       r.Delivered,
		RoutedToParent:  r.RoutedToParent,
		Inputs:          toAPIRecordedInputs(r.Inputs),
	}
}

func toStateInputItems(items []agentopsv1alpha1.InputItem) []state.InputItem {
	if items == nil {
		return nil
	}
	out := make([]state.InputItem, len(items))
	for i, it := range items {
		out[i] = state.InputItem{
			ID:         it.ID,
			Type:       string(it.Type),
			Payload:    it.Payload,
			PayloadRef: objectRefToKey(it.PayloadRef),
			Origin:     toStateInputOrigin(it.Origin),
			ReceivedAt: it.ReceivedAt.Time,
		}
	}
	return out
}

// toAPIInputItems maps the record's queue back to v1alpha1.InputItem.
// Agent (api/v1alpha1.InputItem.Agent, deprecated) has no counterpart in
// state.InputItem and is never set here: nothing in this contract writes
// it (record.go's own comment on the field), and nothing yet calls into
// this backend to round-trip an existing value (section 4 of the
// state-store change is out of scope for this package).
func toAPIInputItems(items []state.InputItem) []agentopsv1alpha1.InputItem {
	if items == nil {
		return nil
	}
	out := make([]agentopsv1alpha1.InputItem, len(items))
	for i, it := range items {
		out[i] = agentopsv1alpha1.InputItem{
			ID:         it.ID,
			Type:       agentopsv1alpha1.InputType(it.Type),
			Payload:    it.Payload,
			PayloadRef: keyToObjectRef(it.PayloadRef),
			Origin:     toAPIInputOrigin(it.Origin),
			ReceivedAt: metav1.NewTime(it.ReceivedAt),
		}
	}
	return out
}

func toStateInputOrigin(o *agentopsv1alpha1.InputOrigin) *state.InputOrigin {
	if o == nil {
		return nil
	}
	return &state.InputOrigin{Kind: string(o.Kind), Name: o.Name, Entry: o.Entry, SignalKind: o.SignalKind, Sender: o.Sender}
}

func toAPIInputOrigin(o *state.InputOrigin) *agentopsv1alpha1.InputOrigin {
	if o == nil {
		return nil
	}
	return &agentopsv1alpha1.InputOrigin{Kind: agentopsv1alpha1.OriginKind(o.Kind), Name: o.Name, Entry: o.Entry, SignalKind: o.SignalKind, Sender: o.Sender}
}

func toStateRecordedInputs(ris []agentopsv1alpha1.RecordedInput) []state.RecordedInput {
	if ris == nil {
		return nil
	}
	out := make([]state.RecordedInput, len(ris))
	for i, ri := range ris {
		out[i] = toStateRecordedInput(ri)
	}
	return out
}

func toAPIRecordedInputs(ris []state.RecordedInput) []agentopsv1alpha1.RecordedInput {
	if ris == nil {
		return nil
	}
	out := make([]agentopsv1alpha1.RecordedInput, len(ris))
	for i, ri := range ris {
		out[i] = toAPIRecordedInput(ri)
	}
	return out
}

func toStateRecordedInput(r agentopsv1alpha1.RecordedInput) state.RecordedInput {
	return state.RecordedInput{
		ID:         r.ID,
		Type:       string(r.Type),
		Text:       r.Text,
		Truncated:  r.Truncated,
		PayloadRef: objectRefToKey(r.PayloadRef),
		Surface:    r.Surface,
		Sender:     r.Sender,
		Origin:     string(r.Origin),
		ReceivedAt: metaTimeToTimePtr(r.ReceivedAt),
	}
}

func toAPIRecordedInput(r state.RecordedInput) agentopsv1alpha1.RecordedInput {
	return agentopsv1alpha1.RecordedInput{
		ID:         r.ID,
		Type:       agentopsv1alpha1.InputType(r.Type),
		Text:       r.Text,
		Truncated:  r.Truncated,
		PayloadRef: keyToObjectRef(r.PayloadRef),
		Surface:    r.Surface,
		Sender:     r.Sender,
		Origin:     agentopsv1alpha1.OriginKind(r.Origin),
		ReceivedAt: timeToMetaTimePtr(r.ReceivedAt),
	}
}

func toStateThreadBindings(tbs []agentopsv1alpha1.ThreadBinding) []state.ThreadBinding {
	if tbs == nil {
		return nil
	}
	out := make([]state.ThreadBinding, len(tbs))
	for i, tb := range tbs {
		out[i] = state.ThreadBinding{
			Channel:          tb.Channel,
			ThreadID:         tb.ThreadID,
			ReadAt:           tb.ReadAt,
			ReadTracked:      tb.ReadTracked,
			Readers:          toStateReaderMarks(tb.Readers),
			UndeliveredReply: tb.UndeliveredReply,
			Claim:            toStateOpClaim(tb.Claim),
		}
	}
	return out
}

func toAPIThreadBindings(tbs []state.ThreadBinding) []agentopsv1alpha1.ThreadBinding {
	if tbs == nil {
		return nil
	}
	out := make([]agentopsv1alpha1.ThreadBinding, len(tbs))
	for i, tb := range tbs {
		out[i] = agentopsv1alpha1.ThreadBinding{
			Channel:          tb.Channel,
			ThreadID:         tb.ThreadID,
			ReadAt:           tb.ReadAt,
			ReadTracked:      tb.ReadTracked,
			Readers:          toAPIReaderMarks(tb.Readers),
			UndeliveredReply: tb.UndeliveredReply,
			Claim:            toAPIOpClaim(tb.Claim),
		}
	}
	return out
}

func toStateReaderMarks(rs []agentopsv1alpha1.ReaderMark) []state.ReaderMark {
	if rs == nil {
		return nil
	}
	out := make([]state.ReaderMark, len(rs))
	for i, r := range rs {
		out[i] = state.ReaderMark{Key: r.Key, ReadAt: r.ReadAt}
	}
	return out
}

func toAPIReaderMarks(rs []state.ReaderMark) []agentopsv1alpha1.ReaderMark {
	if rs == nil {
		return nil
	}
	out := make([]agentopsv1alpha1.ReaderMark, len(rs))
	for i, r := range rs {
		out[i] = agentopsv1alpha1.ReaderMark{Key: r.Key, ReadAt: r.ReadAt}
	}
	return out
}

func toStateOpClaim(c *agentopsv1alpha1.OpClaim) *state.OpClaim {
	if c == nil {
		return nil
	}
	return &state.OpClaim{Holder: c.Holder, ClaimedAt: c.ClaimedAt.Time}
}

func toAPIOpClaim(c *state.OpClaim) *agentopsv1alpha1.OpClaim {
	if c == nil {
		return nil
	}
	return &agentopsv1alpha1.OpClaim{Holder: c.Holder, ClaimedAt: metav1.NewTime(c.ClaimedAt)}
}

func toStateConditions(cs []metav1.Condition) []state.Condition {
	if cs == nil {
		return nil
	}
	out := make([]state.Condition, len(cs))
	for i, c := range cs {
		out[i] = state.Condition{
			Type:               c.Type,
			Status:             string(c.Status),
			Reason:             c.Reason,
			Message:            c.Message,
			ObservedGeneration: c.ObservedGeneration,
			LastTransitionTime: c.LastTransitionTime.Time,
		}
	}
	return out
}

func toAPIConditions(cs []state.Condition) []metav1.Condition {
	if cs == nil {
		return nil
	}
	out := make([]metav1.Condition, len(cs))
	for i, c := range cs {
		out[i] = metav1.Condition{
			Type:               c.Type,
			Status:             metav1.ConditionStatus(c.Status),
			Reason:             c.Reason,
			Message:            c.Message,
			ObservedGeneration: c.ObservedGeneration,
			LastTransitionTime: metav1.NewTime(c.LastTransitionTime),
		}
	}
	return out
}

// refToObjectRef/objectRefToRef convert state.Ref (a bare string, empty
// meaning "no reference" — record.go's own doc on the Ref alias) to/from
// the pointer-optional *v1alpha1.ObjectRef most spec fields use.
func refToObjectRef(r state.Ref) *agentopsv1alpha1.ObjectRef {
	if r == "" {
		return nil
	}
	return &agentopsv1alpha1.ObjectRef{Name: r}
}

func objectRefToRef(r *agentopsv1alpha1.ObjectRef) state.Ref {
	if r == nil {
		return ""
	}
	return r.Name
}

func refsToObjectRefs(refs []state.Ref) []agentopsv1alpha1.ObjectRef {
	if refs == nil {
		return nil
	}
	out := make([]agentopsv1alpha1.ObjectRef, len(refs))
	for i, r := range refs {
		out[i] = agentopsv1alpha1.ObjectRef{Name: r}
	}
	return out
}

func objectRefsToRefs(refs []agentopsv1alpha1.ObjectRef) []state.Ref {
	if refs == nil {
		return nil
	}
	out := make([]state.Ref, len(refs))
	for i, r := range refs {
		out[i] = r.Name
	}
	return out
}

// keyToObjectRef/objectRefToKey convert a state.Key (used for
// InputItem/RecordedInput's PayloadRef, naming a Blob) to/from the
// *v1alpha1.ObjectRef those fields carry on the CRD side.
func keyToObjectRef(k state.Key) *agentopsv1alpha1.ObjectRef {
	if k == "" {
		return nil
	}
	return &agentopsv1alpha1.ObjectRef{Name: string(k)}
}

func objectRefToKey(r *agentopsv1alpha1.ObjectRef) state.Key {
	if r == nil {
		return ""
	}
	return state.Key(r.Name)
}

// metaTimeToTimePtr/timeToMetaTimePtr convert between the contract's plain
// *time.Time and the CRD's *metav1.Time, preserving nil exactly (a nil
// pointer on one side maps to a nil pointer on the other, never a
// zero-value instant) so a round trip never invents a timestamp nobody
// wrote.
func metaTimeToTimePtr(t *metav1.Time) *time.Time {
	if t == nil {
		return nil
	}
	tt := t.Time
	return &tt
}

func timeToMetaTimePtr(t *time.Time) *metav1.Time {
	if t == nil {
		return nil
	}
	mt := metav1.NewTime(*t)
	return &mt
}

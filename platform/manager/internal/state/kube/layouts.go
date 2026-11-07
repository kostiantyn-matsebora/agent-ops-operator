package kube

import (
	"k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// This file holds the layouts of the closed inventory: the one place the
// backend names a kind. A layout is a handful of functions over the typed
// object, so each narrows the client.Object it is handed once and maps.

// maxRunHistory bounds status.runs[] on the stored object, so a long-lived
// conversation cannot grow its CR without limit. It is the bound the work
// report always applied itself; owning it here keeps every writer inside it.
const maxRunHistory = 10

func asConversation(obj client.Object) *agentopsv1alpha1.Conversation {
	return obj.(*agentopsv1alpha1.Conversation)
}

func asSignal(obj client.Object) *agentopsv1alpha1.Signal {
	return obj.(*agentopsv1alpha1.Signal)
}

func asSource(obj client.Object) *agentopsv1alpha1.SignalSource {
	return obj.(*agentopsv1alpha1.SignalSource)
}

// ConversationLayout maps the conversation control record onto a Conversation:
// spec and metadata through the main endpoint, status through the subresource,
// the runs left to RunsLayout.
func ConversationLayout() Layout[state.ConversationRecord] {
	return Layout[state.ConversationRecord]{
		Decl:    state.StoreDeclarations.Conversations,
		New:     func() client.Object { return &agentopsv1alpha1.Conversation{} },
		NewList: func() client.ObjectList { return &agentopsv1alpha1.ConversationList{} },
		Read: func(obj client.Object) (state.ConversationRecord, bool) {
			return toRecord(asConversation(obj)), true
		},
		Init:        func(obj client.Object, rec state.ConversationRecord) { applySpecAndMeta(asConversation(obj), rec) },
		WriteMain:   func(obj client.Object, rec state.ConversationRecord) { applySpecAndMeta(asConversation(obj), rec) },
		WriteStatus: func(obj client.Object, rec state.ConversationRecord) { applyStatus(asConversation(obj), rec) },
		StatusDrift: conversationStatusDrift,
		Finalizers:  func(rec state.ConversationRecord) []string { return rec.Finalizers },
		Prefilter: map[string]func(client.Object, string) bool{
			state.ByConversationID.Name(): func(obj client.Object, v string) bool {
				return asConversation(obj).Spec.ConversationID == v
			},
			state.ByPipelineAndPhase.Name(): func(obj client.Object, v string) bool {
				c := asConversation(obj)
				return state.IndexValue(objectRefToRef(c.Spec.PipelineRef), string(c.Status.Phase)) == v
			},
			state.ByPhase.Name(): func(obj client.Object, v string) bool {
				return string(asConversation(obj).Status.Phase) == v
			},
			state.ByCausedBy.Name(): func(obj client.Object, v string) bool {
				c := asConversation(obj)
				return c.Spec.CausedBy != nil && c.Spec.CausedBy.Parent == v
			},
			state.ByNameBase.Name(): func(obj client.Object, v string) bool {
				return asConversation(obj).Labels[agentopsv1alpha1.LabelNameBase] == v
			},
		},
	}
}

// conversationStatusDrift reports whether current differs from base in any
// status field other than Runs. An update never writes Runs, and an append or a
// grow only changes them, so a difference there is not a competing write to the
// update's own fields.
func conversationStatusDrift(base, current client.Object) bool {
	b := asConversation(base).Status.DeepCopy()
	cur := asConversation(current).Status
	b.Runs = cur.Runs
	return !equality.Semantic.DeepEqual(b, &cur)
}

// RunsLayout maps a conversation's runs onto Conversation.status.runs[].
func RunsLayout() AppendedLayout[state.ConversationRecord, state.Run] {
	return AppendedLayout[state.ConversationRecord, state.Run]{
		Parent:     ConversationLayout(),
		Items:      state.StoreDeclarations.Runs,
		ReadItems:  func(obj client.Object) []state.Run { return toRuns(asConversation(obj).Status.Runs) },
		WriteItems: func(obj client.Object, runs []state.Run) { asConversation(obj).Status.Runs = toRunStatuses(runs) },
		MaxItems:   maxRunHistory,
	}
}

// toRuns maps a status.runs list; an empty one is nil.
func toRuns(in []agentopsv1alpha1.RunStatus) []state.Run {
	if len(in) == 0 {
		return nil
	}
	out := make([]state.Run, len(in))
	for i, rs := range in {
		out[i] = toRun(rs)
	}
	return out
}

func toRunStatuses(in []state.Run) []agentopsv1alpha1.RunStatus {
	if len(in) == 0 {
		return nil
	}
	out := make([]agentopsv1alpha1.RunStatus, len(in))
	for i, r := range in {
		out[i] = toRunStatus(r)
	}
	return out
}

// SignalLayout maps a signal record onto a Signal: publication fields and the
// owner link at create, status through the subresource, the expiry annotation
// settled after it.
func SignalLayout() Layout[state.SignalRecord] {
	return Layout[state.SignalRecord]{
		Decl:    state.StoreDeclarations.Signals,
		New:     func() client.Object { return &agentopsv1alpha1.Signal{} },
		NewList: func() client.ObjectList { return &agentopsv1alpha1.SignalList{} },
		Read: func(obj client.Object) (state.SignalRecord, bool) {
			return toSignalRecord(asSignal(obj)), true
		},
		Init: func(obj client.Object, rec state.SignalRecord) { applySignalSpec(asSignal(obj), rec) },
		// A stored Signal's spec is never rewritten: the main endpoint carries
		// the expiry annotation alone.
		WriteMain: func(obj client.Object, rec state.SignalRecord) {
			setExpiresAt(&asSignal(obj).ObjectMeta, rec.ExpiresAt)
		},
		WriteStatus: func(obj client.Object, rec state.SignalRecord) { applySignalStatus(asSignal(obj), rec) },
		StatusFirst: true,
		Prefilter: map[string]func(client.Object, string) bool{
			state.SignalNonTerminalBySource.Name(): func(obj client.Object, v string) bool {
				s := asSignal(obj)
				return s.Spec.SourceRef.Name == v && !state.TerminalSignalPhase(string(s.Status.Phase))
			},
			state.SignalNonTerminalByInitiation.Name(): func(obj client.Object, v string) bool {
				s := asSignal(obj)
				return state.IndexValue(s.Spec.SourceRef.Name, s.Spec.InitiationID) == v && !state.TerminalSignalPhase(string(s.Status.Phase))
			},
			state.SignalBySource.Name(): func(obj client.Object, v string) bool {
				return asSignal(obj).Spec.SourceRef.Name == v
			},
		},
	}
}

// sourceKey locates a record keyed by a source's name: the source itself.
func sourceKey(key state.Key) (string, string, error) { return string(key), "", nil }

// CooldownLayout maps a source's cooldown window onto SignalSource.status.cooldown,
// written by the same status merge patch the router always made.
func CooldownLayout() Layout[state.CooldownWindow] {
	read := func(obj client.Object) (state.CooldownWindow, bool) {
		cd := asSource(obj).Status.Cooldown
		return state.CooldownWindow{Entries: toCooldownEntries(cd)}, len(cd) > 0
	}
	return Layout[state.CooldownWindow]{
		Decl:    state.StoreDeclarations.Cooldowns,
		New:     func() client.Object { return &agentopsv1alpha1.SignalSource{} },
		NewList: func() client.ObjectList { return &agentopsv1alpha1.SignalSourceList{} },
		Part: &Part[state.CooldownWindow]{
			Locate: sourceKey,
			Read:   func(obj client.Object, _ string) (state.CooldownWindow, bool) { return read(obj) },
			Write: func(obj client.Object, _ string, w state.CooldownWindow) {
				asSource(obj).Status.Cooldown = toAPICooldownEntries(w.Entries)
			},
			Clear: func(obj client.Object, _ string) { asSource(obj).Status.Cooldown = nil },
			Enumerate: func(obj client.Object) []Entry[state.CooldownWindow] {
				if w, ok := read(obj); ok {
					return []Entry[state.CooldownWindow]{{Key: state.Key(obj.GetName()), Record: w}}
				}
				return nil
			},
			Status: true,
		},
	}
}

// ReceiptLayout maps a source's receipt counters onto
// SignalSource.status.receivedTotal and lastReceived.
func ReceiptLayout() Layout[state.Received] {
	read := func(obj client.Object) (state.Received, bool) {
		st := asSource(obj).Status
		out := state.Received{Total: st.ReceivedTotal, Last: metaTimeToTimePtr(st.LastReceived)}
		return out, st.ReceivedTotal != 0 || st.LastReceived != nil
	}
	return Layout[state.Received]{
		Decl:    state.StoreDeclarations.Receipts,
		New:     func() client.Object { return &agentopsv1alpha1.SignalSource{} },
		NewList: func() client.ObjectList { return &agentopsv1alpha1.SignalSourceList{} },
		Part: &Part[state.Received]{
			Locate: sourceKey,
			Read:   func(obj client.Object, _ string) (state.Received, bool) { return read(obj) },
			Write: func(obj client.Object, _ string, r state.Received) {
				st := &asSource(obj).Status
				st.ReceivedTotal = r.Total
				st.LastReceived = timeToMetaTimePtr(r.Last)
			},
			Clear: func(obj client.Object, _ string) {
				st := &asSource(obj).Status
				st.ReceivedTotal, st.LastReceived = 0, nil
			},
			Enumerate: func(obj client.Object) []Entry[state.Received] {
				if r, ok := read(obj); ok {
					return []Entry[state.Received]{{Key: state.Key(obj.GetName()), Record: r}}
				}
				return nil
			},
			Status: true,
		},
	}
}

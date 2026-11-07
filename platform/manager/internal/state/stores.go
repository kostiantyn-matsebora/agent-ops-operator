package state

import "time"

// Stores is the closed inventory of what the store holds (design D-1): one
// typed field per kind of state, and nothing else. Each field's type names
// exactly the capabilities that kind has, so a capability a kind lacks is not
// callable on it — Conversations cannot expire, Signals cannot append — and an
// index of one kind does not type-check against another's List.
//
// Every backend implements every field, and the conformance suite walks this
// struct, so a kind of state cannot be left out of a backend's test. Adding a
// kind is one field here and its declaration in Declarations; no operation
// joins the contract and no backend primitive changes.
//
// Domain operations that read more than one record or decide something (the
// release of a source, the replace of a cooldown window, a receipt count) are
// not here: they are helpers over these fields, in the package that owns the
// decision (design D-2).
type Stores struct {
	// Conversations is the conversation control record, keyed by the
	// conversation's name.
	Conversations Collection[ConversationRecord]
	// Runs are the append-only runs of a conversation, keyed by the
	// conversation's name. Grow merges a delivery marker onto a run under
	// RunsDecl.Merge.
	Runs Appended[ConversationRecord, Run]
	// Signals are the durable messages of admitted signals. Terminal ones
	// expire.
	Signals Expiring[SignalRecord]
	// Cooldowns holds one window per signal source, keyed by the source's
	// name. A source with nothing recorded reads not found.
	Cooldowns Collection[CooldownWindow]
	// Receipts holds one counter per signal source, keyed by the source's
	// name. A source that counted nothing reads not found.
	Receipts Collection[Received]
	// AdapterState holds the one string an adapter persists per surface and
	// key, keyed by AdapterStateKey. The last write wins.
	AdapterState Collection[string]
	// Blobs holds the out-of-line payload of a queued input.
	Blobs Blobs
}

// Declarations is every Stores field's Decl, beside the inventory so reading
// the two answers completely what the store keeps, with which indexes and
// which expiry (design D-1). A backend and the conformance suite take their
// per-kind facts from here and from nowhere else.
type Declarations struct {
	Conversations Decl[ConversationRecord]
	Runs          Decl[Run]
	Signals       Decl[SignalRecord]
	Cooldowns     Decl[CooldownWindow]
	Receipts      Decl[Received]
	AdapterState  Decl[string]
}

// Adapter surfaces, the first part of an AdapterState key: which kind of
// object the adapter registered under. Two surfaces never share a key.
const (
	SurfaceChannel      = "channel"
	SurfaceSignalSource = "signalsource"
)

// AdapterStateKey is the AdapterState key of one adapter value: the kind of
// surface, its name and the adapter's own key (design D-4).
func AdapterStateKey(surface, name, key string) Key {
	return Key(IndexValue(surface, name, key))
}

// The conversation indexes. Each is the equality lookup a manager path
// performs on a record's own field; the value is what a caller passes to List.
var (
	// ByConversationID looks up the equality match every signal re-delivery,
	// reopen's at-most-one-open check and member reuse perform on a
	// conversation's own correlation id. Value: the id.
	ByConversationID = NewIndex("conversationId", func(r *ConversationRecord) (string, bool) {
		return r.ConversationID, r.ConversationID != ""
	})
	// ByPipelineAndPhase looks up a Pipeline's own conversations in one
	// phase, the queue a Pipeline-rooted dispatch sweep reads. Value:
	// IndexValue(pipelineRef, phase).
	ByPipelineAndPhase = NewIndex("pipelineRef+phase", func(r *ConversationRecord) (string, bool) {
		return IndexValue(r.PipelineRef, r.Phase), true
	})
	// ByPhase looks up every conversation in one phase, the admission gate's
	// and housekeeping's own sweep. Value: the phase.
	ByPhase = NewIndex("phase", func(r *ConversationRecord) (string, bool) { return r.Phase, true })
	// ByCausedBy looks up a conversation's direct members: every record whose
	// CausedBy.Parent names the conversation. The close cascade, the tree walk
	// and member reuse read it. Value: the parent's name.
	ByCausedBy = NewIndex("causedBy", func(r *ConversationRecord) (string, bool) {
		if r.CausedBy == nil || r.CausedBy.Parent == "" {
			return "", false
		}
		return r.CausedBy.Parent, true
	})
	// ByNameBase looks up every record sharing one allocated word-chain, so
	// the next free name for it can be chosen. Value: the NameBase.
	ByNameBase = NewIndex("nameBase", func(r *ConversationRecord) (string, bool) {
		return r.NameBase, r.NameBase != ""
	})
	// ByAll lists every visible conversation, the admission gate's slot
	// count, the status view and the signal router's scan. Value: "".
	ByAll = NewIndex("all", func(*ConversationRecord) (string, bool) { return "", true })
)

// The signal indexes (design D-2). A terminal record is bounded by rate times
// TTL and never returned by a non-terminal index, so a flapping source does not
// slow the per-source cap. A signal whose phase is absent counts as
// non-terminal.
var (
	// SignalAll lists every visible signal: the consumer's initial pass after
	// a leader change reads it. Value: "".
	SignalAll = NewIndex("all", func(*SignalRecord) (string, bool) { return "", true })
	// SignalNonTerminalBySource lists one source's signals that are not
	// terminal, the set the per-source cap counts. Value: the source.
	SignalNonTerminalBySource = NewIndex("nonTerminalBySource", func(r *SignalRecord) (string, bool) {
		return r.SourceRef, !r.Terminal()
	})
	// SignalNonTerminalByInitiation lists one conversation's non-terminal
	// signals on one source: the earlier messages a signal must wait for.
	// Value: IndexValue(source, initiationId).
	SignalNonTerminalByInitiation = NewIndex("nonTerminalByInitiation", func(r *SignalRecord) (string, bool) {
		return IndexValue(r.SourceRef, r.InitiationID), !r.Terminal()
	})
	// SignalBySource lists one source's signals in ANY phase, the set the
	// release of a deleted source removes. Value: the source.
	SignalBySource = NewIndex("bySource", func(r *SignalRecord) (string, bool) {
		return r.SourceRef, true
	})
)

// StoreDeclarations is the one Declarations value, the inventory's facts.
var StoreDeclarations = Declarations{
	Conversations: Decl[ConversationRecord]{
		Name:    "conversations",
		Indexes: []Index[ConversationRecord]{ByConversationID, ByPipelineAndPhase, ByPhase, ByCausedBy, ByNameBase, ByAll},
	},
	Runs: Decl[Run]{
		Name:  "runs",
		ID:    func(r *Run) string { return r.RunID },
		Merge: mergeRunEdit,
	},
	Signals: Decl[SignalRecord]{
		Name:    "signals",
		Indexes: []Index[SignalRecord]{SignalAll, SignalNonTerminalBySource, SignalNonTerminalByInitiation, SignalBySource},
		Expiry:  func(r *SignalRecord) *time.Time { return r.ExpiresAt },
	},
	Cooldowns:    Decl[CooldownWindow]{Name: "cooldowns"},
	Receipts:     Decl[Received]{Name: "receipts"},
	AdapterState: Decl[string]{Name: "adapterState"},
}

// mergeRunEdit is the Runs declaration's grow-only edit: edit carries only the
// markers to set. DeliveryTracked and RoutedToParent go false to true, never
// back, and Delivered gains each edit channel not already present, keeping
// first-appearance order. Nothing is removed or overwritten, so applying an
// edit twice equals once and two edits reach the same set of effects in either
// order, which is what lets Grow take no revision and commute with an append.
// The conformance suite proves both properties; the order of Delivered is the
// order of first application, and the proof compares it as a set.
func mergeRunEdit(run *Run, edit Run) {
	run.DeliveryTracked = run.DeliveryTracked || edit.DeliveryTracked
	run.RoutedToParent = run.RoutedToParent || edit.RoutedToParent
	for _, ch := range edit.Delivered {
		if !run.DeliveredTo(ch) {
			run.Delivered = append(run.Delivered, ch)
		}
	}
}

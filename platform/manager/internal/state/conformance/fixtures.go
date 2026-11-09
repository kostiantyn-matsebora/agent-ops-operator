package conformance

// The one Fixture per field of state.Stores (design D-1 of
// state-store-generic-contract): written once here, reused by every backend
// that calls RunStores. Every Fixture's Edit/Marker pair carries an int
// marker through whichever field the kind actually allows Update to touch
// (store.go: publication fields are fixed at Create, only status-shaped
// fields move), so the generic engines in collection.go/expiring.go/
// appended.go never name a concrete field themselves.

import (
	"sort"
	"strconv"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// FixtureSourceName and FixtureChannelName are the one SignalSource and one
// Channel every Cooldowns, Receipts and AdapterState scenario below reads or
// writes through — a Part-based kind's Create needs the owning object to
// already exist (store.go's Collection on a kind whose Decl carries no
// index of its own). A backend wiring RunStores (kube's own test file)
// pre-creates both, fresh, every time its open() is called, so reusing
// these same two names across every leaf scenario is safe: each leaf's
// open() is its own fresh, empty, isolated instance.
const (
	FixtureSourceName  = "conf-source-a"
	FixtureChannelName = "conf-channel-a"
)

func intMarker(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// conversationFixture drives Run over Stores.Conversations.
var conversationFixture = Fixture[state.ConversationRecord]{
	New: func() state.ConversationRecord {
		return state.ConversationRecord{ProfileRef: profileDefault, Phase: "Working"}
	},
	Edit: func(rec state.ConversationRecord, marker int) state.ConversationRecord {
		rec.Title = strconv.Itoa(marker)
		return rec
	},
	Marker: func(rec state.ConversationRecord) int { return intMarker(rec.Title) },
	Key:    func(label string) state.Key { return state.Key("conv-" + label) },
	KeyOf:  func(rec state.ConversationRecord) state.Key { return state.Key(rec.Name) },

	Indexed: conversationIndexPool(),

	Hold: &HoldFixture[state.ConversationRecord]{
		Hold: func(rec state.ConversationRecord) state.ConversationRecord {
			rec.Finalizers = []string{"example.com/hold-a"}
			return rec
		},
		AddSecond: func(rec state.ConversationRecord) state.ConversationRecord {
			rec.Finalizers = append(append([]string{}, rec.Finalizers...), "example.com/hold-b")
			return rec
		},
		Release: func(rec state.ConversationRecord) state.ConversationRecord {
			rec.Finalizers = nil
			return rec
		},
		NumFinalizers: func(rec state.ConversationRecord) int { return len(rec.Finalizers) },
		Marked:        func(rec state.ConversationRecord) bool { return rec.DeletionRequestedAt != nil },
	},
	Identity: &IdentityFixture[state.ConversationRecord]{
		ID:        func(rec state.ConversationRecord) string { return rec.ID },
		CreatedAt: func(rec state.ConversationRecord) time.Time { return rec.CreatedAt },
		Forge: func(rec state.ConversationRecord) state.ConversationRecord {
			fake := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
			rec.ID, rec.CreatedAt, rec.DeletionRequestedAt = "caller-chosen", fake, &fake
			return rec
		},
	},
}

// conversationIndexPool builds a pool of records partitioning every one of
// state.StoreDeclarations.Conversations.Indexes into at least one matching
// and one excluded record: conversationFixture.Key is composed into each so
// it reads exactly like every other scenario's keys.
func conversationIndexPool() []Keyed[state.ConversationRecord] {
	key := func(label string) state.Key { return state.Key("conv-idx-" + label) }
	rec := func(convID, pipeline, phase, nameBase string, causedBy *state.Provenance) state.ConversationRecord {
		return state.ConversationRecord{
			ProfileRef: profileDefault, ConversationID: convID, PipelineRef: pipeline,
			Phase: phase, NameBase: nameBase, CausedBy: causedBy,
		}
	}
	a := key("a")
	return []Keyed[state.ConversationRecord]{
		{Key: a, Record: rec("id-match", "pipe-1", "Working", "base-x", nil)},
		{Key: key("b"), Record: rec("id-other", "pipe-1", "Idle", "base-x", &state.Provenance{Parent: string(a), Entry: "e"})},
		{Key: key("c"), Record: rec("id-match", "pipe-2", "Working", "base-y", &state.Provenance{Parent: string(a), Entry: "e"})},
		{Key: key("d"), Record: rec("id-other2", "pipe-2", "Idle", "base-y", nil)},
	}
}

// signalFixture drives RunExpiring over Stores.Signals. Edit's marker -1 is
// the one convention expiring.go relies on: a negative marker sets a past
// expiry instead of touching Attempts, since a time.Time has no int
// representation the way every other status field here does. Attempts,
// not Reason, carries the marker: Reason is a closed enum of drop reasons
// (agentops.dev_signals.yaml) and rejects an arbitrary int.
var signalFixture = Fixture[state.SignalRecord]{
	New: func() state.SignalRecord {
		return state.SignalRecord{SourceRef: "fixture-source", Adapter: "cron", InitiationID: "fixture-init", Attempts: -1}
	},
	Edit: func(rec state.SignalRecord, marker int) state.SignalRecord {
		if marker < 0 {
			past := time.Now().Add(time.Duration(marker) * time.Hour)
			rec.ExpiresAt = &past
			return rec
		}
		rec.Attempts = int32(marker)
		return rec
	},
	Marker: func(rec state.SignalRecord) int { return int(rec.Attempts) },
	Key:    func(label string) state.Key { return state.Key("sig-" + label) },
	KeyOf:  func(rec state.SignalRecord) state.Key { return state.Key(rec.Name) },

	Indexed: signalIndexPool(),

	Identity: &IdentityFixture[state.SignalRecord]{
		ID:        func(rec state.SignalRecord) string { return rec.ID },
		CreatedAt: func(rec state.SignalRecord) time.Time { return rec.CreatedAt },
		Forge: func(rec state.SignalRecord) state.SignalRecord {
			rec.ID, rec.CreatedAt = "forged", time.Unix(1, 0)
			return rec
		},
	},
}

// signalIndexPool partitions every one of
// state.StoreDeclarations.Signals.Indexes: SignalAll (everything),
// SignalNonTerminalBySource and SignalNonTerminalByInitiation (sig-idx-b is
// terminal and excluded from both), SignalBySource (every phase, sig-idx-b
// included).
func signalIndexPool() []Keyed[state.SignalRecord] {
	rec := func(name, source, init, phase string) state.SignalRecord {
		return state.SignalRecord{Name: name, SourceRef: source, Adapter: "cron", InitiationID: init, Phase: phase}
	}
	return []Keyed[state.SignalRecord]{
		{Key: "sig-idx-a", Record: rec("sig-idx-a", idxSourceA, idxInitX, "")},
		{Key: "sig-idx-b", Record: rec("sig-idx-b", idxSourceA, idxInitX, state.SignalRouted)},
		{Key: "sig-idx-c", Record: rec("sig-idx-c", idxSourceA, "idx-init-y", "")},
		{Key: "sig-idx-d", Record: rec("sig-idx-d", "idx-source-b", idxInitX, "")},
	}
}

// runFixture drives RunAppended over Stores.Runs. Edit's marker is the name
// of one channel Grow marks delivered, except the two reserved names below,
// which flip the booleans mergeRunEdit also only ever grows — so Effects
// proves idempotence and commutativity exactly the way stores.go's
// mergeRunEdit actually merges: booleans that only turn true, a set that
// only grows, first-appearance order (compared here as a set, per
// StoreDeclarations' own comment).
var runFixture = AppendedFixture[state.ConversationRecord, state.Run]{
	NewParent: func() state.ConversationRecord { return state.ConversationRecord{ProfileRef: profileDefault} },
	NewItem:   func(id string) state.Run { return state.Run{RunID: id, Status: "succeeded"} },
	Edit: func(marker string) state.Run {
		switch marker {
		case "tracked":
			return state.Run{DeliveryTracked: true}
		case "routed":
			return state.Run{RoutedToParent: true}
		default:
			return state.Run{Delivered: []string{marker}}
		}
	},
	Effects: func(item state.Run) []string {
		var out []string
		if item.DeliveryTracked {
			out = append(out, "tracked")
		}
		if item.RoutedToParent {
			out = append(out, "routed")
		}
		out = append(out, item.Delivered...)
		sort.Strings(out)
		return out
	},
	AllIndex: state.ByAll,
	AllValue: "",
	KeyOf:    func(rec state.ConversationRecord) state.Key { return state.Key(rec.Name) },
	// A close touches the record alone: no run is appended.
	EditParent:    func(rec *state.ConversationRecord) { rec.Phase = state.PhaseClosed },
	ParentChanged: func(rec state.ConversationRecord) bool { return rec.Phase == state.PhaseClosed },
}

// cooldownFixture drives Run over Stores.Cooldowns. The whole window is
// replaced on every write (design D-3: "a replace writes all of it"), so
// Edit ignores rec entirely and Marker reads the one entry's InitiationID —
// Key always names FixtureSourceName, the one SignalSource every leaf's
// fresh open() provisions.
var cooldownFixture = Fixture[state.CooldownWindow]{
	New: func() state.CooldownWindow {
		return state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: "seed", At: time.Now().UTC().Truncate(time.Second)}}}
	},
	Edit: func(_ state.CooldownWindow, marker int) state.CooldownWindow {
		return state.CooldownWindow{Entries: []state.CooldownEntry{{InitiationID: strconv.Itoa(marker), At: time.Now().UTC().Truncate(time.Second)}}}
	},
	Marker: func(rec state.CooldownWindow) int {
		if len(rec.Entries) == 0 {
			return -999
		}
		return intMarker(rec.Entries[0].InitiationID)
	},
	Key: func(string) state.Key { return state.Key(FixtureSourceName) },
}

// receiptFixture drives Run over Stores.Receipts. New's Total is -1, distinct
// from every marker Edit ever writes (0 and up): a write landing is proved
// by the revision moving, and a same-valued write (New's baseline happening
// to equal a scenario's first marker) would patch no actual difference and
// never move it.
var receiptFixture = Fixture[state.Received]{
	New: func() state.Received {
		now := time.Now().UTC().Truncate(time.Second)
		return state.Received{Total: -1, Last: &now}
	},
	Edit: func(_ state.Received, marker int) state.Received {
		now := time.Now().UTC().Truncate(time.Second)
		return state.Received{Total: int64(marker), Last: &now}
	},
	Marker: func(rec state.Received) int { return int(rec.Total) },
	Key:    func(string) state.Key { return state.Key(FixtureSourceName) },
}

// adapterStateFixture drives Run over Stores.AdapterState: the record IS the
// plain string a caller stores, so Edit ignores the old value — "the last
// write wins" (stores.go).
var adapterStateFixture = Fixture[string]{
	New:    func() string { return "0" },
	Edit:   func(_ string, marker int) string { return strconv.Itoa(marker) },
	Marker: intMarker,
	Key: func(label string) state.Key {
		return state.AdapterStateKey(state.SurfaceChannel, FixtureChannelName, label)
	},
}

package nats

import (
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// This file holds the layouts of the closed inventory (design D-1's bucket
// table): the one place this backend names a concrete record type outside
// the generic machinery of layout.go/collection.go.

// ConversationsLayout maps the conversation control record directly:
// ID/CreatedAt/DeletionRequestedAt/Finalizers are fields of
// state.ConversationRecord itself.
func ConversationsLayout() Layout[state.ConversationRecord] {
	return Layout[state.ConversationRecord]{
		Decl: state.StoreDeclarations.Conversations,
		Identity: &IdentityLayout[state.ConversationRecord]{
			Get: func(r *state.ConversationRecord) (string, time.Time) { return r.ID, r.CreatedAt },
			Set: func(r *state.ConversationRecord, id string, at time.Time) { r.ID, r.CreatedAt = id, at },
		},
		Hold: &HoldLayout[state.ConversationRecord]{
			Finalizers:    func(r *state.ConversationRecord) []string { return r.Finalizers },
			SetFinalizers: func(r *state.ConversationRecord, fs []string) { r.Finalizers = fs },
			DeletionRequestedAt: func(r *state.ConversationRecord) *time.Time {
				return r.DeletionRequestedAt
			},
			SetDeletionRequestedAt: func(r *state.ConversationRecord, at *time.Time) { r.DeletionRequestedAt = at },
		},
		SetName: func(r *state.ConversationRecord, name string) { r.Name = name },
	}
}

// SignalsLayout maps the signal record: ID/CreatedAt are its own fields; it
// carries no finalizer notion (a terminal Signal expires instead, design
// D-6), so Hold stays nil.
func SignalsLayout() Layout[state.SignalRecord] {
	return Layout[state.SignalRecord]{
		Decl: state.StoreDeclarations.Signals,
		Identity: &IdentityLayout[state.SignalRecord]{
			Get: func(r *state.SignalRecord) (string, time.Time) { return r.ID, r.CreatedAt },
			Set: func(r *state.SignalRecord, id string, at time.Time) { r.ID, r.CreatedAt = id, at },
		},
		SetName: func(r *state.SignalRecord, name string) { r.Name = name },
	}
}

// CooldownsLayout maps a source's whole cooldown window. It carries neither
// a minted identity nor a finalizer notion: a replace writes all of it
// (store.go), so Identity and Hold both stay nil.
func CooldownsLayout() Layout[state.CooldownWindow] {
	return Layout[state.CooldownWindow]{Decl: state.StoreDeclarations.Cooldowns}
}

// ReceiptsLayout maps a source's receipt counters, carrying neither identity
// nor a finalizer notion.
func ReceiptsLayout() Layout[state.Received] {
	return Layout[state.Received]{Decl: state.StoreDeclarations.Receipts}
}

// AdapterStateLayout maps the one string an adapter persists. The record IS
// the value (stores.go: "the last write wins"), carrying neither identity
// nor a finalizer notion.
func AdapterStateLayout() Layout[string] {
	return Layout[string]{Decl: state.StoreDeclarations.AdapterState}
}

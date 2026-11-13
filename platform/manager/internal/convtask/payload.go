// Package convtask is the "conv" queue's one wire vocabulary
// (async-command-bus design D-9/D-5): a producer (internal/httpapi's
// accept handlers, internal/controller's reconciler) builds a Payload and
// hands it to commandbus.Bus.Submit as opaque bytes; the consumer
// (internal/httpapi's HandleConvTask) decodes the same shape back.
//
// Its own package, rather than living in internal/httpapi where
// HandleConvTask itself does, because internal/controller needs to build
// one too (the idle timer's auto-close, the deadline budget edge) and
// internal/httpapi already imports internal/controller (CredentialEnvPrefix,
// ConditionContextContinuity) — the reverse import would cycle. Neither
// producer package depends on the other; both depend inward on this one.
package convtask

import (
	"encoding/json"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
)

// Queue is the one commandbus.Queue every Payload here travels on — named
// once so internal/httpapi (the accept side and HandleConvTask) and
// internal/controller (the two call sites task 5.4 moves) never spell
// "conv" as a second, driftable literal.
const Queue = commandbus.Queue("conv")

// Verb is the closed set of operations the "conv" queue carries — never
// read by the bus itself, which treats Payload.Encode's bytes as opaque.
type Verb string

const (
	VerbClose          Verb = "close"
	VerbExit           Verb = "exit"
	VerbReopen         Verb = "reopen"
	VerbDelete         Verb = "delete"
	VerbResetContext   Verb = "reset-context"
	VerbInboundMessage Verb = "inbound-message"
	// VerbCloseCoordinated is /coordinate/close's own task (async-command-bus
	// task 5.2/5.5): calls chat.Router.CloseCoordinated, which the HTTP
	// handler used to call inline. Carries BOTH names a plain VerbClose
	// never needs — Conversation is the TARGET, Caller the acting
	// conversation CloseCoordinated's own widened-scope check resolves
	// against — because the two are only sometimes the same conversation.
	VerbCloseCoordinated Verb = "close-coordinated"
	// VerbEscalate is /coordinate/escalate's own task, and the FINAL slot of
	// a budget-exceeded cascade (task 5.3/5.5): calls chat.Router.Escalate.
	// Conversation is the escalating conversation itself — escalate takes no
	// separate target.
	VerbEscalate Verb = "escalate"
)

// CloseKind picks which of the three existing chat.Router close methods a
// VerbClose task's handler calls.
type CloseKind string

const (
	CloseManual         CloseKind = "manual"
	CloseAuto           CloseKind = "auto"
	CloseBudgetExceeded CloseKind = "budget-exceeded"
)

// Payload is the "conv" queue's one wire shape. Every field beyond
// Verb/Conversation is specific to one or two verbs.
type Payload struct {
	Verb         Verb   `json:"verb"`
	Conversation string `json:"conversation"`

	// VerbClose
	CloseKind CloseKind `json:"closeKind,omitempty"`
	// Reason carries VerbClose's CloseBudgetExceeded digest, a cascade
	// member's close reason (design D-7: "same reason verbatim"),
	// VerbCloseCoordinated's required reason, or VerbEscalate's message —
	// one field, since no verb ever needs two of these at once. A plain
	// manual/auto close (CascadeMember false, CloseKind unset or
	// Manual/Auto) needs none — the Router methods compose their own
	// farewell text.
	Reason string `json:"reason,omitempty"`
	// IdleForMS is CloseAuto's idle window, round-tripped as milliseconds
	// (a Payload is JSON; time.Duration marshals as an opaque int64 of
	// nanoseconds, which is correct but unreadable on /state/tasks).
	IdleForMS int64 `json:"idleForMs,omitempty"`
	// CascadeMember marks a VerbClose task as ONE node of a decomposed close
	// cascade (design D-7, task 5.3): close exactly this conversation,
	// through chat.Router.CloseConversationReason, never the recursive
	// closeWithCascade/cascadeCloseMembers — the recursion that used to
	// happen inside a single task now lives in the accept-time (or
	// controller call-site-time) walk that built this task and its
	// siblings. CloseKind/IdleForMS are meaningless here; Reason is the
	// cascade's own reason, verbatim.
	CascadeMember bool `json:"cascadeMember,omitempty"`

	// VerbCloseCoordinated. Conversation (above) is the TARGET.
	Caller string `json:"caller,omitempty"`

	// VerbInboundMessage
	Channel  string  `json:"channel,omitempty"`
	ThreadID *string `json:"threadId,omitempty"`
	Text     string  `json:"text,omitempty"`
	Sender   string  `json:"sender,omitempty"`
}

// Encode marshals p for commandbus.SubmitRequest.Payload. Never fails — a
// struct of strings/ints/one *string — so callers compose it inline rather
// than carry an error only panic could ever reach.
func (p Payload) Encode() []byte {
	b, _ := json.Marshal(p)
	return b
}

// Decode is Encode's inverse, HandleConvTask's own entry point.
func Decode(raw []byte) (Payload, error) {
	var p Payload
	err := json.Unmarshal(raw, &p)
	return p, err
}

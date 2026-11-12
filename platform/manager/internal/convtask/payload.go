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
	// Reason carries VerbClose's CloseBudgetExceeded digest. A plain
	// manual/auto close needs none — the Router methods compose their own
	// farewell text.
	Reason string `json:"reason,omitempty"`
	// IdleForMS is CloseAuto's idle window, round-tripped as milliseconds
	// (a Payload is JSON; time.Duration marshals as an opaque int64 of
	// nanoseconds, which is correct but unreadable on /state/tasks).
	IdleForMS int64 `json:"idleForMs,omitempty"`

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

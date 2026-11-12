// The accept step (async-command-bus design D-9): every handler below does
// the CHEAP part inline — auth (already run by the surrounding
// adapter/state auth middleware), shape, an unknown channel/source/
// conversation, a refusal a plain read already settles — then mints one
// invocationId, durably enqueues on the command bus and replies. The work
// itself runs later, on whichever replica's consumer.Pool (cmd/manager)
// next claims the task; HandleConvTask (convtasks.go) is that pool's
// Handler.
//
// Queue and Key, by command (this package's own reading of design D-5,
// which names "conv" and "in.signal" but leaves the exact command->queue->
// key table to whoever wires the manager's own verbs onto it):
//
//	Command                                   Queue  Key
//	/close, /exit (chat, on a bound thread)    conv   the conversation
//	a plain inbound chat reply                 conv   the conversation
//	reopen, delete, reset-context               conv   the conversation
//	the idle timer's auto-close                 conv   the conversation
//	a budget-exceeded close+escalate             conv   the conversation
//	                                                    (always a Coordinator
//	                                                    root by construction)
//
// A conversation IS its own cascade's root for every verb this package
// enqueues: /close's and the idle timer's cascades run wholesale inside one
// task, calling the exact Router methods that already recurse live over
// `causedBy` (chat.Router.CloseConversation/AutoCloseConversation/
// CloseBudgetExceeded, unchanged) — so "the ancestor re-checks its
// descendants when it runs" (design D-7) holds for them today, at the cost
// of the per-member ledger visibility a fuller decomposition would add
// (see the package-level report this change was handed back with). DELETE
// is the one verb decomposed per D-7's letter: handleConversationDelete
// enqueues one task per already-closed descendant, deepest-first, ending
// with the target itself, all sharing the target's name as Key (task 5.10).
//
// in.signal (source + initiationId) carries no producer in this change —
// see the hand-back report for why handleSignalInbound's own publish/
// answerCommands split was deferred rather than guessed at.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"time"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
)

// ConvQueue is the one Queue every handler in this file and convtasks.go
// enqueues onto — see this file's own doc comment for why a conversation
// is always its own Key. Re-exported from internal/convtask, the package
// internal/controller's own two call sites (task 5.4) share it from —
// one name, not two driftable literals.
const ConvQueue = convtask.Queue

// acceptedResponse is ADDED to whatever an endpoint's body already was
// (design D-9's own "added to whatever the endpoint returned before") —
// every accept handler below embeds it rather than replacing the response
// shape outright.
type acceptedResponse struct {
	InvocationID string   `json:"invocationId"`
	TaskIDs      []string `json:"taskIds"`
}

// newInvocationID mints a human-legible, request-scoped batch tag (design
// D-9 step 7) — never a dedup key itself, so collision only costs a
// confusing /state/tasks grouping, never a wrong suppression. Time-ordered
// like generatedSignalName's own id, plus a short random tail so two
// accepts within the same millisecond still get distinct tags.
func newInvocationID() string {
	var buf [4]byte
	_, _ = rand.Read(buf[:])
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf[:])
	return fmt.Sprintf("inv-%d-%s", time.Now().UnixMilli(), suffix)
}

// acceptAndEnqueue is the one place every accept handler reaches the bus
// (design D-9 step 7): stamp one invocationId onto every req — a batch's
// members and the single-task case alike — and submit each in order. Order
// matters for a cascade (design D-7: "members go before the parent"): reqs
// must already be sorted that way by the caller, acceptAndEnqueue submits
// strictly in slice order and never reorders or parallelizes.
//
// A Submit failure partway through a batch is NOT rolled back — the bus has
// no multi-task transaction (design non-goal: "multi-task atomicity") — so a
// caller gets back the invocationId and whichever taskIds landed before the
// error, alongside the error itself, and can inspect /state/tasks for what
// was actually accepted rather than guess.
func (s *Server) acceptAndEnqueue(ctx context.Context, reqs ...commandbus.SubmitRequest) (acceptedResponse, error) {
	resp := acceptedResponse{InvocationID: newInvocationID(), TaskIDs: make([]string, 0, len(reqs))}
	for i := range reqs {
		reqs[i].InvocationID = resp.InvocationID
		task, err := s.Bus.Submit(ctx, reqs[i])
		if err != nil {
			return resp, fmt.Errorf("enqueue %s (command %s): %w", reqs[i].Queue, reqs[i].CommandID, err)
		}
		resp.TaskIDs = append(resp.TaskIDs, task.UID)
	}
	return resp, nil
}

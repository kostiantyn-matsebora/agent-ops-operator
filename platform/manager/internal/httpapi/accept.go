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
//	the idle timer's auto-close                 conv   the conversation (root)
//	a budget-exceeded close+escalate             conv   the conversation (root)
//	                                                    (always a Coordinator
//	                                                    root by construction)
//	/coordinate/close                           conv   the TARGET (root)
//	/coordinate/escalate                        conv   the conversation
//	/signal/inbound, one admitted signal   in.signal   source + initiationId
//	  (async-signal-ingest)
//
// A conversation is its own cascade's root for every close-shaped verb
// this package or internal/controller enqueues (design D-7, task 5.3):
// the idle timer's auto-close and a budget-exceeded close+escalate each
// walk their own target's causedBy subtree (closeCascadeRequests,
// closecascade.go) and submit one CascadeMember task per LIVE descendant,
// members before the parent, ahead of that verb's OWN final request — the
// exact Router call that already recursed live over causedBy before this
// change (chat.Router.CloseConversation/AutoCloseConversation/
// CloseBudgetExceeded, unchanged), now reused as the ancestor's own
// run-time re-check.
//
// /coordinate/close does NOT pre-build a member cascade this way — its
// scope check is deferred into the one task it submits and gates the
// whole subtree, so Router.CloseCoordinated performs its own cascade
// internally, only once it has confirmed the caller may act. See
// handleCoordinateClose's and closecascade.go's own doc comments.
//
// A plain chat `/close` (and the console's bulk close, N individual chat
// closes) is the one close-shaped verb this does NOT reach: it is
// discovered only by parsing text inside chat.Router.HandleMessage, which
// already runs INSIDE a single "conv" task with no commandbus.Bus of its
// own to reach — see closecascade.go's own doc comment for why. It still
// cascades correctly, wholesale, inside that one task.
//
// DELETE is decomposed per D-7's letter too, over an already fully-closed
// subtree with no run-time re-check needed (a Closed conversation can
// invoke no new member): handleConversationDelete enqueues one task per
// already-closed descendant, deepest-first, ending with the target itself,
// all sharing the target's name as Key (task 5.10).
//
// in.signal now has both a producer and a consumer (async-signal-ingest):
// handleSignalInbound (publish.go) is the accept step, enqueuing one
// ingest-signal task per admitted signal, and HandleIngestSignalTask
// (ingesttask.go) is that queue's own consumer.Pool Handler — answering an
// in-place chat command or creating the Signal CR, work that used to run
// inside the request.
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

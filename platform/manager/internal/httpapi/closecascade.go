// The close cascade's shared accept-time decomposition (async-command-bus
// design D-7, task 5.3): every close-shaped entry point this package or
// internal/controller owns that is NOT gated by a deferred per-call
// authorization check — the idle timer's auto-close and CloseBudgetExceeded
// (both call sites) — walks its own target's causedBy subtree and builds
// one CascadeMember task per LIVE descendant, members before the parent,
// all sharing the target's own name as Key, ending with that top-level
// verb's OWN unchanged final request.
//
// /coordinate/close is deliberately NOT one of these callers (see
// handleCoordinateClose's own doc comment, coordinate.go): its scope check
// is deferred into the final task and gates the whole subtree, so
// pre-building a member cascade ahead of that check would close live
// descendants before an unauthorized request's own task ever failed it.
//
// Deliberately not every close-shaped entry point: a plain chat `/close`
// (and the console's bulk close, already established as N individual chat
// closes) is discovered only by PARSING the inbound message's text, inside
// chat.Router.HandleMessage — which runs already INSIDE a single "conv"
// task (taskInboundMessage) that has no commandbus.Bus of its own to reach
// (chat.Router holds no Bus field, and giving it one would be the one
// place this package's own dependency direction — httpapi/controller
// depend into chat, never the reverse — would have to bend). Decomposing
// it further would mean either chat importing commandbus directly, or
// httpapi re-parsing command text ahead of HandleMessage, which
// taskInboundMessage's own doc comment already argues against (forking one
// piece of logic into two places that must then be kept in step). Left as
// the documented simplification it already was: a /close command still
// cascades correctly, wholesale, inside the one task HandleMessage runs in
// — see accept.go's own package doc.
package httpapi

import (
	"context"

	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
)

// closeCascadeRequests lists rootName's own causedBy subtree
// (chat.Router.Descendants: current state, any depth) and returns one
// CascadeMember request per LIVE descendant (convtask.CloseMemberRequests),
// deepest-first, with rootReq appended last — rootReq is the caller's own
// top-level request, carrying whatever verb/payload that entry point's
// unchanged Router call needs (CloseConversation, AutoCloseConversation,
// CloseBudgetExceeded or CloseCoordinated) to perform its own close AND
// re-check its descendants fresh when it runs — design D-7's "the ancestor
// re-checks its descendants at run time and closes any open one" is this
// unchanged recursive call's own existing behaviour, simply moved from an
// inline call into a task rather than reimplemented here.
func (s *Server) closeCascadeRequests(ctx context.Context, rootName, reason string, rootReq commandbus.SubmitRequest) ([]commandbus.SubmitRequest, error) {
	descendants, err := s.Router.Descendants(ctx, rootName)
	if err != nil {
		return nil, err
	}
	reqs := convtask.CloseMemberRequests(rootName, reason, descendants)
	return append(reqs, rootReq), nil
}

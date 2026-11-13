// Cascade ordering (async-command-bus design D-7, task 5.3): the one place
// a causedBy subtree's flat descendant list becomes an ORDERED set of
// commandbus.SubmitRequest — deepest-first, the root last, every one
// sharing the root's own name as Key. internal/httpapi (delete, task 5.10;
// every close-shaped verb, task 5.3) and internal/controller (the idle
// timer's auto-close, the deadline budget edge) both need this exact
// ordering over a list chat.Router.Descendants already resolved, so it
// lives here rather than in either producer package — the same "both
// depend inward on this one" reason Payload itself does (see this
// package's own doc comment).
package convtask

import (
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// CascadeOrder turns a flat descendant list into deepest-first post-order,
// rootName last — a child's own children are visited (and so ordered)
// before it, and rootName is visited only once every other name has been.
// Built from CausedBy.Parent alone (an index the caller resolved the list
// against, such as state.ByCausedBy, is a hint, never trusted twice here).
// The algorithm httpapi's own deleteCascadeOrder first walked for delete
// (task 5.10), lifted here so the close cascade (task 5.3) does not
// reimplement it.
func CascadeOrder(rootName string, descendants []state.ConversationRecord) []string {
	children := map[string][]string{}
	for i := range descendants {
		d := &descendants[i]
		if d.CausedBy != nil {
			children[d.CausedBy.Parent] = append(children[d.CausedBy.Parent], d.Name)
		}
	}
	var out []string
	var visit func(name string)
	visit = func(name string) {
		for _, child := range children[name] {
			visit(child)
		}
		out = append(out, name)
	}
	visit(rootName)
	return out
}

// CloseMemberRequests builds one CascadeMember-flagged "close" task per LIVE
// descendant of rootName, in CascadeOrder, EXCLUDING rootName itself — the
// caller supplies its own final request for rootName, since manual, auto,
// budget-exceeded and coordinated close all need a different one (design
// D-7: "the ancestor re-checks its descendants at run time" — that re-check
// is each top-level verb's own already-existing recursive Router call,
// reused unchanged as the final request, never reimplemented here). Every
// request shares rootName as Key, so the whole subtree serialises on one
// lane ahead of that final request.
//
// An already-Closed descendant gets no task: CloseConversationReason is
// idempotent on one anyway, but a task that can only ever no-op is a
// /state/tasks entry with nothing to show for itself.
func CloseMemberRequests(rootName, reason string, descendants []state.ConversationRecord) []commandbus.SubmitRequest {
	names := CascadeOrder(rootName, descendants)
	live := make(map[string]bool, len(descendants))
	for i := range descendants {
		if descendants[i].Phase != state.PhaseClosed {
			live[descendants[i].Name] = true
		}
	}
	reqs := make([]commandbus.SubmitRequest, 0, len(names))
	for _, name := range names {
		if name == rootName || !live[name] {
			continue
		}
		reqs = append(reqs, commandbus.SubmitRequest{
			Queue:     Queue,
			CommandID: commandbus.CommandID("close-member:" + name),
			Key:       commandbus.Key(rootName),
			Payload: Payload{
				Verb: VerbClose, Conversation: name,
				CascadeMember: true, Reason: reason,
			}.Encode(),
		})
	}
	return reqs
}

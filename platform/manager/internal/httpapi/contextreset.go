package httpapi

import (
	"context"
	"net/http"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/convtask"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// POST /channel/conversations/{name}/reset-context — the way out of an
// unrecoverable context loss.
//
// Before this, a conversation whose context store was destroyed had two
// options, both bad: fail every subsequent run forever, because a promised
// context that cannot be reached FAILS rather than silently starting fresh; or
// be deleted, which throws away its threads and its entire recorded history for
// a reason that has nothing to do with them.
//
// This is the third: clear the handle, keep everything else, and SAY SO.
//
// EXPLICIT AND OPERATOR-INITIATED, always. It is deliberately not something a
// failed continuation can trigger on its own, because an automatic version
// would be indistinguishable from the silent degradation the continuity rules
// exist to forbid — an agent quietly answering without its memory, and nobody
// able to tell that it had lost one.
//
// A MANAGER VERB whose reach is the BINDING, exactly as reopen and delete are:
// the caller is authorised against the channels the conversation is bound to,
// read off the conversation and never off the request.
//
// async-command-bus (design D-9): the binding check above is the one cheap
// read accept keeps — everything this verb used to do synchronously (the
// patch, the announcement) now runs inside the "conv" queue's task
// (convtasks.go's taskResetContext). BREAKING: this reply used to be `200`;
// it is `202` now, the same accepted-not-performed semantics reopen and
// delete already carried (task 5.9: docs/contracts/channel-adapter.md names
// no status code for this endpoint specifically, so this is additive, not a
// spec conflict — see this change's hand-back report).
func (s *Server) handleContextReset(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.reachConversation(w, r)
	if !ok {
		return
	}
	resp, err := s.acceptAndEnqueue(r.Context(), commandbus.SubmitRequest{
		Queue:     ConvQueue,
		CommandID: commandbus.CommandID("reset-context:" + conv.Name),
		Key:       commandbus.Key(conv.Name),
		Payload:   convtask.Payload{Verb: convtask.VerbResetContext, Conversation: conv.Name}.Encode(),
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "invocationId": resp.InvocationID, "taskIds": resp.TaskIDs})
}

// announceContextReset posts a notice to each bound thread — called from
// taskResetContext (convtasks.go), after the patch it announces has
// actually landed.
//
// A TYPED message, never rendered text: the manager composes meaning and the
// adapter composes presentation, so no transport dialect appears here.
func (s *Server) announceContextReset(ctx context.Context, conv *state.ConversationRecord) {
	if s.Ops == nil {
		return
	}
	for _, ref := range conv.ChannelRefs {
		var ch agentopsv1alpha1.Channel
		key := types.NamespacedName{Namespace: s.Namespace, Name: ref}
		if err := s.Reader.Get(ctx, key, &ch); err != nil {
			continue // channel gone: nobody left to tell
		}
		s.Ops.EnqueueMessage(ctx, &ch, conv, conv.ThreadFor(ref), chat.Warn(
			"This conversation's stored context was reset by an operator. "+
				"Its previous memory is gone, and the next message starts fresh."))
	}
}

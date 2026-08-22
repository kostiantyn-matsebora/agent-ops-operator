package httpapi

import (
	"net/http"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/controller"
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
func (s *Server) handleContextReset(w http.ResponseWriter, r *http.Request) {
	conv, ok := s.reachConversation(w, r)
	if !ok {
		return
	}

	had := conv.ContextID()
	if had == "" {
		// Nothing to clear. Reported as success rather than as an error: the
		// caller wanted a conversation with no stale handle, and that is what
		// they have.
		writeJSON(w, 200, map[string]any{
			"ok": true, "cleared": false,
			"detail": "conversation carries no context handle; nothing to reset",
		})
		return
	}

	patch := client.MergeFrom(conv.DeepCopy())
	conv.Status.RuntimeContextID = ""
	// The retired spelling is cleared too. Leaving it would let the dual-read
	// that exists for upgrades resurrect the handle this call just removed.
	conv.Status.SessionID = ""
	conv.Status.ContextCheckpoint = nil
	apimeta.SetStatusCondition(&conv.Status.Conditions, metav1.Condition{
		Type:   controller.ConditionContextContinuity,
		Status: metav1.ConditionFalse,
		Reason: "ResetByOperator",
		Message: "an operator reset this conversation's context; its previous memory is " +
			"gone and the next run starts fresh",
	})
	if err := s.Client.Status().Patch(r.Context(), conv, patch); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	// SAY SO, on every bound thread. A conversation that silently resumed
	// without its memory is the failure this verb exists to make impossible,
	// so the announcement is part of the verb rather than the caller's job.
	s.announceContextReset(r, conv)

	writeJSON(w, 200, map[string]any{"ok": true, "cleared": true})
}

// announceContextReset posts a notice to each bound thread.
//
// A TYPED message, never rendered text: the manager composes meaning and the
// adapter composes presentation, so no transport dialect appears here.
func (s *Server) announceContextReset(r *http.Request, conv *agentopsv1alpha1.Conversation) {
	if s.Ops == nil {
		return
	}
	for _, ref := range conv.Spec.ChannelRefs {
		var ch agentopsv1alpha1.Channel
		key := types.NamespacedName{Namespace: s.Namespace, Name: ref.Name}
		if err := s.Reader.Get(r.Context(), key, &ch); err != nil {
			continue // channel gone: nobody left to tell
		}
		s.Ops.EnqueueMessage(r.Context(), &ch, conv.ThreadFor(ref.Name), chat.Warn(
			"This conversation's stored context was reset by an operator. "+
				"Its previous memory is gone, and the next message starts fresh."))
	}
}

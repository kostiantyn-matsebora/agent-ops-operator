// A Coordinator root's turn ends in a move (coordination-loop, design D-B):
// `invoke`, `escalate`, `close`, or an answer to the person who asked. The
// verbs stamp the move on the caller's inflight run (coordinate.go, design
// D-A); `/work/done` reads the stamp here, after recording the run and before
// deciding its phase, and refuses to let an undecided turn end — handing it
// back once as an input of origin `manager`, escalating on a second miss in a
// row. The manager never picks the move.
package httpapi

import (
	"context"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
)

// turnEvidence is what the manager knows about a completed run from its OWN
// inflight entry, carried out of it before `/work/done` clears it. A runtime's
// report (`toolCalls[]`) never feeds this — it stays telemetry.
type turnEvidence struct {
	// Dispatched: the report named the run the manager dispatched and its
	// inputs were consumed rather than held. A report matching no inflight run
	// is not a turn, and nothing is judged.
	Dispatched bool
	// Moved: an accepted `invoke`, `escalate` or `close` stamped this run.
	Moved bool
}

// rootTurnVerdict is what judgeRootTurn concluded about one completed run.
type rootTurnVerdict int

const (
	// turnEnded: not a miss, or not a turn the manager judges at all.
	turnEnded rootTurnVerdict = iota
	// turnFirstMiss: the run ended undecided and the one before it did not.
	turnFirstMiss
	// turnSecondMiss: the run consumed exactly the corrective input the first
	// miss appended, and ended undecided again.
	turnSecondMiss
)

// maxRosterNames bounds the `agents[]` names the corrective texts repeat, so a
// wide Coordinator cannot turn one input into a page.
const maxRosterNames = 12

// maxDigestResultRunes bounds the last result quoted in the escalation digest.
const maxDigestResultRunes = 500

// judgeRootTurn decides whether a completed run is a MISS (coordination-loop:
// "Every root turn ends in a move"), from the manager's own evidence alone.
//
// Only a conversation carrying `coordinatorRef` is judged — a member
// (`causedBy` and no `coordinatorRef`) answers its parent with its result,
// which IS its move; a nested Coordinator's root (both set) is judged as a
// root. A failed run takes the existing failure paths. A Closed conversation
// is inert and never judged, which is also what makes `close` on the caller
// itself end the turn cleanly. A turn that consumed any input a person typed
// (origin `channel`) is answered by its own text, whatever verbs it called.
func judgeRootTurn(conv *agentopsv1alpha1.Conversation, status string, ev turnEvidence,
	recorded []agentopsv1alpha1.RecordedInput) rootTurnVerdict {

	if conv.Spec.CoordinatorRef == nil || !ev.Dispatched || ev.Moved || status != "succeeded" ||
		conv.Status.Phase == agentopsv1alpha1.ConversationClosed || answeredAPerson(recorded) {
		return turnEnded
	}
	// An escalated root is waiting on a person: the escalation WAS its move,
	// and demanding another only invites the cheapest one — a close that
	// archives the thread the person was asked to act in.
	if conv.Status.EscalatedAt != nil {
		return turnEnded
	}
	if consumedOnlyTheCorrectiveInput(recorded) {
		return turnSecondMiss
	}
	return turnFirstMiss
}

// answeredAPerson reports whether any input this run consumed was a person's
// message. A person asked, and the run's text is the answer.
//
// A message typed into a channel records origin `channel`. A message that
// reached the manager as a chat signal, such as one that opened the root
// through a chat origination adapter, records origin `signal` but ALSO
// records the surface it was typed on. An alert, a job tick or a posted task
// records no surface, so the surface is what separates a person from a
// machine for a signal-origin input.
func answeredAPerson(recorded []agentopsv1alpha1.RecordedInput) bool {
	for i := range recorded {
		if recorded[i].Origin == agentopsv1alpha1.OriginChannel || recorded[i].Surface != "" {
			return true
		}
	}
	return false
}

// consumedOnlyTheCorrectiveInput is the "two misses IN A ROW" test: the run's
// consumed inputs are exactly one input of origin `manager`. Anything else
// between the two misses — a member's result, a signal — resets the count.
func consumedOnlyTheCorrectiveInput(recorded []agentopsv1alpha1.RecordedInput) bool {
	return len(recorded) == 1 && recorded[0].Origin == agentopsv1alpha1.OriginManager
}

// actOnMiss performs a verdict's consequence on the conversation itself and
// reports whether a corrective input was appended, so the caller lands the
// phase Queued for it. Never fails `/work/done`: the run is already recorded,
// and a consequence that could not be written is logged for the record.
func (s *Server) actOnMiss(ctx context.Context, conv *agentopsv1alpha1.Conversation,
	runID, result string, verdict rootTurnVerdict) bool {

	switch verdict {
	case turnFirstMiss:
		return s.appendCorrectiveInput(ctx, conv, runID)
	case turnSecondMiss:
		// A COPY: Escalate patches the object it is handed and the API's reply
		// overwrites it, which would discard the run record /work/done is still
		// holding in memory for its own status write.
		s.escalateSecondMiss(ctx, conv.DeepCopy(), result)
	}
	return false
}

// appendCorrectiveInput hands the turn back ONCE (design D-C): one ordinary
// `task` input, origin `{manager, move-per-turn}`, naming the rule and the
// roster read at this moment. Idempotent per run, so a retried report
// appends it once.
func (s *Server) appendCorrectiveInput(ctx context.Context, conv *agentopsv1alpha1.Conversation, runID string) bool {
	text := correctiveInputText(s.coordinatorRoster(ctx, conv))
	if err := s.Router.AppendManagerInput(ctx, conv.Name, chat.MoveInputID(conv.Name, runID), text); err != nil {
		log.FromContext(ctx).Info("appending the corrective input after a turn without a move",
			"conversation", conv.Name, "runId", runID, "error", err.Error())
		return false
	}
	return true
}

// escalateSecondMiss hands the stuck root to a person (design D-D): the
// escalation digest, never a close — nothing was exceeded. An already
// escalated root replays no digest (Escalate is a no-op there), so the miss is
// logged and the corrective loop stops; the person already holds the thread.
func (s *Server) escalateSecondMiss(ctx context.Context, conv *agentopsv1alpha1.Conversation, result string) {
	logger := log.FromContext(ctx).WithValues("conversation", conv.Name)
	if conv.Status.EscalatedAt != nil {
		logger.Info("a second turn without a move on an already escalated root; the person holds the thread")
		return
	}
	digest := secondMissDigest(s.coordinatorRoster(ctx, conv), result)
	if err := s.Router.Escalate(ctx, conv, digest); err != nil {
		logger.Info("escalating after two turns without a move", "error", err.Error())
	}
}

// coordinatorRoster reads the `agents[]` names of the Coordinator this root
// runs for, bounded, at the moment of the miss. An unreadable Coordinator
// yields no names, and the texts then say "the roster" instead.
func (s *Server) coordinatorRoster(ctx context.Context, conv *agentopsv1alpha1.Conversation) []string {
	if conv.Spec.CoordinatorRef == nil {
		return nil
	}
	var co agentopsv1alpha1.Coordinator
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: s.Namespace, Name: conv.Spec.CoordinatorRef.Name}, &co); err != nil {
		return nil
	}
	names := make([]string, 0, len(co.Spec.Agents))
	for i := range co.Spec.Agents {
		if len(names) == maxRosterNames {
			break
		}
		names = append(names, co.Spec.Agents[i].Name)
	}
	return names
}

// correctiveInputText is the fixed hand-back (design D-C): it names the rule
// the model has read in its tool descriptions (design D-G) and the roster, and
// decides nothing — the three moves are listed, none is chosen.
func correctiveInputText(roster []string) string {
	return "Your last turn ended without a move. Every turn ends in invoke, escalate, close, " +
		"or an answer to the person who asked. Nobody asked this time. Read the roster and decide: " +
		"invoke one of " + rosterPhrase(roster) + ", escalate to a person, or close with a reason."
}

// secondMissDigest is the fixed escalation text (design D-D): what happened,
// who was available, and what the last run said, bounded.
func secondMissDigest(roster []string, result string) string {
	digest := "The coordinating agent ended two turns in a row without invoking, escalating or closing (" +
		chat.MoveRuleName + "). Roster: " + rosterPhrase(roster) + "."
	if last := strings.TrimSpace(result); last != "" {
		digest += " Its last result: " + truncateRunes(last, maxDigestResultRunes)
	}
	return digest
}

// rosterPhrase renders the names for prose, or "the roster" when none could
// be read.
func rosterPhrase(roster []string) string {
	if len(roster) == 0 {
		return "the roster"
	}
	return strings.Join(roster, ", ")
}

// truncateRunes cuts at a rune boundary: a byte cut would halve a multi-byte
// character, and a run's result is exactly where non-ASCII shows up.
func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

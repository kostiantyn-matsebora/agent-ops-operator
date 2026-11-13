package chat

import (
	"context"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/types"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

// reopenNoticeText is what a reopened conversation says on every bound thread.
const reopenNoticeText = "↩️ Conversation reopened — this thread is live again, with everything above it intact. " +
	"Reply here to continue."

// entryKind names what a thread entry is. The order of the constants is the
// tiebreak between entries stamped the same instant: a reopen opens the
// stretch, an escalation precedes the reply of the run it happened in, and a
// run's reply precedes an input received no earlier than it finished.
type entryKind int

const (
	entryReopenNotice entryKind = iota
	entryEscalation
	entryRunReply
	entryInput
)

// threadEntry is one owed line of a conversation's thread.
type threadEntry struct {
	kind entryKind
	// at is when it happened. undated entries (a run with no recorded finish)
	// sort after everything that has a time, which is where the reconciler
	// backstop always delivered them.
	at      time.Time
	undated bool
	input   *state.InputItem
	run     *state.Run
}

// fanOutTarget is one bound channel a thread entry may be delivered to.
type fanOutTarget struct {
	ref      string
	threadID string // empty: a binding with no topic yet, only a placeholder
	channel  *agentopsv1alpha1.Channel
}

// FanOutThread is THE ONE POINT that enqueues a thread entry (design D-15).
// Given the conversation record, its runs and its inputs, it enqueues every
// owed entry — each input, each tracked run's reply, the escalation digest and
// the reopen notice — to the queue of every bound channel, IN THE ORDER THEY
// HAPPENED: entries are sorted by their timestamp (input ReceivedAt, run
// FinishedAt, escalation time; the reopen notice opens the pass), with a run's
// reply before an input received no earlier than it finished. A queue keeps
// enqueue order and nothing more, so this is what keeps a thread reading in
// the order things happened — measured live, a nudge was once enqueued before
// the analysis it reacted to and reached the console first.
//
// It is called from the router the moment an input lands, from `/work/done`
// the moment a run is recorded, and from the reconciler on every pass as the
// BACKSTOP, which is why every rule below is idempotent:
//
//   - every op id is stable (per conversation, input or run, channel), so a
//     re-enqueue dedups against the bus's pending/settled window, and a run's
//     delivery marker on the record covers the window after a restart;
//   - an input is delivered to every bound channel EXCEPT the surface it was
//     typed on when that surface shows its own messages ("already seen" is a
//     fact about a SURFACE, never about a message), and an input that
//     predates provenance is delivered nowhere;
//   - a run's reply skips a channel already marked delivered (settling a stale
//     undelivered note), and a run written by a manager that did not track
//     delivery is skipped entirely — the caller backfills it;
//   - a binding with no thread yet is skipped, and a channel owed a run reply
//     it could not yet receive is reported in the result, so the reconcile
//     the thread triggers comes back for it.
//
// A COORDINATOR ROOT'S CHANNELS ARE BOUND AT CREATION, NOT AT ESCALATION
// (coordinator-unconditional-channels): every channel in `conv.ChannelRefs`
// already has a thread from the conversation's start, so nothing is fenced on
// `status.escalatedAt`.
//
// The result names every channel still owed a run reply (true), for the
// reconciler's DeliveryPending condition. A nil ops or an unbound conversation
// delivers nothing and owes nothing.
func FanOutThread(ctx context.Context, env Env, ops *OpQueue, conv *state.ConversationRecord, runs []state.Run) map[string]bool {
	owed := map[string]bool{}
	if ops == nil || len(conv.ChannelRefs) == 0 {
		return owed
	}
	f := &fanOut{
		pass: &deliveryPass{env: env, conv: conv, echoes: map[string]bool{}},
		ops:  ops, conv: conv, owed: owed,
		targets: boundTargets(ctx, env, conv),
	}
	for _, e := range orderedEntries(conv, runs) {
		f.deliver(ctx, e)
	}
	return owed
}

// boundTargets resolves every bound channel that has a thread binding, once
// per pass. A channel that is gone or names no adapter keeps its target with a
// nil channel: it owes a run reply nobody can receive, exactly as before.
func boundTargets(ctx context.Context, env Env, conv *state.ConversationRecord) []fanOutTarget {
	var targets []fanOutTarget
	for _, ref := range conv.ChannelRefs {
		binding := conv.Thread(ref)
		if binding == nil {
			continue
		}
		t := fanOutTarget{ref: ref, threadID: binding.ThreadID}
		if t.threadID != "" {
			var ch agentopsv1alpha1.Channel
			if err := env.Reader.Get(ctx, types.NamespacedName{Namespace: env.Namespace, Name: ref}, &ch); err == nil && ch.Spec.Adapter != "" {
				t.channel = &ch
			}
		}
		targets = append(targets, t)
	}
	return targets
}

// orderedEntries collects every entry the conversation may owe and sorts them
// causally. The sort is stable, so entries equal in both time and kind keep
// the order the record holds them in.
func orderedEntries(conv *state.ConversationRecord, runs []state.Run) []threadEntry {
	var entries []threadEntry
	if conv.Reopens > 0 {
		entries = append(entries, threadEntry{kind: entryReopenNotice})
	}
	if conv.EscalatedAt != nil {
		entries = append(entries, threadEntry{kind: entryEscalation, at: *conv.EscalatedAt})
	}
	for i := range runs {
		run := &runs[i]
		if !run.DeliveryTracked {
			continue
		}
		e := threadEntry{kind: entryRunReply, run: run}
		if run.FinishedAt != nil {
			e.at = *run.FinishedAt
		} else {
			e.undated = true
		}
		entries = append(entries, e)
	}
	for i := range conv.Inputs {
		item := &conv.Inputs[i]
		if item.Origin == nil { // predates provenance: delivered nowhere
			continue
		}
		entries = append(entries, threadEntry{kind: entryInput, at: item.ReceivedAt, input: item})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.undated != b.undated {
			return b.undated
		}
		if !a.at.Equal(b.at) {
			return a.at.Before(b.at)
		}
		return a.kind < b.kind
	})
	return entries
}

// fanOut is the state one FanOutThread call carries.
type fanOut struct {
	pass    *deliveryPass
	ops     *OpQueue
	conv    *state.ConversationRecord
	owed    map[string]bool
	targets []fanOutTarget
}

// deliver enqueues one entry to every target that should show it.
func (f *fanOut) deliver(ctx context.Context, e threadEntry) {
	switch e.kind {
	case entryReopenNotice:
		f.eachThread(func(ch *agentopsv1alpha1.Channel, tid *string) {
			f.ops.EnqueueReopenNotice(ctx, ch, f.conv, tid, Notice(reopenNoticeText))
		})
	case entryEscalation:
		f.eachThread(func(ch *agentopsv1alpha1.Channel, tid *string) {
			f.ops.EnqueueEscalationMessage(ctx, ch, f.conv, tid, Notice(f.conv.EscalationMessage))
		})
	case entryRunReply:
		f.deliverRunReply(ctx, e.run)
	case entryInput:
		f.deliverInput(ctx, e.input)
	}
}

// eachThread runs fn for every target that has a thread and a serving channel.
func (f *fanOut) eachThread(fn func(ch *agentopsv1alpha1.Channel, tid *string)) {
	for i := range f.targets {
		t := &f.targets[i]
		if t.channel == nil {
			continue
		}
		fn(t.channel, &t.threadID)
	}
}

// deliverRunReply enqueues one tracked run's reply to every bound thread that
// has not received it, and records in owed each channel still owing it.
func (f *fanOut) deliverRunReply(ctx context.Context, run *state.Run) {
	msg := RunReplyMessage(run)
	for i := range f.targets {
		t := &f.targets[i]
		if run.DeliveredTo(t.ref) {
			// A crash between markDelivered's run write and its note write
			// leaves the note naming a delivered run; this is the only pass
			// that visits it.
			if f.pass.env.Conversations != nil {
				SettleDeliveredNote(ctx, f.pass.env.Conversations, f.conv, t.ref, run.RunID)
			}
			continue
		}
		f.owed[t.ref] = true
		// A binding carrying no ThreadID yet is a dispatch in flight, not a
		// topic. EnqueueRunReply would still accept the empty id and the
		// adapter would park the reply on a channel-level pseudo-thread,
		// marking it delivered there PERMANENTLY — the real thread created
		// moments later would never receive the answer the person is waiting
		// for. Leaving it owed is what brings the reconcile back once the
		// thread exists.
		if t.threadID == "" || t.channel == nil {
			continue
		}
		f.ops.EnqueueRunReply(ctx, t.channel, f.conv, run.RunID, &t.threadID, msg)
	}
}

// deliverInput enqueues one input to every bound thread but the surface that
// displayed it.
func (f *fanOut) deliverInput(ctx context.Context, item *state.InputItem) {
	msg, surface, echoes := f.pass.inputMessage(ctx, item)
	f.eachThread(func(ch *agentopsv1alpha1.Channel, tid *string) {
		if item.DeliverTo(ch.Name, surface, echoes) {
			f.ops.EnqueueInputDelivery(ctx, ch, f.conv, item.ID, tid, msg)
		}
	})
}

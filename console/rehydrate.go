package main

import (
	"fmt"
	"sort"
	"time"
)

// Merging the LIVE buffer with the DURABLE record.
//
// The live buffer is in-memory and bounded on purpose, and its own note used to
// claim a restart "loses unscrolled live messages and nothing else". It does
// not: a restart empties the buffer, and a conversation's Runs tab stayed full
// while its Transcript tab said "No messages on the console thread yet" — same
// conversation, same view, same moment.
//
// The first attempt at this rehydrated ONLY when the buffer was empty, which
// was the wrong shape and broke in a worse way: the moment a reader typed a
// reply, their own message made the buffer non-empty, the durable record
// stopped being served, and the history vanished again mid-conversation.
//
// So it is a MERGE, unconditionally, on every read:
//
//   - Every agent answer in `status.runs[]` is durable and belongs in the
//     thread, whether or not the buffer still holds it.
//   - Every MESSAGE those runs consumed is durable too, and belongs in the
//     thread in the order it was received. This is the half that did not exist:
//     a conversation recorded its answers and not its questions, so the console
//     watched the input queue and captured what people typed before pruning
//     deleted it. That workaround is gone, and so is the reason for it.
//   - Everything the buffer holds that is NOT durable — manager notices and
//     acks — belongs too, and only the buffer has it.
//   - A message present in both must appear ONCE, matched by the INPUT it
//     stands for rather than by comparing text.
//
// WHAT STILL CANNOT COME BACK after a restart: acks and notices. Nothing
// records them, because a conversation's state does not depend on them.

// mergeTranscript returns the thread as it should be read: the live buffer plus
// every durable message the buffer no longer holds, in time order.
//
// ownChannel is this console's Channel, which is how a recorded message is
// attributed: typed HERE, typed on another surface, or an event no surface
// displayed at all.
func mergeTranscript(thread, ownChannel string, live []Message, runs []Run) []Message {
	// A MULTISET of what the buffer already shows as agent output. A count,
	// not a set: an agent that answered the same thing twice produced two runs
	// and two messages, and collapsing them would silently drop one.
	liveAgentText := map[string]int{}
	// The inputs the buffer already stands for, by id. An id, not the text:
	// a bubble typed here, its delivered copy and the record entry are ONE
	// message wearing three ids, and text matching is what made that guesswork.
	liveInputs := map[string]bool{}
	for _, m := range live {
		if m.Kind == MsgAgent {
			liveAgentText[m.Text]++
		}
		if m.recordID != "" {
			liveInputs[m.recordID] = true
		}
	}

	out := make([]Message, 0, len(live)+len(runs))
	out = append(out, live...)

	for _, r := range runs {
		for _, in := range r.Inputs {
			if in.Text == "" || liveInputs[in.ID] {
				continue
			}
			out = append(out, recordedMessage(thread, ownChannel, in))
		}
		if r.Result == "" {
			continue // a run with no result said nothing; an empty line would be a lie
		}
		if liveAgentText[r.Result] > 0 {
			liveAgentText[r.Result]-- // already on screen; do not double it
			continue
		}
		at := r.FinishedAt
		if at == "" {
			at = r.StartedAt
		}
		out = append(out, Message{
			// Derived from the run id, so re-reading the same thread cannot
			// produce duplicate-looking lines with shifting ids.
			ID:     fmt.Sprintf("run:%s", r.RunID),
			Thread: thread,
			Kind:   MsgAgent,
			Text:   r.Result,
			At:     at,
		})
	}

	sortByTime(out)
	return out
}

// recordedMessage renders one recorded input as a transcript line.
//
// The speaker is named for a READER: whoever sent it when a sender is known,
// and otherwise the surface it came from. An internal kind identifier is never
// shown as somebody's name.
//
// Which bubble it is follows from WHERE it entered, the same fact the delivery
// rule reads: this console's own surface means somebody typed it here, another
// surface means somebody else's words, and no surface at all means an event —
// an alert, a job tick, a posted task — that no person typed anywhere.
func recordedMessage(thread, ownChannel string, in RecordedInput) Message {
	msg := Message{
		ID: "input:" + in.ID, Thread: thread, Text: in.Text, At: in.ReceivedAt,
		recordID: in.ID,
	}
	if in.Truncated {
		msg.Text += "\n\n_…truncated — the full payload is not kept in the conversation record._"
	}
	switch {
	case in.Surface == "":
		msg.Kind = MsgSignal
	case ownChannel != "" && in.Surface == ownChannel:
		msg.Kind, msg.Sender = MsgLocal, in.Sender
	default:
		msg.Kind = MsgRelay
		msg.Sender = in.Surface
		if in.Sender != "" {
			msg.Sender = in.Surface + "/" + in.Sender
		}
	}
	return msg
}

// sortByTime orders a thread oldest-first.
//
// STABLE, and unparseable timestamps keep their relative position rather than
// being shuffled to one end: a message with a missing time is still a message,
// and moving it would be a second bug wearing the first one's clothes.
func sortByTime(msgs []Message) {
	sort.SliceStable(msgs, func(i, j int) bool {
		ti, oki := parseAt(msgs[i].At)
		tj, okj := parseAt(msgs[j].At)
		if !oki || !okj {
			return false // treat as equal; SliceStable keeps the original order
		}
		return ti.Before(tj)
	})
}

func parseAt(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

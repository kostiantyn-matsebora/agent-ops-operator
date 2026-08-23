package main

import (
	"fmt"
	"sort"
	"strings"
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
func mergeTranscript(thread, ownChannel string, live []Message, runs []Run, conv ConversationSummary) []Message {
	// The RUNS the buffer already shows, by id.
	//
	// This was a multiset of the answer TEXT, and structured output broke it in
	// one release: the manager sends `body` flattened from the blocks it
	// parsed, while `status.runs[].result` holds the raw text the agent
	// printed. The two stopped comparing equal, so every answer rendered TWICE
	// — once from the record as text, once from the buffer as blocks.
	//
	// An id also fixes the case the multiset was built for, and better: two
	// runs that answered identically are two ids, so neither is collapsed.
	liveRuns := map[string]bool{}
	// The inputs the buffer already stands for, by id. An id, not the text:
	// a bubble typed here, its delivered copy and the record entry are ONE
	// message wearing three ids, and text matching is what made that guesswork.
	liveInputs := map[string]bool{}
	for _, m := range live {
		if m.Kind == MsgAgent && m.runID != "" {
			liveRuns[m.runID] = true
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
			out = append(out, recordedMessage(thread, ownChannel, in, conv))
		}
		if r.Result == "" {
			continue // a run with no result said nothing; an empty line would be a lie
		}
		if liveRuns[r.RunID] {
			continue // already on screen as a live op; do not double it
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
func recordedMessage(thread, ownChannel string, in RecordedInput, conv ConversationSummary) Message {
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
		// A REBUILT SIGNAL IS STILL A CARD.
		//
		// The live path composes one from the op's structured fields; the record
		// keeps only the payload text, so a reopened conversation showed a raw
		// JSON document as prose. What the CR still knows — the title and the
		// source that opened it — rebuilds the head, and the payload moves into
		// the field the browser already folds.
		msg.Text, msg.Payload = recordedSignalCard(conv), signalPayload(in)
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

// recordedSignalCard rebuilds an event card from what the CONVERSATION records.
//
// It is the same card the live path composes, because the same facts are now on
// the CR: the source and the labels used to live only on things built to be
// pruned — `spec.inputs[].origin` and the ConversationInput — so a reopened
// conversation lost its source and its whole label table.
func recordedSignalCard(conv ConversationSummary) string {
	var parts []string
	if conv.Title != "" {
		parts = append(parts, "📣 **"+conv.Title+"**")
	}
	var from []string
	if conv.Source != "" {
		from = append(from, "**Source** `"+conv.Source+"`")
	}
	if conv.Pipeline != "" {
		from = append(from, "**Pipeline** `"+conv.Pipeline+"`")
	}
	if len(from) > 0 {
		parts = append(parts, strings.Join(from, " · "))
	}
	// LABELS AS A TABLE, dropping any the card already states — the same rule
	// the live card uses, so the two do not diverge in what they show.
	shown := map[string]bool{conv.Source: true, conv.Pipeline: true}
	for _, w := range strings.Fields(strings.ReplaceAll(conv.Title, ":", " ")) {
		shown[w] = true
	}
	if rows := labelRows(&OpMessage{Labels: conv.SignalLabels}, shown); len(rows) > 0 {
		parts = append(parts, "| label | value |\n|---|---|\n"+strings.Join(rows, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

// signalPayload is the recorded event document, carried apart from the card so
// the browser can collapse it — the same shape the live path produces.
func signalPayload(in RecordedInput) string {
	body := strings.TrimSpace(in.Text)
	if in.Truncated {
		body += "\n\n…truncated — the full payload is not kept in the conversation record."
	}
	return body
}

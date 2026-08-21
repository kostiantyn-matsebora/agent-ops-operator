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
//   - Everything the buffer holds that is NOT durable — the signal card, relays
//     from sibling channels, manager notices, a just-typed local message —
//     belongs too, and only the buffer has it.
//   - An answer present in both must appear ONCE.
//
// WHAT STILL CANNOT COME BACK after a restart: the signal card and relayed
// sibling messages. Neither was ever CR state — the signal's input is pruned
// once processed and its payload object garbage collected. Reconstructing them
// would mean inventing text nobody said, so a restarted thread starts at the
// first answer rather than lying about how it began.

// mergeTranscript returns the thread as it should be read: the live buffer plus
// any durable answer the buffer no longer holds, in time order.
func mergeTranscript(thread string, live []Message, runs []Run) []Message {
	// A MULTISET of what the buffer already shows as agent output. A count,
	// not a set: an agent that answered the same thing twice produced two runs
	// and two messages, and collapsing them would silently drop one.
	liveAgentText := map[string]int{}
	for _, m := range live {
		if m.Kind == MsgAgent {
			liveAgentText[m.Text]++
		}
	}

	out := make([]Message, 0, len(live)+len(runs))
	out = append(out, live...)

	for _, r := range runs {
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

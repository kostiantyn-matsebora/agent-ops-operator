package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"
)

// Rendering what the PERSON typed.
//
// The manager posts an input to a conversation's bound threads only when the
// person has not already seen it — a signal card for an alert, and nothing for
// a message somebody typed, because posting that back would be an echo on the
// surface it was typed on.
//
// That rule is right for a transport and WRONG for a viewer, and the difference
// is what this file exists for. A Telegram user's own message is already in
// their thread, put there by Telegram. A console user's is not: the console
// renders a thread from what it was SENT, so an input nobody sends it is an
// input nobody can read. The symptom was a conversation started from the
// composer whose transcript began at the agent's answer, with the question that
// caused it missing.
//
// The `Send` path already covers a message typed INTO an open conversation
// (`AppendLocal`). This covers the other two: the message that STARTED the
// conversation, and anything typed on another surface that reached this one as
// an input rather than as a relay.
//
// The set is read off the manager's own rule rather than guessed: an input the
// manager will not post to channels BECAUSE THE PERSON TYPED IT is exactly the
// input the console must render itself. That is `origin.kind = channel`, and
// `origin.kind = signal` with `signalKind = chat`.
//
// An input with NO origin predates provenance and is skipped, for the same
// reason the manager skips it: it cannot be told from an alert, and inventing
// the wrong bubble is worse than the missing one.
//
// Bounded and in-memory like the rest of the live buffer. A console restart
// still loses what was never CR state — and an input IS CR state only until it
// is processed, so this captures it while it exists rather than reconstructing
// it afterwards.

// inputSpec is the slice of a Conversation's spec this needs. Decoded from the
// cached raw spec, so the watch pays for it once.
type inputSpec struct {
	Inputs []struct {
		ID      string `json:"id"`
		Type    string `json:"type"`
		Payload string `json:"payload,omitempty"`
		Origin  *struct {
			Kind       string `json:"kind"`
			Name       string `json:"name,omitempty"`
			SignalKind string `json:"signalKind,omitempty"`
		} `json:"origin,omitempty"`
		ReceivedAt string `json:"receivedAt,omitempty"`
	} `json:"inputs,omitempty"`
}

// typedByAPerson reports whether this input is one the manager deliberately
// does not post to bound threads because somebody typed it.
func typedByAPerson(kind, signalKind string) bool {
	switch kind {
	case "channel":
		return true
	case "signal":
		return signalKind == "chat"
	default:
		return false
	}
}

// senderHintTTL bounds how long an origination is remembered. It only has to
// outlive the round trip from "the console posted a task" to "the manager
// created a conversation carrying it", which is seconds.
const senderHintTTL = 5 * time.Minute

// maxSenderHints bounds the map, because nothing else does: every origination
// adds one and only a matching input removes it.
const maxSenderHints = 256

// TypedInputs watches conversations and records the messages people typed into
// the transcript buffer.
type TypedInputs struct {
	cache       *Cache
	transcripts *Transcripts
	adapter     *Adapter

	mu    sync.Mutex
	hints map[string]senderHint
}

// senderHint remembers WHO started a conversation from this console.
//
// The input a task becomes carries no sender — the manager records provenance
// (which source, which lane), not authorship — so without this the opening
// message of a conversation you started would read "user" while your reply two
// lines below it reads your address. One thread, two names for one person.
//
// Keyed on the task text, which is exact: the payload the manager stores is the
// string this console posted. A wrong match would need two people to type the
// same words within the TTL, and it would attribute one person's message to the
// other — so the hint is CONSUMED on use and a miss simply falls back.
type senderHint struct {
	identity string
	// original is the text the person actually typed, which is NOT what the
	// manager stores. An addressed task (`/ha-control turn the AC on`) reaches
	// the conversation as the REST — the manager consumed the address deciding
	// who answers — so rendering the stored payload shows a message with its
	// first words missing. The console posted the whole thing and is the only
	// component that still has it.
	original string
	at       time.Time
}

func NewTypedInputs(cache *Cache, transcripts *Transcripts, adapter *Adapter) *TypedInputs {
	return &TypedInputs{cache: cache, transcripts: transcripts, adapter: adapter, hints: map[string]senderHint{}}
}

// RememberOrigination records what was posted from this console, and by whom.
//
// Filed under TWO keys, because the payload that comes back is not always what
// went out: an addressed task arrives as the rest, with the address consumed.
// Both keys point at one hint, and consuming either drops both.
func (t *TypedInputs) RememberOrigination(task, identity string) {
	if task == "" {
		return
	}
	h := senderHint{identity: identity, original: task, at: time.Now()}
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, old := range t.hints {
		if h.at.Sub(old.at) > senderHintTTL {
			delete(t.hints, k)
		}
	}
	if len(t.hints) >= maxSenderHints {
		return
	}
	t.hints[task] = h
	if rest := addressedRest(task); rest != "" && rest != task {
		t.hints[rest] = h
	}
}

// addressedRest returns what the manager will store for an addressed task: the
// text after `/<pipeline>`. Empty when the task addresses nobody.
//
// A deliberate SUBSET of the manager's parser — this only has to recognise the
// shape well enough to file a second key, and a miss costs a message rendered
// as the manager stored it rather than a wrong one.
func addressedRest(task string) string {
	if !strings.HasPrefix(task, "/") {
		return ""
	}
	i := strings.IndexAny(task, " \t")
	if i < 0 {
		return ""
	}
	return strings.TrimSpace(task[i+1:])
}

// hintFor consumes the hint for this payload, if one is live. It returns the
// identity and the text as typed.
func (t *TypedInputs) hintFor(payload string) (identity, original string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	h, ok := t.hints[payload]
	if !ok || time.Since(h.at) > senderHintTTL {
		return "", ""
	}
	delete(t.hints, payload)
	delete(t.hints, h.original)
	if rest := addressedRest(h.original); rest != "" {
		delete(t.hints, rest)
	}
	return h.identity, h.original
}

// Run consumes cache deltas until the context ends.
//
// It reads the CURRENT store on resync as well as individual deltas, because a
// resync replaces a kind wholesale and its delta carries no object.
func (t *TypedInputs) Run(ctx context.Context) {
	deltas, cancel := t.cache.Subscribe()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deltas:
			if !ok {
				return
			}
			if d.Kind != "conversations" {
				continue
			}
			switch {
			case d.Object != nil:
				t.record(d.Object)
			case d.Type == DeltaResync:
				for _, obj := range t.cache.List("conversations") {
					t.record(obj)
				}
			}
		}
	}
}

// record appends every not-yet-seen typed input of one conversation.
func (t *TypedInputs) record(obj *Object) {
	if len(obj.Spec) == 0 {
		return
	}
	var spec inputSpec
	if err := json.Unmarshal(obj.Spec, &spec); err != nil {
		log.Printf("typed inputs: decoding %s: %v", obj.Metadata.Name, err)
		return
	}
	// The UID from the OBJECT, not from a cache lookup: this delta may be the
	// first sighting, and a lookup that missed would key the message to a thread
	// nothing reads.
	thread := "console-" + obj.Metadata.UID
	if obj.Metadata.UID == "" {
		thread = t.adapter.threadID(obj.Metadata.Name)
	}
	for _, in := range spec.Inputs {
		if in.Payload == "" || in.Origin == nil {
			continue
		}
		if !typedByAPerson(in.Origin.Kind, in.Origin.SignalKind) {
			continue
		}
		// Keyed on the INPUT id, which is stable for the life of the input, so
		// the many watch events one conversation produces render it once.
		// The text as TYPED when this console posted it, so an addressed task
		// reads as the person wrote it rather than starting mid-sentence.
		sender, text := t.hintFor(in.Payload)
		if text == "" {
			text = in.Payload
		}
		t.transcripts.AppendTyped("input:"+obj.Metadata.Name+":"+in.ID, thread, sender, text, in.ReceivedAt)
	}
}

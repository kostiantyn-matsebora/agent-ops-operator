package main

import (
	"strings"
	"sync"
	"time"
)

// The live wire: what the manager posts to the console's threads, kept in
// memory and streamed to browsers.
//
// Deliberately ephemeral and bounded (design decision 5). The durable record
// of a conversation is `status.runs[]`, which the console already has from its
// watch; this buffer only carries the part of the exchange that has not
// finished becoming CR state yet. A restart loses unscrolled live messages and
// nothing else.

const (
	// maxThreadMessages bounds one thread's live buffer.
	maxThreadMessages = 200
	// maxThreads bounds how many threads are retained (oldest touched first).
	maxThreads = 200
	// pendingTTL gives a locally-sent message this long to be confirmed by an
	// incoming op before it is shown as unconfirmed.
	pendingTTL = 2 * time.Minute
)

// Message kinds. The manager sends TEXT, not semantics — these are the
// console's best reading of it, not a contract. (The pending
// `adapter-rendered-messages` change replaces transport-rendered text with
// semantic messages; when it lands, classification here becomes a field read
// instead of a prefix match.)
const (
	// MsgAgent: agent output or an operator notice.
	MsgAgent = "agent"
	// MsgAck: the router's busy/acknowledged notice.
	MsgAck = "ack"
	// MsgRelay: a user message from a SIBLING channel, attributed to its sender.
	MsgRelay = "relay"
	// MsgLocal: typed in this console, awaiting confirmation.
	MsgLocal = "local"
)

// Message is one line of a thread transcript.
type Message struct {
	ID     string `json:"id"`
	Thread string `json:"thread"`
	Kind   string `json:"kind"`
	// Sender is set for relayed sibling-channel messages ("<channel>/<user>").
	Sender string `json:"sender,omitempty"`
	Text   string `json:"text"`
	At     string `json:"at"`
	// Pending marks a locally-typed message the manager has not confirmed yet.
	Pending bool `json:"pending,omitempty"`
}

// Transcripts holds every live thread buffer.
type Transcripts struct {
	mu       sync.Mutex
	threads  map[string]*threadLog
	order    []string // touch order for eviction
	seen     map[string]bool
	seenRing []string

	subMu  sync.Mutex
	subs   map[int]chan Message
	nextID int
}

type threadLog struct {
	messages []Message
}

// NewTranscripts builds an empty store.
func NewTranscripts() *Transcripts {
	return &Transcripts{
		threads: map[string]*threadLog{},
		seen:    map[string]bool{},
		subs:    map[int]chan Message{},
	}
}

// AppendOp records one incoming `send` op. Returns false when the op id was
// already recorded — channel ops are at-least-once, so a redelivered op must
// render exactly once.
func (t *Transcripts) AppendOp(opID, thread, text string) bool {
	t.mu.Lock()
	if t.seen[opID] {
		t.mu.Unlock()
		return false
	}
	t.seen[opID] = true
	t.seenRing = append(t.seenRing, opID)
	if len(t.seenRing) > 4096 {
		delete(t.seen, t.seenRing[0])
		t.seenRing = t.seenRing[1:]
	}
	t.mu.Unlock()

	kind, sender, body := classify(text)
	msg := Message{ID: opID, Thread: thread, Kind: kind, Sender: sender, Text: body, At: nowRFC3339()}
	t.append(msg, kind == MsgAck || kind == MsgRelay)
	return true
}

// AppendLocal records a message typed in this console as pending. It is NEVER
// posted back inbound by anything reading this buffer — the send path is the
// only writer to /channel/inbound, and it is called once, from the browser
// handler.
func (t *Transcripts) AppendLocal(id, thread, sender, text string) Message {
	msg := Message{ID: id, Thread: thread, Kind: MsgLocal, Sender: sender, Text: text, At: nowRFC3339(), Pending: true}
	t.append(msg, false)
	return msg
}

// append stores and publishes a message; confirmPending clears the thread's
// outstanding local messages (an ack or a relay coming back means the manager
// took them).
func (t *Transcripts) append(msg Message, confirmPending bool) {
	t.mu.Lock()
	log := t.threads[msg.Thread]
	if log == nil {
		log = &threadLog{}
		t.threads[msg.Thread] = log
		t.order = append(t.order, msg.Thread)
		for len(t.order) > maxThreads {
			delete(t.threads, t.order[0])
			t.order = t.order[1:]
		}
	}
	var confirmed []Message
	if confirmPending {
		for i := range log.messages {
			if log.messages[i].Pending {
				log.messages[i].Pending = false
				confirmed = append(confirmed, log.messages[i])
			}
		}
	}
	log.messages = append(log.messages, msg)
	if len(log.messages) > maxThreadMessages {
		log.messages = log.messages[len(log.messages)-maxThreadMessages:]
	}
	t.mu.Unlock()

	for _, c := range confirmed {
		t.publish(c)
	}
	t.publish(msg)
}

// Thread returns a copy of one thread's buffer, marking long-unconfirmed local
// messages so the UI can say so rather than spinning forever.
func (t *Transcripts) Thread(thread string) []Message {
	t.mu.Lock()
	defer t.mu.Unlock()
	log := t.threads[thread]
	if log == nil {
		return nil
	}
	out := make([]Message, len(log.messages))
	copy(out, log.messages)
	for i := range out {
		if out[i].Pending && olderThan(out[i].At, pendingTTL) {
			out[i].Pending = false
			out[i].Kind = MsgLocal
		}
	}
	return out
}

// Subscribe streams appended messages.
func (t *Transcripts) Subscribe() (<-chan Message, func()) {
	t.subMu.Lock()
	defer t.subMu.Unlock()
	id := t.nextID
	t.nextID++
	ch := make(chan Message, 128)
	t.subs[id] = ch
	return ch, func() {
		t.subMu.Lock()
		defer t.subMu.Unlock()
		if cur, ok := t.subs[id]; ok {
			delete(t.subs, id)
			close(cur)
		}
	}
}

func (t *Transcripts) publish(msg Message) {
	t.subMu.Lock()
	defer t.subMu.Unlock()
	for _, ch := range t.subs {
		select {
		case ch <- msg:
		default: // slow browser: the transcript is an overlay, dropping is safe
		}
	}
}

// relayPrefix is how internal/chat/router.go attributes a sibling-channel
// message: "💬 <b>channel/sender</b>: text".
const relayPrefix = "💬 <b>"

// ackMarkers are the router's fire-and-forget notices.
var ackMarkers = []string{"🔧 ", "⏳ ", "⚠️ ", "🤖 "}

// classify reads a send op's text into (kind, sender, body).
//
// A prefix match is a poor substitute for a typed message, and it is what the
// contract offers today: `send` carries rendered text only. Getting it wrong
// only mislabels a bubble — never drops or duplicates one — so the console
// guesses and stays honest about it in the UI.
func classify(text string) (kind, sender, body string) {
	if rest, ok := strings.CutPrefix(text, relayPrefix); ok {
		if who, msg, found := strings.Cut(rest, "</b>: "); found {
			return MsgRelay, who, msg
		}
	}
	for _, m := range ackMarkers {
		if strings.HasPrefix(text, m) {
			return MsgAck, "", text
		}
	}
	return MsgAgent, "", text
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func olderThan(ts string, d time.Duration) bool {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return false
	}
	return time.Since(t) > d
}

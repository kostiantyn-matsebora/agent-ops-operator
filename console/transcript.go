package main

import (
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

// Message kinds — now a FIELD READ, not a guess. The manager sends semantic
// messages, so the console maps one enum to another instead of sniffing
// Telegram HTML prefixes off text it did not compose.
const (
	// MsgAgent: agent output (an `answer`).
	MsgAgent = "agent"
	// MsgAck: a manager notice — ack, listing, refusal.
	MsgAck = "ack"
	// MsgRelay: a user message from a SIBLING channel, attributed to its sender.
	MsgRelay = "relay"
	// MsgSignal: the event that opened or advanced the conversation. Its own
	// kind because it is neither the agent speaking nor the manager: it is what
	// the agent was woken for, and a thread reads as event-then-work.
	MsgSignal = "signal"
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
	// archived: the conversation was closed and its thread ended. The buffer is
	// kept — a closed conversation's last exchange is exactly what someone
	// wants to read — but the UI stops offering a reply box for it.
	archived bool
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
func (t *Transcripts) AppendOp(opID, thread string, m *OpMessage) bool {
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

	kind, sender := transcriptKind(m)
	msg := Message{ID: opID, Thread: thread, Kind: kind, Sender: sender, Text: m.Render(), At: nowRFC3339()}
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

// Archive marks a thread closed, keeping its transcript for this console
// session. Idempotent: close-topic ops are at-least-once.
func (t *Transcripts) Archive(thread string) {
	t.mu.Lock()
	log := t.threads[thread]
	if log == nil {
		log = &threadLog{}
		t.threads[thread] = log
		t.order = append(t.order, thread)
	}
	already := log.archived
	log.archived = true
	t.mu.Unlock()
	if already {
		return
	}
	t.append(Message{
		ID: "archived:" + thread, Thread: thread, Kind: MsgAgent,
		Text: "🗄 Conversation closed — this thread is archived.", At: nowRFC3339(),
	}, false)
}

// Reopen clears a thread's archived flag, because the conversation it belongs
// to came back.
//
// UI STATE ONLY — it posts no message. The console derives "you cannot type
// here" from this flag, so without clearing it a reopened conversation renders
// alive with no composer. The reopen ANNOUNCEMENT is the manager's: it fans a
// notice to every bound thread, which arrives here as an ordinary send and
// lands in the transcript like any other message. Saying it here as well would
// put it on this surface and no other, and would be a second author of a fact
// the manager already owns.
//
// Idempotent, and silent when the thread was not archived: ensure-topic is
// at-least-once and is also delivered for threads that were never closed.
func (t *Transcripts) Reopen(thread string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if log := t.threads[thread]; log != nil {
		log.archived = false
	}
}

// Archived reports whether a thread has been closed.
func (t *Transcripts) Archived(thread string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	log := t.threads[thread]
	return log != nil && log.archived
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

// transcriptKind maps a contract message kind onto a transcript bubble, and
// composes the relay attribution the UI shows in place of a kind label.
//
// This replaced a prefix match against "💬 <b>" and a list of emoji markers —
// the console reverse-engineering Telegram HTML the manager had rendered for a
// different surface. A message kind is now stated, so nothing here can be
// wrong about it.
func transcriptKind(m *OpMessage) (kind, sender string) {
	if m == nil {
		return MsgAgent, ""
	}
	switch m.Kind {
	case "relay":
		who := m.Origin
		if m.Sender != "" {
			who = m.Origin + "/" + m.Sender
		}
		return MsgRelay, who
	case "notice":
		return MsgAck, ""
	case "signal":
		return MsgSignal, ""
	default:
		return MsgAgent, ""
	}
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func olderThan(ts string, d time.Duration) bool {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return false
	}
	return time.Since(t) > d
}

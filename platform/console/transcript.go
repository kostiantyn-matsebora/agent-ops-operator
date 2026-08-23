package main

import (
	"strings"
	"sync"
	"time"
)

// The live wire: what the manager posts to the console's threads, kept in
// memory and streamed to browsers.
//
// Deliberately ephemeral and bounded (design decision 5). The durable record of
// a conversation is `status.runs[]` — the answers AND the messages they
// answered — which the console already has from its watch.
//
// This used to claim a restart "loses unscrolled live messages and nothing
// else". It does not, and the gap was visible: after a restart a conversation's
// Runs tab was full while its Transcript tab said no messages — the same
// conversation, the same view, the same moment. The premise was right and the
// second half was never built. `mergeTranscript` is the missing half: every
// read merges this buffer with `status.runs[]`, so a thread shows its full
// history whether or not the buffer still holds it.
//
// It MERGES rather than falling back, and that distinction is the substance of
// the fix. A first attempt served the durable record only when the buffer was
// EMPTY, which broke worse than the bug it replaced: typing a reply made the
// buffer non-empty, so the history vanished mid-conversation.
//
// What a restart genuinely loses is what was never CR state at all: acks and
// other manager notices, which nothing records because nothing has to. Every
// MESSAGE — the event that opened the thread, what people typed here and
// elsewhere, what the agent answered — is read back from the record.

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

	// Choices are the actions this message OFFERS. Kept STRUCTURED all the way
	// to the browser: the console is the surface that can render them as
	// controls, and flattening them into the text here would throw that away.
	//
	// An adapter that cannot render controls prints the same list, which the
	// manager's own prose already carries — so these are additive, never the
	// only account of what is on offer.
	Choices []OpChoice `json:"choices,omitempty"`

	// Payload is a SIGNAL's raw event document, carried apart from Text so the
	// browser can put it behind a disclosure control.
	//
	// It is the tallest thing in an event card and the least often read — the
	// same argument `<details>` makes for an agent's answer, one message kind
	// over, and what the Telegram adapter does with an expandable quote. Left
	// inside Text it is a wall of JSON between the card and the reply box.
	Payload string `json:"payload,omitempty"`

	// runID is the RUN this bubble reports, when it reports one. Internal
	// correlation only, exactly like recordID — and the same lesson: a bubble
	// and the durable run behind it are ONE message wearing two ids, so the
	// merge matches them by ID.
	//
	// IT USED TO MATCH ON TEXT, and structured output broke that instantly. The
	// manager now sends `body` FLATTENED from the blocks it parsed, so the
	// buffer holds the flattened form while `status.runs[].result` holds the raw
	// text the agent printed. They no longer compare equal, dedup failed, and
	// every agent answer rendered TWICE in the transcript — once as text, once
	// as blocks.
	runID string

	// recordID is the conversation input this bubble stands for, when it stands
	// for one. Internal correlation only — never rendered, never sent to the
	// browser — and it is what lets a read MERGE the buffer with the durable
	// record without comparing text: a message already on screen is the same
	// message as the record entry naming the same input.
	recordID string
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
//
// ownChannel is this console's Channel, and it is what tells one of THIS
// surface's own users' messages from somebody else's: the manager delivers a
// message to every bound channel except the surface that displayed it, and a
// viewer displays nothing it was not sent — so a message typed here comes back
// here, as a relay whose origin is this channel. It CONFIRMS the bubble already
// on screen rather than becoming a second one.
func (t *Transcripts) AppendOp(opID, thread string, m *OpMessage, ownChannel string) bool {
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

	kind, sender := transcriptKind(m, ownChannel)
	text := m.Render()
	record := inputIDOf(opID)
	if kind == MsgLocal {
		// Rendered plain: the relay prefix names a speaker for somebody else's
		// words, and these are the words of the person reading this thread.
		text = m.Body
		if confirmed, ok := t.confirmLocal(thread, sender, record); ok {
			if confirmed != nil {
				t.publish(*confirmed)
			}
			return true
		}
	}
	msg := Message{ID: opID, Thread: thread, Kind: kind, Sender: sender, Text: text,
		At: nowRFC3339(), Choices: m.Choices, Payload: m.SignalPayload(),
		recordID: record, runID: runIDOf(opID)}
	t.append(msg, kind == MsgAck || kind == MsgRelay)
	return true
}

// confirmLocal marks the bubble a delivered copy stands for as durable,
// returning it so callers can republish it (the UI drops its "sending…" label).
//
// Matched on ORDER and, when both are known, on the SENDER — never on the text.
// Recovering a message by comparing text to something posted earlier is the
// workaround this whole change removes, and it produced a truncated message, a
// duplicated one and a missing one in a single evening. FIFO is sound because
// the manager delivers a thread's messages in the order it took them.
func (t *Transcripts) confirmLocal(thread, sender, record string) (*Message, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	log := t.threads[thread]
	if log == nil {
		return nil, false
	}
	for i := range log.messages {
		m := &log.messages[i]
		if m.Kind != MsgLocal || m.recordID != "" {
			continue // not a local bubble, or already standing for a message
		}
		if sender != "" && m.Sender != "" && m.Sender != sender {
			continue // somebody else's message on a shared surface
		}
		m.Pending = false
		m.recordID = record
		confirmed := *m
		return &confirmed, true
	}
	return nil, false
}

// inputIDOf reads the input id out of a delivery op id
// ("input:<conversation>:<input>:<channel>"), or "" for any other op.
//
// The correlation between the buffer and the record is an ID, not a string
// comparison — which is the difference between a merge that is right and one
// that is usually right.
// runIDOf reads the run out of a reply op id (`send:<conv>:<channel>:<runId>`),
// which is what lets the merge tell a buffered answer from the durable record
// of the SAME run without comparing a single character of either.
func runIDOf(opID string) string {
	parts := strings.Split(opID, ":")
	if len(parts) != 4 || parts[0] != "send" {
		return ""
	}
	return parts[3]
}

func inputIDOf(opID string) string {
	parts := strings.Split(opID, ":")
	if len(parts) != 4 || parts[0] != "input" {
		return ""
	}
	return parts[2]
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
//
// A relay whose ORIGIN is this console's own channel is one of this surface's
// users' messages coming back, so it renders as a local message rather than as
// somebody else's words. The speaker is named for a reader: the sender when one
// is known, and never an internal kind identifier.
func transcriptKind(m *OpMessage, ownChannel string) (kind, sender string) {
	if m == nil {
		return MsgAgent, ""
	}
	switch m.Kind {
	case "relay":
		if ownChannel != "" && m.Origin == ownChannel {
			return MsgLocal, m.Sender
		}
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

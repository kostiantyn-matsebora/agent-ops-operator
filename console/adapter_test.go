package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// The console is a channel adapter first. These tests drive it against a fake
// manager implementing the /channel/* contract, so the properties that matter
// — at-least-once tolerance, deterministic thread ids, and the no-relay-loop
// rule — are pinned without a cluster.

// fakeManager serves the slice of the contract the console consumes and
// records everything it was told.
type fakeManager struct {
	mu       sync.Mutex
	ops      []Op
	next     int
	done     []map[string]string
	inbound  []map[string]any
	channels []ChannelInfo
	server   *httptest.Server
}

func newFakeManager(t *testing.T, channels ...ChannelInfo) *fakeManager {
	t.Helper()
	f := &fakeManager{channels: channels}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /channel/ops", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.next >= len(f.ops) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		op := f.ops[f.next]
		f.next++
		writeJSON(w, http.StatusOK, op)
	})
	mux.HandleFunc("POST /channel/ops/{id}/done", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body == nil {
			body = map[string]string{}
		}
		body["id"] = r.PathValue("id")
		f.mu.Lock()
		f.done = append(f.done, body)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /channel/inbound", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.inbound = append(f.inbound, body)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /channel/channels", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		writeJSON(w, http.StatusOK, f.channels)
	})
	mux.HandleFunc("POST /channel/channels/{name}/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeManager) queue(ops ...Op) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, ops...)
}

func (f *fakeManager) completions() []map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]string, len(f.done))
	copy(out, f.done)
	return out
}

func (f *fakeManager) inbounds() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]map[string]any, len(f.inbound))
	copy(out, f.inbound)
	return out
}

// consoleUnderTest wires an adapter over a pre-populated cache.
func consoleUnderTest(t *testing.T, f *fakeManager, objs ...*Object) (*Adapter, *Transcripts, *Cache) {
	t.Helper()
	cache := staticCache(objs...)
	// the static cache never lists, so mark it synced for Run()
	for _, k := range Kinds {
		cache.replace(k, cache.List(k))
	}
	tr := NewTranscripts()
	a := NewAdapter(NewManager(f.server.URL, "token"), cache, tr, "console")
	return a, tr, cache
}

func runUntil(t *testing.T, a *Adapter, cond func() bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Run(ctx)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("adapter did not reach the expected state")
}

func TestEnsureTopicIsDeterministicFromConversationUID(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	conv := obj("conversations", "task-abc", "1", `{"profileRef":{"name":"ops"}}`, "{}")
	conv.Metadata.UID = "7f3c-uid"
	a, _, _ := consoleUnderTest(t, f, conv)

	f.queue(Op{ID: "topic:task-abc:console", Channel: "console", Conversation: "task-abc", Kind: "ensure-topic", Topic: &TopicDescriptor{Conversation: "task-abc", Title: "t"}})
	runUntil(t, a, func() bool { return len(f.completions()) > 0 })

	got := f.completions()[0]
	if got["threadId"] != "console-7f3c-uid" {
		t.Fatalf("thread id not derived from the conversation UID: %+v", got)
	}
	// a fresh console (new process, same cluster) must mint the same id
	a2, _, _ := consoleUnderTest(t, f, conv)
	if id := a2.threadID("task-abc"); id != got["threadId"] {
		t.Fatalf("thread id not stable across restarts: %q vs %q", id, got["threadId"])
	}
}

func TestRedeliveredSendRendersOnce(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	a, tr, _ := consoleUnderTest(t, f)
	thread := "console-uid-1"
	send := Op{ID: "send:42", Channel: "console", Kind: "send", ThreadID: &thread, Message: &OpMessage{Kind: "answer", Body: "done"}}
	f.queue(send, send) // at-least-once: the manager may redeliver

	runUntil(t, a, func() bool { return len(f.completions()) == 2 })
	if msgs := tr.Thread(thread); len(msgs) != 1 {
		t.Fatalf("redelivered op must render once, got %d: %+v", len(msgs), msgs)
	}
}

func TestRelayIsAttributedAndAcksAreDistinct(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	a, tr, _ := consoleUnderTest(t, f)
	thread := "console-uid-1"
	f.queue(
		Op{ID: "s1", Channel: "console", Kind: "send", ThreadID: &thread,
			Message: &OpMessage{Kind: "relay", Origin: "telegram", Sender: "kim", Body: "look at this"}},
		Op{ID: "s2", Channel: "console", Kind: "send", ThreadID: &thread, Message: &OpMessage{Kind: "notice", Body: "🔧 On it…"}},
		Op{ID: "s3", Channel: "console", Kind: "send", ThreadID: &thread, Message: &OpMessage{Kind: "answer", Body: "restarted the deployment"}},
	)
	runUntil(t, a, func() bool { return len(tr.Thread(thread)) == 3 })

	msgs := tr.Thread(thread)
	// The kind and the sender are FIELD READS now, not a prefix match against
	// Telegram HTML. The text keeps the attribution too, because the console
	// renders one string per bubble and the SPA shows it verbatim.
	if msgs[0].Kind != MsgRelay || msgs[0].Sender != "telegram/kim" ||
		!strings.Contains(msgs[0].Text, "look at this") {
		t.Fatalf("relay not attributed: %+v", msgs[0])
	}
	if msgs[1].Kind != MsgAck {
		t.Fatalf("ack not classified: %+v", msgs[1])
	}
	if msgs[2].Kind != MsgAgent {
		t.Fatalf("agent output should not be an ack: %+v", msgs[2])
	}
}

// The rule the console exists to not break: an op it RECEIVED is never posted
// back inbound. Only a human typing in the browser reaches /channel/inbound.
func TestNoRelayLoop(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	conv := obj("conversations", "conv", "1", `{"profileRef":{"name":"ops"}}`,
		`{"threads":[{"channel":"console","threadId":"console-uid-1"}]}`)
	a, tr, _ := consoleUnderTest(t, f, conv)
	thread := "console-uid-1"

	// the manager fans a console user's own message back as a relay
	f.queue(Op{ID: "s1", Channel: "console", Kind: "send", ThreadID: &thread,
		Message: &OpMessage{Kind: "relay", Origin: "console", Sender: "kim", Body: "restart it"}})
	runUntil(t, a, func() bool { return len(tr.Thread(thread)) == 1 })

	if got := f.inbounds(); len(got) != 0 {
		t.Fatalf("a received op was re-posted inbound: %+v", got)
	}
}

func TestSendMarksPendingThenConfirms(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	conv := obj("conversations", "conv", "1", `{"profileRef":{"name":"ops"}}`,
		`{"threads":[{"channel":"console","threadId":"console-uid-1"}]}`)
	a, tr, _ := consoleUnderTest(t, f, conv)
	// serve the channel list so PrimaryChannel resolves
	a.refreshChannels(context.Background())

	if _, err := a.Send(context.Background(), "conv", "kim", "restart it"); err != nil {
		t.Fatal(err)
	}
	msgs := tr.Thread("console-uid-1")
	if len(msgs) != 1 || !msgs[0].Pending || msgs[0].Kind != MsgLocal {
		t.Fatalf("local message should be pending: %+v", msgs)
	}
	in := f.inbounds()
	if len(in) != 1 || in[0]["threadId"] != "console-uid-1" || in[0]["text"] != "restart it" || in[0]["sender"] != "kim" {
		t.Fatalf("inbound payload wrong: %+v", in)
	}

	// the manager's ack confirms it
	thread := "console-uid-1"
	f.queue(Op{ID: "ack1", Channel: "console", Kind: "send", ThreadID: &thread, Message: &OpMessage{Kind: "notice", Body: "🔧 On it…"}})
	runUntil(t, a, func() bool { return len(tr.Thread(thread)) == 2 })
	if tr.Thread(thread)[0].Pending {
		t.Fatal("ack must confirm the pending message")
	}
}

func TestSendRefusedForObservedConversation(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	// bound to telegram only: no console thread, so nothing to reply into
	conv := obj("conversations", "conv", "1", `{"profileRef":{"name":"ops"}}`,
		`{"threads":[{"channel":"telegram","threadId":"55"}]}`)
	a, _, _ := consoleUnderTest(t, f, conv)
	a.refreshChannels(context.Background())

	if _, err := a.Send(context.Background(), "conv", "kim", "hello"); err != errNotJoined {
		t.Fatalf("observed conversations must refuse sends, got %v", err)
	}
	if got := f.inbounds(); len(got) != 0 {
		t.Fatalf("nothing should have been posted: %+v", got)
	}
}

// The UI token is a per-channel credential resolved from the projected env,
// never read from a Secret through the API.
func TestUITokenComesFromProjectedChannelCredentials(t *testing.T) {
	prefix := "AGENTOPS_CRED_CONSOLE_"
	t.Setenv(prefix+"uiToken", "s3cret")
	f := newFakeManager(t, ChannelInfo{Name: "console", CredentialEnvPrefix: prefix})
	a, _, _ := consoleUnderTest(t, f)
	a.refreshChannels(context.Background())
	if got := a.UITokenFromChannel(); got != "s3cret" {
		t.Fatalf("uiToken not resolved from projected credentials: %q", got)
	}
}

func TestChannelNoticeWithoutThreadIsStillVisible(t *testing.T) {
	f := newFakeManager(t, ChannelInfo{Name: "console"})
	a, tr, _ := consoleUnderTest(t, f)
	f.queue(Op{ID: "n1", Channel: "console", Kind: "send", Message: &OpMessage{Kind: "notice", Body: "🤖 **Agents**: /ops"}})
	runUntil(t, a, func() bool { return len(tr.Thread("channel:console")) == 1 })
	if msg := tr.Thread("channel:console")[0]; !strings.Contains(msg.Text, "Agents") {
		t.Fatalf("channel-level notice lost: %+v", msg)
	}
}

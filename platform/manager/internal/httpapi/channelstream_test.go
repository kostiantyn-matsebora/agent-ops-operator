package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentopsv1alpha1 "github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/api/v1alpha1"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/chat"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/commandbus"
	"github.com/kostiantyn-matsebora/agent-ops-operator/platform/manager/internal/state"
)

const streamAdapter = "console"

func streamChannel() *agentopsv1alpha1.Channel {
	ch := &agentopsv1alpha1.Channel{}
	ch.Name = "console-ui"
	ch.Spec.Adapter = streamAdapter
	return ch
}

// enqueueStreamOps puts n ops on the console channel, each on its own
// conversation so none is serialized behind another's acknowledgement, and
// returns their ids in enqueue order.
func enqueueStreamOps(t *testing.T, q *chat.OpQueue, prefix string, n int) []string {
	t.Helper()
	var ids []string
	for i := 0; i < n; i++ {
		conv := &state.ConversationRecord{Name: fmt.Sprintf("%s-%d", prefix, i)}
		id := chat.InputOpID(conv.Name, "in", "console-ui")
		thread := "thread-" + conv.Name
		q.EnqueueInputDelivery(context.Background(), streamChannel(), conv, "in", &thread, chat.Notice(id))
		ids = append(ids, id)
	}
	return ids
}

// secondReplica is a Server over the SAME bus as s and no memory of s's claims:
// what a reconnect landing on another manager replica meets.
func secondReplica(s *Server, bus commandbus.Bus) *Server {
	q := &chat.OpQueue{
		Registry: chat.NewRegistry(), Bus: bus, FeedAdapters: []string{streamAdapter},
		Conversations: s.Conversations, Runs: s.Runs,
	}
	return &Server{
		Reader: s.Reader, Client: s.Client, Conversations: s.Conversations, Runs: s.Runs, Blobs: s.Blobs,
		Namespace: s.Namespace, AdapterToken: s.AdapterToken, Ops: q, Router: s.Router, Bus: bus,
	}
}

type sseEvent struct {
	event string
	id    string
	data  string
}

// streamClient is one open GET /channel/ops/stream.
type streamClient struct {
	t      *testing.T
	cancel context.CancelFunc
	events chan sseEvent
	status int
}

func openStream(t *testing.T, base, query, lastEventID string) *streamClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/channel/ops/stream?"+query, nil)
	req.Header.Set("Authorization", "Bearer bus-test-master")
	if lastEventID != "" {
		req.Header.Set(lastEventIDHeader, lastEventID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("open stream: %v", err)
	}
	c := &streamClient{t: t, cancel: cancel, events: make(chan sseEvent, 64), status: resp.StatusCode}
	t.Cleanup(cancel)
	if resp.StatusCode != 200 {
		resp.Body.Close()
		close(c.events)
		return c
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}
	if resp.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("X-Accel-Buffering must be no so a proxy does not hold frames back")
	}
	go func() {
		defer resp.Body.Close()
		defer close(c.events)
		sc := bufio.NewScanner(resp.Body)
		var cur sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if cur.event != "" {
					c.events <- cur
				}
				cur = sseEvent{}
			case strings.HasPrefix(line, "event: "):
				cur.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "id: "):
				cur.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				cur.data = strings.TrimPrefix(line, "data: ")
			}
		}
	}()
	return c
}

// next returns the next `op` event, failing on a timeout.
func (c *streamClient) next() (sseEvent, chat.Op) {
	c.t.Helper()
	select {
	case ev, ok := <-c.events:
		if !ok {
			c.t.Fatal("stream ended while an op was expected")
		}
		var op chat.Op
		if err := json.Unmarshal([]byte(ev.data), &op); err != nil {
			c.t.Fatalf("op frame data: %v", err)
		}
		return ev, op
	case <-time.After(5 * time.Second):
		c.t.Fatal("no op frame within 5s")
	}
	return sseEvent{}, chat.Op{}
}

// expectQuiet fails if any frame arrives within d.
func (c *streamClient) expectQuiet(d time.Duration) {
	c.t.Helper()
	select {
	case ev, ok := <-c.events:
		if ok {
			c.t.Fatalf("unexpected frame %+v", ev)
		}
	case <-time.After(d):
	}
}

// feedServer is a Server whose console adapter reads a retained feed.
func feedServer(t *testing.T, objs ...client.Object) (*Server, commandbus.Bus) {
	t.Helper()
	s, _, bus := busServer(t, objs...)
	s.Ops.FeedAdapters = []string{streamAdapter}
	return s, bus
}

func streamQuery() string { return "adapter=" + streamAdapter + "&contract=" + chat.ContractVersion }

func TestChannelOpsStreamDeliversInLogOrderWithTheCommandIDAsEventID(t *testing.T) {
	s, _ := feedServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "order", 3)

	c := openStream(t, srv.URL, streamQuery(), "")
	for i, want := range ids {
		ev, op := c.next()
		if ev.event != "op" || op.ID != want {
			t.Fatalf("event %d = %q %q, want op %q", i, ev.event, op.ID, want)
		}
		if ev.id != want {
			t.Fatalf("event %d id = %q, want the op's command id %q", i, ev.id, want)
		}
		if op.Message == nil || op.Channel != "console-ui" {
			t.Fatalf("the body is the op: %+v", op)
		}
	}
	// An op produced after connect is pushed without a reconnect.
	late := enqueueStreamOps(t, s.Ops, "late", 1)
	if _, op := c.next(); op.ID != late[0] {
		t.Fatalf("late op = %q, want %q", op.ID, late[0])
	}
}

// A reconnect names the last event it RECEIVED, with no acknowledgement ever
// sent: it gets every later entry, in order, and the ones produced while away.
func TestChannelOpsStreamReconnectReplaysAfterTheLastReceivedID(t *testing.T) {
	s, _ := feedServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "resume", 4)

	c := openStream(t, srv.URL, streamQuery(), "")
	for range ids[:2] {
		c.next()
	}
	c.cancel() // drops after ids[1]
	late := enqueueStreamOps(t, s.Ops, "away", 1)

	r := openStream(t, srv.URL, streamQuery(), ids[1])
	for _, want := range []string{ids[2], ids[3], late[0]} {
		if _, op := r.next(); op.ID != want {
			t.Fatalf("replayed %q, want %q", op.ID, want)
		}
	}
	r.expectQuiet(300 * time.Millisecond)
	r.cancel()

	// ?since= wins over the header.
	q := openStream(t, srv.URL, streamQuery()+"&since="+url.QueryEscape(ids[2]), ids[0])
	for _, want := range []string{ids[3], late[0]} {
		if _, op := q.next(); op.ID != want {
			t.Fatalf("since=%s replayed %q, want %q", ids[2], op.ID, want)
		}
	}
}

// An id the log does not hold is a cursor from a log that has since trimmed it,
// or a forgery: the stream starts from the oldest retained entry. Nothing was
// removed by the delivery before it, so the whole log is there.
func TestChannelOpsStreamUnknownCursorReplaysTheRetainedLog(t *testing.T) {
	s, _ := feedServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "unknown", 3)

	first := openStream(t, srv.URL, streamQuery(), "")
	for range ids {
		first.next()
	}
	first.cancel()
	for _, cursor := range []string{"an-id-no-op-ever-had", ""} {
		r := openStream(t, srv.URL, streamQuery(), cursor)
		for _, want := range ids {
			if _, op := r.next(); op.ID != want {
				t.Fatalf("cursor %q replayed %q, want %q", cursor, op.ID, want)
			}
		}
		r.cancel()
	}
}

func TestChannelOpsStreamResumesOnADifferentReplica(t *testing.T) {
	a, bus := feedServer(t)
	srvA := httptest.NewServer(a.Handler())
	t.Cleanup(srvA.Close)
	ids := enqueueStreamOps(t, a.Ops, "xrep", 3)
	c := openStream(t, srvA.URL, streamQuery(), "")
	c.next()
	c.cancel() // replica A is gone as far as the consumer is concerned

	b := secondReplica(a, bus)
	srvB := httptest.NewServer(b.Handler())
	t.Cleanup(srvB.Close)
	r := openStream(t, srvB.URL, streamQuery(), ids[0])
	for _, want := range ids[1:] {
		if _, op := r.next(); op.ID != want {
			t.Fatalf("replica B replayed %q, want %q", op.ID, want)
		}
	}
}

func TestChannelOpsStreamRefusals(t *testing.T) {
	other := &agentopsv1alpha1.ChannelAdapter{ObjectMeta: metav1.ObjectMeta{Name: "telegram", Namespace: "agent-ops"}}
	s, _ := feedServer(t, other)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	get := func(query, token string) int {
		req, _ := http.NewRequest("GET", srv.URL+"/channel/ops/stream?"+query, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, err := http.DefaultClient.Do(req.WithContext(ctx))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	scoped := chat.DeriveAdapterToken("bus-test-master", "telegram")
	for name, tc := range map[string]struct {
		query, token string
		want         int
	}{
		"no token":                {streamQuery(), "", 401},
		"missing adapter":         {"contract=" + chat.ContractVersion, "bus-test-master", 400},
		"wrong contract":          {"adapter=console&contract=1", "bus-test-master", 400},
		"no contract":             {"adapter=console", "bus-test-master", 400},
		"another adapter's scope": {streamQuery(), scoped, 403},
		"unknown parameter":       {streamQuery() + "&wait=5", "bus-test-master", 400},
		"control character in id": {streamQuery() + "&since=a%0Ab", "bus-test-master", 400},
		"over-long cursor":        {streamQuery() + "&since=" + strings.Repeat("x", 600), "bus-test-master", 400},
		"any plain id":            {streamQuery() + "&since=abc", "bus-test-master", 200},
	} {
		if got := get(tc.query, tc.token); got != tc.want {
			t.Errorf("%s: status %d, want %d", name, got, tc.want)
		}
	}
	// Telegram keeps the long-poll with its claim and acknowledgement: its own
	// scope passes authorization and is told the stream is not its surface.
	if got := get("adapter=telegram&contract="+chat.ContractVersion, scoped); got != 404 {
		t.Errorf("an adapter without a feed: status %d, want 404", got)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/channel/ops/stream?"+streamQuery(), nil)
	req.Header.Set("Authorization", "Bearer bus-test-master")
	req.Header.Set(lastEventIDHeader, strings.Repeat("x", 600))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("bad Last-Event-ID: status %d, want 400", resp.StatusCode)
	}
}

func TestChannelOpsStreamEndsWhenTheClientDisconnects(t *testing.T) {
	s, _ := feedServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/channel/ops/stream?"+streamQuery(), nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer bus-test-master")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		s.Handler().ServeHTTP(rec, req)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the stream handler kept running after the client went away")
	}
}

// openZombieStream opens a stream whose client then never reads or closes: the
// shape of a consumer behind a path that dropped without a FIN.
func openZombieStream(t *testing.T, base string) {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(base, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	fmt.Fprintf(conn, "GET /channel/ops/stream?%s HTTP/1.1\r\nHost: x\r\nAuthorization: Bearer bus-test-master\r\n\r\n", streamQuery())
	time.Sleep(200 * time.Millisecond) // the handler is registered and serving
}

func receiveAll(t *testing.T, c *streamClient, ids []string) {
	t.Helper()
	for _, want := range ids {
		if _, op := c.next(); op.ID != want {
			t.Fatalf("received %q, want %q", op.ID, want)
		}
	}
}

// A dead connection holds no claim, so a live one receives everything at once,
// with no lease to wait out.
func TestChannelOpsStreamADeadConnectionStrandsNothing(t *testing.T) {
	s, _ := feedServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	openZombieStream(t, srv.URL)
	live := openStream(t, srv.URL, streamQuery(), "")
	ids := enqueueStreamOps(t, s.Ops, "zombie", 6)
	receiveAll(t, live, ids)
}

func TestChannelOpsStreamTwoConnectionsBothReceiveEverything(t *testing.T) {
	s, _ := feedServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	a := openStream(t, srv.URL, streamQuery(), "")
	b := openStream(t, srv.URL, streamQuery(), "")
	ids := enqueueStreamOps(t, s.Ops, "two", 5)
	receiveAll(t, a, ids)
	receiveAll(t, b, ids)
}

// Delivered means the frame was written to a live stream: the side effect an
// acknowledgement used to trigger runs then, with no acknowledgement sent, and
// the thread id is the one the manager assigned.
func TestChannelOpsStreamRunsTheDeliverySideEffects(t *testing.T) {
	conv := &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Name: "conv-1", Namespace: "agent-ops", UID: "uid-1"}}
	s, c, _ := busServer(t, conv)
	s.Ops.FeedAdapters = []string{streamAdapter}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	rec := &state.ConversationRecord{Name: "conv-1", ID: "uid-1"}
	s.Ops.EnqueueEnsureTopic(context.Background(), streamChannel(), rec, chat.TopicDescriptor{})

	stream := openStream(t, srv.URL, streamQuery(), "")
	_, op := stream.next()
	if op.ThreadID == nil || *op.ThreadID != "console-uid-1" {
		t.Fatalf("ensure-topic carries the thread the manager assigned, got %+v", op.ThreadID)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got agentopsv1alpha1.Conversation
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(conv), &got); err != nil {
			t.Fatal(err)
		}
		if b := got.Status.Thread("console-ui"); b != nil && b.ThreadID == "console-uid-1" && s.Ops.Settled(op.ID) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("delivery did not bind the thread without an acknowledgement: %+v", got.Status.Threads)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

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
		Registry: chat.NewRegistry(), Bus: bus, ReceiptAdapters: []string{streamAdapter},
		ChannelsFor:   func(context.Context, string) []string { return []string{"console-ui"} },
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

func openStream(t *testing.T, base, query string) *streamClient {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/channel/ops/stream?"+query, nil)
	req.Header.Set("Authorization", "Bearer bus-test-master")
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

// receiptServer is a Server whose console adapter acknowledges by receipt.
func receiptServer(t *testing.T, objs ...client.Object) (*Server, commandbus.Bus) {
	t.Helper()
	s, _, bus := busServer(t, objs...)
	s.Ops.ReceiptAdapters = []string{streamAdapter}
	return s, bus
}

func streamQuery() string { return "adapter=" + streamAdapter + "&contract=" + chat.ContractVersion }

// ack is the console's plain call: POST /channel/ops/{id}/ack.
func ack(t *testing.T, base, adapter, id, token string) (int, ackReply) {
	t.Helper()
	req, _ := http.NewRequest("POST", base+"/channel/ops/"+url.PathEscape(id)+"/ack?adapter="+adapter, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var reply ackReply
	_ = json.NewDecoder(resp.Body).Decode(&reply)
	return resp.StatusCode, reply
}

func TestChannelOpsStreamPushesInOrderThenNewOnes(t *testing.T) {
	s, _ := receiptServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "order", 3)

	c := openStream(t, srv.URL, streamQuery())
	for i, want := range ids {
		ev, op := c.next()
		if ev.event != "op" || op.ID != want {
			t.Fatalf("event %d = %q %q, want op %q", i, ev.event, op.ID, want)
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
	// Within one connection an unacknowledged message is pushed once.
	c.expectQuiet(600 * time.Millisecond)
}

// The ack is a plain call: the manager consumes the message, and a reconnect is
// offered only what was never acknowledged.
func TestChannelOpsStreamReconnectReOffersOnlyUnacknowledged(t *testing.T) {
	s, _ := receiptServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "re", 4)

	first := openStream(t, srv.URL, streamQuery())
	receiveAll(t, first, ids)
	for _, id := range ids[:2] {
		if code, reply := ack(t, srv.URL, streamAdapter, id, "bus-test-master"); code != 200 || !reply.Consumed {
			t.Fatalf("ack %s = %d %+v", id, code, reply)
		}
	}
	first.cancel() // the connection drops

	second := openStream(t, srv.URL, streamQuery())
	receiveAll(t, second, ids[2:])
	second.expectQuiet(600 * time.Millisecond)
	if s.Ops.Pending(ids[0]) || !s.Ops.Settled(ids[0]) {
		t.Fatal("an acknowledged message is consumed")
	}
}

func TestChannelOpsStreamReOffersOnAnotherReplica(t *testing.T) {
	s, bus := receiptServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "rep", 3)
	if code, _ := ack(t, srv.URL, streamAdapter, ids[0], "bus-test-master"); code != 200 {
		t.Fatalf("ack = %d", code)
	}
	other := httptest.NewServer(secondReplica(s, bus).Handler())
	t.Cleanup(other.Close)
	c := openStream(t, other.URL, streamQuery())
	receiveAll(t, c, ids[1:])
	c.expectQuiet(600 * time.Millisecond)
}

func TestChannelOpsAckRefusals(t *testing.T) {
	other := &agentopsv1alpha1.ChannelAdapter{ObjectMeta: metav1.ObjectMeta{Name: "telegram", Namespace: "agent-ops"}}
	s, _ := receiptServer(t, other)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "ref", 1)
	scoped := chat.DeriveAdapterToken("bus-test-master", "telegram")

	if code, _ := ack(t, srv.URL, streamAdapter, ids[0], ""); code != 401 {
		t.Errorf("no token = %d, want 401", code)
	}
	if code, _ := ack(t, srv.URL, streamAdapter, ids[0], scoped); code != 403 {
		t.Errorf("another adapter's scope = %d, want 403", code)
	}
	if code, _ := ack(t, srv.URL, streamAdapter, "no-such-op", "bus-test-master"); code != 404 {
		t.Errorf("unknown message = %d, want 404", code)
	}
	if code, _ := ack(t, srv.URL, streamAdapter, ids[0], "bus-test-master"); code != 200 {
		t.Fatalf("ack = %d", code)
	}
	if code, _ := ack(t, srv.URL, streamAdapter, ids[0], "bus-test-master"); code != 404 {
		t.Errorf("a duplicate acknowledgement of a consumed message = %d, want 404", code)
	}
}

func TestChannelOpsStreamRefusals(t *testing.T) {
	other := &agentopsv1alpha1.ChannelAdapter{ObjectMeta: metav1.ObjectMeta{Name: "telegram", Namespace: "agent-ops"}}
	s, _ := receiptServer(t, other)
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
	} {
		if got := get(tc.query, tc.token); got != tc.want {
			t.Errorf("%s: status %d, want %d", name, got, tc.want)
		}
	}
	// Telegram reads GET /channel/ops: its own scope passes authorization and is
	// told the stream is not its surface.
	if got := get("adapter=telegram&contract="+chat.ContractVersion, scoped); got != 404 {
		t.Errorf("an adapter that is not a receipt adapter: status %d, want 404", got)
	}
}

func TestChannelOpsStreamEndsWhenTheClientDisconnects(t *testing.T) {
	s, _ := receiptServer(t)
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

// A dead connection holds no claim, so a live one receives everything at once.
func TestChannelOpsStreamADeadConnectionStrandsNothing(t *testing.T) {
	s, _ := receiptServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	openZombieStream(t, srv.URL)
	live := openStream(t, srv.URL, streamQuery())
	ids := enqueueStreamOps(t, s.Ops, "zombie", 6)
	receiveAll(t, live, ids)
}

func TestChannelOpsStreamTwoConnectionsBothReceiveEverything(t *testing.T) {
	s, _ := receiptServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	a := openStream(t, srv.URL, streamQuery())
	b := openStream(t, srv.URL, streamQuery())
	ids := enqueueStreamOps(t, s.Ops, "two", 5)
	receiveAll(t, a, ids)
	receiveAll(t, b, ids)
}

// The receipt, not the write, is the delivery: the side effects (the thread
// binding) run at the acknowledgement, never at the push, and the thread id is
// the one the manager assigned.
func TestChannelOpsAckRunsTheDeliverySideEffects(t *testing.T) {
	conv := &agentopsv1alpha1.Conversation{ObjectMeta: metav1.ObjectMeta{Name: "conv-1", Namespace: "agent-ops", UID: "uid-1"}}
	s, c, _ := busServer(t, conv)
	s.Ops.ReceiptAdapters = []string{streamAdapter}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	rec := &state.ConversationRecord{Name: "conv-1", ID: "uid-1"}
	s.Ops.EnqueueEnsureTopic(context.Background(), streamChannel(), rec, chat.TopicDescriptor{})

	stream := openStream(t, srv.URL, streamQuery())
	_, op := stream.next()
	if op.ThreadID == nil || *op.ThreadID != "console-uid-1" {
		t.Fatalf("ensure-topic carries the thread the manager assigned, got %+v", op.ThreadID)
	}
	var got agentopsv1alpha1.Conversation
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status.Thread("console-ui") != nil {
		t.Fatal("a push alone must not bind the thread")
	}
	if code, _ := ack(t, srv.URL, streamAdapter, op.ID, "bus-test-master"); code != 200 {
		t.Fatalf("ack = %d", code)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(conv), &got); err != nil {
		t.Fatal(err)
	}
	if b := got.Status.Thread("console-ui"); b == nil || b.ThreadID != "console-uid-1" || !s.Ops.Settled(op.ID) {
		t.Fatalf("the acknowledgement did not bind the thread: %+v", got.Status.Threads)
	}
}

// The three steps for an adapter that is not a receipt adapter: GET changes
// nothing, ACK answers the delivery TTL, DONE (or FAILED) reports.
func TestChannelOpsReadAckDoneForAnAdapterWithTheThreeSteps(t *testing.T) {
	slack := &agentopsv1alpha1.ChannelAdapter{ObjectMeta: metav1.ObjectMeta{Name: "slack", Namespace: "agent-ops"}}
	s, _, _ := busServer(t, slack)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	scoped := chat.DeriveAdapterToken("bus-test-master", "slack")
	ch := &agentopsv1alpha1.Channel{}
	ch.Name, ch.Spec.Adapter = "tg", "slack"
	conv := &state.ConversationRecord{Name: "conv-9"}
	thread := "t1"
	s.Ops.EnqueueInputDelivery(context.Background(), ch, conv, "in", &thread, chat.Notice("hello"))
	id := chat.InputOpID("conv-9", "in", "tg")

	read := func() (int, chat.Op) {
		req, _ := http.NewRequest("GET", srv.URL+"/channel/ops?adapter=slack&contract="+chat.ContractVersion, nil)
		req.Header.Set("Authorization", "Bearer "+scoped)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var op chat.Op
		_ = json.NewDecoder(resp.Body).Decode(&op)
		return resp.StatusCode, op
	}
	for i := 0; i < 3; i++ {
		if code, op := read(); code != 200 || op.ID != id {
			t.Fatalf("read %d = %d %q, want the same message every time until acknowledged", i, code, op.ID)
		}
	}
	code, reply := ack(t, srv.URL, "slack", id, scoped)
	if code != 200 || reply.Consumed || reply.DeliveryTTLSeconds != 600 {
		t.Fatalf("ack = %d %+v, want a 600s delivery TTL and no consumption", code, reply)
	}
	if code, _ := read(); code != 204 {
		t.Fatalf("an acknowledged message was read again: %d", code)
	}
	post := func(body string) int {
		req, _ := http.NewRequest("POST", srv.URL+"/channel/ops/"+url.PathEscape(id)+"/done", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+scoped)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if post(`{"error":"the surface refused"}`) != 200 {
		t.Fatal("done report refused")
	}
	if s.Ops.Pending(id) || s.Ops.Settled(id) {
		t.Fatal("a failed report is final: neither pending nor settled")
	}
	if code, _ := read(); code != 204 {
		t.Fatalf("a failed message was offered again: %d", code)
	}
}

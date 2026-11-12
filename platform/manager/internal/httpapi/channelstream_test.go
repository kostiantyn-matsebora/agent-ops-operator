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
		Registry: chat.NewRegistry(), Bus: bus,
		ChannelsFor: func(context.Context, string) []string { return []string{"console-ui"} },
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
func (c *streamClient) next() (sseEvent, claimedOp) {
	c.t.Helper()
	select {
	case ev, ok := <-c.events:
		if !ok {
			c.t.Fatal("stream ended while an op was expected")
		}
		var op claimedOp
		if err := json.Unmarshal([]byte(ev.data), &op); err != nil {
			c.t.Fatalf("op frame data: %v", err)
		}
		return ev, op
	case <-time.After(5 * time.Second):
		c.t.Fatal("no op frame within 5s")
	}
	return sseEvent{}, claimedOp{}
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

func ackOp(t *testing.T, base, id string) {
	t.Helper()
	req, _ := http.NewRequest("POST", base+"/channel/ops/"+id+"/done", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer bus-test-master")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("ack %s: %v %v", id, resp, err)
	}
	resp.Body.Close()
}

func streamQuery() string { return "adapter=" + streamAdapter + "&contract=" + chat.ContractVersion }

func TestChannelOpsStreamDeliversInQueueOrderWithTheCommandIDAsEventID(t *testing.T) {
	s, _, _ := busServer(t)
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
			t.Fatalf("event %d id = %q, want the op's command id %q (the cursor is never an order)", i, ev.id, want)
		}
		if op.ReclaimAfterSeconds != int(chat.ReclaimAfter.Seconds()) {
			t.Fatalf("reclaimAfterSeconds = %d, want the lease", op.ReclaimAfterSeconds)
		}
		if op.Message == nil || op.Channel != "console-ui" {
			t.Fatalf("the body is the long-poll's claimedOp: %+v", op)
		}
	}

	// An op produced after connect is pushed without a reconnect.
	late := enqueueStreamOps(t, s.Ops, "late", 1)
	if _, op := c.next(); op.ID != late[0] {
		t.Fatalf("late op = %q, want %q", op.ID, late[0])
	}
}

func TestChannelOpsStreamResumeReplaysOnlyUnacknowledgedOpsAfterTheCommandID(t *testing.T) {
	s, _, _ := busServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "resume", 4)

	c := openStream(t, srv.URL, streamQuery(), "")
	for range ids {
		c.next()
	}
	ackOp(t, srv.URL, ids[0])
	ackOp(t, srv.URL, ids[1])
	c.cancel() // the connection drops with ops 2 and 3 pushed but unacknowledged

	// While away, one more op is produced.
	late := enqueueStreamOps(t, s.Ops, "away", 1)

	// Last-Event-ID names the command id of the last acknowledged op.
	r := openStream(t, srv.URL, streamQuery(), ids[1])
	for _, want := range []string{ids[2], ids[3], late[0]} {
		if _, op := r.next(); op.ID != want {
			t.Fatalf("replayed %q, want %q (acknowledged ops are never re-sent, then new ops follow)", op.ID, want)
		}
	}
	r.expectQuiet(300 * time.Millisecond)
	r.cancel()

	// The cursor is advisory: ?since= or the header, every unacknowledged op
	// is re-sent and acknowledged ones never are.
	q := openStream(t, srv.URL, streamQuery()+"&since="+url.QueryEscape(ids[2]), ids[0])
	for _, want := range []string{ids[2], ids[3], late[0]} {
		if _, op := q.next(); op.ID != want {
			t.Fatalf("since=%s replayed %q, want %q", ids[2], op.ID, want)
		}
	}
}

// An id the queue does not know is a cursor from a log that has since dropped
// it, or a forgery: either way the stream starts from the oldest unacknowledged
// op rather than refusing or skipping anything.
func TestChannelOpsStreamUnknownCursorStartsFromTheOldestUnacknowledged(t *testing.T) {
	s, _, _ := busServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "unknown", 3)

	c := openStream(t, srv.URL, streamQuery(), "")
	for range ids {
		c.next()
	}
	ackOp(t, srv.URL, ids[0])
	c.cancel()

	r := openStream(t, srv.URL, streamQuery(), "an-id-no-op-ever-had")
	for _, want := range ids[1:] {
		if _, op := r.next(); op.ID != want {
			t.Fatalf("replayed %q, want %q (every unacknowledged op, oldest first)", op.ID, want)
		}
	}
	r.expectQuiet(300 * time.Millisecond)
}

func TestChannelOpsStreamAcknowledgedOpIsNeverResentAndStaysSettled(t *testing.T) {
	s, _, bus := busServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "acked", 1)

	c := openStream(t, srv.URL, streamQuery(), "")
	ev, _ := c.next()
	ackOp(t, srv.URL, ids[0])
	c.cancel()

	r := openStream(t, srv.URL, streamQuery(), ev.id)
	r.expectQuiet(300 * time.Millisecond)
	if st, err := bus.Stats(context.Background(), "out.console-ui", commandbus.KindOutEphemeral); err != nil || st.Running != 0 || st.Pending != 0 {
		t.Fatalf("acknowledged op %s must be settled, not outstanding: %+v %v", ev.id, st, err)
	}
}

func TestChannelOpsStreamResumesOnADifferentReplica(t *testing.T) {
	a, _, bus := busServer(t)
	srvA := httptest.NewServer(a.Handler())
	t.Cleanup(srvA.Close)
	ids := enqueueStreamOps(t, a.Ops, "xrep", 3)

	c := openStream(t, srvA.URL, streamQuery(), "")
	for range ids {
		c.next()
	}
	ackOp(t, srvA.URL, ids[0])
	c.cancel() // replica A is gone as far as the consumer is concerned

	b := secondReplica(a, bus)
	srvB := httptest.NewServer(b.Handler())
	t.Cleanup(srvB.Close)

	r := openStream(t, srvB.URL, streamQuery(), ids[0])
	for _, want := range ids[1:] {
		if ev, op := r.next(); op.ID != want || ev.id == "" {
			t.Fatalf("replica B replayed %q, want %q", op.ID, want)
		}
	}
	// The claim A made is acknowledged on B: nothing is left to lapse and be
	// delivered again.
	ackOp(t, srvB.URL, ids[1])
	ackOp(t, srvB.URL, ids[2])
	r.cancel()
	if st, _ := bus.Stats(context.Background(), "out.console-ui", commandbus.KindOutEphemeral); st.Running != 0 {
		t.Fatalf("acks on the second replica must settle the first replica's claims: %+v", st)
	}
	again := openStream(t, srvB.URL, streamQuery(), ids[2])
	again.expectQuiet(300 * time.Millisecond)
}

func TestChannelOpsStreamRefusals(t *testing.T) {
	other := &agentopsv1alpha1.ChannelAdapter{ObjectMeta: metav1.ObjectMeta{Name: "telegram", Namespace: "agent-ops"}}
	s, _, _ := busServer(t, other)
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
	// A token scoped to the adapter it asks for passes.
	if got := get("adapter=telegram&contract="+chat.ContractVersion, scoped); got != 200 {
		t.Errorf("own scope: status %d, want 200", got)
	}
	// An over-long Last-Event-ID header is refused the same way as a bad ?since=.
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
	s, _, _ := busServer(t)
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

// Every unacknowledged op the dropped stream held stays leased, so a reconnect
// before the lease lapses is the only thing that re-sends it, and nothing else
// can claim it meanwhile.
func TestChannelOpsStreamHeldOpsAreNotHandedToAnotherClaimant(t *testing.T) {
	s, _, _ := busServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "held", 1)

	c := openStream(t, srv.URL, streamQuery(), "")
	if _, op := c.next(); op.ID != ids[0] {
		t.Fatalf("got %q", op.ID)
	}
	c.cancel()
	if op := s.Ops.Claim(streamAdapter); op != nil {
		t.Fatalf("an op pushed and not acknowledged was claimed by someone else: %+v", op)
	}
}

// openZombieStream opens a stream whose client then never reads or closes: the
// shape of a consumer behind a path that dropped without a FIN. The manager's
// handler stays alive, claims, and writes into a connection nobody reads.
func openZombieStream(t *testing.T, base string) {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(base, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	fmt.Fprintf(conn, "GET /channel/ops/stream?%s HTTP/1.1\r\nHost: x\r\nAuthorization: Bearer bus-test-master\r\n\r\n", streamQuery())
	time.Sleep(200 * time.Millisecond) // the handler is registered and polling
}

// An op claimed by a stream nobody reads is leased to it for the whole lease
// (and everything behind it on its key waits). The live stream must deliver it
// without waiting for the lease to lapse.
func TestChannelOpsStreamDeliversOpsAZombieStreamClaimed(t *testing.T) {
	prev := channelStreamSweep
	channelStreamSweep = 100 * time.Millisecond
	t.Cleanup(func() { channelStreamSweep = prev })
	s, _, _ := busServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)

	openZombieStream(t, srv.URL)
	live := openStream(t, srv.URL, streamQuery(), "")
	want := map[string]bool{}
	for _, id := range enqueueStreamOps(t, s.Ops, "zombie", 6) {
		want[id] = true
	}
	deadline := time.After(8 * time.Second)
	for len(want) > 0 {
		select {
		case ev := <-live.events:
			if ev.event == "op" {
				delete(want, ev.id)
			}
		case <-deadline:
			t.Fatalf("%d ops never reached the live stream; they sit leased to a dead one", len(want))
		}
	}
}

// A reconnect names the last op acknowledged. An op claimed earlier whose frame
// the consumer never saw (and so never acknowledged) sits BEFORE that cursor in
// the log, and must still be re-sent.
func TestChannelOpsStreamResumeIgnoresTheCursorForUnacknowledgedOps(t *testing.T) {
	s, _, _ := busServer(t)
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ids := enqueueStreamOps(t, s.Ops, "skip", 4)

	c := openStream(t, srv.URL, streamQuery(), "")
	for range ids {
		c.next()
	}
	// Frames 0 and 1 were lost on the way: only 2 and 3 are acknowledged.
	ackOp(t, srv.URL, ids[2])
	ackOp(t, srv.URL, ids[3])
	c.cancel()

	r := openStream(t, srv.URL, streamQuery(), ids[3])
	for _, want := range ids[:2] {
		if _, op := r.next(); op.ID != want {
			t.Fatalf("replayed %q, want %q", op.ID, want)
		}
	}
}

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fastReports keeps the report retries from slowing a test that fails them.
func fastReports(t *testing.T) {
	t.Helper()
	oa, op := reportAttempts, reportPause
	reportAttempts, reportPause = 2, time.Millisecond
	t.Cleanup(func() { reportAttempts, reportPause = oa, op })
}

// handoverManager is the manager's three-step contract from the adapter's side:
// GET reads (and changes nothing), ack hands the op over, done reports. It
// records the calls in order.
type handoverManager struct {
	t   *testing.T
	srv *httptest.Server

	mu        sync.Mutex
	op        Op
	acked     bool
	calls     []string
	ackStatus int // 0 = 200
	ttlSecs   int
	reports   []map[string]string
}

func newHandoverManager(t *testing.T, op Op) *handoverManager {
	t.Helper()
	h := &handoverManager{t: t, op: op, ttlSecs: 600}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/channel/ops":
			h.calls = append(h.calls, "GET")
			if h.acked {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_ = json.NewEncoder(w).Encode(h.op)
		case strings.HasSuffix(r.URL.Path, "/ack"):
			h.calls = append(h.calls, "ACK")
			if h.ackStatus != 0 {
				w.WriteHeader(h.ackStatus)
				return
			}
			h.acked = true
			_, _ = w.Write([]byte(`{"ok":true,"deliveryTtlSeconds":` + ttlJSON(h.ttlSecs) + `}`))
		case strings.HasSuffix(r.URL.Path, "/done"):
			body, _ := io.ReadAll(r.Body)
			var rep map[string]string
			_ = json.Unmarshal(body, &rep)
			h.calls = append(h.calls, "DONE")
			h.reports = append(h.reports, rep)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func ttlJSON(n int) string { b, _ := json.Marshal(n); return string(b) }

func (h *handoverManager) sequence() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.calls, " ")
}

func handoverAdapter(h *handoverManager, botURL string) *adapter {
	return &adapter{
		mgr: NewManager(h.srv.URL, "tok"), pace: newPacer(), completed: newCompletedOps(16),
		channels: map[string]servedChannel{"tg": {
			cfg: channelConfig{ChatID: "-100"}, gatewayURL: botURL, gatewayToken: "gw",
		}},
		reported: map[string]string{}, clients: map[string]*Telegram{}, channelType: "telegram",
	}
}

func botReturning(t *testing.T, calls *[]string, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, strings.TrimPrefix(r.URL.Path, "/telegram/call/"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// GET, then ACK, then the Bot API, then DONE: the manager has the op handed over
// before the surface is touched, and a second read finds nothing.
func TestDeliveryReadsAcknowledgesDeliversThenReportsDone(t *testing.T) {
	tid := "42"
	h := newHandoverManager(t, Op{ID: "close-1", Channel: "tg", Kind: "close-topic", ThreadID: &tid})
	var bot []string
	a := handoverAdapter(h, botReturning(t, &bot, 200, `{"ok":true,"result":true}`).URL)

	a.pollOnce(context.Background())
	if got := h.sequence(); got != "GET ACK DONE" {
		t.Fatalf("calls = %q, want GET ACK DONE", got)
	}
	if len(bot) == 0 {
		t.Fatal("the surface was never called")
	}
	if rep := h.reports[0]; rep["error"] != "" {
		t.Fatalf("a delivered op reports done, got %+v", rep)
	}
	a.pollOnce(context.Background()) // an acknowledged op is not read again
	if got := h.sequence(); got != "GET ACK DONE GET" {
		t.Fatalf("calls = %q: nothing is read, acknowledged or reported twice", got)
	}
}

// An op the manager refuses to hand over (gone, or behind an earlier one) is not
// delivered and not reported.
func TestOpTheManagerWillNotHandOverIsNotDelivered(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusConflict} {
		tid := "42"
		h := newHandoverManager(t, Op{ID: "close-1", Channel: "tg", Kind: "close-topic", ThreadID: &tid})
		h.ackStatus = status
		var bot []string
		a := handoverAdapter(h, botReturning(t, &bot, 200, `{"ok":true,"result":true}`).URL)
		a.pollOnce(context.Background())
		if got := h.sequence(); got != "GET ACK" || len(bot) != 0 {
			t.Fatalf("status %d: calls %q, bot %v; want GET ACK and no delivery", status, got, bot)
		}
	}
}

// A failed acknowledgement leaves the op with the manager: no delivery, no report.
func TestFailedAcknowledgementDeliversNothing(t *testing.T) {
	tid := "42"
	h := newHandoverManager(t, Op{ID: "close-1", Channel: "tg", Kind: "close-topic", ThreadID: &tid})
	h.ackStatus = http.StatusInternalServerError
	var bot []string
	a := handoverAdapter(h, botReturning(t, &bot, 200, `{"ok":true,"result":true}`).URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond) // cuts the 5s back-off
	defer cancel()
	a.pollOnce(ctx)
	if got := h.sequence(); got != "GET ACK" || len(bot) != 0 {
		t.Fatalf("calls %q, bot %v; want GET ACK and no delivery", got, bot)
	}
}

// A surface that refuses, or asks for a wait longer than the delivery TTL allows,
// ends in FAILED, reported by the adapter itself.
func TestDeliveryThatCannotFinishInsideTheTTLReportsFailed(t *testing.T) {
	tid := "42"
	h := newHandoverManager(t, Op{ID: "close-1", Channel: "tg", Kind: "close-topic", ThreadID: &tid})
	h.ttlSecs = 2 // budget under 2s; Telegram asks for 30s
	var bot []string
	a := handoverAdapter(h, botReturning(t, &bot, 429,
		`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":30}}`).URL)
	a.pollOnce(context.Background())
	if got := h.sequence(); got != "GET ACK DONE" {
		t.Fatalf("calls = %q", got)
	}
	if rep := h.reports[0]; rep["error"] == "" {
		t.Fatalf("a delivery that could not finish inside the TTL reports failed, got %+v", rep)
	}
}

// The ensure-topic topic id travels back in the done report.
func TestEnsureTopicReportsTheNewTopicInTheDoneReport(t *testing.T) {
	h := newHandoverManager(t, Op{ID: "topic-1", Channel: "tg", Kind: "ensure-topic", Conversation: "c1",
		Topic: &TopicDescriptor{Conversation: "c1", Title: "t"}})
	var bot []string
	a := handoverAdapter(h, botReturning(t, &bot, 200, `{"ok":true,"result":{"message_thread_id":777}}`).URL)
	a.pollOnce(context.Background())
	if got := h.sequence(); got != "GET ACK DONE" {
		t.Fatalf("calls = %q", got)
	}
	if rep := h.reports[0]; rep["threadId"] != "777" || rep["error"] != "" {
		t.Fatalf("done report = %+v, want threadId 777", rep)
	}
}

// A restart after the acknowledgement does not deliver again: the manager never
// offers the op, so a fresh process reads nothing.
func TestRestartAfterAcknowledgementDoesNotDuplicate(t *testing.T) {
	tid := "42"
	h := newHandoverManager(t, Op{ID: "close-1", Channel: "tg", Kind: "close-topic", ThreadID: &tid})
	var bot []string
	botURL := botReturning(t, &bot, 200, `{"ok":true,"result":true}`).URL
	if _, err := handoverAdapter(h, botURL).mgr.AckOp(context.Background(), "telegram", "close-1"); err != nil {
		t.Fatal(err)
	}
	restarted := handoverAdapter(h, botURL)
	restarted.pollOnce(context.Background())
	if len(bot) != 0 {
		t.Fatalf("a restarted adapter delivered an acknowledged op: %v", bot)
	}
}

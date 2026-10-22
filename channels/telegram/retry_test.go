package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// A 429 is backpressure, not failure. Telegram says how long to wait; we wait
// exactly that and retry the SAME call — and because a rejected call posted
// nothing, the retry cannot double-post.

// stubBotAPI answers each request from a script of responses.
func stubBotAPI(t *testing.T, script ...string) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var calls []string
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, strings.TrimPrefix(r.URL.Path, "/telegram/call/"))
		body := script[len(script)-1]
		if n < len(script) {
			body = script[n]
		}
		n++
		mu.Unlock()
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

const rateLimited = `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`

func TestRetryAfterIsHonouredThenSucceeds(t *testing.T) {
	srv, calls := stubBotAPI(t, rateLimited, `{"ok":true,"result":{"message_id":7}}`)
	tg := NewTelegram(srv.URL, "gw-token")

	start := time.Now()
	if err := tg.Send(context.Background(), "-100", nil, "hello"); err != nil {
		t.Fatalf("a 429 followed by a 200 must succeed: %v", err)
	}
	// it WAITED the interval Telegram stated, rather than a backoff of its own
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Fatalf("retry_after: 1 must be slept out, returned after %v", elapsed)
	}
	if len(*calls) != 2 {
		t.Fatalf("want exactly one retry, got %d calls: %v", len(*calls), *calls)
	}
	// EXACTLY ONE POST: the rejected call posted nothing, so the retry is not a
	// duplicate. This is the property that makes retrying safe at all.
	for _, c := range *calls {
		if c != "sendMessage" {
			t.Fatalf("unexpected call %q", c)
		}
	}
}

func TestSustainedRateLimitFailsInsideTheBudget(t *testing.T) {
	srv, calls := stubBotAPI(t, rateLimited)
	tg := NewTelegram(srv.URL, "gw-token")

	// a 4-second budget against a 1s retry_after: it gives up rather than
	// running past the claim window
	ctx := WithRetryBudget(context.Background(), 4*time.Second)
	start := time.Now()
	err := tg.Send(ctx, "-100", nil, "hello")
	if err == nil {
		t.Fatal("a sustained 429 must be reported as a failure once the budget is spent")
	}
	if !strings.Contains(err.Error(), "claim budget") {
		t.Fatalf("the error must say the budget ran out, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 6*time.Second {
		t.Fatalf("gave up after %v — the budget must bound the wait", elapsed)
	}
	if len(*calls) > 5 {
		t.Fatalf("a 4s budget at 1s per retry must not make %d calls", len(*calls))
	}
}

// createForumTopic rides out backpressure the same way — it was 105 of the
// 2026-08-13 rejections.
func TestCreateTopicHonoursRetryAfter(t *testing.T) {
	srv, calls := stubBotAPI(t, rateLimited, `{"ok":true,"result":{"message_thread_id":4242}}`)
	tg := NewTelegram(srv.URL, "gw-token")

	id, err := tg.CreateTopic(context.Background(), "-100", "PodCrashLooping")
	if err != nil {
		t.Fatalf("createForumTopic must ride out a 429: %v", err)
	}
	if id != 4242 {
		t.Fatalf("thread id: %d", id)
	}
	if len(*calls) != 2 {
		t.Fatalf("want one retry, got %v", *calls)
	}
}

// An error WITHOUT retry_after is terminal: nothing to wait for, so report it
// and let the manager re-derive.
func TestNonRateLimitErrorIsNotRetried(t *testing.T) {
	srv, calls := stubBotAPI(t, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`)
	tg := NewTelegram(srv.URL, "gw-token")

	if err := tg.Send(context.Background(), "-100", nil, "hello"); err == nil {
		t.Fatal("a 400 must be reported")
	}
	if len(*calls) != 1 {
		t.Fatalf("a terminal error must not be retried, got %d calls", len(*calls))
	}
}

// ---- A1/A2/A9: one budget, shared across every call an operation makes -----

const rateLimited2s = `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 2","parameters":{"retry_after":2}}`

// THE PROPERTY A2 PINS: the budget is minted ONCE per operation and every
// call the operation goes on to make spends from the SAME remaining time — a
// per-call budget (the bug) would let a second call retry on its own fresh
// allowance and the operation as a whole would run well past what the manager
// actually left it.
func TestRetryBudgetIsSharedAcrossEveryCallInTheOperation(t *testing.T) {
	srv, calls := stubBotAPI(t, rateLimited2s, `{"ok":true,"result":{"message_id":1}}`, rateLimited2s)
	tg := NewTelegram(srv.URL, "gw-token")

	ctx := WithRetryBudget(context.Background(), 3*time.Second)
	start := time.Now()

	// First call (a chunk send, say): one 429 then success — spends ~2s of
	// the operation's 3s budget.
	if err := tg.Send(ctx, "-100", nil, "chunk one"); err != nil {
		t.Fatalf("first chunk must succeed: %v", err)
	}
	// Second call, the SAME operation's context: a fresh 429 needs another
	// 2s, which the shared budget no longer has (~1s left) — it must fail
	// without sleeping again.
	err := tg.Send(ctx, "-100", nil, "chunk two")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("the second call must fail once the shared operation budget is spent")
	}
	if !strings.Contains(err.Error(), "claim budget") {
		t.Fatalf("want the budget named as the reason, got %v", err)
	}
	if elapsed >= 3*time.Second {
		t.Fatalf("the operation ran %v, at or past its 3s shared budget — "+
			"the budget was not shared across its calls", elapsed)
	}
	if len(*calls) != 3 {
		t.Fatalf("want exactly 3 Bot API calls (429, 200, 429-and-stop), got %d: %v", len(*calls), *calls)
	}
}

// A dial error or a timeout posted nothing, so retrying it is exactly as safe
// as retrying a 429 — and it must happen, or a transient blip fails an
// operation that a second's wait would have ridden out (A1).
func TestDroppedConnectionIsRetriedWithinTheBudget(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			// A dropped connection, no response at all — what a dial failure or
			// a timeout looks like to the client making the call.
			hj := w.(http.Hijacker)
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()
	tg := NewTelegram(srv.URL, "gw-token")

	if err := tg.Send(context.Background(), "-100", nil, "hi"); err != nil {
		t.Fatalf("a dropped connection must be retried, not reported: %v", err)
	}
	if n != 2 {
		t.Fatalf("want one retry after the dropped connection, got %d calls", n)
	}
}

// A bare 5xx — the gateway's own leg to Telegram down, or Telegram's own
// internal error — is the same transient condition a dial error is, and rides
// the same clock rather than failing the operation outright (A1).
func TestBareGatewayFiveHundredIsRetriedWithinTheBudget(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			http.Error(w, "upstream dial failed", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()
	tg := NewTelegram(srv.URL, "gw-token")

	if err := tg.Send(context.Background(), "-100", nil, "hi"); err != nil {
		t.Fatalf("a bare 502 must be retried, not reported: %v", err)
	}
	if n != 2 {
		t.Fatalf("want one retry after the 502, got %d calls", n)
	}
}

// A sustained transient failure still gives up inside the operation's budget —
// the same bound a sustained 429 is held to.
func TestSustainedTransientFailureFailsInsideTheBudget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream dial failed", http.StatusBadGateway)
	}))
	defer srv.Close()
	tg := NewTelegram(srv.URL, "gw-token")

	ctx := WithRetryBudget(context.Background(), 3*time.Second)
	start := time.Now()
	err := tg.Send(ctx, "-100", nil, "hi")
	if err == nil {
		t.Fatal("a sustained 502 must be reported as a failure once the budget is spent")
	}
	if !strings.Contains(err.Error(), "claim budget") {
		t.Fatalf("want the budget named as the reason, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("gave up after %v — the budget must bound the wait", elapsed)
	}
}

// close-topic rides the SAME retry rule as any other call (A9): a 429 against
// it is absorbed exactly like one against sendMessage.
func TestCloseTopicHonoursRetryAfter(t *testing.T) {
	srv, calls := stubBotAPI(t, rateLimited, `{"ok":true,"result":{}}`)
	tg := NewTelegram(srv.URL, "gw-token")

	if err := tg.CloseTopic(context.Background(), "-100", 9876); err != nil {
		t.Fatalf("closeForumTopic must ride out a 429: %v", err)
	}
	if len(*calls) != 2 {
		t.Fatalf("want one retry, got %v", *calls)
	}
}

// The budget comes from the manager's advertised claim window, halved — the op
// still has to be sent and completed after the last retry.
func TestRetryBudgetIsHalfTheAdvertisedClaim(t *testing.T) {
	cases := []struct {
		advertised int
		want       time.Duration
	}{
		{300, 150 * time.Second}, // the shipped ReclaimAfter
		{60, 30 * time.Second},
		{0, defaultRetryBudget}, // an older manager advertises nothing
	}
	for _, tc := range cases {
		op := &Op{ReclaimAfterSeconds: tc.advertised}
		if got := op.RetryBudget(); got != tc.want {
			t.Fatalf("reclaimAfterSeconds=%d: budget %v, want %v", tc.advertised, got, tc.want)
		}
	}
	// and it must stay STRICTLY below the window, or a second claimant
	// duplicates the message
	op := &Op{ReclaimAfterSeconds: 300}
	if op.RetryBudget() >= 300*time.Second {
		t.Fatal("the retry budget must stay strictly inside the claim window")
	}
}

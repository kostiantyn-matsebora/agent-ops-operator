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
		calls = append(calls, strings.TrimPrefix(r.URL.Path, "/botbot-token/"))
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
	tg := &Telegram{Token: "bot-token", HTTP: routeTo(srv.URL)}

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
	tg := &Telegram{Token: "bot-token", HTTP: routeTo(srv.URL)}

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
	tg := &Telegram{Token: "bot-token", HTTP: routeTo(srv.URL)}

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
	tg := &Telegram{Token: "bot-token", HTTP: routeTo(srv.URL)}

	if err := tg.Send(context.Background(), "-100", nil, "hello"); err == nil {
		t.Fatal("a 400 must be reported")
	}
	if len(*calls) != 1 {
		t.Fatalf("a terminal error must not be retried, got %d calls", len(*calls))
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

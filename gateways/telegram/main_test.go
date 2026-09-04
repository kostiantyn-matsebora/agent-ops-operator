package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestLoadConfigDiscoversProjectedBotToken closes the gap on the
// AGENTOPS_CRED_* discovery loop: the existing apibase_test.go only exercises
// the plain TELEGRAM_BOT_TOKEN path. It also pins that SIGNAL_TARGET's
// trailing slash is trimmed exactly as APIBase's already is.
func TestLoadConfigDiscoversProjectedBotToken(t *testing.T) {
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("SIGNAL_TARGET", "http://signal/")
	t.Setenv("CHANNEL_TARGET", "http://channel")
	t.Setenv("AGENTOPS_CRED_TELEGRAM_botToken", "projected-token")
	t.Setenv(apiBaseEnv, "")

	cfg := loadConfig()
	if cfg.Token != "projected-token" {
		t.Fatalf("loadConfig did not discover the projected bot token, got %q", cfg.Token)
	}
	if cfg.SignalTarget != "http://signal" {
		t.Fatalf("SignalTarget trailing slash not trimmed, got %q", cfg.SignalTarget)
	}
}

// TestSleepCtxReturnsWhenContextCancelled: sleepCtx is the primitive both
// error-retry branches of poll() rely on. An already-cancelled context must
// short-circuit the wait rather than sitting out the full duration.
func TestSleepCtxReturnsWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	sleepCtx(ctx, 5*time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("sleepCtx took %s, want a prompt return on cancellation", elapsed)
	}
}

// TestSleepCtxWaitsOutTheDuration is the other half of the select: with no
// cancellation, it actually waits for the timer.
func TestSleepCtxWaitsOutTheDuration(t *testing.T) {
	start := time.Now()
	sleepCtx(context.Background(), 20*time.Millisecond)
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("sleepCtx returned after %s, want >= 20ms", elapsed)
	}
}

// TestPollForwardsUpdatesAndAdvancesOffset drives the whole poll loop over
// real HTTP test servers: initial offset acquisition (offset < 0), a batch
// with one update forwarded to the signal target, the offset advancing past
// it, and the confirmed offset reported downstream. None of this ran before
// — the existing suite only called route() and the Downstream methods in
// isolation, never poll() itself.
func TestPollForwardsUpdatesAndAdvancesOffset(t *testing.T) {
	var forwarded string
	signalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		forwarded = string(b)
	}))
	defer signalSrv.Close()

	var offsetPuts []string
	channelSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/offset":
			_ = json.NewEncoder(w).Encode(map[string]string{"value": "10"})
		case r.Method == http.MethodPut && r.URL.Path == "/offset":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			offsetPuts = append(offsetPuts, in["value"])
		}
	}))
	defer channelSrv.Close()

	var getCalls int32
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&getCalls, 1) == 1 {
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"update_id":10,"message":{"text":"hi","chat":{"id":1}}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer tgSrv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)

	cfg := config{SignalTarget: signalSrv.URL, ChannelTarget: channelSrv.URL, Token: "t", APIBase: tgSrv.URL}
	r := &router{cfg: cfg, down: NewDownstream(), tg: NewTelegram(cfg.Token, cfg.APIBase)}
	r.poll(ctx)

	if forwarded == "" {
		t.Fatal("the update was never forwarded to the signal target")
	}
	if len(offsetPuts) == 0 || offsetPuts[0] != "11" {
		t.Fatalf("offset not advanced and reported correctly, got %v", offsetPuts)
	}
}

// TestPollRetriesOnOffsetReadError exercises the "offset read: %v" branch and
// its sleepCtx backoff — offset acquisition fails, and the loop must retry
// rather than crash, until the context is cancelled out from under it.
func TestPollRetriesOnOffsetReadError(t *testing.T) {
	var reads int32
	channelSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reads, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer channelSrv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	cfg := config{SignalTarget: "http://unused", ChannelTarget: channelSrv.URL, Token: "t"}
	r := &router{cfg: cfg, down: NewDownstream(), tg: NewTelegram("t", "http://unused")}
	r.poll(ctx)

	if atomic.LoadInt32(&reads) == 0 {
		t.Fatal("expected at least one offset-read attempt before the context was cancelled")
	}
}

// TestPollRetriesOnGetUpdatesError exercises the "getUpdates: %v" branch: the
// offset read succeeds, but the Bot API call itself fails.
func TestPollRetriesOnGetUpdatesError(t *testing.T) {
	channelSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"value": "5"})
	}))
	defer channelSrv.Close()

	var calls int32
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tgSrv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)

	cfg := config{ChannelTarget: channelSrv.URL, Token: "t", APIBase: tgSrv.URL}
	r := &router{cfg: cfg, down: NewDownstream(), tg: NewTelegram(cfg.Token, cfg.APIBase)}
	r.poll(ctx)

	if atomic.LoadInt32(&calls) == 0 {
		t.Fatal("expected at least one getUpdates attempt before the context was cancelled")
	}
}

// TestPollBreaksInnerLoopOnOffsetPutError closes the one branch none of the
// above reach: a PutOffset failure after forwarding an update must break the
// inner per-batch loop (skipping the rest of the batch) rather than looping
// forever retrying the same PUT — the outer poll loop is what recovers by
// re-reading getUpdates on the next iteration.
func TestPollBreaksInnerLoopOnOffsetPutError(t *testing.T) {
	channelSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"value": "1"})
		case http.MethodPut:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer channelSrv.Close()

	var forwardedCount int32
	signalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&forwardedCount, 1)
	}))
	defer signalSrv.Close()

	var getCalls int32
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&getCalls, 1) == 1 {
			_, _ = w.Write([]byte(`{"ok":true,"result":[
				{"update_id":1,"message":{"text":"a","chat":{"id":1}}},
				{"update_id":2,"message":{"text":"b","chat":{"id":1}}}
			]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer tgSrv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)

	cfg := config{SignalTarget: signalSrv.URL, ChannelTarget: channelSrv.URL, Token: "t", APIBase: tgSrv.URL}
	r := &router{cfg: cfg, down: NewDownstream(), tg: NewTelegram(cfg.Token, cfg.APIBase)}
	r.poll(ctx)

	if got := atomic.LoadInt32(&forwardedCount); got != 1 {
		t.Fatalf("expected exactly one update forwarded before the PUT failure broke the inner loop, got %d", got)
	}
}

// TestRouteAcknowledgesASelectionBeforeForwarding closes the acknowledge
// branch of route(): the existing table (telegram_test.go) only ever routed
// updates with no callback id, so r.tg was never even set. A selection must
// both be acknowledged (so the tapper's client stops spinning) and still be
// forwarded verbatim, in that order.
func TestRouteAcknowledgesASelectionBeforeForwarding(t *testing.T) {
	var acked string
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		acked, _ = body["callback_query_id"].(string)
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer tgSrv.Close()

	var forwarded string
	signalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		forwarded = string(b)
	}))
	defer signalSrv.Close()

	cfg := config{SignalTarget: signalSrv.URL, ChannelTarget: "http://unused"}
	r := &router{cfg: cfg, down: NewDownstream(), tg: NewTelegram("t", tgSrv.URL)}

	raw := `{"update_id":30,"callback_query":{"id":"cb-9","data":"p:x","message":{"chat":{"id":1}}}}`
	u, err := classifyUpdate(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	r.route(context.Background(), u)

	if acked != "cb-9" {
		t.Fatalf("selection was not acknowledged: got callback id %q", acked)
	}
	if forwarded != raw {
		t.Fatalf("selection was not also forwarded verbatim: %q", forwarded)
	}
}

// TestRouteStillForwardsWhenAcknowledgeFails: a failed acknowledgement is
// logged and dropped, never allowed to stop the forward — a dropped
// acknowledgement is a worse-turning spinner, but a dropped selection would
// be worse still.
func TestRouteStillForwardsWhenAcknowledgeFails(t *testing.T) {
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer tgSrv.Close()

	var forwarded string
	signalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		forwarded = string(b)
	}))
	defer signalSrv.Close()

	cfg := config{SignalTarget: signalSrv.URL, ChannelTarget: "http://unused"}
	r := &router{cfg: cfg, down: NewDownstream(), tg: NewTelegram("t", tgSrv.URL)}

	raw := `{"update_id":31,"callback_query":{"id":"cb-10","data":"p:x"}}`
	u, err := classifyUpdate(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	r.route(context.Background(), u) // must not panic despite the failed acknowledge

	if forwarded != raw {
		t.Fatalf("update must still be forwarded even when acknowledge fails: %q", forwarded)
	}
}

// TestMainStartsAndStopsOnContextCancellation is the only way to close the
// gap on main() itself: it wires loadConfig → NewDownstream/NewTelegram →
// poll() under a signal.NotifyContext, and nothing else in the suite ever
// calls it. baseContext is swapped so the test can cancel the SAME context
// main() derives its signal context from, directly — the derived ctx sees
// that exactly as it would a real SIGINT — without sending any OS signal
// anywhere and without a fixed sleep to dodge a signal-registration race:
// the first GetUpdates request proves poll() has actually entered its loop
// before cancellation, deterministically. Calling the real main() in-process
// (rather than a subprocess) is also what lets it show up in coverage at
// all — coverage data does not cross a process boundary.
func TestMainStartsAndStopsOnContextCancellation(t *testing.T) {
	channelSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/offset" {
			_ = json.NewEncoder(w).Encode(map[string]string{"value": "1"})
		}
	}))
	defer channelSrv.Close()

	polled := make(chan struct{}, 1)
	tgSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case polled <- struct{}{}:
		default:
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer tgSrv.Close()

	t.Setenv("SIGNAL_TARGET", "http://unused")
	t.Setenv("CHANNEL_TARGET", channelSrv.URL)
	t.Setenv("TELEGRAM_BOT_TOKEN", "t")
	t.Setenv(apiBaseEnv, tgSrv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	origBaseContext := baseContext
	baseContext = func() context.Context { return ctx }
	defer func() { baseContext = origBaseContext }()

	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	select {
	case <-polled:
	case <-time.After(5 * time.Second):
		t.Fatal("poll() never reached the Telegram double")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("main() did not return after its context was cancelled")
	}
}

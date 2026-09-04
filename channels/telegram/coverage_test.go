package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// This file closes coverage gaps left after errorpaths_test.go and
// sanitizelog_test.go (sonar-ratings-baseline): the largest remaining
// uncovered code was execute()'s "send"/"close-topic" cases and the
// ensure-topic reopen branch (never exercised — every existing execute()
// test used "ensure-topic" fresh-create or "delete-conversation"), plus a
// handful of small pure functions sitting at 0%.

// ---- execute(): "ensure-topic" ---------------------------------------------

// A reopen carries the closed topic's own id as a hint. Telegram CAN
// un-archive it, so the SAME id must come back — that is what makes the
// conversation continue in the thread it already had rather than a fresh one.
// No existing test ever set PreviousThreadID, so this branch of execute() had
// never run.
func TestExecuteReopensTopicFromPreviousThreadID(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	a := &adapter{
		channels: map[string]servedChannel{"tg": {cfg: channelConfig{ChatID: "-100"}, token: "bot-token", apiBase: srv.URL}},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	op := &Op{Channel: "tg", Kind: "ensure-topic", Topic: &TopicDescriptor{
		Conversation: "conv-1", PreviousThreadID: "99",
	}}
	threadID, opErr := a.execute(context.Background(), op)
	if opErr != "" {
		t.Fatalf("opErr = %q, want empty", opErr)
	}
	if threadID != "99" {
		t.Fatalf("threadID = %q, want the PREVIOUS id (99), never a new one", threadID)
	}
	if len(calls) != 1 || calls[0] != "reopenForumTopic" {
		t.Fatalf("want exactly one reopenForumTopic call, got %v", calls)
	}
}

// A signal with no topic descriptor at all cannot be actioned, and must be
// reported rather than panic on a nil dereference.
func TestExecuteEnsureTopicWithoutADescriptorIsReported(t *testing.T) {
	a := &adapter{
		channels: map[string]servedChannel{"tg": {cfg: channelConfig{ChatID: "-100"}, token: "bot-token"}},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	_, opErr := a.execute(context.Background(), &Op{Channel: "tg", Kind: "ensure-topic"})
	if opErr == "" {
		t.Fatal("an ensure-topic with no topic descriptor must be reported, not silently dropped")
	}
}

// ---- execute(): "send" ------------------------------------------------------

// The ordinary send path: no existing test ever drove a "send" op through a
// SERVED channel far enough to reach SendWith — the two "send" cases in
// errorpaths_test.go both target an UNSERVED channel and never leave
// execute()'s early "not served" branch.
func TestExecuteSendsAnOrdinaryMessage(t *testing.T) {
	var calls []string
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer srv.Close()

	a := &adapter{
		channels: map[string]servedChannel{"tg": {cfg: channelConfig{ChatID: "-100"}, token: "bot-token", apiBase: srv.URL}},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	tid := "5"
	msg := Message{Kind: MsgAnswer, Body: "the fix landed"}
	threadID, opErr := a.execute(context.Background(), &Op{
		Channel: "tg", Kind: "send", ThreadID: &tid, Message: &msg,
	})
	if opErr != "" {
		t.Fatalf("opErr = %q, want empty", opErr)
	}
	if threadID != "" {
		t.Fatalf("threadID = %q, want empty (send never mints one)", threadID)
	}
	if len(calls) != 1 || calls[0] != "sendMessage" {
		t.Fatalf("want exactly one sendMessage, got %v", calls)
	}
	if !strings.Contains(bodies[0], `"message_thread_id":5`) {
		t.Fatalf("the op's thread id must carry through: %s", bodies[0])
	}
}

// A send with no message is meaningless and must be reported, not sent as an
// empty string.
func TestExecuteSendWithoutAMessageIsReported(t *testing.T) {
	a := &adapter{
		channels: map[string]servedChannel{"tg": {cfg: channelConfig{ChatID: "-100"}, token: "bot-token"}},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	_, opErr := a.execute(context.Background(), &Op{Channel: "tg", Kind: "send"})
	if opErr == "" {
		t.Fatal("a send with no message must be reported, not silently ignored")
	}
}

// A signal payload over the document threshold is uploaded as an attachment
// rather than posted as a wall of chunked messages — the "send" case's
// asDocument branch, never exercised: it needs a REAL body long enough to
// cross documentThreshold, which no existing execute() test builds.
func TestExecuteSendsAnOversizedSignalAsADocument(t *testing.T) {
	var calls []string
	var filename string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		calls = append(calls, method)
		if method == "sendDocument" {
			if err := r.ParseMultipartForm(1 << 20); err == nil {
				if fh := r.MultipartForm.File["document"]; len(fh) == 1 {
					filename = fh[0].Filename
				}
			}
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	a := &adapter{
		channels: map[string]servedChannel{"tg": {cfg: channelConfig{ChatID: "-100"}, token: "bot-token", apiBase: srv.URL}},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	big := strings.Repeat("line\n", 3000) // well past documentThreshold (3*4096)
	msg := Message{Kind: MsgSignal, Source: "prom", Title: "PodCrashLooping", Body: big}
	threadID, opErr := a.execute(context.Background(), &Op{Channel: "tg", Kind: "send", Message: &msg})
	if opErr != "" {
		t.Fatalf("opErr = %q, want empty", opErr)
	}
	if threadID != "" {
		t.Fatalf("threadID = %q, want empty", threadID)
	}
	if len(calls) != 1 || calls[0] != "sendDocument" {
		t.Fatalf("an oversized signal must upload, not chunk-send: %v", calls)
	}
	if filename != "prom.txt" {
		t.Fatalf("documentName must name the attachment after the signal's source: got %q", filename)
	}
}

// ---- execute(): "close-topic" ----------------------------------------------

// The ordinary close: no existing test drives Kind:"close-topic" through
// execute() at all.
func TestExecuteClosesATopic(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer srv.Close()

	a := &adapter{
		channels: map[string]servedChannel{"tg": {cfg: channelConfig{ChatID: "-100"}, token: "bot-token", apiBase: srv.URL}},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	tid := "42"
	_, opErr := a.execute(context.Background(), &Op{Channel: "tg", Kind: "close-topic", ThreadID: &tid})
	if opErr != "" {
		t.Fatalf("opErr = %q, want empty", opErr)
	}
	if len(calls) != 1 || calls[0] != "closeForumTopic" {
		t.Fatalf("want exactly one closeForumTopic call, got %v", calls)
	}
}

// A close-topic with no thread id names nothing to archive.
func TestExecuteCloseTopicWithoutAThreadIsReported(t *testing.T) {
	a := &adapter{
		channels: map[string]servedChannel{"tg": {cfg: channelConfig{ChatID: "-100"}, token: "bot-token"}},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	_, opErr := a.execute(context.Background(), &Op{Channel: "tg", Kind: "close-topic"})
	if opErr == "" {
		t.Fatal("a close-topic with no thread id must be reported")
	}
}

// A thread id that is not a Telegram topic id (never happens in practice, but
// execute() must report it rather than pass a garbage id to the Bot API).
func TestExecuteCloseTopicWithAnUnparsableThreadIsReported(t *testing.T) {
	a := &adapter{
		channels: map[string]servedChannel{"tg": {cfg: channelConfig{ChatID: "-100"}, token: "bot-token"}},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	tid := "not-a-number"
	_, opErr := a.execute(context.Background(), &Op{Channel: "tg", Kind: "close-topic", ThreadID: &tid})
	if opErr == "" || !strings.Contains(opErr, "not a telegram topic id") {
		t.Fatalf("opErr = %q, want a parse-failure message", opErr)
	}
}

// ---- execute(): unknown kind ------------------------------------------------

// An op kind this adapter has never heard of must be reported rather than
// silently dropped — the fallthrough at the end of execute()'s switch, never
// reached by any existing test.
func TestExecuteReportsAnUnknownOpKind(t *testing.T) {
	a := &adapter{
		channels: map[string]servedChannel{"tg": {cfg: channelConfig{ChatID: "-100"}, token: "bot-token"}},
		clients:  map[string]*Telegram{}, reported: map[string]string{},
	}
	_, opErr := a.execute(context.Background(), &Op{Channel: "tg", Kind: "reticulate-splines"})
	if !strings.Contains(opErr, "reticulate-splines") {
		t.Fatalf("opErr = %q, want it to name the unknown kind", opErr)
	}
}

// ---- handleUpdate: the real HTTP entry point --------------------------------
//
// Every other dispatch test calls a.dispatch directly. Nothing exercised the
// HTTP handler itself — request body reading, JSON decoding and the status
// codes the router actually sees.

func TestHandleUpdateAcceptsAValidUpdate(t *testing.T) {
	a, got := testAdapter(t, map[string]channelConfig{"tg": {ChatID: "-100"}})
	h := a.handler()
	body := `{"update_id":1,"message":{"text":"go","chat":{"id":-100},"from":{"id":1},"is_topic_message":true,"message_thread_id":7}}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader(body)))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if len(*got) != 1 {
		t.Fatal("a valid topic update must reach the manager as inbound")
	}
}

func TestHandleUpdateRejectsMalformedJSON(t *testing.T) {
	a, _ := testAdapter(t, nil)
	h := a.handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/updates", strings.NewReader("not json")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// errReader always fails, to drive handleUpdate's io.ReadAll error branch —
// the router is a dumb pipe, so a truncated or broken body must answer 400
// rather than panic.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestHandleUpdateRejectsAnUnreadableBody(t *testing.T) {
	a, _ := testAdapter(t, nil)
	h := a.handler()
	req := httptest.NewRequest(http.MethodPost, "/updates", io.NopCloser(errReader{}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// ---- menu.displayName -------------------------------------------------------
//
// The reverse of publishedName: rendering a PUBLISHED name in the spelling the
// menu completes. Was at 0% — nothing called it at all.

func TestMenuDisplayName(t *testing.T) {
	m := newMenu()
	if got := m.displayName("k8s-observe"); got != "k8s_observe" {
		t.Fatalf("displayName(k8s-observe) = %q, want k8s_observe", got)
	}
	// A name Telegram cannot express (a dot) passes through unchanged, exactly
	// as spellForTelegram documents.
	if got := m.displayName("profile.v2"); got != "profile.v2" {
		t.Fatalf("displayName(profile.v2) = %q, want unchanged", got)
	}
	var nilMenu *menu
	if got := nilMenu.displayName("k8s-observe"); got != "k8s-observe" {
		t.Fatalf("a nil menu must be inert, got %q", got)
	}
}

// publishedName's non-nil path was never exercised — TestNilMenuIsInert only
// calls it on a nil receiver. This drives the real lookup: a name the menu
// registered comes back as the manager's own spelling, an unregistered one
// falls back to the dash/underscore reversal.
func TestMenuPublishedNameOnARegisteredMenu(t *testing.T) {
	m := newMenu()
	m.spelling = map[string]string{"k8s_observe": "k8s-observe"}
	if got := m.publishedName("k8s_observe"); got != "k8s-observe" {
		t.Fatalf("publishedName(k8s_observe) = %q, want the registered spelling", got)
	}
	if got := m.publishedName("unregistered_name"); got != "unregistered-name" {
		t.Fatalf("publishedName(unregistered_name) = %q, want the dash fallback", got)
	}
}

// ---- pure render helpers, all at 0% -----------------------------------------

// utf8Boundary is what keeps a hard chunk cut from splitting a multi-byte
// rune. Exercised indirectly by splitBlocks on very long input, but never
// asserted directly against its own contract.
func TestUtf8BoundaryFindsWhereARuneStarts(t *testing.T) {
	// "é" is two bytes (0xC3 0xA9); index 1 lands inside it, not at its start.
	s := "é"
	if utf8Boundary(s, 1) {
		t.Fatal("index 1 is a continuation byte, not a rune boundary")
	}
	if !utf8Boundary(s, 0) {
		t.Fatal("index 0 starts the rune")
	}
	if !utf8Boundary(s, len(s)) {
		t.Fatal("the end of the string is always a boundary")
	}
}

// documentName falls back to a generic name when the signal carries no
// source — a manager older than the source label, or a hand-built signal.
func TestDocumentNameFallsBackWithNoSource(t *testing.T) {
	if got := documentName(Message{}); got != "signal.txt" {
		t.Fatalf("documentName({}) = %q, want signal.txt", got)
	}
	if got := documentName(Message{Source: "ha"}); got != "ha.txt" {
		t.Fatalf("documentName(source=ha) = %q, want ha.txt", got)
	}
}

// asDocument only ever applies to a signal past the threshold — an answer, or
// a short signal, must never be pulled out of the thread as a file.
func TestAsDocumentOnlyAppliesToOversizedSignals(t *testing.T) {
	if _, _, ok := asDocument(Message{Kind: MsgAnswer, Body: strings.Repeat("x", documentThreshold+1)}); ok {
		t.Fatal("an answer, however long, must never become a document")
	}
	if _, _, ok := asDocument(Message{Kind: MsgSignal, Body: "short"}); ok {
		t.Fatal("a short signal must stay in the thread")
	}
	caption, content, ok := asDocument(Message{Kind: MsgSignal, Title: "Big", Body: strings.Repeat("x", documentThreshold+1)})
	if !ok {
		t.Fatal("an oversized signal must qualify")
	}
	if content == "" {
		t.Fatal("the raw payload must be the attachment content")
	}
	if strings.Contains(caption, strings.Repeat("x", documentThreshold+1)) {
		t.Fatal("the caption must NOT carry the payload — that is what makes it a caption and not a duplicate")
	}
}

// degradeQuotes is what SendWith falls back to on a Bot API old enough to
// refuse `<blockquote expandable>` — a plain quote with a visible marker
// instead of an unclosed tag the transport would reject outright.
func TestDegradeQuotesRewritesTheExpandableTag(t *testing.T) {
	got := degradeQuotes("before <blockquote expandable>hidden</blockquote> after")
	if strings.Contains(got, "expandable") {
		t.Fatalf("the expandable form must be gone: %q", got)
	}
	if !strings.Contains(got, "<b>Details</b>") {
		t.Fatalf("the degraded form must keep a visible marker: %q", got)
	}
}

// ---- SendWith's own degrade-and-retry, never driven end to end -------------
//
// render_test.go pins the RENDERING side of the latch (quote() picks the
// degraded form once expandableQuotes is false). Nothing drove the SEND side:
// a live refusal naming the quote tag, degradeQuotes rewriting the ALREADY
// RENDERED html, and a second attempt that must not repeat the first one's
// tag.
func TestSendWithDegradesAndRetriesOnAnUnsupportedQuoteTag(t *testing.T) {
	t.Cleanup(func() { expandableQuotes.Store(true) })
	expandableQuotes.Store(true)

	var bodies []string
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		n++
		if n == 1 {
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: can't parse entities: Can't find end tag corresponding to start tag \"blockquote\""}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer srv.Close()

	tg := &Telegram{Token: "bot-token", HTTP: srv.Client(), BaseURL: srv.URL}
	html := "<blockquote expandable>hidden</blockquote>"
	if err := tg.SendWith(context.Background(), "-100", nil, html, SendExtras{}); err != nil {
		t.Fatalf("must degrade and retry rather than fail outright: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("want a probe then a degraded retry, got %d calls", len(bodies))
	}
	if strings.Contains(bodies[1], "blockquote expandable") {
		t.Fatalf("the retry must use the degraded form, not repeat the refused tag: %s", bodies[1])
	}
	if expandableQuotes.Load() {
		t.Fatal("the refusal must latch the feature off for the rest of the process")
	}
}

// ---- bucket.doSleep on the REAL clock ---------------------------------------
//
// Every limiter_test.go case injects b.sleep, so the real select{} in
// doSleep (used whenever nobody overrides it — i.e. in production) had never
// run at all.

func TestBucketDoSleepOnTheRealClockHonoursCancellation(t *testing.T) {
	b := newBucket(1, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if b.doSleep(ctx, time.Second) {
		t.Fatal("a cancelled context must return false rather than sleeping")
	}
}

func TestBucketDoSleepOnTheRealClockWaitsOut(t *testing.T) {
	b := newBucket(1, 1)
	start := time.Now()
	if !b.doSleep(context.Background(), 5*time.Millisecond) {
		t.Fatal("an un-cancelled wait must report success")
	}
	if time.Since(start) < 5*time.Millisecond {
		t.Fatal("doSleep must actually wait out the duration on the real clock")
	}
}

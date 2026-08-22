package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- the transport-local spelling ---------------------------------------

// INJECTIVE BY CONSTRUCTION. A Kubernetes object name is a DNS-1123 subdomain
// and cannot contain an underscore, so `-` -> `_` introduces a character no
// published name can hold and `_` -> `-` reverses it exactly. Two Pipelines can
// therefore never share a spelling, and signal-telegram can reverse this with
// one line and no shared state — which it must, because the menu-completed
// command arrives THERE.
func TestSpellForTelegram(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"k8s-observe", "k8s_observe", true},
		{"pipelines", "pipelines", true},
		{"alert-investigator", "alert_investigator", true},
		// A dot is legal in a Kubernetes name and illegal in a Telegram command.
		// Not registered — still typable, and nothing is refused over it.
		{"team.ops", "", false},
		{strings.Repeat("a", 33), "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := spellForTelegram(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("spellForTelegram(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func vocab() Vocabulary {
	return Vocabulary{
		Revision: "rev-1",
		Entries: []VocabularyEntry{
			{Kind: "builtin", Name: "pipelines", Description: "List the pipelines you can address", Position: "general"},
			{Kind: "builtin", Name: "exit", Description: "Release this conversation's runtime", Position: "thread"},
			{Kind: "pipeline", Name: "k8s-observe", Description: "k8s-engineer", Position: "general", Profile: "k8s-engineer", Icon: "🔎"},
			{Kind: "pipeline", Name: "team.ops", Description: "nobody", Position: "general"},
		},
	}
}

// BOTH KINDS ARE REGISTERED. A menu listing only the built-ins would teach that
// Pipelines are not something you type.
func TestCommandsForRegistersBuiltinsAndPipelines(t *testing.T) {
	got := map[string]string{}
	for _, c := range commandsFor(vocab()) {
		got[c.Command] = c.Description
	}
	for _, want := range []string{"pipelines", "exit", "k8s_observe"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("%q missing from the menu: %v", want, got)
		}
	}
	// The icon LEADS the description: a Telegram command name takes no emoji.
	if got["k8s_observe"] != "🔎 k8s-engineer" {
		t.Fatalf("pipeline description = %q, want the icon leading it", got["k8s_observe"])
	}
	// Telegram cannot express a dot, so it is left out — not renamed, not
	// refused, not reported.
	for name := range got {
		if strings.Contains(name, ".") {
			t.Fatalf("registered a name Telegram rejects: %q", name)
		}
	}
	if _, ok := got["team.ops"]; ok {
		t.Fatal("dotted name registered")
	}
}

// Telegram's position scope is a CHAT, which spans a forum's general surface
// and its topics, so both positions are registered and the manager's usage
// replies correct a command used in the wrong place.
func TestBothPositionsAreRegistered(t *testing.T) {
	var general, thread bool
	for _, c := range commandsFor(vocab()) {
		switch c.Command {
		case "pipelines":
			general = true
		case "exit":
			thread = true
		}
	}
	if !general || !thread {
		t.Fatalf("union not registered: general=%v thread=%v", general, thread)
	}
}

func TestOverflowTruncatesRatherThanFails(t *testing.T) {
	v := Vocabulary{Revision: "r"}
	for i := 0; i < telegramMaxCommands+20; i++ {
		v.Entries = append(v.Entries, VocabularyEntry{
			Kind: "pipeline", Name: "p" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + itoa(i),
			Description: "d", Position: "general",
		})
	}
	if n := len(commandsFor(v)); n > telegramMaxCommands {
		t.Fatalf("registered %d commands, Telegram caps at %d", n, telegramMaxCommands)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// ---- re-registration ------------------------------------------------------

// ASKING IS NOT RECORDING. `stale` is a pure question and `synced` is the
// answer, and the split is not stylistic: recording inside the question is what
// made a live cluster register nothing. A poll that arrived while the adapter
// had no channels yet consumed the revision and never came back to it.
func TestStaleAsksAndSyncedRecords(t *testing.T) {
	m := newMenu()
	if !m.stale("rev-1") {
		t.Fatal("first revision should be stale")
	}
	// Still stale: nothing has been done about it yet.
	if !m.stale("rev-1") {
		t.Fatal("asking consumed the revision — a run that did nothing would never retry")
	}
	m.synced("rev-1")
	if m.stale("rev-1") {
		t.Fatal("a synced revision is still reported stale")
	}
	if !m.stale("rev-2") {
		t.Fatal("changed revision not noticed")
	}
	// A manager that predates the vocabulary reports nothing, and nothing
	// happens — this is additive.
	if m.stale("") {
		t.Fatal("absent revision treated as a change")
	}
}

// A vocabulary change Telegram cannot express leaves the ADAPTED list identical,
// so no registration call is made.
func TestInconsequentialChangeLeavesTheAdaptedListIdentical(t *testing.T) {
	before := commandFingerprint(commandsFor(vocab()))
	v := vocab()
	v.Revision = "rev-2"
	v.Entries = append(v.Entries, VocabularyEntry{
		Kind: "pipeline", Name: "another.dotted", Description: "x", Position: "general",
	})
	if after := commandFingerprint(commandsFor(v)); after != before {
		t.Fatal("a name Telegram cannot express changed the registered list")
	}
}

// ---- one spelling per surface --------------------------------------------

func loadedMenu(t *testing.T) *menu {
	t.Helper()
	m := newMenu()
	for _, e := range vocab().Entries {
		if spelled, ok := spellForTelegram(e.Name); ok {
			m.spelling[spelled] = e.Name
		}
	}
	return m
}

// What a listing prints and what the composer completes must be ONE string.
func TestProseNamesPipelinesInTheRegisteredSpelling(t *testing.T) {
	m := loadedMenu(t)
	got := m.rewriteCommands("Address one of them:\n• /k8s-observe &lt;task&gt;\n/pipelines shows this list")
	if !strings.Contains(got, "/k8s_observe") {
		t.Fatalf("pipeline not rendered in the completed spelling: %q", got)
	}
	if !strings.Contains(got, "/pipelines") {
		t.Fatalf("built-in should be unchanged: %q", got)
	}
}

// Only REGISTERED names are touched. Everything else is somebody's text.
func TestRewriteLeavesForeignSlashesAlone(t *testing.T) {
	m := loadedMenu(t)
	for _, s := range []string{
		"the checkout is at /data/workspace",
		"see 2026/08/22 for the incident",
		"https://example.com/some-path",
		"/unknown-pipeline is not registered",
	} {
		if got := m.rewriteCommands(s); got != s {
			t.Errorf("rewrote foreign text: %q -> %q", s, got)
		}
	}
}

// A nil menu is INERT, never a panic: rendering must work for a manager that
// publishes no vocabulary at all.
func TestNilMenuIsInert(t *testing.T) {
	var m *menu
	if got := m.rewriteCommands("/k8s-observe do it"); got != "/k8s-observe do it" {
		t.Fatalf("nil menu rewrote: %q", got)
	}
	if m.stale("rev") {
		t.Fatal("nil menu reported staleness")
	}
	if got := m.publishedName("k8s_observe"); got != "k8s-observe" {
		t.Fatalf("nil menu reverse = %q", got)
	}
}

// ---- offered controls -----------------------------------------------------

// INLINE, attached to the message — never a reply keyboard, which is shown to
// every member of a group and replaces their own composer.
func TestChoicesRenderAsInlineControls(t *testing.T) {
	kb, ok := inlineKeyboard([]Choice{
		{Label: "k8s-observe", Command: "/k8s-observe"},
		{Label: "alert-investigator", Command: "/alert-investigator"},
	}).(map[string]any)
	if !ok {
		t.Fatal("no keyboard rendered")
	}
	if _, isReply := kb["keyboard"]; isReply {
		t.Fatal("rendered a chat-wide reply keyboard")
	}
	rows, _ := kb["inline_keyboard"].([][]map[string]string)
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0][0]["callback_data"] != "p:k8s-observe" {
		t.Fatalf("callback data = %q — it carries the REAL name", rows[0][0]["callback_data"])
	}
}

func TestNoChoicesMeansNoKeyboard(t *testing.T) {
	if kb := inlineKeyboard(nil); kb != nil {
		t.Fatalf("empty choices produced %v", kb)
	}
}

// A choice whose payload will not fit is left out of the KEYBOARD only — the
// manager's prose still names it and its addressed form, which is the same
// fallback a transport with no controls gets.
func TestOversizedChoiceFallsBackToProse(t *testing.T) {
	long := strings.Repeat("a", telegramMaxCallbackData)
	kb := inlineKeyboard([]Choice{{Label: long, Command: "/" + long}})
	if kb != nil {
		t.Fatalf("oversized callback data was sent: %v", kb)
	}
}

// ---- the reply linkage ----------------------------------------------------

// The handle is OPAQUE and round-trips: this adapter supplied it, the manager
// stored it unaltered, and it comes back as the message to answer.
func TestSendUsesTheReplyHandleWhenPresent(t *testing.T) {
	var body map[string]any
	srv := botAPI(t, &body)
	defer srv.Close()
	tg := &Telegram{Token: "t", HTTP: srv.Client(), BaseURL: srv.URL}

	if err := tg.SendWith(context.Background(), "-100", nil, "hi", SendExtras{ReplyTo: "314"}); err != nil {
		t.Fatal(err)
	}
	rp, ok := body["reply_parameters"].(map[string]any)
	if !ok {
		t.Fatalf("no reply linkage sent: %v", body)
	}
	if rp["message_id"] != float64(314) {
		t.Fatalf("message_id = %v", rp["message_id"])
	}
	// The original may have been deleted. Losing the linkage is better than
	// losing the message.
	if rp["allow_sending_without_reply"] != true {
		t.Fatalf("a deleted original would drop the message: %v", rp)
	}
}

func TestSendOmitsTheLinkageWhenAbsent(t *testing.T) {
	var body map[string]any
	srv := botAPI(t, &body)
	defer srv.Close()
	tg := &Telegram{Token: "t", HTTP: srv.Client(), BaseURL: srv.URL}

	if err := tg.SendWith(context.Background(), "-100", nil, "hi", SendExtras{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["reply_parameters"]; ok {
		t.Fatalf("linkage invented from nothing: %v", body)
	}
	if _, ok := body["reply_markup"]; ok {
		t.Fatalf("keyboard invented from nothing: %v", body)
	}
}

// Registration is scoped to the CHAT, so the bot claims no command vocabulary
// in chats it does not serve.
func TestSetCommandsIsScopedToTheChat(t *testing.T) {
	var body map[string]any
	srv := botAPI(t, &body)
	defer srv.Close()
	tg := &Telegram{Token: "t", HTTP: srv.Client(), BaseURL: srv.URL}

	if err := tg.SetCommands(context.Background(), "-100", []BotCommand{{Command: "pipelines", Description: "d"}}); err != nil {
		t.Fatal(err)
	}
	scope, ok := body["scope"].(map[string]any)
	if !ok || scope["type"] != "chat" || scope["chat_id"] != "-100" {
		t.Fatalf("scope = %v, want this chat only", body["scope"])
	}
}

// A SELECTION on the general surface belongs to signal-telegram — origination
// is the signal path's, and this adapter must never start a conversation. A
// forwarded callback is therefore inert here.
func TestForwardedSelectionIsInertInTheChannelAdapter(t *testing.T) {
	a := &adapter{channels: map[string]servedChannel{}, reported: map[string]string{}, clients: map[string]*Telegram{}}
	var upd tgUpdate
	mustJSON(t, `{"update_id":30,"callback_query":{"id":"cb-1","data":"p:k8s-observe",
		"message":{"message_id":9,"chat":{"id":-100}}}}`, &upd)
	// No panic, no inbound, nothing started.
	a.dispatch(context.Background(), upd)
}

func botAPI(t *testing.T, body *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
}

func mustJSON(t *testing.T, raw string, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		t.Fatal(err)
	}
}

// PIPELINES FIRST. Telegram lists commands in the order given, and typing `/`
// is overwhelmingly somebody reaching for an agent — the manager's own commands
// are the rarer errand.
func TestPipelinesAreListedBeforeBuiltins(t *testing.T) {
	cmds := commandsFor(vocab())
	var firstBuiltin, lastPipeline = -1, -1
	for i, c := range cmds {
		switch c.Command {
		case "pipelines", "exit", "help", "close":
			if firstBuiltin < 0 {
				firstBuiltin = i
			}
		default:
			lastPipeline = i
		}
	}
	if firstBuiltin < 0 || lastPipeline < 0 {
		t.Fatalf("expected both kinds in the menu: %+v", cmds)
	}
	if lastPipeline > firstBuiltin {
		t.Fatalf("a builtin was listed before a pipeline: %+v", cmds)
	}
}

// An entry with no icon is rendered without one — nothing is invented, and the
// description is not left with a leading space.
func TestMissingIconLeavesTheDescriptionAlone(t *testing.T) {
	v := Vocabulary{Revision: "r", Entries: []VocabularyEntry{
		{Kind: "pipeline", Name: "plain", Description: "some-profile", Position: "general"},
	}}
	if got := commandsFor(v)[0].Description; got != "some-profile" {
		t.Fatalf("description = %q", got)
	}
}

// ---- syncCommands, end to end -------------------------------------------
//
// The pieces were each tested and the whole was not, and the whole is what
// failed: registration silently did nothing on a live cluster while every unit
// test passed. This drives the real path — a manager serving a vocabulary, a
// served channel, and a Bot API recording what it was told.

func TestSyncCommandsRegistersForEveryServedChat(t *testing.T) {
	var setCalls []map[string]any
	bot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		if strings.HasSuffix(r.URL.Path, "/setMyCommands") {
			setCalls = append(setCalls, body)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer bot.Close()

	mgr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/channel/vocabulary" {
			_ = json.NewEncoder(w).Encode(vocab())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer mgr.Close()

	a := &adapter{
		mgr:      NewManager(mgr.URL, "tok"),
		menu:     newMenu(),
		channels: map[string]servedChannel{},
		reported: map[string]string{},
		clients:  map[string]*Telegram{},
	}
	a.channels["home-ops"] = servedChannel{cfg: channelConfig{ChatID: "-100123"}, token: "bot-token"}
	a.clients["bot-token"] = &Telegram{Token: "bot-token", HTTP: bot.Client(), BaseURL: bot.URL}

	a.syncCommands(context.Background(), "rev-1")

	if len(setCalls) != 1 {
		t.Fatalf("setMyCommands called %d time(s), want 1", len(setCalls))
	}
	cmds, _ := setCalls[0]["commands"].([]any)
	if len(cmds) == 0 {
		t.Fatalf("registered an EMPTY command list — that CLEARS the menu: %v", setCalls[0])
	}
	scope, _ := setCalls[0]["scope"].(map[string]any)
	if scope["chat_id"] != "-100123" {
		t.Fatalf("scope = %v", scope)
	}

	// Same revision again: no second call, because registration is rate-limited.
	a.syncCommands(context.Background(), "rev-1")
	if len(setCalls) != 1 {
		t.Fatalf("re-registered an unchanged list: %d calls", len(setCalls))
	}
}

// A revision consumed by a run that did NOTHING must not stop the next attempt.
// stale() records the revision before the work happens, so a fetch that fails —
// or a moment with no channels yet — would otherwise never be retried.
func TestSyncCommandsRetriesAfterAFailedRun(t *testing.T) {
	fail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer fail.Close()

	a := &adapter{
		mgr: NewManager(fail.URL, "tok"), menu: newMenu(),
		channels: map[string]servedChannel{}, reported: map[string]string{}, clients: map[string]*Telegram{},
	}
	a.syncCommands(context.Background(), "rev-1")
	if !a.menu.stale("rev-1") {
		t.Fatal("a failed run consumed the revision — the next poll will skip it and the menu never registers")
	}
}

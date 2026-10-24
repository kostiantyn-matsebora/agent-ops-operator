//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The channel adapter conformance set, run against every channel adapter the
// repository ships. A new adapter joins by being LISTED here — its own source
// is untouched.

// assertContractHandshake: every ops poll declared contract= and waited.
func assertContractHandshake(t *testing.T, mgr *FakeManager, adapter string) {
	t.Helper()
	reqs := mgr.OpsRequests()
	if len(reqs) == 0 {
		t.Fatalf("%s never polled /channel/ops", adapter)
	}
	longPolled := false
	for _, r := range reqs {
		if r.Refused {
			t.Fatalf("%s polled without contract=%s — it would post empty messages and look healthy", adapter, mgr.ContractVersion)
		}
		if r.Adapter != adapter {
			t.Fatalf("%s polled as adapter=%q", adapter, r.Adapter)
		}
		if r.Wait > 0 {
			longPolled = true
		}
	}
	if !longPolled {
		t.Fatalf("%s never long-polled (wait=0 on every request)", adapter)
	}
}

// completionsFor counts the completions of one op id and returns the last.
func completionsFor(mgr *FakeManager, id string) (int, Completion) {
	n, last := 0, Completion{}
	for _, c := range mgr.Completions() {
		if c.ID == id {
			n++
			last = c
		}
	}
	return n, last
}

// ---- channel-telegram ------------------------------------------------------

func TestChannelTelegramConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	bot := tgStartFakeBotAPI(t)
	p := tgStartAdapterWithTwoChannels(t, mgr, bot)

	tgAssertBrokenChannelReportedGoodChannelStaysUp(t, mgr, p)

	topic := tgEnsureTopicAndAssertContract(t, mgr, bot)
	send := tgSendAndAssertRendered(t, mgr, bot, topic)
	tgAssertRedeliveredSendNotReposted(t, mgr, bot, send)
	assertRetriesAtOnceAfterNonLeaderRejection(t, mgr, "ops", "c1")
	tgAssertInboundPushForwarded(t, mgr, p)
	tgAssertNoRelayLoopOrUnauthenticated(t, mgr)
	tgAssertCloseTopicIdempotent(t, mgr, topic)
	tgAssertThreadlessSendVisibleOrError(t, mgr, bot)
	tgAssertDeleteConversationActsOrErrors(t, mgr, bot)
	tgAssertRetryBudgetStaysUnderReclaimWindow(t, mgr, bot)
}

// tgStartFakeBotAPI starts the Bot API double as a BINARY too — the same one
// the system pack deploys.
func tgStartFakeBotAPI(t *testing.T) *Process {
	t.Helper()
	botPort := freePort(t)
	bot := start(t, "fake-bot-api", build(t, "test/fakebotapi"), []string{fmt.Sprintf("LISTEN_ADDR=127.0.0.1:%d", botPort)})
	bot.Port = botPort
	waitHealthy(t, bot)
	return bot
}

// botCallsFor lists the Bot API double's recorded calls to method.
func botCallsFor(t *testing.T, bot *Process, method string) []map[string]any {
	t.Helper()
	resp, err := http.Get(bot.URL() + "/control/calls?method=" + method)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var calls []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&calls)
	return calls
}

// botQueueFail arms the Bot API double to reject the NEXT call to method with
// a real 429 (carrying retryAfter) or a bare 5xx, exactly once.
func botQueueFail(t *testing.T, bot *Process, method, mode string, retryAfter int) {
	t.Helper()
	botFail(t, bot, method, mode, retryAfter, false)
}

// botFailForever arms the double to reject EVERY call to method until
// botClearFail — backpressure that never clears (A1/A2's "unending" case).
func botFailForever(t *testing.T, bot *Process, method, mode string, retryAfter int) {
	t.Helper()
	botFail(t, bot, method, mode, retryAfter, true)
}

func botFail(t *testing.T, bot *Process, method, mode string, retryAfter int, forever bool) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"method": method, "mode": mode, "retryAfter": retryAfter, "forever": forever})
	resp, err := http.Post(bot.URL()+"/control/fail", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// botClearFail disarms any injected failure for method.
func botClearFail(t *testing.T, bot *Process, method string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, bot.URL()+"/control/fail?method="+method, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// tgStartGateway starts gateway-telegram as a BINARY, the same one the
// system pack and the chart deploy — chat-ops-broker-telegram-gateway made it
// the sole holder of the bot credential, so channel-telegram no longer talks
// to `bot` directly; it relays every Bot API call through this process'
// POST /telegram/call/{method} instead.
//
// SIGNAL_TARGET is unused by this suite (nothing exercises getUpdates/
// origination here) but is required at startup, so it points at a port
// nothing serves — the gateway never calls it unless an update actually
// arrives from `bot`, which this test never sends. CHANNEL_TARGET is the
// real channel-telegram URL, since a reopen/offset round trip would need it
// if this suite ever grows one.
func tgStartGateway(t *testing.T, bot *Process, channelTarget string, callToken string) *Process {
	t.Helper()
	port := freePort(t)
	env := []string{
		"TELEGRAM_API_BASE=" + bot.URL(),
		"TELEGRAM_BOT_TOKEN=123:gateway-token",
		"SIGNAL_TARGET=http://127.0.0.1:" + fmt.Sprint(freePort(t)),
		"CHANNEL_TARGET=" + channelTarget,
		"TELEGRAM_CALL_TOKEN=" + callToken,
		fmt.Sprintf("LISTEN_ADDR=127.0.0.1:%d", port),
	}
	g := start(t, "gateway-telegram", build(t, "gateways/telegram"), env)
	g.Port = port
	// No /healthz on this process (it serves exactly one route, POST
	// /telegram/call/{method}) — waitHealthy would hang forever. A dial is
	// enough: the call server and the getUpdates poll loop start together in
	// main(), so a listening port means the call route is live too.
	waitFor(t, g.Name+" to listen", 10*time.Second, func() bool {
		if g.Exited() {
			t.Fatalf("%s exited before it was listening:\n%s", g.Name, g.Output())
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	})
	return g
}

// tgStartAdapterWithTwoChannels serves one good surface with a projected
// credential, and one whose config the adapter cannot accept. Both channels'
// credentials now carry a GATEWAY binding (gatewayUrl/gatewayToken), never a
// bot token — channel-telegram holds none, see tgStartGateway.
func tgStartAdapterWithTwoChannels(t *testing.T, mgr *FakeManager, bot *Process) *Process {
	t.Helper()
	mgr.ServeChannels(
		ChannelInfo{Name: "ops", Config: json.RawMessage(`{"chatId":"-1001234567890"}`), CredentialEnvPrefix: "AGENTOPS_CRED_OPS_"},
		ChannelInfo{Name: "broken", Config: json.RawMessage(`{"feedThreadId":1}`), CredentialEnvPrefix: "AGENTOPS_CRED_BROKEN_"},
	)
	port := freePort(t)
	channelURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	const callToken = "gateway-call-token"
	gw := tgStartGateway(t, bot, channelURL, callToken)
	env := append(contractEnv(mgr, "telegram", port),
		"AGENTOPS_CRED_OPS_gatewayUrl="+gw.URL(),
		"AGENTOPS_CRED_OPS_gatewayToken="+callToken,
		"AGENTOPS_CRED_BROKEN_gatewayUrl="+gw.URL(),
		"AGENTOPS_CRED_BROKEN_gatewayToken="+callToken,
	)
	p := start(t, "channel-telegram", build(t, "channels/telegram"), env)
	p.Port = port
	waitHealthy(t, p)
	waitFor(t, "the adapter to list its channels", 10*time.Second, func() bool { return mgr.Listed("channel", "telegram") > 0 })
	return p
}

// tgAssertBrokenChannelReportedGoodChannelStaysUp: listing and status — the
// broken surface is reported, the good one served.
func tgAssertBrokenChannelReportedGoodChannelStaysUp(t *testing.T, mgr *FakeManager, p *Process) {
	t.Helper()
	waitFor(t, "a status report for the broken channel", 10*time.Second, func() bool {
		for _, s := range mgr.StatusReports() {
			if s.Name == "broken" && !s.Ready {
				return true
			}
		}
		return false
	})
	if p.Exited() {
		t.Fatalf("an invalid channel config must not be fatal:\n%s", p.Output())
	}
}

// tgEnsureTopicAndAssertContract: long-poll, contract, typed messages —
// ensure-topic and the handshake it rides on.
func tgEnsureTopicAndAssertContract(t *testing.T, mgr *FakeManager, bot *Process) Completion {
	t.Helper()
	mgr.QueueOp(map[string]any{"id": "op-topic", "channel": "ops", "conversation": "c1", "kind": "ensure-topic",
		"topic": map[string]any{"conversation": "c1", "title": "disk is full", "kind": "alert", "source": "vm-alerts"}})
	waitFor(t, "ensure-topic completion", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-topic"); return n > 0 })
	_, topic := completionsFor(mgr, "op-topic")
	if topic.Error != "" || topic.ThreadID == "" {
		t.Fatalf("ensure-topic must complete with a thread id: %+v", topic)
	}
	if len(botCallsFor(t, bot, "createForumTopic")) != 1 {
		t.Fatalf("ensure-topic must create exactly one forum topic, calls=%v", botCallsFor(t, bot, "createForumTopic"))
	}
	assertContractHandshake(t, mgr, "telegram")
	return topic
}

func tgSendAndAssertRendered(t *testing.T, mgr *FakeManager, bot *Process, topic Completion) map[string]any {
	t.Helper()
	send := map[string]any{"id": "op-send", "channel": "ops", "conversation": "c1", "kind": "send", "threadId": topic.ThreadID,
		"message": map[string]any{"kind": "answer", "body": "**found it** — the disk is full", "status": "succeeded"}}
	mgr.QueueOp(send)
	waitFor(t, "send completion", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-send"); return n > 0 })
	if _, c := completionsFor(mgr, "op-send"); c.Error != "" {
		t.Fatalf("send failed: %+v", c)
	}
	sends := botCallsFor(t, bot, "sendMessage")
	if len(sends) != 1 {
		t.Fatalf("one send op must be one sendMessage, got %d", len(sends))
	}
	body, _ := sends[0]["body"].(map[string]any)
	if body["chat_id"] != "-1001234567890" || fmt.Sprint(body["message_thread_id"]) != topic.ThreadID {
		t.Fatalf("sendMessage must target the surface's chat and the op's thread: %v", body)
	}
	if text, _ := body["text"].(string); !strings.Contains(text, "found it") || strings.Contains(text, "**") {
		t.Fatalf("the typed body must be RENDERED from markdown, not posted raw: %q", text)
	}
	// chat-ops-broker-telegram-gateway: channel-telegram no longer holds a
	// bot token at all — it relayed this send through gateway-telegram's
	// POST /telegram/call, and the TOKEN THE BOT API SAW IS THE GATEWAY'S
	// OWN (tgStartGateway's TELEGRAM_BOT_TOKEN), never a per-channel one.
	// That is the whole point of the consolidation: one credential, one
	// holder, for every channel sharing this bot.
	if sends[0]["token"] != "123:gateway-token" {
		t.Fatalf("the send must be relayed through the gateway's own credential, got %v", sends[0]["token"])
	}
	return send
}

// tgAssertRedeliveredSendNotReposted: at-least-once — the SAME op id again
// is acknowledged, not repeated.
func tgAssertRedeliveredSendNotReposted(t *testing.T, mgr *FakeManager, bot *Process, send map[string]any) {
	t.Helper()
	mgr.QueueOp(send)
	waitFor(t, "second completion of op-send", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-send"); return n >= 2 })
	if got := len(botCallsFor(t, bot, "sendMessage")); got != 1 {
		t.Fatalf("a redelivered op must not post again: %d sendMessage calls", got)
	}
}

// assertRetriesAtOnceAfterNonLeaderRejection: durable-chat-ops-broker — a
// poll rejected with 503 (not the current leader) is treated as "nothing to
// do right now" and retried, never as a failure. Shared by every adapter's
// conformance suite, since the behavior is the same contract on every one.
//
// This is the black-box half, proving the WIRING end to end (a real 503 off
// the wire reaches a running adapter and it keeps polling rather than
// wedging or erroring out); the deterministic, UNIT-level proof that no
// artificial backoff fires belongs to each adapter's own fast tests
// (errorpaths_test.go's TestPollOnceDoesNotSleepOnANonLeaderRejection here,
// and platform/console's own), since a black-box timing assertion at this
// layer would also have to account for an adapter's own, unrelated pacing
// (telegram's send-rate budget) and would mostly measure that instead.
func assertRetriesAtOnceAfterNonLeaderRejection(t *testing.T, mgr *FakeManager, channel, conversation string) {
	t.Helper()
	before := len(mgr.OpsRequests())
	mgr.RejectOpsAsNonLeader(1 << 20)
	waitFor(t, "at least one non-leader rejection", 30*time.Second,
		func() bool { return len(mgr.OpsRequests())-before >= 1 })

	mgr.RejectOpsAsNonLeader(0)
	mgr.QueueOp(map[string]any{"id": "op-nonleader", "channel": channel, "conversation": conversation, "kind": "close-topic",
		"threadId": "whatever"})
	waitFor(t, "completion once this replica answers as leader again", 30*time.Second,
		func() bool { n, _ := completionsFor(mgr, "op-nonleader"); return n > 0 })
}

// tgAssertInboundPushForwarded: inbound push with threadId — the router's
// forwarded topic update.
func tgAssertInboundPushForwarded(t *testing.T, mgr *FakeManager, p *Process) {
	t.Helper()
	before := len(mgr.Inbound())
	code, resp := postJSON(t, p.URL()+"/updates", fixture(t, "telegram-update-topic.json"), nil)
	if code/100 != 2 {
		t.Fatalf("POST /updates: %d %s", code, resp)
	}
	waitFor(t, "inbound push", 10*time.Second, func() bool { return len(mgr.Inbound()) > before })
	in := mgr.Inbound()[len(mgr.Inbound())-1]
	if in["threadId"] != "42" || in["channel"] != "ops" || !strings.Contains(fmt.Sprint(in["text"]), "memory") {
		t.Fatalf("inbound must carry the thread, the channel and the text: %v", in)
	}
}

// tgAssertNoRelayLoopOrUnauthenticated: no relay loop — the outbound posts
// never came back as inbound — and no unauthenticated request reached the
// manager.
func tgAssertNoRelayLoopOrUnauthenticated(t *testing.T, mgr *FakeManager) {
	t.Helper()
	time.Sleep(2 * time.Second)
	for _, in := range mgr.Inbound() {
		if strings.Contains(fmt.Sprint(in["text"]), "found it") {
			t.Fatalf("an outbound post returned as inbound: %v", in)
		}
	}
	if mgr.Unauthorized() != 0 {
		t.Fatalf("%d unauthenticated requests reached the manager", mgr.Unauthorized())
	}
}

// tgAssertCloseTopicIdempotent: close-topic completes with an empty body,
// and a redelivery is not an error.
func tgAssertCloseTopicIdempotent(t *testing.T, mgr *FakeManager, topic Completion) {
	t.Helper()
	close := map[string]any{"id": "op-close", "channel": "ops", "conversation": "c1", "kind": "close-topic", "threadId": topic.ThreadID}
	mgr.QueueOp(close)
	mgr.QueueOp(close)
	waitFor(t, "close-topic completions", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-close"); return n >= 2 })
	if _, c := completionsFor(mgr, "op-close"); c.Error != "" {
		t.Fatalf("close-topic: %+v", c)
	}
}

// tgAssertThreadlessSendVisibleOrError: a `send` with no threadId at all must
// either surface somewhere a person can see it or fail outright — never
// complete successfully with nothing to show for it (B1). Telegram's visible
// fallback is the chat's GENERAL surface: SendWith with a nil thread id omits
// message_thread_id, which Telegram (and this fake) route to General.
func tgAssertThreadlessSendVisibleOrError(t *testing.T, mgr *FakeManager, bot *Process) {
	t.Helper()
	before := len(botCallsFor(t, bot, "sendMessage"))
	send := map[string]any{"id": "op-send-nothread", "channel": "ops", "conversation": "c1", "kind": "send",
		"message": map[string]any{"kind": "answer", "body": "general surface fallback", "status": "succeeded"}}
	mgr.QueueOp(send)
	waitFor(t, "thread-less send completion", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-send-nothread"); return n > 0 })
	_, c := completionsFor(mgr, "op-send-nothread")
	if c.Error != "" {
		return // scenario 2: an explicit error is an accepted outcome too.
	}
	// scenario 1: it must be VISIBLE — a new sendMessage with no thread id.
	sends := botCallsFor(t, bot, "sendMessage")
	if len(sends) <= before {
		t.Fatalf("a thread-less send completed successfully with no sendMessage call at all")
	}
	body, _ := sends[len(sends)-1]["body"].(map[string]any)
	if _, hasThread := body["message_thread_id"]; hasThread {
		t.Fatalf("a thread-less send must land on the GENERAL surface (no message_thread_id), got %v", body)
	}
	if !strings.Contains(fmt.Sprint(body["text"]), "general surface fallback") {
		t.Fatalf("the general-surface send must carry the op's own message: %v", body)
	}
}

// tgAssertDeleteConversationActsOrErrors pins B3 in both reachable states: a
// thread the adapter holds is actually closed, and a missing thread id never
// completes as a silent success.
func tgAssertDeleteConversationActsOrErrors(t *testing.T, mgr *FakeManager, bot *Process) {
	t.Helper()
	// Cannot act: no threadId at all.
	missing := map[string]any{"id": "op-del-missing", "channel": "ops", "conversation": "c1", "kind": "delete-conversation"}
	mgr.QueueOp(missing)
	waitFor(t, "delete-conversation without a thread id completes", 20*time.Second,
		func() bool { n, _ := completionsFor(mgr, "op-del-missing"); return n > 0 })
	if _, c := completionsFor(mgr, "op-del-missing"); c.Error == "" {
		t.Fatalf("delete-conversation with no thread id must not silently succeed")
	}

	// Held: a fresh topic this run created, named for this assertion so it
	// never collides with the topic the rest of the suite is still using.
	mgr.QueueOp(map[string]any{"id": "op-topic-del", "channel": "ops", "conversation": "c-del", "kind": "ensure-topic",
		"topic": map[string]any{"conversation": "c-del", "title": "delete-conversation probe", "kind": "task"}})
	waitFor(t, "probe topic created", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-topic-del"); return n > 0 })
	_, topic := completionsFor(mgr, "op-topic-del")
	if topic.Error != "" || topic.ThreadID == "" {
		t.Fatalf("probe ensure-topic: %+v", topic)
	}
	before := len(botCallsFor(t, bot, "closeForumTopic"))
	held := map[string]any{"id": "op-del-held", "channel": "ops", "conversation": "c-del", "kind": "delete-conversation",
		"threadId": topic.ThreadID}
	mgr.QueueOp(held)
	waitFor(t, "delete-conversation for a held thread completes", 20*time.Second,
		func() bool { n, _ := completionsFor(mgr, "op-del-held"); return n > 0 })
	if _, c := completionsFor(mgr, "op-del-held"); c.Error != "" {
		t.Fatalf("delete-conversation for a thread the adapter holds must act, not error: %+v", c)
	}
	if got := len(botCallsFor(t, bot, "closeForumTopic")); got <= before {
		t.Fatalf("delete-conversation for a held thread left no observable transport effect: closeForumTopic calls %d -> %d", before, got)
	}
}

// tgAssertRetryBudgetStaysUnderReclaimWindow pins A1/A2: injected 429/5xx
// backpressure is absorbed in-process, but the operation never spends more
// than the claim window doing it. Run LAST — it reshapes the fake manager's
// claim window and the bot double's failure queue for the rest of the test.
func tgAssertRetryBudgetStaysUnderReclaimWindow(t *testing.T, mgr *FakeManager, bot *Process) {
	t.Helper()
	// budget = reclaimSeconds/2 = 6s (Op.RetryBudget) — generous above the ~3s
	// the two scripted absorbed failures actually cost (1s retry_after plus
	// one transientBackoff), so ordinary scheduling jitter can never flip a
	// should-succeed retry into a budget failure.
	const reclaimSeconds = 12
	mgr.SetReclaimAfterSeconds(reclaimSeconds)

	// A fresh topic, so this assertion's sendMessage calls are unambiguous.
	mgr.QueueOp(map[string]any{"id": "op-topic-retry", "channel": "ops", "conversation": "c-retry", "kind": "ensure-topic",
		"topic": map[string]any{"conversation": "c-retry", "title": "retry budget probe", "kind": "task"}})
	waitFor(t, "probe topic created", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-topic-retry"); return n > 0 })
	_, topic := completionsFor(mgr, "op-topic-retry")
	if topic.Error != "" || topic.ThreadID == "" {
		t.Fatalf("probe ensure-topic: %+v", topic)
	}

	// Scenario 1: backpressure that CLEARS — a 429 then a bare 5xx, each
	// absorbed, then the real answer. Completes inside the window.
	botQueueFail(t, bot, "sendMessage", "429", 1)
	botQueueFail(t, bot, "sendMessage", "5xx", 0)
	start := time.Now()
	absorbed := map[string]any{"id": "op-send-absorbed", "channel": "ops", "conversation": "c-retry", "kind": "send", "threadId": topic.ThreadID,
		"message": map[string]any{"kind": "answer", "body": "absorbed backpressure", "status": "succeeded"}}
	mgr.QueueOp(absorbed)
	waitFor(t, "absorbed-backpressure send completes", 20*time.Second,
		func() bool { n, _ := completionsFor(mgr, "op-send-absorbed"); return n > 0 })
	elapsed := time.Since(start)
	if _, c := completionsFor(mgr, "op-send-absorbed"); c.Error != "" {
		t.Fatalf("absorbed 429/5xx backpressure must still complete the op: %+v", c)
	}
	if elapsed >= reclaimSeconds*time.Second {
		t.Fatalf("absorbed-backpressure send took %s, must stay strictly below the %ds claim window", elapsed, reclaimSeconds)
	}

	// Scenario 2: backpressure that NEVER clears. The op must be abandoned
	// (failed) comfortably before the claim window ends, not silently ride
	// past it.
	botFailForever(t, bot, "sendMessage", "429", 1)
	t.Cleanup(func() { botClearFail(t, bot, "sendMessage") })
	start = time.Now()
	unending := map[string]any{"id": "op-send-unending", "channel": "ops", "conversation": "c-retry", "kind": "send", "threadId": topic.ThreadID,
		"message": map[string]any{"kind": "answer", "body": "unending backpressure", "status": "succeeded"}}
	mgr.QueueOp(unending)
	waitFor(t, "unending-backpressure send gives up", 20*time.Second,
		func() bool { n, _ := completionsFor(mgr, "op-send-unending"); return n > 0 })
	elapsed = time.Since(start)
	if _, c := completionsFor(mgr, "op-send-unending"); c.Error == "" {
		t.Fatalf("unending 429 backpressure must fail the op, never silently exceed the claim window")
	}
	if elapsed >= reclaimSeconds*time.Second {
		t.Fatalf("unending-backpressure send took %s to give up, must stay strictly below the %ds claim window", elapsed, reclaimSeconds)
	}
}

// ---- console -----------------------------------------------------------------

func TestConsoleConformance(t *testing.T) {
	mgr := NewFakeManager(t, "adapter-token")
	api := NewFakeAPIServer(t)
	convPath := "/apis/agentops.dev/v1alpha1/namespaces/agent-ops/conversations"
	api.Seed(convPath, map[string]any{
		"apiVersion": "agentops.dev/v1alpha1", "kind": "Conversation",
		"metadata": map[string]any{"name": "c1", "namespace": "agent-ops", "uid": "uid-c1"},
		"spec":     map[string]any{"channelRefs": []any{map[string]any{"name": "console"}}},
		"status":   map[string]any{"phase": "Idle"},
	})
	mgr.ServeChannels(ChannelInfo{Name: "console",
		Config:              json.RawMessage(`{"authEnabled":true,"writeEnabled":true,"signalSource":"console"}`),
		CredentialEnvPrefix: "AGENTOPS_CRED_CONSOLE_"})

	port := freePort(t)
	env := append(contractEnv(mgr, "console", port), api.Env()...)
	env = append(env,
		"SIGNAL_ADAPTER_TOKEN=signal-token",
		"UI_DIR="+t.TempDir(),
		"AGENTOPS_CRED_CONSOLE_uiToken=ui-secret",
	)
	// -tags dev serves the SPA from disk instead of embedding ui/dist, so no
	// npm build stands between the suite and the adapter under test.
	p := start(t, "console", build(t, "platform/console", "dev"), env)
	p.Port = port
	waitHealthy(t, p)
	waitFor(t, "the console to list its channels", 15*time.Second, func() bool { return mgr.Listed("channel", "console") > 0 })

	// 1–3. ensure-topic yields a thread id; a send lands in the transcript.
	mgr.QueueOp(map[string]any{"id": "op-topic", "channel": "console", "conversation": "c1", "kind": "ensure-topic",
		"topic": map[string]any{"conversation": "c1", "title": "disk is full", "kind": "alert"}})
	waitFor(t, "ensure-topic completion", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-topic"); return n > 0 })
	_, topic := completionsFor(mgr, "op-topic")
	if topic.Error != "" || topic.ThreadID == "" {
		t.Fatalf("ensure-topic: %+v", topic)
	}
	assertContractHandshake(t, mgr, "console")
	// Bind the thread on the conversation, as the manager would.
	api.Push(convPath, "MODIFIED", map[string]any{
		"apiVersion": "agentops.dev/v1alpha1", "kind": "Conversation",
		"metadata": map[string]any{"name": "c1", "namespace": "agent-ops", "uid": "uid-c1"},
		"spec":     map[string]any{"channelRefs": []any{map[string]any{"name": "console"}}},
		"status": map[string]any{"phase": "Idle",
			"threads": []any{map[string]any{"channel": "console", "threadId": topic.ThreadID}}},
	})

	send := map[string]any{"id": "op-send", "channel": "console", "conversation": "c1", "kind": "send", "threadId": topic.ThreadID,
		"message": map[string]any{"kind": "answer", "body": "**found it** — the disk is full", "status": "succeeded"}}
	mgr.QueueOp(send)
	mgr.QueueOp(send) // 4. at-least-once
	waitFor(t, "both send completions", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-send"); return n >= 2 })
	if _, c := completionsFor(mgr, "op-send"); c.Error != "" {
		t.Fatalf("send: %+v", c)
	}
	transcript := func() string {
		req, _ := http.NewRequest("GET", p.URL()+"/api/conversations/c1", nil)
		req.Header.Set("Authorization", "Bearer ui-secret")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		b, _ := json.Marshal(out["transcript"])
		return string(b)
	}
	waitFor(t, "the transcript to show the answer", 10*time.Second, func() bool { return strings.Contains(transcript(), "found it") })
	if strings.Count(transcript(), "found it") != 1 {
		t.Fatalf("a redelivered send must appear once in the transcript:\n%s", transcript())
	}

	// 5. Inbound push through the console's OWN write path, authenticated.
	code, resp := postJSON(t, p.URL()+"/api/conversations/c1/messages", []byte(`{"text":"and the memory?"}`), nil)
	if code != 401 {
		t.Fatalf("an unauthenticated write must be refused, got %d %s", code, resp)
	}
	code, resp = postJSON(t, p.URL()+"/api/conversations/c1/messages", []byte(`{"text":"and the memory?"}`),
		map[string]string{"Authorization": "Bearer ui-secret"})
	if code/100 != 2 {
		t.Fatalf("an authenticated write must be accepted, got %d %s", code, resp)
	}
	waitFor(t, "inbound push", 10*time.Second, func() bool { return len(mgr.Inbound()) > 0 })
	in := mgr.Inbound()[0]
	if in["threadId"] != topic.ThreadID || in["channel"] != "console" || in["text"] != "and the memory?" {
		t.Fatalf("inbound must carry the thread, the channel and the text: %v", in)
	}
	// 7. No relay loop.
	time.Sleep(time.Second)
	if len(mgr.Inbound()) != 1 {
		t.Fatalf("outbound posts must never return as inbound: %v", mgr.Inbound())
	}
	if mgr.Unauthorized() != 0 {
		t.Fatalf("%d unauthenticated requests reached the manager", mgr.Unauthorized())
	}
	consoleAssertThreadlessSendVisibleOrError(t, mgr, p)
	consoleAssertDeleteConversationActsOrErrors(t, mgr, p, topic.ThreadID)
	consoleAssertOperationNeverExceedsReclaimWindow(t, mgr, api, convPath)
	assertRetriesAtOnceAfterNonLeaderRejection(t, mgr, "console", "c1")
}

// consoleReadJSON GETs an authenticated console endpoint and decodes it.
func consoleReadJSON(t *testing.T, p *Process, path string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest("GET", p.URL()+path, nil)
	req.Header.Set("Authorization", "Bearer ui-secret")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

// consoleAssertThreadlessSendVisibleOrError pins B1 for the console: a `send`
// with no threadId either lands on the channel's general feed (design:
// "Thread-less console sends surface as the channel's general feed",
// convapi.go's GET /api/channel/feed) or fails outright — never a success
// with nothing anywhere to show for it.
func consoleAssertThreadlessSendVisibleOrError(t *testing.T, mgr *FakeManager, p *Process) {
	t.Helper()
	send := map[string]any{"id": "op-send-nothread", "channel": "console", "conversation": "c1", "kind": "send",
		"message": map[string]any{"kind": "answer", "body": "general feed fallback", "status": "succeeded"}}
	mgr.QueueOp(send)
	waitFor(t, "thread-less send completion", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-send-nothread"); return n > 0 })
	_, c := completionsFor(mgr, "op-send-nothread")
	if c.Error != "" {
		return // scenario 2: an explicit error is an accepted outcome too.
	}
	// scenario 1: it must be VISIBLE on the channel's general feed.
	out := consoleReadJSON(t, p, "/api/channel/feed")
	b, _ := json.Marshal(out["messages"])
	if !strings.Contains(string(b), "general feed fallback") {
		t.Fatalf("a thread-less send completed successfully but is nowhere visible: feed=%s", b)
	}
}

// consoleAssertDeleteConversationActsOrErrors pins B3 for the console in both
// reachable states: a held thread is archived (an observable transport
// effect: the tombstone message), and a missing thread id never completes as
// a silent success.
func consoleAssertDeleteConversationActsOrErrors(t *testing.T, mgr *FakeManager, p *Process, heldThreadID string) {
	t.Helper()
	// Cannot act: no threadId at all.
	missing := map[string]any{"id": "op-del-missing", "channel": "console", "conversation": "c1", "kind": "delete-conversation"}
	mgr.QueueOp(missing)
	waitFor(t, "delete-conversation without a thread id completes", 20*time.Second,
		func() bool { n, _ := completionsFor(mgr, "op-del-missing"); return n > 0 })
	if _, c := completionsFor(mgr, "op-del-missing"); c.Error == "" {
		t.Fatalf("delete-conversation with no thread id must not silently succeed")
	}

	// Held: the thread established earlier in this run.
	held := map[string]any{"id": "op-del-held", "channel": "console", "conversation": "c1", "kind": "delete-conversation",
		"threadId": heldThreadID}
	mgr.QueueOp(held)
	waitFor(t, "delete-conversation for a held thread completes", 20*time.Second,
		func() bool { n, _ := completionsFor(mgr, "op-del-held"); return n > 0 })
	if _, c := completionsFor(mgr, "op-del-held"); c.Error != "" {
		t.Fatalf("delete-conversation for a thread the adapter holds must act, not error: %+v", c)
	}
	waitFor(t, "the transcript to carry the archive tombstone", 10*time.Second, func() bool {
		b, _ := json.Marshal(consoleReadJSON(t, p, "/api/conversations/c1")["transcript"])
		return strings.Contains(string(b), "archived")
	})
}

// consoleAssertOperationNeverExceedsReclaimWindow pins A1/A2 for the console.
// The console carries no per-operation retry loop of its own (cache.go's
// GetOrFetch makes exactly ONE direct read on a cache miss, see adapter.go's
// threadID) — it ignores reclaimAfterSeconds entirely, which OPS-4/OPS-5
// make fully conformant PROVIDED it still never silently outlives the claim
// window. This drives that one direct read through a sustained 500 — the
// shape backpressure takes for a GET — and pins the outcome: the operation
// completes (as a failure) near-instantly, nowhere close to the window,
// rather than hanging past it.
func consoleAssertOperationNeverExceedsReclaimWindow(t *testing.T, mgr *FakeManager, api *FakeAPIServer, convPath string) {
	t.Helper()
	const conversation = "c-retry-budget"
	path := convPath + "/" + conversation
	api.FailPath(path, http.StatusInternalServerError, -1) // never clears
	defer api.ClearFailPath(path)

	start := time.Now()
	mgr.QueueOp(map[string]any{"id": "op-retry-budget", "channel": "console", "conversation": conversation, "kind": "ensure-topic",
		"topic": map[string]any{"conversation": conversation, "title": "retry budget probe", "kind": "task"}})
	waitFor(t, "ensure-topic completes under unending backpressure", 10*time.Second,
		func() bool { n, _ := completionsFor(mgr, "op-retry-budget"); return n > 0 })
	elapsed := time.Since(start)
	if _, c := completionsFor(mgr, "op-retry-budget"); c.Error == "" {
		t.Fatalf("ensure-topic against an unreachable conversation read must not silently succeed: %+v", c)
	}
	// chat.ReclaimAfter — the documented default a console ignoring the field
	// is judged against (OPS-5) — is 5 minutes.
	if docDefault := 5 * time.Minute; elapsed >= docDefault {
		t.Fatalf("ensure-topic must complete well inside the documented %s reclaim window, took %s", docDefault, elapsed)
	}
}

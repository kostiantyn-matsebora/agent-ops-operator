//go:build conformance

package conformance

import (
	"encoding/json"
	"fmt"
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

	// The Bot API double, as a BINARY too — the same one the e2e pack deploys.
	botPort := freePort(t)
	bot := start(t, "fake-bot-api", build(t, "test/fakebotapi"), []string{fmt.Sprintf("LISTEN_ADDR=127.0.0.1:%d", botPort)})
	bot.Port = botPort
	waitHealthy(t, bot)
	botCalls := func(method string) []map[string]any {
		resp, err := http.Get(bot.URL() + "/control/calls?method=" + method)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var calls []map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&calls)
		return calls
	}

	// One good surface with a projected credential, one whose config the
	// adapter cannot accept.
	mgr.ServeChannels(
		ChannelInfo{Name: "ops", Config: json.RawMessage(`{"chatId":"-1001234567890"}`), CredentialEnvPrefix: "AGENTOPS_CRED_OPS_"},
		ChannelInfo{Name: "broken", Config: json.RawMessage(`{"feedThreadId":1}`), CredentialEnvPrefix: "AGENTOPS_CRED_BROKEN_"},
	)
	port := freePort(t)
	env := append(contractEnv(mgr, "telegram", port),
		"TELEGRAM_API_BASE="+bot.URL(),
		"AGENTOPS_CRED_OPS_botToken=123:ops-token",
		"AGENTOPS_CRED_BROKEN_botToken=123:broken-token",
	)
	p := start(t, "channel-telegram", build(t, "channels/telegram"), env)
	p.Port = port
	waitHealthy(t, p)
	waitFor(t, "the adapter to list its channels", 10*time.Second, func() bool { return mgr.Listed("channel", "telegram") > 0 })

	// 6. Listing and status: the broken surface is reported, the good one served.
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

	// 1–3. Long-poll, contract, typed messages: ensure-topic then send.
	mgr.QueueOp(map[string]any{"id": "op-topic", "channel": "ops", "conversation": "c1", "kind": "ensure-topic",
		"topic": map[string]any{"conversation": "c1", "title": "disk is full", "kind": "alert", "source": "vm-alerts"}})
	waitFor(t, "ensure-topic completion", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-topic"); return n > 0 })
	_, topic := completionsFor(mgr, "op-topic")
	if topic.Error != "" || topic.ThreadID == "" {
		t.Fatalf("ensure-topic must complete with a thread id: %+v", topic)
	}
	if len(botCalls("createForumTopic")) != 1 {
		t.Fatalf("ensure-topic must create exactly one forum topic, calls=%v", botCalls("createForumTopic"))
	}
	assertContractHandshake(t, mgr, "telegram")

	send := map[string]any{"id": "op-send", "channel": "ops", "conversation": "c1", "kind": "send", "threadId": topic.ThreadID,
		"message": map[string]any{"kind": "answer", "body": "**found it** — the disk is full", "status": "succeeded"}}
	mgr.QueueOp(send)
	waitFor(t, "send completion", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-send"); return n > 0 })
	if _, c := completionsFor(mgr, "op-send"); c.Error != "" {
		t.Fatalf("send failed: %+v", c)
	}
	sends := botCalls("sendMessage")
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
	if sends[0]["token"] != "123:ops-token" {
		t.Fatalf("the send must use the surface's projected credential, got %v", sends[0]["token"])
	}

	// 4. At-least-once: the SAME op id again — acknowledged, not repeated.
	mgr.QueueOp(send)
	waitFor(t, "second completion of op-send", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-send"); return n >= 2 })
	if got := len(botCalls("sendMessage")); got != 1 {
		t.Fatalf("a redelivered op must not post again: %d sendMessage calls", got)
	}

	// 5. Inbound push with threadId — the router's forwarded topic update.
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

	// 7. No relay loop: the outbound posts never came back as inbound.
	time.Sleep(2 * time.Second)
	for _, in := range mgr.Inbound() {
		if strings.Contains(fmt.Sprint(in["text"]), "found it") {
			t.Fatalf("an outbound post returned as inbound: %v", in)
		}
	}
	if mgr.Unauthorized() != 0 {
		t.Fatalf("%d unauthenticated requests reached the manager", mgr.Unauthorized())
	}
	// Close-topic completes with an empty body, and a redelivery is not an error.
	close := map[string]any{"id": "op-close", "channel": "ops", "conversation": "c1", "kind": "close-topic", "threadId": topic.ThreadID}
	mgr.QueueOp(close)
	mgr.QueueOp(close)
	waitFor(t, "close-topic completions", 20*time.Second, func() bool { n, _ := completionsFor(mgr, "op-close"); return n >= 2 })
	if _, c := completionsFor(mgr, "op-close"); c.Error != "" {
		t.Fatalf("close-topic: %+v", c)
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
}

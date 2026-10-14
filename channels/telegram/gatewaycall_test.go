package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// This adapter holds no Telegram credential at all (chat-ops-broker-telegram-gateway):
// every Bot API call it makes goes through gateway-telegram's
// /telegram/call/{method}, authenticated with a shared token resolved the
// same way the bot token used to be — per channel, from the credential
// Secret's projected env (keys gatewayUrl/gatewayToken), with a process-level
// fallback. These tests replace the old apibase_test.go, which pinned the
// per-channel botToken/apiBase machinery this change deletes.

// Every call this client makes — topic creation, send, document upload,
// topic close — goes to <gatewayURL>/telegram/call/<method>, carrying the
// bearer token, never to api.telegram.org directly.
func TestEveryCallGoesThroughTheGateway(t *testing.T) {
	var methods []string
	var gotAuth []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/telegram/call/") {
			t.Fatalf("unexpected path %q — every call must go through /telegram/call", r.URL.Path)
		}
		methods = append(methods, strings.TrimPrefix(r.URL.Path, "/telegram/call/"))
		gotAuth = append(gotAuth, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"message_thread_id":7}}`))
	}))
	defer srv.Close()

	tg := NewTelegram(srv.URL, "gw-token")
	ctx := context.Background()
	if _, err := tg.CreateTopic(ctx, "-100", "t"); err != nil {
		t.Fatal(err)
	}
	if err := tg.Send(ctx, "-100", nil, "hi"); err != nil {
		t.Fatal(err)
	}
	if err := tg.SendDocument(ctx, "-100", nil, "a.txt", "", "x"); err != nil {
		t.Fatal(err)
	}
	if err := tg.CloseTopic(ctx, "-100", 7); err != nil {
		t.Fatal(err)
	}
	want := []string{"createForumTopic", "sendMessage", "sendDocument", "closeForumTopic"}
	if strings.Join(methods, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want %v", methods, want)
	}
	for _, a := range gotAuth {
		if a != "Bearer gw-token" {
			t.Fatalf("every call must carry the gateway token, got %q", a)
		}
	}
}

// A Telegram-side error relayed by the gateway (ok:false, no retry_after) is
// reported as an ordinary API error — no special gateway-aware parsing.
func TestGatewayRelayedTelegramErrorIsReported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	}))
	defer srv.Close()

	tg := NewTelegram(srv.URL, "gw-token")
	err := tg.Send(context.Background(), "-100", nil, "hi")
	if err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("err = %v, want it to carry Telegram's own description", err)
	}
}

// The gateway's OWN rejection (bad bearer token, missing method, upstream
// dial failure) is plain text via http.Error, never Telegram's JSON shape.
// A naive JSON decode of that body produces an opaque parse error — the
// status and body must surface instead, so a rotated gatewayToken reads as
// an auth problem, not a cryptic "invalid character" failure.
func TestGatewayOwnRejectionIsReportedNotDecoded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "missing or invalid bearer token", http.StatusUnauthorized)
	}))
	defer srv.Close()

	tg := NewTelegram(srv.URL, "wrong-token")
	err := tg.Send(context.Background(), "-100", nil, "hi")
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "missing or invalid bearer token") {
		t.Fatalf("err = %v, want it to name the gateway's status and body", err)
	}
}

// A served channel's spec.config MUST NOT choose its gateway binding: anyone
// able to edit a Channel would otherwise redirect that bot's calls to a host
// of their choosing. Only the credential Secret — which already holds the
// binding — may. This is the reduced form of the old
// TestChannelConfigCannotRedirectItsToken, same precedence, new keys.
func TestChannelConfigCannotRedirectTheGatewayBinding(t *testing.T) {
	// The process fallback is set too, so the precedence is exercised: the
	// projected credential for the surface that names one, the fallback for
	// the one that does not, spec.config for neither.
	t.Setenv("AGENTOPS_CRED_OPS_gatewayUrl", "http://real-gateway.test")
	t.Setenv("AGENTOPS_CRED_OPS_gatewayToken", "real-token")
	listing := `[{"name":"ops","config":{"chatId":"-100","gatewayUrl":"http://attacker.test","apiBase":"http://attacker.test"},"credentialEnvPrefix":"AGENTOPS_CRED_OPS_"},
	            {"name":"plain","config":{"chatId":"-101","gatewayUrl":"http://attacker.test"},"credentialEnvPrefix":"AGENTOPS_CRED_PLAIN_"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "channels") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(listing))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	a := &adapter{
		mgr:                  NewManager(srv.URL, "test-token"),
		channelType:          "telegram",
		fallbackGatewayURL:   "http://fallback-gateway.test",
		fallbackGatewayToken: "fallback-token",
		channels:             map[string]servedChannel{},
		reported:             map[string]string{},
		clients:              map[string]*Telegram{},
	}
	a.refreshChannels(context.Background())
	ops, ok := a.channel("ops")
	if !ok {
		t.Fatalf("ops must be served; listing decoded as %v", a.channels)
	}
	if got := a.client(ops).GatewayURL; got != "http://real-gateway.test" {
		t.Fatalf("the credential Secret's gatewayUrl must win, got %q", got)
	}
	plain, ok := a.channel("plain")
	if !ok {
		t.Fatalf("plain must be served")
	}
	if got := a.client(plain).GatewayURL; got != "http://fallback-gateway.test" {
		t.Fatalf("spec.config must never choose the binding, and the fallback applies; got %q", got)
	}
}

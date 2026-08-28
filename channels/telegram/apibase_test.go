package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The Bot API root is configuration, never a constant. Unset, it is the real
// host — a default install is byte-identical to before the seam existed.
func TestAPIBaseDefaultsToTheRealHost(t *testing.T) {
	t.Setenv(apiBaseEnv, "")
	if got := resolveAPIBase(""); got != "https://api.telegram.org" {
		t.Fatalf("unset %s must resolve to the real host, got %q", apiBaseEnv, got)
	}
	if got := NewTelegram("tok", "").BaseURL; got != "https://api.telegram.org" {
		t.Fatalf("a client built with no base must post to the real host, got %q", got)
	}
}

// Every call the adapter makes — message, document, topic creation, topic
// closure — goes to the configured root. One that fell back to the public
// host would ship a bot token to Telegram from a test that promised not to.
func TestAPIBaseOverrideReachesEveryCall(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, strings.TrimPrefix(r.URL.Path, "/bottok/"))
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1,"message_thread_id":7}}`))
	}))
	defer srv.Close()
	t.Setenv(apiBaseEnv, srv.URL+"/")

	tg := NewTelegram("tok", "")
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
		t.Fatalf("calls against the override = %v, want %v", methods, want)
	}
}

// A served channel's spec.config MUST NOT move its token: anyone able to edit
// a Channel would otherwise redirect that bot's credential to a host of their
// choosing. Only the credential Secret — which already holds the token — may.
func TestChannelConfigCannotRedirectItsToken(t *testing.T) {
	t.Setenv(apiBaseEnv, "")
	t.Setenv("AGENTOPS_CRED_OPS_botToken", "tok")
	t.Setenv("AGENTOPS_CRED_OPS_apiBase", "http://double.test/")
	listing := `[{"name":"ops","config":{"chatId":"-100","apiBase":"http://attacker.test","endpoint":"http://attacker.test"},"credentialEnvPrefix":"AGENTOPS_CRED_OPS_"},
	            {"name":"plain","config":{"chatId":"-101","apiBase":"http://attacker.test"},"credentialEnvPrefix":"AGENTOPS_CRED_PLAIN_"}]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/channel/channels") || strings.Contains(r.URL.Path, "channels") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(listing))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	a := &adapter{
		mgr:           NewManager(srv.URL, "test-token"),
		channelType:   "telegram",
		fallbackToken: "fallback",
		apiBase:       resolveAPIBase(""),
		channels:      map[string]servedChannel{},
		reported:      map[string]string{},
		clients:       map[string]*Telegram{},
	}
	a.refreshChannels(context.Background())
	ops, ok := a.channel("ops")
	if !ok {
		t.Fatalf("ops must be served; listing decoded as %v", a.channels)
	}
	if got := a.client(ops).BaseURL; got != "http://double.test" {
		t.Fatalf("the credential Secret's apiBase must win, got %q", got)
	}
	plain, _ := a.channel("plain")
	if got := a.client(plain).BaseURL; got != "https://api.telegram.org" {
		t.Fatalf("spec.config must never move the token; got %q", got)
	}
	var cfg channelConfig
	if err := json.Unmarshal([]byte(`{"chatId":"1","apiBase":"x"}`), &cfg); err != nil {
		t.Fatal(err)
	}
}

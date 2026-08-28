package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TELEGRAM_API_BASE is optional, unlike the forwarding targets and the token:
// unset takes the real host silently.
func TestAPIBaseDefaultsToTheRealHost(t *testing.T) {
	t.Setenv(apiBaseEnv, "")
	if got := resolveAPIBase(); got != "https://api.telegram.org" {
		t.Fatalf("unset must resolve to the real host, got %q", got)
	}
	if got := NewTelegram("t", "").BaseURL; got != "https://api.telegram.org" {
		t.Fatalf("a client built with no base must poll the real host, got %q", got)
	}
}

// The one getUpdates consumer polls wherever it is pointed; classification and
// forwarding are unchanged by it, and there is still exactly one poller.
func TestGetUpdatesHonoursTheOverride(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer srv.Close()
	t.Setenv(apiBaseEnv, srv.URL+"/")
	t.Setenv("SIGNAL_TARGET", "http://signal/")
	t.Setenv("CHANNEL_TARGET", "http://channel")
	t.Setenv("TELEGRAM_BOT_TOKEN", "t")

	cfg := loadConfig()
	if cfg.APIBase != srv.URL {
		t.Fatalf("loadConfig must read %s (trailing slash dropped), got %q", apiBaseEnv, cfg.APIBase)
	}
	tg := NewTelegram(cfg.Token, cfg.APIBase)
	if _, err := tg.GetUpdates(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "/bott/getUpdates" {
		t.Fatalf("getUpdates must be issued against the override, got %v", paths)
	}
}

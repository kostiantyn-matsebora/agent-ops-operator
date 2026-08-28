//go:build conformance

package conformance

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"golang.org/x/net/websocket"
)

// FakeHA is the slice of Home Assistant's websocket API signal-ha speaks:
// the auth handshake, the commands it issues, and pushed system_log_event
// events. One session at a time is enough for conformance.
type FakeHA struct {
	srv   *httptest.Server
	Token string

	mu       sync.Mutex
	conns    []*websocket.Conn
	subs     map[*websocket.Conn]int64 // subscription id per connection
	commands []string
	authed   int
}

// NewFakeHA starts the fake. Endpoint() is what a source's config.endpoint
// names; the adapter derives /api/websocket from it.
func NewFakeHA(t *testing.T, token string) *FakeHA {
	t.Helper()
	f := &FakeHA{Token: token, subs: map[*websocket.Conn]int64{}}
	mux := http.NewServeMux()
	// No Origin check: the adapter's hand-written client sends none, and
	// Home Assistant itself does not require one from an API client.
	mux.Handle("/api/websocket", websocket.Server{
		Handshake: func(*websocket.Config, *http.Request) error { return nil },
		Handler:   f.serve,
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// Endpoint is the Home Assistant base URL.
func (f *FakeHA) Endpoint() string { return f.srv.URL }

// Commands lists the command types the adapter issued.
func (f *FakeHA) Commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.commands...)
}

// Authed reports how many sessions authenticated.
func (f *FakeHA) Authed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.authed
}

// PushLog delivers one system_log_event to every subscribed session.
func (f *FakeHA) PushLog(rec map[string]any) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for conn, id := range f.subs {
		msg := map[string]any{"id": id, "type": "event",
			"event": map[string]any{"event_type": "system_log_event", "data": rec, "origin": "LOCAL"}}
		if websocket.JSON.Send(conn, msg) == nil {
			n++
		}
	}
	return n
}

func (f *FakeHA) serve(conn *websocket.Conn) {
	defer conn.Close()
	_ = websocket.JSON.Send(conn, map[string]any{"type": "auth_required", "ha_version": "2026.8.0"})
	var auth struct {
		Type        string `json:"type"`
		AccessToken string `json:"access_token"`
	}
	if err := websocket.JSON.Receive(conn, &auth); err != nil {
		return
	}
	if auth.Type != "auth" || auth.AccessToken != f.Token {
		_ = websocket.JSON.Send(conn, map[string]any{"type": "auth_invalid", "message": "Invalid access token or password"})
		return
	}
	_ = websocket.JSON.Send(conn, map[string]any{"type": "auth_ok", "ha_version": "2026.8.0"})
	f.mu.Lock()
	f.authed++
	f.conns = append(f.conns, conn)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.subs, conn)
		f.mu.Unlock()
	}()
	for {
		var raw json.RawMessage
		if err := websocket.JSON.Receive(conn, &raw); err != nil {
			return
		}
		var cmd struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		}
		_ = json.Unmarshal(raw, &cmd)
		f.mu.Lock()
		f.commands = append(f.commands, cmd.Type)
		f.mu.Unlock()
		reply := map[string]any{"id": cmd.ID, "type": "result", "success": true}
		switch cmd.Type {
		case "ping":
			reply = map[string]any{"id": cmd.ID, "type": "pong"}
		case "subscribe_events":
			f.mu.Lock()
			f.subs[conn] = cmd.ID
			f.mu.Unlock()
			reply["result"] = nil
		case "system_log/list", "config_entries/get":
			reply["result"] = []any{}
		case "auth/current_user":
			reply["result"] = map[string]any{"id": "user-1", "name": "agent-ops", "is_owner": false}
		default:
			reply["result"] = nil
		}
		if err := websocket.JSON.Send(conn, reply); err != nil {
			return
		}
	}
}

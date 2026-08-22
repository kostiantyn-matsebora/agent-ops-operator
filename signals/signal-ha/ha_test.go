package main

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func TestWSURLFor(t *testing.T) {
	cases := map[string]string{
		"http://ha.local:8123":    "ws://ha.local:8123/api/websocket",
		"https://ha.example.org":  "wss://ha.example.org/api/websocket",
		"https://ha.example.org/": "wss://ha.example.org/api/websocket",
		"http://ha.local/hass":    "ws://ha.local/hass/api/websocket",
	}
	for in, want := range cases {
		got, err := wsURLFor(in)
		if err != nil {
			t.Fatalf("wsURLFor(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("wsURLFor(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"ha.local", "ftp://ha", "https://"} {
		if _, err := wsURLFor(bad); err == nil {
			t.Errorf("wsURLFor(%q) should fail", bad)
		}
	}
}

func TestConnectAuthenticatesAndCommands(t *testing.T) {
	ha := newFakeHA(t, "secret")
	ha.SetRecords(logRecord{
		Name: "homeassistant.components.hue", Level: "ERROR",
		Message: []string{"broken"}, Count: 2, Timestamp: 1755782400,
	})
	ha.SetEntries(configEntry{Domain: "hue", State: "setup_retry"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events := make(chan logRecord, 4)
	sess, err := haConnect(ctx, ha.URL, "secret", func(eventType string, data json.RawMessage) {
		if eventType != "system_log_event" {
			return
		}
		var rec logRecord
		if json.Unmarshal(data, &rec) == nil {
			events <- rec
		}
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	if err := sess.SubscribeEvents(ctx, "system_log_event"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	user, err := sess.CurrentUser(ctx)
	if err != nil || user.Name != "ops-bot" {
		t.Fatalf("current user = %+v, err %v", user, err)
	}
	recs, err := sess.SystemLog(ctx)
	if err != nil || len(recs) != 1 || recs[0].Count != 2 {
		t.Fatalf("system_log = %+v, err %v", recs, err)
	}
	entries, err := sess.ConfigEntries(ctx)
	if err != nil || len(entries) != 1 || entries[0].State != "setup_retry" {
		t.Fatalf("config entries = %+v, err %v", entries, err)
	}

	ha.PushLogEvent(logRecord{Name: "homeassistant.components.zwave_js", Level: "ERROR", Message: []string{"gone"}})
	select {
	case rec := <-events:
		if rec.Name != "homeassistant.components.zwave_js" {
			t.Fatalf("unexpected event %+v", rec)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event delivered")
	}
}

// A rejected token is a configuration fact. It must surface as an error naming
// the rejection, not as a retry loop indistinguishable from an unreachable host.
func TestBadTokenIsReported(t *testing.T) {
	ha := newFakeHA(t, "secret")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := haConnect(ctx, ha.URL, "wrong", nil)
	if err == nil || !strings.Contains(err.Error(), "rejected the access token") {
		t.Fatalf("expected a token rejection, got %v", err)
	}
}

// A non-admin token gets no config-entry predicate. That is rung 2 of the
// ladder, not an error to report on the source.
func TestConfigEntriesErrorIsAnOrdinaryError(t *testing.T) {
	ha := newFakeHA(t, "secret")
	ha.mu.Lock()
	ha.adminErr = true
	ha.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sess, err := haConnect(ctx, ha.URL, "secret", nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()
	if _, err := sess.ConfigEntries(ctx); err == nil {
		t.Fatal("expected the unauthorized result to surface as an error")
	}
	// The session must still be usable afterwards.
	if _, err := sess.SystemLog(ctx); err != nil {
		t.Fatalf("a failed command must not kill the session: %v", err)
	}
}

// Fragmentation and an interleaved control frame are the two framing rules a
// hand-written client gets wrong, and both are invisible until a real server
// uses them.
func TestFragmentedMessageWithInterleavedPing(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan error, 1)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		br := newReader(conn)
		if err := serverHandshake(conn, br); err != nil {
			done <- err
			return
		}
		parts := [][]byte{[]byte(`{"type":"auth_`), []byte(`required"}`)}
		done <- writeServerFragments(conn, parts, true)
		// Stay open: the client answers the interleaved ping with a pong, and
		// closing here would make that write fail for reasons the test is not
		// about.
		<-stop
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := wsDial(ctx, "ws://"+ln.Addr().String()+"/api/websocket", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	msg, err := c.ReadMessage(time.Now().Add(5 * time.Second))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(msg) != `{"type":"auth_required"}` {
		t.Fatalf("reassembled message = %q", msg)
	}
	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
}

func TestLogRecordShape(t *testing.T) {
	raw := `{"name":"homeassistant.components.hue","message":["a","b"],"level":"ERROR",
	         "source":["components/hue/light.py",88],"timestamp":1755782400.5,"count":3,
	         "first_occurred":1755782000,"exception":"Traceback"}`
	var rec logRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rec.Text() != "a b" {
		t.Fatalf("Text() = %q", rec.Text())
	}
	if rec.Location() != "components/hue/light.py:88" {
		t.Fatalf("Location() = %q", rec.Location())
	}
	if rec.Key() != "homeassistant.components.hue@components/hue/light.py:88" {
		t.Fatalf("Key() = %q", rec.Key())
	}
	if !rec.At().Equal(time.Unix(1755782400, 500000000).UTC()) {
		t.Fatalf("At() = %v", rec.At())
	}
	if !epoch(0).IsZero() {
		t.Fatal("a missing timestamp must be the zero time")
	}
}

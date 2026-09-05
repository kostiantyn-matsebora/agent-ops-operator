package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeHA is a Home Assistant WebSocket API double.
//
// It writes UNMASKED frames, as a conformant server must, so the client's
// unmask-on-read path is exercised the way it is in production — and it asserts
// that every CLIENT frame arrives masked, which is the rule a hand-written
// client is most likely to get wrong and which Home Assistant enforces by
// closing the connection.
type fakeHA struct {
	t   *testing.T
	ln  net.Listener
	URL string

	mu       sync.Mutex
	token    string
	records  []logRecord
	entries  []configEntry
	user     currentUser
	adminErr bool // config_entries/get fails, as it does for a non-admin token
	conns    []net.Conn
	subs     int
	listCall int

	subscribed chan struct{}
}

func newFakeHA(t *testing.T, token string) *fakeHA {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeHA{
		t:          t,
		ln:         ln,
		URL:        "http://" + ln.Addr().String(),
		token:      token,
		user:       currentUser{ID: "user-1", Name: "ops-bot"},
		subscribed: make(chan struct{}, 8),
	}
	go f.accept()
	t.Cleanup(f.Close)
	return f
}

func (f *fakeHA) Close() {
	f.ln.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.conns {
		c.Close()
	}
	f.conns = nil
}

func (f *fakeHA) SetRecords(recs ...logRecord) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = recs
}

func (f *fakeHA) SetEntries(entries ...configEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = entries
}

func (f *fakeHA) ListCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCall
}

// PushLogEvent delivers a system_log_event to every connected client.
//
// Written under f.mu, which dispatch() also holds while it answers a command:
// a server's frames on one connection must be serialised, and an event written
// while a command's result frame is mid-write interleaves the two — the client
// then reads a frame header as JSON and the session dies with "invalid
// character '\u0081'". It surfaced under -race, where the adapter's connect-time
// commands and a test's push overlap far more often.
func (f *fakeHA) PushLogEvent(rec logRecord) {
	payload, _ := json.Marshal(map[string]any{
		"id":   1,
		"type": "event",
		"event": map[string]any{
			"event_type": "system_log_event",
			"data":       rec,
		},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.conns {
		_ = writeServerFrame(c, opText, payload)
	}
}

func (f *fakeHA) accept() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.serve(conn)
	}
}

func (f *fakeHA) serve(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if !strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
		fmt.Fprint(conn, "HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
		return
	}
	accept := acceptKey(req.Header.Get("Sec-WebSocket-Key"))
	fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\n"+
		"Connection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)

	sc := &wsConn{conn: conn, br: br}
	_ = sendServerJSON(conn, map[string]any{"type": "auth_required", "ha_version": "2026.8.0"})

	raw, err := f.readClientMessage(sc)
	if err != nil {
		return
	}
	var auth struct {
		Type  string `json:"type"`
		Token string `json:"access_token"`
	}
	if json.Unmarshal(raw, &auth) != nil || auth.Type != "auth" {
		return
	}
	if auth.Token != f.token {
		_ = sendServerJSON(conn, map[string]any{"type": "auth_invalid", "message": "Invalid access token"})
		return
	}
	// Registered BEFORE auth_ok goes out: the client returns from connect the
	// moment it reads that frame, so a test that then calls Close() must find
	// this connection in the list or the sever it asked for never happens.
	// The frame goes out under the lock, as every other server write does —
	// see PushLogEvent for why.
	f.mu.Lock()
	f.conns = append(f.conns, conn)
	_ = sendServerJSON(conn, map[string]any{"type": "auth_ok", "ha_version": "2026.8.0"})
	f.mu.Unlock()

	for {
		raw, err := f.readClientMessage(sc)
		if err != nil {
			return
		}
		var cmd struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		}
		if json.Unmarshal(raw, &cmd) != nil {
			return
		}
		if err := f.dispatch(conn, cmd.ID, cmd.Type); err != nil {
			return
		}
	}
}

func (f *fakeHA) dispatch(conn net.Conn, id int64, typ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch typ {
	case "ping":
		return sendServerJSON(conn, map[string]any{"id": id, "type": "pong"})
	case "subscribe_events":
		f.subs++
		select {
		case f.subscribed <- struct{}{}:
		default:
		}
		return sendServerJSON(conn, map[string]any{"id": id, "type": "result", "success": true, "result": nil})
	case "system_log/list":
		f.listCall++
		return sendServerJSON(conn, map[string]any{
			"id": id, "type": "result", "success": true, "result": f.records,
		})
	case "config_entries/get":
		if f.adminErr {
			return sendServerJSON(conn, map[string]any{
				"id": id, "type": "result", "success": false,
				"error": map[string]string{"code": "unauthorized", "message": "Unauthorized"},
			})
		}
		return sendServerJSON(conn, map[string]any{
			"id": id, "type": "result", "success": true, "result": f.entries,
		})
	case "auth/current_user":
		return sendServerJSON(conn, map[string]any{
			"id": id, "type": "result", "success": true, "result": f.user,
		})
	default:
		return sendServerJSON(conn, map[string]any{
			"id": id, "type": "result", "success": false,
			"error": map[string]string{"code": "unknown_command", "message": typ},
		})
	}
}

// readClientMessage reads one client frame and asserts it was masked. Home
// Assistant closes the connection on an unmasked client frame, so a client that
// forgot to mask would look like an unreachable instance.
func (f *fakeHA) readClientMessage(sc *wsConn) ([]byte, error) {
	_ = sc.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	head, err := sc.br.Peek(2)
	if err != nil {
		return nil, err
	}
	if head[1]&0x80 == 0 {
		f.t.Error("client frame arrived UNMASKED — RFC 6455 requires every client frame to be masked")
	}
	return sc.ReadMessage(time.Now().Add(10 * time.Second))
}

// writeServerFrame writes one unmasked frame, as a server must.
func writeServerFrame(conn net.Conn, opcode byte, payload []byte) error {
	var head [10]byte
	head[0] = 0x80 | opcode
	n := 2
	switch {
	case len(payload) < 126:
		head[1] = byte(len(payload))
	case len(payload) <= 0xFFFF:
		head[1] = 126
		binary.BigEndian.PutUint16(head[2:4], uint16(len(payload)))
		n = 4
	default:
		head[1] = 127
		binary.BigEndian.PutUint64(head[2:10], uint64(len(payload)))
		n = 10
	}
	if _, err := conn.Write(head[:n]); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

// writeServerFragments writes one message as several frames, so the client's
// reassembly and its handling of a control frame INTERLEAVED between fragments
// are both exercised.
func writeServerFragments(conn net.Conn, parts [][]byte, interleavePing bool) error {
	for i, p := range parts {
		var head []byte
		op := opContinuation
		if i == 0 {
			op = opText
		}
		fin := byte(0)
		if i == len(parts)-1 {
			fin = 0x80
		}
		head = append(head, fin|op, byte(len(p)))
		if _, err := conn.Write(head); err != nil {
			return err
		}
		if _, err := conn.Write(p); err != nil {
			return err
		}
		if interleavePing && i == 0 {
			if err := writeServerFrame(conn, opPing, []byte("keepalive")); err != nil {
				return err
			}
		}
	}
	return nil
}

func sendServerJSON(conn net.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeServerFrame(conn, opText, b)
}

var _ = io.EOF

// newReader and serverHandshake let a bare-bones test server do the upgrade
// without the whole Home Assistant double.
func newReader(conn net.Conn) *bufio.Reader { return bufio.NewReader(conn) }

func serverHandshake(conn net.Conn, br *bufio.Reader) error {
	req, err := http.ReadRequest(br)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\n"+
		"Connection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", acceptKey(req.Header.Get("Sec-WebSocket-Key")))
	return err
}
